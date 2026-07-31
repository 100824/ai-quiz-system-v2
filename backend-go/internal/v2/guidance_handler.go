package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (h *Handler) HandleGetSectionAIGuidance(w http.ResponseWriter, r *http.Request) {
	sectionID, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "部分ID无效"})
		return
	}
	config, err := h.repo.GetSectionAIGuidanceConfig(sectionID)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"aiGuidance": config}})
}

func (h *Handler) HandleSetSectionAIGuidance(w http.ResponseWriter, r *http.Request) {
	sectionID, err := pathInt(r, "id")
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "部分ID无效"})
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	config, err := h.repo.SetSectionAIGuidance(sectionID, body.Enabled)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"aiGuidance": config}})
}

func guidanceIdentity(r *http.Request) (courseID, classID, studentID, sectionID int) {
	courseID, _ = strconv.Atoi(r.URL.Query().Get("courseId"))
	classID, _ = strconv.Atoi(r.URL.Query().Get("classId"))
	studentID, _ = strconv.Atoi(r.URL.Query().Get("studentId"))
	sectionID, _ = strconv.Atoi(r.URL.Query().Get("sectionId"))
	return
}

func (h *Handler) HandleGetStudentAIGuidance(w http.ResponseWriter, r *http.Request) {
	courseID, classID, studentID, sectionID := guidanceIdentity(r)
	if courseID <= 0 || classID <= 0 || studentID <= 0 || sectionID <= 0 {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "课堂、班级、学生和部分不能为空"})
		return
	}
	session, config, err := h.repo.LoadAIGuidanceSession(courseID, classID, studentID, sectionID)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{
		"aiGuidance": config,
		"session":    session,
		"maxRounds":  5,
	}})
}

type guidanceRequest struct {
	CourseID  int `json:"courseId"`
	ClassID   int `json:"classId"`
	StudentID int `json:"studentId"`
	SectionID int `json:"sectionId"`
}

func (h *Handler) guidanceLock(key string) *sync.Mutex {
	value, _ := h.guidanceLocks.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func (h *Handler) HandleGenerateStudentAIGuidance(w http.ResponseWriter, r *http.Request) {
	var body guidanceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	if body.CourseID <= 0 || body.ClassID <= 0 || body.StudentID <= 0 || body.SectionID <= 0 {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "课堂、班级、学生和部分不能为空"})
		return
	}
	lock := h.guidanceLock(fmt.Sprintf("%d:%d:%d:%d", body.CourseID, body.ClassID, body.StudentID, body.SectionID))
	lock.Lock()
	defer lock.Unlock()

	guidanceContext, err := h.repo.guidanceContext(body.CourseID, body.ClassID, body.StudentID, body.SectionID)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	if guidanceContext.SubmissionID > 0 {
		existing, loadErr := h.repo.loadAIGuidanceSessionBySubmission(guidanceContext.SubmissionID, body.SectionID)
		if loadErr == nil && (existing.Status == "ready" || existing.Status == "completed" || existing.Status == "skipped") {
			h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"session": existing, "reused": true}})
			return
		}
	}
	snapshot, err := h.repo.BuildAIGuidanceSnapshot(guidanceContext)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	session, reused, err := h.repo.StartAIGuidanceGeneration(guidanceContext, snapshot)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	if reused {
		h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"session": session, "reused": true}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	answer, err := h.callDeepSeekWithSystem(ctx, guidanceSystemPrompt(guidanceContext.Config.Phase), []AIChatMessage{{
		Role:    "user",
		Content: "请严格依据下面的平台数据完成任务。字段为“未记录”时不得推测或补造。\n\n" + snapshot,
	}})
	if err != nil {
		_ = h.repo.FailAIGuidanceGeneration(session.ID, err)
		failed, _ := h.repo.loadAIGuidanceSessionBySubmission(guidanceContext.SubmissionID, body.SectionID)
		h.writeJSON(w, http.StatusBadGateway, APIResponse{Success: false, Error: err.Error(), Data: map[string]interface{}{"session": failed}})
		return
	}
	session, err = h.repo.CompleteAIGuidanceGeneration(session.ID, answer)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"session": session, "reused": false}})
}

func (h *Handler) HandleStudentAIGuidanceMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		guidanceRequest
		SessionID int    `json:"sessionId"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	body.Message = strings.TrimSpace(body.Message)
	if body.SessionID <= 0 || body.CourseID <= 0 || body.ClassID <= 0 || body.StudentID <= 0 || body.Message == "" {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "会话和提问内容不能为空"})
		return
	}
	if runeLen(body.Message) > 500 {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "单次提问最多500字"})
		return
	}
	if err := h.repo.ValidateAIGuidanceSessionOwner(body.SessionID, body.CourseID, body.ClassID, body.StudentID); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	lock := h.guidanceLock(fmt.Sprintf("session:%d", body.SessionID))
	lock.Lock()
	defer lock.Unlock()
	session, err := h.repo.AIGuidanceSessionByID(body.SessionID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	if session.Status != "ready" {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "当前AI学习指导已完成或尚未生成"})
		return
	}
	if session.FollowUpRounds >= 5 {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "最多支持5轮追问"})
		return
	}
	snapshot, err := h.repo.AIGuidanceSnapshot(session.ID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	history := make([]AIChatMessage, 0, len(session.Messages)+1)
	for _, message := range session.Messages {
		history = append(history, AIChatMessage{Role: message.Role, Content: message.Content})
	}
	history = append(history, AIChatMessage{Role: "user", Content: body.Message})
	system := guidanceSystemPrompt(session.Phase) + "\n\n以下是生成初始指导时冻结的平台数据，请保持上下文一致：\n" + snapshot
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	answer, err := h.callDeepSeekWithSystem(ctx, system, history)
	if err != nil {
		h.writeJSON(w, http.StatusBadGateway, APIResponse{Success: false, Error: err.Error()})
		return
	}
	session, err = h.repo.AppendAIGuidanceExchange(session.ID, body.Message, answer)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"session": session, "maxRounds": 5}})
}

func (h *Handler) HandleCompleteStudentAIGuidance(w http.ResponseWriter, r *http.Request) {
	var body struct {
		guidanceRequest
		Skip bool `json:"skip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	session, err := h.repo.CompleteOrSkipAIGuidance(body.CourseID, body.ClassID, body.StudentID, body.SectionID, body.Skip)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: err.Error()})
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: map[string]interface{}{"session": session}})
}

func guidanceSystemPrompt(phase string) string {
	common := strings.Join([]string{
		"只处理学习相关内容；遇到与学习无关、危险、违法、隐私或攻击性内容时应礼貌拒绝并引导回学习。",
		"每次回复不超过1000个中文字符，使用小学五年级学生能理解的短句、例子和鼓励性表达。",
		"不得臆造平台没有提供的数据；标记为“未记录”的字段必须按缺失处理。",
		"可以使用Markdown组织内容，但不要输出隐藏思考过程。",
	}, "\n")
	if phase == "evaluation" {
		return strings.Join([]string{
			"你是一位小学生五年级学生的人工智能学习指导助手。平台将为你提供以下数据：",
			"【输入数据】",
			"- 本节课学习目标",
			"- 学生的预测分数与实际分数（满分5分，对应5道选择题）",
			"- 学生作答详情（每道题的题干、学生选项、正确答案及是否答对）",
			"- 学生本课第一至第三部分的其他作答内容",
			"- 学生过往学习表现（每课的学习目标、预测分数、实际分数；若该字段为空，说明学生无前期数据）",
			"【任务要求】",
			"1. 为学生生成本节课的简短学习评价与反思引导；",
			"2. 逐题分析错题：指出学生错在哪道题、正确解法是什么、错误可能反映的知识点漏洞，并将错题与本节课学习目标对应；",
			"3. 若有前期数据：结合学生长期表现趋势，分析其预测准确性与学习进步情况；",
			"4. 若无前期数据：仅依据本节课表现生成评价，不臆造学生过往表现；",
			"5. 以提问方式引导学生反思，而非只直接给出结论；",
			"6. 语言简短、鼓励为主，符合小学生的理解水平。",
			common,
		}, "\n")
	}
	return strings.Join([]string{
		"你是一位小学生五年级学生的人工智能学习指导助手。平台将为你提供以下数据：",
		"【输入数据】",
		"- 本节课学习目标",
		"- 学生填写的预测分数（满分5分，对应5道选择题）",
		"- 学生过往学习表现（每课的学习目标、预测分数、实际分数；若该字段为空，说明学生无前期数据）",
		"【任务要求】",
		"1. 为学生生成本节课的简短学习计划；",
		"2. 若有前期数据：结合学生过往预测与实际分数的差距，帮助其校准本节课的目标预期与学习策略；",
		"3. 若无前期数据：仅依据本节课学习目标和预测分数生成计划，不臆造学生过往表现；",
		"4. 语言简短、亲切，符合小学生的理解水平。",
		common,
	}, "\n")
}
