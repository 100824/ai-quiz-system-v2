package v2

import (
	"database/sql"
	"fmt"
)

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

func InitDB(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS classes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			deleted_at TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS students (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			class_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			student_no TEXT NOT NULL DEFAULT '',
			deleted_at TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(class_id, name),
			FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS course_templates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			config_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS courses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			template_id INTEGER,
			template_code TEXT NOT NULL DEFAULT 'free',
			title TEXT NOT NULL,
			mode TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			deleted_at TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (template_id) REFERENCES course_templates(id)
		)`,
		`CREATE TABLE IF NOT EXISTS course_classes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			class_id INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(course_id, class_id),
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE,
			FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS course_sections (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			section_key TEXT NOT NULL,
			title TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'custom',
			sort_order INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			fixed INTEGER NOT NULL DEFAULT 0,
			rules_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(course_id, section_key),
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS questions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			section_id INTEGER NOT NULL,
			question_key TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			options_json TEXT NOT NULL DEFAULT '[]',
			correct_answer_json TEXT NOT NULL DEFAULT 'null',
			explanation TEXT NOT NULL DEFAULT '',
			score INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			fixed INTEGER NOT NULL DEFAULT 0,
			rules_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE,
			FOREIGN KEY (section_id) REFERENCES course_sections(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS classroom_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			class_id INTEGER NOT NULL,
			stage_id INTEGER NOT NULL DEFAULT 0,
			blackboard TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(course_id, class_id),
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE,
			FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS submissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			class_id INTEGER NOT NULL,
			student_id INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'in_progress',
			started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(course_id, student_id),
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE,
			FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE,
			FOREIGN KEY (student_id) REFERENCES students(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS answer_attempts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			submission_id INTEGER NOT NULL,
			section_id INTEGER NOT NULL,
			attempt_no INTEGER NOT NULL,
			score INTEGER,
			submitted_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(submission_id, section_id, attempt_no),
			FOREIGN KEY (submission_id) REFERENCES submissions(id) ON DELETE CASCADE,
			FOREIGN KEY (section_id) REFERENCES course_sections(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS answer_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			attempt_id INTEGER NOT NULL,
			question_id INTEGER NOT NULL,
			answer_json TEXT NOT NULL DEFAULT 'null',
			score INTEGER,
			is_correct INTEGER,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (attempt_id) REFERENCES answer_attempts(id) ON DELETE CASCADE,
			FOREIGN KEY (question_id) REFERENCES questions(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS score_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			submission_id INTEGER NOT NULL UNIQUE,
			predicted_score INTEGER,
			actual_score INTEGER,
			actual_score_source TEXT NOT NULL DEFAULT 'none',
			guess_result TEXT NOT NULL DEFAULT 'unknown',
			teacher_score INTEGER,
			teacher_score_note TEXT NOT NULL DEFAULT '',
			teacher_score_updated_at TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (submission_id) REFERENCES submissions(id) ON DELETE CASCADE
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	if err := ensureColumn(db, "score_records", "teacher_score", "INTEGER"); err != nil {
		return err
	}
	if err := ensureColumn(db, "score_records", "teacher_score_note", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "score_records", "teacher_score_updated_at", "TEXT"); err != nil {
		return err
	}
	if err := dedupeCourseTitles(db); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_courses_title_active ON courses(title) WHERE deleted_at IS NULL`); err != nil {
		return err
	}
	return seedTemplates(db)
}

func dedupeCourseTitles(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT title
		FROM courses
		WHERE deleted_at IS NULL
		GROUP BY title
		HAVING COUNT(*) > 1
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return err
		}
		var keepID int
		if err := db.QueryRow(`
			SELECT id
			FROM courses
			WHERE title = ? AND deleted_at IS NULL
			ORDER BY id DESC
			LIMIT 1
		`, title).Scan(&keepID); err != nil {
			return err
		}
		if _, err := db.Exec(`
			UPDATE courses
			SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE title = ? AND deleted_at IS NULL AND id <> ?
		`, title, keepID); err != nil {
			return err
		}
	}
	return rows.Err()
}

func ensureColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, definition))
	return err
}

func seedTemplates(db *sql.DB) error {
	templates := []struct {
		code, name, description, config string
	}{
		{
			code:        "free",
			name:        "自由模式",
			description: "教师自由创建部分并添加题目。",
			config:      `{"sections":[],"rules":{"teacherCanAddSections":true}}`,
		},
		{
			code:        "reflection",
			name:        "反思模式",
			description: "基于预测、学习、测验、反思的模板规则生成课程。",
			config: `{
				"sections":[
					{"key":"prediction","title":"第一部分：课前预测","type":"prediction","fixed":true,"rules":{"fixedPredictionQuestion":true,"teacherCanAddQuestions":true}},
					{"key":"learning","title":"第二部分：学习与思考","type":"learning","fixed":true,"rules":{"teacherCanAddQuestions":true}},
					{"key":"quiz","title":"第三部分：小测验","type":"quiz","fixed":true,"rules":{"fixedQuestionCount":5,"allowRetry":true,"recordScoreAttempt":1}},
					{"key":"reflection","title":"第四部分：反思总结","type":"reflection","fixed":true,"rules":{"showScoreCompare":true,"teacherCanAddQuestions":true}}
				],
				"scoreRules":{"predictionSection":"prediction","actualScoreSection":"quiz","actualScoreAttempt":1}
			}`,
		},
	}
	for _, item := range templates {
		if _, err := db.Exec(`
			INSERT INTO course_templates (code, name, description, config_json)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				config_json = excluded.config_json,
				updated_at = CURRENT_TIMESTAMP
		`, item.code, item.name, item.description, item.config); err != nil {
			return err
		}
	}
	return nil
}
