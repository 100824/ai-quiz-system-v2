package v2

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

func (h *Handler) registerDesignImages(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/design-plans/{id}/images", h.handleDesignImageState)
	mux.HandleFunc("POST /api/v2/design-plans/{id}/image-jobs", h.handleDesignImageJob)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/image-jobs/{jobId}", h.handleGetDesignImageJob)
	mux.HandleFunc("POST /api/v2/design-plans/{id}/final-image", h.handleFinalizeDesignImage)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/images/{imageId}", h.handleDesignImageFile)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/records/{studentId}/images", h.handleTeacherDesignImages)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/records/{studentId}/images/{imageId}", h.handleTeacherDesignImageFile)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/images-export", h.handleDesignImageExport)
}

func (h *Handler) designStudentQuery(r *http.Request) (int, int, int, error) {
	p, err := designID(r)
	if err != nil {
		return 0, 0, 0, err
	}
	c, err := designQueryID(r, "classId", false)
	if err != nil {
		return 0, 0, 0, err
	}
	s, err := designQueryID(r, "studentId", false)
	if err == nil {
		err = validateDesignStudent(h.repo.db, p, c, s)
	}
	return p, c, s, err
}
func (h *Handler) handleDesignImageState(w http.ResponseWriter, r *http.Request) {
	p, _, s, err := h.designStudentQuery(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	h.writeDesignImageState(w, r, p, s)
}
func (h *Handler) writeDesignImageState(w http.ResponseWriter, r *http.Request, p, s int) {
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
	state, err := h.repo.DesignImageState(p, s, v, m)
	if err == nil && r.PathValue("studentId") == "" {
		state.Messages = []DesignImageMessage{}
		state.MessagesHasMore = false
	}
	h.designResponse(w, state, err)
}
func (h *Handler) handleTeacherDesignImages(w http.ResponseWriter, r *http.Request) {
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
	h.writeDesignImageState(w, r, p, s)
}

type designImageBody struct {
	ClassID    int      `json:"classId"`
	StudentID  int      `json:"studentId"`
	RequestKey string   `json:"requestKey"`
	Message    string   `json:"message"`
	ImageID    string   `json:"imageId"`
	Answers    []string `json:"answers"`
}

func (h *Handler) readDesignImageBody(w http.ResponseWriter, r *http.Request) (int, *designImageBody, error) {
	p, err := designID(r)
	if err != nil {
		return 0, nil, err
	}
	var b designImageBody
	err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&b)
	return p, &b, err
}
func (h *Handler) handleDesignImageJob(w http.ResponseWriter, r *http.Request) {
	p, b, err := h.readDesignImageBody(w, r)
	if err != nil {
		h.designResponse(w, nil, fmt.Errorf("请求内容无效"))
		return
	}
	if h.designImages == nil || envFirst("ARK_API_KEY", "ARK_KEY") == "" {
		h.designResponse(w, nil, fmt.Errorf("图片服务尚未配置，请联系老师"))
		return
	}
	j, err := h.repo.EnqueueDesignImage(p, b.ClassID, b.StudentID, b.RequestKey, b.Message, b.Answers)
	h.designResponse(w, map[string]any{"job": j}, err)
}
func (h *Handler) handleGetDesignImageJob(w http.ResponseWriter, r *http.Request) {
	p, _, s, err := h.designStudentQuery(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	j, err := scanDesignJob(h.repo.db.QueryRow(`SELECT `+designJobColumns+` FROM design_image_jobs WHERE id=? AND session_id=(SELECT id FROM design_image_sessions WHERE plan_id=? AND student_id=?)`, r.PathValue("jobId"), p, s))
	h.designResponse(w, map[string]any{"job": j}, err)
}
func (h *Handler) handleFinalizeDesignImage(w http.ResponseWriter, r *http.Request) {
	p, b, err := h.readDesignImageBody(w, r)
	if err == nil {
		err = h.repo.FinalizeDesignImage(p, b.ClassID, b.StudentID, b.ImageID)
	}
	h.designResponse(w, nil, err)
}
func (h *Handler) handleDesignImageFile(w http.ResponseWriter, r *http.Request) {
	p, _, s, err := h.designStudentQuery(r)
	if err != nil {
		http.Error(w, "图片不可访问", http.StatusNotFound)
		return
	}
	h.serveDesignImage(w, r, p, s)
}
func (h *Handler) handleTeacherDesignImageFile(w http.ResponseWriter, r *http.Request) {
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
	h.serveDesignImage(w, r, p, s)
}
func (h *Handler) serveDesignImage(w http.ResponseWriter, r *http.Request, p, s int) {
	if h.designImages == nil {
		http.NotFound(w, r)
		return
	}
	i, err := h.repo.OwnedDesignImage(p, s, r.PathValue("imageId"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, err := h.designImages.imagePath(i.FileName)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := "image/png"
	if path.Ext(i.FileName) == ".jpg" || path.Ext(i.FileName) == ".jpeg" {
		contentType = "image/jpeg"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="design-v%d%s"`, i.Version, path.Ext(i.FileName)))
	}
	http.ServeFile(w, r, file)
}

func designZipSegment(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, s)
	s = strings.Trim(s, " .")
	if s == "" {
		s = "未命名"
	}
	rs := []rune(s)
	if len(rs) > 60 {
		s = string(rs[:60])
	}
	return s
}

func (h *Handler) handleDesignImageExport(w http.ResponseWriter, r *http.Request) {
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
		h.designResponse(w, nil, fmt.Errorf("图片存储尚未配置"))
		return
	}
	items, err := h.repo.DesignRecords(p, cid)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	summary := [][]string{{"班级", "姓名", "文字方案状态", "图片数", "最终图片", "最终提交时间"}}
	versions := [][]string{{"班级", "姓名", "图片版本", "图片ID", "前置图片ID", "模型", "生成时间", "是否最终作品", "文件", "状态"}}
	messages := [][]string{{"班级", "姓名", "角色", "对话", "时间", "任务状态", "图片ID"}}
	type zipImage struct{ name, file string }
	files := []zipImage{}
	exceptions := []string{}
	for _, student := range items {
		prefix := fmt.Sprintf("%s_%d/%s_%d", designZipSegment(student.ClassName), student.ClassID, designZipSegment(student.Name), student.StudentID)
		finalFile := ""
		beforeV, beforeM := 0, 0
		for {
			state, e := h.repo.DesignImageState(p, student.StudentID, beforeV, beforeM)
			if e != nil {
				h.designResponse(w, nil, e)
				return
			}
			for _, i := range state.Images {
				name := path.Join(prefix, fmt.Sprintf("第%d版_%s%s", i.Version, i.ID, path.Ext(i.FileName)))
				file, e := h.designImages.imagePath(i.FileName)
				status := "正常"
				if e == nil {
					_, e = os.Stat(file)
				}
				if e != nil {
					status = "图片文件缺失"
					exceptions = append(exceptions, name+"：图片文件缺失")
				} else {
					files = append(files, zipImage{name, file})
				}
				final := "否"
				if i.ID == state.FinalImageID {
					final = "是"
					finalFile = name
				}
				versions = append(versions, []string{student.ClassName, student.Name, strconv.Itoa(i.Version), i.ID, i.ParentImageID, i.Model, i.CreatedAt, final, name, status})
			}
			for _, m := range state.Messages {
				messages = append(messages, []string{student.ClassName, student.Name, m.Role, m.Content, m.CreatedAt, m.Status, m.ImageID})
			}
			if !state.ImagesHasMore && !state.MessagesHasMore {
				break
			}
			if len(state.Images) > 0 {
				beforeV = state.Images[len(state.Images)-1].Version
			} else {
				beforeV = 1
			}
			if len(state.Messages) > 0 {
				beforeM = state.Messages[len(state.Messages)-1].ID
			} else {
				beforeM = 1
			}
		}
		summary = append(summary, []string{student.ClassName, student.Name, designStatus(len(student.Versions)), strconv.Itoa(student.ImageCount), finalFile, student.FinalizedAt})
	}
	wb := excelize.NewFile()
	defer wb.Close()
	for _, sheet := range []struct {
		name string
		rows [][]string
	}{{"完成状态", summary}, {"图片版本", versions}, {"对话记录", messages}} {
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
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="design-plan-%d-images.zip"`, p))
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
