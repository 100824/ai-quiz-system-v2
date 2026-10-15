package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func configureTestAIKeys(t *testing.T) {
	t.Helper()
	for _, key := range []string{"DEEPSEEK_KEY", "DEEPSEEK_API_KEY", "GLM_KEY", "GLM_API_KEY", "DEEPSEEK_MODEL", "GLM_MODEL"} {
		t.Setenv(key, "")
	}
	t.Setenv("DEEPSEEK_KEY", "secret-ds")
	t.Setenv("GLM_KEY", "secret-glm")
}

func writeTestAIAnswer(w http.ResponseWriter, answer string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", mustJSONStreamChunk(answer))
}

func TestAIProviderFallbackFailures(t *testing.T) {
	for _, scenario := range []string{"http", "empty", "malformed", "partial", "stream-error", "timeout"} {
		t.Run(scenario, func(t *testing.T) {
			configureTestAIKeys(t)
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch scenario {
				case "http":
					http.Error(w, "outage", http.StatusServiceUnavailable)
				case "empty":
					fmt.Fprint(w, "data: [DONE]\n\n")
				case "malformed":
					fmt.Fprint(w, "data: not-json\n\n")
				case "partial":
					fmt.Fprintf(w, "data: %s\n\n", mustJSONStreamChunk("unfinished"))
				case "stream-error":
					fmt.Fprint(w, `data: {"error":{"code":"server_error"}}`+"\n\n")
				case "timeout":
					select {
					case <-r.Context().Done():
					case <-time.After(300 * time.Millisecond):
					}
				}
			}))
			defer primary.Close()
			fallbackCalls := 0
			fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fallbackCalls++
				if r.Header.Get("Authorization") != "Bearer secret-glm" {
					t.Error("wrong GLM auth")
				}
				var payload struct {
					Model    string          `json:"model"`
					Messages []AIChatMessage `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload.Model != "glm-5.3-flash" || len(payload.Messages) != 2 || payload.Messages[1].Content != "question" {
					t.Errorf("wrong fallback context: %+v", payload)
				}
				writeTestAIAnswer(w, "backup answer")
			}))
			defer fallback.Close()
			h := NewHandler(nil)
			h.deepSeekURL, h.glmURL = primary.URL, fallback.URL
			h.aiProviderTimeout = 100 * time.Millisecond
			var visible string
			resets := 0
			answer, model, err := h.streamAI(context.Background(), "system", []AIChatMessage{{Role: "user", Content: "question"}}, func(delta string) error { visible += delta; return nil }, func() error { visible = ""; resets++; return nil })
			if err != nil || answer != "backup answer" || model != "glm-5.3-flash" || visible != answer || fallbackCalls != 1 {
				t.Fatalf("fallback: %q %q %q %v calls=%d", answer, model, visible, err, fallbackCalls)
			}
			if scenario == "partial" && resets != 1 {
				t.Fatalf("partial output was not reset")
			}
		})
	}
}

func TestAISettingsPersistAndReverseFallback(t *testing.T) {
	configureTestAIKeys(t)
	db, repo := setupGuidanceTestRepository(t)
	defer db.Close()
	h := NewHandler(repo)
	if provider, err := repo.GetPrimaryAIProvider(); err != nil || provider != "deepseek" {
		t.Fatalf("default primary must remain DeepSeek: %q %v", provider, err)
	}
	t.Setenv("GLM_MODEL", "ignored-old-model-setting")
	if model := h.aiProviders()[1].Model; model != "glm-5.3-flash" {
		t.Fatalf("GLM model must be fixed: %s", model)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v2/ai-settings", nil))
	if get.Code != http.StatusOK || strings.Contains(get.Body.String(), "secret-") {
		t.Fatalf("invalid or leaked settings: %s", get.Body.String())
	}
	for _, value := range []string{"glm", "unknown"} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/v2/ai-settings", strings.NewReader(`{"primaryProvider":"`+value+`"}`)))
		want := http.StatusOK
		if value == "unknown" {
			want = http.StatusBadRequest
		}
		if response.Code != want {
			t.Fatalf("settings status=%d want=%d", response.Code, want)
		}
	}
	if provider, err := NewHandler(repo).repo.GetPrimaryAIProvider(); err != nil || provider != "glm" {
		t.Fatalf("primary not persisted: %q %v", provider, err)
	}
	glmCalls, dsCalls := 0, 0
	glm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { glmCalls++; http.Error(w, "outage", 500) }))
	defer glm.Close()
	ds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { dsCalls++; writeTestAIAnswer(w, "ds backup") }))
	defer ds.Close()
	h.glmURL, h.deepSeekURL = glm.URL, ds.URL
	answer, model, err := h.streamAI(context.Background(), "system", nil, nil, nil)
	if err != nil || answer != "ds backup" || model != aiGuidanceModel || glmCalls != 1 || dsCalls != 1 {
		t.Fatalf("reverse fallback failed: %s %s %v", answer, model, err)
	}
	t.Setenv("GLM_KEY", "")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/v2/ai-settings", strings.NewReader(`{"primaryProvider":"glm"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatal("allowed unconfigured primary")
	}
}

func TestAIStopsOnCancelOrClientWriteFailure(t *testing.T) {
	configureTestAIKeys(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeTestAIAnswer(w, "answer") }))
	defer server.Close()
	backupCalls := 0
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { backupCalls++; writeTestAIAnswer(w, "backup") }))
	defer backup.Close()
	h := NewHandler(nil)
	h.deepSeekURL, h.glmURL = server.URL, backup.URL
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := h.streamAI(ctx, "", nil, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	_, _, err := h.streamAI(context.Background(), "", nil, func(string) error { return errors.New("client disconnected") }, nil)
	if err == nil || backupCalls != 0 {
		t.Fatal("retried after client disconnected")
	}
}

func TestAIBothProvidersFailAndMissingPrimaryKey(t *testing.T) {
	configureTestAIKeys(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "secret-upstream", 500) }))
	defer server.Close()
	h := NewHandler(nil)
	h.deepSeekURL, h.glmURL = server.URL, server.URL
	_, _, err := h.streamAI(context.Background(), "", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "DeepSeek") || !strings.Contains(err.Error(), "GLM") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("bad final error: %v", err)
	}
	t.Setenv("DEEPSEEK_KEY", "")
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeTestAIAnswer(w, "glm only") }))
	defer fallback.Close()
	h.glmURL = fallback.URL
	if answer, _, err := h.streamAI(context.Background(), "", nil, nil, nil); err != nil || answer != "glm only" {
		t.Fatalf("missing key fallback: %q %v", answer, err)
	}
}
