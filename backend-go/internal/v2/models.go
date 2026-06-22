package v2

import "encoding/json"

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

type Class struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	StudentCount int    `json:"studentCount,omitempty"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type Student struct {
	ID        int    `json:"id"`
	ClassID   int    `json:"classId"`
	Name      string `json:"name"`
	StudentNo string `json:"studentNo"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type CourseTemplate struct {
	ID          int             `json:"id"`
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config"`
}

type Course struct {
	ID           int    `json:"id"`
	TemplateID   int    `json:"templateId"`
	TemplateCode string `json:"templateCode"`
	Title        string `json:"title"`
	Mode         string `json:"mode"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type Section struct {
	ID         int             `json:"id"`
	CourseID   int             `json:"courseId"`
	SectionKey string          `json:"sectionKey"`
	Title      string          `json:"title"`
	Type       string          `json:"type"`
	SortOrder  int             `json:"sortOrder"`
	Enabled    bool            `json:"enabled"`
	Fixed      bool            `json:"fixed"`
	Rules      json.RawMessage `json:"rules"`
}

type Question struct {
	ID            int             `json:"id"`
	CourseID      int             `json:"courseId"`
	SectionID     int             `json:"sectionId"`
	QuestionKey   string          `json:"questionKey"`
	Type          string          `json:"type"`
	Title         string          `json:"title"`
	Description   string          `json:"description"`
	Options       json.RawMessage `json:"options"`
	CorrectAnswer json.RawMessage `json:"correctAnswer"`
	Explanation   string          `json:"explanation"`
	Score         int             `json:"score"`
	SortOrder     int             `json:"sortOrder"`
	Enabled       bool            `json:"enabled"`
	Fixed         bool            `json:"fixed"`
	Rules         json.RawMessage `json:"rules"`
}

type AIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Classroom struct {
	ID         int    `json:"id"`
	CourseID   int    `json:"courseId"`
	ClassID    int    `json:"classId"`
	StageID    int    `json:"stageId"`
	Blackboard string `json:"blackboard"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type StatsSummary struct {
	ClassCount           int               `json:"classCount"`
	StudentCount         int               `json:"studentCount"`
	CourseCount          int               `json:"courseCount"`
	SectionCount         int               `json:"sectionCount"`
	QuestionCount        int               `json:"questionCount"`
	TotalClass           int               `json:"totalClass"`
	SubmittedCount       int               `json:"submittedCount"`
	Classes              []Class           `json:"classes"`
	Courses              []Course          `json:"courses"`
	Sections             []Section         `json:"sections"`
	Parts                []PartStats       `json:"parts"`
	Students             []StudentStats    `json:"students"`
	NotSubmittedStudents []string          `json:"notSubmittedStudents"`
	PredictionSummary    PredictionSummary `json:"predictionSummary"`
	Part1Stats           Part1Stats        `json:"part1Stats"`
	Part2Stats           Part2Stats        `json:"part2Stats"`
	Part3Stats           Part3Stats        `json:"part3Stats"`
	Classroom            *Classroom        `json:"classroom,omitempty"`
}

type PartStats struct {
	Part      int    `json:"part"`
	SectionID int    `json:"sectionId"`
	Title     string `json:"title"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
}

type StudentStats struct {
	ClassID           int    `json:"classId"`
	ClassName         string `json:"className"`
	StudentID         int    `json:"studentId"`
	StudentName       string `json:"studentName"`
	SubmissionID      int    `json:"submissionId,omitempty"`
	Status            string `json:"status"`
	StatusText        string `json:"statusText"`
	CompletedParts    int    `json:"completedParts"`
	PredictedScore    *int   `json:"predictedScore,omitempty"`
	QuizScore         *int   `json:"quizScore,omitempty"`
	TeacherScore      *int   `json:"teacherScore,omitempty"`
	ActualScore       *int   `json:"actualScore,omitempty"`
	ActualScoreSource string `json:"actualScoreSource,omitempty"`
	TeacherScoreNote  string `json:"teacherScoreNote,omitempty"`
	GuessResult       string `json:"guessResult"`
	GuessResultText   string `json:"guessResultText"`
	UpdatedAt         string `json:"updatedAt,omitempty"`
}

type PredictionSummary struct {
	Correct int `json:"correct"`
	High    int `json:"high"`
	Low     int `json:"low"`
	Unknown int `json:"unknown"`
}

type Part1Stats struct {
	PredictionScoreDistribution map[string]int `json:"predictionScoreDistribution"`
	LearningMethodsDistribution map[string]int `json:"learningMethodsDistribution"`
}

type Part2Stats struct {
	TotalCount                int            `json:"totalCount"`
	FilledCount               int            `json:"filledCount"`
	UnderstandingDistribution map[string]int `json:"understandingDistribution"`
}

type QuestionCorrectRate struct {
	QuestionID   int    `json:"questionId"`
	QuestionText string `json:"questionText"`
	SortOrder    int    `json:"sortOrder"`
	CorrectCount int    `json:"correctCount"`
	TotalCount   int    `json:"totalCount"`
	CorrectRate  int    `json:"correctRate"`
}

type Part3Stats struct {
	ScoreDistribution   map[int]int           `json:"scoreDistribution"`
	QuestionCorrectRate []QuestionCorrectRate `json:"questionCorrectRate"`
}

type StudentHistoryRecord struct {
	CourseID          int    `json:"courseId"`
	CourseTitle       string `json:"courseTitle"`
	Mode              string `json:"mode,omitempty"`
	ClassID           int    `json:"classId"`
	ClassName         string `json:"className"`
	StudentID         int    `json:"studentId"`
	StudentName       string `json:"studentName"`
	SubmissionID      int    `json:"submissionId"`
	Status            string `json:"status"`
	StatusText        string `json:"statusText"`
	PredictedScore    *int   `json:"predictedScore,omitempty"`
	QuizScore         *int   `json:"quizScore,omitempty"`
	TeacherScore      *int   `json:"teacherScore,omitempty"`
	ActualScore       *int   `json:"actualScore,omitempty"`
	ActualScoreSource string `json:"actualScoreSource"`
	TeacherScoreNote  string `json:"teacherScoreNote,omitempty"`
	GuessResult       string `json:"guessResult"`
	GuessResultText   string `json:"guessResultText"`
	CompletedParts    int    `json:"completedParts"`
	StartedAt         string `json:"startedAt"`
	CompletedAt       string `json:"completedAt,omitempty"`
	UpdatedAt         string `json:"updatedAt"`
}

type ScoreSummary struct {
	PredictedScore    *int   `json:"predictedScore,omitempty"`
	QuizScore         *int   `json:"quizScore,omitempty"`
	TeacherScore      *int   `json:"teacherScore,omitempty"`
	ActualScore       *int   `json:"actualScore,omitempty"`
	ActualScoreSource string `json:"actualScoreSource"`
	TeacherScoreNote  string `json:"teacherScoreNote,omitempty"`
	GuessResult       string `json:"guessResult"`
	GuessResultText   string `json:"guessResultText"`
}

type StudentDetail struct {
	SubmissionID      int                    `json:"submissionId"`
	CourseID          int                    `json:"courseId"`
	CourseTitle       string                 `json:"courseTitle"`
	Mode              string                 `json:"mode,omitempty"`
	ClassID           int                    `json:"classId"`
	ClassName         string                 `json:"className"`
	StudentID         int                    `json:"studentId"`
	StudentName       string                 `json:"studentName"`
	Status            string                 `json:"status"`
	StatusText        string                 `json:"statusText"`
	PredictedScore    *int                   `json:"predictedScore,omitempty"`
	QuizScore         *int                   `json:"quizScore,omitempty"`
	TeacherScore      *int                   `json:"teacherScore,omitempty"`
	ActualScore       *int                   `json:"actualScore,omitempty"`
	ActualScoreSource string                 `json:"actualScoreSource"`
	TeacherScoreNote  string                 `json:"teacherScoreNote,omitempty"`
	GuessResult       string                 `json:"guessResult"`
	GuessResultText   string                 `json:"guessResultText"`
	CompletedParts    int                    `json:"completedParts"`
	StartedAt         string                 `json:"startedAt"`
	CompletedAt       string                 `json:"completedAt,omitempty"`
	UpdatedAt         string                 `json:"updatedAt"`
	Sections          []StudentDetailSection `json:"sections"`
}

type StudentDetailSection struct {
	SectionID int                    `json:"sectionId"`
	Title     string                 `json:"title"`
	Type      string                 `json:"type"`
	SortOrder int                    `json:"sortOrder"`
	Fixed     bool                   `json:"fixed"`
	Completed bool                   `json:"completed"`
	Attempts  []StudentDetailAttempt `json:"attempts"`
}

type StudentDetailAttempt struct {
	AttemptNo   int                     `json:"attemptNo"`
	Score       *int                    `json:"score,omitempty"`
	SubmittedAt string                  `json:"submittedAt"`
	Questions   []StudentDetailQuestion `json:"questions"`
}

type StudentDetailQuestion struct {
	QuestionID    int             `json:"questionId"`
	QuestionText  string          `json:"questionText"`
	QuestionType  string          `json:"questionType"`
	SortOrder     int             `json:"sortOrder"`
	Answer        string          `json:"answer"`
	CorrectAnswer string          `json:"correctAnswer"`
	Explanation   string          `json:"explanation"`
	Score         int             `json:"score"`
	IsCorrect     bool            `json:"isCorrect"`
	ChatMessages  []AIChatMessage `json:"chatMessages,omitempty"`
}
