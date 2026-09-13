package v2

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestDeleteQuestionPermanentlyRemovesQuestionAndAnswers(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	if err := ConfigureDB(db); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(db); err != nil {
		t.Fatal(err)
	}

	classResult, err := db.Exec(`INSERT INTO classes (name) VALUES ('删除题目测试班')`)
	if err != nil {
		t.Fatal(err)
	}
	classID, _ := classResult.LastInsertId()
	studentResult, err := db.Exec(`INSERT INTO students (class_id, name) VALUES (?, '测试学生')`, classID)
	if err != nil {
		t.Fatal(err)
	}
	studentID, _ := studentResult.LastInsertId()
	courseResult, err := db.Exec(`INSERT INTO courses (template_code, title, mode) VALUES ('free', '删除题目测试课程', 'free')`)
	if err != nil {
		t.Fatal(err)
	}
	courseID, _ := courseResult.LastInsertId()
	sectionResult, err := db.Exec(`INSERT INTO course_sections (course_id, section_key, title, type) VALUES (?, 'custom_delete', '删除测试环节', 'custom')`, courseID)
	if err != nil {
		t.Fatal(err)
	}
	sectionID, _ := sectionResult.LastInsertId()
	questionResult, err := db.Exec(`INSERT INTO questions (course_id, section_id, type, title) VALUES (?, ?, 'fill_blank', '待删除题目')`, courseID, sectionID)
	if err != nil {
		t.Fatal(err)
	}
	questionID, _ := questionResult.LastInsertId()
	submissionResult, err := db.Exec(`INSERT INTO submissions (course_id, class_id, student_id) VALUES (?, ?, ?)`, courseID, classID, studentID)
	if err != nil {
		t.Fatal(err)
	}
	submissionID, _ := submissionResult.LastInsertId()
	attemptResult, err := db.Exec(`INSERT INTO answer_attempts (submission_id, section_id, attempt_no) VALUES (?, ?, 1)`, submissionID, sectionID)
	if err != nil {
		t.Fatal(err)
	}
	attemptID, _ := attemptResult.LastInsertId()
	if _, err := db.Exec(`INSERT INTO answer_records (attempt_id, question_id, answer_json) VALUES (?, ?, '"answer"')`, attemptID, questionID); err != nil {
		t.Fatal(err)
	}

	repo := NewRepository(db)
	if err := repo.DeleteQuestion(int(questionID)); err != nil {
		t.Fatalf("delete question: %v", err)
	}

	var questionCount, answerCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM questions WHERE id = ?`, questionID).Scan(&questionCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM answer_records WHERE question_id = ?`, questionID).Scan(&answerCount); err != nil {
		t.Fatal(err)
	}
	if questionCount != 0 || answerCount != 0 {
		t.Fatalf("question count = %d, answer count = %d; want both zero", questionCount, answerCount)
	}
}
