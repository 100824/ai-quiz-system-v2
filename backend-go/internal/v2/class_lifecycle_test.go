package v2

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestCreateClassRestoresSoftDeletedClassWithRoster(t *testing.T) {
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

	repo := NewRepository(db)
	classID, err := repo.CreateClass("恢复测试班", "原班级说明")
	if err != nil {
		t.Fatal(err)
	}
	studentID, err := repo.CreateStudent(classID, "原名单学生", "001")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteClass(classID); err != nil {
		t.Fatal(err)
	}

	restoredID, err := repo.CreateClass("  恢复测试班  ", "新说明不应覆盖原记录")
	if err != nil {
		t.Fatalf("restore soft-deleted class: %v", err)
	}
	if restoredID != classID {
		t.Fatalf("restored class id = %d, want original id %d", restoredID, classID)
	}

	var deletedAt sql.NullString
	var description string
	if err := db.QueryRow(`SELECT deleted_at, description FROM classes WHERE id = ?`, classID).Scan(&deletedAt, &description); err != nil {
		t.Fatal(err)
	}
	if deletedAt.Valid {
		t.Fatalf("restored class still has deleted_at: %q", deletedAt.String)
	}
	if description != "原班级说明" {
		t.Fatalf("restored class description = %q, want original description", description)
	}

	students, err := repo.ListStudents(classID)
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 1 || students[0].ID != studentID || students[0].Name != "原名单学生" {
		t.Fatalf("restored roster = %#v", students)
	}

	_, err = repo.CreateClass("恢复测试班", "")
	if err == nil || !strings.Contains(err.Error(), "班级名称已存在") {
		t.Fatalf("active duplicate class error = %v", err)
	}
}
