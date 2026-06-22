package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"ai-quiz-system-v2/backend-go/internal/utils"
	"github.com/xuri/excelize/v2"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/meta", h.HandleMeta)
	mux.HandleFunc("GET /api/v2/classes", h.HandleListClasses)
	mux.HandleFunc("POST /api/v2/classes", h.HandleCreateClass)
	mux.HandleFunc("DELETE /api/v2/classes/{id}", h.HandleDeleteClass)
	mux.HandleFunc("GET /api/v2/classes/{id}/students", h.HandleListStudents)
	mux.HandleFunc("POST /api/v2/classes/{id}/students", h.HandleCreateStudent)
	mux.HandleFunc("POST /api/v2/classes/{id}/students/batch", h.HandleCreateStudentsBatch)
	mux.HandleFunc("GET /api/v2/classes/{id}/courses", h.HandleListClassCourses)
	mux.HandleFunc("PUT /api/v2/classes/{id}/courses", h.HandleSetClassCourses)
	mux.HandleFunc("DELETE /api/v2/students/{id}", h.HandleDeleteStudent)
	mux.HandleFunc("GET /api/v2/course-templates", h.HandleListTemplates)
	mux.HandleFunc("GET /api/v2/courses", h.HandleListCourses)
	mux.HandleFunc("POST /api/v2/courses", h.HandleCreateCourse)
	mux.HandleFunc("DELETE /api/v2/courses/{id}", h.HandleDeleteCourse)
	mux.HandleFunc("POST /api/v2/courses/{id}/classes", h.HandleBindCourseClass)
	mux.HandleFunc("GET /api/v2/courses/{id}/sections", h.HandleListSections)
	mux.HandleFunc("POST /api/v2/courses/{id}/sections", h.HandleCreateSection)
	mux.HandleFunc("GET /api/v2/sections/{id}/questions", h.HandleListQuestions)
	mux.HandleFunc("POST /api/v2/sections/{id}/questions", h.HandleCreateQuestion)
	mux.HandleFunc("PUT /api/v2/questions/{id}", h.HandleUpdateQuestion)
	mux.HandleFunc("DELETE /api/v2/questions/{id}", h.HandleDeleteQuestion)
	mux.HandleFunc("GET /api/v2/classrooms/{courseId}/{classId}", h.HandleGetClassroom)
	mux.HandleFunc("POST /api/v2/classrooms/{courseId}/{classId}/stage", h.HandleSetStage)
	mux.HandleFunc("POST /api/v2/classrooms/{courseId}/{classId}/blackboard", h.HandleSetBlackboard)
	mux.HandleFunc("POST /api/v2/student/ai-chat", h.HandleStudentAIChat)
	mux.HandleFunc("POST /api/v2/student/submit-section", h.HandleSubmitSection)
	mux.HandleFunc("GET /api/v2/student/score-summary", h.HandleScoreSummary)
	mux.HandleFunc("GET /api/v2/stats", h.HandleStats)
	mux.HandleFunc("GET /api/v2/stats/export", h.HandleStatsExport)
	mux.HandleFunc("GET /api/v2/stats/export-all", h.HandleStatsExportAll)
	mux.HandleFunc("GET /api/v2/stats/export-prediction-summary", h.HandlePredictionSummaryExport)
	mux.HandleFunc("POST /api/v2/stats/manual-score", h.HandleManualScore)
	mux.HandleFunc("GET /api/v2/stats/student-detail", h.HandleStudentDetail)
	mux.HandleFunc("GET /api/v2/student-history", h.HandleStudentHistory)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload APIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	h.writeJSON(w, http.StatusInternalServerError, APIResponse{Success: false, Error: err.Error()})
}

func pathInt(r *http.Request, name string) (int, error) {
	return strconv.Atoi(r.PathValue(name))
}

func (h *Handler) HandleMeta(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]string{
		"version": "v2",
		"status":  "ready",
	}})
}

func (h *Handler) HandleListClasses(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListClasses()
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"classes": items}})
}

func (h *Handler) HandleCreateClass(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	id, err := h.repo.CreateClass(body.Name, body.Description)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]int{"id": id}})
}

func (h *Handler) HandleDeleteClass(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "班级ID无效"})
		return
	}
	if err := h.repo.DeleteClass(id); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (h *Handler) HandleListStudents(w http.ResponseWriter, r *http.Request) {
	classID, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "班级ID无效"})
		return
	}
	items, err := h.repo.ListStudents(classID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"students": items}})
}

func (h *Handler) HandleCreateStudent(w http.ResponseWriter, r *http.Request) {
	classID, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "班级ID无效"})
		return
	}
	var body struct {
		Name      string `json:"name"`
		StudentNo string `json:"studentNo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	id, err := h.repo.CreateStudent(classID, body.Name, body.StudentNo)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]int{"id": id}})
}

func (h *Handler) HandleCreateStudentsBatch(w http.ResponseWriter, r *http.Request) {
	classID, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "班级ID无效"})
		return
	}
	var body struct {
		Students []Student `json:"students"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	ids, skipped, err := h.repo.CreateStudentsBatch(classID, body.Students)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"ids": ids, "count": len(ids), "skipped": skipped}})
}

func (h *Handler) HandleDeleteStudent(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "学生ID无效"})
		return
	}
	if err := h.repo.DeleteStudent(id); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (h *Handler) HandleListClassCourses(w http.ResponseWriter, r *http.Request) {
	classID, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "班级ID无效"})
		return
	}
	items, err := h.repo.ListCoursesForClass(classID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"courses": items}})
}

func (h *Handler) HandleSetClassCourses(w http.ResponseWriter, r *http.Request) {
	classID, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "班级ID无效"})
		return
	}
	var body struct {
		CourseIDs []int `json:"courseIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	if err := h.repo.SetClassCourses(classID, body.CourseIDs); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (h *Handler) HandleListTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListTemplates()
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"templates": items}})
}

func (h *Handler) HandleListCourses(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListCourses()
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"courses": items}})
}

func (h *Handler) HandleCreateCourse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title        string `json:"title"`
		Description  string `json:"description"`
		TemplateCode string `json:"templateCode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	id, err := h.repo.CreateCourse(body.Title, body.Description, body.TemplateCode)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]int{"id": id}})
}

func (h *Handler) HandleDeleteCourse(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "课程ID无效"})
		return
	}
	if err := h.repo.DeleteCourse(id); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (h *Handler) HandleBindCourseClass(w http.ResponseWriter, r *http.Request) {
	courseID, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "课程ID无效"})
		return
	}
	var body struct {
		ClassID int `json:"classId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	if err := h.repo.BindCourseClass(courseID, body.ClassID); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (h *Handler) HandleListSections(w http.ResponseWriter, r *http.Request) {
	courseID, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "课程ID无效"})
		return
	}
	items, err := h.repo.ListSections(courseID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"sections": items}})
}

func (h *Handler) HandleCreateSection(w http.ResponseWriter, r *http.Request) {
	courseID, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "课程ID无效"})
		return
	}
	var body struct {
		Title string `json:"title"`
		Type  string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	id, err := h.repo.CreateSection(courseID, body.Title, body.Type)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]int{"id": id}})
}

func (h *Handler) HandleListQuestions(w http.ResponseWriter, r *http.Request) {
	sectionID, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "部分ID无效"})
		return
	}
	items, err := h.repo.ListQuestions(sectionID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"questions": items}})
}

func (h *Handler) HandleCreateQuestion(w http.ResponseWriter, r *http.Request) {
	sectionID, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "部分ID无效"})
		return
	}
	var body Question
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	body.SectionID = sectionID
	id, err := h.repo.CreateQuestion(body)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]int{"id": id}})
}

func (h *Handler) HandleDeleteQuestion(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "题目ID无效"})
		return
	}
	if err := h.repo.DeleteQuestion(id); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (h *Handler) HandleUpdateQuestion(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "题目ID无效"})
		return
	}
	var body Question
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	body.ID = id
	if err := h.repo.UpdateQuestion(body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (h *Handler) HandleGetClassroom(w http.ResponseWriter, r *http.Request) {
	courseID, classID, ok := h.classroomIDs(w, r)
	if !ok {
		return
	}
	item, err := h.repo.GetOrCreateClassroom(courseID, classID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"classroom": item}})
}

func (h *Handler) HandleSetStage(w http.ResponseWriter, r *http.Request) {
	courseID, classID, ok := h.classroomIDs(w, r)
	if !ok {
		return
	}
	var body struct {
		StageID int `json:"stageId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	if err := h.repo.SetClassroomStage(courseID, classID, body.StageID); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (h *Handler) HandleSetBlackboard(w http.ResponseWriter, r *http.Request) {
	courseID, classID, ok := h.classroomIDs(w, r)
	if !ok {
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	if err := h.repo.SetBlackboard(courseID, classID, body.Content); err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (h *Handler) HandleStudentAIChat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CourseID   int             `json:"courseId"`
		ClassID    int             `json:"classId"`
		StudentID  int             `json:"studentId"`
		SectionID  int             `json:"sectionId"`
		QuestionID int             `json:"questionId"`
		Message    string          `json:"message"`
		Messages   []AIChatMessage `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	userMessage := strings.TrimSpace(body.Message)
	if body.CourseID <= 0 || body.ClassID <= 0 || body.StudentID <= 0 || body.SectionID <= 0 || body.QuestionID <= 0 {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "课堂、班级、学生、部分和题目不能为空"})
		return
	}
	if userMessage == "" {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "请输入要和 AI 讨论的问题"})
		return
	}
	if runeLen(userMessage) > 500 {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "单次提问最多 500 字"})
		return
	}
	if err := h.repo.ValidateAIChatQuestion(body.CourseID, body.SectionID, body.QuestionID); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}

	history := sanitizeAIChatMessages(body.Messages)
	rounds := countAIRounds(history)
	if rounds >= 5 {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "单个 AI 对话题最多支持 5 轮对话"})
		return
	}
	history = append(history, AIChatMessage{Role: "user", Content: userMessage})

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	answer, err := h.callDeepSeek(ctx, history)
	if err != nil {
		h.writeJSON(w, http.StatusBadGateway, APIResponse{Success: false, Error: err.Error()})
		return
	}
	history = append(history, AIChatMessage{Role: "assistant", Content: answer})
	h.writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"assistantMessage": answer,
			"messages":         history,
			"rounds":           rounds + 1,
			"maxRounds":        5,
		},
	})
}

func (h *Handler) callDeepSeek(ctx context.Context, history []AIChatMessage) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("DEEPSEEK_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	}
	if apiKey == "" {
		return "", fmt.Errorf("未配置 DEEPSEEK_KEY 环境变量")
	}
	messages := []AIChatMessage{{
		Role: "system",
		Content: strings.Join([]string{
			"你是小学五年级人工智能课堂里的学习助手。",
			"只回答学习相关问题，包括人工智能、课堂知识、学习方法、作业思路和学科知识。",
			"如果学生询问与学习无关、娱乐八卦、违法危险、隐私攻击等内容，请礼貌拒绝，并引导回学习问题。",
			"每次回复必须控制在1000个中文字符以内。",
			"解释要符合小学五年级学生理解水平：用短句、例子和鼓励性的语气，不要堆砌术语。",
			"不要直接代写完整作业答案，可以给思路、步骤、提示和检查方法。",
		}, "\n"),
	}}
	messages = append(messages, history...)

	payload := map[string]interface{}{
		"model":       "deepseek-v4-flash",
		"messages":    messages,
		"stream":      false,
		"temperature": 0.3,
		"max_tokens":  1200,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.deepseek.com/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("调用 DeepSeek 失败：%w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errBody) == nil && strings.TrimSpace(errBody.Error.Message) != "" {
			return "", fmt.Errorf("DeepSeek 返回错误：%s", errBody.Error.Message)
		}
		return "", fmt.Errorf("DeepSeek 返回错误状态：%s", resp.Status)
	}
	var result struct {
		Choices []struct {
			Message AIChatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析 DeepSeek 响应失败：%w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("DeepSeek 没有返回回答")
	}
	answer := strings.TrimSpace(result.Choices[0].Message.Content)
	if answer == "" {
		return "", fmt.Errorf("DeepSeek 返回空回答")
	}
	return truncateRunes(answer, 1000), nil
}

func sanitizeAIChatMessages(messages []AIChatMessage) []AIChatMessage {
	result := make([]AIChatMessage, 0, len(messages))
	for _, item := range messages {
		role := strings.TrimSpace(item.Role)
		content := strings.TrimSpace(item.Content)
		if content == "" || (role != "user" && role != "assistant") {
			continue
		}
		result = append(result, AIChatMessage{Role: role, Content: truncateRunes(content, 1000)})
		if len(result) >= 10 {
			break
		}
	}
	return result
}

func countAIRounds(messages []AIChatMessage) int {
	count := 0
	for _, item := range messages {
		if item.Role == "user" {
			count++
		}
	}
	return count
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func runeLen(value string) int {
	return len([]rune(value))
}

func (h *Handler) HandleSubmitSection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CourseID  int                        `json:"courseId"`
		ClassID   int                        `json:"classId"`
		StudentID int                        `json:"studentId"`
		SectionID int                        `json:"sectionId"`
		Answers   map[string]json.RawMessage `json:"answers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	result, err := h.repo.SubmitSection(body.CourseID, body.ClassID, body.StudentID, body.SectionID, body.Answers)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: result})
}

func (h *Handler) HandleScoreSummary(w http.ResponseWriter, r *http.Request) {
	courseID, _ := strconv.Atoi(r.URL.Query().Get("courseId"))
	classID, _ := strconv.Atoi(r.URL.Query().Get("classId"))
	studentID, _ := strconv.Atoi(r.URL.Query().Get("studentId"))
	if courseID <= 0 || classID <= 0 || studentID <= 0 {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "课堂、班级和学生不能为空"})
		return
	}
	summary, err := h.repo.GetScoreSummary(courseID, classID, studentID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"summary": summary}})
}

func (h *Handler) HandleManualScore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SubmissionID int    `json:"submissionId"`
		CourseID     int    `json:"courseId"`
		ClassID      int    `json:"classId"`
		StudentID    int    `json:"studentId"`
		TeacherScore *int   `json:"teacherScore"`
		Note         string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	if body.TeacherScore != nil && (*body.TeacherScore < 0 || *body.TeacherScore > 5) {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "教师评分必须是0到5分"})
		return
	}
	submissionID, err := h.repo.SaveTeacherScoreForStudent(
		body.SubmissionID,
		body.CourseID,
		body.ClassID,
		body.StudentID,
		body.TeacherScore,
		body.Note,
	)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"submissionId": submissionID}})
}

func (h *Handler) HandleStudentDetail(w http.ResponseWriter, r *http.Request) {
	submissionID, _ := strconv.Atoi(r.URL.Query().Get("submissionId"))
	if submissionID <= 0 {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "提交ID不能为空"})
		return
	}
	detail, err := h.repo.GetStudentDetail(submissionID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"detail": detail}})
}

func (h *Handler) classroomIDs(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	courseID, err := pathInt(r, "courseId")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "课程ID无效"})
		return 0, 0, false
	}
	classID, err := pathInt(r, "classId")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "班级ID无效"})
		return 0, 0, false
	}
	return courseID, classID, true
}

func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	courseID, _ := strconv.Atoi(r.URL.Query().Get("courseId"))
	classID, _ := strconv.Atoi(r.URL.Query().Get("classId"))
	stats, err := h.repo.BuildStatsSummary(courseID, classID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"stats": stats}})
}

func (h *Handler) HandleStudentHistory(w http.ResponseWriter, r *http.Request) {
	classID, _ := strconv.Atoi(r.URL.Query().Get("classId"))
	studentID, _ := strconv.Atoi(r.URL.Query().Get("studentId"))
	if classID <= 0 || studentID <= 0 {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "班级和学生信息不能为空"})
		return
	}
	records, err := h.repo.ListStudentHistory(classID, studentID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"records": records}})
}

func (h *Handler) HandleStatsExport(w http.ResponseWriter, r *http.Request) {
	courseID, _ := strconv.Atoi(r.URL.Query().Get("courseId"))
	classID, _ := strconv.Atoi(r.URL.Query().Get("classId"))
	stats, err := h.repo.BuildStatsSummary(courseID, classID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	wb := excelize.NewFile()
	if err := h.writeStatsWorkbook(wb, stats, courseID, classID); err != nil {
		h.writeError(w, err)
		return
	}
	filename := fmt.Sprintf("v2-stats_%s.xlsx", time.Now().Format("2006-01-02"))
	h.writeXLSX(w, wb, filename)
}

func (h *Handler) HandleStatsExportAll(w http.ResponseWriter, r *http.Request) {
	stats, err := h.repo.BuildStatsSummary(0, 0)
	if err != nil {
		h.writeError(w, err)
		return
	}
	wb := excelize.NewFile()
	if err := h.writeAllStatsWorkbook(wb, stats); err != nil {
		h.writeError(w, err)
		return
	}
	filename := fmt.Sprintf("v2-all-stats_%s.xlsx", time.Now().Format("2006-01-02"))
	h.writeXLSX(w, wb, filename)
}

func (h *Handler) HandlePredictionSummaryExport(w http.ResponseWriter, r *http.Request) {
	classID, _ := strconv.Atoi(r.URL.Query().Get("classId"))
	stats, err := h.repo.BuildStatsSummary(0, 0)
	if err != nil {
		h.writeError(w, err)
		return
	}
	wb := excelize.NewFile()
	sheet := "学生预测汇总"
	rows := [][]string{{"班级", "姓名", "猜对次数", "猜高次数", "猜低次数", "未知次数", "参与课堂数"}}
	type summary struct {
		className                   string
		correct, high, low, unknown int
		total                       int
	}
	byStudent := map[int]*summary{}
	for _, course := range stats.Courses {
		courseStats, err := h.repo.BuildStatsSummary(course.ID, classID)
		if err != nil {
			h.writeError(w, err)
			return
		}
		for _, student := range courseStats.Students {
			item := byStudent[student.StudentID]
			if item == nil {
				item = &summary{className: student.ClassName}
				byStudent[student.StudentID] = item
			}
			item.total++
			switch student.GuessResult {
			case "correct":
				item.correct++
			case "high":
				item.high++
			case "low":
				item.low++
			default:
				item.unknown++
			}
		}
	}
	for _, classItem := range stats.Classes {
		students, err := h.repo.ListStudents(classItem.ID)
		if err != nil {
			h.writeError(w, err)
			return
		}
		for _, student := range students {
			if classID > 0 && student.ClassID != classID {
				continue
			}
			item := byStudent[student.ID]
			if item == nil {
				item = &summary{className: classItem.Name}
			}
			rows = append(rows, []string{
				item.className,
				student.Name,
				strconv.Itoa(item.correct),
				strconv.Itoa(item.high),
				strconv.Itoa(item.low),
				strconv.Itoa(item.unknown),
				strconv.Itoa(item.total),
			})
		}
	}
	if err := writeMatrixSheet(wb, sheet, rows, true); err != nil {
		h.writeError(w, err)
		return
	}
	filename := fmt.Sprintf("v2-prediction-summary_%s.xlsx", time.Now().Format("2006-01-02"))
	h.writeXLSX(w, wb, filename)
}

func statsRows(stats StatsSummary) [][]string {
	rows := [][]string{{"模块", "指标", "数值"}}
	rows = append(rows,
		[]string{"汇总", "班级数", strconv.Itoa(stats.ClassCount)},
		[]string{"汇总", "学生数", strconv.Itoa(stats.StudentCount)},
		[]string{"汇总", "课程数", strconv.Itoa(stats.CourseCount)},
		[]string{"汇总", "部分数", strconv.Itoa(stats.SectionCount)},
		[]string{"汇总", "题目数", strconv.Itoa(stats.QuestionCount)},
		[]string{"完成情况", "班级总人数", strconv.Itoa(stats.TotalClass)},
		[]string{"完成情况", "已答题", strconv.Itoa(stats.SubmittedCount)},
		[]string{"完成情况", "未提交", strconv.Itoa(len(stats.NotSubmittedStudents))},
	)
	for _, part := range stats.Parts {
		rows = append(rows, []string{
			"各部分完成情况",
			fmt.Sprintf("第%d部分 %s", part.Part, part.Title),
			fmt.Sprintf("%d/%d", part.Completed, part.Total),
		})
	}
	rows = append(rows, []string{"学生明细", "班级", "姓名", "完成状态", "预测分", "小测分", "教师评分", "实际分", "实际分来源", "猜测结果", "评分备注"})
	for _, student := range stats.Students {
		rows = append(rows, []string{
			"学生明细",
			student.ClassName,
			student.StudentName,
			student.StatusText,
			intPtrText(student.PredictedScore, "未填写"),
			intPtrText(student.QuizScore, "待评分"),
			intPtrText(student.TeacherScore, ""),
			intPtrText(student.ActualScore, "待评分"),
			student.ActualScoreSource,
			student.GuessResultText,
			student.TeacherScoreNote,
		})
	}
	if stats.Classroom != nil {
		rows = append(rows, []string{"课堂状态", "当前开启部分ID", strconv.Itoa(stats.Classroom.StageID)})
		rows = append(rows, []string{"课堂状态", "课堂黑板", stats.Classroom.Blackboard})
	}
	return rows
}

type exportSummaryRow struct {
	CourseTitle      string
	ClassName        string
	StudentName      string
	StatusText       string
	PredictedScore   string
	QuizScore        string
	TeacherScore     string
	ActualScore      string
	ActualSourceText string
	GuessResultText  string
	AnswerCount      int
	AIChatRounds     int
	TeacherScoreNote string
	UpdatedAt        string
}

func (h *Handler) writeStatsWorkbook(wb *excelize.File, stats StatsSummary, courseID, classID int) error {
	overviewRows := statsRows(stats)
	if err := writeMatrixSheet(wb, "统计概览", overviewRows, true); err != nil {
		return err
	}

	courseTitle := ""
	if courseID > 0 {
		courseTitle = courseTitleByID(stats.Courses, courseID)
	}
	summaryRows, questionRows, chatRows, err := h.buildExportRows(stats.Students, courseTitle)
	if err != nil {
		return err
	}
	if err := writeMatrixSheet(wb, "学生答题汇总", summaryRows, true); err != nil {
		return err
	}
	if err := writeMatrixSheet(wb, "题目回答明细", questionRows, true); err != nil {
		return err
	}
	if err := writeMatrixSheet(wb, "AI聊天记录", chatRows, true); err != nil {
		return err
	}
	return nil
}

func (h *Handler) writeAllStatsWorkbook(wb *excelize.File, stats StatsSummary) error {
	overviewRows := statsRows(stats)
	if err := writeMatrixSheet(wb, "统计概览", overviewRows, true); err != nil {
		return err
	}

	summaryRows := [][]string{{
		"课程", "班级", "姓名", "完成状态", "预测分", "小测分", "教师评分", "实际分", "实际分来源", "猜测结果", "答题题数", "AI对话轮次", "评分备注", "最后提交时间",
	}}
	questionRows := [][]string{{
		"课程", "班级", "姓名", "部分", "提交次数", "题目序号", "题目", "题型", "学生答案", "正确答案", "是否正确", "分值", "解析", "提交时间",
	}}
	chatRows := [][]string{{
		"课程", "班级", "姓名", "部分", "题目", "提交次数", "轮次", "角色", "内容", "提交时间",
	}}

	for _, course := range stats.Courses {
		courseStats, err := h.repo.BuildStatsSummary(course.ID, 0)
		if err != nil {
			return err
		}
		courseSummaryRows, courseQuestionRows, courseChatRows, err := h.buildExportRows(courseStats.Students, course.Title)
		if err != nil {
			return err
		}
		summaryRows = append(summaryRows, courseSummaryRows[1:]...)
		questionRows = append(questionRows, courseQuestionRows[1:]...)
		chatRows = append(chatRows, courseChatRows[1:]...)
	}

	if err := writeMatrixSheet(wb, "学生答题汇总", summaryRows, true); err != nil {
		return err
	}
	if err := writeMatrixSheet(wb, "题目回答明细", questionRows, true); err != nil {
		return err
	}
	if err := writeMatrixSheet(wb, "AI聊天记录", chatRows, true); err != nil {
		return err
	}
	return nil
}

func (h *Handler) buildExportRows(students []StudentStats, courseTitle string) ([][]string, [][]string, [][]string, error) {
	summaryRows := [][]string{{
		"课程", "班级", "姓名", "完成状态", "预测分", "小测分", "教师评分", "实际分", "实际分来源", "猜测结果", "答题题数", "AI对话轮次", "评分备注", "最后提交时间",
	}}
	questionRows := [][]string{{
		"课程", "班级", "姓名", "部分", "提交次数", "题目序号", "题目", "题型", "学生答案", "正确答案", "是否正确", "分值", "解析", "提交时间",
	}}
	chatRows := [][]string{{
		"课程", "班级", "姓名", "部分", "题目", "提交次数", "轮次", "角色", "内容", "提交时间",
	}}

	for _, student := range students {
		summary := exportSummaryRow{
			CourseTitle:      courseTitle,
			ClassName:        student.ClassName,
			StudentName:      student.StudentName,
			StatusText:       student.StatusText,
			PredictedScore:   intPtrText(student.PredictedScore, "未填写"),
			QuizScore:        intPtrText(student.QuizScore, "待评分"),
			TeacherScore:     intPtrText(student.TeacherScore, "未评分"),
			ActualScore:      intPtrText(student.ActualScore, "待评分"),
			ActualSourceText: actualScoreSourceText(student.ActualScoreSource),
			GuessResultText:  student.GuessResultText,
			TeacherScoreNote: student.TeacherScoreNote,
			UpdatedAt:        student.UpdatedAt,
		}
		if student.SubmissionID <= 0 {
			summaryRows = append(summaryRows, []string{
				summary.CourseTitle,
				summary.ClassName,
				summary.StudentName,
				summary.StatusText,
				summary.PredictedScore,
				summary.QuizScore,
				summary.TeacherScore,
				summary.ActualScore,
				summary.ActualSourceText,
				summary.GuessResultText,
				"0",
				"0",
				summary.TeacherScoreNote,
				summary.UpdatedAt,
			})
			continue
		}

		detail, err := h.repo.GetStudentDetail(student.SubmissionID)
		if err != nil {
			return nil, nil, nil, err
		}
		answerCount := 0
		aiRounds := 0
		for _, section := range detail.Sections {
			for _, attempt := range section.Attempts {
				for _, question := range attempt.Questions {
					answerCount++
					if question.QuestionType == "ai_chat" {
						for _, msg := range question.ChatMessages {
							if msg.Role == "user" {
								aiRounds++
							}
						}
					}
				}
			}
		}
		summary.AnswerCount = answerCount
		summary.AIChatRounds = aiRounds
		summaryRows = append(summaryRows, []string{
			summary.CourseTitle,
			summary.ClassName,
			summary.StudentName,
			summary.StatusText,
			summary.PredictedScore,
			summary.QuizScore,
			summary.TeacherScore,
			summary.ActualScore,
			summary.ActualSourceText,
			summary.GuessResultText,
			strconv.Itoa(summary.AnswerCount),
			strconv.Itoa(summary.AIChatRounds),
			summary.TeacherScoreNote,
			summary.UpdatedAt,
		})

		for _, section := range detail.Sections {
			for _, attempt := range section.Attempts {
				for _, question := range attempt.Questions {
					questionRows = append(questionRows, []string{
						detail.CourseTitle,
						detail.ClassName,
						detail.StudentName,
						section.Title,
						strconv.Itoa(attempt.AttemptNo),
						strconv.Itoa(question.SortOrder),
						question.QuestionText,
						question.QuestionType,
						func() string {
							if question.QuestionType == "open_text" {
								return openTextExportAnswer(question.Answer)
							}
							return question.Answer
						}(),
						question.CorrectAnswer,
						boolText(question.IsCorrect),
						strconv.Itoa(question.Score),
						question.Explanation,
						attempt.SubmittedAt,
					})

					if question.QuestionType != "ai_chat" {
						continue
					}
					round := 0
					for _, msg := range question.ChatMessages {
						if msg.Role == "user" {
							round++
						}
						chatRows = append(chatRows, []string{
							detail.CourseTitle,
							detail.ClassName,
							detail.StudentName,
							section.Title,
							question.QuestionText,
							strconv.Itoa(attempt.AttemptNo),
							strconv.Itoa(round),
							roleText(msg.Role),
							msg.Content,
							attempt.SubmittedAt,
						})
					}
				}
			}
		}
	}

	return summaryRows, questionRows, chatRows, nil
}

func courseTitleByID(courses []Course, courseID int) string {
	for _, course := range courses {
		if course.ID == courseID {
			return course.Title
		}
	}
	if courseID > 0 {
		return fmt.Sprintf("课程%d", courseID)
	}
	return "全部课程"
}

func actualScoreSourceText(source string) string {
	switch source {
	case "teacher":
		return "教师评分"
	case "part3":
		return "第三部分小测"
	default:
		return "无"
	}
}

func roleText(role string) string {
	switch strings.TrimSpace(role) {
	case "user":
		return "学生"
	case "assistant":
		return "AI"
	default:
		return role
	}
}

func boolText(value bool) string {
	if value {
		return "是"
	}
	return "否"
}

func writeMatrixSheet(wb *excelize.File, sheetName string, rows [][]string, autoWidth bool) error {
	if len(rows) == 0 {
		rows = [][]string{{}}
	}
	if wb.GetSheetName(0) == "Sheet1" {
		wb.SetSheetName("Sheet1", sheetName)
	} else {
		wb.NewSheet(sheetName)
	}
	headers := rows[0]
	for rowIndex, row := range rows {
		for colIndex, cellValue := range row {
			cell, _ := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
			wb.SetCellValue(sheetName, cell, cellValue)
		}
	}
	if len(headers) > 0 {
		headerEnd, _ := excelize.CoordinatesToCellName(len(headers), 1)
		_ = wb.SetCellStyle(sheetName, "A1", headerEnd, headerStyle(wb))
		_ = wb.AutoFilter(sheetName, fmt.Sprintf("A1:%s", headerEnd), nil)
	}
	if autoWidth {
		setSheetWidths(wb, sheetName, rows)
	}
	return nil
}

func headerStyle(wb *excelize.File) int {
	style, err := wb.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#4F8FEA"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return 0
	}
	return style
}

func setSheetWidths(wb *excelize.File, sheetName string, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	columnCount := 0
	for _, row := range rows {
		if len(row) > columnCount {
			columnCount = len(row)
		}
	}
	for colIndex := 0; colIndex < columnCount; colIndex++ {
		maxWidth := 10.0
		for _, row := range rows {
			if colIndex >= len(row) {
				continue
			}
			width := float64(len([]rune(row[colIndex])))
			if width > maxWidth {
				maxWidth = width
			}
		}
		if maxWidth > 40 {
			maxWidth = 40
		}
		col, _ := excelize.ColumnNumberToName(colIndex + 1)
		_ = wb.SetColWidth(sheetName, col, col, maxWidth+2)
	}
}

func (h *Handler) writeXLSX(w http.ResponseWriter, wb *excelize.File, filename string) {
	defer func() {
		_ = wb.Close()
	}()
	buf, err := wb.WriteToBuffer()
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", utils.URLEncode(filename)))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func (h *Handler) writeCSV(w http.ResponseWriter, filename string, rows [][]string) {
	var builder strings.Builder
	builder.WriteString("\uFEFF")
	for _, row := range rows {
		for index, cell := range row {
			if index > 0 {
				builder.WriteString(",")
			}
			builder.WriteString(csvCell(cell))
		}
		builder.WriteString("\n")
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(builder.String()))
}

func intPtrText(value *int, fallback string) string {
	if value == nil {
		return fallback
	}
	return strconv.Itoa(*value)
}

func csvCell(value string) string {
	value = strings.ReplaceAll(value, `"`, `""`)
	return fmt.Sprintf(`"%s"`, value)
}
