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
	v2 "ai-quiz-system-v2/backend-go/internal/v2"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	dataDir := utils.ResolvePath("data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	historyDataDir := utils.ResolvePath("history-data")
	if err := os.MkdirAll(historyDataDir, 0o755); err != nil {
		log.Fatalf("create history data dir: %v", err)
	}

	uploadsDir := filepath.Join(dataDir, "uploads", "images")
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		log.Fatalf("create uploads dir: %v", err)
	}
	historyUploadsDir := filepath.Join(historyDataDir, "uploads", "images")
	if err := os.MkdirAll(historyUploadsDir, 0o755); err != nil {
		log.Fatalf("create history uploads dir: %v", err)
	}

	legacyDBPath := filepath.Join(historyDataDir, "quiz-system-legacy.db")
	legacyDB, err := sql.Open("sqlite3", legacyDBPath)
	if err != nil {
		log.Fatalf("open legacy db: %v", err)
	}
	defer legacyDB.Close()

	if err := migrations.ConfigureDB(legacyDB); err != nil {
		log.Fatalf("configure legacy db: %v", err)
	}
	if err := migrations.InitDB(legacyDB); err != nil {
		log.Fatalf("init legacy db: %v", err)
	}

	v2DBPath := filepath.Join(dataDir, "quiz-system-v2.db")
	v2DB, err := sql.Open("sqlite3", v2DBPath)
	if err != nil {
		log.Fatalf("open v2 db: %v", err)
	}
	defer v2DB.Close()

	if err := v2.ConfigureDB(v2DB); err != nil {
		log.Fatalf("configure v2 db: %v", err)
	}

	if err := v2.InitDB(v2DB); err != nil {
		log.Fatalf("init v2 db: %v", err)
	}

	legacyRepo := repository.NewRepository(legacyDB)
	legacySvc := service.NewService(legacyRepo)
	legacyHandler := handler.NewHandler(legacyRepo, legacySvc, historyUploadsDir)
	uploadHandler := handler.NewHandler(legacyRepo, legacySvc, uploadsDir)
	legacyMux := http.NewServeMux()
	registerLegacyRoutes(legacyMux, legacyHandler)

	v2Repo := v2.NewRepository(v2DB)
	v2Handler := v2.NewHandler(v2Repo)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", legacyHandler.HandleHealth)
	mux.Handle("/history/api/", http.StripPrefix("/history", legacyMux))
	mux.HandleFunc("POST /api/teacher/upload-image", uploadHandler.HandleUploadImage)
	mux.HandleFunc("GET /api/uploads/images/{filename}", uploadHandler.HandleServeImage)
	v2Handler.Register(mux)

	appHandler := middleware.WithCORS(middleware.WithLogging(mux))
	port := utils.Getenv("PORT", "8080")
	addr := "0.0.0.0:" + port
	log.Printf("Go backend listening on http://%s", addr)
	if err := http.ListenAndServe(addr, appHandler); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func registerLegacyRoutes(mux *http.ServeMux, h *handler.Handler) {
	mux.HandleFunc("GET /api/teacher/courses", h.HandleTeacherCourses)
	mux.HandleFunc("POST /api/teacher/current-course", h.HandleSetCurrentCourse)
	mux.HandleFunc("GET /api/teacher/class-list/{courseId}/{className}", h.HandleTeacherClassList)
	mux.HandleFunc("POST /api/teacher/class-list", h.HandleImportClassList)
	mux.HandleFunc("GET /api/teacher/classes/{courseId}", h.HandleTeacherClasses)
	mux.HandleFunc("GET /api/teacher/stage/{courseId}", h.HandleTeacherStage)
	mux.HandleFunc("POST /api/teacher/stage", h.HandleSetTeacherStage)
	mux.HandleFunc("POST /api/teacher/bind-class-course", h.HandleBindClassCourse)
	mux.HandleFunc("GET /api/teacher/tip", h.HandleGetTeacherTip)
	mux.HandleFunc("POST /api/teacher/tip", h.HandleTeacherTip)
	mux.HandleFunc("GET /api/teacher/questions/{courseId}/{part}", h.HandleTeacherQuestions)
	mux.HandleFunc("POST /api/teacher/question/{id}", h.HandleUpdateQuestion)
	mux.HandleFunc("POST /api/teacher/question/{id}/enabled", h.HandleSetQuestionEnabled)
	mux.HandleFunc("GET /api/teacher/part2-guide/{courseId}", h.HandleGetPart2Guide)
	mux.HandleFunc("POST /api/teacher/part2-guide/{courseId}", h.HandleSetPart2Guide)
	mux.HandleFunc("GET /api/teacher/part-settings/{courseId}", h.HandleGetPartSettings)
	mux.HandleFunc("POST /api/teacher/part-settings/{courseId}", h.HandleSetPartSettings)
	mux.HandleFunc("GET /api/teacher/stats/{courseId}", h.HandleTeacherStats)
	mux.HandleFunc("GET /api/teacher/stats/export-all", h.HandleExportAllStats)
	mux.HandleFunc("GET /api/teacher/stats/export-prediction-summary", h.HandleExportPredictionSummary)
	mux.HandleFunc("GET /api/teacher/stats/{courseId}/export", h.HandleExportStats)
	mux.HandleFunc("POST /api/teacher/manual-score", h.HandleTeacherManualScore)
	mux.HandleFunc("POST /api/student/validate", h.HandleStudentValidate)
	mux.HandleFunc("GET /api/student/tip", h.HandleStudentTip)
	mux.HandleFunc("GET /api/student/questions/{part}", h.HandleStudentQuestions)
	mux.HandleFunc("GET /api/student-history/{studentId}", h.HandleStudentHistoryDetails)
	mux.HandleFunc("GET /api/student/{studentId}", h.HandleStudentStatus)
	mux.HandleFunc("POST /api/student/{studentId}/part1", h.HandleStudentPart1)
	mux.HandleFunc("POST /api/student/{studentId}/part2", h.HandleStudentPart2)
	mux.HandleFunc("POST /api/student/{studentId}/part3", h.HandleStudentPart3)
	mux.HandleFunc("POST /api/student/{studentId}/part4", h.HandleStudentPart4)
	mux.HandleFunc("POST /api/teacher/upload-image", h.HandleUploadImage)
	mux.HandleFunc("GET /api/uploads/images/{filename}", h.HandleServeImage)
}
