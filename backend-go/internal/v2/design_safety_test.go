package v2

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDesignTextBoundarySafety(t *testing.T) {
	for _, text := range []string{"中文标点！English ' \" <script>SQL; DROP TABLE students</script>", "小猫👨‍👩‍👧‍👦🧑🏽‍🚀", "第一行\r\n第二行\t第三行", strings.Repeat("字", designAnswerLimit)} {
		if _, err := normalizeDesignText(text, "答案", designAnswerLimit, true); err != nil {
			t.Fatalf("valid input rejected: %v", err)
		}
	}
	for _, text := range []string{" \n\t", "\u200d\ufe0f", "\u0301", "a\x00b", "a\x1bb", "a\u0085b", "a\u202eb", "a\u2066b", "a\u200bb", string([]byte{0xff}), "\ufffd", strings.Repeat("字", designAnswerLimit+1)} {
		if _, err := normalizeDesignText(text, "答案", designAnswerLimit, true); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if designComparable("小猫\u200d\ufe0f") != designComparable("小猫") {
		t.Fatal("invisible characters counted as optimization")
	}
	if _, err := normalizeDesignAnswers([]string{strings.Repeat("a", 4000), strings.Repeat("b", 4000), strings.Repeat("c", 4000), "d"}); err == nil {
		t.Fatal("aggregate answers exceeded limit")
	}
	_, repo, p := setupDesignImages(t)
	for _, answers := range [][]string{{strings.Repeat("x", 4001), "正常"}, {"\u200d", "正常"}, {"a\x00", "正常"}} {
		if err := repo.SubmitDesignVersion(p.ID, 1, 1, 1, answers); err == nil {
			t.Fatal("invalid first version saved")
		}
	}
	versions, _ := loadDesignVersions(repo.db, p.ID, 1)
	if len(versions) != 0 {
		t.Fatal("invalid answers partially saved")
	}
	badPlan := *p
	badPlan.Title = strings.Repeat("题", designTitleLimit+1)
	if _, err := repo.SaveDesignPlan(badPlan); err == nil {
		t.Fatal("oversize title saved")
	}
	badPlan = *p
	badPlan.Questions = make([]string, designQuestionCount+1)
	if _, err := repo.SaveDesignPlan(badPlan); err == nil {
		t.Fatal("too many questions saved")
	}
	badPlan = *p
	badPlan.Questions = []string{strings.Repeat("题", designQuestionLimit+1)}
	if _, err := repo.SaveDesignPlan(badPlan); err == nil {
		t.Fatal("oversize question saved")
	}
	badPlan = *p
	badPlan.ClassIDs = make([]int, 201)
	if _, err := repo.SaveDesignPlan(badPlan); err == nil {
		t.Fatal("unbounded binding list accepted")
	}
}

func TestDesignStrictJSONAndEnqueueGuards(t *testing.T) {
	for _, mode := range []string{"image", "html"} {
		t.Run(mode, func(t *testing.T) {
			_, repo, p := setupDesignImages(t)
			p.OutputType = mode
			if _, err := repo.SaveDesignPlan(*p); err != nil {
				t.Fatal(err)
			}
			completeDesignPlans(t, repo, p.ID)
			t.Setenv("ARK_API_KEY", "fixture")
			t.Setenv("DEEPSEEK_KEY", "fixture")
			h := NewHandler(repo)
			h.designImages, _ = newDesignImageService(repo, t.TempDir(), "", nil)
			mux := http.NewServeMux()
			h.Register(mux)
			path := fmt.Sprintf("/api/v2/design-plans/%d/%s-jobs", p.ID, mode)
			valid := `{"classId":1,"studentId":1,"requestKey":"safe","message":"正常"}`
			for _, body := range []string{valid + ` {}`, valid + strings.Repeat(" ", designRequestLimit), strings.Replace(valid, "正常", string([]byte{0xff}), 1), strings.Replace(valid, "正常", `\ud800`, 1), strings.Replace(valid, "正常", `\u0000`, 1), strings.Replace(valid, "safe", `bad\nkey`, 1), strings.Replace(valid, "正常", strings.Repeat("字", 4001), 1), `{"classId":1,"studentId":1,"requestKey":"bad-answer","answers":["` + strings.Repeat("字", 4001) + `","正常"]}`} {
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
				if w.Code != 400 {
					t.Fatalf("bad input accepted: %d", w.Code)
				}
			}
			var jobs int
			repo.db.QueryRow(`SELECT (SELECT COUNT(*) FROM design_image_jobs)+(SELECT COUNT(*) FROM design_html_jobs)`).Scan(&jobs)
			if jobs != 0 {
				t.Fatal("invalid input created tasks")
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(valid)))
			if w.Code != 200 {
				t.Fatal("normal generation rejected", w.Body.String())
			}
		})
	}
}

func imageResponse(data []byte) []byte {
	b, _ := json.Marshal(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(data)}}})
	return b
}

func TestDesignImageResponseLimitsAndCorruption(t *testing.T) {
	png := designTestPNG(t)
	img, _, _ := image.Decode(bytes.NewReader(png))
	var buffer bytes.Buffer
	jpeg.Encode(&buffer, img, nil)
	for _, data := range [][]byte{png, buffer.Bytes()} {
		if _, _, _, err := decodeDesignImageResponse(bytes.NewReader(imageResponse(data))); err != nil {
			t.Fatal("valid image rejected", err)
		}
	}
	// IHDR with an enormous pixel count but a valid checksum must be rejected
	// before a decoder allocates the advertised image buffer.
	bomb := append([]byte{}, png...)
	binary.BigEndian.PutUint32(bomb[16:20], 100000)
	binary.BigEndian.PutUint32(bomb[29:33], crc32.ChecksumIEEE(bomb[12:29]))
	for _, body := range [][]byte{imageResponse(png[:len(png)/2]), imageResponse(buffer.Bytes()[:buffer.Len()/2]), imageResponse(bomb), imageResponse(make([]byte, designImageLimit+1)), []byte(`{"data":[{"b64_json":"bad!"}]}`), []byte(`{"data":[]}`), append(imageResponse(png), []byte(`{}`)...), append(imageResponse(png), bytes.Repeat([]byte(" "), designImageResponseLimit)...)} {
		if _, _, _, err := decodeDesignImageResponse(bytes.NewReader(body)); err == nil {
			t.Fatal("oversize/corrupt response accepted")
		}
	}
}

func TestDesignHTMLComplexityAndUnsafeOutput(t *testing.T) {
	for _, body := range []string{
		strings.Replace(designHTMLFixture, "</body>", strings.Repeat("<div>", 140)+strings.Repeat("</div>", 140)+"</body>", 1),
		strings.Replace(designHTMLFixture, "</body>", strings.Repeat("<span>x</span>", 6000)+"</body>", 1),
		strings.Replace(designHTMLFixture, "let n=0;", "while(true){}", 1),
		strings.Replace(designHTMLFixture, "let n=0;", "for(;;){}", 1),
		strings.Replace(designHTMLFixture, "let n=0;", "eval('x');", 1),
		strings.Replace(designHTMLFixture, "let n=0;", "window.open('x');", 1),
		strings.Replace(designHTMLFixture, "let n=0;", strings.Repeat(" ", 256<<10), 1),
		strings.Replace(designHTMLFixture, "</body>", "\x00</body>", 1),
		strings.Repeat("x", designHTMLLimit+1),
	} {
		if _, err := validateDesignHTML(body); err == nil {
			t.Fatal("complex or unsafe HTML accepted")
		}
	}
	if _, err := validateDesignHTML(designHTMLFixture); err != nil {
		t.Fatal("offline interaction rejected", err)
	}
	if _, err := validateDesignHTML(strings.Replace(designHTMLFixture, "()=>", "function()", 1)); err != nil {
		t.Fatal("ordinary anonymous event handler rejected", err)
	}
}

func TestDesignGenerationRateBudgetAndIdempotency(t *testing.T) {
	_, repo, p := setupDesignImages(t)
	completeDesignPlans(t, repo, p.ID)
	var first *DesignImageJob
	for n := 0; n < designJobsPerMinute; n++ {
		j, err := repo.EnqueueDesignImage(p.ID, 1, 1, fmt.Sprintf("burst-%d", n), "")
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			first = j
		}
		j, err = repo.ClaimDesignImageJob()
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.FailDesignImageJob(j, "mock failure"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.EnqueueDesignImage(p.ID, 1, 1, "too-fast", ""); err == nil {
		t.Fatal("rate limit bypassed")
	}
	if j, err := repo.EnqueueDesignImage(p.ID, 1, 1, first.RequestKey, ""); err != nil || j.ID != first.ID {
		t.Fatal("idempotent read blocked by rate limit", err)
	}
	// No total-version cap: a later genuine request is allowed.
	repo.db.Exec(`UPDATE design_image_jobs SET created_at=?`, time.Now().UTC().Add(-2*time.Minute).Format(time.RFC3339Nano))
	if _, err := repo.EnqueueDesignImage(p.ID, 1, 1, "later", ""); err != nil {
		t.Fatal(err)
	}
}

func TestDesignMixedQueueAdmissionLimits(t *testing.T) {
	_, repo, p := setupDesignImages(t)
	completeDesignPlans(t, repo, p.ID)
	if _, err := repo.EnqueueDesignImage(p.ID, 1, 1, "original", ""); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < designPendingLimit-1; n++ {
		sid := 100 + n
		if _, err := repo.db.Exec(`INSERT INTO students(id,class_id,name)VALUES(?,1,?)`, sid, fmt.Sprintf("队列验收%d", n)); err != nil {
			t.Fatal(err)
		}
		table := "image"
		if n%2 == 0 {
			table = "html"
		}
		res, err := repo.db.Exec(`INSERT INTO design_`+table+`_sessions(plan_id,class_id,student_id,snapshot_json)VALUES(?,1,?,'{}')`, p.ID, sid)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if _, err = repo.db.Exec(`INSERT INTO design_`+table+`_jobs(id,session_id,request_key,message,prompt,status,created_at,updated_at)VALUES(?,?,'queue-fixture','','','queued',?,?)`, fmt.Sprintf("queue-%d", n), id, designNow(), designNow()); err != nil {
			t.Fatal(err)
		}
	}
	tx, _ := repo.db.Begin()
	err := checkDesignGenerationBudget(tx, 2)
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "任务较多") {
		t.Fatal("mixed queue cap bypassed", err)
	}
	if _, err := repo.EnqueueDesignImage(p.ID, 1, 1, "original", ""); err != nil {
		t.Fatal("full queue blocked idempotent request", err)
	}
	// Failed jobs from both modes share the same student's rolling budget.
	repo.db.Exec(`UPDATE design_image_jobs SET status='failed'`)
	repo.db.Exec(`UPDATE design_html_jobs SET status='failed'`)
	res, _ := repo.db.Exec(`INSERT INTO design_html_sessions(plan_id,class_id,student_id,snapshot_json)VALUES(?,1,2,'{}')`, p.ID)
	session, _ := res.LastInsertId()
	for n := 0; n < designJobsPerMinute; n++ {
		repo.db.Exec(`INSERT INTO design_html_jobs(id,session_id,request_key,message,prompt,status,created_at,updated_at)VALUES(?,?,'rate'||?,'','','failed',?,?)`, fmt.Sprintf("rate-%d", n), session, n, designNow(), designNow())
	}
	tx, _ = repo.db.Begin()
	err = checkDesignGenerationBudget(tx, 2)
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "频繁") {
		t.Fatal("HTML retries bypassed shared rate limit", err)
	}
}

type designPanicTransport struct{}

func (designPanicTransport) RoundTrip(*http.Request) (*http.Response, error) {
	panic("fixture panic must not kill server")
}

func TestDesignWorkerPanicLeavesControlledFailure(t *testing.T) {
	for _, mode := range []string{"image", "html"} {
		t.Run(mode, func(t *testing.T) {
			_, repo, p := setupDesignImages(t)
			p.OutputType = mode
			repo.SaveDesignPlan(*p)
			completeDesignPlans(t, repo, p.ID)
			t.Setenv("ARK_API_KEY", "fixture")
			t.Setenv("DEEPSEEK_KEY", "fixture")
			svc, _ := newDesignImageService(repo, t.TempDir(), "http://fixture", &http.Client{Transport: designPanicTransport{}})
			if err := os.MkdirAll(svc.htmlDir, 0700); err != nil {
				t.Fatal(err)
			}
			if mode == "image" {
				repo.EnqueueDesignImage(p.ID, 1, 1, "panic", "")
				j, _ := repo.ClaimDesignImageJob()
				svc.run(context.Background(), j)
				state, _ := repo.DesignImageState(p.ID, 1, 0, 0)
				if state.LastJob.Status != "failed" || state.ActiveJob != nil || state.ImageCount != 0 {
					t.Fatal("panic left running image job")
				}
			} else {
				repo.EnqueueDesignHTML(p.ID, 1, 1, "panic", "")
				j, _ := repo.ClaimDesignHTMLJob()
				h := NewHandler(repo)
				h.httpClient = svc.client
				svc.htmlHandler = h
				svc.runHTML(context.Background(), j)
				state, _ := repo.DesignHTMLState(p.ID, 1, 0, 0)
				if state.LastJob.Status != "failed" || state.ActiveJob != nil || state.VersionCount != 0 {
					t.Fatal("panic left running HTML job")
				}
			}
		})
	}
}

func TestDesignArtifactReadAndStreamCancellation(t *testing.T) {
	_, repo, p := setupDesignImages(t)
	completeDesignPlans(t, repo, p.ID)
	svc, _ := newDesignImageService(repo, t.TempDir(), "", nil)
	name := designRandomID() + ".png"
	file := filepath.Join(svc.dir, name)
	if err := os.WriteFile(file, designTestPNG(t), 0600); err != nil {
		t.Fatal(err)
	}
	repo.db.Exec(`INSERT INTO design_image_sessions(plan_id,class_id,student_id,snapshot_json)VALUES(?,1,1,'{}')`, p.ID)
	repo.db.Exec(`INSERT INTO design_image_jobs(id,session_id,request_key,message,prompt,status,created_at,updated_at)VALUES('old',1,'old','','','succeeded','x','x')`)
	repo.db.Exec(`INSERT INTO design_images(id,session_id,job_id,version,file_name,width,height,model,created_at)VALUES('reference',1,'old',1,?,64,96,'x','x')`, name)
	ctx, cancel := context.WithCancel(context.Background())
	body, err := svc.imageRequestBody(ctx, &DesignImageJob{SessionID: 1, SourceImageID: "reference", Prompt: "修改图片"})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	done := make(chan error, 1)
	go func() { _, e := io.Copy(io.Discard, body); done <- e }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled body succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled upload leaked")
	}
	body.Close()
	huge := filepath.Join(t.TempDir(), "huge.html")
	f, _ := os.Create(huge)
	f.Truncate(designHTMLLimit + 1)
	f.Close()
	if _, err := readDesignArtifact(huge, designHTMLLimit); err == nil {
		t.Fatal("oversize disk file loaded")
	}
	if err := checkDesignStorage(t.TempDir(), ^uint64(0)); err == nil {
		t.Fatal("disk capacity check bypassed")
	}
}

func TestDesignHTMLStreamSafetyLimits(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		switch r.URL.Path {
		case "/content":
			writeDesignHTMLStream(w, strings.Repeat("x", designHTMLLimit+1))
		case "/reasoning":
			b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"reasoning_content": strings.Repeat("x", (256<<10)+1)}}}})
			fmt.Fprintf(w, "data: %s\n\n", b)
			writeDesignHTMLStream(w, designHTMLFixture)
		case "/wire":
			for n := 0; n < 2100; n++ {
				if _, err := io.WriteString(w, ":"+strings.Repeat("x", 4096)+"\n\n"); err != nil {
					return
				}
			}
			writeDesignHTMLStream(w, designHTMLFixture)
		case "/truncated":
			fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"partial"}}]}`+"\n\n")
		case "/length":
			fmt.Fprint(w, `data: {"choices":[{"finish_reason":"length","delta":{"content":"partial"}}]}`+"\n\ndata: [DONE]\n\n")
		}
	}))
	defer upstream.Close()
	h := NewHandler(nil)
	for _, path := range []string{"/content", "/reasoning", "/wire", "/truncated", "/length"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := h.streamProviderOutput(ctx, aiProvider{ID: "deepseek", Model: "fixture", url: upstream.URL + path, key: "fixture"}, "fixture", nil, nil, func(string) error { return nil }, 16384, designHTMLLimit)
		cancel()
		if err == nil {
			t.Fatal("unsafe response accepted", path)
		}
	}
}

func TestDesignOversizeHTMLKeepsPreviousWork(t *testing.T) {
	_, repo, p := setupDesignImages(t)
	p.OutputType = "html"
	repo.SaveDesignPlan(*p)
	completeDesignPlans(t, repo, p.ID)
	t.Setenv("DEEPSEEK_KEY", "fixture")
	t.Setenv("GLM_KEY", "fixture")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeDesignHTMLStream(w, strings.Repeat("x", designHTMLLimit+1))
	}))
	defer upstream.Close()
	svc, _ := newDesignImageService(repo, t.TempDir(), "", upstream.Client())
	os.MkdirAll(svc.htmlDir, 0700)
	h := NewHandler(repo)
	h.deepSeekURL = upstream.URL
	h.glmURL = upstream.URL
	svc.htmlHandler = h
	repo.EnqueueDesignHTML(p.ID, 1, 1, "first", "")
	first, _ := repo.ClaimDesignHTMLJob()
	v := &DesignHTMLVersion{ID: designRandomID(), Model: "fixture", SizeBytes: len(designHTMLFixture)}
	v.FileName = v.ID + ".html"
	if err := writeDesignHTMLFile(svc.htmlDir, v.FileName, []byte(designHTMLFixture)); err != nil {
		t.Fatal(err)
	}
	if err := repo.CompleteDesignHTMLJob(first, v); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.EnqueueDesignHTML(p.ID, 1, 1, "oversize", "调整页面"); err != nil {
		t.Fatal(err)
	}
	job, _ := repo.ClaimDesignHTMLJob()
	svc.runHTML(context.Background(), job)
	state, err := repo.DesignHTMLState(p.ID, 1, 0, 0)
	if err != nil || state.ActiveJob != nil || state.VersionCount != 1 || state.LatestVersion.ID != v.ID || state.LastJob.Status != "failed" || !strings.Contains(state.LastJob.Error, "512KiB") {
		t.Fatalf("oversize result replaced previous work: %+v %v", state, err)
	}
	files, _ := os.ReadDir(svc.htmlDir)
	if len(files) != 1 {
		t.Fatal("oversize output left orphan files")
	}
}

func FuzzDesignText(f *testing.F) {
	for _, s := range []string{"正常文本👨‍👩‍👧‍👦", "\x00", "\u202e", "<script>", string([]byte{0xff})} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { _, _ = normalizeDesignText(s, "内容", designAnswerLimit, true) })
}
func FuzzDesignHTML(f *testing.F) {
	for _, s := range []string{designHTMLFixture, "<div>", "<!doctype html><html><head></head><body>\x00</body></html>"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { _, _ = validateDesignHTML(s) })
}
