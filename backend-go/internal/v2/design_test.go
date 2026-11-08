package v2

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestDesignPlanLifecycle(t *testing.T) {
	db, repo := setupGuidanceTestRepository(t)
	defer db.Close()
	if err := initDesignPlans(db); err != nil {
		t.Fatal(err)
	} // Migration is repeatable.
	for _, stmt := range []string{`INSERT INTO classes(id,name) VALUES(1,'甲班'),(2,'乙班')`, `INSERT INTO students(id,class_id,name) VALUES(1,1,'学生甲'),(2,1,'学生乙'),(3,2,'学生丙'),(4,1,'学生丁')`} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	p, err := repo.SaveDesignPlan(DesignPlan{Title: "设计测试", Questions: []string{"目标是什么？", "怎么实现？"}, ClassIDs: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.SetActiveDesignPlan(1, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.SaveDesignPlan(DesignPlan{Title: "空题", Questions: []string{" "}}); err == nil {
		t.Fatal("accepted blank question")
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 1, 2, []string{"目标", "步骤"}); err == nil {
		t.Fatal("accepted second before first")
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 3, 1, []string{"目标", "步骤"}); err == nil {
		t.Fatal("accepted wrong roster")
	}
	if err = repo.SubmitDesignVersion(p.ID, 2, 3, 1, []string{"目标", "步骤"}); err == nil {
		t.Fatal("accepted unbound class")
	}
	for _, answers := range [][]string{{"目标"}, {"目标", " \n "}} {
		if err = repo.SubmitDesignVersion(p.ID, 1, 1, 1, answers); err == nil {
			t.Fatal("accepted missing answer")
		}
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 1, 1, []string{" 目标 ", "原 步骤"}); err != nil {
		t.Fatal(err)
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 1, 1, []string{"覆盖", "覆盖"}); err == nil {
		t.Fatal("accepted duplicate")
	}
	p.Questions[0] = "换题"
	if _, err = repo.SaveDesignPlan(*p); err == nil {
		t.Fatal("accepted locked question edit")
	}
	p.Questions[0] = "目标是什么？"
	p.Title = "更新标题"
	if _, err = repo.SaveDesignPlan(*p); err != nil {
		t.Fatal(err)
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 1, 2, []string{"改进目标", "原 步骤"}); err == nil {
		t.Fatal("second version opened without teacher")
	}
	if err = repo.AdvanceDesignProgress(p.ID, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 1, 2, []string{"目\n标", "原\t步骤"}); err == nil {
		t.Fatal("accepted whitespace only change")
	}
	// The teacher, not elapsed time, permits the second submission.
	if err = repo.SubmitDesignVersion(p.ID, 1, 1, 2, []string{"优化后的目标", "原 步骤"}); err != nil {
		t.Fatal(err)
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 1, 2, []string{"再次修改", "步骤"}); err == nil {
		t.Fatal("accepted second overwrite")
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 1, 3, []string{"目标", "步骤"}); err == nil {
		t.Fatal("accepted third version")
	}
	versions, err := loadDesignVersions(db, p.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].Title != "设计测试" || versions[1].Title != "设计测试" || versions[0].Answers[0] != "目标" || versions[1].Answers[0] != "优化后的目标" || versions[0].SubmittedAt == "" {
		t.Fatalf("wrong snapshots: %+v", versions)
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 2, 1, []string{"另一目标", "另一步骤"}); err != nil {
		t.Fatal(err)
	}
	other, err := repo.SaveDesignPlan(DesignPlan{Title: "另一个方案", Questions: []string{"独立问题"}, ClassIDs: []int{2}})
	if err != nil {
		t.Fatal(err)
	}
	otherVersions, err := loadDesignVersions(db, other.ID, 1)
	if err != nil || len(otherVersions) != 0 {
		t.Fatal("versions leaked across plans")
	}
	plans, err := repo.ListDesignPlans(1)
	if err != nil || len(plans) != 1 || plans[0].ID != p.ID {
		t.Fatal("class binding leaked")
	}

	mux := http.NewServeMux()
	NewHandler(repo).Register(mux)
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	base := "/api/v2/design-plans/" + strconv.Itoa(p.ID)
	w := get(base + "/export?classId=1")
	wb, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	summary, err := wb.GetRows("完成状态")
	if err != nil {
		t.Fatal(err)
	}
	if len(summary) != 4 {
		t.Fatalf("wrong export rows: %+v", summary)
	}
	details, err := wb.GetRows("两版答案对照")
	if err != nil || len(details) != 7 {
		t.Fatalf("wrong detail export: %+v %v", details, err)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("PK")) {
		t.Fatal("not xlsx")
	}
	var result struct {
		Data struct {
			Students []DesignStudentRecord `json:"students"`
		} `json:"data"`
	}
	if err = json.Unmarshal(get(base+"/records").Body.Bytes(), &result); err != nil || len(result.Data.Students) != 3 {
		t.Fatal("records missing", err)
	}
	get(base + "/student?classId=1&studentId=1")
	get("/api/v2/classes/1/design-plans?studentId=1")
	// Unbinding retains historical records and both versions; student access follows binding.
	p.ClassIDs = []int{2}
	if _, err = repo.SaveDesignPlan(*p); err != nil {
		t.Fatal(err)
	}
	records, err := repo.DesignRecords(p.ID, 1)
	if err != nil || len(records) != 2 {
		t.Fatal("unbinding lost history", err)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", base+"/student?classId=1&studentId=1", nil))
	if w.Code != 400 {
		t.Fatal("unbound student retained access")
	}
	// No changes to existing course/submission records.
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM submissions`).Scan(&count); err != nil || count != 0 {
		t.Fatal("design touched classroom submissions")
	}
}
