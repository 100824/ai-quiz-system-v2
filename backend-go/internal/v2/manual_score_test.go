package v2

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestTeacherCanScoreStudentWithoutSubmission(t *testing.T) {
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

	classResult, err := db.Exec(`INSERT INTO classes (name) VALUES ('评分测试班')`)
	if err != nil {
		t.Fatal(err)
	}
	classID, _ := classResult.LastInsertId()
	studentResult, err := db.Exec(`INSERT INTO students (class_id, name) VALUES (?, '未答题学生')`, classID)
	if err != nil {
		t.Fatal(err)
	}
	studentID, _ := studentResult.LastInsertId()
	courseResult, err := db.Exec(`INSERT INTO courses (template_code, title, mode) VALUES ('free', '评分测试课堂', 'free')`)
	if err != nil {
		t.Fatal(err)
	}
	courseID, _ := courseResult.LastInsertId()

	repo := NewRepository(db)
	score := 4
	submissionID, err := repo.SaveTeacherScoreForStudent(0, int(courseID), int(classID), int(studentID), &score, "线下作业评分")
	if err != nil {
		t.Fatalf("teacher score without submission failed: %v", err)
	}
	if submissionID <= 0 {
		t.Fatalf("invalid generated submission id: %d", submissionID)
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM submissions WHERE id = ?`, submissionID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "teacher_scored" {
		t.Fatalf("placeholder submission status = %q, want teacher_scored", status)
	}

	stats, err := repo.BuildStatsSummary(int(courseID), int(classID))
	if err != nil {
		t.Fatal(err)
	}
	if stats.SubmittedCount != 0 {
		t.Fatalf("teacher-only score was counted as a student submission: %d", stats.SubmittedCount)
	}
	if len(stats.NotSubmittedStudents) != 1 || stats.NotSubmittedStudents[0] != "未答题学生" {
		t.Fatalf("unexpected not-submitted students: %#v", stats.NotSubmittedStudents)
	}
	if len(stats.Students) != 1 || stats.Students[0].TeacherScore == nil || *stats.Students[0].TeacherScore != score {
		t.Fatalf("teacher score missing from stats: %#v", stats.Students)
	}
	if stats.Students[0].SubmissionID != submissionID || stats.Students[0].StatusText != "未开始" {
		t.Fatalf("unexpected teacher-only student status: %#v", stats.Students[0])
	}
}
