package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamDeepSeekConcatenatesContentAndIgnoresReasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Stream   bool                     `json:"stream"`
			Thinking map[string]string        `json:"thinking"`
			Messages []map[string]interface{} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !request.Stream || request.Thinking["type"] != "disabled" || len(request.Messages) != 2 {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"hidden\",\"content\":\"你好\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"\\n**世界**\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	t.Setenv("DEEPSEEK_KEY", "test-key")
	handler := NewHandler(nil)
	handler.deepSeekURL = server.URL
	handler.httpClient = server.Client()
	var deltas []string
	answer, err := handler.streamDeepSeek(context.Background(), "system", []AIChatMessage{{Role: "user", Content: "question"}}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("streamDeepSeek() error = %v", err)
	}
	if answer != "你好\n**世界**" {
		t.Fatalf("answer = %q, want %q", answer, "你好\n**世界**")
	}
	if strings.Join(deltas, "") != answer {
		t.Fatalf("deltas = %q, want %q", strings.Join(deltas, ""), answer)
	}
}

func TestStreamDeepSeekDoesNotTruncateLongOutput(t *testing.T) {
	const unit = "这是完整的AI回复。"
	longOutput := strings.Repeat(unit, 120)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", mustJSONStreamChunk(longOutput))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	t.Setenv("DEEPSEEK_KEY", "test-key")
	handler := NewHandler(nil)
	handler.deepSeekURL = server.URL
	handler.httpClient = server.Client()
	answer, err := handler.streamDeepSeek(context.Background(), "system", []AIChatMessage{{Role: "user", Content: "question"}}, nil)
	if err != nil {
		t.Fatalf("streamDeepSeek() error = %v", err)
	}
	if answer != longOutput {
		t.Fatalf("answer length = %d, want %d", len([]rune(answer)), len([]rune(longOutput)))
	}
}

func mustJSONStreamChunk(content string) string {
	payload, err := json.Marshal(map[string]interface{}{
		"choices": []map[string]interface{}{{
			"delta": map[string]string{"content": content},
		}},
	})
	if err != nil {
		panic(err)
	}
	return string(payload)
}
