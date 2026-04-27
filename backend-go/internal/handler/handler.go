package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-quiz-system-v2/backend-go/internal/models"
	"ai-quiz-system-v2/backend-go/internal/repository"
	"ai-quiz-system-v2/backend-go/internal/service"
	"ai-quiz-system-v2/backend-go/internal/utils"
	"github.com/xuri/excelize/v2"
)

// Handler holds HTTP handlers, wiring repository and service layers.
type Handler struct {
	repo *repository.Repository
	svc  *service.Service
	tips *TipStore
}

// TipStore holds in-memory class tips.
type TipStore struct {
	mu   sync.RWMutex
	data map[string]models.TipPayload
}

// NewHandler creates a Handler.
func NewHandler(repo *repository.Repository, svc *service.Service) *Handler {
	return &Handler{
		repo: repo,
		svc:  svc,
		tips: &TipStore{data: make(map[string]models.TipPayload)},
	}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload models.APIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	log.Printf("request failed: %v", err)
	h.writeJSON(w, http.StatusInternalServerError, models.APIResponse{Success: false, Error: err.Error()})
}

func (h *Handler) resolveCourseID(r *http.Request) (int, error) {
	if v := r.URL.Query().Get("courseId"); strings.TrimSpace(v) != "" {
		id, err := strconv.Atoi(v)
		if err != nil {
			return 0, errors.New("courseId 无效")
		}
		return id, nil
	}
	return h.repo.GetCurrentCourseID()
}

func (h *Handler) resolveCourseIDFromBody(courseID *int) (int, error) {
	if courseID != nil && *courseID > 0 {
		return *courseID, nil
	}
	return h.repo.GetCurrentCourseID()
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

// HandleHealth responds to health checks.
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]string{"status": "ok"}})
}

// ---------------------------------------------------------------------------
// Teacher
// ---------------------------------------------------------------------------

// HandleTeacherCourses lists all courses and the current active one.
func (h *Handler) HandleTeacherCourses(w http.ResponseWriter, r *http.Request) {
	courses, err := h.repo.GetAllCourses()
	if err != nil {
		h.writeError(w, err)
		return
	}
	currentCourseID, err := h.repo.GetCurrentCourseID()
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]interface{}{
		"courses":         courses,
		"currentCourseId": currentCourseID,
	}})
}

// HandleSetCurrentCourse updates the active course.
func (h *Handler) HandleSetCurrentCourse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CourseID models.FlexInt `json:"courseId"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	if int(body.CourseID) <= 0 {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	if err := h.repo.SetCurrentCourseID(int(body.CourseID)); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: "切换课程成功"})
}

// HandleTeacherClassList returns the roster for a class.
func (h *Handler) HandleTeacherClassList(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	className := r.PathValue("className")
	if courseID <= 0 {
		courseID, err = h.repo.GetClassBoundCourse(className)
		if err != nil {
			h.writeError(w, err)
			return
		}
		if courseID <= 0 {
			courseID, err = h.repo.GetCurrentCourseID()
			if err != nil {
				h.writeError(w, err)
				return
			}
		}
	}
	students, err := h.repo.GetClassList(courseID, className)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]interface{}{"students": students}})
}

// HandleImportClassList imports a class roster.
func (h *Handler) HandleImportClassList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CourseID     models.FlexInt `json:"courseId"`
		ClassName    string         `json:"className"`
		StudentNames []string       `json:"studentNames"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	if err := h.repo.ImportClassList(int(body.CourseID), body.ClassName, body.StudentNames); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: "导入成功"})
}

// HandleTeacherClasses lists all classes for a course.
func (h *Handler) HandleTeacherClasses(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	classes, err := h.repo.GetAllClasses(courseID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]interface{}{"classes": classes}})
}

// HandleTeacherStage returns the current stage for a course/class.
func (h *Handler) HandleTeacherStage(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	stage, err := h.repo.GetCurrentStage(courseID, r.URL.Query().Get("className"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]int{"stage": stage}})
}

// HandleSetTeacherStage updates the current stage.
func (h *Handler) HandleSetTeacherStage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CourseID  json.RawMessage `json:"courseId"`
		Stage     json.RawMessage `json:"stage"`
		ClassName string          `json:"className"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	courseID, err := utils.ParseRawFlexibleInt(body.CourseID)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	stage, err := utils.ParseRawFlexibleInt(body.Stage)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "stage 无效"})
		return
	}
	if courseID <= 0 || stage < 0 {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 或 stage 无效"})
		return
	}
	if stage > 0 {
		enabled, err := h.repo.IsPartEnabled(courseID, stage)
		if err != nil {
			h.writeError(w, err)
			return
		}
		if !enabled {
			h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: fmt.Sprintf("第%d部分当前已关闭，无法切换", stage)})
			return
		}
	}
	if err := h.repo.SetCurrentStage(courseID, stage, body.ClassName); err != nil {
		h.writeError(w, err)
		return
	}
	target := "所有班级"
	if strings.TrimSpace(body.ClassName) != "" {
		target = body.ClassName
	}
	stageLabel := fmt.Sprintf("第%d部分", stage)
	if stage == 0 {
		stageLabel = "准备环节"
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: fmt.Sprintf("已将%s切换到%s", target, stageLabel)})
}

// HandleGetPartSettings returns enabled parts for a course.
func (h *Handler) HandleGetPartSettings(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	settings, err := h.repo.GetPartSettings(courseID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]interface{}{"settings": settings}})
}

// HandleSetPartSettings updates enabled parts.
func (h *Handler) HandleSetPartSettings(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	var body struct {
		Settings []models.PartSetting `json:"settings"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	if len(body.Settings) == 0 {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "缺少部分配置"})
		return
	}
	if err := h.repo.UpdatePartSettings(courseID, body.Settings); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: "部分启用状态更新成功"})
}

// HandleBindClassCourse binds a class to a course.
func (h *Handler) HandleBindClassCourse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClassName string         `json:"className"`
		CourseID  models.FlexInt `json:"courseId"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	if int(body.CourseID) <= 0 {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	if err := h.repo.SetClassBoundCourse(body.ClassName, int(body.CourseID)); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: "班级课程绑定成功"})
}

// HandleTeacherTip stores a class tip in memory.
func (h *Handler) HandleTeacherTip(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClassName string `json:"className"`
		Content   string `json:"content"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.tips.mu.Lock()
	h.tips.data[body.ClassName] = models.TipPayload{Content: body.Content, UpdatedAt: time.Now().UnixMilli()}
	h.tips.mu.Unlock()
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: "提示语提交成功"})
}

// HandleTeacherQuestions lists questions for a course part.
func (h *Handler) HandleTeacherQuestions(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	part, err := strconv.Atoi(r.PathValue("part"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "part 无效"})
		return
	}
	questions, err := h.repo.GetQuestionsByPart(courseID, part)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]interface{}{"questions": questions}})
}

// HandleUpdateQuestion updates a question.
func (h *Handler) HandleUpdateQuestion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "question id 无效"})
		return
	}
	var body struct {
		QuestionType      string  `json:"questionType"`
		QuestionText      string  `json:"questionText"`
		Options           *string `json:"options"`
		CorrectAnswer     *string `json:"correctAnswer"`
		Explanation       *string `json:"explanation"`
		AnnotationEnabled *bool   `json:"annotationEnabled"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	annotationEnabled := true
	if enabled, err := h.repo.GetQuestionAnnotationEnabled(id); err == nil {
		annotationEnabled = enabled
	} else {
		h.writeError(w, err)
		return
	}
	if body.AnnotationEnabled != nil {
		annotationEnabled = *body.AnnotationEnabled
	}
	if err := h.repo.UpdateQuestion(id, body.QuestionType, body.QuestionText, body.Options, body.CorrectAnswer, body.Explanation, annotationEnabled); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: "更新成功"})
}

// HandleSetQuestionEnabled toggles a question's enabled state.
func (h *Handler) HandleSetQuestionEnabled(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "question id 无效"})
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	if err := h.repo.SetQuestionEnabled(id, body.Enabled); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: "题目启用状态更新成功"})
}

// HandleGetPart2Guide returns the part-2 guide text.
func (h *Handler) HandleGetPart2Guide(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	guide, err := h.repo.GetPart2Guide(courseID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]string{"guide": guide}})
}

// HandleSetPart2Guide updates the part-2 guide text.
func (h *Handler) HandleSetPart2Guide(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	if err := h.repo.UpdatePart2Guide(courseID, body.Content); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: "答题指引更新成功"})
}

// HandleTeacherStats returns aggregated statistics.
func (h *Handler) HandleTeacherStats(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	className := r.URL.Query().Get("className")
	stats, err := h.svc.BuildStats(courseID, className)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: stats})
}

// HandleExportStats generates an Excel export.
func (h *Handler) HandleExportStats(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "courseId 无效"})
		return
	}
	className := r.URL.Query().Get("className")
	students, err := h.repo.GetAllStudentSurveys(courseID, className)
	if err != nil {
		h.writeError(w, err)
		return
	}

	file := excelize.NewFile()
	sheet := "答题记录"
	file.SetSheetName("Sheet1", sheet)
	headers := []string{
		"班级", "姓名", "完成状态", "第一部分-预测得分", "第一部分-学习方法", "第一部分-自定义学习方法",
		"第二部分-理解程度", "第二部分-开放题答案", "第三部分-第1题答案", "第三部分-第2题答案", "第三部分-第3题答案",
		"第三部分-第4题答案", "第三部分-第5题答案", "第三部分-总得分", "第四部分-实际得分",
		"第四部分-预测得分", "第四部分-第2题答案", "第四部分-第2题自定义内容", "第四部分-第3题答案",
		"第四部分-第3题自定义内容", "最后提交时间",
	}
	for i, hText := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		file.SetCellValue(sheet, cell, hText)
	}
	for rowIndex, st := range students {
		record := service.BuildExportRow(st)
		for colIndex, value := range record {
			cell, _ := excelize.CoordinatesToCellName(colIndex+1, rowIndex+2)
			file.SetCellValue(sheet, cell, value)
		}
	}

	filename := fmt.Sprintf("答题记录_%s_%s.xlsx", utils.Fallback(className, "所有班级"), time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", utils.URLEncode(filename)))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.WriteHeader(http.StatusOK)
	if _, err := file.WriteTo(w); err != nil {
		log.Printf("write export file: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Student
// ---------------------------------------------------------------------------

// HandleStudentValidate checks whether a student is in the class roster.
func (h *Handler) HandleStudentValidate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CourseID    *int   `json:"courseId"`
		ClassName   string `json:"className"`
		StudentName string `json:"studentName"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	effectiveCourseID, err := h.repo.GetClassBoundCourse(body.ClassName)
	if err != nil {
		h.writeError(w, err)
		return
	}
	if effectiveCourseID == 0 {
		if body.CourseID != nil && *body.CourseID > 0 {
			effectiveCourseID = *body.CourseID
		} else {
			effectiveCourseID, err = h.repo.GetCurrentCourseID()
			if err != nil {
				h.writeError(w, err)
				return
			}
		}
	}
	valid, err := h.repo.ValidateStudent(effectiveCourseID, body.ClassName, body.StudentName)
	if err != nil {
		h.writeError(w, err)
		return
	}
	message := "验证成功"
	if !valid {
		message = "学生不在班级名单中，请联系老师"
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]interface{}{
		"valid":    valid,
		"message":  message,
		"courseId": effectiveCourseID,
	}})
}

// HandleStudentTip returns the tip for a class.
func (h *Handler) HandleStudentTip(w http.ResponseWriter, r *http.Request) {
	className := r.URL.Query().Get("className")
	h.tips.mu.RLock()
	tip, ok := h.tips.data[className]
	h.tips.mu.RUnlock()
	if !ok {
		h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]string{}})
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]string{"content": tip.Content}})
}

// HandleStudentStatus returns the student's current survey status.
func (h *Handler) HandleStudentStatus(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	courseID, err := h.resolveCourseID(r)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	survey, err := h.repo.GetStudentSurvey(courseID, studentID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	className, _ := utils.SplitStudentID(studentID)
	stage, err := h.repo.GetCurrentStage(courseID, className)
	if err != nil {
		h.writeError(w, err)
		return
	}
	course, err := h.repo.GetCourseByID(courseID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	history, err := h.repo.GetStudentHistoryScores(studentID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	settings, err := h.repo.GetPartSettings(courseID)
	if err != nil {
		settings = []models.PartSetting{
			{Part: 1, Enabled: true},
			{Part: 2, Enabled: true},
			{Part: 3, Enabled: true},
			{Part: 4, Enabled: true},
		}
	}
	settingsMap := make(map[string]bool, len(settings))
	for _, s := range settings {
		settingsMap[strconv.Itoa(s.Part)] = s.Enabled
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]interface{}{
		"survey":        survey,
		"currentStage":  stage,
		"courseName":    course.Name,
		"lessonNumber":  course.LessonNo,
		"historyScores": history,
		"partSettings":  settingsMap,
	}})
}

// HandleStudentHistoryDetails returns historical records.
func (h *Handler) HandleStudentHistoryDetails(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	courseID, err := h.resolveCourseID(r)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	items, err := h.repo.GetStudentHistoryDetails(studentID, courseID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]interface{}{
		"items": items,
	}})
}

// HandleStudentQuestions returns enabled questions for a part.
func (h *Handler) HandleStudentQuestions(w http.ResponseWriter, r *http.Request) {
	part, err := strconv.Atoi(r.PathValue("part"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "part 无效"})
		return
	}
	courseID, err := h.resolveCourseID(r)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	questions, err := h.repo.GetQuestionsByPart(courseID, part)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Data: map[string]interface{}{"questions": repository.FilterEnabledQuestions(questions)}})
}

// HandleStudentPart1 saves part-1 answers.
func (h *Handler) HandleStudentPart1(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	var body struct {
		CourseID    *int            `json:"courseId"`
		StudentName string          `json:"studentName"`
		ClassName   string          `json:"className"`
		Answers     json.RawMessage `json:"answers"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	courseID, err := h.resolveCourseIDFromBody(body.CourseID)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	if err := h.repo.SaveStudentPart1(courseID, studentID, body.StudentName, body.ClassName, body.Answers); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: "第一部分提交成功", Data: json.RawMessage(body.Answers)})
}

// HandleStudentPart2 saves part-2 answers.
func (h *Handler) HandleStudentPart2(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	var body struct {
		CourseID  *int     `json:"courseId"`
		Answer    string   `json:"answer"`
		Answers   []string `json:"answers"`
		Responses []models.Part2ResponseInput `json:"responses"`
	}
	type part2Response struct {
		Answer       string `json:"answer"`
		DisplayValue string `json:"displayValue"`
		QuestionType string `json:"questionType"`
		Explanation  string `json:"explanation"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	courseID, err := h.resolveCourseIDFromBody(body.CourseID)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	questions, err := h.repo.GetQuestionsByPart(courseID, 2)
	if err != nil {
		h.writeError(w, err)
		return
	}
	if len(questions) == 0 {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "第二部分题目不存在"})
		return
	}
	activeQuestions := repository.FilterEnabledQuestions(questions)
	if len(activeQuestions) == 0 {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: "第二部分题目未启用"})
		return
	}
	storedAnswer, displayValue, explanation, err := service.BuildPart2StoredAnswers(activeQuestions, body.Answer, body.Answers, body.Responses)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	if err := h.repo.SaveStudentPart2(courseID, studentID, storedAnswer); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: "第二部分提交成功", Data: part2Response{
		Answer:       storedAnswer,
		DisplayValue: displayValue,
		QuestionType: "mixed",
		Explanation:  explanation,
	}})
}

// HandleStudentPart3 saves part-3 answers and calculates the score.
func (h *Handler) HandleStudentPart3(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	var body struct {
		CourseID *int              `json:"courseId"`
		Answers  map[string]string `json:"answers"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	courseID, err := h.resolveCourseIDFromBody(body.CourseID)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	questions, err := h.repo.GetQuestionsByPart(courseID, 3)
	if err != nil {
		h.writeError(w, err)
		return
	}
	score := 0
	results := make([]map[string]interface{}, 0, len(questions))
	for i, q := range questions {
		key := fmt.Sprintf("q%d", i+1)
		answer := body.Answers[key]
		correct := q.CorrectAnswer != nil && answer == *q.CorrectAnswer
		if correct {
			score++
		}
		results = append(results, map[string]interface{}{
			"question":      q.QuestionText,
			"studentAnswer": answer,
			"correctAnswer": utils.Deref(q.CorrectAnswer),
			"explanation":   utils.Deref(q.Explanation),
			"isCorrect":     correct,
		})
	}
	raw, _ := json.Marshal(body.Answers)
	if err := h.repo.SaveStudentPart3(courseID, studentID, raw, score); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: fmt.Sprintf("第三部分提交成功，得分%d/%d", score, len(questions)), Data: map[string]interface{}{
		"score":   score,
		"total":   len(questions),
		"results": results,
	}})
}

// HandleStudentPart4 saves part-4 answers and marks completion.
func (h *Handler) HandleStudentPart4(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	var body struct {
		CourseID *int            `json:"courseId"`
		Answers  json.RawMessage `json:"answers"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	courseID, err := h.resolveCourseIDFromBody(body.CourseID)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}
	if err := h.repo.SaveStudentPart4(courseID, studentID, body.Answers); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, models.APIResponse{Success: true, Message: "问卷提交完成，感谢参与！"})
}
