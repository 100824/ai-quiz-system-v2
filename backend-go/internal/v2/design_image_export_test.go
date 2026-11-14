package v2

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// Exercise both pagination cursors and repeat exports using synthetic records only.
func TestDesignImageExportPagedHistoryAndClassScope(t *testing.T) {
	db, repo, plan := setupDesignImages(t)
	dir := t.TempDir()
	service, err := newDesignImageService(repo, dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	imageData := designTestPNG(t)
	for _, fixture := range []struct{ student, class, count int }{{1, 1, 25}, {3, 2, 3}} {
		result, err := db.Exec(`INSERT INTO design_image_sessions(plan_id,class_id,student_id,snapshot_json) VALUES(?,?,?,'{}')`, plan.ID, fixture.class, fixture.student)
		if err != nil {
			t.Fatal(err)
		}
		session, _ := result.LastInsertId()
		for version := 1; version <= fixture.count; version++ {
			id := fmt.Sprintf("fixture-%d-%d", fixture.student, version)
			file := id + ".png"
			if err = os.WriteFile(filepath.Join(dir, file), imageData, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO design_image_jobs(id,session_id,request_key,message,prompt,status,created_at,updated_at) VALUES(?,?,?,'fixture','fixture','succeeded',?,?)`, id, session, id, designNow(), designNow()); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO design_images(id,session_id,job_id,version,file_name,width,height,model,created_at) VALUES(?,?,?,?,?,64,96,'fixture',?)`, id, session, id, version, file, designNow()); err != nil {
				t.Fatal(err)
			}
			for _, role := range []string{"user", "assistant"} {
				if _, err = db.Exec(`INSERT INTO design_image_messages(session_id,job_id,role,content,image_id,created_at) VALUES(?,?,?,'fixture',?,?)`, session, id, role, id, designNow()); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	handler := NewHandler(repo)
	handler.designImages = service
	mux := http.NewServeMux()
	handler.Register(mux)
	for _, scope := range []struct {
		query string
		count int
	}{{"?classId=1", 25}, {"", 28}, {"?classId=2", 3}, {"?classId=1", 25}} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/api/v2/design-plans/%d/images-export%s", plan.ID, scope.query), nil))
		if w.Code != http.StatusOK {
			t.Fatalf("export %s: HTTP %d", scope.query, w.Code)
		}
		archive, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
		if err != nil {
			t.Fatal(err)
		}
		images, spreadsheets := 0, 0
		for _, file := range archive.File {
			reader, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, readErr := io.ReadAll(reader) // Also validates each ZIP entry's checksum.
			reader.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if strings.HasSuffix(file.Name, ".png") {
				images++
				if !bytes.Equal(data, imageData) {
					t.Fatal("image bytes changed during export")
				}
			} else if file.Name == "成果清单.xlsx" {
				spreadsheets++
				workbook, err := excelize.OpenReader(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				versions, versionErr := workbook.GetRows("图片版本")
				messages, messageErr := workbook.GetRows("对话记录")
				workbook.Close()
				if versionErr != nil || messageErr != nil || len(versions) != scope.count+1 || len(messages) != 2*scope.count+1 {
					t.Fatalf("export %s lost paged rows: versions=%d messages=%d", scope.query, len(versions), len(messages))
				}
			}
		}
		if images != scope.count || spreadsheets != 1 {
			t.Fatalf("export %s: got %d images and %d spreadsheets", scope.query, images, spreadsheets)
		}
	}
}
