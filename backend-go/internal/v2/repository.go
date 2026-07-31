package v2

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListClasses() ([]Class, error) {
	rows, err := r.db.Query(`
		SELECT c.id, c.name, c.description, COUNT(s.id), c.created_at, c.updated_at
		FROM classes c
		LEFT JOIN students s ON s.class_id = c.id AND s.deleted_at IS NULL
		WHERE c.deleted_at IS NULL
		GROUP BY c.id
		ORDER BY c.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Class
	for rows.Next() {
		var item Class
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.StudentCount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateClass(name, description string) (int, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("班级名称不能为空")
	}
	res, err := r.db.Exec(`INSERT INTO classes (name, description) VALUES (?, ?)`, name, strings.TrimSpace(description))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, errors.New("班级名称已存在")
		}
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (r *Repository) DeleteClass(id int) error {
	_, err := r.db.Exec(`UPDATE classes SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

func (r *Repository) ListStudents(classID int) ([]Student, error) {
	rows, err := r.db.Query(`
		SELECT id, class_id, name, student_no, created_at, updated_at
		FROM students
		WHERE class_id = ? AND deleted_at IS NULL
		ORDER BY name
	`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Student
	for rows.Next() {
		var item Student
		if err := rows.Scan(&item.ID, &item.ClassID, &item.Name, &item.StudentNo, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateStudent(classID int, name, studentNo string) (int, error) {
	name = strings.TrimSpace(name)
	if classID <= 0 || name == "" {
		return 0, errors.New("班级和学生姓名不能为空")
	}
	res, err := r.db.Exec(`INSERT INTO students (class_id, name, student_no) VALUES (?, ?, ?)`, classID, name, strings.TrimSpace(studentNo))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, fmt.Errorf("%s同学已存在，无法重复添加", name)
		}
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (r *Repository) CreateStudentsBatch(classID int, students []Student) (ids []int, skipped []string, err error) {
	if classID <= 0 {
		return nil, nil, errors.New("班级ID无效")
	}
	if len(students) == 0 {
		return nil, nil, errors.New("学生列表不能为空")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO students (class_id, name, student_no) VALUES (?, ?, ?)`)
	if err != nil {
		return nil, nil, err
	}
	defer stmt.Close()

	ids = make([]int, 0, len(students))
	skipped = make([]string, 0)
	for _, s := range students {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}
		res, err := stmt.Exec(classID, name, strings.TrimSpace(s.StudentNo))
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				skipped = append(skipped, name)
				continue
			}
			return nil, nil, err
		}
		id, _ := res.LastInsertId()
		ids = append(ids, int(id))
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return ids, skipped, nil
}

func (r *Repository) DeleteStudent(id int) error {
	_, err := r.db.Exec(`UPDATE students SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

func (r *Repository) ListTemplates() ([]CourseTemplate, error) {
	rows, err := r.db.Query(`SELECT id, code, name, description, config_json FROM course_templates ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CourseTemplate
	for rows.Next() {
		var raw string
		var item CourseTemplate
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.Description, &raw); err != nil {
			return nil, err
		}
		item.Config = json.RawMessage(raw)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListCourses() ([]Course, error) {
	rows, err := r.db.Query(`
		SELECT id, COALESCE(template_id, 0), template_code, title, mode, description, learning_objective, status, created_at, updated_at
		FROM courses
		WHERE deleted_at IS NULL
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Course{}
	for rows.Next() {
		var item Course
		if err := rows.Scan(&item.ID, &item.TemplateID, &item.TemplateCode, &item.Title, &item.Mode, &item.Description, &item.LearningObjective, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateCourse(title, description, learningObjective, templateCode string) (int, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return 0, errors.New("课程名称不能为空")
	}
	var exists int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM courses WHERE title = ? AND deleted_at IS NULL`, title).Scan(&exists); err != nil {
		return 0, err
	}
	if exists > 0 {
		return 0, errors.New("课堂名称已存在，请换一个名称")
	}
	if templateCode == "" {
		templateCode = "free"
	}
	var template CourseTemplate
	var rawConfig string
	err := r.db.QueryRow(`SELECT id, code, name, description, config_json FROM course_templates WHERE code = ?`, templateCode).
		Scan(&template.ID, &template.Code, &template.Name, &template.Description, &rawConfig)
	if err != nil {
		return 0, err
	}
	template.Config = json.RawMessage(rawConfig)

	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`
		INSERT INTO courses (template_id, template_code, title, mode, description, learning_objective, status)
		VALUES (?, ?, ?, ?, ?, ?, 'active')
	`, template.ID, template.Code, title, template.Code, strings.TrimSpace(description), strings.TrimSpace(learningObjective))
	if err != nil {
		return 0, err
	}
	courseID64, _ := res.LastInsertId()
	courseID := int(courseID64)
	if err := createSectionsFromTemplate(tx, courseID, template); err != nil {
		return 0, err
	}
	if template.Code == "reflection" {
		if err := createReflectionDefaultQuestions(tx, courseID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return courseID, nil
}

func (r *Repository) UpdateCourse(id int, description, learningObjective string) error {
	if id <= 0 {
		return errors.New("课堂ID无效")
	}
	learningObjective = strings.TrimSpace(learningObjective)
	if learningObjective == "" {
		sections, err := r.ListSections(id)
		if err != nil {
			return err
		}
		for _, section := range sections {
			if parseAIGuidanceConfig(section).Enabled {
				return errors.New("请先关闭AI学习指导，再清空学习目标")
			}
		}
	}
	res, err := r.db.Exec(`
		UPDATE courses
		SET description = ?, learning_objective = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND deleted_at IS NULL
	`, strings.TrimSpace(description), learningObjective, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return errors.New("课堂不存在")
	}
	return nil
}

func createSectionsFromTemplate(tx *sql.Tx, courseID int, template CourseTemplate) error {
	var config struct {
		Sections []struct {
			Key   string                 `json:"key"`
			Title string                 `json:"title"`
			Type  string                 `json:"type"`
			Fixed bool                   `json:"fixed"`
			Rules map[string]interface{} `json:"rules"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(template.Config, &config); err != nil {
		return err
	}
	for index, section := range config.Sections {
		rules, _ := json.Marshal(section.Rules)
		if _, err := tx.Exec(`
			INSERT INTO course_sections (course_id, section_key, title, type, sort_order, fixed, rules_json)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, courseID, section.Key, section.Title, section.Type, index+1, boolToInt(section.Fixed), string(rules)); err != nil {
			return err
		}
	}
	return nil
}

func createReflectionDefaultQuestions(tx *sql.Tx, courseID int) error {
	sections, err := courseSectionIDs(tx, courseID)
	if err != nil {
		return err
	}
	predictionSectionID := sections["prediction"]
	quizSectionID := sections["quiz"]
	if predictionSectionID > 0 {
		options, _ := json.Marshal([]string{"0分 - 完全没把握", "1分 - 有一点把握", "2分 - 还需要努力", "3分 - 基本可以", "4分 - 比较有把握", "5分 - 非常有把握"})
		if err := insertDefaultQuestion(tx, courseID, predictionSectionID, "fixed_prediction_score", "single_choice", "你预测自己这节课能得到几分？", options, json.RawMessage("null"), "", 0, 1, `{"scoreRole":"prediction"}`); err != nil {
			return err
		}
	}
	if quizSectionID > 0 {
		part3Single, _ := json.Marshal([]string{"A. 数据", "B. 算法", "C. 规则", "D. 算力"})
		part3Judge, _ := json.Marshal([]string{"A. 对", "B. 错"})
		defaults := []struct {
			key, title  string
			options     json.RawMessage
			correct     json.RawMessage
			explanation string
			sortOrder   int
		}{
			{"fixed_quiz_1", "下面哪一项不是人工智能的三个核心要素？", part3Single, json.RawMessage(`"C"`), "人工智能的三个核心要素是数据、算法和算力。规则不是核心要素。", 1},
			{"fixed_quiz_2", "下面哪一项是让人工智能算得又快又猛的“火力”？", part3Single, json.RawMessage(`"D"`), "算力是让人工智能算得又快又猛的“火力”，它决定了人工智能的计算速度和能力。", 2},
			{"fixed_quiz_3", "人工智能是人类制造的、能模仿人类智力和能力的一种技术。", part3Judge, json.RawMessage(`"A"`), "这个说法是正确的。人工智能是人类制造的、能模仿人类智力和能力的一种技术。", 3},
			{"fixed_quiz_4", "学习累了的时候，可以让人工智能帮忙写作业。", part3Judge, json.RawMessage(`"B"`), "这个说法是错误的。我们应该自己完成作业，人工智能可以帮助我们学习，但不能代替我们写作业。", 4},
			{"fixed_quiz_5", "人工智能是一种可以帮助我们学习的工具。", part3Judge, json.RawMessage(`"A"`), "这个说法是正确的。人工智能是一种可以帮助我们学习的工具，我们应该正确使用它。", 5},
		}
		for _, item := range defaults {
			if err := insertDefaultQuestion(tx, courseID, quizSectionID, item.key, "single_choice", item.title, item.options, item.correct, item.explanation, 1, item.sortOrder, `{"scoreRole":"quiz"}`); err != nil {
				return err
			}
		}
	}
	return nil
}

func courseSectionIDs(tx *sql.Tx, courseID int) (map[string]int, error) {
	rows, err := tx.Query(`SELECT section_key, id FROM course_sections WHERE course_id = ?`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sections := map[string]int{}
	for rows.Next() {
		var key string
		var id int
		if err := rows.Scan(&key, &id); err != nil {
			return nil, err
		}
		sections[key] = id
	}
	return sections, rows.Err()
}

func insertDefaultQuestion(tx *sql.Tx, courseID, sectionID int, key, questionType, title string, options, correct json.RawMessage, explanation string, score, sortOrder int, rules string) error {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM questions WHERE course_id = ? AND question_key = ?`, courseID, key).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := tx.Exec(`
		INSERT INTO questions (course_id, section_id, question_key, type, title, options_json, correct_answer_json, explanation, score, sort_order, fixed, rules_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
	`, courseID, sectionID, key, questionType, title, string(options), string(correct), explanation, score, sortOrder, rules)
	return err
}

func (r *Repository) DeleteCourse(id int) error {
	_, err := r.db.Exec(`UPDATE courses SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

func (r *Repository) CloneCourse(originalID int, newTitle string) (int, error) {
	newTitle = strings.TrimSpace(newTitle)
	if newTitle == "" {
		return 0, errors.New("课程名称不能为空")
	}

	// Check uniqueness
	var exists int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM courses WHERE title = ? AND deleted_at IS NULL`, newTitle).Scan(&exists); err != nil {
		return 0, err
	}
	if exists > 0 {
		return 0, errors.New("课堂名称已存在，请换一个名称")
	}

	// Fetch original course
	var orig Course
	err := r.db.QueryRow(`SELECT id, COALESCE(template_id,0), template_code, title, mode, description, learning_objective, status, created_at, updated_at
		FROM courses WHERE id = ? AND deleted_at IS NULL`, originalID).Scan(
		&orig.ID, &orig.TemplateID, &orig.TemplateCode, &orig.Title, &orig.Mode, &orig.Description, &orig.LearningObjective, &orig.Status, &orig.CreatedAt, &orig.UpdatedAt)
	if err != nil {
		return 0, fmt.Errorf("原课程不存在: %w", err)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Create new course
	res, err := tx.Exec(`INSERT INTO courses (template_id, template_code, title, mode, description, learning_objective, status)
		VALUES (?, ?, ?, ?, ?, ?, 'active')`, orig.TemplateID, orig.TemplateCode, newTitle, orig.Mode, orig.Description, orig.LearningObjective)
	if err != nil {
		return 0, err
	}
	newID64, _ := res.LastInsertId()
	newID := int(newID64)

	// Copy sections
	sectionRows, err := tx.Query(`SELECT id, section_key, title, type, sort_order, enabled, fixed, rules_json
		FROM course_sections WHERE course_id = ? ORDER BY sort_order`, originalID)
	if err != nil {
		return 0, err
	}
	defer sectionRows.Close()

	type sectionMapping struct {
		oldID int
		newID int
	}
	var sectionMap []sectionMapping

	for sectionRows.Next() {
		var oldSectID, sortOrder, enabled, fixed int
		var key, title, typ, rulesRaw string
		if err := sectionRows.Scan(&oldSectID, &key, &title, &typ, &sortOrder, &enabled, &fixed, &rulesRaw); err != nil {
			return 0, err
		}
		rulesRaw = disableAIGuidanceRule(rulesRaw)
		res, err := tx.Exec(`INSERT INTO course_sections (course_id, section_key, title, type, sort_order, enabled, fixed, rules_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, newID, key, title, typ, sortOrder, enabled, fixed, rulesRaw)
		if err != nil {
			return 0, err
		}
		newSectID64, _ := res.LastInsertId()
		sectionMap = append(sectionMap, sectionMapping{oldID: oldSectID, newID: int(newSectID64)})
	}
	if err := sectionRows.Err(); err != nil {
		return 0, err
	}

	// Copy questions for each section
	for _, sm := range sectionMap {
		qRows, err := tx.Query(`SELECT question_key, type, title, description, options_json, correct_answer_json,
			explanation, score, sort_order, enabled, fixed, rules_json
			FROM questions WHERE section_id = ? ORDER BY sort_order`, sm.oldID)
		if err != nil {
			return 0, err
		}
		for qRows.Next() {
			var key, qType, qTitle, qDesc, qOptions, qCorrect, qExplanation, qRules string
			var qScore, qSort, qEnabled, qFixed int
			if err := qRows.Scan(&key, &qType, &qTitle, &qDesc, &qOptions, &qCorrect, &qExplanation, &qScore, &qSort, &qEnabled, &qFixed, &qRules); err != nil {
				qRows.Close()
				return 0, err
			}
			if _, err := tx.Exec(`INSERT INTO questions (course_id, section_id, question_key, type, title, description, options_json,
				correct_answer_json, explanation, score, sort_order, enabled, fixed, rules_json)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				newID, sm.newID, key, qType, qTitle, qDesc, qOptions, qCorrect, qExplanation, qScore, qSort, qEnabled, qFixed, qRules); err != nil {
				qRows.Close()
				return 0, err
			}
		}
		qRows.Close()
		if err := qRows.Err(); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return newID, nil
}

func (r *Repository) BindCourseClass(courseID, classID int) error {
	if courseID <= 0 || classID <= 0 {
		return errors.New("课程和班级不能为空")
	}
	_, err := r.db.Exec(`
		INSERT INTO course_classes (course_id, class_id)
		VALUES (?, ?)
		ON CONFLICT(course_id, class_id) DO NOTHING
	`, courseID, classID)
	return err
}

func (r *Repository) SetClassCourses(classID int, courseIDs []int) error {
	if classID <= 0 {
		return errors.New("班级不能为空")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM course_classes WHERE class_id = ?`, classID); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO course_classes (course_id, class_id)
		VALUES (?, ?)
		ON CONFLICT(course_id, class_id) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	seen := map[int]bool{}
	for _, courseID := range courseIDs {
		if courseID <= 0 || seen[courseID] {
			continue
		}
		seen[courseID] = true
		if _, err := stmt.Exec(courseID, classID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func disableAIGuidanceRule(raw string) string {
	rules := map[string]interface{}{}
	if json.Unmarshal([]byte(raw), &rules) != nil {
		return raw
	}
	guidance, ok := rules["aiGuidance"].(map[string]interface{})
	if !ok {
		return raw
	}
	guidance["enabled"] = false
	rules["aiGuidance"] = guidance
	encoded, err := json.Marshal(rules)
	if err != nil {
		return raw
	}
	return string(encoded)
}

func (r *Repository) ListCoursesForClass(classID int) ([]Course, error) {
	rows, err := r.db.Query(`
		SELECT c.id, COALESCE(c.template_id, 0), c.template_code, c.title, c.mode, c.description, c.learning_objective, c.status, c.created_at, c.updated_at
		FROM courses c
		JOIN course_classes cc ON cc.course_id = c.id
		WHERE cc.class_id = ? AND c.deleted_at IS NULL
		ORDER BY c.id DESC
	`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Course{}
	for rows.Next() {
		var item Course
		if err := rows.Scan(&item.ID, &item.TemplateID, &item.TemplateCode, &item.Title, &item.Mode, &item.Description, &item.LearningObjective, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListSections(courseID int) ([]Section, error) {
	if err := r.EnsureReflectionDefaults(courseID); err != nil {
		return nil, err
	}
	rows, err := r.db.Query(`
		SELECT id, course_id, section_key, title, type, sort_order, enabled, fixed, rules_json
		FROM course_sections
		WHERE course_id = ?
		ORDER BY sort_order, id
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Section
	for rows.Next() {
		var item Section
		var enabled, fixed int
		var rules string
		if err := rows.Scan(&item.ID, &item.CourseID, &item.SectionKey, &item.Title, &item.Type, &item.SortOrder, &enabled, &fixed, &rules); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		item.Fixed = fixed == 1
		item.Rules = json.RawMessage(rules)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) EnsureReflectionDefaults(courseID int) error {
	if courseID <= 0 {
		return nil
	}
	var mode string
	err := r.db.QueryRow(`SELECT mode FROM courses WHERE id = ? AND deleted_at IS NULL`, courseID).Scan(&mode)
	if err == sql.ErrNoRows || mode != "reflection" {
		return nil
	}
	if err != nil {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := createReflectionDefaultQuestions(tx, courseID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) CreateSection(courseID int, title, sectionType string) (int, error) {
	title = strings.TrimSpace(title)
	if courseID <= 0 || title == "" {
		return 0, errors.New("课程和部分名称不能为空")
	}
	if sectionType == "" {
		sectionType = "custom"
	}
	var nextOrder int
	_ = r.db.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM course_sections WHERE course_id = ?`, courseID).Scan(&nextOrder)
	key := fmt.Sprintf("custom_%d", nextOrder)
	res, err := r.db.Exec(`
		INSERT INTO course_sections (course_id, section_key, title, type, sort_order)
		VALUES (?, ?, ?, ?, ?)
	`, courseID, key, title, sectionType, nextOrder)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (r *Repository) ListQuestions(sectionID int) ([]Question, error) {
	rows, err := r.db.Query(`
		SELECT id, course_id, section_id, question_key, type, title, description, options_json,
		       correct_answer_json, explanation, score, sort_order, enabled, fixed, rules_json
		FROM questions
		WHERE section_id = ?
		ORDER BY sort_order, id
	`, sectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Question
	for rows.Next() {
		var item Question
		var options, correct, rules string
		var enabled, fixed int
		if err := rows.Scan(&item.ID, &item.CourseID, &item.SectionID, &item.QuestionKey, &item.Type, &item.Title, &item.Description, &options, &correct, &item.Explanation, &item.Score, &item.SortOrder, &enabled, &fixed, &rules); err != nil {
			return nil, err
		}
		item.Options = json.RawMessage(options)
		item.CorrectAnswer = json.RawMessage(correct)
		item.Rules = json.RawMessage(rules)
		item.Enabled = enabled == 1
		item.Fixed = fixed == 1
		// The prediction question is a reflection-mode contract. Older saves may
		// have overwritten its rules, so restore its display metadata on read.
		if item.QuestionKey == "fixed_prediction_score" {
			normalizeFixedPredictionQuestion(&item)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateQuestion(q Question) (int, error) {
	if q.SectionID <= 0 || strings.TrimSpace(q.Title) == "" {
		return 0, errors.New("部分和题目内容不能为空")
	}
	if locked, err := r.isLockedReflectionQuizSection(q.SectionID); err != nil {
		return 0, err
	} else if locked {
		return 0, errors.New("反思模式第三部分固定为 5 道小测题，不能新增题目")
	}
	if q.Type == "" {
		q.Type = "open_text"
	}
	q = normalizeOpenTextQuestion(q)
	if len(q.Options) == 0 {
		q.Options = json.RawMessage("[]")
	}
	if len(q.CorrectAnswer) == 0 {
		q.CorrectAnswer = json.RawMessage("null")
	}
	if len(q.Rules) == 0 {
		q.Rules = json.RawMessage("{}")
	}
	if q.CourseID <= 0 {
		if err := r.db.QueryRow(`SELECT course_id FROM course_sections WHERE id = ?`, q.SectionID).Scan(&q.CourseID); err != nil {
			return 0, err
		}
	}
	var nextOrder int
	_ = r.db.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM questions WHERE section_id = ?`, q.SectionID).Scan(&nextOrder)
	res, err := r.db.Exec(`
		INSERT INTO questions (course_id, section_id, question_key, type, title, description, options_json, correct_answer_json, explanation, score, sort_order, rules_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, q.CourseID, q.SectionID, q.QuestionKey, q.Type, strings.TrimSpace(q.Title), q.Description, string(q.Options), string(q.CorrectAnswer), q.Explanation, q.Score, nextOrder, string(q.Rules))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (r *Repository) DeleteQuestion(id int) error {
	if locked, err := r.isLockedReflectionQuizQuestion(id); err != nil {
		return err
	} else if locked {
		return errors.New("反思模式第三部分固定为 5 道小测题，不能停用或删除题目")
	}
	_, err := r.db.Exec(`UPDATE questions SET enabled = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

func (r *Repository) isLockedReflectionQuizSection(sectionID int) (bool, error) {
	var mode, sectionType string
	var fixed int
	err := r.db.QueryRow(`
		SELECT c.mode, cs.type, cs.fixed
		FROM course_sections cs
		JOIN courses c ON c.id = cs.course_id
		WHERE cs.id = ? AND c.deleted_at IS NULL
	`, sectionID).Scan(&mode, &sectionType, &fixed)
	if err != nil {
		return false, err
	}
	return mode == "reflection" && sectionType == "quiz" && fixed == 1, nil
}

func (r *Repository) isLockedReflectionQuizQuestion(questionID int) (bool, error) {
	var sectionID int
	if err := r.db.QueryRow(`SELECT section_id FROM questions WHERE id = ?`, questionID).Scan(&sectionID); err != nil {
		return false, err
	}
	return r.isLockedReflectionQuizSection(sectionID)
}

func (r *Repository) ValidateAIChatQuestion(courseID, sectionID, questionID int) error {
	var questionType string
	var enabled int
	err := r.db.QueryRow(`
		SELECT q.type, q.enabled
		FROM questions q
		JOIN course_sections cs ON cs.id = q.section_id
		WHERE q.id = ? AND q.section_id = ? AND q.course_id = ? AND cs.course_id = ?
	`, questionID, sectionID, courseID, courseID).Scan(&questionType, &enabled)
	if err != nil {
		return errors.New("未找到对应的 AI 对话题")
	}
	if enabled != 1 {
		return errors.New("该 AI 对话题未启用")
	}
	if questionType != "ai_chat" {
		return errors.New("当前题目不是 AI 对话题")
	}
	return nil
}

func (r *Repository) UpdateQuestion(q Question) error {
	if q.ID <= 0 || strings.TrimSpace(q.Title) == "" {
		return errors.New("题目ID和题目内容不能为空")
	}
	if q.Type == "" {
		q.Type = "open_text"
	}
	var questionKey string
	if err := r.db.QueryRow(`SELECT question_key FROM questions WHERE id = ?`, q.ID).Scan(&questionKey); err != nil {
		return err
	}
	// Teachers can customize the wording, but the reflection prediction card
	// must always keep its six score choices and scoreRole metadata.
	if questionKey == "fixed_prediction_score" {
		normalizeFixedPredictionQuestion(&q)
	}
	q = normalizeOpenTextQuestion(q)
	if len(q.Options) == 0 {
		q.Options = json.RawMessage("[]")
	}
	if len(q.CorrectAnswer) == 0 {
		q.CorrectAnswer = json.RawMessage("null")
	}
	if len(q.Rules) == 0 {
		q.Rules = json.RawMessage("{}")
	}
	_, err := r.db.Exec(`
		UPDATE questions
		SET type = ?,
		    title = ?,
		    description = ?,
		    options_json = ?,
		    correct_answer_json = ?,
		    explanation = ?,
		    score = ?,
		    rules_json = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, q.Type, strings.TrimSpace(q.Title), q.Description, string(q.Options), string(q.CorrectAnswer), q.Explanation, q.Score, string(q.Rules), q.ID)
	return err
}

func normalizeFixedPredictionQuestion(q *Question) {
	options, _ := json.Marshal([]string{
		"0分 - 完全没把握",
		"1分 - 有一点把握",
		"2分 - 还需要努力",
		"3分 - 基本可以",
		"4分 - 比较有把握",
		"5分 - 非常有把握",
	})
	q.Type = "single_choice"
	q.Options = options
	q.Rules = json.RawMessage(`{"scoreRole":"prediction"}`)
}

func (r *Repository) SubmitSection(courseID, classID, studentID, sectionID int, answers map[string]json.RawMessage) (map[string]interface{}, error) {
	if courseID <= 0 || classID <= 0 || studentID <= 0 || sectionID <= 0 {
		return nil, errors.New("课堂、班级、学生和部分不能为空")
	}
	section, err := r.getSection(sectionID)
	if err != nil {
		return nil, err
	}
	questions, err := r.ListQuestions(sectionID)
	if err != nil {
		return nil, err
	}
	for _, q := range questions {
		if !q.Enabled || q.Type != "open_text" {
			continue
		}
		key := strconv.Itoa(q.ID)
		normalized, err := normalizeOpenTextAnswer(answers[key])
		if err != nil {
			return nil, fmt.Errorf("%s：%w", q.Title, err)
		}
		answers[key] = normalized
	}
	lockedReflectionQuiz, err := r.isLockedReflectionQuizSection(sectionID)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	submissionID, err := ensureSubmissionTx(tx, courseID, classID, studentID)
	if err != nil {
		return nil, err
	}
	attemptNo, err := nextAttemptNoTx(tx, submissionID, sectionID)
	if err != nil {
		return nil, err
	}
	if lockedReflectionQuiz && attemptNo > 2 {
		return nil, errors.New("第三部分小测最多只能重测一次")
	}
	score, total, results := scoreQuestions(questions, answers)
	res, err := tx.Exec(`
		INSERT INTO answer_attempts (submission_id, section_id, attempt_no, score)
		VALUES (?, ?, ?, ?)
	`, submissionID, sectionID, attemptNo, nullableScore(section.Type, score))
	if err != nil {
		return nil, err
	}
	attemptID64, _ := res.LastInsertId()
	attemptID := int(attemptID64)
	for _, q := range questions {
		if !q.Enabled {
			continue
		}
		answer := answers[strconv.Itoa(q.ID)]
		if len(answer) == 0 {
			answer = json.RawMessage("null")
		}
		isCorrect := questionCorrect(q, answer)
		scoreValue := 0
		if isCorrect {
			scoreValue = q.Score
		}
		if _, err := tx.Exec(`
			INSERT INTO answer_records (attempt_id, question_id, answer_json, score, is_correct)
			VALUES (?, ?, ?, ?, ?)
		`, attemptID, q.ID, string(answer), scoreValue, boolToInt(isCorrect)); err != nil {
			return nil, err
		}
	}
	if err := updateScoreRecordTx(tx, submissionID, section.Type, answers, score); err != nil {
		return nil, err
	}
	if lockedReflectionQuiz && attemptNo == 2 {
		if err := r.ResetEvaluationGuidanceAfterRetakeTx(tx, submissionID, courseID); err != nil {
			return nil, err
		}
	}
	if section.Type == "reflection" {
		if err := r.CompleteReflectionGuidanceTx(tx, submissionID, sectionID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`UPDATE submissions SET status = 'completed', completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, submissionID); err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.Exec(`UPDATE submissions SET status = 'in_progress', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, submissionID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"submissionId": submissionID,
		"attemptNo":    attemptNo,
		"score":        score,
		"total":        total,
		"results":      results,
	}, nil
}

func (r *Repository) getSection(sectionID int) (Section, error) {
	var item Section
	var enabled, fixed int
	var rules string
	err := r.db.QueryRow(`
		SELECT id, course_id, section_key, title, type, sort_order, enabled, fixed, rules_json
		FROM course_sections
		WHERE id = ?
	`, sectionID).Scan(&item.ID, &item.CourseID, &item.SectionKey, &item.Title, &item.Type, &item.SortOrder, &enabled, &fixed, &rules)
	item.Enabled = enabled == 1
	item.Fixed = fixed == 1
	item.Rules = json.RawMessage(rules)
	return item, err
}

func ensureSubmissionTx(tx *sql.Tx, courseID, classID, studentID int) (int, error) {
	if _, err := tx.Exec(`
		INSERT INTO submissions (course_id, class_id, student_id)
		VALUES (?, ?, ?)
		ON CONFLICT(course_id, student_id) DO UPDATE SET updated_at = CURRENT_TIMESTAMP
	`, courseID, classID, studentID); err != nil {
		return 0, err
	}
	var id int
	err := tx.QueryRow(`SELECT id FROM submissions WHERE course_id = ? AND student_id = ?`, courseID, studentID).Scan(&id)
	return id, err
}

func nextAttemptNoTx(tx *sql.Tx, submissionID, sectionID int) (int, error) {
	var next int
	err := tx.QueryRow(`SELECT COALESCE(MAX(attempt_no), 0) + 1 FROM answer_attempts WHERE submission_id = ? AND section_id = ?`, submissionID, sectionID).Scan(&next)
	return next, err
}

func nullableScore(sectionType string, score int) interface{} {
	if sectionType == "quiz" {
		return score
	}
	return nil
}

func scoreQuestions(questions []Question, answers map[string]json.RawMessage) (int, int, []map[string]interface{}) {
	score := 0
	total := 0
	results := []map[string]interface{}{}
	for _, q := range questions {
		if !q.Enabled {
			continue
		}
		answer := answers[strconv.Itoa(q.ID)]
		correct := questionCorrect(q, answer)
		if q.Score > 0 {
			total += q.Score
		}
		if correct && q.Score > 0 {
			score += q.Score
		}
		results = append(results, map[string]interface{}{
			"questionId":    q.ID,
			"question":      q.Title,
			"studentAnswer": rawAnswerText(answer),
			"correctAnswer": rawAnswerText(q.CorrectAnswer),
			"explanation":   q.Explanation,
			"isCorrect":     correct,
			"chatMessages":  aiChatMessagesFromRaw(answer),
		})
	}
	return score, total, results
}

func questionCorrect(q Question, answer json.RawMessage) bool {
	if len(q.CorrectAnswer) == 0 || string(q.CorrectAnswer) == "null" {
		return false
	}
	return strings.Trim(rawAnswerText(answer), `" `) == strings.Trim(rawAnswerText(q.CorrectAnswer), `" `)
}

func rawAnswerText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if messages := aiChatMessagesFromRaw(raw); len(messages) > 0 {
		lines := make([]string, 0, len(messages))
		for _, message := range messages {
			role := "AI"
			if message.Role == "user" {
				role = "学生"
			}
			lines = append(lines, fmt.Sprintf("%s：%s", role, strings.TrimSpace(message.Content)))
		}
		return strings.Join(lines, "\n")
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		return strings.Join(values, "、")
	}
	return string(raw)
}

func aiChatMessagesFromRaw(raw json.RawMessage) []AIChatMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var wrapped struct {
		Messages []AIChatMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && len(wrapped.Messages) > 0 {
		return normalizeStoredAIChatMessages(wrapped.Messages)
	}
	var messages []AIChatMessage
	if err := json.Unmarshal(raw, &messages); err == nil && len(messages) > 0 {
		return normalizeStoredAIChatMessages(messages)
	}
	return nil
}

func normalizeStoredAIChatMessages(messages []AIChatMessage) []AIChatMessage {
	result := make([]AIChatMessage, 0, len(messages))
	for _, item := range messages {
		role := strings.TrimSpace(item.Role)
		content := strings.TrimSpace(item.Content)
		if content == "" || (role != "user" && role != "assistant") {
			continue
		}
		result = append(result, AIChatMessage{Role: role, Content: content})
	}
	return result
}

func updateScoreRecordTx(tx *sql.Tx, submissionID int, sectionType string, answers map[string]json.RawMessage, score int) error {
	if _, err := tx.Exec(`
		INSERT INTO score_records (submission_id)
		VALUES (?)
		ON CONFLICT(submission_id) DO NOTHING
	`, submissionID); err != nil {
		return err
	}
	switch sectionType {
	case "prediction":
		predicted := firstIntAnswer(answers)
		if predicted == nil {
			return nil
		}
		_, err := tx.Exec(`
			UPDATE score_records
			SET predicted_score = ?, guess_result = CASE
				WHEN actual_score IS NULL THEN 'unknown'
				WHEN ? = actual_score THEN 'correct'
				WHEN ? > actual_score THEN 'high'
				ELSE 'low'
			END, updated_at = CURRENT_TIMESTAMP
			WHERE submission_id = ?
		`, *predicted, *predicted, *predicted, submissionID)
		return err
	case "quiz":
		_, err := tx.Exec(`
			UPDATE score_records
			SET actual_score = COALESCE(actual_score, ?),
			    actual_score_source = CASE
					WHEN teacher_score IS NOT NULL THEN 'teacher'
					WHEN actual_score IS NULL THEN 'quiz'
					ELSE actual_score_source
				END,
			    guess_result = CASE
					WHEN teacher_score IS NOT NULL THEN CASE
						WHEN predicted_score IS NULL OR teacher_score IS NULL THEN 'unknown'
						WHEN predicted_score = teacher_score THEN 'correct'
						WHEN predicted_score > teacher_score THEN 'high'
						ELSE 'low'
					END
					WHEN predicted_score IS NULL THEN 'unknown'
					WHEN predicted_score = COALESCE(actual_score, ?) THEN 'correct'
					WHEN predicted_score > COALESCE(actual_score, ?) THEN 'high'
					ELSE 'low'
				END,
			    updated_at = CURRENT_TIMESTAMP
			WHERE submission_id = ?
		`, score, score, score, submissionID)
		return err
	default:
		return nil
	}
}

func compareGuessResult(predicted, actual *int) string {
	if predicted == nil || actual == nil {
		return "unknown"
	}
	if *predicted == *actual {
		return "correct"
	}
	if *predicted > *actual {
		return "high"
	}
	return "low"
}

func scoreSourceLabel(actualScoreSource string, teacherScore *int, quizScore *int) string {
	switch {
	case teacherScore != nil:
		return "teacher"
	case quizScore != nil:
		if actualScoreSource == "" || actualScoreSource == "none" {
			return "quiz"
		}
		return actualScoreSource
	default:
		return "none"
	}
}

func intFromNull(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int64)
	return &v
}

func firstValidScore(predicted, quiz, teacher sql.NullInt64) *int {
	switch {
	case teacher.Valid:
		v := int(teacher.Int64)
		return &v
	case quiz.Valid:
		v := int(quiz.Int64)
		return &v
	default:
		return nil
	}
}

func teacherScoreRows(predicted, quiz, teacher sql.NullInt64, actualSource string, note string) (predictedScore, quizScore, teacherScore, actualScore *int, actualScoreSource, guessResult, guessResultLabel string, teacherScoreNote string) {
	predictedScore = intFromNull(predicted)
	quizScore = intFromNull(quiz)
	teacherScore = intFromNull(teacher)
	actualScore = firstValidScore(predicted, quiz, teacher)
	actualScoreSource = scoreSourceLabel(actualSource, teacherScore, quizScore)
	teacherScoreNote = note
	if predictedScore == nil || actualScore == nil {
		guessResult = "unknown"
	} else {
		guessResult = compareGuessResult(predictedScore, actualScore)
	}
	guessResultLabel = guessResultText(guessResult)
	return
}

func firstIntAnswer(answers map[string]json.RawMessage) *int {
	for _, raw := range answers {
		var value int
		if err := json.Unmarshal(raw, &value); err == nil {
			return &value
		}
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			if parsed, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(text, "分"))); err == nil {
				return &parsed
			}
		}
	}
	return nil
}

func (r *Repository) SaveTeacherScore(submissionID int, teacherScore *int, note string) error {
	if submissionID <= 0 {
		return errors.New("提交ID无效")
	}
	_, err := r.SaveTeacherScoreForStudent(submissionID, 0, 0, 0, teacherScore, note)
	return err
}

func (r *Repository) SaveTeacherScoreForStudent(submissionID, courseID, classID, studentID int, teacherScore *int, note string) (int, error) {
	if teacherScore != nil && (*teacherScore < 0 || *teacherScore > 5) {
		return 0, errors.New("教师评分必须是0到5分")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if submissionID <= 0 {
		if courseID <= 0 || classID <= 0 || studentID <= 0 {
			return 0, errors.New("课堂、班级和学生信息不能为空")
		}
		var valid int
		if err := tx.QueryRow(`
			SELECT COUNT(1)
			FROM students s
			JOIN classes c ON c.id = s.class_id AND c.deleted_at IS NULL
			JOIN courses co ON co.id = ? AND co.deleted_at IS NULL
			WHERE s.id = ? AND s.class_id = ? AND s.deleted_at IS NULL
		`, courseID, studentID, classID).Scan(&valid); err != nil {
			return 0, err
		}
		if valid == 0 {
			return 0, errors.New("课堂、班级或学生信息无效")
		}
		if _, err := tx.Exec(`
			INSERT INTO submissions (course_id, class_id, student_id, status)
			VALUES (?, ?, ?, 'teacher_scored')
			ON CONFLICT(course_id, student_id) DO NOTHING
		`, courseID, classID, studentID); err != nil {
			return 0, err
		}
		if err := tx.QueryRow(`SELECT id FROM submissions WHERE course_id = ? AND student_id = ?`, courseID, studentID).Scan(&submissionID); err != nil {
			return 0, err
		}
	}

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM submissions WHERE id = ?`, submissionID).Scan(&exists); err != nil {
		return 0, err
	}
	if exists == 0 {
		return 0, errors.New("提交记录不存在")
	}

	note = strings.TrimSpace(note)
	if _, err := tx.Exec(`
		INSERT INTO score_records (submission_id)
		VALUES (?)
		ON CONFLICT(submission_id) DO NOTHING
	`, submissionID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`
		UPDATE score_records
		SET teacher_score = ?,
		    teacher_score_note = ?,
		    teacher_score_updated_at = CURRENT_TIMESTAMP,
		    actual_score_source = CASE
				WHEN ? IS NOT NULL THEN 'teacher'
				WHEN actual_score IS NULL THEN 'none'
				ELSE 'quiz'
			END,
		    guess_result = CASE
				WHEN ? IS NOT NULL THEN CASE
					WHEN predicted_score IS NULL THEN 'unknown'
					WHEN predicted_score = ? THEN 'correct'
					WHEN predicted_score > ? THEN 'high'
					ELSE 'low'
				END
				WHEN actual_score IS NULL OR predicted_score IS NULL THEN 'unknown'
				WHEN predicted_score = actual_score THEN 'correct'
				WHEN predicted_score > actual_score THEN 'high'
				ELSE 'low'
			END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE submission_id = ?
	`, nullableInt(teacherScore), note, nullableInt(teacherScore), nullableInt(teacherScore), nullableInt(teacherScore), nullableInt(teacherScore), submissionID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return submissionID, nil
}

func nullableInt(value *int) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func (r *Repository) GetScoreSummary(courseID, classID, studentID int) (ScoreSummary, error) {
	var item ScoreSummary
	item.ActualScoreSource = "none"
	item.GuessResult = "unknown"
	var submissionID int
	var predicted, quizScore, teacherScore sql.NullInt64
	var actualSource, teacherNote string
	err := r.db.QueryRow(`
		SELECT s.id, sr.predicted_score, sr.actual_score, COALESCE(sr.teacher_score, NULL), COALESCE(sr.actual_score_source, 'none'), COALESCE(sr.teacher_score_note, '')
		FROM submissions s
		LEFT JOIN score_records sr ON sr.submission_id = s.id
		WHERE s.course_id = ? AND s.class_id = ? AND s.student_id = ?
	`, courseID, classID, studentID).Scan(&submissionID, &predicted, &quizScore, &teacherScore, &actualSource, &teacherNote)
	if err == sql.ErrNoRows {
		item.GuessResultText = guessResultText(item.GuessResult)
		return item, nil
	}
	if err != nil {
		return item, err
	}
	item.PredictedScore, item.QuizScore, item.TeacherScore, item.ActualScore, item.ActualScoreSource, item.GuessResult, item.GuessResultText, item.TeacherScoreNote = teacherScoreRows(predicted, quizScore, teacherScore, actualSource, teacherNote)
	var retakeScore sql.NullInt64
	err = r.db.QueryRow(`
		SELECT aa.score
		FROM answer_attempts aa
		JOIN course_sections cs ON cs.id = aa.section_id
		WHERE aa.submission_id = ? AND cs.type = 'quiz' AND aa.attempt_no > 1 AND aa.score IS NOT NULL
		ORDER BY aa.attempt_no DESC, aa.id DESC
		LIMIT 1
	`, submissionID).Scan(&retakeScore)
	if err != nil && err != sql.ErrNoRows {
		return item, err
	}
	item.RetakeScore = intFromNull(retakeScore)
	return item, nil
}

func (r *Repository) GetOrCreateClassroom(courseID, classID int) (Classroom, error) {
	_, err := r.db.Exec(`
		INSERT INTO classroom_sessions (course_id, class_id)
		VALUES (?, ?)
		ON CONFLICT(course_id, class_id) DO NOTHING
	`, courseID, classID)
	if err != nil {
		return Classroom{}, err
	}
	var item Classroom
	err = r.db.QueryRow(`
		SELECT id, course_id, class_id, stage_id, blackboard, created_at, updated_at
		FROM classroom_sessions
		WHERE course_id = ? AND class_id = ?
	`, courseID, classID).Scan(&item.ID, &item.CourseID, &item.ClassID, &item.StageID, &item.Blackboard, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r *Repository) SetClassroomStage(courseID, classID, stageID int) error {
	if _, err := r.GetOrCreateClassroom(courseID, classID); err != nil {
		return err
	}
	_, err := r.db.Exec(`
		UPDATE classroom_sessions SET stage_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE course_id = ? AND class_id = ?
	`, stageID, courseID, classID)
	return err
}

func (r *Repository) SetBlackboard(courseID, classID int, content string) error {
	if _, err := r.GetOrCreateClassroom(courseID, classID); err != nil {
		return err
	}
	_, err := r.db.Exec(`
		UPDATE classroom_sessions SET blackboard = ?, updated_at = CURRENT_TIMESTAMP
		WHERE course_id = ? AND class_id = ?
	`, content, courseID, classID)
	return err
}

func (r *Repository) BuildStatsSummary(courseID, classID int) (StatsSummary, error) {
	var stats StatsSummary
	classes, err := r.ListClasses()
	if err != nil {
		return stats, err
	}
	courses, err := r.ListCourses()
	if err != nil {
		return stats, err
	}
	stats.Classes = classes
	stats.Courses = courses
	stats.ClassCount = len(classes)
	stats.CourseCount = len(courses)

	if err := r.db.QueryRow(`SELECT COUNT(*) FROM students WHERE deleted_at IS NULL`).Scan(&stats.StudentCount); err != nil {
		return stats, err
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM questions WHERE enabled = 1`).Scan(&stats.QuestionCount); err != nil {
		return stats, err
	}

	if courseID > 0 {
		sections, err := r.ListSections(courseID)
		if err != nil {
			return stats, err
		}
		stats.Sections = sections
		stats.SectionCount = len(sections)
		var questionCount int
		if err := r.db.QueryRow(`SELECT COUNT(*) FROM questions WHERE course_id = ? AND enabled = 1`, courseID).Scan(&questionCount); err != nil {
			return stats, err
		}
		stats.QuestionCount = questionCount
	} else if err := r.db.QueryRow(`SELECT COUNT(*) FROM course_sections`).Scan(&stats.SectionCount); err != nil {
		return stats, err
	}

	if classID > 0 {
		var studentCount int
		if err := r.db.QueryRow(`SELECT COUNT(*) FROM students WHERE class_id = ? AND deleted_at IS NULL`, classID).Scan(&studentCount); err != nil {
			return stats, err
		}
		stats.StudentCount = studentCount
	}

	if courseID > 0 && classID > 0 {
		classroom, err := r.GetOrCreateClassroom(courseID, classID)
		if err != nil {
			return stats, err
		}
		stats.Classroom = &classroom
	}
	if courseID > 0 {
		if err := r.fillCourseStats(&stats, courseID, classID); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

func (r *Repository) fillCourseStats(stats *StatsSummary, courseID, classID int) error {
	sections := stats.Sections
	if len(sections) == 0 {
		var err error
		sections, err = r.ListSections(courseID)
		if err != nil {
			return err
		}
		stats.Sections = sections
		stats.SectionCount = len(sections)
	}

	studentQuery := `
		SELECT c.id, c.name, s.id, s.name
		FROM students s
		JOIN classes c ON c.id = s.class_id AND c.deleted_at IS NULL
		WHERE s.deleted_at IS NULL
	`
	args := []interface{}{}
	if classID > 0 {
		studentQuery += ` AND c.id = ?`
		args = append(args, classID)
	}
	studentQuery += ` ORDER BY c.name, s.name`
	rows, err := r.db.Query(studentQuery, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type baseStudent struct {
		classID, studentID int
		className, name    string
	}
	var students []baseStudent
	for rows.Next() {
		var item baseStudent
		if err := rows.Scan(&item.classID, &item.className, &item.studentID, &item.name); err != nil {
			return err
		}
		students = append(students, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	stats.TotalClass = len(students)

	sectionIndex := map[int]int{}
	for index, section := range sections {
		sectionIndex[section.ID] = index
		stats.Parts = append(stats.Parts, PartStats{
			Part:      index + 1,
			SectionID: section.ID,
			Title:     section.Title,
			Completed: 0,
			Total:     len(students),
		})
	}
	stats.Part1Stats = Part1Stats{
		PredictionScoreDistribution: map[string]int{},
		LearningMethodsDistribution: map[string]int{},
	}
	stats.Part2Stats = Part2Stats{
		UnderstandingDistribution: map[string]int{},
	}
	stats.Part3Stats = Part3Stats{
		ScoreDistribution: map[int]int{},
	}

	submissionQuery := `
		SELECT s.id, s.class_id, s.student_id, s.status, COALESCE(s.updated_at, ''),
		       sr.predicted_score, sr.actual_score, sr.teacher_score, COALESCE(sr.actual_score_source, 'none'), COALESCE(sr.teacher_score_note, '')
		FROM submissions s
		LEFT JOIN score_records sr ON sr.submission_id = s.id
		WHERE s.course_id = ?
	`
	subArgs := []interface{}{courseID}
	if classID > 0 {
		submissionQuery += ` AND s.class_id = ?`
		subArgs = append(subArgs, classID)
	}
	subRows, err := r.db.Query(submissionQuery, subArgs...)
	if err != nil {
		return err
	}
	defer subRows.Close()

	type submissionStats struct {
		id, classID, studentID int
		status, updatedAt      string
		predicted, quizScore   sql.NullInt64
		teacherScore           sql.NullInt64
		source, teacherNote    string
		completedSections      map[int]bool
	}
	submissions := map[int]*submissionStats{}
	for subRows.Next() {
		var item submissionStats
		item.completedSections = map[int]bool{}
		if err := subRows.Scan(&item.id, &item.classID, &item.studentID, &item.status, &item.updatedAt, &item.predicted, &item.quizScore, &item.teacherScore, &item.source, &item.teacherNote); err != nil {
			return err
		}
		submissions[item.studentID] = &item
	}
	if err := subRows.Err(); err != nil {
		return err
	}

	if len(submissions) > 0 {
		attemptRows, err := r.db.Query(`
			SELECT s.student_id, aa.section_id
			FROM answer_attempts aa
			JOIN submissions s ON s.id = aa.submission_id
			WHERE s.course_id = ?
		`, courseID)
		if err != nil {
			return err
		}
		defer attemptRows.Close()
		for attemptRows.Next() {
			var studentID, sectionID int
			if err := attemptRows.Scan(&studentID, &sectionID); err != nil {
				return err
			}
			submission := submissions[studentID]
			if submission == nil {
				continue
			}
			if _, ok := sectionIndex[sectionID]; ok {
				submission.completedSections[sectionID] = true
			}
		}
		if err := attemptRows.Err(); err != nil {
			return err
		}
		for _, submission := range submissions {
			effective, err := r.effectiveCompletedSections(submission.id, submission.completedSections, sections)
			if err != nil {
				return err
			}
			submission.completedSections = effective
		}
	}

	part2FilledStudents := map[int]bool{}
	type questionRateAccumulator struct {
		questionID   int
		questionText string
		sortOrder    int
		correctCount int
		totalCount   int
	}
	questionRates := map[int]*questionRateAccumulator{}
	answerRows, err := r.db.Query(`
		SELECT s.student_id, cs.type, q.id, q.question_key, q.title, q.type, q.sort_order, ar.answer_json, ar.is_correct
		FROM answer_attempts aa
		JOIN submissions s ON s.id = aa.submission_id
		JOIN course_sections cs ON cs.id = aa.section_id
		JOIN answer_records ar ON ar.attempt_id = aa.id
		JOIN questions q ON q.id = ar.question_id
		WHERE s.course_id = ? AND aa.attempt_no = 1
		`+func() string {
		if classID > 0 {
			return ` AND s.class_id = ?`
		}
		return ``
	}()+`
		ORDER BY s.student_id, cs.sort_order, aa.id, q.sort_order, ar.id
	`, func() []interface{} {
		args := []interface{}{courseID}
		if classID > 0 {
			args = append(args, classID)
		}
		return args
	}()...)
	if err != nil {
		return err
	}
	defer answerRows.Close()
	for answerRows.Next() {
		var studentID int
		var sectionType, questionKey, questionTitle, questionType string
		var questionID, questionSort int
		var answerRaw string
		var isCorrect int
		if err := answerRows.Scan(&studentID, &sectionType, &questionID, &questionKey, &questionTitle, &questionType, &questionSort, &answerRaw, &isCorrect); err != nil {
			return err
		}
		if sectionType == "learning" {
			part2FilledStudents[studentID] = true
			answerText := strings.TrimSpace(rawAnswerText(json.RawMessage(answerRaw)))
			if answerText != "" {
				if questionType != "open_text" && questionType != "ai_chat" {
					stats.Part2Stats.UnderstandingDistribution[answerText]++
				}
			}
		}
		if sectionType == "prediction" && questionKey != "fixed_prediction_score" {
			answerText := strings.TrimSpace(rawAnswerText(json.RawMessage(answerRaw)))
			if answerText != "" {
				switch questionType {
				case "open_text":
					stats.Part1Stats.LearningMethodsDistribution["开放题（已完成）"]++
				case "ai_chat":
					stats.Part1Stats.LearningMethodsDistribution["AI 对话题（已完成）"]++
				default:
					stats.Part1Stats.LearningMethodsDistribution[answerText]++
				}
			}
		}
		if sectionType == "quiz" {
			acc := questionRates[questionID]
			if acc == nil {
				acc = &questionRateAccumulator{questionID: questionID, questionText: questionTitle, sortOrder: questionSort}
				questionRates[questionID] = acc
			}
			acc.totalCount++
			if isCorrect == 1 {
				acc.correctCount++
			}
		}
	}
	if err := answerRows.Err(); err != nil {
		return err
	}

	scoreRows, err := r.db.Query(`
		SELECT aa.score
		FROM answer_attempts aa
		JOIN submissions s ON s.id = aa.submission_id
		JOIN course_sections cs ON cs.id = aa.section_id
		WHERE s.course_id = ? AND aa.attempt_no = 1 AND cs.type = 'quiz'
		`+func() string {
		if classID > 0 {
			return ` AND s.class_id = ?`
		}
		return ``
	}()+`
	`, func() []interface{} {
		args := []interface{}{courseID}
		if classID > 0 {
			args = append(args, classID)
		}
		return args
	}()...)
	if err != nil {
		return err
	}
	defer scoreRows.Close()
	for scoreRows.Next() {
		var score sql.NullInt64
		if err := scoreRows.Scan(&score); err != nil {
			return err
		}
		if !score.Valid {
			continue
		}
		stats.Part3Stats.ScoreDistribution[int(score.Int64)]++
	}
	if err := scoreRows.Err(); err != nil {
		return err
	}

	submittedCount := 0
	for _, student := range students {
		submission := submissions[student.studentID]
		item := StudentStats{
			ClassID:         student.classID,
			ClassName:       student.className,
			StudentID:       student.studentID,
			StudentName:     student.name,
			Status:          "not_started",
			StatusText:      "未开始",
			GuessResult:     "unknown",
			GuessResultText: "-",
		}
		if submission == nil {
			stats.NotSubmittedStudents = append(stats.NotSubmittedStudents, student.name)
			stats.Students = append(stats.Students, item)
			continue
		}
		if submission.status == "teacher_scored" && len(submission.completedSections) == 0 {
			stats.NotSubmittedStudents = append(stats.NotSubmittedStudents, student.name)
		} else {
			submittedCount++
		}
		item.SubmissionID = submission.id
		item.Status = submission.status
		item.UpdatedAt = submission.updatedAt
		item.PredictedScore, item.QuizScore, item.TeacherScore, item.ActualScore, item.ActualScoreSource, item.GuessResult, item.GuessResultText, item.TeacherScoreNote = teacherScoreRows(
			submission.predicted,
			submission.quizScore,
			submission.teacherScore,
			submission.source,
			submission.teacherNote,
		)
		if item.QuizScore == nil && submission.quizScore.Valid {
			v := int(submission.quizScore.Int64)
			item.QuizScore = &v
		}
		if item.TeacherScore == nil && submission.teacherScore.Valid {
			v := int(submission.teacherScore.Int64)
			item.TeacherScore = &v
		}
		if item.PredictedScore != nil {
			stats.Part1Stats.PredictionScoreDistribution[strconv.Itoa(*item.PredictedScore)]++
		}
		for sectionID := range submission.completedSections {
			index, ok := sectionIndex[sectionID]
			if !ok {
				continue
			}
			stats.Parts[index].Completed++
			item.CompletedParts++
		}
		item.StatusText = completionText(item.CompletedParts, len(sections), submission.status)
		switch item.GuessResult {
		case "correct":
			stats.PredictionSummary.Correct++
		case "high":
			stats.PredictionSummary.High++
		case "low":
			stats.PredictionSummary.Low++
		default:
			stats.PredictionSummary.Unknown++
		}
		stats.Students = append(stats.Students, item)
	}
	stats.SubmittedCount = submittedCount
	stats.Part2Stats.TotalCount = len(students)
	stats.Part2Stats.FilledCount = len(part2FilledStudents)
	stats.Part3Stats.QuestionCorrectRate = make([]QuestionCorrectRate, 0, len(questionRates))
	for _, acc := range questionRates {
		correctRate := 0
		if acc.totalCount > 0 {
			correctRate = int(math.Round(float64(acc.correctCount) * 100 / float64(acc.totalCount)))
		}
		stats.Part3Stats.QuestionCorrectRate = append(stats.Part3Stats.QuestionCorrectRate, QuestionCorrectRate{
			QuestionID:   acc.questionID,
			QuestionText: acc.questionText,
			SortOrder:    acc.sortOrder,
			CorrectCount: acc.correctCount,
			TotalCount:   acc.totalCount,
			CorrectRate:  correctRate,
		})
	}
	sort.Slice(stats.Part3Stats.QuestionCorrectRate, func(i, j int) bool {
		if stats.Part3Stats.QuestionCorrectRate[i].SortOrder == stats.Part3Stats.QuestionCorrectRate[j].SortOrder {
			return stats.Part3Stats.QuestionCorrectRate[i].QuestionID < stats.Part3Stats.QuestionCorrectRate[j].QuestionID
		}
		return stats.Part3Stats.QuestionCorrectRate[i].SortOrder < stats.Part3Stats.QuestionCorrectRate[j].SortOrder
	})
	return r.FillAIGuidanceStats(stats, courseID, classID)
}

func completionText(completed, total int, status string) string {
	if total > 0 && completed >= total {
		return "已完成全部"
	}
	if completed > 0 {
		return fmt.Sprintf("完成到第%d部分", completed)
	}
	if status == "completed" {
		return "已完成"
	}
	return "未开始"
}

func guessResultText(value string) string {
	switch value {
	case "correct":
		return "猜中"
	case "high":
		return "猜高"
	case "low":
		return "猜低"
	default:
		return "-"
	}
}

func (r *Repository) ListStudentHistory(classID, studentID int) ([]StudentHistoryRecord, error) {
	rows, err := r.db.Query(`
		SELECT c.id, c.title, c.mode, cl.id, cl.name, st.id, st.name,
		       s.id, s.status, s.started_at, COALESCE(s.completed_at, ''), COALESCE(s.updated_at, ''),
		       sr.predicted_score, sr.actual_score, sr.teacher_score, COALESCE(sr.actual_score_source, 'none'), COALESCE(sr.teacher_score_note, '')
		FROM submissions s
		JOIN courses c ON c.id = s.course_id AND c.deleted_at IS NULL
		JOIN classes cl ON cl.id = s.class_id AND cl.deleted_at IS NULL
		JOIN students st ON st.id = s.student_id AND st.deleted_at IS NULL
		LEFT JOIN score_records sr ON sr.submission_id = s.id
		WHERE s.class_id = ? AND s.student_id = ?
		ORDER BY COALESCE(s.completed_at, s.updated_at, s.started_at) DESC, s.id DESC
	`, classID, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []StudentHistoryRecord{}
	for rows.Next() {
		var item StudentHistoryRecord
		var predicted, quizScore, teacherScore sql.NullInt64
		var actualSource, teacherNote string
		if err := rows.Scan(
			&item.CourseID, &item.CourseTitle, &item.Mode, &item.ClassID, &item.ClassName, &item.StudentID, &item.StudentName,
			&item.SubmissionID, &item.Status, &item.StartedAt, &item.CompletedAt, &item.UpdatedAt,
			&predicted, &quizScore, &teacherScore, &actualSource, &teacherNote,
		); err != nil {
			return nil, err
		}
		item.PredictedScore, item.QuizScore, item.TeacherScore, item.ActualScore, item.ActualScoreSource, item.GuessResult, item.GuessResultText, item.TeacherScoreNote = teacherScoreRows(
			predicted,
			quizScore,
			teacherScore,
			actualSource,
			teacherNote,
		)
		sections, err := r.ListSections(item.CourseID)
		if err != nil {
			return nil, err
		}
		completed := map[int]bool{}
		attemptRows, err := r.db.Query(`SELECT DISTINCT section_id FROM answer_attempts WHERE submission_id = ?`, item.SubmissionID)
		if err != nil {
			return nil, err
		}
		for attemptRows.Next() {
			var sectionID int
			if err := attemptRows.Scan(&sectionID); err != nil {
				attemptRows.Close()
				return nil, err
			}
			completed[sectionID] = true
		}
		if err := attemptRows.Close(); err != nil {
			return nil, err
		}
		completed, err = r.effectiveCompletedSections(item.SubmissionID, completed, sections)
		if err != nil {
			return nil, err
		}
		item.CompletedParts = len(completed)
		item.AIGuidance, err = r.ListAIGuidanceForSubmission(item.SubmissionID)
		if err != nil {
			return nil, err
		}
		item.StatusText = completionText(item.CompletedParts, 0, item.Status)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) GetStudentDetail(submissionID int) (StudentDetail, error) {
	var item StudentDetail
	if submissionID <= 0 {
		return item, errors.New("提交ID无效")
	}

	var predicted, quizScore, teacherScore sql.NullInt64
	var actualSource, teacherNote string
	var status, startedAt, completedAt, updatedAt string
	err := r.db.QueryRow(`
		SELECT s.course_id, c.title, c.mode, s.class_id, cl.name, s.student_id, st.name,
		       s.status, s.started_at, COALESCE(s.completed_at, ''), COALESCE(s.updated_at, ''),
		       sr.predicted_score, sr.actual_score, sr.teacher_score, COALESCE(sr.actual_score_source, 'none'), COALESCE(sr.teacher_score_note, '')
		FROM submissions s
		JOIN courses c ON c.id = s.course_id AND c.deleted_at IS NULL
		JOIN classes cl ON cl.id = s.class_id AND cl.deleted_at IS NULL
		JOIN students st ON st.id = s.student_id AND st.deleted_at IS NULL
		LEFT JOIN score_records sr ON sr.submission_id = s.id
		WHERE s.id = ?
	`, submissionID).Scan(
		&item.CourseID, &item.CourseTitle, &item.Mode, &item.ClassID, &item.ClassName, &item.StudentID, &item.StudentName,
		&status, &startedAt, &completedAt, &updatedAt,
		&predicted, &quizScore, &teacherScore, &actualSource, &teacherNote,
	)
	if err != nil {
		return item, err
	}
	item.Status = status
	item.StartedAt = startedAt
	item.CompletedAt = completedAt
	item.UpdatedAt = updatedAt
	item.PredictedScore, item.QuizScore, item.TeacherScore, item.ActualScore, item.ActualScoreSource, item.GuessResult, item.GuessResultText, item.TeacherScoreNote = teacherScoreRows(
		predicted,
		quizScore,
		teacherScore,
		actualSource,
		teacherNote,
	)

	sections, err := r.ListSections(item.CourseID)
	if err != nil {
		return item, err
	}
	item.Sections = make([]StudentDetailSection, 0, len(sections))
	sectionIndexByID := map[int]int{}
	for _, section := range sections {
		item.Sections = append(item.Sections, StudentDetailSection{
			SectionID: section.ID,
			Title:     section.Title,
			Type:      section.Type,
			SortOrder: section.SortOrder,
			Fixed:     section.Fixed,
			Attempts:  []StudentDetailAttempt{},
		})
		sectionIndexByID[section.ID] = len(item.Sections) - 1
	}

	attemptRows, err := r.db.Query(`
		SELECT aa.id, aa.section_id, aa.attempt_no, aa.score, COALESCE(aa.submitted_at, '')
		FROM answer_attempts aa
		WHERE aa.submission_id = ?
		ORDER BY aa.section_id, aa.attempt_no, aa.id
	`, submissionID)
	if err != nil {
		return item, err
	}
	defer attemptRows.Close()
	type attemptInfo struct {
		sectionIndex int
		attemptIndex int
	}
	attemptByID := map[int]*attemptInfo{}
	completedSections := map[int]bool{}
	for attemptRows.Next() {
		var attemptID, sectionID, attemptNo int
		var score sql.NullInt64
		var submittedAt string
		if err := attemptRows.Scan(&attemptID, &sectionID, &attemptNo, &score, &submittedAt); err != nil {
			return item, err
		}
		sectionIndex, ok := sectionIndexByID[sectionID]
		if !ok {
			continue
		}
		attemptItem := StudentDetailAttempt{
			AttemptNo:   attemptNo,
			SubmittedAt: submittedAt,
			Questions:   []StudentDetailQuestion{},
		}
		if score.Valid {
			v := int(score.Int64)
			attemptItem.Score = &v
		}
		item.Sections[sectionIndex].Attempts = append(item.Sections[sectionIndex].Attempts, attemptItem)
		item.Sections[sectionIndex].Completed = true
		attemptIndex := len(item.Sections[sectionIndex].Attempts) - 1
		attemptByID[attemptID] = &attemptInfo{
			sectionIndex: sectionIndex,
			attemptIndex: attemptIndex,
		}
		completedSections[sectionID] = true
	}
	if err := attemptRows.Err(); err != nil {
		return item, err
	}

	answerRows, err := r.db.Query(`
		SELECT ar.attempt_id, q.id, q.title, q.type, q.sort_order,
		       ar.answer_json, q.correct_answer_json, q.explanation, q.score, ar.is_correct,
		       q.rules_json
		FROM answer_records ar
		JOIN questions q ON q.id = ar.question_id
		JOIN answer_attempts aa ON aa.id = ar.attempt_id
		WHERE aa.submission_id = ?
		ORDER BY aa.section_id, aa.attempt_no, aa.id, q.sort_order, ar.id
	`, submissionID)
	if err != nil {
		return item, err
	}
	defer answerRows.Close()
	for answerRows.Next() {
		var attemptID, questionID, questionSort, questionScore int
		var questionTitle, questionType, answerRaw, correctRaw, explanation, questionRulesRaw string
		var isCorrect int
		if err := answerRows.Scan(&attemptID, &questionID, &questionTitle, &questionType, &questionSort, &answerRaw, &correctRaw, &explanation, &questionScore, &isCorrect, &questionRulesRaw); err != nil {
			return item, err
		}
		target := attemptByID[attemptID]
		if target == nil {
			continue
		}
		attempt := &item.Sections[target.sectionIndex].Attempts[target.attemptIndex]
		answerJSON := json.RawMessage(answerRaw)
		answerText := strings.TrimSpace(rawAnswerText(answerJSON))
		if questionType == "open_text" {
			answerText = openTextDisplayAnswer(answerText)
		}
		attempt.Questions = append(attempt.Questions, StudentDetailQuestion{
			QuestionID:   questionID,
			QuestionText: questionTitle,
			QuestionType: questionType,
			SortOrder:    questionSort,
			Answer:       answerText,
			CorrectAnswer: func() string {
				text := strings.TrimSpace(rawAnswerText(json.RawMessage(correctRaw)))
				if text == "" || text == "null" {
					return "-"
				}
				return text
			}(),
			Explanation: explanation,
			Score:       questionScore,
			IsCorrect:   isCorrect == 1,
			ChatMessages: func() []AIChatMessage {
				if questionType != "ai_chat" {
					return nil
				}
				return aiChatMessagesFromRaw(answerJSON)
			}(),
			Rules: json.RawMessage(questionRulesRaw),
		})
	}
	if err := answerRows.Err(); err != nil {
		return item, err
	}

	completedSections, err = r.effectiveCompletedSections(submissionID, completedSections, sections)
	if err != nil {
		return item, err
	}
	for index := range item.Sections {
		item.Sections[index].Completed = completedSections[item.Sections[index].SectionID]
	}
	item.AIGuidance, err = r.ListAIGuidanceForSubmission(submissionID)
	if err != nil {
		return item, err
	}
	item.CompletedParts = len(completedSections)
	item.StatusText = completionText(item.CompletedParts, len(item.Sections), item.Status)
	return item, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
