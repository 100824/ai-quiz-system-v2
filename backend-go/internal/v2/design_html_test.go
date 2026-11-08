package v2

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

const designHTMLFixture = `<!doctype html><html lang="zh-CN"><head><title>校园计数器</title><style>body{font-family:sans-serif;padding:20px;background:#eef6ff}button{padding:16px}</style></head><body><h1>绿色校园</h1><button id="add">加一</button><output id="score">0</output><script>let n=0;document.getElementById('add').addEventListener('click',()=>{document.getElementById('score').textContent=++n;});</script></body></html>`

func TestDesignHTMLOldSchemaMigration(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`CREATE TABLE design_plans(id INTEGER PRIMARY KEY,title TEXT NOT NULL,created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);INSERT INTO design_plans(id,title) VALUES(1,'已有方案')`); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		if err = InitDB(db); err != nil {
			t.Fatal(err)
		}
	}
	p, err := loadDesignPlan(db, 1)
	if err != nil || p.Title != "已有方案" || p.OutputType != "image" || p.OutputTypeLocked {
		t.Fatal("old plan was not preserved", p, err)
	}
}

func TestDesignHTMLProgressMigrationKeepsExistingJob(t *testing.T) {
	_, repo, p := setupDesignImages(t)
	p.OutputType = "html"
	if _, err := repo.SaveDesignPlan(*p); err != nil {
		t.Fatal(err)
	}
	completeDesignPlans(t, repo, p.ID)
	j, err := repo.EnqueueDesignHTML(p.ID, 1, 1, "existing-job", "保留要求")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"reasoning", "generation_phase", "generation_model"} {
		if _, err = repo.db.Exec(`ALTER TABLE design_html_jobs DROP COLUMN ` + name); err != nil {
			t.Fatal(err)
		}
	}
	for n := 0; n < 2; n++ {
		if err = InitDB(repo.db); err != nil {
			t.Fatal(err)
		}
	}
	state, err := repo.DesignHTMLState(p.ID, 1, 0, 0)
	if err != nil || state.ActiveJob == nil || state.ActiveJob.ID != j.ID || state.ActiveJob.Message != "保留要求" || state.ActiveJob.Reasoning != "" {
		t.Fatalf("migration lost existing job: %+v %v", state, err)
	}
	versions, err := loadDesignVersions(repo.db, p.ID, 1)
	if err != nil || len(versions) != 2 {
		t.Fatal("migration changed submitted plans", err)
	}
}

func writeDesignHTMLStream(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"先确定内容结构，再设计按钮交互。\"}}]}\n\n")
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": code}}}})
	fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"finish_reason\":\"stop\",\"delta\":{}}]}\n\ndata: [DONE]\n\n", b)
}

func TestDesignHTMLValidation(t *testing.T) {
	code, err := validateDesignHTML("```html\n" + designHTMLFixture + "\n```")
	if err != nil || !strings.Contains(code, "connect-src &#39;none&#39;") {
		t.Fatalf("valid offline document rejected or policy missing: %v %s", err, code)
	}
	for _, source := range []string{
		"<html><body>fragment</body></html>",
		strings.TrimSuffix(designHTMLFixture, "</html>"),
		strings.Replace(designHTMLFixture, "</head>", `<script src="https://example.org/x.js"></script></head>`, 1),
		strings.Replace(designHTMLFixture, "</head>", `<style>@import 'x.css';</style></head>`, 1),
		strings.Replace(designHTMLFixture, "</body>", `<img src="/api/v2/classes"></body>`, 1),
		strings.Replace(designHTMLFixture, "</body>", `<iframe srcdoc="x"></iframe></body>`, 1),
		strings.Replace(designHTMLFixture, "let n=0;", "fetch('/api/v2/classes');", 1),
		strings.Replace(designHTMLFixture, "</head>", `<meta http-equiv="refresh" content="0;url=https://example.org"></head>`, 1),
		strings.Replace(designHTMLFixture, "</head>", `<style>body{background:url(https://example.org/x.png)}</style></head>`, 1),
		strings.Repeat("x", designHTMLLimit) + designHTMLFixture,
	} {
		if _, err := validateDesignHTML(source); err == nil {
			t.Fatal("unsafe or incomplete HTML accepted")
		}
	}
}

func TestDesignHTMLLiveReasoningProgress(t *testing.T) {
	_, repo, p := setupDesignImages(t)
	p.OutputType = "html"
	if _, err := repo.SaveDesignPlan(*p); err != nil {
		t.Fatal(err)
	}
	completeDesignPlans(t, repo, p.ID)
	t.Setenv("DEEPSEEK_KEY", "fixture-ds")
	t.Setenv("GLM_KEY", "")
	t.Setenv("GLM_API_KEY", "")
	if _, err := repo.EnqueueDesignHTML(p.ID, 1, 1, "reasoning", ""); err != nil {
		t.Fatal(err)
	}
	j, err := repo.ClaimDesignHTMLJob()
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"先设计布局。\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-req.Context().Done():
			return
		}
		chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"reasoning_content": strings.Repeat("思", designHTMLReasoningLimit)}}}})
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		writeDesignHTMLStream(w, designHTMLFixture)
	}))
	defer func() { once.Do(func() { close(release) }); upstream.Close() }()
	h := NewHandler(repo)
	h.deepSeekURL = upstream.URL
	svc, err := newDesignImageService(repo, t.TempDir(), upstream.URL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(svc.htmlDir, 0700); err != nil {
		t.Fatal(err)
	}
	svc.htmlHandler = h
	finished := make(chan struct{})
	go func() { defer close(finished); svc.runHTML(context.Background(), j) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		state, err := repo.DesignHTMLState(p.ID, 1, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if state.ActiveJob != nil && state.ActiveJob.Reasoning == "先设计布局。" {
			if state.ActiveJob.GenerationPhase != "thinking" || state.ActiveJob.GenerationModel == "" || state.VersionCount != 0 {
				t.Fatalf("wrong live state: %+v", state)
			}
			break
		}
		if time.Now().After(deadline) {
			once.Do(func() { close(release) })
			t.Fatal("reasoning unavailable while model was still running")
		}
		time.Sleep(10 * time.Millisecond)
	}
	once.Do(func() { close(release) })
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("generation did not complete")
	}
	state, err := repo.DesignHTMLState(p.ID, 1, 0, 0)
	if err != nil || state.ActiveJob != nil || state.VersionCount != 1 {
		t.Fatalf("generation failed: %+v %v", state, err)
	}
	if !strings.HasSuffix(state.LastJob.Reasoning, "（后续思路已省略）") || len([]rune(state.LastJob.Reasoning)) > designHTMLReasoningLimit+20 {
		t.Fatal("reasoning not bounded")
	}
	data, err := os.ReadFile(filepath.Join(svc.htmlDir, state.LatestVersion.FileName))
	if err != nil || strings.Contains(string(data), "先设计布局") {
		t.Fatal("reasoning leaked into HTML", err)
	}
	if err = repo.UpdateDesignHTMLProgress(j.ID, "wrong", "thinking", "stale callback"); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := repo.DesignHTMLState(p.ID, 1, 0, 0)
	if reloaded.LastJob.Reasoning != state.LastJob.Reasoning {
		t.Fatal("completed task overwritten by late progress")
	}
}

func TestDesignHTMLLifecycleAndFallback(t *testing.T) {
	_, repo, p := setupDesignImages(t)
	completeDesignPlans(t, repo, p.ID)
	if p.OutputType != "image" {
		t.Fatal("old/default plan not image")
	}
	p.OutputType = "html"
	p, err := repo.SaveDesignPlan(*p)
	if err != nil {
		t.Fatal(err)
	}
	omitted := *p
	omitted.OutputType = ""
	saved, err := repo.SaveDesignPlan(omitted)
	if err != nil || saved.OutputType != "html" {
		t.Fatal("omitted mode reset HTML", err)
	}
	if _, err = repo.EnqueueDesignImage(p.ID, 1, 1, "wrong-mode", ""); err == nil {
		t.Fatal("image job accepted for HTML plan")
	}
	t.Setenv("DEEPSEEK_KEY", "fixture-ds")
	t.Setenv("GLM_KEY", "fixture-glm")
	var primary, backup atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MaxTokens       int               `json:"max_tokens"`
			Messages        []AIChatMessage   `json:"messages"`
			Thinking        map[string]string `json:"thinking"`
			ReasoningEffort string            `json:"reasoning_effort"`
		}
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		if body.MaxTokens != 16384 {
			t.Error("HTML token budget not used")
		}
		if body.Thinking["type"] != "enabled" || body.ReasoningEffort != "low" {
			t.Error("HTML thinking was not enabled at low effort")
		}
		if strings.Contains(body.Messages[len(body.Messages)-1].Content, "模拟失败") {
			http.Error(w, "fixture", 503)
			return
		}
		if r.Header.Get("Authorization") == "Bearer fixture-ds" {
			if primary.Add(1) == 1 {
				writeDesignHTMLStream(w, "not HTML")
				return
			}
			if len(body.Messages) != 3 || !strings.Contains(body.Messages[1].Content, "绿色校园") {
				t.Error("latest HTML absent in edit context")
			}
			writeDesignHTMLStream(w, strings.ReplaceAll(designHTMLFixture, "绿色校园", "蓝色校园"))
			return
		}
		backup.Add(1)
		writeDesignHTMLStream(w, designHTMLFixture)
	}))
	defer upstream.Close()
	h := NewHandler(repo)
	h.deepSeekURL = upstream.URL
	h.glmURL = upstream.URL
	svc, err := newDesignImageService(repo, t.TempDir(), upstream.URL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(svc.htmlDir, 0700); err != nil {
		t.Fatal(err)
	}
	svc.htmlHandler = h
	h.designImages = svc
	if _, err = repo.EnqueueDesignHTML(p.ID, 1, 1, "empty", "", []string{" ", "x"}); err == nil {
		t.Fatal("empty source accepted")
	}
	j, err := repo.EnqueueDesignHTML(p.ID, 1, 1, "first", "", []string{"做一个校园计数器", "点击按钮计数"})
	if err != nil {
		t.Fatal(err)
	}
	dup, err := repo.EnqueueDesignHTML(p.ID, 1, 1, "first", "")
	if err != nil || dup.ID != j.ID {
		t.Fatal("idempotency broken", err)
	}
	if _, err = repo.EnqueueDesignHTML(p.ID, 1, 1, "parallel", "x"); err == nil {
		t.Fatal("parallel student job accepted")
	}
	locked, _ := loadDesignPlan(repo.db, p.ID)
	if !locked.OutputTypeLocked {
		t.Fatal("type not locked")
	}
	locked.OutputType = "image"
	if _, err = repo.SaveDesignPlan(*locked); err == nil {
		t.Fatal("type changed after generation began")
	}
	claimed, err := repo.ClaimDesignHTMLJob()
	if err != nil {
		t.Fatal(err)
	}
	svc.runHTML(context.Background(), claimed)
	state, err := repo.DesignHTMLState(p.ID, 1, 0, 0)
	if err != nil || state.VersionCount != 1 {
		t.Fatalf("first version missing: %+v %v", state, err)
	}
	first := state.LatestVersion
	if first.Model != glmModel || primary.Load() != 1 || backup.Load() != 1 {
		t.Fatal("invalid primary HTML did not fall back")
	}
	if state.LastJob.Reasoning != "先确定内容结构，再设计按钮交互。" || state.LastJob.GenerationModel != glmModel || state.LastJob.GenerationPhase != "saving" {
		t.Fatalf("reasoning not persisted or provider attempts mixed: %+v", state.LastJob)
	}
	if state.SourcePlan.Answers[0] != "做一个校园计数器" {
		t.Fatal("source snapshot missing")
	}
	versions, _ := loadDesignVersions(repo.db, p.ID, 1)
	if versions[1].Answers[0] != "科学使用AI" {
		t.Fatal("submitted second version overwritten")
	}
	if _, err = repo.EnqueueDesignHTML(p.ID, 1, 1, "edit", "标题改为蓝色校园"); err != nil {
		t.Fatal(err)
	}
	if err = repo.FinalizeDesignHTML(p.ID, 1, 1, first.ID); err == nil {
		t.Fatal("finalized while generating")
	}
	claimed, _ = repo.ClaimDesignHTMLJob()
	if claimed.SourceVersionID != first.ID {
		t.Fatal("wrong source version")
	}
	svc.runHTML(context.Background(), claimed)
	state, _ = repo.DesignHTMLState(p.ID, 1, 0, 0)
	if state.VersionCount != 2 || state.LatestVersion.ParentVersionID != first.ID {
		t.Fatal("edit history missing")
	}
	if _, err = repo.EnqueueDesignHTML(p.ID, 1, 1, "fail", "模拟失败"); err != nil {
		t.Fatal(err)
	}
	claimed, _ = repo.ClaimDesignHTMLJob()
	svc.runHTML(context.Background(), claimed)
	state, _ = repo.DesignHTMLState(p.ID, 1, 0, 0)
	if state.VersionCount != 2 || state.LastJob.Status != "failed" {
		t.Fatal("failed task destroyed history")
	}
	if err = repo.FinalizeDesignHTML(p.ID, 1, 2, first.ID); err == nil {
		t.Fatal("final ownership bypass")
	}
	if err = repo.FinalizeDesignHTML(p.ID, 1, 1, first.ID); err != nil {
		t.Fatal(err)
	}
	if err = repo.FinalizeDesignHTML(p.ID, 1, 1, state.LatestVersion.ID); err == nil {
		t.Fatal("changed final")
	}
	if _, err = repo.EnqueueDesignHTML(p.ID, 1, 1, "after-final", "x"); err == nil {
		t.Fatal("generated after final")
	}
	mux := http.NewServeMux()
	h.Register(mux)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	base := fmt.Sprintf("/api/v2/design-plans/%d", p.ID)
	w := get(base + "/html?classId=1&studentId=1")
	if w.Code != 200 || strings.Contains(w.Body.String(), `"role":"user"`) {
		t.Fatal("student dialogue exposed")
	}
	w = get(base + "/html-versions/" + first.ID + "?classId=1&studentId=2")
	if w.Code != 404 {
		t.Fatal("source ownership bypass")
	}
	w = get(base + "/html-versions/" + first.ID + "?classId=1&studentId=1")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("source served as executable HTML")
	}
	w = get(base + "/html-versions/" + first.ID + "?classId=1&studentId=1&download=1")
	if !strings.Contains(w.Header().Get("Content-Disposition"), ".html") || !strings.Contains(w.Body.String(), "绿色校园") {
		t.Fatal("HTML download failed")
	}
	w = get(base + "/records/1/html")
	if !strings.Contains(w.Body.String(), `"role":"user"`) {
		t.Fatal("teacher conversation missing")
	}
	w = get(base + "/html-export")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	z, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, file := range z.File {
		if strings.HasSuffix(file.Name, ".html") {
			files++
		}
		if file.Name == "成果清单.xlsx" {
			reader, _ := file.Open()
			data, _ := io.ReadAll(reader)
			reader.Close()
			wb, e := excelize.OpenReader(bytes.NewReader(data))
			if e != nil {
				t.Fatal(e)
			}
			rows, _ := wb.GetRows("网页版本")
			wb.Close()
			if len(rows) != 3 {
				t.Fatal("export missing versions")
			}
		}
	}
	if files != 2 {
		t.Fatal("export missing HTML files")
	}
	if err = os.Remove(filepath.Join(svc.htmlDir, first.FileName)); err != nil {
		t.Fatal(err)
	}
	w = get(base + "/html-export")
	z, err = zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	missing := false
	for _, f := range z.File {
		missing = missing || f.Name == "导出异常.txt"
	}
	if !missing {
		t.Fatal("missing file omitted silently")
	}
}

func TestDesignHTMLProgressAndRecovery(t *testing.T) {
	_, r, p := setupDesignImages(t)
	p.OutputType = "html"
	p, err := r.SaveDesignPlan(*p)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.SubmitDesignVersion(p.ID, 1, 1, 1, []string{"目标", "计划"}); err != nil {
		t.Fatal(err)
	}
	if _, err = r.EnqueueDesignHTML(p.ID, 1, 1, "early", ""); err == nil {
		t.Fatal("stage bypass")
	}
	complete := func() {
		if err = r.AdvanceDesignProgress(p.ID, 1, 2); err != nil {
			t.Fatal(err)
		}
		if err = r.SubmitDesignVersion(p.ID, 1, 1, 2, []string{"新目标", "计划"}); err != nil {
			t.Fatal(err)
		}
	}
	complete()
	if _, err = r.EnqueueDesignHTML(p.ID, 1, 1, "early-2", ""); err == nil {
		t.Fatal("image stage bypass")
	}
	if err = r.AdvanceDesignProgress(p.ID, 1, 3); err != nil {
		t.Fatal(err)
	}
	j, err := r.EnqueueDesignHTML(p.ID, 1, 1, "recover", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.ClaimDesignHTMLJob(); err != nil {
		t.Fatal(err)
	}
	if err = r.RecoverDesignHTMLJobs(); err != nil {
		t.Fatal(err)
	}
	state, _ := r.DesignHTMLState(p.ID, 1, 0, 0)
	if state.LastJob.Status != "interrupted" {
		t.Fatal("running task not interrupted")
	}
	dup, err := r.EnqueueDesignHTML(p.ID, 1, 1, "recover", "")
	if err != nil || dup.ID != j.ID {
		t.Fatal("recovery lost request key")
	}
	if _, err = r.EnqueueDesignHTML(p.ID, 1, 1, "retry", ""); err != nil {
		t.Fatal(err)
	}
	if err = r.RecoverDesignHTMLJobs(); err != nil {
		t.Fatal(err)
	}
	state, _ = r.DesignHTMLState(p.ID, 1, 0, 0)
	if state.ActiveJob.Status != "queued" {
		t.Fatal("queued task not recoverable")
	}
}

func TestDesignSharedGenerationQueueOrder(t *testing.T) {
	_, r, p := setupDesignImages(t)
	completeDesignPlans(t, r, p.ID)
	imageJob, err := r.EnqueueDesignImage(p.ID, 1, 1, "image", "")
	if err != nil {
		t.Fatal(err)
	}
	hp, err := r.SaveDesignPlan(DesignPlan{Title: "网页", OutputType: "html", Questions: []string{"标题", "内容"}, ClassIDs: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if err = r.SetActiveDesignPlan(1, hp.ID); err != nil {
		t.Fatal(err)
	}
	completeDesignPlans(t, r, hp.ID)
	time.Sleep(time.Millisecond)
	htmlJob, err := r.EnqueueDesignHTML(hp.ID, 1, 1, "html", "")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := newDesignImageService(r, t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	i, h, err := svc.claimNextGeneration()
	if err != nil || i == nil || i.ID != imageJob.ID || h != nil {
		t.Fatal("older image was not first", err)
	}
	i, h, err = svc.claimNextGeneration()
	if err != nil || h == nil || h.ID != htmlJob.ID || i != nil {
		t.Fatal("HTML queue was skipped", err)
	}
}

func TestDesignMixedWorkersShareTenSlots(t *testing.T) {
	_, r, imagePlan := setupDesignImages(t)
	htmlPlan, err := r.SaveDesignPlan(DesignPlan{Title: "交互网页", OutputType: "html", Questions: []string{"目标", "步骤"}, ClassIDs: []int{2}})
	if err != nil {
		t.Fatal(err)
	}
	if err = r.SetActiveDesignPlan(2, htmlPlan.ID); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]int{{imagePlan.ID, 1}, {htmlPlan.ID, 2}} {
		for stage := 2; stage <= 3; stage++ {
			if err = r.AdvanceDesignProgress(pair[0], pair[1], stage); err != nil {
				t.Fatal(err)
			}
		}
	}
	for n := 0; n < 20; n++ {
		cid, pid := 1, imagePlan.ID
		if n%2 == 1 {
			cid, pid = 2, htmlPlan.ID
		}
		sid := 100 + n
		if _, err = r.db.Exec(`INSERT INTO students(id,class_id,name) VALUES(?,?,?)`, sid, cid, fmt.Sprintf("并发学生%d", n)); err != nil {
			t.Fatal(err)
		}
		for version := 1; version <= 2; version++ {
			if err = r.SubmitDesignVersion(pid, cid, sid, version, []string{fmt.Sprintf("目标%d", version), "步骤"}); err != nil {
				t.Fatal(err)
			}
		}
		if cid == 1 {
			_, err = r.EnqueueDesignImage(pid, cid, sid, fmt.Sprint(n), "")
		} else {
			_, err = r.EnqueueDesignHTML(pid, cid, sid, fmt.Sprint(n), "")
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ARK_API_KEY", "mixed-image-fixture")
	t.Setenv("DEEPSEEK_KEY", "mixed-text-fixture")
	t.Setenv("GLM_KEY", "mixed-backup-fixture")
	var active, peak, total atomic.Int32
	started, release := make(chan struct{}, 20), make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	data := designTestPNG(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Messages []AIChatMessage `json:"messages"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		n := active.Add(1)
		defer active.Add(-1)
		total.Add(1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-req.Context().Done():
			return
		}
		if len(body.Messages) > 0 {
			writeDesignHTMLStream(w, designHTMLFixture)
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(data)}}})
		}
	}))
	defer upstream.Close()
	defer finish()
	h := NewHandler(r)
	h.deepSeekURL = upstream.URL
	h.glmURL = upstream.URL
	svc, err := newDesignImageService(r, t.TempDir(), upstream.URL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	svc.htmlHandler = h
	if err = svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	for n := 0; n < 10; n++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("ten tasks did not start")
		}
	}
	select {
	case <-started:
		t.Fatal("more than ten tasks ran concurrently")
	case <-time.After(100 * time.Millisecond):
	}
	if peak.Load() != 10 {
		t.Fatal("worker pool did not reach ten")
	}
	finish()
	deadline := time.Now().Add(8 * time.Second)
	for {
		var completed int
		if err = r.db.QueryRow(`SELECT (SELECT COUNT(*) FROM design_image_jobs WHERE status='succeeded')+(SELECT COUNT(*) FROM design_html_jobs WHERE status='succeeded')`).Scan(&completed); err != nil {
			t.Fatal(err)
		}
		if completed == 20 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d mixed tasks completed", completed)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if total.Load() != 20 || peak.Load() > 10 {
		t.Fatal("mixed queue exceeded its shared limit")
	}
}
