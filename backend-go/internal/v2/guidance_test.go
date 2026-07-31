package v2

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupGuidanceTestRepository(t *testing.T) (*sql.DB, *Repository) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := ConfigureDB(db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := InitDB(db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db, NewRepository(db)
}

func TestReflectionAIGuidanceDefaultsAndCompletion(t *testing.T) {
	db, repo := setupGuidanceTestRepository(t)
	defer db.Close()

	courseID, err := repo.CreateCourse("指导测试课堂", "", "理解人工智能的基本概念", "reflection")
	if err != nil {
		t.Fatal(err)
	}
	sections, err := repo.ListSections(courseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 4 {
		t.Fatalf("expected 4 reflection sections, got %d", len(sections))
	}
	planConfig := parseAIGuidanceConfig(sections[0])
	evaluationConfig := parseAIGuidanceConfig(sections[3])
	if planConfig.Enabled || evaluationConfig.Enabled {
		t.Fatal("new reflection guidance modules must default to disabled")
	}
	if planConfig.Phase != "plan" || evaluationConfig.Phase != "evaluation" {
		t.Fatalf("unexpected guidance phases: %#v %#v", planConfig, evaluationConfig)
	}

	if _, err := repo.SetSectionAIGuidance(sections[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetSectionAIGuidance(sections[3].ID, true); err != nil {
		t.Fatal(err)
	}
	classID, err := repo.CreateClass("指导测试班", "")
	if err != nil {
		t.Fatal(err)
	}
	studentID, err := repo.CreateStudent(classID, "测试学生", "")
	if err != nil {
		t.Fatal(err)
	}
	previousCourseID, err := repo.CreateCourse("上一节课堂", "", "认识数据", "free")
	if err != nil {
		t.Fatal(err)
	}
	previousSectionID, err := repo.CreateSection(previousCourseID, "学习记录", "custom")
	if err != nil {
		t.Fatal(err)
	}
	previousQuestionID, err := repo.CreateQuestion(Question{
		SectionID: previousSectionID,
		Type:      "fill_blank",
		Title:     "上一节课学到了什么？",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SubmitSection(previousCourseID, classID, studentID, previousSectionID, map[string]json.RawMessage{
		stringID(previousQuestionID): json.RawMessage(`"认识了训练数据"`),
	}); err != nil {
		t.Fatal(err)
	}
	currentCustomQuestionID, err := repo.CreateQuestion(Question{
		SectionID: sections[0].ID,
		Type:      "fill_blank",
		Title:     "这节课准备怎么学习？",
	})
	if err != nil {
		t.Fatal(err)
	}
	questions, err := repo.ListQuestions(sections[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(questions) == 0 {
		t.Fatal("prediction question missing")
	}
	answers := map[string]json.RawMessage{
		stringID(questions[0].ID):         json.RawMessage(`"3"`),
		stringID(currentCustomQuestionID): json.RawMessage(`"先听讲再提问"`),
	}
	result, err := repo.SubmitSection(courseID, classID, studentID, sections[0].ID, answers)
	if err != nil {
		t.Fatal(err)
	}
	submissionID := result["submissionId"].(int)
	detail, err := repo.GetStudentDetail(submissionID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Sections[0].Completed {
		t.Fatal("plan section must remain incomplete until guidance is completed or skipped")
	}
	planContext, err := repo.guidanceContext(courseID, classID, studentID, sections[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	planSnapshot, err := repo.BuildAIGuidanceSnapshot(planContext)
	if err != nil {
		t.Fatal(err)
	}
	var planPayload map[string]interface{}
	if err := json.Unmarshal([]byte(planSnapshot), &planPayload); err != nil {
		t.Fatal(err)
	}
	if planPayload["learningObjective"] != "理解人工智能的基本概念" || planPayload["predictedScore"] != float64(3) {
		t.Fatalf("plan snapshot missing current objective or prediction: %s", planSnapshot)
	}
	if history, ok := planPayload["previousCourses"].([]interface{}); !ok || len(history) != 1 {
		t.Fatalf("plan snapshot should contain all prior course performance: %s", planSnapshot)
	}

	quizQuestions, err := repo.ListQuestions(sections[2].ID)
	if err != nil {
		t.Fatal(err)
	}
	quizAnswers := map[string]json.RawMessage{}
	for _, question := range quizQuestions {
		quizAnswers[stringID(question.ID)] = json.RawMessage(`"A"`)
	}
	firstQuizResult, err := repo.SubmitSection(courseID, classID, studentID, sections[2].ID, quizAnswers)
	if err != nil {
		t.Fatal(err)
	}
	evaluationContext, err := repo.guidanceContext(courseID, classID, studentID, sections[3].ID)
	if err != nil {
		t.Fatal(err)
	}
	evaluationSnapshot, err := repo.BuildAIGuidanceSnapshot(evaluationContext)
	if err != nil {
		t.Fatal(err)
	}
	var evaluationPayload map[string]interface{}
	if err := json.Unmarshal([]byte(evaluationSnapshot), &evaluationPayload); err != nil {
		t.Fatal(err)
	}
	if quizItems, ok := evaluationPayload["quizQuestions"].([]interface{}); !ok || len(quizItems) != 5 {
		t.Fatalf("evaluation snapshot should contain the five first-attempt quiz details: %s", evaluationSnapshot)
	}
	if answers, ok := evaluationPayload["currentCourseOtherAnswers"].([]interface{}); !ok || len(answers) == 0 {
		t.Fatalf("evaluation snapshot should contain current course answers: %s", evaluationSnapshot)
	}
	evaluationSession, reused, err := repo.StartAIGuidanceGeneration(evaluationContext, evaluationSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if reused {
		t.Fatal("first evaluation generation should not reuse a session")
	}
	if _, err := repo.CompleteAIGuidanceGeneration(evaluationSession.ID, "第一次小测评价"); err != nil {
		t.Fatal(err)
	}
	retakeAnswers := map[string]json.RawMessage{}
	for _, question := range quizQuestions {
		retakeAnswers[stringID(question.ID)] = json.RawMessage(`"B"`)
	}
	retakeResult, err := repo.SubmitSection(courseID, classID, studentID, sections[2].ID, retakeAnswers)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.loadAIGuidanceSessionBySubmission(submissionID, sections[3].ID); err != sql.ErrNoRows {
		t.Fatalf("retake should invalidate the unsubmitted evaluation session, got %v", err)
	}
	evaluationSnapshot, err = repo.BuildAIGuidanceSnapshot(evaluationContext)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(evaluationSnapshot), &evaluationPayload); err != nil {
		t.Fatal(err)
	}
	if evaluationPayload["retakeTaken"] != true || evaluationPayload["retakeQuizScore"] != float64(retakeResult["score"].(int)) {
		t.Fatalf("evaluation snapshot should include retake score: %s", evaluationSnapshot)
	}
	if quizItems, ok := evaluationPayload["retakeQuizQuestions"].([]interface{}); !ok || len(quizItems) != 5 {
		t.Fatalf("evaluation snapshot should contain the five retake quiz details: %s", evaluationSnapshot)
	}
	scoreSummary, err := repo.GetScoreSummary(courseID, classID, studentID)
	if err != nil {
		t.Fatal(err)
	}
	if scoreSummary.ActualScore == nil || *scoreSummary.ActualScore != firstQuizResult["score"].(int) {
		t.Fatalf("retake must not overwrite the official first score: %#v", scoreSummary)
	}
	if scoreSummary.RetakeScore == nil || *scoreSummary.RetakeScore != retakeResult["score"].(int) {
		t.Fatalf("retake score was not stored separately: %#v", scoreSummary)
	}

	handler := NewHandler(repo)
	mux := http.NewServeMux()
	handler.Register(mux)
	requestBody, err := json.Marshal(map[string]interface{}{
		"courseId":  courseID,
		"classId":   classID,
		"studentId": studentID,
		"sectionId": sections[0].ID,
		"skip":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v2/student/ai-guidance/complete", bytes.NewReader(requestBody))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("guidance completion endpoint returned %d: %s", recorder.Code, recorder.Body.String())
	}
	detail, err = repo.GetStudentDetail(submissionID)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Sections[0].Completed {
		t.Fatal("skipped guidance should complete the plan section")
	}
	if _, err := repo.SetSectionAIGuidance(sections[0].ID, false); err == nil {
		t.Fatal("guidance switch must lock after section answers exist")
	}
}

func stringID(value int) string {
	return strconv.Itoa(value)
}
