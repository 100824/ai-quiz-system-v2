package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"ai-quiz-system-v2/backend-go/internal/models"
	"ai-quiz-system-v2/backend-go/internal/utils"
)

// Repository encapsulates all database operations.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new Repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// ---------------------------------------------------------------------------
// Courses
// ---------------------------------------------------------------------------

// GetAllCourses returns every course ordered by lesson_number.
func (r *Repository) GetAllCourses() ([]models.Course, error) {
	rows, err := r.db.Query(`SELECT id, name, lesson_number, description, part2_guide, created_at, updated_at FROM courses ORDER BY lesson_number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []models.Course
	for rows.Next() {
		var c models.Course
		var description, guide sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.LessonNo, &description, &guide, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if description.Valid {
			c.Description = utils.Ptr(description.String)
		}
		if guide.Valid {
			c.Part2Guide = utils.Ptr(guide.String)
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

// GetCourseByID fetches a single course.
func (r *Repository) GetCourseByID(id int) (models.Course, error) {
	var c models.Course
	var description, guide sql.NullString
	err := r.db.QueryRow(`SELECT id, name, lesson_number, description, part2_guide, created_at, updated_at FROM courses WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.LessonNo, &description, &guide, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return c, err
	}
	if description.Valid {
		c.Description = utils.Ptr(description.String)
	}
	if guide.Valid {
		c.Part2Guide = utils.Ptr(guide.String)
	}
	return c, nil
}

// GetCurrentCourseID returns the active course from system_config.
func (r *Repository) GetCurrentCourseID() (int, error) {
	var value string
	err := r.db.QueryRow(`SELECT value FROM system_config WHERE key = 'current_course_id'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	id, err := strconv.Atoi(value)
	if err != nil {
		return 1, nil
	}
	return id, nil
}

// SetCurrentCourseID persists the active course.
func (r *Repository) SetCurrentCourseID(id int) error {
	_, err := r.db.Exec(`
		INSERT INTO system_config (key, value, updated_at)
		VALUES ('current_course_id', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, strconv.Itoa(id))
	return err
}

// ---------------------------------------------------------------------------
// Classes
// ---------------------------------------------------------------------------

func (r *Repository) queryDistinctNames(query string, args ...interface{}) ([]string, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func uniqueSortedStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

// GetClassRosterNames returns the student roster for a class.
func (r *Repository) GetClassRosterNames(courseID int, className string) ([]string, error) {
	className = strings.TrimSpace(className)
	if className == "" {
		return nil, nil
	}

	sources := []struct {
		query string
		args  []interface{}
	}{
		{
			query: `SELECT DISTINCT student_name FROM class_lists WHERE class_name = ? ORDER BY student_name`,
			args:  []interface{}{className},
		},
		{
			query: `SELECT DISTINCT student_name FROM student_surveys WHERE course_id = ? AND class_name = ? ORDER BY student_name`,
			args:  []interface{}{courseID, className},
		},
		{
			query: `SELECT DISTINCT student_name FROM student_surveys WHERE class_name = ? ORDER BY student_name`,
			args:  []interface{}{className},
		},
	}

	for _, source := range sources {
		names, err := r.queryDistinctNames(source.query, source.args...)
		if err != nil {
			return nil, err
		}
		names = uniqueSortedStrings(names)
		if len(names) > 0 {
			return names, nil
		}
	}

	return nil, nil
}

// GetClassList returns the roster formatted for API responses.
func (r *Repository) GetClassList(courseID int, className string) ([]map[string]string, error) {
	names, err := r.GetClassRosterNames(courseID, className)
	if err != nil {
		return nil, err
	}

	var students []map[string]string
	for _, name := range names {
		students = append(students, map[string]string{"student_name": name})
	}
	return students, nil
}

// ImportClassList replaces the roster for a class.
func (r *Repository) ImportClassList(courseID int, className string, studentNames []string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	className = strings.TrimSpace(className)
	if className == "" {
		return errors.New("班级名称不能为空")
	}
	if courseID <= 0 {
		courseID = 1
	}
	if _, err := tx.Exec(`DELETE FROM class_lists WHERE class_name = ?`, className); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO class_lists (course_id, class_name, student_name) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	seen := map[string]struct{}{}
	for _, rawName := range studentNames {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if _, err := stmt.Exec(courseID, className, name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ValidateStudent checks whether a student is in the class roster.
func (r *Repository) ValidateStudent(courseID int, className, studentName string) (bool, error) {
	studentName = strings.TrimSpace(studentName)
	if studentName == "" {
		return false, nil
	}

	names, err := r.GetClassRosterNames(courseID, className)
	if err != nil {
		return false, err
	}
	for _, name := range names {
		if name == studentName {
			return true, nil
		}
	}
	return false, nil
}

// GetAllClasses returns every class name associated with a course.
func (r *Repository) GetAllClasses(courseID int) ([]map[string]string, error) {
	classLists, err := r.queryDistinctNames(`SELECT DISTINCT class_name FROM class_lists ORDER BY class_name`)
	if err != nil {
		return nil, err
	}
	surveyClasses, err := r.queryDistinctNames(`SELECT DISTINCT class_name FROM student_surveys WHERE course_id = ? ORDER BY class_name`, courseID)
	if err != nil {
		return nil, err
	}
	boundClasses, err := r.queryDistinctNames(`SELECT DISTINCT class_name FROM class_course_bind WHERE course_id = ? ORDER BY class_name`, courseID)
	if err != nil {
		return nil, err
	}

	allNames := uniqueSortedStrings(append(append(classLists, surveyClasses...), boundClasses...))
	var classes []map[string]string
	for _, name := range allNames {
		classes = append(classes, map[string]string{"class_name": name})
	}
	return classes, nil
}

// GetClassBoundCourse returns the course bound to a class name.
func (r *Repository) GetClassBoundCourse(className string) (int, error) {
	var courseID int
	err := r.db.QueryRow(`SELECT course_id FROM class_course_bind WHERE class_name = ?`, className).Scan(&courseID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return courseID, err
}

// SetClassBoundCourse binds a class to a course.
func (r *Repository) SetClassBoundCourse(className string, courseID int) error {
	_, err := r.db.Exec(`
		INSERT INTO class_course_bind (class_name, course_id, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(class_name) DO UPDATE SET course_id = excluded.course_id, updated_at = CURRENT_TIMESTAMP
	`, className, courseID)
	return err
}

// GetClassTip returns the persisted classroom tip for a class.
func (r *Repository) GetClassTip(className string) (models.TipPayload, error) {
	var tip models.TipPayload
	className = strings.TrimSpace(className)
	if className == "" {
		return tip, nil
	}

	err := r.db.QueryRow(`
		SELECT class_name, content, updated_at
		FROM class_tips
		WHERE class_name = ?
	`, className).Scan(&tip.ClassName, &tip.Content, &tip.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.TipPayload{ClassName: className}, nil
	}
	return tip, err
}

// SetClassTip upserts the persisted classroom tip for a class.
func (r *Repository) SetClassTip(className, content string) error {
	className = strings.TrimSpace(className)
	if className == "" {
		return errors.New("班级名称不能为空")
	}
	_, err := r.db.Exec(`
		INSERT INTO class_tips (class_name, content, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(class_name) DO UPDATE SET content = excluded.content, updated_at = CURRENT_TIMESTAMP
	`, className, content)
	return err
}

// ---------------------------------------------------------------------------
// Stages
// ---------------------------------------------------------------------------

// GetCurrentStage returns the active stage for a course/class.
func (r *Repository) GetCurrentStage(courseID int, className string) (int, error) {
	if strings.TrimSpace(className) != "" {
		var stage int
		err := r.db.QueryRow(`SELECT current_stage FROM course_stages WHERE course_id = ? AND class_name = ?`, courseID, className).Scan(&stage)
		if err == nil {
			return stage, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		err = r.db.QueryRow(`SELECT current_stage FROM course_stages WHERE course_id = ? AND class_name = 'common'`, courseID).Scan(&stage)
		if err == nil {
			return stage, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		return 0, nil
	}
	var stage sql.NullInt64
	err := r.db.QueryRow(`SELECT MAX(current_stage) FROM course_stages WHERE course_id = ?`, courseID).Scan(&stage)
	if err != nil {
		return 0, err
	}
	if !stage.Valid {
		return 0, nil
	}
	return int(stage.Int64), nil
}

// SetCurrentStage sets the active stage for a course/class.
func (r *Repository) SetCurrentStage(courseID, stage int, className string) error {
	target := strings.TrimSpace(className)
	if target == "" {
		target = "common"
	}
	_, err := r.db.Exec(`
		INSERT INTO course_stages (course_id, class_name, current_stage, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(course_id, class_name) DO UPDATE SET current_stage = excluded.current_stage, updated_at = CURRENT_TIMESTAMP
	`, courseID, target, stage)
	return err
}

// GetPartSettings returns the enabled state for each of the 4 parts.
func (r *Repository) GetPartSettings(courseID int) ([]models.PartSetting, error) {
	rows, err := r.db.Query(`
		SELECT part, enabled
		FROM course_part_settings
		WHERE course_id = ?
		ORDER BY part
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := make([]models.PartSetting, 0, 4)
	for rows.Next() {
		var part int
		var enabledInt int
		if err := rows.Scan(&part, &enabledInt); err != nil {
			return nil, err
		}
		settings = append(settings, models.PartSetting{
			Part:    part,
			Enabled: enabledInt == 1,
		})
	}
	if len(settings) == 0 {
		for part := 1; part <= 4; part++ {
			settings = append(settings, models.PartSetting{Part: part, Enabled: true})
		}
	}
	return settings, rows.Err()
}

// IsPartEnabled checks whether a specific part is enabled.
func (r *Repository) IsPartEnabled(courseID, part int) (bool, error) {
	var enabledInt int
	err := r.db.QueryRow(`SELECT enabled FROM course_part_settings WHERE course_id = ? AND part = ?`, courseID, part).Scan(&enabledInt)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return enabledInt == 1, nil
}

// UpdatePartSettings persists part enable/disable flags.
func (r *Repository) UpdatePartSettings(courseID int, settings []models.PartSetting) error {
	enabledCount := 0
	for _, setting := range settings {
		if setting.Enabled {
			enabledCount++
		}
	}
	if enabledCount == 0 {
		return errors.New("至少需要开启一个部分")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, setting := range settings {
		if setting.Part < 1 || setting.Part > 4 {
			return fmt.Errorf("第%d部分无效", setting.Part)
		}
		enabledInt := 0
		if setting.Enabled {
			enabledInt = 1
		}
		if _, err := tx.Exec(`
			INSERT INTO course_part_settings (course_id, part, enabled, updated_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(course_id, part) DO UPDATE SET enabled = excluded.enabled, updated_at = CURRENT_TIMESTAMP
		`, courseID, setting.Part, enabledInt); err != nil {
			return err
		}
	}

	var currentStage sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(current_stage) FROM course_stages WHERE course_id = ?`, courseID).Scan(&currentStage); err != nil {
		return err
	}
	if currentStage.Valid {
		enabledMap := make(map[int]bool, len(settings))
		for _, setting := range settings {
			enabledMap[setting.Part] = setting.Enabled
		}
		if currentStage.Int64 > 0 && !enabledMap[int(currentStage.Int64)] {
			nextStage := utils.FirstEnabledPart(enabledMap)
			if nextStage == 0 {
				nextStage = 1
			}
			if _, err := tx.Exec(`UPDATE course_stages SET current_stage = ?, updated_at = CURRENT_TIMESTAMP WHERE course_id = ?`, nextStage, courseID); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Questions
// ---------------------------------------------------------------------------

// GetQuestionsByPart returns all questions for a course part.
func (r *Repository) GetQuestionsByPart(courseID, part int) ([]models.Question, error) {
	rows, err := r.db.Query(`
		SELECT id, course_id, part, question_type, question_text, options, correct_answer, explanation, enabled, annotation_enabled, sort_order, created_at, updated_at
		FROM questions
		WHERE course_id = ? AND part = ?
		ORDER BY sort_order, id
	`, courseID, part)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var questions []models.Question
	for rows.Next() {
		var q models.Question
		var enabledInt, annotationEnabledInt int
		var options, correctAnswer, explanation sql.NullString
		if err := rows.Scan(&q.ID, &q.CourseID, &q.Part, &q.QuestionType, &q.QuestionText, &options, &correctAnswer, &explanation, &enabledInt, &annotationEnabledInt, &q.SortOrder, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, err
		}
		if options.Valid {
			q.Options = utils.Ptr(options.String)
		}
		if correctAnswer.Valid {
			q.CorrectAnswer = utils.Ptr(correctAnswer.String)
		}
		if explanation.Valid {
			q.Explanation = utils.Ptr(explanation.String)
		}
		q.Enabled = enabledInt == 1
		q.AnnotationEnabled = annotationEnabledInt == 1
		questions = append(questions, q)
	}
	return questions, rows.Err()
}

// FilterEnabledQuestions returns only enabled questions.
func FilterEnabledQuestions(items []models.Question) []models.Question {
	filtered := make([]models.Question, 0, len(items))
	for _, q := range items {
		if q.Enabled {
			filtered = append(filtered, q)
		}
	}
	return filtered
}

// UpdateQuestion modifies question content.
func (r *Repository) UpdateQuestion(id int, questionType, questionText string, options, correctAnswer, explanation *string, annotationEnabled bool) error {
	annotationEnabledInt := 0
	if annotationEnabled {
		annotationEnabledInt = 1
	}
	_, err := r.db.Exec(`
		UPDATE questions
		SET question_type = ?, question_text = ?, options = ?, correct_answer = ?, explanation = ?, annotation_enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, questionType, questionText, utils.EmptyToNil(options), utils.EmptyToNil(correctAnswer), utils.EmptyToNil(explanation), annotationEnabledInt, id)
	return err
}

// SetQuestionEnabled toggles a question's enabled flag.
func (r *Repository) SetQuestionEnabled(id int, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err := r.db.Exec(`UPDATE questions SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, enabledInt, id)
	return err
}

// GetQuestionAnnotationEnabled returns the annotation_enabled flag for a question.
func (r *Repository) GetQuestionAnnotationEnabled(id int) (bool, error) {
	var enabled int
	err := r.db.QueryRow(`SELECT annotation_enabled FROM questions WHERE id = ?`, id).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	return enabled == 1, err
}

// GetPart2Guide returns the part-2 guide text for a course.
func (r *Repository) GetPart2Guide(courseID int) (string, error) {
	var guide sql.NullString
	err := r.db.QueryRow(`SELECT part2_guide FROM courses WHERE id = ?`, courseID).Scan(&guide)
	if err != nil {
		return "", err
	}
	if !guide.Valid {
		return "", nil
	}
	return guide.String, nil
}

// UpdatePart2Guide updates the part-2 guide text.
func (r *Repository) UpdatePart2Guide(courseID int, content string) error {
	_, err := r.db.Exec(`UPDATE courses SET part2_guide = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, content, courseID)
	return err
}

// ---------------------------------------------------------------------------
// Surveys
// ---------------------------------------------------------------------------

// SaveStudentPart1 persists part-1 answers.
func (r *Repository) SaveStudentPart1(courseID int, studentID, studentName, className string, answers json.RawMessage) error {
	_, err := r.db.Exec(`
		INSERT INTO student_surveys (course_id, student_id, student_name, class_name, part1_answers, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(course_id, student_id) DO UPDATE SET
			student_name = excluded.student_name,
			class_name = excluded.class_name,
			part1_answers = excluded.part1_answers,
			updated_at = CURRENT_TIMESTAMP
	`, courseID, studentID, studentName, className, string(answers))
	return err
}

// EnsureStudentSurveyRecord creates a skeleton row if one doesn't exist.
func (r *Repository) EnsureStudentSurveyRecord(courseID int, studentID string) error {
	className, studentName := utils.SplitStudentID(studentID)
	_, err := r.db.Exec(`
		INSERT INTO student_surveys (course_id, student_id, student_name, class_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(course_id, student_id) DO UPDATE SET
			student_name = CASE
				WHEN excluded.student_name <> '' THEN excluded.student_name
				ELSE student_surveys.student_name
			END,
			class_name = CASE
				WHEN excluded.class_name <> '' THEN excluded.class_name
				ELSE student_surveys.class_name
			END,
			updated_at = CURRENT_TIMESTAMP
	`, courseID, studentID, studentName, className)
	return err
}

// SaveStudentPart2 persists part-2 answers.
func (r *Repository) SaveStudentPart2(courseID int, studentID, answer string) error {
	if err := r.EnsureStudentSurveyRecord(courseID, studentID); err != nil {
		return err
	}
	_, err := r.db.Exec(`UPDATE student_surveys SET part2_answer = ?, updated_at = CURRENT_TIMESTAMP WHERE course_id = ? AND student_id = ?`, answer, courseID, studentID)
	return err
}

// SaveStudentPart3 persists part-3 answers and score.
func (r *Repository) SaveStudentPart3(courseID int, studentID string, answers json.RawMessage, score int) error {
	if err := r.EnsureStudentSurveyRecord(courseID, studentID); err != nil {
		return err
	}
	_, err := r.db.Exec(`UPDATE student_surveys SET part3_answers = ?, part3_score = ?, updated_at = CURRENT_TIMESTAMP WHERE course_id = ? AND student_id = ?`, string(answers), score, courseID, studentID)
	return err
}

// SaveStudentPart4 persists part-4 answers and marks submission complete.
func (r *Repository) SaveStudentPart4(courseID int, studentID string, answers json.RawMessage) error {
	if err := r.EnsureStudentSurveyRecord(courseID, studentID); err != nil {
		return err
	}
	_, err := r.db.Exec(`UPDATE student_surveys SET part4_answers = ?, submitted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE course_id = ? AND student_id = ?`, string(answers), courseID, studentID)
	return err
}

// SaveTeacherScore persists a manual score without touching the part-3 quiz score.
func (r *Repository) SaveTeacherScore(courseID int, studentID string, score *int, note string) error {
	if err := r.EnsureStudentSurveyRecord(courseID, studentID); err != nil {
		return err
	}
	if score == nil {
		_, err := r.db.Exec(`
			UPDATE student_surveys
			SET teacher_score = NULL, teacher_score_note = NULL, teacher_score_updated_at = NULL, updated_at = CURRENT_TIMESTAMP
			WHERE course_id = ? AND student_id = ?
		`, courseID, studentID)
		return err
	}
	_, err := r.db.Exec(`
		UPDATE student_surveys
		SET teacher_score = ?, teacher_score_note = ?, teacher_score_updated_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE course_id = ? AND student_id = ?
	`, *score, utils.EmptyToNil(&note), courseID, studentID)
	return err
}

func setStudentActualScore(s *models.StudentSurvey) {
	s.ActualScore = nil
	s.ActualScoreSource = "none"
	if s.TeacherScore != nil {
		s.ActualScore = s.TeacherScore
		s.ActualScoreSource = "teacher"
		return
	}
	if s.Part3Score != nil {
		s.ActualScore = s.Part3Score
		s.ActualScoreSource = "part3"
	}
}

func setHistoryActualScore(item *models.StudentHistoryDetail) {
	item.ActualScore = nil
	item.ActualScoreSource = "none"
	if item.TeacherScore != nil {
		item.ActualScore = item.TeacherScore
		item.ActualScoreSource = "teacher"
		return
	}
	if item.Part3Score != nil {
		item.ActualScore = item.Part3Score
		item.ActualScoreSource = "part3"
	}
}

// GetStudentSurvey returns a single student's survey record.
func (r *Repository) GetStudentSurvey(courseID int, studentID string) (*models.StudentSurvey, error) {
	var s models.StudentSurvey
	var part1, part3, part4 sql.NullString
	var part2, submittedAt, teacherNote, teacherScoreUpdatedAt sql.NullString
	var part3Score, teacherScore sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, course_id, student_id, student_name, class_name, part1_answers, part2_answer, part3_answers, part3_score,
		       teacher_score, teacher_score_note, teacher_score_updated_at, part4_answers, submitted_at, created_at, updated_at
		FROM student_surveys WHERE course_id = ? AND student_id = ?
	`, courseID, studentID).Scan(&s.ID, &s.CourseID, &s.StudentID, &s.StudentName, &s.ClassName, &part1, &part2, &part3, &part3Score, &teacherScore, &teacherNote, &teacherScoreUpdatedAt, &part4, &submittedAt, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if part1.Valid {
		s.Part1 = json.RawMessage(part1.String)
	}
	if part2.Valid {
		s.Part2 = utils.Ptr(part2.String)
	}
	if part3.Valid {
		s.Part3 = json.RawMessage(part3.String)
	}
	if part3Score.Valid {
		val := int(part3Score.Int64)
		s.Part3Score = &val
	}
	if teacherScore.Valid {
		val := int(teacherScore.Int64)
		s.TeacherScore = &val
	}
	if teacherNote.Valid {
		s.TeacherNote = utils.Ptr(teacherNote.String)
	}
	if teacherScoreUpdatedAt.Valid {
		s.TeacherScoreUpdatedAt = utils.Ptr(teacherScoreUpdatedAt.String)
	}
	if part4.Valid {
		s.Part4 = json.RawMessage(part4.String)
	}
	if submittedAt.Valid {
		s.SubmittedAt = utils.Ptr(submittedAt.String)
	}
	setStudentActualScore(&s)
	return &s, nil
}

// GetAllStudentSurveys returns every survey for a course, optionally filtered by class.
func (r *Repository) GetAllStudentSurveys(courseID int, className string) ([]models.StudentSurvey, error) {
	query := `
		SELECT id, course_id, student_id, student_name, class_name, part1_answers, part2_answer, part3_answers, part3_score,
		       teacher_score, teacher_score_note, teacher_score_updated_at, part4_answers, submitted_at, created_at, updated_at
		FROM student_surveys WHERE course_id = ?
	`
	args := []interface{}{courseID}
	if strings.TrimSpace(className) != "" {
		query += ` AND class_name = ?`
		args = append(args, className)
	}
	query += ` ORDER BY class_name, student_name`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []models.StudentSurvey
	for rows.Next() {
		var s models.StudentSurvey
		var part1, part2, part3, part4, submittedAt, teacherNote, teacherScoreUpdatedAt sql.NullString
		var part3Score, teacherScore sql.NullInt64
		if err := rows.Scan(&s.ID, &s.CourseID, &s.StudentID, &s.StudentName, &s.ClassName, &part1, &part2, &part3, &part3Score, &teacherScore, &teacherNote, &teacherScoreUpdatedAt, &part4, &submittedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		if part1.Valid {
			s.Part1 = json.RawMessage(part1.String)
		}
		if part2.Valid {
			s.Part2 = utils.Ptr(part2.String)
		}
		if part3.Valid {
			s.Part3 = json.RawMessage(part3.String)
		}
		if part3Score.Valid {
			score := int(part3Score.Int64)
			s.Part3Score = &score
		}
		if teacherScore.Valid {
			score := int(teacherScore.Int64)
			s.TeacherScore = &score
		}
		if teacherNote.Valid {
			s.TeacherNote = utils.Ptr(teacherNote.String)
		}
		if teacherScoreUpdatedAt.Valid {
			s.TeacherScoreUpdatedAt = utils.Ptr(teacherScoreUpdatedAt.String)
		}
		if part4.Valid {
			s.Part4 = json.RawMessage(part4.String)
		}
		if submittedAt.Valid {
			s.SubmittedAt = utils.Ptr(submittedAt.String)
		}
		setStudentActualScore(&s)
		items = append(items, s)
	}
	return items, rows.Err()
}

// GetCompletionStats calculates total/completed counts for a part.
func (r *Repository) GetCompletionStats(courseID, part int, className string) (models.CompletionStats, error) {
	column := fmt.Sprintf("part%d_answers", part)
	if part == 2 {
		column = "part2_answer"
	}

	total := 0
	if strings.TrimSpace(className) != "" {
		names, err := r.GetClassRosterNames(courseID, className)
		if err != nil {
			return models.CompletionStats{}, err
		}
		total = len(names)
	} else {
		if err := r.db.QueryRow(`
			SELECT COUNT(*) FROM (
				SELECT DISTINCT class_name || '|' || student_name AS roster_key
				FROM class_lists
				UNION
				SELECT DISTINCT class_name || '|' || student_name AS roster_key
				FROM student_surveys
				WHERE course_id = ?
			)
		`, courseID).Scan(&total); err != nil {
			return models.CompletionStats{}, err
		}
	}

	query := fmt.Sprintf(`SELECT SUM(CASE WHEN %s IS NOT NULL THEN 1 ELSE 0 END) FROM student_surveys WHERE course_id = ?`, column)
	args := []interface{}{courseID}
	if strings.TrimSpace(className) != "" {
		query += ` AND class_name = ?`
		args = append(args, className)
	}
	var completed sql.NullInt64
	if err := r.db.QueryRow(query, args...).Scan(&completed); err != nil {
		return models.CompletionStats{}, err
	}
	stats := models.CompletionStats{Total: total}
	if completed.Valid {
		stats.Completed = int(completed.Int64)
	}
	return stats, nil
}

// GetStudentHistoryScores returns the score history for a student.
func (r *Repository) GetStudentHistoryScores(studentID string) ([]models.HistoryScore, error) {
	rows, err := r.db.Query(`
		SELECT c.name, c.lesson_number, s.part3_score, s.teacher_score, s.teacher_score_note, s.part1_answers
		FROM student_surveys s
		JOIN courses c ON s.course_id = c.id
		WHERE s.student_id = ? AND (s.teacher_score IS NOT NULL OR s.part3_score IS NOT NULL)
		ORDER BY c.lesson_number ASC
	`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []models.HistoryScore
	for rows.Next() {
		var name string
		var lessonNo int
		var part3Score, teacherScore sql.NullInt64
		var teacherNote, rawPart1 sql.NullString
		if err := rows.Scan(&name, &lessonNo, &part3Score, &teacherScore, &teacherNote, &rawPart1); err != nil {
			return nil, err
		}
		item := models.HistoryScore{CourseName: name, LessonNumber: lessonNo, ActualScoreSource: "none"}
		if teacherScore.Valid {
			score := int(teacherScore.Int64)
			item.ActualScore = &score
			item.ActualScoreSource = "teacher"
		} else if part3Score.Valid {
			score := int(part3Score.Int64)
			item.ActualScore = &score
			item.ActualScoreSource = "part3"
		}
		if teacherNote.Valid {
			item.TeacherScoreNote = utils.Ptr(teacherNote.String)
		}
		if rawPart1.Valid {
			var answers models.Part1Answers
			if err := json.Unmarshal([]byte(rawPart1.String), &answers); err == nil {
				item.PredictedScore = answers.PredictionScore
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetStudentHistoryDetails returns full historical records up to the current course.
func (r *Repository) GetStudentHistoryDetails(studentID string, currentCourseID int) ([]models.StudentHistoryDetail, error) {
	currentCourse, err := r.GetCourseByID(currentCourseID)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(`
		SELECT s.course_id, c.name, c.lesson_number, s.student_id, s.student_name, s.class_name,
		       s.part1_answers, s.part2_answer, s.part3_answers, s.part3_score,
		       s.teacher_score, s.teacher_score_note, s.teacher_score_updated_at, s.part4_answers,
		       s.submitted_at, s.updated_at
		FROM student_surveys s
		JOIN courses c ON s.course_id = c.id
		WHERE s.student_id = ? AND c.lesson_number <= ?
		ORDER BY c.lesson_number DESC
	`, studentID, currentCourse.LessonNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.StudentHistoryDetail
	for rows.Next() {
		var item models.StudentHistoryDetail
		var part1, part2, part3, part4, submittedAt, teacherNote, teacherScoreUpdatedAt sql.NullString
		var part3Score, teacherScore sql.NullInt64
		if err := rows.Scan(
			&item.CourseID, &item.CourseName, &item.LessonNo, &item.StudentID, &item.StudentName, &item.ClassName,
			&part1, &part2, &part3, &part3Score, &teacherScore, &teacherNote, &teacherScoreUpdatedAt, &part4, &submittedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if part1.Valid {
			item.Part1 = json.RawMessage(part1.String)
		}
		if part2.Valid {
			item.Part2 = utils.Ptr(part2.String)
		}
		if part3.Valid {
			item.Part3 = json.RawMessage(part3.String)
		}
		if part3Score.Valid {
			score := int(part3Score.Int64)
			item.Part3Score = &score
		}
		if teacherScore.Valid {
			score := int(teacherScore.Int64)
			item.TeacherScore = &score
		}
		if teacherNote.Valid {
			item.TeacherNote = utils.Ptr(teacherNote.String)
		}
		if teacherScoreUpdatedAt.Valid {
			item.TeacherScoreUpdatedAt = utils.Ptr(teacherScoreUpdatedAt.String)
		}
		if part4.Valid {
			item.Part4 = json.RawMessage(part4.String)
		}
		if submittedAt.Valid {
			item.SubmittedAt = utils.Ptr(submittedAt.String)
		}
		setHistoryActualScore(&item)
		items = append(items, item)
	}
	return items, rows.Err()
}
