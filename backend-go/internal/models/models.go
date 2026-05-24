package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FlexInt supports integer fields that may arrive as strings or numbers in JSON.
type FlexInt int

func (f *FlexInt) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		*f = 0
		return nil
	}

	if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		unquoted, err := strconv.Unquote(raw)
		if err != nil {
			return err
		}
		unquoted = strings.TrimSpace(unquoted)
		if unquoted == "" {
			*f = 0
			return nil
		}
		value, err := strconv.Atoi(unquoted)
		if err != nil {
			return fmt.Errorf("invalid integer value %q", unquoted)
		}
		*f = FlexInt(value)
		return nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("invalid integer value %q", raw)
	}
	*f = FlexInt(value)
	return nil
}

// APIResponse is the unified JSON envelope for every endpoint.
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

// Course represents a lesson / course.
type Course struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	LessonNo    int     `json:"lesson_number"`
	Description *string `json:"description"`
	Part2Guide  *string `json:"part2_guide"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// Question represents a quiz question.
type Question struct {
	ID                int     `json:"id"`
	CourseID          int     `json:"course_id"`
	Part              int     `json:"part"`
	QuestionType      string  `json:"question_type"`
	QuestionText      string  `json:"question_text"`
	Options           *string `json:"options"`
	CorrectAnswer     *string `json:"correct_answer"`
	Explanation       *string `json:"explanation"`
	Enabled           bool    `json:"enabled"`
	AnnotationEnabled bool    `json:"annotation_enabled"`
	SortOrder         int     `json:"sort_order"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

// StudentSurvey holds a single student's answers.
type StudentSurvey struct {
	ID                    int                      `json:"id"`
	CourseID              int                      `json:"course_id"`
	StudentID             string                   `json:"student_id"`
	StudentName           string                   `json:"student_name"`
	ClassName             string                   `json:"class_name"`
	Part1                 json.RawMessage          `json:"part1_answers"`
	Part2                 *string                  `json:"part2_answer"`
	Part3                 json.RawMessage          `json:"part3_answers"`
	Part3Score            *int                     `json:"part3_score"`
	TeacherScore          *int                     `json:"teacher_score"`
	TeacherNote           *string                  `json:"teacher_score_note"`
	TeacherScoreUpdatedAt *string                  `json:"teacher_score_updated_at"`
	ActualScore           *int                     `json:"actual_score"`
	ActualScoreSource     string                   `json:"actual_score_source"`
	Part3Results          []map[string]interface{} `json:"part3_results,omitempty"`
	Part4                 json.RawMessage          `json:"part4_answers"`
	SubmittedAt           *string                  `json:"submitted_at"`
	CreatedAt             string                   `json:"created_at"`
	UpdatedAt             string                   `json:"updated_at"`
}

// CompletionStats tracks how many students have finished a part.
type CompletionStats struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
}

// Part1Answers is the shape stored in part1_answers JSON.
type Part1Answers struct {
	PredictionScore int      `json:"predictionScore"`
	LearningMethods []string `json:"learningMethods"`
	CustomMethod    string   `json:"customMethod"`
}

// Part2AnswerPayload is the stored shape for a part-2 answer.
type Part2AnswerPayload struct {
	QuestionType    string              `json:"questionType"`
	Value           string              `json:"value"`
	Values          []string            `json:"values,omitempty"`
	Label           string              `json:"label,omitempty"`
	Labels          []string            `json:"labels,omitempty"`
	AnnotationCount int                 `json:"annotationCount,omitempty"`
	Version         int                 `json:"version,omitempty"`
	Responses       []Part2ResponseItem `json:"responses,omitempty"`
}

// Part2ResponseItem is one question's answer inside part 2.
type Part2ResponseItem struct {
	QuestionID      int      `json:"questionId,omitempty"`
	QuestionText    string   `json:"questionText,omitempty"`
	QuestionType    string   `json:"questionType"`
	Value           string   `json:"value,omitempty"`
	Values          []string `json:"values,omitempty"`
	Label           string   `json:"label,omitempty"`
	Labels          []string `json:"labels,omitempty"`
	AnnotationCount int      `json:"annotationCount,omitempty"`
}

// Part2ResponseInput is the request shape for a single part-2 response.
type Part2ResponseInput struct {
	QuestionID int      `json:"questionId"`
	Answer     string   `json:"answer"`
	Answers    []string `json:"answers"`
}

// HistoryScore is used by the student history list.
type HistoryScore struct {
	CourseName        string  `json:"courseName"`
	LessonNumber      int     `json:"lessonNumber"`
	ActualScore       *int    `json:"actualScore"`
	ActualScoreSource string  `json:"actualScoreSource"`
	TeacherScoreNote  *string `json:"teacherScoreNote,omitempty"`
	PredictedScore    int     `json:"predictedScore"`
}

// StudentHistoryDetail is a single historical lesson record.
type StudentHistoryDetail struct {
	CourseID              int             `json:"course_id"`
	CourseName            string          `json:"course_name"`
	LessonNo              int             `json:"lesson_number"`
	StudentID             string          `json:"student_id"`
	StudentName           string          `json:"student_name"`
	ClassName             string          `json:"class_name"`
	Part1                 json.RawMessage `json:"part1_answers"`
	Part2                 *string         `json:"part2_answer"`
	Part3                 json.RawMessage `json:"part3_answers"`
	Part3Score            *int            `json:"part3_score"`
	TeacherScore          *int            `json:"teacher_score"`
	TeacherNote           *string         `json:"teacher_score_note"`
	TeacherScoreUpdatedAt *string         `json:"teacher_score_updated_at"`
	ActualScore           *int            `json:"actual_score"`
	ActualScoreSource     string          `json:"actual_score_source"`
	Part4                 json.RawMessage `json:"part4_answers"`
	SubmittedAt           *string         `json:"submitted_at"`
	UpdatedAt             string          `json:"updated_at"`
}

// QuestionRate is used in part-3 statistics.
type QuestionRate struct {
	QuestionID   int    `json:"questionId"`
	QuestionText string `json:"questionText"`
	SortOrder    int    `json:"sortOrder"`
	CorrectCount int    `json:"correctCount"`
	TotalCount   int    `json:"totalCount"`
	CorrectRate  int    `json:"correctRate"`
}

// PartSetting controls whether a lesson part is enabled.
type PartSetting struct {
	Part    int  `json:"part"`
	Enabled bool `json:"enabled"`
}

// StatsPayload is returned by the teacher statistics endpoint.
type StatsPayload struct {
	Part1      CompletionStats `json:"part1"`
	Part2      CompletionStats `json:"part2"`
	Part3      CompletionStats `json:"part3"`
	Part4      CompletionStats `json:"part4"`
	Students   []StudentSurvey `json:"students"`
	Part1Stats struct {
		PredictionScoreDistribution map[string]int `json:"predictionScoreDistribution"`
		LearningMethodsDistribution map[string]int `json:"learningMethodsDistribution"`
	} `json:"part1Stats"`
	Part2Stats struct {
		FilledCount               int            `json:"filledCount"`
		TotalCount                int            `json:"totalCount"`
		UnderstandingDistribution map[string]int `json:"understandingDistribution"`
	} `json:"part2Stats"`
	Part3Stats struct {
		ScoreDistribution   map[string]int `json:"scoreDistribution"`
		QuestionCorrectRate []QuestionRate `json:"questionCorrectRate"`
	} `json:"part3Stats"`
}

// TipPayload is a persisted classroom tip for a class.
type TipPayload struct {
	ClassName string `json:"className,omitempty"`
	Content   string `json:"content"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}
