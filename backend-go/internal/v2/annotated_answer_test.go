package v2

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/xuri/excelize/v2"
)

func TestNormalizeOpenTextAnswerPreservesOnlySupportedHighlights(t *testing.T) {
	raw, _ := json.Marshal(`<div><b>第一行</b></div><div><span style="color:red" class="part2-highlight part2-highlight--green">关键事实</span><script>alert(1)</script></div>`)
	normalized, err := normalizeOpenTextAnswer(raw)
	if err != nil {
		t.Fatalf("normalizeOpenTextAnswer returned error: %v", err)
	}
	var answer string
	if err := json.Unmarshal(normalized, &answer); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(answer, "<b>") || strings.Contains(answer, "script") || strings.Contains(answer, "style=") {
		t.Fatalf("unsupported markup was preserved: %s", answer)
	}
	if !strings.Contains(answer, `class="part2-highlight part2-highlight--green"`) {
		t.Fatalf("supported highlight was lost: %s", answer)
	}
	if !strings.Contains(answer, "第一行") || !strings.Contains(answer, "关键事实") {
		t.Fatalf("answer text was lost: %s", answer)
	}
}

func TestSubmitSectionValidatesAndStoresSanitizedOpenText(t *testing.T) {
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

	classResult, err := db.Exec(`INSERT INTO classes (name) VALUES ('测试班')`)
	if err != nil {
		t.Fatal(err)
	}
	classID, _ := classResult.LastInsertId()
	studentResult, err := db.Exec(`INSERT INTO students (class_id, name) VALUES (?, '测试学生')`, classID)
	if err != nil {
		t.Fatal(err)
	}
	studentID, _ := studentResult.LastInsertId()
	courseResult, err := db.Exec(`INSERT INTO courses (template_code, title, mode) VALUES ('free', '测试课堂', 'free')`)
	if err != nil {
		t.Fatal(err)
	}
	courseID, _ := courseResult.LastInsertId()
	sectionResult, err := db.Exec(`INSERT INTO course_sections (course_id, section_key, title, type, sort_order) VALUES (?, 'custom_1', '开放题', 'custom', 1)`, courseID)
	if err != nil {
		t.Fatal(err)
	}
	sectionID, _ := sectionResult.LastInsertId()

	repo := NewRepository(db)
	questionID, err := repo.CreateQuestion(Question{
		CourseID:  int(courseID),
		SectionID: int(sectionID),
		Type:      "open_text",
		Title:     "请标注你的想法",
	})
	if err != nil {
		t.Fatal(err)
	}

	plainAnswer, _ := json.Marshal("没有标注")
	_, err = repo.SubmitSection(int(courseID), int(classID), int(studentID), int(sectionID), map[string]json.RawMessage{
		jsonKey(questionID): plainAnswer,
	})
	if err == nil || !strings.Contains(err.Error(), "至少需要标注一处颜色") {
		t.Fatalf("expected server-side annotation validation, got %v", err)
	}

	richAnswer, _ := json.Marshal(`<div><font color="red">第一行</font></div><div><span class="part2-highlight part2-highlight--green" style="font-size:30px">关键事实</span></div>`)
	result, err := repo.SubmitSection(int(courseID), int(classID), int(studentID), int(sectionID), map[string]json.RawMessage{
		jsonKey(questionID): richAnswer,
	})
	if err != nil {
		t.Fatalf("valid annotated answer was rejected: %v", err)
	}
	if result["submissionId"] == nil {
		t.Fatalf("submission id missing: %#v", result)
	}

	var stored string
	if err := db.QueryRow(`SELECT answer_json FROM answer_records LIMIT 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "font") || strings.Contains(stored, "style") {
		t.Fatalf("unsupported pasted formatting reached storage: %s", stored)
	}
	if !strings.Contains(stored, "part2-highlight--green") {
		t.Fatalf("highlight was not persisted: %s", stored)
	}

	submissionID, ok := result["submissionId"].(int)
	if !ok || submissionID <= 0 {
		t.Fatalf("invalid submission id: %#v", result["submissionId"])
	}
	handler := NewHandler(repo)
	_, questionRows, _, _, err := handler.buildExportRows([]StudentStats{{
		SubmissionID: submissionID,
		ClassName:    "测试班",
		StudentName:  "测试学生",
	}}, "测试课堂")
	if err != nil {
		t.Fatal(err)
	}
	if len(questionRows) != 2 || !strings.Contains(questionRows[1][8], "[绿色]关键事实[/绿色]") {
		t.Fatalf("open-text export did not preserve color meaning: %#v", questionRows)
	}
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := writeMatrixSheet(workbook, "题目回答明细", questionRows, true); err != nil {
		t.Fatal(err)
	}
	exportedAnswer, err := workbook.GetCellValue("题目回答明细", "I2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exportedAnswer, "[绿色]关键事实[/绿色]") || strings.Contains(exportedAnswer, "<span") {
		t.Fatalf("xlsx answer cell is not readable plain text: %q", exportedAnswer)
	}
}

func jsonKey(value int) string {
	return strconv.Itoa(value)
}

func TestNormalizeOpenTextAnswerRequiresHighlight(t *testing.T) {
	raw, _ := json.Marshal("只有普通文字")
	_, err := normalizeOpenTextAnswer(raw)
	if err == nil || !strings.Contains(err.Error(), "至少需要标注一处颜色") {
		t.Fatalf("expected highlight validation error, got %v", err)
	}
}

func TestOpenTextExportAnswerIncludesColorMeaning(t *testing.T) {
	answer := `<span class="part2-highlight part2-highlight--yellow">不清楚</span><br><span class="part2-highlight part2-highlight--red">不同意</span>`
	got := openTextExportAnswer(answer)
	want := "[黄色]不清楚[/黄色]\n[红色]不同意[/红色]"
	if got != want {
		t.Fatalf("unexpected export text:\nwant %q\n got %q", want, got)
	}
}
