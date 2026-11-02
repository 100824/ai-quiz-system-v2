package v2

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
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
	NewHandler(repo).Register(mux)
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
