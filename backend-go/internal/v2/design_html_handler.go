package v2

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

func (h *Handler) registerDesignHTML(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/design-plans/{id}/html", h.handleDesignHTMLState)
	mux.HandleFunc("POST /api/v2/design-plans/{id}/html-jobs", h.handleDesignHTMLJob)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/html-jobs/{jobId}", h.handleGetDesignHTMLJob)
	mux.HandleFunc("POST /api/v2/design-plans/{id}/final-html", h.handleFinalizeDesignHTML)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/html-versions/{versionId}", h.handleDesignHTMLFile)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/records/{studentId}/html", h.handleTeacherDesignHTML)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/records/{studentId}/html-versions/{versionId}", h.handleTeacherDesignHTMLFile)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/html-export", h.handleDesignHTMLExport)
}

func (h *Handler) handleDesignHTMLState(w http.ResponseWriter, r *http.Request) {
	p, _, s, err := h.designStudentQuery(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	h.writeDesignHTMLState(w, r, p, s)
}
func (h *Handler) writeDesignHTMLState(w http.ResponseWriter, r *http.Request, p, s int) {
	v, err := designQueryID(r, "beforeVersion", true)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	m, err := designQueryID(r, "beforeMessageId", true)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	state, err := h.repo.DesignHTMLState(p, s, v, m)
	if err == nil && r.PathValue("studentId") == "" {
		state.Messages = []DesignHTMLMessage{}
		state.MessagesHasMore = false
	}
	h.designResponse(w, state, err)
}
func (h *Handler) handleTeacherDesignHTML(w http.ResponseWriter, r *http.Request) {
	p, err := designID(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	s, err := pathInt(r, "studentId")
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	h.writeDesignHTMLState(w, r, p, s)
}

type designHTMLBody struct {
	ClassID    int      `json:"classId"`
	StudentID  int      `json:"studentId"`
	RequestKey string   `json:"requestKey"`
	Message    string   `json:"message"`
	VersionID  string   `json:"versionId"`
	Answers    []string `json:"answers"`
}

func (h *Handler) readDesignHTMLBody(w http.ResponseWriter, r *http.Request) (int, *designHTMLBody, error) {
	p, err := designID(r)
	if err != nil {
		return 0, nil, err
	}
	var b designHTMLBody
	err = readDesignJSON(w, r, &b)
	return p, &b, err
}
func (h *Handler) handleDesignHTMLJob(w http.ResponseWriter, r *http.Request) {
	p, b, err := h.readDesignHTMLBody(w, r)
	if err != nil {
		h.designResponse(w, nil, fmt.Errorf("请求内容无效"))
		return
	}
	if h.designImages == nil || !h.designHTMLConfigured() {
		h.designResponse(w, nil, fmt.Errorf("网页服务尚未配置，请联系老师"))
		return
	}
	j, err := h.repo.EnqueueDesignHTML(p, b.ClassID, b.StudentID, b.RequestKey, b.Message, b.Answers)
	h.designResponse(w, map[string]any{"job": j}, err)
}
func (h *Handler) handleGetDesignHTMLJob(w http.ResponseWriter, r *http.Request) {
	p, _, s, err := h.designStudentQuery(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	j, err := scanDesignHTMLJob(h.repo.db.QueryRow(`SELECT `+designHTMLJobColumns+` FROM design_html_jobs WHERE id=? AND session_id=(SELECT id FROM design_html_sessions WHERE plan_id=? AND student_id=?)`, r.PathValue("jobId"), p, s))
	h.designResponse(w, map[string]any{"job": j}, err)
}
func (h *Handler) handleFinalizeDesignHTML(w http.ResponseWriter, r *http.Request) {
	p, b, err := h.readDesignHTMLBody(w, r)
	if err == nil {
		err = h.repo.FinalizeDesignHTML(p, b.ClassID, b.StudentID, b.VersionID)
	}
	h.designResponse(w, nil, err)
}
func (h *Handler) handleDesignHTMLFile(w http.ResponseWriter, r *http.Request) {
	p, _, s, err := h.designStudentQuery(r)
	if err != nil {
		http.Error(w, "网页不可访问", http.StatusNotFound)
		return
	}
	h.serveDesignHTML(w, r, p, s)
}
func (h *Handler) handleTeacherDesignHTMLFile(w http.ResponseWriter, r *http.Request) {
	p, err := designID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s, err := pathInt(r, "studentId")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.serveDesignHTML(w, r, p, s)
}
func (h *Handler) serveDesignHTML(w http.ResponseWriter, r *http.Request, p, s int) {
	if h.designImages == nil {
		http.NotFound(w, r)
		return
	}
	i, err := h.repo.OwnedDesignHTMLVersion(p, s, r.PathValue("versionId"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, err := h.designImages.htmlPath(i.FileName)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	f, err := os.Open(file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	if r.URL.Query().Get("download") == "1" {
		stat, err := f.Stat()
		if err != nil || !stat.Mode().IsRegular() {
			http.Error(w, "网页文件不可读取", 500)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="design-v%d.html"`, i.Version))
		http.ServeContent(w, r, fmt.Sprintf("design-v%d.html", i.Version), stat.ModTime(), f)
		return
	}
	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "网页文件不可读取", 500)
		return
	}
	h.designResponse(w, map[string]any{"version": i, "html": string(data)}, nil)
}

func (h *Handler) handleDesignHTMLExport(w http.ResponseWriter, r *http.Request) {
	p, err := designID(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	cid, err := designQueryID(r, "classId", true)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	if h.designImages == nil {
		h.designResponse(w, nil, fmt.Errorf("网页存储尚未配置"))
		return
	}
	items, err := h.repo.DesignRecords(p, cid)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	summary := [][]string{{"班级", "姓名", "文字方案状态", "网页数", "最终网页", "最终提交时间"}}
	versions := [][]string{{"班级", "姓名", "网页版本", "网页ID", "前置网页ID", "模型", "生成时间", "是否最终作品", "文件", "状态"}}
	messages := [][]string{{"班级", "姓名", "角色", "对话", "时间", "任务状态", "网页ID"}}
	type zipImage struct{ name, file string }
	files := []zipImage{}
	exceptions := []string{}
	for _, student := range items {
		prefix := fmt.Sprintf("%s_%d/%s_%d", designZipSegment(student.ClassName), student.ClassID, designZipSegment(student.Name), student.StudentID)
		finalFile := ""
		beforeV, beforeM := 0, 0
		for {
			state, e := h.repo.DesignHTMLState(p, student.StudentID, beforeV, beforeM)
			if e != nil {
				h.designResponse(w, nil, e)
				return
			}
			for _, i := range state.Versions {
				name := path.Join(prefix, fmt.Sprintf("第%d版_%s%s", i.Version, i.ID, path.Ext(i.FileName)))
				file, e := h.designImages.htmlPath(i.FileName)
				status := "正常"
				if e == nil {
					_, e = os.Stat(file)
				}
				if e != nil {
					status = "网页文件缺失"
					exceptions = append(exceptions, name+"：网页文件缺失")
				} else {
					files = append(files, zipImage{name, file})
				}
				final := "否"
				if i.ID == state.FinalVersionID {
					final = "是"
					finalFile = name
				}
				versions = append(versions, []string{student.ClassName, student.Name, strconv.Itoa(i.Version), i.ID, i.ParentVersionID, i.Model, i.CreatedAt, final, name, status})
			}
			for _, m := range state.Messages {
				messages = append(messages, []string{student.ClassName, student.Name, m.Role, m.Content, m.CreatedAt, m.Status, m.VersionID})
			}
			if !state.VersionsHasMore && !state.MessagesHasMore {
				break
			}
			if len(state.Versions) > 0 {
				beforeV = state.Versions[len(state.Versions)-1].Version
			} else {
				beforeV = 1
			}
			if len(state.Messages) > 0 {
				beforeM = state.Messages[len(state.Messages)-1].ID
			} else {
				beforeM = 1
			}
		}
		summary = append(summary, []string{student.ClassName, student.Name, designStatus(len(student.Versions)), strconv.Itoa(student.HTMLVersionCount), finalFile, student.HTMLFinalizedAt})
	}
	wb := excelize.NewFile()
	defer wb.Close()
	for _, sheet := range []struct {
		name string
		rows [][]string
	}{{"完成状态", summary}, {"网页版本", versions}, {"对话记录", messages}} {
		if err = writeMatrixSheet(wb, sheet.name, sheet.rows, true); err != nil {
			h.writeError(w, err)
			return
		}
	}
	_ = wb.DeleteSheet("Sheet1")
	buf, err := wb.WriteToBuffer()
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="design-plan-%d-html.zip"`, p))
	z := zip.NewWriter(w)
	defer z.Close()
	entry, err := z.Create("成果清单.xlsx")
	if err != nil {
		return
	}
	if _, err = entry.Write(buf.Bytes()); err != nil {
		return
	}
	for _, f := range files {
		file, e := os.Open(f.file)
		if e != nil {
			exceptions = append(exceptions, f.name+"：读取失败")
			continue
		}
		entry, e = z.Create(f.name)
		if e == nil {
			_, e = io.Copy(entry, file)
		}
		file.Close()
		if e != nil {
			return
		}
	}
	if len(exceptions) > 0 {
		entry, e := z.Create("导出异常.txt")
		if e == nil {
			_, _ = io.WriteString(entry, strings.Join(exceptions, "\n"))
		}
	}
}

func (h *Handler) designHTMLConfigured() bool {
	for _, p := range h.aiProviders() {
		if p.Configured {
			return true
		}
	}
	return false
}
