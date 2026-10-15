package v2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt-in only: sends synthetic classroom data to the real GLM API. Never
// embeds credentials or writes to the application's on-disk database.
func TestGLMLiveClassroomWorkflow(t *testing.T) {
	if os.Getenv("AI_LIVE_TEST") != "1" {
		t.Skip("set AI_LIVE_TEST=1 and GLM_KEY to run real-provider validation")
	}
	if envFirst("GLM_KEY", "GLM_API_KEY") == "" {
		t.Fatal("GLM key is required")
	}
	t.Setenv("DEEPSEEK_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "")
	db, repo := setupGuidanceTestRepository(t)
	defer db.Close()
	h := NewHandler(repo)
	mux := http.NewServeMux()
	h.Register(mux)
	classID, err := repo.CreateClass("合成测试班", "")
	if err != nil {
		t.Fatal(err)
	}
	studentID, err := repo.CreateStudent(classID, "合成测试学生", "")
	if err != nil {
		t.Fatal(err)
	}
	courseID, err := repo.CreateCourse("GLM 集成测试", "", "认识人工智能的用途、局限及训练数据", "reflection")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.BindCourseClass(courseID, classID); err != nil {
		t.Fatal(err)
	}
	sections, err := repo.ListSections(courseID)
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{0, 3} {
		if _, err := repo.SetSectionAIGuidance(sections[index].ID, true); err != nil {
			t.Fatal(err)
		}
	}
	questionID, err := repo.CreateQuestion(Question{SectionID: sections[1].ID, Type: "ai_chat", Title: "人工智能如何学习？"})
	if err != nil {
		t.Fatal(err)
	}
	request := map[string]interface{}{"courseId": courseID, "classId": classID, "studentId": studentID, "sectionId": sections[1].ID, "questionId": questionID, "message": "用两句话向五年级学生解释人工智能如何从数据中学习。"}
	if !t.Run("teacher_selects_glm", func(t *testing.T) {
		body := strings.NewReader(`{"primaryProvider":"glm"}`)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v2/ai-settings", body))
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), envFirst("GLM_KEY", "GLM_API_KEY")) {
			t.Fatalf("invalid settings response: status=%d", w.Code)
		}
		if primary, err := NewHandler(repo).repo.GetPrimaryAIProvider(); err != nil || primary != "glm" {
			t.Fatalf("primary not persisted: %s %v", primary, err)
		}
	}) {
		return
	}
	var first map[string]json.RawMessage
	if !t.Run("real_glm_streaming_chat", func(t *testing.T) { first = liveAIStream(t, mux, "/api/v2/student/ai-chat", request, false) }) {
		return
	}
	if !t.Run("real_glm_chat_follow_up", func(t *testing.T) {
		var messages []AIChatMessage
		if err := json.Unmarshal(first["messages"], &messages); err != nil {
			t.Fatal(err)
		}
		request["messages"], request["message"] = messages, "请补充一个生活中的例子，用两句话回答。"
		done := liveAIStream(t, mux, "/api/v2/student/ai-chat", request, false)
		if string(done["rounds"]) != "2" {
			t.Fatalf("wrong chat rounds: %s", done["rounds"])
		}
	}) {
		return
	}
	// A real GLM response must replace, rather than append to, failed primary output.
	failedPrimary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", mustJSONStreamChunk("FAILED_PRIMARY_PARTIAL"))
	}))
	defer failedPrimary.Close()
	h.deepSeekURL = failedPrimary.URL
	t.Setenv("DEEPSEEK_KEY", "synthetic-mock-key")
	if err := repo.SetPrimaryAIProvider("deepseek"); err != nil {
		t.Fatal(err)
	}
	if !t.Run("failed_primary_to_real_glm_with_reset", func(t *testing.T) {
		liveAIStream(t, mux, "/api/v2/student/ai-chat", request, true)
	}) {
		return
	}
	if err := repo.SetPrimaryAIProvider("glm"); err != nil {
		t.Fatal(err)
	}
	predictionQuestions, err := repo.ListQuestions(sections[0].ID)
	if err != nil || len(predictionQuestions) == 0 {
		t.Fatalf("prediction setup: %v", err)
	}
	if _, err := repo.SubmitSection(courseID, classID, studentID, sections[0].ID, map[string]json.RawMessage{stringID(predictionQuestions[0].ID): json.RawMessage(`"3"`)}); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []struct {
		name    string
		section int
	}{{"learning_plan", 0}, {"evaluation", 3}} {
		if phase.section == 3 {
			questions, err := repo.ListQuestions(sections[2].ID)
			if err != nil {
				t.Fatal(err)
			}
			answers := map[string]json.RawMessage{}
			for _, question := range questions {
				answers[stringID(question.ID)] = json.RawMessage(`"A"`)
			}
			if _, err := repo.SubmitSection(courseID, classID, studentID, sections[2].ID, answers); err != nil {
				t.Fatal(err)
			}
		}
		identity := map[string]interface{}{"courseId": courseID, "classId": classID, "studentId": studentID, "sectionId": sections[phase.section].ID}
		var session AIGuidanceSession
		if !t.Run("real_glm_"+phase.name, func(t *testing.T) {
			done := liveAIStream(t, mux, "/api/v2/student/ai-guidance/generate", identity, false)
			if err := json.Unmarshal(done["session"], &session); err != nil {
				t.Fatal(err)
			}
			if session.Model != h.aiProviders()[1].Model || session.Status != "ready" || len(session.Messages) != 1 {
				t.Fatalf("invalid saved session: model=%s status=%s messages=%d", session.Model, session.Status, len(session.Messages))
			}
		}) {
			return
		}
		identity["sessionId"], identity["message"] = session.ID, "请用两句话告诉我下一步先做什么。"
		if !t.Run("real_glm_"+phase.name+"_follow_up", func(t *testing.T) {
			done := liveAIStream(t, mux, "/api/v2/student/ai-guidance/messages", identity, false)
			if err := json.Unmarshal(done["session"], &session); err != nil {
				t.Fatal(err)
			}
			if len(session.Messages) != 3 || session.FollowUpRounds != 1 {
				t.Fatalf("wrong saved exchange count: messages=%d rounds=%d", len(session.Messages), session.FollowUpRounds)
			}
		}) {
			return
		}
		if _, err := repo.CompleteOrSkipAIGuidance(courseID, classID, studentID, sections[phase.section].ID, false); err != nil {
			t.Fatal(err)
		}
	}
}

func liveAIStream(t *testing.T, mux *http.ServeMux, path string, payload map[string]interface{}, wantReset bool) map[string]json.RawMessage {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	started := time.Now()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("%s status=%d response=%s", path, w.Code, w.Body.String())
	}
	var text string
	var done map[string]json.RawMessage
	resets, deltas := 0, 0
	for _, event := range strings.Split(w.Body.String(), "\n\n") {
		lines := strings.Split(event, "\n")
		if len(lines) < 2 {
			continue
		}
		typ := strings.TrimPrefix(lines[0], "event: ")
		data := strings.TrimPrefix(lines[1], "data: ")
		switch typ {
		case "error":
			t.Fatalf("%s stream error: %s", path, data)
		case "reset":
			text = ""
			resets++
		case "delta":
			var delta struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(data), &delta); err != nil {
				t.Fatal(err)
			}
			text += delta.Content
			deltas++
		case "done":
			if err := json.Unmarshal([]byte(data), &done); err != nil {
				t.Fatal(err)
			}
		}
	}
	if done == nil || text == "" || deltas == 0 || strings.Contains(text, "FAILED_PRIMARY_PARTIAL") {
		t.Fatalf("invalid completed stream: done=%t chars=%d deltas=%d", done != nil, len([]rune(text)), deltas)
	}
	if (resets > 0) != wantReset {
		t.Fatalf("reset count=%d wantReset=%t", resets, wantReset)
	}
	if raw, ok := done["assistantMessage"]; ok {
		var answer string
		_ = json.Unmarshal(raw, &answer)
		if answer != text {
			t.Fatal("stream and final answer differ")
		}
	}
	t.Logf("%s: duration=%s chars=%d deltas=%d resets=%d", path, time.Since(started).Round(time.Millisecond), len([]rune(text)), deltas, resets)
	return done
}
