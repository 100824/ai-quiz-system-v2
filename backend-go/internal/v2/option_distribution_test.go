package v2

import (
	"database/sql"
	"encoding/json"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestOptionDistributionsPreserveOrderAndIgnoreRetakes(t *testing.T) {
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

	classID, err := repo.CreateClass("分布测试班", "")
	if err != nil {
		t.Fatal(err)
	}
	student1, _ := repo.CreateStudent(classID, "学生一", "")
	student2, _ := repo.CreateStudent(classID, "学生二", "")
	courseID, err := repo.CreateCourse("分布测试课堂", "", "", "free")
	if err != nil {
		t.Fatal(err)
	}
	sectionID, err := repo.CreateSection(courseID, "选择练习", "quiz")
	if err != nil {
		t.Fatal(err)
	}

	choiceOptions := json.RawMessage(`["A. 苹果","B. 香蕉","C. 梨","其它：____"]`)
	singleID, err := repo.CreateQuestion(Question{SectionID: sectionID, Type: "single_choice", Title: "最喜欢哪种水果？", Options: choiceOptions, CorrectAnswer: json.RawMessage(`"B"`), SortOrder: 1})
	if err != nil {
		t.Fatal(err)
	}
	multipleID, err := repo.CreateQuestion(Question{SectionID: sectionID, Type: "multiple_choice", Title: "选择学习方法", Options: json.RawMessage(`["听讲","练习","讨论"]`), CorrectAnswer: json.RawMessage(`["听讲","2"]`), SortOrder: 2})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.SubmitSection(courseID, classID, student1, sectionID, map[string]json.RawMessage{
		stringID(singleID):   json.RawMessage(`"A"`),
		stringID(multipleID): json.RawMessage(`["0","2"]`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SubmitSection(courseID, classID, student2, sectionID, map[string]json.RawMessage{
		stringID(singleID):   json.RawMessage(`"其它：草莓"`),
		stringID(multipleID): json.RawMessage(`["0"]`),
	}); err != nil {
		t.Fatal(err)
	}
	// A second submission is the retake and must not alter the official distribution.
	if _, err := repo.SubmitSection(courseID, classID, student1, sectionID, map[string]json.RawMessage{
		stringID(singleID):   json.RawMessage(`"B"`),
		stringID(multipleID): json.RawMessage(`["1"]`),
	}); err != nil {
		t.Fatal(err)
	}

	stats, err := repo.BuildStatsSummary(courseID, classID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.OptionDistributions) != 2 {
		t.Fatalf("got %d option distributions, want 2", len(stats.OptionDistributions))
	}
	single := stats.OptionDistributions[0]
	if single.QuestionID != singleID || single.RespondentCount != 2 {
		t.Fatalf("unexpected single-choice distribution: %#v", single)
	}
	wantSingle := []OptionDistributionItem{
		{Label: "A. 苹果", Count: 1, Percentage: 50},
		{Label: "B. 香蕉", Count: 0, Percentage: 0, IsCorrect: true},
		{Label: "C. 梨", Count: 0, Percentage: 0},
		{Label: "其它（自定义）", Count: 1, Percentage: 50},
	}
	if !optionDistributionItemsEqual(single.Options, wantSingle) {
		t.Fatalf("single-choice options = %#v, want %#v", single.Options, wantSingle)
	}

	multiple := stats.OptionDistributions[1]
	if multiple.QuestionID != multipleID || multiple.RespondentCount != 2 {
		t.Fatalf("unexpected multiple-choice distribution: %#v", multiple)
	}
	wantMultiple := []OptionDistributionItem{
		{Label: "听讲", Count: 2, Percentage: 100, IsCorrect: true},
		{Label: "练习", Count: 0, Percentage: 0},
		{Label: "讨论", Count: 1, Percentage: 50, IsCorrect: true},
	}
	if !optionDistributionItemsEqual(multiple.Options, wantMultiple) {
		t.Fatalf("multiple-choice options = %#v, want %#v", multiple.Options, wantMultiple)
	}
}

func optionDistributionItemsEqual(left, right []OptionDistributionItem) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
