package v2

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestDefaultClassCanBeSetChangedAndClearedOnDelete(t *testing.T) {
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
	firstID, err := repo.CreateClass("默认班级一", "")
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := repo.CreateClass("默认班级二", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.SetDefaultClassID(firstID); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetDefaultClassID(); err != nil || got != firstID {
		t.Fatalf("default class = %d, err = %v; want %d", got, err, firstID)
	}
	if err := repo.SetDefaultClassID(secondID); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetDefaultClassID(); err != nil || got != secondID {
		t.Fatalf("changed default class = %d, err = %v; want %d", got, err, secondID)
	}
	if err := repo.DeleteClass(secondID); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetDefaultClassID(); err != nil || got != 0 {
		t.Fatalf("default class after delete = %d, err = %v; want 0", got, err)
	}
	if err := repo.SetDefaultClassID(secondID); err == nil || !strings.Contains(err.Error(), "班级不存在或已删除") {
		t.Fatalf("set deleted class as default error = %v", err)
	}
}
