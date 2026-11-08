package v2

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Opt-in real HTTP fixture for frontend/tests/design-real.test.mjs.
// Only an in-memory database is used; no application files or model APIs are accessed.
func TestDesignBrowserServer(t *testing.T) {
	if os.Getenv("DESIGN_BROWSER_TEST") != "1" {
		t.Skip("browser fixture disabled")
	}
	db, repo := setupGuidanceTestRepository(t)
	defer db.Close()
	for _, stmt := range []string{
		`INSERT INTO classes(id,name) VALUES(1,'验收甲班'),(2,'验收乙班')`,
		`INSERT INTO students(id,class_id,name) VALUES(1,1,'学生甲'),(2,1,'学生乙'),(3,2,'学生丙')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	t.Setenv("ARK_API_KEY", "browser-fixture-key")
	t.Setenv("DEEPSEEK_KEY", "browser-text-fixture-key")
	t.Setenv("GLM_KEY", "browser-text-fixture-glm")
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Prompt   string          `json:"prompt"`
			Messages []AIChatMessage `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Messages) > 0 {
			last := body.Messages[len(body.Messages)-1].Content
			if strings.Contains(last, "模拟失败") {
				http.Error(w, "fixture failure", 503)
				return
			}
			code := designHTMLFixture
			if len(body.Messages) > 2 {
				code = strings.ReplaceAll(code, "绿色校园", "蓝色校园")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"先设计布局与按钮交互。<img src=x onerror=window.reasoningXSS=1>\"}}]}\n\n")
			w.(http.Flusher).Flush()
			if len(body.Messages) == 2 {
				time.Sleep(6500 * time.Millisecond)
			}
			writeDesignHTMLStream(w, code)
			return
		}
		time.Sleep(120 * time.Millisecond)
		if strings.Contains(body.Prompt, "模拟失败") {
			http.Error(w, "fixture failure", 503)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(designTestPNG(t))}}})
	}))
	defer upstream.Close()
	service, err := newDesignImageService(repo, t.TempDir(), upstream.URL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(repo)
	h.deepSeekURL = upstream.URL
	h.glmURL = upstream.URL
	service.htmlHandler = h
	if err = service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	h.designImages = service
	h.Register(mux)
	mux.HandleFunc("GET /__test/image-calls", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]int32{"calls": calls.Load()})
	})
	stopped := make(chan struct{})
	var once sync.Once
	mux.HandleFunc("POST /__test/stop", func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(stopped) })
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	fmt.Println("DESIGN_BROWSER_URL=" + server.URL)
	select {
	case <-stopped:
	case <-time.After(4 * time.Minute):
		t.Fatal("browser fixture timed out")
	}
}
