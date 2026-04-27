package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"

	"ai-quiz-system-v2/backend-go/internal/handler"
	"ai-quiz-system-v2/backend-go/internal/middleware"
	"ai-quiz-system-v2/backend-go/internal/migrations"
	"ai-quiz-system-v2/backend-go/internal/repository"
	"ai-quiz-system-v2/backend-go/internal/service"
	"ai-quiz-system-v2/backend-go/internal/utils"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	dataDir := utils.ResolvePath("data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	dbPath := filepath.Join(dataDir, "quiz-system.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := migrations.ConfigureDB(db); err != nil {
		log.Fatalf("configure db: %v", err)
	}

	if err := migrations.InitDB(db); err != nil {
		log.Fatalf("init db: %v", err)
	}

	repo := repository.NewRepository(db)
	svc := service.NewService(repo)
	h := handler.NewHandler(repo, svc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.HandleHealth)
	mux.HandleFunc("GET /api/teacher/courses", h.HandleTeacherCourses)
	mux.HandleFunc("POST /api/teacher/current-course", h.HandleSetCurrentCourse)
	mux.HandleFunc("GET /api/teacher/class-list/{courseId}/{className}", h.HandleTeacherClassList)
	mux.HandleFunc("POST /api/teacher/class-list", h.HandleImportClassList)
	mux.HandleFunc("GET /api/teacher/classes/{courseId}", h.HandleTeacherClasses)
	mux.HandleFunc("GET /api/teacher/stage/{courseId}", h.HandleTeacherStage)
	mux.HandleFunc("POST /api/teacher/stage", h.HandleSetTeacherStage)
	mux.HandleFunc("POST /api/teacher/bind-class-course", h.HandleBindClassCourse)
	mux.HandleFunc("POST /api/teacher/tip", h.HandleTeacherTip)
	mux.HandleFunc("GET /api/teacher/questions/{courseId}/{part}", h.HandleTeacherQuestions)
	mux.HandleFunc("POST /api/teacher/question/{id}", h.HandleUpdateQuestion)
	mux.HandleFunc("POST /api/teacher/question/{id}/enabled", h.HandleSetQuestionEnabled)
	mux.HandleFunc("GET /api/teacher/part2-guide/{courseId}", h.HandleGetPart2Guide)
	mux.HandleFunc("POST /api/teacher/part2-guide/{courseId}", h.HandleSetPart2Guide)
	mux.HandleFunc("GET /api/teacher/part-settings/{courseId}", h.HandleGetPartSettings)
	mux.HandleFunc("POST /api/teacher/part-settings/{courseId}", h.HandleSetPartSettings)
	mux.HandleFunc("GET /api/teacher/stats/{courseId}", h.HandleTeacherStats)
	mux.HandleFunc("GET /api/teacher/stats/{courseId}/export", h.HandleExportStats)
	mux.HandleFunc("POST /api/student/validate", h.HandleStudentValidate)
	mux.HandleFunc("GET /api/student/tip", h.HandleStudentTip)
	mux.HandleFunc("GET /api/student/questions/{part}", h.HandleStudentQuestions)
	mux.HandleFunc("GET /api/student-history/{studentId}", h.HandleStudentHistoryDetails)
	mux.HandleFunc("GET /api/student/{studentId}", h.HandleStudentStatus)
	mux.HandleFunc("POST /api/student/{studentId}/part1", h.HandleStudentPart1)
	mux.HandleFunc("POST /api/student/{studentId}/part2", h.HandleStudentPart2)
	mux.HandleFunc("POST /api/student/{studentId}/part3", h.HandleStudentPart3)
	mux.HandleFunc("POST /api/student/{studentId}/part4", h.HandleStudentPart4)

	appHandler := middleware.WithCORS(middleware.WithLogging(mux))
	port := utils.Getenv("PORT", "8080")
	addr := "0.0.0.0:" + port
	log.Printf("Go backend listening on http://%s", addr)
	if err := http.ListenAndServe(addr, appHandler); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
