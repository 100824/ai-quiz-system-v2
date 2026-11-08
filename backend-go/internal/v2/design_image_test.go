package v2

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
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

func designTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{40, 100, 210, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func setupDesignImages(t *testing.T) (*sql.DB, *Repository, *DesignPlan) {
	t.Helper()
	db, r := setupGuidanceTestRepository(t)
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{`INSERT INTO classes(id,name) VALUES(1,'甲班'),(2,'乙班')`, `INSERT INTO students(id,class_id,name) VALUES(1,1,'学生甲'),(2,1,'学生乙'),(3,2,'学生丙')`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	p, err := r.SaveDesignPlan(DesignPlan{Title: "海报方案", Questions: []string{"海报标题", "建议"}, ClassIDs: []int{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if err = r.SetActiveDesignPlan(1, p.ID); err != nil {
		t.Fatal(err)
	}
	if err = r.SetActiveDesignPlan(2, p.ID); err != nil {
		t.Fatal(err)
	}
	return db, r, p
}
func completeDesignPlans(t *testing.T, r *Repository, p int) {
	t.Helper()
	if err := r.SubmitDesignVersion(p, 1, 1, 1, []string{"使用AI", "保护隐私"}); err != nil {
		t.Fatal(err)
	}
	if err := r.AdvanceDesignProgress(p, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := r.SubmitDesignVersion(p, 1, 1, 2, []string{"科学使用AI", "保护隐私，先思考再使用"}); err != nil {
		t.Fatal(err)
	}
	if err := r.AdvanceDesignProgress(p, 1, 3); err != nil {
		t.Fatal(err)
	}
}

func TestDesignProgressIsolationAndPersistence(t *testing.T) {
	_, r, p := setupDesignImages(t)
	if err := r.AdvanceDesignProgress(p.ID, 1, 3); err == nil {
		t.Fatal("stage skipped")
	}
	if err := r.AdvanceDesignProgress(p.ID, 1, 2); err != nil {
		t.Fatal(err)
	}
	if s, _ := r.DesignProgress(p.ID, 2); s != 1 {
		t.Fatal("cross class progression")
	}
	if err := r.AdvanceDesignProgress(p.ID, 1, 1); err == nil {
		t.Fatal("stage moved backward")
	}
	p.Title = "更新标题"
	p.ClassIDs = []int{2}
	if _, err := r.SaveDesignPlan(*p); err != nil {
		t.Fatal(err)
	}
	p.ClassIDs = []int{1, 2}
	if _, err := r.SaveDesignPlan(*p); err != nil {
		t.Fatal(err)
	}
	if s, _ := r.DesignProgress(p.ID, 1); s != 2 {
		t.Fatal("save/rebind reset progress")
	}
	if _, err := r.EnqueueDesignImage(p.ID, 1, 1, "blocked", ""); err == nil {
		t.Fatal("image before stage/versions")
	}
	// Late students still need to complete both versions even after the teacher advances.
	if err := r.AdvanceDesignProgress(p.ID, 1, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := r.EnqueueDesignImage(p.ID, 1, 2, "late", ""); err == nil {
		t.Fatal("late student skipped text versions")
	}
}

func TestActiveDesignPlanTeacherControl(t *testing.T) {
	_, repo, p := setupDesignImages(t)
	if err := repo.SubmitDesignVersion(p.ID, 1, 1, 1, []string{"原标题", "原建议"}); err != nil {
		t.Fatal(err)
	}
	other, err := repo.SaveDesignPlan(DesignPlan{Title: "另一任务", Questions: []string{"新问题"}, ClassIDs: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.SetActiveDesignPlan(1, other.ID); err != nil {
		t.Fatal(err)
	}
	if err = validateDesignStudent(repo.db, p.ID, 1, 1); err == nil {
		t.Fatal("inactive plan accessible")
	}
	if active, _ := repo.ActiveDesignPlan(2); active == nil || active.ID != p.ID {
		t.Fatal("other class was affected")
	}
	if err = repo.SetActiveDesignPlan(2, other.ID); err == nil {
		t.Fatal("unbound plan activated")
	}
	versions, err := loadDesignVersions(repo.db, p.ID, 1)
	if err != nil || len(versions) != 1 {
		t.Fatal("history lost on switch")
	}
	if err = repo.SetActiveDesignPlan(1, p.ID); err != nil {
		t.Fatal(err)
	}
	if err = validateDesignStudent(repo.db, p.ID, 1, 1); err != nil {
		t.Fatal(err)
	}
}

func TestDesignImageGenerationLifecycle(t *testing.T) {
	_, r, p := setupDesignImages(t)
	completeDesignPlans(t, r, p.ID)
	t.Setenv("ARK_API_KEY", "fixture-private-key")
	t.Setenv("ARK_KEY", "wrong-alias")
	img, _, err := image.Decode(bytes.NewReader(designTestPNG(t)))
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err = jpeg.Encode(&encoded, img, nil); err != nil {
		t.Fatal(err)
	}
	jpegData := encoded.Bytes()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if req.Header.Get("Authorization") != "Bearer fixture-private-key" {
			t.Error("wrong env precedence")
		}
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != designImageModel || body["size"] != "1K" || body["output_format"] != "jpeg" {
			t.Error("wrong model, size or output format")
		}
		if !strings.Contains(fmt.Sprint(body["prompt"]), "科学使用AI") {
			t.Error("second version missing")
		}
		if strings.Contains(fmt.Sprint(body["prompt"]), "变深") && body["image"] != "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(jpegData) {
			t.Error("missing or invalid previous JPEG image")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(jpegData)}}})
	}))
	defer upstream.Close()
	dir := t.TempDir()
	svc, err := newDesignImageService(r, dir, upstream.URL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.EnqueueDesignImage(p.ID, 1, 1, "blank-source", "", []string{"科学使用AI", "  "}); err == nil {
		t.Fatal("blank generation content accepted")
	}
	if _, err = r.EnqueueDesignImage(p.ID, 1, 1, "incomplete-source", "", []string{"科学使用AI"}); err == nil {
		t.Fatal("incomplete generation content accepted")
	}
	answers := []string{"科学使用AI（生图确认）", "保护隐私，先思考再使用"}
	j, err := r.EnqueueDesignImage(p.ID, 1, 1, "first", "", answers)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(j.Prompt, answers[0]) || !strings.Contains(j.Prompt, answers[1]) {
		t.Fatal("confirmed generation content missing from prompt")
	}
	duplicate, err := r.EnqueueDesignImage(p.ID, 1, 1, "first", "")
	if err != nil || duplicate.ID != j.ID {
		t.Fatal("duplicate was not idempotent")
	}
	if _, err = r.EnqueueDesignImage(p.ID, 1, 1, "second", "并发修改"); err == nil {
		t.Fatal("parallel task accepted")
	}
	claimed, err := r.ClaimDesignImageJob()
	if err != nil {
		t.Fatal(err)
	}
	svc.run(context.Background(), claimed)
	state, err := r.DesignImageState(p.ID, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Images) != 1 || len(state.Messages) != 2 || state.ActiveJob != nil {
		t.Fatalf("wrong state %+v", state)
	}
	if state.SourcePlan == nil || state.SourcePlan.Answers[0] != answers[0] {
		t.Fatal("confirmed generation content was not saved")
	}
	versions, err := loadDesignVersions(r.db, p.ID, 1)
	if err != nil || versions[1].Answers[0] != "科学使用AI" || versions[0].Answers[0] != "使用AI" {
		t.Fatal("generation content changed submitted versions", err)
	}
	first := state.LatestImage
	if filepath.Ext(first.FileName) != ".jpg" {
		t.Fatal("JPEG file extension missing")
	}
	if _, err = os.Stat(filepath.Join(dir, first.FileName)); err != nil {
		t.Fatal(err)
	}
	if _, err = r.EnqueueDesignImage(p.ID, 1, 1, "empty-edit", ""); err == nil {
		t.Fatal("blank image edit")
	}
	if _, err = r.EnqueueDesignImage(p.ID, 1, 1, "replace-source", "颜色变深", answers); err == nil {
		t.Fatal("source replaced after successful generation")
	}
	if _, err = r.EnqueueDesignImage(p.ID, 1, 1, "edit", "颜色变深"); err != nil {
		t.Fatal(err)
	}
	if err = r.FinalizeDesignImage(p.ID, 1, 1, first.ID); err == nil {
		t.Fatal("finalized during task")
	}
	claimed, err = r.ClaimDesignImageJob()
	if err != nil {
		t.Fatal(err)
	}
	if claimed.SourceImageID != first.ID {
		t.Fatal("wrong parent")
	}
	svc.run(context.Background(), claimed)
	state, err = r.DesignImageState(p.ID, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if state.ImageCount != 2 || state.LatestImage.ParentImageID != first.ID || calls.Load() != 2 {
		t.Fatal("edit history incorrect")
	}
	if err = r.FinalizeDesignImage(p.ID, 1, 2, first.ID); err == nil {
		t.Fatal("other student final")
	}
	if err = r.FinalizeDesignImage(p.ID, 1, 1, first.ID); err != nil {
		t.Fatal(err)
	}
	if err = r.FinalizeDesignImage(p.ID, 1, 1, state.LatestImage.ID); err == nil {
		t.Fatal("final changed")
	}
	if _, err = r.EnqueueDesignImage(p.ID, 1, 1, "after-final", "修改"); err == nil {
		t.Fatal("generated after final")
	}
	h := NewHandler(r)
	h.designImages = svc
	mux := http.NewServeMux()
	h.Register(mux)
	get := func(url string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		return w
	}
	base := fmt.Sprintf("/api/v2/design-plans/%d", p.ID)
	w := get(base + "/images/" + first.ID + "?classId=1&studentId=2")
	if w.Code != 404 {
		t.Fatal("image ownership bypass")
	}
	w = get(base + "/images/" + first.ID + "?classId=1&studentId=1&download=1")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), jpegData) || w.Header().Get("Content-Type") != "image/jpeg" || !strings.Contains(w.Header().Get("Content-Disposition"), ".jpg") {
		t.Fatal("image serving failed")
	}
	w = get(base + "/images-export")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	z, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	images := 0
	for _, f := range z.File {
		if strings.HasSuffix(f.Name, ".jpg") {
			images++
		}
		if f.Name == "成果清单.xlsx" {
			file, _ := f.Open()
			data, _ := io.ReadAll(file)
			file.Close()
			wb, err := excelize.OpenReader(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			rows, _ := wb.GetRows("图片版本")
			if len(rows) != 3 {
				t.Fatal("history missing in ZIP")
			}
			dialog, _ := wb.GetRows("对话记录")
			if len(dialog) != 5 {
				t.Fatal("dialog missing")
			}
			wb.Close()
		}
	}
	if images != 2 {
		t.Fatal("ZIP missing images")
	}
	// Missing files are explicitly reported, not silently discarded.
	if err = os.Remove(filepath.Join(dir, first.FileName)); err != nil {
		t.Fatal(err)
	}
	w = get(base + "/images-export")
	z, err = zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range z.File {
		if f.Name == "导出异常.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing-file exception omitted")
	}
}

func TestDesignImageFailureRecoveryAndPaging(t *testing.T) {
	db, r, p := setupDesignImages(t)
	completeDesignPlans(t, r, p.ID)
	j, err := r.EnqueueDesignImage(p.ID, 1, 1, "running", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.ClaimDesignImageJob(); err != nil {
		t.Fatal(err)
	}
	if err = r.RecoverDesignImageJobs(); err != nil {
		t.Fatal(err)
	}
	recovered, err := scanDesignJob(db.QueryRow(`SELECT `+designJobColumns+` FROM design_image_jobs WHERE id=?`, j.ID))
	if err != nil || recovered.Status != "interrupted" {
		t.Fatal("not recovered")
	}
	if _, err = r.ClaimDesignImageJob(); err != sql.ErrNoRows {
		t.Fatal("interrupted task retried automatically")
	}
	// New clicks use new keys; a repeat of an uncertain request returns the original state.
	old, err := r.EnqueueDesignImage(p.ID, 1, 1, "running", "")
	if err != nil || old.ID != j.ID {
		t.Fatal("recovery lost idempotency")
	}
	for n := 0; n < 25; n++ {
		job, err := r.EnqueueDesignImage(p.ID, 1, 1, fmt.Sprintf("page-%d", n), "修改")
		if err != nil {
			t.Fatal(err)
		}
		claimed, err := r.ClaimDesignImageJob()
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			if err = r.FailDesignImageJob(claimed, "服务异常，请重试。"); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err = r.CompleteDesignImageJob(job, &DesignImage{ID: designRandomID(), FileName: designRandomID() + ".png", Width: 64, Height: 96}); err != nil {
			t.Fatal(err)
		}
	}
	s, err := r.DesignImageState(p.ID, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Images) != 20 || !s.ImagesHasMore || len(s.Messages) != 40 || !s.MessagesHasMore {
		t.Fatal("paging not bounded")
	}
	older, err := r.DesignImageState(p.ID, 1, s.Images[len(s.Images)-1].Version, s.Messages[len(s.Messages)-1].ID)
	if err != nil || len(older.Images) != 4 || older.ImagesHasMore {
		t.Fatal("older history missing")
	}
}

func TestDesignImageWorkerFailureAndAlias(t *testing.T) {
	_, r, p := setupDesignImages(t)
	completeDesignPlans(t, r, p.ID)
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("ARK_KEY", "alias-test")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer alias-test" {
			t.Error("alias failed")
		}
		http.Error(w, "secret upstream details", 503)
	}))
	defer server.Close()
	svc, err := newDesignImageService(r, t.TempDir(), server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	j, err := r.EnqueueDesignImage(p.ID, 1, 1, "failed", "")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, _ := r.DesignImageState(p.ID, 1, 0, 0)
		if state != nil && len(state.Messages) == 2 {
			if state.ImageCount != 0 || strings.Contains(state.Messages[0].Content, "secret") {
				t.Fatal("failure corrupted history or leaked upstream")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("worker did not process queued job", j.ID)
}

func TestDesignImageDuplicateWorkerDoesNotInterrupt(t *testing.T) {
	_, repo, plan := setupDesignImages(t)
	completeDesignPlans(t, repo, plan.ID)
	t.Setenv("ARK_API_KEY", "duplicate-worker-fixture")
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	finish := func() { releaseOnce.Do(func() { close(release) }) }
	var calls atomic.Int32
	data := designTestPNG(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(data)}}})
	}))
	defer upstream.Close()
	defer finish()
	dir := t.TempDir()
	first, err := newDesignImageService(repo, dir, upstream.URL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err = first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err = repo.EnqueueDesignImage(plan.ID, 1, 1, "still-running", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first worker did not start generation")
	}
	duplicate, err := newDesignImageService(repo, dir, upstream.URL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err = duplicate.Start(context.Background()); err == nil {
		duplicate.Close()
		t.Fatal("duplicate worker started and can interrupt the live task")
	}
	state, err := repo.DesignImageState(plan.ID, 1, 0, 0)
	if err != nil || state.ActiveJob == nil || state.ActiveJob.Status != "running" {
		t.Fatal("duplicate startup changed the running task", err)
	}
	finish()
	deadline := time.Now().Add(5 * time.Second)
	for {
		state, err = repo.DesignImageState(plan.ID, 1, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if state.ImageCount == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("original image generation did not finish")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// A stopped owner releases its lock so a replacement can start normally.
	first.Close()
	if err = duplicate.Start(context.Background()); err != nil {
		t.Fatal("replacement could not start after owner stopped", err)
	}
	defer duplicate.Close()
	if calls.Load() != 1 {
		t.Fatal("replacement repeated the paid generation request")
	}
}

func TestDesignImageDiskRestart(t *testing.T) {
	dir := t.TempDir()
	open := func() (*sql.DB, *Repository) {
		db, err := sql.Open("sqlite3", filepath.Join(dir, "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		if err = ConfigureDB(db); err != nil {
			t.Fatal(err)
		}
		if err = InitDB(db); err != nil {
			t.Fatal(err)
		}
		return db, NewRepository(db)
	}
	db, repo := open()
	for _, stmt := range []string{`INSERT INTO classes(id,name) VALUES(1,'恢复测试班')`, `INSERT INTO students(id,class_id,name) VALUES(1,1,'甲'),(2,1,'乙')`} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	p, err := repo.SaveDesignPlan(DesignPlan{Title: "重启恢复", Questions: []string{"标题", "建议"}, ClassIDs: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.SetActiveDesignPlan(1, p.ID); err != nil {
		t.Fatal(err)
	}
	completeDesignPlans(t, repo, p.ID)
	t.Setenv("ARK_API_KEY", "disk-test")
	data := designTestPNG(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(data)}}})
	}))
	defer server.Close()
	svc, err := newDesignImageService(repo, filepath.Join(dir, "images"), server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.EnqueueDesignImage(p.ID, 1, 1, "first", ""); err != nil {
		t.Fatal(err)
	}
	j, err := repo.ClaimDesignImageJob()
	if err != nil {
		t.Fatal(err)
	}
	svc.run(context.Background(), j)
	saved, err := repo.DesignImageState(p.ID, 1, 0, 0)
	if err != nil || saved.ImageCount != 1 {
		t.Fatal("initial image missing", err)
	}
	if _, err = repo.EnqueueDesignImage(p.ID, 1, 1, "uncertain", "颜色加深"); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ClaimDesignImageJob(); err != nil {
		t.Fatal(err)
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 2, 1, []string{"标题", "建议"}); err != nil {
		t.Fatal(err)
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 2, 2, []string{"新标题", "建议"}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.EnqueueDesignImage(p.ID, 1, 2, "queued", ""); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, repo = open()
	defer db.Close()
	svc, err = newDesignImageService(repo, filepath.Join(dir, "images"), server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	otherDB, otherRepo := open()
	defer otherDB.Close()
	duplicate, err := newDesignImageService(otherRepo, filepath.Join(dir, "other-images"), server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err = duplicate.Start(context.Background()); err == nil {
		duplicate.Close()
		t.Fatal("same database accepted a worker with a different image directory")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		s, err := repo.DesignImageState(p.ID, 2, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if s.ImageCount == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("queued job not recovered")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if calls.Load() != 2 {
		t.Fatal("uncertain running job was automatically retried")
	}
	restored, err := repo.DesignImageState(p.ID, 1, 0, 0)
	if err != nil || restored.ImageCount != 1 || restored.ActiveJob != nil {
		t.Fatal("persisted state lost", err)
	}
	if bytes, err := os.ReadFile(filepath.Join(dir, "images", restored.LatestImage.FileName)); err != nil || len(bytes) == 0 {
		t.Fatal("persisted image lost")
	}
	if stage, _ := repo.DesignProgress(p.ID, 1); stage != 3 {
		t.Fatal("persisted progress lost")
	}
	if err = repo.FinalizeDesignImage(p.ID, 1, 1, restored.LatestImage.ID); err != nil {
		t.Fatal(err)
	}
}
