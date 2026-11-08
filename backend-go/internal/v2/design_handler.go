package v2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/xuri/excelize/v2"
)

func (h *Handler) registerDesignPlans(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/classes/{id}/design-active-plan", h.handleActiveDesignPlan)
	mux.HandleFunc("PUT /api/v2/classes/{id}/design-active-plan", h.handleActiveDesignPlan)
	h.registerDesignImages(mux)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/classes/{classId}/progress", h.handleDesignProgress)
	mux.HandleFunc("PUT /api/v2/design-plans/{id}/classes/{classId}/progress", h.handleDesignProgress)
	mux.HandleFunc("GET /api/v2/design-plans", h.handleDesignPlans)
	mux.HandleFunc("POST /api/v2/design-plans", h.handleSaveDesignPlan)
	mux.HandleFunc("PUT /api/v2/design-plans/{id}", h.handleSaveDesignPlan)
	mux.HandleFunc("GET /api/v2/classes/{id}/design-plans", h.handleClassDesignPlans)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/student", h.handleStudentDesignPlan)
	mux.HandleFunc("POST /api/v2/design-plans/{id}/versions", h.handleSubmitDesignVersion)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/records", h.handleDesignRecords)
	mux.HandleFunc("GET /api/v2/design-plans/{id}/export", h.handleDesignExport)
}

func (h *Handler) handleActiveDesignPlan(w http.ResponseWriter, r *http.Request) {
	cid, err := designID(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			PlanID int `json:"planId"`
		}
		err = json.NewDecoder(r.Body).Decode(&body)
		if err == nil {
			err = h.repo.SetActiveDesignPlan(cid, body.PlanID)
		}
		if err != nil {
			h.designResponse(w, nil, err)
			return
		}
	}
	if r.URL.Query().Get("studentId") != "" {
		sid, e := designQueryID(r, "studentId", false)
		var ok bool
		if e == nil {
			e = h.repo.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM students WHERE id=? AND class_id=? AND deleted_at IS NULL)`, sid, cid).Scan(&ok)
		}
		if e != nil || !ok {
			h.designResponse(w, nil, fmt.Errorf("学生不在班级名单中"))
			return
		}
	}
	p, err := h.repo.ActiveDesignPlan(cid)
	h.designResponse(w, map[string]any{"plan": p}, err)
}

func (h *Handler) designResponse(w http.ResponseWriter, data any, err error) {
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: data})
}

func designID(r *http.Request) (int, error) {
	id, err := pathInt(r, "id")
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("ID无效")
	}
	return id, nil
}

func designQueryID(r *http.Request, key string, optional bool) (int, error) {
	s := r.URL.Query().Get(key)
	if s == "" && optional {
		return 0, nil
	}
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s无效", key)
	}
	return id, nil
}

func (h *Handler) handleDesignPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.repo.ListDesignPlans(0)
	h.designResponse(w, map[string]any{"plans": plans}, err)
}

func (h *Handler) handleSaveDesignPlan(w http.ResponseWriter, r *http.Request) {
	var p DesignPlan
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&p); err != nil {
		h.designResponse(w, nil, fmt.Errorf("方案内容格式错误"))
		return
	}
	p.ID = 0
	if r.Method == http.MethodPut {
		id, err := designID(r)
		if err != nil {
			h.designResponse(w, nil, err)
			return
		}
		p.ID = id
	}
	plan, err := h.repo.SaveDesignPlan(p)
	h.designResponse(w, map[string]any{"plan": plan}, err)
}

func (h *Handler) handleClassDesignPlans(w http.ResponseWriter, r *http.Request) {
	classID, err := designID(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	studentID, err := designQueryID(r, "studentId", false)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	var ok bool
	err = h.repo.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM students s JOIN classes c ON c.id=s.class_id WHERE s.id=? AND c.id=? AND s.deleted_at IS NULL AND c.deleted_at IS NULL)`, studentID, classID).Scan(&ok)
	if err != nil || !ok {
		h.designResponse(w, nil, fmt.Errorf("请选择班级名单中的学生"))
		return
	}
	plans, err := h.repo.ListDesignPlans(classID)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	items := []map[string]any{}
	for _, p := range plans {
		vs, e := loadDesignVersions(h.repo.db, p.ID, studentID)
		if e != nil {
			h.designResponse(w, nil, e)
			return
		}
		items = append(items, map[string]any{"id": p.ID, "title": p.Title, "versionCount": len(vs)})
	}
	h.designResponse(w, map[string]any{"plans": items}, nil)
}

func (h *Handler) handleStudentDesignPlan(w http.ResponseWriter, r *http.Request) {
	id, err := designID(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	cid, err := designQueryID(r, "classId", false)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	sid, err := designQueryID(r, "studentId", false)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	if err = validateDesignStudent(h.repo.db, id, cid, sid); err != nil {
		h.designResponse(w, nil, err)
		return
	}
	p, err := loadDesignPlan(h.repo.db, id)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	vs, err := loadDesignVersions(h.repo.db, id, sid)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	stage, err := designStage(h.repo.db, id, cid)
	h.designResponse(w, map[string]any{"plan": p, "versions": vs, "stage": stage, "permissions": map[string]bool{"canSubmitFirst": len(vs) == 0, "canSubmitSecond": len(vs) == 1 && stage >= 2, "canEnterImages": len(vs) == 2 && stage >= 3}}, err)
}

func (h *Handler) handleDesignProgress(w http.ResponseWriter, r *http.Request) {
	id, err := designID(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	cid, err := pathInt(r, "classId")
	if err != nil || cid <= 0 {
		h.designResponse(w, nil, fmt.Errorf("班级ID无效"))
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			Stage int `json:"stage"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err == nil {
			err = h.repo.AdvanceDesignProgress(id, cid, body.Stage)
		}
		if err != nil {
			h.designResponse(w, nil, err)
			return
		}
	}
	stage, err := h.repo.DesignProgress(id, cid)
	h.designResponse(w, map[string]any{"stage": stage}, err)
}

func (h *Handler) handleSubmitDesignVersion(w http.ResponseWriter, r *http.Request) {
	id, err := designID(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	var body struct {
		ClassID   int      `json:"classId"`
		StudentID int      `json:"studentId"`
		Version   int      `json:"version"`
		Answers   []string `json:"answers"`
	}
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body); err != nil {
		h.designResponse(w, nil, fmt.Errorf("提交内容格式错误"))
		return
	}
	err = h.repo.SubmitDesignVersion(id, body.ClassID, body.StudentID, body.Version, body.Answers)
	h.designResponse(w, nil, err)
}

func (h *Handler) handleDesignRecords(w http.ResponseWriter, r *http.Request) {
	id, err := designID(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	cid, err := designQueryID(r, "classId", true)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	items, err := h.repo.DesignRecords(id, cid)
	h.designResponse(w, map[string]any{"students": items}, err)
}

func designStatus(count int) string {
	if count == 2 {
		return "两版完成"
	}
	if count == 1 {
		return "仅第一版"
	}
	return "未提交"
}

func (h *Handler) handleDesignExport(w http.ResponseWriter, r *http.Request) {
	id, err := designID(r)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	cid, err := designQueryID(r, "classId", true)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	p, err := loadDesignPlan(h.repo.db, id)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	items, err := h.repo.DesignRecords(id, cid)
	if err != nil {
		h.designResponse(w, nil, err)
		return
	}
	summary := [][]string{{"方案", "班级", "姓名", "状态", "第一版提交时间", "第二版提交时间"}}
	detail := [][]string{{"方案", "班级", "姓名", "状态", "题号", "题目", "第一版答案", "第二版答案", "第一版提交时间", "第二版提交时间"}}
	for _, item := range items {
		first, second := "", ""
		qs := p.Questions
		title := p.Title
		if len(item.Versions) > 0 {
			v := item.Versions[0]
			first = v.SubmittedAt
			qs = v.Questions
			title = v.Title
		}
		if len(item.Versions) > 1 {
			second = item.Versions[1].SubmittedAt
		}
		status := designStatus(len(item.Versions))
		summary = append(summary, []string{title, item.ClassName, item.Name, status, first, second})
		for i, q := range qs {
			a, b := "", ""
			if len(item.Versions) > 0 {
				a = item.Versions[0].Answers[i]
			}
			if len(item.Versions) > 1 {
				b = item.Versions[1].Answers[i]
			}
			detail = append(detail, []string{title, item.ClassName, item.Name, status, strconv.Itoa(i + 1), q, a, b, first, second})
		}
	}
	wb := excelize.NewFile()
	if err = writeMatrixSheet(wb, "完成状态", summary, true); err == nil {
		err = writeMatrixSheet(wb, "两版答案对照", detail, true)
	}
	if err != nil {
		wb.Close()
		h.writeError(w, err)
		return
	}
	_ = wb.DeleteSheet("Sheet1")
	h.writeXLSX(w, wb, fmt.Sprintf("方案设计-%d.xlsx", id))
}
