package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/xuri/excelize/v2"
)

type app struct {
	db   *sql.DB
	tips *tipStore
}

type tipStore struct {
	mu   sync.RWMutex
	data map[string]tipPayload
}

type tipPayload struct {
	Content   string `json:"content"`
	UpdatedAt int64  `json:"updatedAt"`
}

type apiResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

type flexInt int

func (f *flexInt) UnmarshalJSON(data []byte) error {
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
		*f = flexInt(value)
		return nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("invalid integer value %q", raw)
	}
	*f = flexInt(value)
	return nil
}

type course struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	LessonNo    int     `json:"lesson_number"`
	Description *string `json:"description"`
	Part2Guide  *string `json:"part2_guide"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type question struct {
	ID            int     `json:"id"`
	CourseID      int     `json:"course_id"`
	Part          int     `json:"part"`
	QuestionType  string  `json:"question_type"`
	QuestionText  string  `json:"question_text"`
	Options       *string `json:"options"`
	CorrectAnswer *string `json:"correct_answer"`
	Explanation   *string `json:"explanation"`
	SortOrder     int     `json:"sort_order"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type studentSurvey struct {
	ID          int             `json:"id"`
	CourseID    int             `json:"course_id"`
	StudentID   string          `json:"student_id"`
	StudentName string          `json:"student_name"`
	ClassName   string          `json:"class_name"`
	Part1       json.RawMessage `json:"part1_answers"`
	Part2       *string         `json:"part2_answer"`
	Part3       json.RawMessage `json:"part3_answers"`
	Part3Score  *int            `json:"part3_score"`
	Part4       json.RawMessage `json:"part4_answers"`
	SubmittedAt *string         `json:"submitted_at"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

type completionStats struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
}

type part1Answers struct {
	PredictionScore int      `json:"predictionScore"`
	LearningMethods []string `json:"learningMethods"`
	CustomMethod    string   `json:"customMethod"`
}

type part2AnswerPayload struct {
	QuestionType string   `json:"questionType"`
	Value        string   `json:"value"`
	Values       []string `json:"values,omitempty"`
	Label        string   `json:"label,omitempty"`
	Labels       []string `json:"labels,omitempty"`
}

type historyScore struct {
	CourseName     string `json:"courseName"`
	LessonNumber   int    `json:"lessonNumber"`
	ActualScore    int    `json:"actualScore"`
	PredictedScore int    `json:"predictedScore"`
}

type studentHistoryDetail struct {
	CourseID    int             `json:"course_id"`
	CourseName  string          `json:"course_name"`
	LessonNo    int             `json:"lesson_number"`
	StudentID   string          `json:"student_id"`
	StudentName string          `json:"student_name"`
	ClassName   string          `json:"class_name"`
	Part1       json.RawMessage `json:"part1_answers"`
	Part2       *string         `json:"part2_answer"`
	Part3       json.RawMessage `json:"part3_answers"`
	Part3Score  *int            `json:"part3_score"`
	Part4       json.RawMessage `json:"part4_answers"`
	SubmittedAt *string         `json:"submitted_at"`
	UpdatedAt   string          `json:"updated_at"`
}

type questionRate struct {
	QuestionID   int    `json:"questionId"`
	QuestionText string `json:"questionText"`
	SortOrder    int    `json:"sortOrder"`
	CorrectCount int    `json:"correctCount"`
	TotalCount   int    `json:"totalCount"`
	CorrectRate  int    `json:"correctRate"`
}

type partSetting struct {
	Part    int  `json:"part"`
	Enabled bool `json:"enabled"`
}

type statsPayload struct {
	Part1      completionStats `json:"part1"`
	Part2      completionStats `json:"part2"`
	Part3      completionStats `json:"part3"`
	Part4      completionStats `json:"part4"`
	Students   []studentSurvey `json:"students"`
	Part1Stats struct {
		PredictionScoreDistribution map[string]int `json:"predictionScoreDistribution"`
		LearningMethodsDistribution map[string]int `json:"learningMethodsDistribution"`
	} `json:"part1Stats"`
	Part2Stats struct {
		FilledCount int `json:"filledCount"`
		TotalCount  int `json:"totalCount"`
	} `json:"part2Stats"`
	Part3Stats struct {
		ScoreDistribution   map[string]int `json:"scoreDistribution"`
		QuestionCorrectRate []questionRate `json:"questionCorrectRate"`
	} `json:"part3Stats"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	dataDir := resolvePath("data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	dbPath := filepath.Join(dataDir, "quiz-system.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := configureDB(db); err != nil {
		log.Fatalf("configure db: %v", err)
	}

	a := &app{
		db: db,
		tips: &tipStore{
			data: make(map[string]tipPayload),
		},
	}

	if err := a.initDB(); err != nil {
		log.Fatalf("init db: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealth)
	mux.HandleFunc("GET /api/teacher/courses", a.handleTeacherCourses)
	mux.HandleFunc("POST /api/teacher/current-course", a.handleSetCurrentCourse)
	mux.HandleFunc("GET /api/teacher/class-list/{courseId}/{className}", a.handleTeacherClassList)
	mux.HandleFunc("POST /api/teacher/class-list", a.handleImportClassList)
	mux.HandleFunc("GET /api/teacher/classes/{courseId}", a.handleTeacherClasses)
	mux.HandleFunc("GET /api/teacher/stage/{courseId}", a.handleTeacherStage)
	mux.HandleFunc("POST /api/teacher/stage", a.handleSetTeacherStage)
	mux.HandleFunc("POST /api/teacher/bind-class-course", a.handleBindClassCourse)
	mux.HandleFunc("POST /api/teacher/tip", a.handleTeacherTip)
	mux.HandleFunc("GET /api/teacher/questions/{courseId}/{part}", a.handleTeacherQuestions)
	mux.HandleFunc("POST /api/teacher/question/{id}", a.handleUpdateQuestion)
	mux.HandleFunc("GET /api/teacher/part2-guide/{courseId}", a.handleGetPart2Guide)
	mux.HandleFunc("POST /api/teacher/part2-guide/{courseId}", a.handleSetPart2Guide)
	mux.HandleFunc("GET /api/teacher/part-settings/{courseId}", a.handleGetPartSettings)
	mux.HandleFunc("POST /api/teacher/part-settings/{courseId}", a.handleSetPartSettings)
	mux.HandleFunc("GET /api/teacher/stats/{courseId}", a.handleTeacherStats)
	mux.HandleFunc("GET /api/teacher/stats/{courseId}/export", a.handleExportStats)
	mux.HandleFunc("POST /api/student/validate", a.handleStudentValidate)
	mux.HandleFunc("GET /api/student/tip", a.handleStudentTip)
	mux.HandleFunc("GET /api/student/questions/{part}", a.handleStudentQuestions)
	mux.HandleFunc("GET /api/student-history/{studentId}", a.handleStudentHistoryDetails)
	mux.HandleFunc("GET /api/student/{studentId}", a.handleStudentStatus)
	mux.HandleFunc("POST /api/student/{studentId}/part1", a.handleStudentPart1)
	mux.HandleFunc("POST /api/student/{studentId}/part2", a.handleStudentPart2)
	mux.HandleFunc("POST /api/student/{studentId}/part3", a.handleStudentPart3)
	mux.HandleFunc("POST /api/student/{studentId}/part4", a.handleStudentPart4)

	handler := withCORS(withLogging(mux))
	port := getenv("PORT", "8080")
	addr := "0.0.0.0:" + port
	log.Printf("Go backend listening on http://%s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func configureDB(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) initDB() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS system_config (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS courses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			lesson_number INTEGER NOT NULL UNIQUE,
			description TEXT,
			part2_guide TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS class_lists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			class_name TEXT NOT NULL,
			student_name TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(course_id, class_name, student_name),
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS course_stages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			class_name TEXT NOT NULL,
			current_stage INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(course_id, class_name),
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS class_course_bind (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			class_name TEXT NOT NULL UNIQUE,
			course_id INTEGER NOT NULL,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS questions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			part INTEGER NOT NULL,
			question_type TEXT NOT NULL,
			question_text TEXT NOT NULL,
			options TEXT,
			correct_answer TEXT,
			explanation TEXT,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS student_surveys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			student_id TEXT NOT NULL,
			student_name TEXT NOT NULL,
			class_name TEXT NOT NULL,
			part1_answers TEXT,
			part2_answer TEXT,
			part3_answers TEXT,
			part3_score INTEGER,
			part4_answers TEXT,
			submitted_at TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(course_id, student_id),
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS course_part_settings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL,
			part INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(course_id, part),
			FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
		)`,
	}
	for _, stmt := range stmts {
		if _, err := a.db.Exec(stmt); err != nil {
			return err
		}
	}

	if _, err := a.db.Exec(`ALTER TABLE courses ADD COLUMN part2_guide TEXT`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}

	var courseCount int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM courses`).Scan(&courseCount); err != nil {
		return err
	}
	if courseCount == 0 {
		for i := 1; i <= 12; i++ {
			if _, err := a.db.Exec(`INSERT INTO courses (name, lesson_number) VALUES (?, ?)`, fmt.Sprintf("第%d节课", i), i); err != nil {
				return err
			}
		}
	}

	if _, err := a.db.Exec(`
		INSERT INTO system_config (key, value, updated_at)
		VALUES ('current_course_id', '1', CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO NOTHING
	`); err != nil {
		return err
	}

	return a.initDefaultQuestions()
}

func (a *app) initDefaultQuestions() error {
	rows, err := a.db.Query(`SELECT id FROM courses ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var courseIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return err
		}
		courseIDs = append(courseIDs, id)
	}

	insertStmt := `INSERT INTO questions (course_id, part, question_type, question_text, options, correct_answer, explanation, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	for _, courseID := range courseIDs {
		for part := 1; part <= 4; part++ {
			if _, err := a.db.Exec(`
				INSERT INTO course_part_settings (course_id, part, enabled, updated_at)
				VALUES (?, ?, 1, CURRENT_TIMESTAMP)
				ON CONFLICT(course_id, part) DO NOTHING
			`, courseID, part); err != nil {
				return err
			}
		}
		var count int
		if err := a.db.QueryRow(`SELECT COUNT(*) FROM questions WHERE course_id = ?`, courseID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		defaults := defaultQuestions(courseID)
		for _, q := range defaults {
			if _, err := a.db.Exec(insertStmt, q.CourseID, q.Part, q.QuestionType, q.QuestionText, q.Options, q.CorrectAnswer, q.Explanation, q.SortOrder); err != nil {
				return err
			}
		}
	}
	return nil
}

func defaultQuestions(courseID int) []question {
	options1 := mustJSON([]string{"0分 - 完全没把握", "1分 - 我觉得很难", "2分 - 可能会有点难", "3分 - 能学会一半吧", "4分 - 大部分能学会", "5分 - 我全都能学会！"})
	options2 := mustJSON([]string{"竖起耳朵认真听讲", "遇到不懂的查资料", "请教老师或同学", "边听边记笔记", "和同桌讨论", "其他"})
	part3Single := mustJSON([]string{"A. 数据", "B. 算法", "C. 规则", "D. 算力"})
	part3Judge := mustJSON([]string{"对", "错"})
	return []question{
		{CourseID: courseID, Part: 1, QuestionType: "single", QuestionText: "今天这节课，你觉得你能学会多少呢？0到5分，给自己打个分吧！", Options: &options1, SortOrder: 1},
		{CourseID: courseID, Part: 1, QuestionType: "multi", QuestionText: "为了达到学习效果，你打算怎么做？", Options: &options2, SortOrder: 2},
		{CourseID: courseID, Part: 2, QuestionType: "text", QuestionText: "人工智能可以代替人类做什么，不能做什么？请写下你的思考。", SortOrder: 1},
		{CourseID: courseID, Part: 3, QuestionType: "single", QuestionText: "下面哪一项不是人工智能的三个核心要素？", Options: &part3Single, CorrectAnswer: ptr("C"), Explanation: ptr("人工智能的三个核心要素是数据、算法和算力。规则不是核心要素。"), SortOrder: 1},
		{CourseID: courseID, Part: 3, QuestionType: "single", QuestionText: "下面哪一项是让人工智能算得又快又猛的\"火力\"？", Options: &part3Single, CorrectAnswer: ptr("D"), Explanation: ptr("算力是让人工智能算得又快又猛的\"火力\"，它决定了人工智能的计算速度和能力。"), SortOrder: 2},
		{CourseID: courseID, Part: 3, QuestionType: "judge", QuestionText: "人工智能是人类制造的、能模仿人类智力和能力的一种技术。", Options: &part3Judge, CorrectAnswer: ptr("对"), Explanation: ptr("这个说法是正确的。人工智能是人类制造的、能模仿人类智力和能力的一种技术。"), SortOrder: 3},
		{CourseID: courseID, Part: 3, QuestionType: "judge", QuestionText: "学习累了的时候，可以让人工智能帮忙写作业。", Options: &part3Judge, CorrectAnswer: ptr("错"), Explanation: ptr("这个说法是错误的。我们应该自己完成作业，人工智能可以帮助我们学习，但不能代替我们写作业。"), SortOrder: 4},
		{CourseID: courseID, Part: 3, QuestionType: "judge", QuestionText: "人工智能是一种可以帮助我们学习的工具。", Options: &part3Judge, CorrectAnswer: ptr("对"), Explanation: ptr("这个说法是正确的。人工智能是一种可以帮助我们学习的工具，我们应该正确使用它。"), SortOrder: 5},
	}
}

func (a *app) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]string{"status": "ok"}})
}

func (a *app) handleTeacherCourses(w http.ResponseWriter, r *http.Request) {
	courses, err := a.getAllCourses()
	if err != nil {
		writeError(w, err)
		return
	}
	currentCourseID, err := a.getCurrentCourseID()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]interface{}{
		"courses":         courses,
		"currentCourseId": currentCourseID,
	}})
}

func (a *app) handleSetCurrentCourse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CourseID flexInt `json:"courseId"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	if int(body.CourseID) <= 0 {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 无效"})
		return
	}
	if err := a.setCurrentCourseID(int(body.CourseID)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "切换课程成功"})
}

func (a *app) handleTeacherClassList(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 无效"})
		return
	}
	className := r.PathValue("className")
	if courseID <= 0 {
		courseID, err = a.getClassBoundCourse(className)
		if err != nil {
			writeError(w, err)
			return
		}
		if courseID <= 0 {
			courseID, err = a.getCurrentCourseID()
			if err != nil {
				writeError(w, err)
				return
			}
		}
	}
	students, err := a.getClassList(courseID, className)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]interface{}{"students": students}})
}

func (a *app) handleImportClassList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CourseID     flexInt  `json:"courseId"`
		ClassName    string   `json:"className"`
		StudentNames []string `json:"studentNames"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	if err := a.importClassList(int(body.CourseID), body.ClassName, body.StudentNames); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "导入成功"})
}

func (a *app) handleTeacherClasses(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 无效"})
		return
	}
	classes, err := a.getAllClasses(courseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]interface{}{"classes": classes}})
}

func (a *app) handleTeacherStage(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 无效"})
		return
	}
	stage, err := a.getCurrentStage(courseID, r.URL.Query().Get("className"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]int{"stage": stage}})
}

func (a *app) handleSetTeacherStage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CourseID  flexInt `json:"courseId"`
		Stage     flexInt `json:"stage"`
		ClassName string  `json:"className"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	if int(body.CourseID) <= 0 || int(body.Stage) <= 0 {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 或 stage 无效"})
		return
	}
	enabled, err := a.isPartEnabled(int(body.CourseID), int(body.Stage))
	if err != nil {
		writeError(w, err)
		return
	}
	if !enabled {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: fmt.Sprintf("第%d部分当前已关闭，无法切换", int(body.Stage))})
		return
	}
	if err := a.setCurrentStage(int(body.CourseID), int(body.Stage), body.ClassName); err != nil {
		writeError(w, err)
		return
	}
	target := "所有班级"
	if strings.TrimSpace(body.ClassName) != "" {
		target = body.ClassName
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: fmt.Sprintf("已将%s切换到第%d部分", target, int(body.Stage))})
}

func (a *app) handleGetPartSettings(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 无效"})
		return
	}
	settings, err := a.getPartSettings(courseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]interface{}{"settings": settings}})
}

func (a *app) handleSetPartSettings(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 无效"})
		return
	}
	var body struct {
		Settings []partSetting `json:"settings"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	if len(body.Settings) == 0 {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "缺少部分配置"})
		return
	}
	if err := a.updatePartSettings(courseID, body.Settings); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "部分启用状态更新成功"})
}

func (a *app) handleBindClassCourse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClassName string  `json:"className"`
		CourseID  flexInt `json:"courseId"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	if int(body.CourseID) <= 0 {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 无效"})
		return
	}
	if err := a.setClassBoundCourse(body.ClassName, int(body.CourseID)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "班级课程绑定成功"})
}

func (a *app) handleTeacherTip(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClassName string `json:"className"`
		Content   string `json:"content"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	a.tips.mu.Lock()
	a.tips.data[body.ClassName] = tipPayload{Content: body.Content, UpdatedAt: time.Now().UnixMilli()}
	a.tips.mu.Unlock()
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "提示语提交成功"})
}

func (a *app) handleTeacherQuestions(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 无效"})
		return
	}
	part, err := strconv.Atoi(r.PathValue("part"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "part 无效"})
		return
	}
	questions, err := a.getQuestionsByPart(courseID, part)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]interface{}{"questions": questions}})
}

func (a *app) handleUpdateQuestion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "question id 无效"})
		return
	}
	var body struct {
		QuestionType  string  `json:"questionType"`
		QuestionText  string  `json:"questionText"`
		Options       *string `json:"options"`
		CorrectAnswer *string `json:"correctAnswer"`
		Explanation   *string `json:"explanation"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	if err := a.updateQuestion(id, body.QuestionType, body.QuestionText, body.Options, body.CorrectAnswer, body.Explanation); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "更新成功"})
}

func (a *app) handleGetPart2Guide(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 无效"})
		return
	}
	guide, err := a.getPart2Guide(courseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]string{"guide": guide}})
}

func (a *app) handleSetPart2Guide(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 无效"})
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	if err := a.updatePart2Guide(courseID, body.Content); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "答题指引更新成功"})
}

func (a *app) handleTeacherStats(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 无效"})
		return
	}
	className := r.URL.Query().Get("className")
	stats, err := a.buildStats(courseID, className)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: stats})
}

func (a *app) handleExportStats(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.Atoi(r.PathValue("courseId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "courseId 无效"})
		return
	}
	className := r.URL.Query().Get("className")
	students, err := a.getAllStudentSurveys(courseID, className)
	if err != nil {
		writeError(w, err)
		return
	}

	file := excelize.NewFile()
	sheet := "答题记录"
	file.SetSheetName("Sheet1", sheet)
	headers := []string{
		"班级", "姓名", "完成状态", "第一部分-预测得分", "第一部分-学习方法", "第一部分-自定义学习方法",
		"第二部分-开放题答案", "第三部分-第1题答案", "第三部分-第2题答案", "第三部分-第3题答案",
		"第三部分-第4题答案", "第三部分-第5题答案", "第三部分-总得分", "第四部分-实际得分",
		"第四部分-预测得分", "第四部分-第2题答案", "第四部分-第2题自定义内容", "第四部分-第3题答案",
		"第四部分-第3题自定义内容", "最后提交时间",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		file.SetCellValue(sheet, cell, h)
	}
	for rowIndex, s := range students {
		record := buildExportRow(s)
		for colIndex, value := range record {
			cell, _ := excelize.CoordinatesToCellName(colIndex+1, rowIndex+2)
			file.SetCellValue(sheet, cell, value)
		}
	}

	filename := fmt.Sprintf("答题记录_%s_%s.xlsx", fallback(className, "所有班级"), time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", urlEncode(filename)))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.WriteHeader(http.StatusOK)
	if _, err := file.WriteTo(w); err != nil {
		log.Printf("write export file: %v", err)
	}
}

func (a *app) handleStudentValidate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CourseID    *int   `json:"courseId"`
		ClassName   string `json:"className"`
		StudentName string `json:"studentName"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	effectiveCourseID, err := a.getClassBoundCourse(body.ClassName)
	if err != nil {
		writeError(w, err)
		return
	}
	if effectiveCourseID == 0 {
		if body.CourseID != nil && *body.CourseID > 0 {
			effectiveCourseID = *body.CourseID
		} else {
			effectiveCourseID, err = a.getCurrentCourseID()
			if err != nil {
				writeError(w, err)
				return
			}
		}
	}
	valid, err := a.validateStudent(effectiveCourseID, body.ClassName, body.StudentName)
	if err != nil {
		writeError(w, err)
		return
	}
	message := "验证成功"
	if !valid {
		message = "学生不在班级名单中，请联系老师"
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]interface{}{
		"valid":    valid,
		"message":  message,
		"courseId": effectiveCourseID,
	}})
}

func (a *app) handleStudentTip(w http.ResponseWriter, r *http.Request) {
	className := r.URL.Query().Get("className")
	a.tips.mu.RLock()
	tip, ok := a.tips.data[className]
	a.tips.mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]string{}})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]string{"content": tip.Content}})
}

func (a *app) handleStudentStatus(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	courseID, err := a.resolveCourseID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	survey, err := a.getStudentSurvey(courseID, studentID)
	if err != nil {
		writeError(w, err)
		return
	}
	className, _ := splitStudentID(studentID)
	stage, err := a.getCurrentStage(courseID, className)
	if err != nil {
		writeError(w, err)
		return
	}
	course, err := a.getCourseByID(courseID)
	if err != nil {
		writeError(w, err)
		return
	}
	history, err := a.getStudentHistoryScores(studentID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]interface{}{
		"survey":        survey,
		"currentStage":  stage,
		"courseName":    course.Name,
		"lessonNumber":  course.LessonNo,
		"historyScores": history,
		"partSettings":  mustPartSettingsMap(a, courseID),
	}})
}

func (a *app) handleStudentHistoryDetails(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	courseID, err := a.resolveCourseID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	items, err := a.getStudentHistoryDetails(studentID, courseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]interface{}{
		"items": items,
	}})
}

func (a *app) handleStudentQuestions(w http.ResponseWriter, r *http.Request) {
	part, err := strconv.Atoi(r.PathValue("part"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "part 无效"})
		return
	}
	courseID, err := a.resolveCourseID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	questions, err := a.getQuestionsByPart(courseID, part)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]interface{}{"questions": questions}})
}

func (a *app) handleStudentPart1(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	var body struct {
		CourseID    *int            `json:"courseId"`
		StudentName string          `json:"studentName"`
		ClassName   string          `json:"className"`
		Answers     json.RawMessage `json:"answers"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	courseID, err := a.resolveCourseIDFromBody(body.CourseID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	if err := a.saveStudentPart1(courseID, studentID, body.StudentName, body.ClassName, body.Answers); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "第一部分提交成功", Data: json.RawMessage(body.Answers)})
}

func (a *app) handleStudentPart2(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	var body struct {
		CourseID *int     `json:"courseId"`
		Answer   string   `json:"answer"`
		Answers  []string `json:"answers"`
	}
	type part2Response struct {
		Answer       string `json:"answer"`
		DisplayValue string `json:"displayValue"`
		QuestionType string `json:"questionType"`
		Explanation  string `json:"explanation"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	courseID, err := a.resolveCourseIDFromBody(body.CourseID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	questions, err := a.getQuestionsByPart(courseID, 2)
	if err != nil {
		writeError(w, err)
		return
	}
	if len(questions) == 0 {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "第二部分题目不存在"})
		return
	}
	question := questions[0]
	storedAnswer, displayValue, err := buildPart2StoredAnswer(question, strings.TrimSpace(body.Answer), body.Answers)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	if err := a.saveStudentPart2(courseID, studentID, storedAnswer); err != nil {
		writeError(w, err)
		return
	}
	explanation := ""
	if question.Explanation != nil {
		explanation = *question.Explanation
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "第二部分提交成功", Data: part2Response{
		Answer:       storedAnswer,
		DisplayValue: displayValue,
		QuestionType: question.QuestionType,
		Explanation:  explanation,
	}})
}

func (a *app) handleStudentPart3(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	var body struct {
		CourseID *int              `json:"courseId"`
		Answers  map[string]string `json:"answers"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	courseID, err := a.resolveCourseIDFromBody(body.CourseID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	questions, err := a.getQuestionsByPart(courseID, 3)
	if err != nil {
		writeError(w, err)
		return
	}
	score := 0
	results := make([]map[string]interface{}, 0, len(questions))
	for i, q := range questions {
		key := fmt.Sprintf("q%d", i+1)
		answer := body.Answers[key]
		correct := q.CorrectAnswer != nil && answer == *q.CorrectAnswer
		if correct {
			score++
		}
		results = append(results, map[string]interface{}{
			"question":      q.QuestionText,
			"studentAnswer": answer,
			"correctAnswer": deref(q.CorrectAnswer),
			"explanation":   deref(q.Explanation),
			"isCorrect":     correct,
		})
	}
	raw, _ := json.Marshal(body.Answers)
	if err := a.saveStudentPart3(courseID, studentID, raw, score); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: fmt.Sprintf("第三部分提交成功，得分%d/%d", score, len(questions)), Data: map[string]interface{}{
		"score":   score,
		"total":   len(questions),
		"results": results,
	}})
}

func (a *app) handleStudentPart4(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	var body struct {
		CourseID *int            `json:"courseId"`
		Answers  json.RawMessage `json:"answers"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	courseID, err := a.resolveCourseIDFromBody(body.CourseID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
		return
	}
	if err := a.saveStudentPart4(courseID, studentID, body.Answers); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "问卷提交完成，感谢参与！"})
}

func (a *app) getAllCourses() ([]course, error) {
	rows, err := a.db.Query(`SELECT id, name, lesson_number, description, part2_guide, created_at, updated_at FROM courses ORDER BY lesson_number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []course
	for rows.Next() {
		var c course
		var description, guide sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.LessonNo, &description, &guide, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if description.Valid {
			c.Description = ptr(description.String)
		}
		if guide.Valid {
			c.Part2Guide = ptr(guide.String)
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

func (a *app) getCourseByID(id int) (course, error) {
	var c course
	var description, guide sql.NullString
	err := a.db.QueryRow(`SELECT id, name, lesson_number, description, part2_guide, created_at, updated_at FROM courses WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.LessonNo, &description, &guide, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return c, err
	}
	if description.Valid {
		c.Description = ptr(description.String)
	}
	if guide.Valid {
		c.Part2Guide = ptr(guide.String)
	}
	return c, nil
}

func (a *app) getCurrentCourseID() (int, error) {
	var value string
	err := a.db.QueryRow(`SELECT value FROM system_config WHERE key = 'current_course_id'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	id, err := strconv.Atoi(value)
	if err != nil {
		return 1, nil
	}
	return id, nil
}

func (a *app) setCurrentCourseID(id int) error {
	_, err := a.db.Exec(`
		INSERT INTO system_config (key, value, updated_at)
		VALUES ('current_course_id', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, strconv.Itoa(id))
	return err
}

func (a *app) queryDistinctNames(query string, args ...interface{}) ([]string, error) {
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func uniqueSortedStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func (a *app) getClassRosterNames(courseID int, className string) ([]string, error) {
	className = strings.TrimSpace(className)
	if className == "" {
		return nil, nil
	}

	sources := []struct {
		query string
		args  []interface{}
	}{
		{
			query: `SELECT DISTINCT student_name FROM class_lists WHERE class_name = ? ORDER BY student_name`,
			args:  []interface{}{className},
		},
		{
			query: `SELECT DISTINCT student_name FROM student_surveys WHERE course_id = ? AND class_name = ? ORDER BY student_name`,
			args:  []interface{}{courseID, className},
		},
		{
			query: `SELECT DISTINCT student_name FROM student_surveys WHERE class_name = ? ORDER BY student_name`,
			args:  []interface{}{className},
		},
	}

	for _, source := range sources {
		names, err := a.queryDistinctNames(source.query, source.args...)
		if err != nil {
			return nil, err
		}
		names = uniqueSortedStrings(names)
		if len(names) > 0 {
			return names, nil
		}
	}

	return nil, nil
}

func (a *app) getClassList(courseID int, className string) ([]map[string]string, error) {
	names, err := a.getClassRosterNames(courseID, className)
	if err != nil {
		return nil, err
	}

	var students []map[string]string
	for _, name := range names {
		students = append(students, map[string]string{"student_name": name})
	}
	return students, nil
}

func (a *app) importClassList(courseID int, className string, studentNames []string) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	className = strings.TrimSpace(className)
	if className == "" {
		return errors.New("班级名称不能为空")
	}
	if courseID <= 0 {
		courseID = 1
	}
	if _, err := tx.Exec(`DELETE FROM class_lists WHERE class_name = ?`, className); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO class_lists (course_id, class_name, student_name) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	seen := map[string]struct{}{}
	for _, rawName := range studentNames {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if _, err := stmt.Exec(courseID, className, name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *app) validateStudent(courseID int, className, studentName string) (bool, error) {
	studentName = strings.TrimSpace(studentName)
	if studentName == "" {
		return false, nil
	}

	names, err := a.getClassRosterNames(courseID, className)
	if err != nil {
		return false, err
	}
	for _, name := range names {
		if name == studentName {
			return true, nil
		}
	}
	return false, nil
}

func (a *app) getAllClasses(courseID int) ([]map[string]string, error) {
	classLists, err := a.queryDistinctNames(`SELECT DISTINCT class_name FROM class_lists ORDER BY class_name`)
	if err != nil {
		return nil, err
	}
	surveyClasses, err := a.queryDistinctNames(`SELECT DISTINCT class_name FROM student_surveys WHERE course_id = ? ORDER BY class_name`, courseID)
	if err != nil {
		return nil, err
	}
	boundClasses, err := a.queryDistinctNames(`SELECT DISTINCT class_name FROM class_course_bind WHERE course_id = ? ORDER BY class_name`, courseID)
	if err != nil {
		return nil, err
	}

	allNames := uniqueSortedStrings(append(append(classLists, surveyClasses...), boundClasses...))
	var classes []map[string]string
	for _, name := range allNames {
		classes = append(classes, map[string]string{"class_name": name})
	}
	return classes, nil
}

func (a *app) getCurrentStage(courseID int, className string) (int, error) {
	if strings.TrimSpace(className) != "" {
		var stage int
		err := a.db.QueryRow(`SELECT current_stage FROM course_stages WHERE course_id = ? AND class_name = ?`, courseID, className).Scan(&stage)
		if err == nil {
			return stage, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		err = a.db.QueryRow(`SELECT current_stage FROM course_stages WHERE course_id = ? AND class_name = 'common'`, courseID).Scan(&stage)
		if err == nil {
			return stage, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		return 1, nil
	}
	var stage sql.NullInt64
	err := a.db.QueryRow(`SELECT MAX(current_stage) FROM course_stages WHERE course_id = ?`, courseID).Scan(&stage)
	if err != nil {
		return 0, err
	}
	if !stage.Valid {
		return 1, nil
	}
	return int(stage.Int64), nil
}

func (a *app) setCurrentStage(courseID, stage int, className string) error {
	target := strings.TrimSpace(className)
	if target == "" {
		target = "common"
	}
	_, err := a.db.Exec(`
		INSERT INTO course_stages (course_id, class_name, current_stage, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(course_id, class_name) DO UPDATE SET current_stage = excluded.current_stage, updated_at = CURRENT_TIMESTAMP
	`, courseID, target, stage)
	return err
}

func (a *app) getPartSettings(courseID int) ([]partSetting, error) {
	rows, err := a.db.Query(`
		SELECT part, enabled
		FROM course_part_settings
		WHERE course_id = ?
		ORDER BY part
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := make([]partSetting, 0, 4)
	for rows.Next() {
		var part int
		var enabledInt int
		if err := rows.Scan(&part, &enabledInt); err != nil {
			return nil, err
		}
		settings = append(settings, partSetting{
			Part:    part,
			Enabled: enabledInt == 1,
		})
	}
	if len(settings) == 0 {
		for part := 1; part <= 4; part++ {
			settings = append(settings, partSetting{Part: part, Enabled: true})
		}
	}
	return settings, rows.Err()
}

func (a *app) isPartEnabled(courseID, part int) (bool, error) {
	var enabledInt int
	err := a.db.QueryRow(`SELECT enabled FROM course_part_settings WHERE course_id = ? AND part = ?`, courseID, part).Scan(&enabledInt)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return enabledInt == 1, nil
}

func (a *app) updatePartSettings(courseID int, settings []partSetting) error {
	enabledCount := 0
	for _, setting := range settings {
		if setting.Enabled {
			enabledCount++
		}
	}
	if enabledCount == 0 {
		return errors.New("至少需要开启一个部分")
	}

	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, setting := range settings {
		if setting.Part < 1 || setting.Part > 4 {
			return fmt.Errorf("第%d部分无效", setting.Part)
		}
		enabledInt := 0
		if setting.Enabled {
			enabledInt = 1
		}
		if _, err := tx.Exec(`
			INSERT INTO course_part_settings (course_id, part, enabled, updated_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(course_id, part) DO UPDATE SET enabled = excluded.enabled, updated_at = CURRENT_TIMESTAMP
		`, courseID, setting.Part, enabledInt); err != nil {
			return err
		}
	}

	var currentStage sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(current_stage) FROM course_stages WHERE course_id = ?`, courseID).Scan(&currentStage); err != nil {
		return err
	}
	if currentStage.Valid {
		enabledMap := make(map[int]bool, len(settings))
		for _, setting := range settings {
			enabledMap[setting.Part] = setting.Enabled
		}
		if !enabledMap[int(currentStage.Int64)] {
			nextStage := firstEnabledPart(enabledMap)
			if nextStage == 0 {
				nextStage = 1
			}
			if _, err := tx.Exec(`UPDATE course_stages SET current_stage = ?, updated_at = CURRENT_TIMESTAMP WHERE course_id = ?`, nextStage, courseID); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (a *app) getClassBoundCourse(className string) (int, error) {
	var courseID int
	err := a.db.QueryRow(`SELECT course_id FROM class_course_bind WHERE class_name = ?`, className).Scan(&courseID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return courseID, err
}

func (a *app) setClassBoundCourse(className string, courseID int) error {
	_, err := a.db.Exec(`
		INSERT INTO class_course_bind (class_name, course_id, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(class_name) DO UPDATE SET course_id = excluded.course_id, updated_at = CURRENT_TIMESTAMP
	`, className, courseID)
	return err
}

func (a *app) getQuestionsByPart(courseID, part int) ([]question, error) {
	rows, err := a.db.Query(`
		SELECT id, course_id, part, question_type, question_text, options, correct_answer, explanation, sort_order, created_at, updated_at
		FROM questions
		WHERE course_id = ? AND part = ?
		ORDER BY sort_order
	`, courseID, part)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var questions []question
	for rows.Next() {
		var q question
		var options, correctAnswer, explanation sql.NullString
		if err := rows.Scan(&q.ID, &q.CourseID, &q.Part, &q.QuestionType, &q.QuestionText, &options, &correctAnswer, &explanation, &q.SortOrder, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, err
		}
		if options.Valid {
			q.Options = ptr(options.String)
		}
		if correctAnswer.Valid {
			q.CorrectAnswer = ptr(correctAnswer.String)
		}
		if explanation.Valid {
			q.Explanation = ptr(explanation.String)
		}
		questions = append(questions, q)
	}
	return questions, rows.Err()
}

func (a *app) updateQuestion(id int, questionType, questionText string, options, correctAnswer, explanation *string) error {
	_, err := a.db.Exec(`
		UPDATE questions
		SET question_type = ?, question_text = ?, options = ?, correct_answer = ?, explanation = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, questionType, questionText, emptyToNil(options), emptyToNil(correctAnswer), emptyToNil(explanation), id)
	return err
}

func (a *app) getPart2Guide(courseID int) (string, error) {
	var guide sql.NullString
	err := a.db.QueryRow(`SELECT part2_guide FROM courses WHERE id = ?`, courseID).Scan(&guide)
	if err != nil {
		return "", err
	}
	if !guide.Valid {
		return "", nil
	}
	return guide.String, nil
}

func (a *app) updatePart2Guide(courseID int, content string) error {
	_, err := a.db.Exec(`UPDATE courses SET part2_guide = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, content, courseID)
	return err
}

func (a *app) saveStudentPart1(courseID int, studentID, studentName, className string, answers json.RawMessage) error {
	_, err := a.db.Exec(`
		INSERT INTO student_surveys (course_id, student_id, student_name, class_name, part1_answers, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(course_id, student_id) DO UPDATE SET
			student_name = excluded.student_name,
			class_name = excluded.class_name,
			part1_answers = excluded.part1_answers,
			updated_at = CURRENT_TIMESTAMP
	`, courseID, studentID, studentName, className, string(answers))
	return err
}

func (a *app) ensureStudentSurveyRecord(courseID int, studentID string) error {
	className, studentName := splitStudentID(studentID)
	_, err := a.db.Exec(`
		INSERT INTO student_surveys (course_id, student_id, student_name, class_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(course_id, student_id) DO UPDATE SET
			student_name = CASE
				WHEN excluded.student_name <> '' THEN excluded.student_name
				ELSE student_surveys.student_name
			END,
			class_name = CASE
				WHEN excluded.class_name <> '' THEN excluded.class_name
				ELSE student_surveys.class_name
			END,
			updated_at = CURRENT_TIMESTAMP
	`, courseID, studentID, studentName, className)
	return err
}

func (a *app) saveStudentPart2(courseID int, studentID, answer string) error {
	if err := a.ensureStudentSurveyRecord(courseID, studentID); err != nil {
		return err
	}
	_, err := a.db.Exec(`UPDATE student_surveys SET part2_answer = ?, updated_at = CURRENT_TIMESTAMP WHERE course_id = ? AND student_id = ?`, answer, courseID, studentID)
	return err
}

func (a *app) saveStudentPart3(courseID int, studentID string, answers json.RawMessage, score int) error {
	if err := a.ensureStudentSurveyRecord(courseID, studentID); err != nil {
		return err
	}
	_, err := a.db.Exec(`UPDATE student_surveys SET part3_answers = ?, part3_score = ?, updated_at = CURRENT_TIMESTAMP WHERE course_id = ? AND student_id = ?`, string(answers), score, courseID, studentID)
	return err
}

func (a *app) saveStudentPart4(courseID int, studentID string, answers json.RawMessage) error {
	if err := a.ensureStudentSurveyRecord(courseID, studentID); err != nil {
		return err
	}
	_, err := a.db.Exec(`UPDATE student_surveys SET part4_answers = ?, submitted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE course_id = ? AND student_id = ?`, string(answers), courseID, studentID)
	return err
}

func (a *app) getStudentSurvey(courseID int, studentID string) (*studentSurvey, error) {
	var s studentSurvey
	var part1, part3, part4 sql.NullString
	var part2, submittedAt sql.NullString
	var part3Score sql.NullInt64
	err := a.db.QueryRow(`
		SELECT id, course_id, student_id, student_name, class_name, part1_answers, part2_answer, part3_answers, part3_score, part4_answers, submitted_at, created_at, updated_at
		FROM student_surveys WHERE course_id = ? AND student_id = ?
	`, courseID, studentID).Scan(&s.ID, &s.CourseID, &s.StudentID, &s.StudentName, &s.ClassName, &part1, &part2, &part3, &part3Score, &part4, &submittedAt, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if part1.Valid {
		s.Part1 = json.RawMessage(part1.String)
	}
	if part2.Valid {
		s.Part2 = ptr(part2.String)
	}
	if part3.Valid {
		s.Part3 = json.RawMessage(part3.String)
	}
	if part3Score.Valid {
		val := int(part3Score.Int64)
		s.Part3Score = &val
	}
	if part4.Valid {
		s.Part4 = json.RawMessage(part4.String)
	}
	if submittedAt.Valid {
		s.SubmittedAt = ptr(submittedAt.String)
	}
	return &s, nil
}

func (a *app) getAllStudentSurveys(courseID int, className string) ([]studentSurvey, error) {
	query := `
		SELECT id, course_id, student_id, student_name, class_name, part1_answers, part2_answer, part3_answers, part3_score, part4_answers, submitted_at, created_at, updated_at
		FROM student_surveys WHERE course_id = ?
	`
	args := []interface{}{courseID}
	if strings.TrimSpace(className) != "" {
		query += ` AND class_name = ?`
		args = append(args, className)
	}
	query += ` ORDER BY class_name, student_name`
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []studentSurvey
	for rows.Next() {
		var s studentSurvey
		var part1, part2, part3, part4, submittedAt sql.NullString
		var part3Score sql.NullInt64
		if err := rows.Scan(&s.ID, &s.CourseID, &s.StudentID, &s.StudentName, &s.ClassName, &part1, &part2, &part3, &part3Score, &part4, &submittedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		if part1.Valid {
			s.Part1 = json.RawMessage(part1.String)
		}
		if part2.Valid {
			s.Part2 = ptr(part2.String)
		}
		if part3.Valid {
			s.Part3 = json.RawMessage(part3.String)
		}
		if part3Score.Valid {
			score := int(part3Score.Int64)
			s.Part3Score = &score
		}
		if part4.Valid {
			s.Part4 = json.RawMessage(part4.String)
		}
		if submittedAt.Valid {
			s.SubmittedAt = ptr(submittedAt.String)
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

func (a *app) getCompletionStats(courseID, part int, className string) (completionStats, error) {
	column := fmt.Sprintf("part%d_answers", part)
	if part == 2 {
		column = "part2_answer"
	}

	total := 0
	if strings.TrimSpace(className) != "" {
		names, err := a.getClassRosterNames(courseID, className)
		if err != nil {
			return completionStats{}, err
		}
		total = len(names)
	} else {
		if err := a.db.QueryRow(`
			SELECT COUNT(*) FROM (
				SELECT DISTINCT class_name || '|' || student_name AS roster_key
				FROM class_lists
				UNION
				SELECT DISTINCT class_name || '|' || student_name AS roster_key
				FROM student_surveys
				WHERE course_id = ?
			)
		`, courseID).Scan(&total); err != nil {
			return completionStats{}, err
		}
	}

	query := fmt.Sprintf(`SELECT SUM(CASE WHEN %s IS NOT NULL THEN 1 ELSE 0 END) FROM student_surveys WHERE course_id = ?`, column)
	args := []interface{}{courseID}
	if strings.TrimSpace(className) != "" {
		query += ` AND class_name = ?`
		args = append(args, className)
	}
	var completed sql.NullInt64
	if err := a.db.QueryRow(query, args...).Scan(&completed); err != nil {
		return completionStats{}, err
	}
	stats := completionStats{Total: total}
	if completed.Valid {
		stats.Completed = int(completed.Int64)
	}
	return stats, nil
}

func (a *app) getStudentHistoryScores(studentID string) ([]historyScore, error) {
	rows, err := a.db.Query(`
		SELECT c.name, c.lesson_number, s.part3_score, s.part1_answers
		FROM student_surveys s
		JOIN courses c ON s.course_id = c.id
		WHERE s.student_id = ? AND s.part3_score IS NOT NULL
		ORDER BY c.lesson_number ASC
	`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []historyScore
	for rows.Next() {
		var name string
		var lessonNo, actualScore int
		var rawPart1 sql.NullString
		if err := rows.Scan(&name, &lessonNo, &actualScore, &rawPart1); err != nil {
			return nil, err
		}
		item := historyScore{CourseName: name, LessonNumber: lessonNo, ActualScore: actualScore}
		if rawPart1.Valid {
			var answers part1Answers
			if err := json.Unmarshal([]byte(rawPart1.String), &answers); err == nil {
				item.PredictedScore = answers.PredictionScore
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *app) getStudentHistoryDetails(studentID string, currentCourseID int) ([]studentHistoryDetail, error) {
	currentCourse, err := a.getCourseByID(currentCourseID)
	if err != nil {
		return nil, err
	}

	rows, err := a.db.Query(`
		SELECT s.course_id, c.name, c.lesson_number, s.student_id, s.student_name, s.class_name,
		       s.part1_answers, s.part2_answer, s.part3_answers, s.part3_score, s.part4_answers,
		       s.submitted_at, s.updated_at
		FROM student_surveys s
		JOIN courses c ON s.course_id = c.id
		WHERE s.student_id = ? AND c.lesson_number <= ?
		ORDER BY c.lesson_number DESC
	`, studentID, currentCourse.LessonNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []studentHistoryDetail
	for rows.Next() {
		var item studentHistoryDetail
		var part1, part2, part3, part4, submittedAt sql.NullString
		var part3Score sql.NullInt64
		if err := rows.Scan(
			&item.CourseID, &item.CourseName, &item.LessonNo, &item.StudentID, &item.StudentName, &item.ClassName,
			&part1, &part2, &part3, &part3Score, &part4, &submittedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if part1.Valid {
			item.Part1 = json.RawMessage(part1.String)
		}
		if part2.Valid {
			item.Part2 = ptr(part2.String)
		}
		if part3.Valid {
			item.Part3 = json.RawMessage(part3.String)
		}
		if part3Score.Valid {
			score := int(part3Score.Int64)
			item.Part3Score = &score
		}
		if part4.Valid {
			item.Part4 = json.RawMessage(part4.String)
		}
		if submittedAt.Valid {
			item.SubmittedAt = ptr(submittedAt.String)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *app) buildStats(courseID int, className string) (statsPayload, error) {
	var stats statsPayload
	var err error
	if stats.Part1, err = a.getCompletionStats(courseID, 1, className); err != nil {
		return stats, err
	}
	if stats.Part2, err = a.getCompletionStats(courseID, 2, className); err != nil {
		return stats, err
	}
	if stats.Part3, err = a.getCompletionStats(courseID, 3, className); err != nil {
		return stats, err
	}
	if stats.Part4, err = a.getCompletionStats(courseID, 4, className); err != nil {
		return stats, err
	}
	students, err := a.getAllStudentSurveys(courseID, className)
	if err != nil {
		return stats, err
	}
	stats.Students = students
	stats.Part1Stats.PredictionScoreDistribution = map[string]int{}
	stats.Part1Stats.LearningMethodsDistribution = map[string]int{}
	stats.Part2Stats.TotalCount = len(students)
	stats.Part3Stats.ScoreDistribution = map[string]int{"0": 0, "1": 0, "2": 0, "3": 0, "4": 0, "5": 0}

	part3Questions, err := a.getQuestionsByPart(courseID, 3)
	if err != nil {
		return stats, err
	}
	for i, q := range part3Questions {
		stats.Part3Stats.QuestionCorrectRate = append(stats.Part3Stats.QuestionCorrectRate, questionRate{
			QuestionID:   q.ID,
			QuestionText: q.QuestionText,
			SortOrder:    i + 1,
		})
	}

	for _, s := range students {
		if len(s.Part1) > 0 {
			var answers part1Answers
			if err := json.Unmarshal(s.Part1, &answers); err == nil {
				stats.Part1Stats.PredictionScoreDistribution[strconv.Itoa(answers.PredictionScore)]++
				for _, method := range answers.LearningMethods {
					stats.Part1Stats.LearningMethodsDistribution[method]++
				}
				if strings.TrimSpace(answers.CustomMethod) != "" {
					stats.Part1Stats.LearningMethodsDistribution["自定义: "+answers.CustomMethod]++
				}
			}
		}
		if s.Part2 != nil && strings.TrimSpace(*s.Part2) != "" {
			stats.Part2Stats.FilledCount++
		}
		if s.Part3Score != nil {
			score := clamp(*s.Part3Score, 0, 5)
			stats.Part3Stats.ScoreDistribution[strconv.Itoa(score)]++
		}
		if len(s.Part3) > 0 {
			var answers map[string]string
			if err := json.Unmarshal(s.Part3, &answers); err == nil {
				for i := range stats.Part3Stats.QuestionCorrectRate {
					key := fmt.Sprintf("q%d", i+1)
					answer, ok := answers[key]
					if !ok {
						continue
					}
					stats.Part3Stats.QuestionCorrectRate[i].TotalCount++
					if i < len(part3Questions) && part3Questions[i].CorrectAnswer != nil && answer == *part3Questions[i].CorrectAnswer {
						stats.Part3Stats.QuestionCorrectRate[i].CorrectCount++
					}
				}
			}
		}
	}
	for i := range stats.Part3Stats.QuestionCorrectRate {
		q := &stats.Part3Stats.QuestionCorrectRate[i]
		if q.TotalCount > 0 {
			q.CorrectRate = int(float64(q.CorrectCount) / float64(q.TotalCount) * 100)
		}
	}
	return stats, nil
}

func buildExportRow(s studentSurvey) []string {
	status := "未开始"
	if len(s.Part1) > 0 {
		status = "完成到第一部分"
	}
	if s.Part2 != nil && strings.TrimSpace(*s.Part2) != "" {
		status = "完成到第二部分"
	}
	if len(s.Part3) > 0 {
		status = "完成到第三部分"
	}
	if len(s.Part4) > 0 {
		status = "已完成全部"
	}

	predictionScore := "未提交"
	learningMethods := "未提交"
	customLearningMethod := "无"
	if len(s.Part1) > 0 {
		var answers part1Answers
		if err := json.Unmarshal(s.Part1, &answers); err == nil {
			predictionScore = fmt.Sprintf("%d分", answers.PredictionScore)
			if len(answers.LearningMethods) > 0 {
				learningMethods = strings.Join(answers.LearningMethods, "、")
			} else {
				learningMethods = "无"
			}
			if strings.TrimSpace(answers.CustomMethod) != "" {
				customLearningMethod = answers.CustomMethod
			}
		}
	}

	part3Answers := map[string]string{}
	if len(s.Part3) > 0 {
		_ = json.Unmarshal(s.Part3, &part3Answers)
	}
	part4 := map[string]interface{}{}
	if len(s.Part4) > 0 {
		_ = json.Unmarshal(s.Part4, &part4)
	}
	actualScore := "未提交"
	predictedScore := "未提交"
	part4Q2Answer := "未提交"
	part4Q2Custom := "无"
	part4Q3Answer := "未提交"
	part4Q3Custom := "无"
	if scoreCompare, ok := part4["scoreCompare"].(map[string]interface{}); ok {
		if v, ok := scoreCompare["actual"]; ok {
			actualScore = fmt.Sprint(v)
		}
		if v, ok := scoreCompare["predicted"]; ok {
			predictedScore = fmt.Sprint(v)
		}
	}
	if q2, ok := part4["q2"].(map[string]interface{}); ok {
		if answers, ok := q2["answers"].([]interface{}); ok {
			part4Q2Answer = joinInterfaces(answers)
		}
		if v, ok := q2["custom"].(string); ok && strings.TrimSpace(v) != "" {
			part4Q2Custom = v
		}
	}
	if q3, ok := part4["q3"].(map[string]interface{}); ok {
		if answers, ok := q3["answers"].([]interface{}); ok {
			part4Q3Answer = joinInterfaces(answers)
		}
		if v, ok := q3["custom"].(string); ok && strings.TrimSpace(v) != "" {
			part4Q3Custom = v
		}
	}

	scoreText := "未完成"
	if s.Part3Score != nil {
		scoreText = fmt.Sprintf("%d/5", *s.Part3Score)
	}

	lastSubmitted := s.UpdatedAt
	return []string{
		truncateCell(s.ClassName),
		truncateCell(s.StudentName),
		truncateCell(status),
		truncateCell(predictionScore),
		truncateCell(learningMethods),
		truncateCell(customLearningMethod),
		truncateCell(formatPart2Answer(deref(s.Part2))),
		truncateCell(valueOr(part3Answers["q1"], "未提交")),
		truncateCell(valueOr(part3Answers["q2"], "未提交")),
		truncateCell(valueOr(part3Answers["q3"], "未提交")),
		truncateCell(valueOr(part3Answers["q4"], "未提交")),
		truncateCell(valueOr(part3Answers["q5"], "未提交")),
		truncateCell(scoreText),
		truncateCell(actualScore),
		truncateCell(predictedScore),
		truncateCell(part4Q2Answer),
		truncateCell(part4Q2Custom),
		truncateCell(part4Q3Answer),
		truncateCell(part4Q3Custom),
		truncateCell(lastSubmitted),
	}
}

func decodeJSON(r *http.Request, target interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(target)
}

func buildPart2StoredAnswer(q question, answer string, answers []string) (stored string, display string, err error) {
	switch q.QuestionType {
	case "text":
		if strings.TrimSpace(answer) == "" {
			return "", "", errors.New("请填写你的答案")
		}
		return answer, answer, nil
	case "multi":
		cleaned := make([]string, 0, len(answers))
		for _, item := range answers {
			item = strings.TrimSpace(item)
			if item != "" {
				cleaned = append(cleaned, item)
			}
		}
		if len(cleaned) == 0 {
			return "", "", errors.New("请至少选择一个答案")
		}
		payload := part2AnswerPayload{
			QuestionType: q.QuestionType,
			Values:       cleaned,
			Labels:       cleaned,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", "", err
		}
		return string(raw), strings.Join(cleaned, "、"), nil
	default:
		if strings.TrimSpace(answer) == "" {
			return "", "", errors.New("请选择一个答案")
		}
		payload := part2AnswerPayload{
			QuestionType: q.QuestionType,
			Value:        answer,
			Label:        answer,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", "", err
		}
		return string(raw), answer, nil
	}
}

func formatPart2Answer(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var payload part2AnswerPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw
	}
	if len(payload.Labels) > 0 {
		return strings.Join(payload.Labels, "、")
	}
	if strings.TrimSpace(payload.Label) != "" {
		return payload.Label
	}
	if len(payload.Values) > 0 {
		return strings.Join(payload.Values, "、")
	}
	if strings.TrimSpace(payload.Value) != "" {
		return payload.Value
	}
	return raw
}

func writeJSON(w http.ResponseWriter, status int, payload apiResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, err error) {
	log.Printf("request failed: %v", err)
	writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) resolveCourseID(r *http.Request) (int, error) {
	if v := r.URL.Query().Get("courseId"); strings.TrimSpace(v) != "" {
		id, err := strconv.Atoi(v)
		if err != nil {
			return 0, errors.New("courseId 无效")
		}
		return id, nil
	}
	return a.getCurrentCourseID()
}

func (a *app) resolveCourseIDFromBody(courseID *int) (int, error) {
	if courseID != nil && *courseID > 0 {
		return *courseID, nil
	}
	return a.getCurrentCourseID()
}

func resolvePath(name string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return name
	}
	candidates := []string{
		filepath.Join(cwd, name),
		filepath.Join(cwd, "..", name),
		filepath.Join(cwd, "..", "..", name),
	}
	for _, candidate := range candidates {
		parent := filepath.Dir(candidate)
		if _, err := os.Stat(parent); err == nil {
			return candidate
		}
	}
	return filepath.Join(cwd, name)
}

func splitStudentID(studentID string) (className, studentName string) {
	parts := strings.Split(studentID, "_")
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return "", parts[0]
	}
	return strings.Join(parts[:len(parts)-1], "_"), parts[len(parts)-1]
}

func ptr[T any](v T) *T {
	return &v
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func emptyToNil(v *string) interface{} {
	if v == nil {
		return nil
	}
	if strings.TrimSpace(*v) == "" {
		return nil
	}
	return *v
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func clamp(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func truncateCell(value string) string {
	value = valueOr(value, "未提交")
	if len(value) <= 32000 {
		return value
	}
	return "[内容过长已截断] " + value[:32000]
}

func joinInterfaces(values []interface{}) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprint(value))
	}
	return strings.Join(parts, "、")
}

func mustPartSettingsMap(a *app, courseID int) map[string]bool {
	settings, err := a.getPartSettings(courseID)
	if err != nil {
		return map[string]bool{
			"1": true,
			"2": true,
			"3": true,
			"4": true,
		}
	}
	result := make(map[string]bool, len(settings))
	for _, setting := range settings {
		result[strconv.Itoa(setting.Part)] = setting.Enabled
	}
	return result
}

func firstEnabledPart(enabled map[int]bool) int {
	for part := 1; part <= 4; part++ {
		if enabled[part] {
			return part
		}
	}
	return 0
}

func urlEncode(value string) string {
	replacer := strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29")
	return replacer.Replace(value)
}
