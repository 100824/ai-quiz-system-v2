package v2

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	aiGuidanceModel = "deepseek-v4-flash"
)

type guidanceContext struct {
	Course       Course
	Section      Section
	Config       AIGuidanceConfig
	SubmissionID int
	StudentID    int
}

func parseAIGuidanceConfig(section Section) AIGuidanceConfig {
	var rules struct {
		AIGuidance AIGuidanceConfig `json:"aiGuidance"`
	}
	_ = json.Unmarshal(section.Rules, &rules)
	return rules.AIGuidance
}

func (r *Repository) GetSectionAIGuidanceConfig(sectionID int) (AIGuidanceConfig, error) {
	var section Section
	var enabled, fixed int
	var raw string
	err := r.db.QueryRow(`
		SELECT id, course_id, section_key, title, type, sort_order, enabled, fixed, rules_json
		FROM course_sections WHERE id = ?
	`, sectionID).Scan(
		&section.ID, &section.CourseID, &section.SectionKey, &section.Title, &section.Type,
		&section.SortOrder, &enabled, &fixed, &raw,
	)
	if err != nil {
		return AIGuidanceConfig{}, err
	}
	section.Enabled = enabled == 1
	section.Fixed = fixed == 1
	section.Rules = json.RawMessage(raw)
	config := parseAIGuidanceConfig(section)
	if config.Phase == "" {
		return config, errors.New("该部分不支持AI学习指导")
	}
	var count int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM answer_attempts WHERE section_id = ?`, sectionID).Scan(&count); err != nil {
		return config, err
	}
	config.Locked = count > 0
	return config, nil
}

func (r *Repository) SetSectionAIGuidance(sectionID int, enabled bool) (AIGuidanceConfig, error) {
	var mode, sectionKey, learningObjective, raw string
	err := r.db.QueryRow(`
		SELECT c.mode, cs.section_key, c.learning_objective, cs.rules_json
		FROM course_sections cs
		JOIN courses c ON c.id = cs.course_id AND c.deleted_at IS NULL
		WHERE cs.id = ?
	`, sectionID).Scan(&mode, &sectionKey, &learningObjective, &raw)
	if err != nil {
		return AIGuidanceConfig{}, err
	}
	if mode != "reflection" || (sectionKey != "prediction" && sectionKey != "reflection") {
		return AIGuidanceConfig{}, errors.New("AI学习指导仅支持反思模板的第一和第四部分")
	}
	var answerCount int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM answer_attempts WHERE section_id = ?`, sectionID).Scan(&answerCount); err != nil {
		return AIGuidanceConfig{}, err
	}
	if answerCount > 0 {
		return AIGuidanceConfig{}, errors.New("该部分已有学生答题，不能修改AI学习指导开关")
	}
	if enabled && strings.TrimSpace(learningObjective) == "" {
		return AIGuidanceConfig{}, errors.New("请先填写本课堂的学习目标")
	}
	rules := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		rules = map[string]interface{}{}
	}
	phase, title := "plan", "AI学习计划"
	if sectionKey == "reflection" {
		phase, title = "evaluation", "AI学习评价与反思"
	}
	rules["aiGuidance"] = map[string]interface{}{"enabled": enabled, "phase": phase, "title": title}
	encoded, _ := json.Marshal(rules)
	if _, err := r.db.Exec(`
		UPDATE course_sections SET rules_json = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, string(encoded), sectionID); err != nil {
		return AIGuidanceConfig{}, err
	}
	return AIGuidanceConfig{Enabled: enabled, Phase: phase, Title: title}, nil
}

func (r *Repository) guidanceContext(courseID, classID, studentID, sectionID int) (guidanceContext, error) {
	var context guidanceContext
	var enabled, fixed int
	var rulesRaw string
	err := r.db.QueryRow(`
		SELECT c.id, COALESCE(c.template_id, 0), c.template_code, c.title, c.mode,
		       c.description, c.learning_objective, c.status, c.created_at, c.updated_at,
		       cs.id, cs.course_id, cs.section_key, cs.title, cs.type, cs.sort_order,
		       cs.enabled, cs.fixed, cs.rules_json,
		       COALESCE(s.id, 0)
		FROM courses c
		JOIN course_sections cs ON cs.course_id = c.id
		LEFT JOIN submissions s ON s.course_id = c.id AND s.class_id = ? AND s.student_id = ?
		WHERE c.id = ? AND cs.id = ? AND c.deleted_at IS NULL
	`, classID, studentID, courseID, sectionID).Scan(
		&context.Course.ID, &context.Course.TemplateID, &context.Course.TemplateCode,
		&context.Course.Title, &context.Course.Mode, &context.Course.Description,
		&context.Course.LearningObjective, &context.Course.Status, &context.Course.CreatedAt,
		&context.Course.UpdatedAt, &context.Section.ID, &context.Section.CourseID,
		&context.Section.SectionKey, &context.Section.Title, &context.Section.Type,
		&context.Section.SortOrder, &enabled, &fixed, &rulesRaw, &context.SubmissionID,
	)
	if err != nil {
		return context, err
	}
	context.Section.Enabled = enabled == 1
	context.Section.Fixed = fixed == 1
	context.Section.Rules = json.RawMessage(rulesRaw)
	context.Config = parseAIGuidanceConfig(context.Section)
	context.StudentID = studentID
	if context.Course.Mode != "reflection" || !context.Config.Enabled ||
		(context.Config.Phase != "plan" && context.Config.Phase != "evaluation") {
		return context, errors.New("当前部分未开启AI学习指导")
	}
	if strings.TrimSpace(context.Course.LearningObjective) == "" {
		return context, errors.New("当前课堂尚未填写学习目标")
	}
	return context, nil
}

func (r *Repository) LoadAIGuidanceSession(courseID, classID, studentID, sectionID int) (*AIGuidanceSession, AIGuidanceConfig, error) {
	context, err := r.guidanceContext(courseID, classID, studentID, sectionID)
	if err != nil {
		return nil, AIGuidanceConfig{}, err
	}
	if context.SubmissionID == 0 {
		return nil, context.Config, nil
	}
	session, err := r.loadAIGuidanceSessionBySubmission(context.SubmissionID, sectionID)
	if err == sql.ErrNoRows {
		return nil, context.Config, nil
	}
	return session, context.Config, err
}

func (r *Repository) loadAIGuidanceSessionBySubmission(submissionID, sectionID int) (*AIGuidanceSession, error) {
	var item AIGuidanceSession
	err := r.db.QueryRow(`
		SELECT id, submission_id, section_id, phase, status, model, error_message,
		       COALESCE(generated_at, ''), COALESCE(completed_at, ''), created_at, updated_at
		FROM ai_guidance_sessions
		WHERE submission_id = ? AND section_id = ?
	`, submissionID, sectionID).Scan(
		&item.ID, &item.SubmissionID, &item.SectionID, &item.Phase, &item.Status,
		&item.Model, &item.ErrorMessage, &item.GeneratedAt, &item.CompletedAt,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	item.Title = guidanceTitle(item.Phase)
	item.Messages, err = r.listAIGuidanceMessages(item.ID)
	if err != nil {
		return nil, err
	}
	for _, message := range item.Messages {
		if message.Role == "user" {
			item.FollowUpRounds++
		}
	}
	return &item, nil
}

func (r *Repository) listAIGuidanceMessages(sessionID int) ([]AIGuidanceMessage, error) {
	rows, err := r.db.Query(`
		SELECT id, message_order, role, content, created_at
		FROM ai_guidance_messages
		WHERE session_id = ?
		ORDER BY message_order, id
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AIGuidanceMessage{}
	for rows.Next() {
		var item AIGuidanceMessage
		if err := rows.Scan(&item.ID, &item.MessageOrder, &item.Role, &item.Content, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func guidanceTitle(phase string) string {
	if phase == "evaluation" {
		return "AI学习评价与反思"
	}
	return "AI学习计划"
}

func (r *Repository) StartAIGuidanceGeneration(context guidanceContext, snapshotJSON string) (*AIGuidanceSession, bool, error) {
	if context.SubmissionID <= 0 {
		return nil, false, errors.New("请先提交当前部分的答题内容")
	}
	var attemptCount int
	if err := r.db.QueryRow(`
		SELECT COUNT(1) FROM answer_attempts WHERE submission_id = ? AND section_id = ?
	`, context.SubmissionID, context.Section.ID).Scan(&attemptCount); err != nil {
		return nil, false, err
	}
	if context.Config.Phase == "plan" && attemptCount == 0 {
		return nil, false, errors.New("请先提交第一部分的预测和题目")
	}
	if context.Config.Phase == "evaluation" {
		if err := r.db.QueryRow(`
			SELECT COUNT(1)
			FROM answer_attempts aa
			JOIN course_sections cs ON cs.id = aa.section_id
			WHERE aa.submission_id = ? AND cs.type = 'quiz' AND aa.attempt_no = 1
		`, context.SubmissionID).Scan(&attemptCount); err != nil {
			return nil, false, err
		}
		if attemptCount == 0 {
			return nil, false, errors.New("请先完成第三部分的小测")
		}
	}
	existing, err := r.loadAIGuidanceSessionBySubmission(context.SubmissionID, context.Section.ID)
	if err == nil && (existing.Status == "ready" || existing.Status == "completed" || existing.Status == "skipped") {
		return existing, true, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return nil, false, err
	}
	_, err = r.db.Exec(`
		INSERT INTO ai_guidance_sessions (
			submission_id, section_id, phase, status, input_snapshot_json, model, error_message
		) VALUES (?, ?, ?, 'generating', ?, ?, '')
		ON CONFLICT(submission_id, section_id) DO UPDATE SET
			status = 'generating',
			input_snapshot_json = excluded.input_snapshot_json,
			model = excluded.model,
			error_message = '',
			generated_at = NULL,
			updated_at = CURRENT_TIMESTAMP
	`, context.SubmissionID, context.Section.ID, context.Config.Phase, snapshotJSON, aiGuidanceModel)
	if err != nil {
		return nil, false, err
	}
	session, err := r.loadAIGuidanceSessionBySubmission(context.SubmissionID, context.Section.ID)
	return session, false, err
}

func (r *Repository) FailAIGuidanceGeneration(sessionID int, generationErr error) error {
	_, err := r.db.Exec(`
		UPDATE ai_guidance_sessions
		SET status = 'failed', error_message = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, truncateRunes(generationErr.Error(), 1000), sessionID)
	return err
}

func (r *Repository) CompleteAIGuidanceGeneration(sessionID int, content string) (*AIGuidanceSession, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM ai_guidance_messages WHERE session_id = ?`, sessionID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`
		INSERT INTO ai_guidance_messages (session_id, message_order, role, content)
		VALUES (?, 1, 'assistant', ?)
	`, sessionID, content); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`
		UPDATE ai_guidance_sessions
		SET status = 'ready', error_message = '', generated_at = CURRENT_TIMESTAMP,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, sessionID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	var submissionID, sectionID int
	if err := r.db.QueryRow(`SELECT submission_id, section_id FROM ai_guidance_sessions WHERE id = ?`, sessionID).Scan(&submissionID, &sectionID); err != nil {
		return nil, err
	}
	return r.loadAIGuidanceSessionBySubmission(submissionID, sectionID)
}

func (r *Repository) AppendAIGuidanceExchange(sessionID int, userMessage, assistantMessage string) (*AIGuidanceSession, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var nextOrder int
	if err := tx.QueryRow(`
		SELECT COALESCE(MAX(message_order), 0) + 1 FROM ai_guidance_messages WHERE session_id = ?
	`, sessionID).Scan(&nextOrder); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`
		INSERT INTO ai_guidance_messages (session_id, message_order, role, content)
		VALUES (?, ?, 'user', ?), (?, ?, 'assistant', ?)
	`, sessionID, nextOrder, userMessage, sessionID, nextOrder+1, assistantMessage); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE ai_guidance_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, sessionID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	var submissionID, sectionID int
	if err := r.db.QueryRow(`SELECT submission_id, section_id FROM ai_guidance_sessions WHERE id = ?`, sessionID).Scan(&submissionID, &sectionID); err != nil {
		return nil, err
	}
	return r.loadAIGuidanceSessionBySubmission(submissionID, sectionID)
}

func (r *Repository) AIGuidanceSnapshot(sessionID int) (string, error) {
	var raw string
	err := r.db.QueryRow(`SELECT input_snapshot_json FROM ai_guidance_sessions WHERE id = ?`, sessionID).Scan(&raw)
	return raw, err
}

func (r *Repository) CompleteOrSkipAIGuidance(courseID, classID, studentID, sectionID int, skip bool) (*AIGuidanceSession, error) {
	context, err := r.guidanceContext(courseID, classID, studentID, sectionID)
	if err != nil {
		return nil, err
	}
	if context.SubmissionID == 0 {
		return nil, errors.New("请先提交当前部分")
	}
	session, err := r.loadAIGuidanceSessionBySubmission(context.SubmissionID, sectionID)
	if err == sql.ErrNoRows && skip {
		if _, err := r.db.Exec(`
			INSERT INTO ai_guidance_sessions (
				submission_id, section_id, phase, status, input_snapshot_json, model, completed_at
			) VALUES (?, ?, ?, 'skipped', '{}', ?, CURRENT_TIMESTAMP)
		`, context.SubmissionID, sectionID, context.Config.Phase, aiGuidanceModel); err != nil {
			return nil, err
		}
		return r.loadAIGuidanceSessionBySubmission(context.SubmissionID, sectionID)
	}
	if err != nil {
		return nil, err
	}
	status := "completed"
	if skip {
		status = "skipped"
	} else if session.Status != "ready" && session.Status != "completed" {
		return nil, errors.New("AI学习指导尚未生成完成，可以重试或选择跳过")
	}
	if _, err := r.db.Exec(`
		UPDATE ai_guidance_sessions
		SET status = ?, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, session.ID); err != nil {
		return nil, err
	}
	return r.loadAIGuidanceSessionBySubmission(context.SubmissionID, sectionID)
}

func (r *Repository) CompleteReflectionGuidanceTx(tx *sql.Tx, submissionID, sectionID int) error {
	var configRaw, sectionKey string
	if err := tx.QueryRow(`SELECT section_key, rules_json FROM course_sections WHERE id = ?`, sectionID).Scan(&sectionKey, &configRaw); err != nil {
		return err
	}
	if sectionKey != "reflection" {
		return nil
	}
	section := Section{SectionKey: sectionKey, Rules: json.RawMessage(configRaw)}
	config := parseAIGuidanceConfig(section)
	if !config.Enabled {
		return nil
	}
	var status string
	err := tx.QueryRow(`
		SELECT status FROM ai_guidance_sessions WHERE submission_id = ? AND section_id = ?
	`, submissionID, sectionID).Scan(&status)
	if err == sql.ErrNoRows {
		_, err = tx.Exec(`
			INSERT INTO ai_guidance_sessions (
				submission_id, section_id, phase, status, input_snapshot_json, model, completed_at
			) VALUES (?, ?, 'evaluation', 'skipped', '{}', ?, CURRENT_TIMESTAMP)
		`, submissionID, sectionID, aiGuidanceModel)
		return err
	}
	if err != nil {
		return err
	}
	nextStatus := "completed"
	if status == "failed" || status == "generating" || status == "pending" {
		nextStatus = "skipped"
	}
	_, err = tx.Exec(`
		UPDATE ai_guidance_sessions
		SET status = ?, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE submission_id = ? AND section_id = ?
	`, nextStatus, submissionID, sectionID)
	return err
}

func (r *Repository) BuildAIGuidanceSnapshot(context guidanceContext) (string, error) {
	if context.SubmissionID <= 0 {
		return "", errors.New("未找到学生的当前课堂答题记录")
	}
	detail, err := r.GetStudentDetail(context.SubmissionID)
	if err != nil {
		return "", err
	}
	history, err := r.guidanceHistory(context)
	if err != nil {
		return "", err
	}
	snapshot := map[string]interface{}{
		"phase":             context.Config.Phase,
		"courseName":        context.Course.Title,
		"learningObjective": missingText(context.Course.LearningObjective),
		"predictedScore":    scoreOrMissing(detail.PredictedScore),
		"previousCourses":   history,
	}
	if context.Config.Phase == "evaluation" {
		snapshot["officialQuizScore"] = scoreOrMissing(detail.QuizScore)
		snapshot["quizQuestions"] = firstQuizQuestionSnapshot(detail)
		snapshot["currentCourseOtherAnswers"] = currentCourseAnswerSnapshot(detail)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (r *Repository) guidanceHistory(context guidanceContext) ([]map[string]interface{}, error) {
	rows, err := r.db.Query(`
		SELECT c.title, c.mode, c.learning_objective, s.status,
		       sr.predicted_score, sr.actual_score, sr.teacher_score,
		       COALESCE(s.completed_at, s.updated_at, s.started_at)
		FROM submissions s
		JOIN courses c ON c.id = s.course_id AND c.deleted_at IS NULL
		LEFT JOIN score_records sr ON sr.submission_id = s.id
		WHERE s.student_id = ? AND s.id <> ?
		  AND datetime(COALESCE(s.completed_at, s.updated_at, s.started_at)) <=
		      datetime((SELECT started_at FROM submissions WHERE id = ?))
		ORDER BY datetime(COALESCE(s.completed_at, s.updated_at, s.started_at)) ASC, s.id ASC
	`, context.StudentID, context.SubmissionID, context.SubmissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var title, mode, objective, status, occurredAt string
		var predicted, quiz, teacher sql.NullInt64
		if err := rows.Scan(&title, &mode, &objective, &status, &predicted, &quiz, &teacher, &occurredAt); err != nil {
			return nil, err
		}
		official := intFromNull(teacher)
		if official == nil {
			official = intFromNull(quiz)
		}
		items = append(items, map[string]interface{}{
			"courseName":        title,
			"mode":              mode,
			"learningObjective": missingText(objective),
			"completionStatus":  status,
			"predictedScore":    scoreOrMissing(intFromNull(predicted)),
			"officialScore":     scoreOrMissing(official),
			"time":              occurredAt,
		})
	}
	return items, rows.Err()
}

func firstQuizQuestionSnapshot(detail StudentDetail) []map[string]interface{} {
	items := []map[string]interface{}{}
	for _, section := range detail.Sections {
		if section.Type != "quiz" || len(section.Attempts) == 0 {
			continue
		}
		for _, question := range section.Attempts[0].Questions {
			items = append(items, map[string]interface{}{
				"question":      question.QuestionText,
				"studentAnswer": missingText(question.Answer),
				"correctAnswer": missingText(question.CorrectAnswer),
				"isCorrect":     question.IsCorrect,
				"explanation":   missingText(question.Explanation),
			})
		}
		break
	}
	return items
}

func currentCourseAnswerSnapshot(detail StudentDetail) []map[string]interface{} {
	items := []map[string]interface{}{}
	for _, section := range detail.Sections {
		if section.SortOrder > 3 || section.Type == "quiz" || len(section.Attempts) == 0 {
			continue
		}
		for _, question := range section.Attempts[0].Questions {
			var rules struct {
				ScoreRole string `json:"scoreRole"`
			}
			_ = json.Unmarshal(question.Rules, &rules)
			if rules.ScoreRole == "prediction" || rules.ScoreRole == "quiz" {
				continue
			}
			value := interface{}(missingText(question.Answer))
			if question.QuestionType == "ai_chat" {
				value = question.ChatMessages
			}
			items = append(items, map[string]interface{}{
				"section":  section.Title,
				"question": question.QuestionText,
				"type":     question.QuestionType,
				"answer":   value,
			})
		}
	}
	return items
}

func missingText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "未记录"
	}
	return value
}

func scoreOrMissing(value *int) interface{} {
	if value == nil {
		return "未记录"
	}
	return *value
}

func (r *Repository) ListAIGuidanceForSubmission(submissionID int) ([]AIGuidanceSession, error) {
	rows, err := r.db.Query(`
		SELECT section_id FROM ai_guidance_sessions
		WHERE submission_id = ? ORDER BY section_id
	`, submissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sectionIDs []int
	for rows.Next() {
		var sectionID int
		if err := rows.Scan(&sectionID); err != nil {
			return nil, err
		}
		sectionIDs = append(sectionIDs, sectionID)
	}
	items := []AIGuidanceSession{}
	for _, sectionID := range sectionIDs {
		session, err := r.loadAIGuidanceSessionBySubmission(submissionID, sectionID)
		if err != nil {
			return nil, err
		}
		items = append(items, *session)
	}
	return items, nil
}

func (r *Repository) effectiveCompletedSections(submissionID int, completed map[int]bool, sections []Section) (map[int]bool, error) {
	result := make(map[int]bool, len(completed))
	for sectionID, value := range completed {
		result[sectionID] = value
	}
	for _, section := range sections {
		config := parseAIGuidanceConfig(section)
		if !config.Enabled || !result[section.ID] {
			continue
		}
		var status string
		err := r.db.QueryRow(`
			SELECT status FROM ai_guidance_sessions
			WHERE submission_id = ? AND section_id = ?
		`, submissionID, section.ID).Scan(&status)
		if err == sql.ErrNoRows || (err == nil && status != "completed" && status != "skipped") {
			delete(result, section.ID)
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *Repository) FillAIGuidanceStats(stats *StatsSummary, courseID, classID int) error {
	for _, section := range stats.Sections {
		config := parseAIGuidanceConfig(section)
		if !config.Enabled {
			continue
		}
		target := &stats.AIGuidanceStats.Plan
		if config.Phase == "evaluation" {
			target = &stats.AIGuidanceStats.Evaluation
		}
		target.Enabled = true
		query := `
			SELECT ags.status,
			       ags.generated_at IS NOT NULL,
			       SUM(CASE WHEN agm.role = 'user' THEN 1 ELSE 0 END)
			FROM ai_guidance_sessions ags
			JOIN submissions s ON s.id = ags.submission_id
			LEFT JOIN ai_guidance_messages agm ON agm.session_id = ags.id
			WHERE s.course_id = ? AND ags.section_id = ?
		`
		args := []interface{}{courseID, section.ID}
		if classID > 0 {
			query += ` AND s.class_id = ?`
			args = append(args, classID)
		}
		query += ` GROUP BY ags.id, ags.status`
		rows, err := r.db.Query(query, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var status string
			var generated bool
			var rounds int
			if err := rows.Scan(&status, &generated, &rounds); err != nil {
				rows.Close()
				return err
			}
			if generated {
				target.GeneratedCount++
			}
			switch status {
			case "completed":
				target.CompletedCount++
			case "skipped":
				target.SkippedCount++
			case "failed":
				target.FailedCount++
			}
			target.FollowUpRounds += rounds
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) AIGuidanceSessionByID(sessionID int) (*AIGuidanceSession, error) {
	var submissionID, sectionID int
	if err := r.db.QueryRow(`
		SELECT submission_id, section_id FROM ai_guidance_sessions WHERE id = ?
	`, sessionID).Scan(&submissionID, &sectionID); err != nil {
		return nil, err
	}
	return r.loadAIGuidanceSessionBySubmission(submissionID, sectionID)
}

func (r *Repository) ValidateAIGuidanceSessionOwner(sessionID, courseID, classID, studentID int) error {
	var count int
	if err := r.db.QueryRow(`
		SELECT COUNT(1)
		FROM ai_guidance_sessions ags
		JOIN submissions s ON s.id = ags.submission_id
		WHERE ags.id = ? AND s.course_id = ? AND s.class_id = ? AND s.student_id = ?
	`, sessionID, courseID, classID, studentID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("AI学习指导会话不存在")
	}
	return nil
}
