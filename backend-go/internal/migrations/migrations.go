package migrations

import (
	"database/sql"
	"fmt"
	"strings"

	"ai-quiz-system-v2/backend-go/internal/models"
	"ai-quiz-system-v2/backend-go/internal/utils"
)

// Part2UnderstandingQuestionText is the fixed understanding-level question in part 2.
const Part2UnderstandingQuestionText = "你对人工智能生成内容的理解程度是？"

// ConfigureDB sets SQLite pragmas for performance and safety.
func ConfigureDB(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return err
		}
	}
	return nil
}

// InitDB creates tables, runs migrations and seeds default data.
func InitDB(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS system_config (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS courses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			lesson_number INTEGER NOT NULL UNIQUE,
			description TEXT,
			part2_guide TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS class_lists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			class_name TEXT NOT NULL,
			student_name TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(course_id, class_name, student_name),
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS course_stages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			class_name TEXT NOT NULL,
			current_stage INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(course_id, class_name),
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS class_course_bind (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			class_name TEXT NOT NULL UNIQUE,
			course_id INTEGER NOT NULL,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS questions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			part INTEGER NOT NULL,
			question_type TEXT NOT NULL,
			question_text TEXT NOT NULL,
			options TEXT,
			correct_answer TEXT,
			explanation TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			annotation_enabled INTEGER NOT NULL DEFAULT 1,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS student_surveys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			student_id TEXT NOT NULL,
			student_name TEXT NOT NULL,
			class_name TEXT NOT NULL,
			part1_answers TEXT,
			part2_answer TEXT,
			part3_answers TEXT,
			part3_score INTEGER,
			part4_answers TEXT,
			submitted_at TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(course_id, student_id),
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS course_part_settings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			part INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(course_id, part),
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS class_tips (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			class_name TEXT NOT NULL UNIQUE,
			content TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	// Lightweight migrations: add columns if they don't exist.
	if _, err := db.Exec(`ALTER TABLE courses ADD COLUMN part2_guide TEXT`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE questions ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE questions ADD COLUMN annotation_enabled INTEGER NOT NULL DEFAULT 1`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}

	// Seed default courses (12 lessons).
	var courseCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM courses`).Scan(&courseCount); err != nil {
		return err
	}
	if courseCount == 0 {
		for i := 1; i <= 12; i++ {
			if _, err := db.Exec(`INSERT INTO courses (name, lesson_number) VALUES (?, ?)`, fmt.Sprintf("第%d节课", i), i); err != nil {
				return err
			}
		}
	}

	// Ensure current_course_id exists.
	if _, err := db.Exec(`
		INSERT INTO system_config (key, value, updated_at)
		VALUES ('current_course_id', '1', CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO NOTHING
	`); err != nil {
		return err
	}

	if err := initDefaultQuestions(db); err != nil {
		return err
	}
	return ensurePart2UnderstandingQuestion(db)
}

func initDefaultQuestions(db *sql.DB) error {
	rows, err := db.Query(`SELECT id FROM courses ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var courseIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return err
		}
		courseIDs = append(courseIDs, id)
	}

	insertStmt := `INSERT INTO questions (course_id, part, question_type, question_text, options, correct_answer, explanation, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	for _, courseID := range courseIDs {
		for part := 1; part <= 4; part++ {
			if _, err := db.Exec(`
				INSERT INTO course_part_settings (course_id, part, enabled, updated_at)
				VALUES (?, ?, 1, CURRENT_TIMESTAMP)
				ON CONFLICT(course_id, part) DO NOTHING
			`, courseID, part); err != nil {
				return err
			}
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM questions WHERE course_id = ?`, courseID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		defaults := buildDefaultQuestions(courseID)
		for _, q := range defaults {
			if _, err := db.Exec(insertStmt, q.CourseID, q.Part, q.QuestionType, q.QuestionText, q.Options, q.CorrectAnswer, q.Explanation, q.SortOrder); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensurePart2UnderstandingQuestion(db *sql.DB) error {
	rows, err := db.Query(`SELECT id FROM courses ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	options := utils.MustJSON([]string{"A. 完全理解", "B. 理解大部分", "C. 理解小部分", "D. 完全不理解"})
	for rows.Next() {
		var courseID int
		if err := rows.Scan(&courseID); err != nil {
			return err
		}

		var existingID int
		err := db.QueryRow(`
			SELECT id
			FROM questions
			WHERE course_id = ? AND part = 2 AND (sort_order = 0 OR question_text = ?)
			LIMIT 1
		`, courseID, Part2UnderstandingQuestionText).Scan(&existingID)
		if err == sql.ErrNoRows {
			if _, err := db.Exec(`
				INSERT INTO questions (course_id, part, question_type, question_text, options, correct_answer, explanation, enabled, sort_order)
				VALUES (?, 2, 'single', ?, ?, NULL, NULL, 1, 0)
			`, courseID, Part2UnderstandingQuestionText, options); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if _, err := db.Exec(`
			UPDATE questions
			SET question_type = 'single',
			    question_text = ?,
			    options = ?,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, Part2UnderstandingQuestionText, options, existingID); err != nil {
			return err
		}
	}
	return rows.Err()
}

func buildDefaultQuestions(courseID int) []models.Question {
	options1 := utils.MustJSON([]string{"0分 - 完全没把握", "1分 - 我觉得很难", "2分 - 可能会有点难", "3分 - 能学会一半吧", "4分 - 大部分能学会", "5分 - 我全都能学会！"})
	options2 := utils.MustJSON([]string{"竖起耳朵认真听讲", "遇到不懂的查资料", "请教老师或同学", "边听边记笔记", "和同桌讨论", "其他"})
	part3Single := utils.MustJSON([]string{"A. 数据", "B. 算法", "C. 规则", "D. 算力"})
	part3Judge := utils.MustJSON([]string{"对", "错"})
	return []models.Question{
		{CourseID: courseID, Part: 1, QuestionType: "single", QuestionText: "今天这节课，你觉得你能学会多少呢？0到5分，给自己打个分吧！", Options: &options1, SortOrder: 1},
		{CourseID: courseID, Part: 1, QuestionType: "multi", QuestionText: "为了达到学习效果，你打算怎么做？", Options: &options2, SortOrder: 2},
		{CourseID: courseID, Part: 2, QuestionType: "text", QuestionText: "人工智能可以代替人类做什么，不能做什么？请写下你的思考。", SortOrder: 1},
		{CourseID: courseID, Part: 3, QuestionType: "single", QuestionText: "下面哪一项不是人工智能的三个核心要素？", Options: &part3Single, CorrectAnswer: utils.Ptr("C"), Explanation: utils.Ptr("人工智能的三个核心要素是数据、算法和算力。规则不是核心要素。"), SortOrder: 1},
		{CourseID: courseID, Part: 3, QuestionType: "single", QuestionText: "下面哪一项是让人工智能算得又快又猛的\"火力\"？", Options: &part3Single, CorrectAnswer: utils.Ptr("D"), Explanation: utils.Ptr("算力是让人工智能算得又快又猛的\"火力\"，它决定了人工智能的计算速度和能力。"), SortOrder: 2},
		{CourseID: courseID, Part: 3, QuestionType: "judge", QuestionText: "人工智能是人类制造的、能模仿人类智力和能力的一种技术。", Options: &part3Judge, CorrectAnswer: utils.Ptr("对"), Explanation: utils.Ptr("这个说法是正确的。人工智能是人类制造的、能模仿人类智力和能力的一种技术。"), SortOrder: 3},
		{CourseID: courseID, Part: 3, QuestionType: "judge", QuestionText: "学习累了的时候，可以让人工智能帮忙写作业。", Options: &part3Judge, CorrectAnswer: utils.Ptr("错"), Explanation: utils.Ptr("这个说法是错误的。我们应该自己完成作业，人工智能可以帮助我们学习，但不能代替我们写作业。"), SortOrder: 4},
		{CourseID: courseID, Part: 3, QuestionType: "judge", QuestionText: "人工智能是一种可以帮助我们学习的工具。", Options: &part3Judge, CorrectAnswer: utils.Ptr("对"), Explanation: utils.Ptr("这个说法是正确的。人工智能是一种可以帮助我们学习的工具，我们应该正确使用它。"), SortOrder: 5},
	}
}
