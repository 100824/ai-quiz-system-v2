package v2

import (
	"context"
	"os"
	"testing"
	"time"
)

// Explicit opt-in: at most one initial generation and one image edit, never automatic retries.
func TestDesignImageLive(t *testing.T) {
	if os.Getenv("DESIGN_IMAGE_LIVE_TEST") != "1" {
		t.Skip("live image test disabled")
	}
	if envFirst("ARK_API_KEY", "ARK_KEY") == "" {
		t.Fatal("missing ARK_API_KEY")
	}
	_, repo, p := setupDesignImages(t)
	completeDesignPlans(t, repo, p.ID)
	dir := os.Getenv("DESIGN_IMAGE_LIVE_OUTPUT_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	h := NewHandler(repo)
	if err := h.StartDesignImages(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	defer h.StopDesignImages()
	for n, message := range []string{"生成科技风AI科普海报，中文清晰。", "颜色加深一点，保留文字和版式。"} {
		started := time.Now()
		job, err := repo.EnqueueDesignImage(p.ID, 1, 1, []string{"live-first", "live-edit"}[n], message)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(250 * time.Second)
		for {
			s, err := repo.DesignImageState(p.ID, 1, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if s.LatestImage != nil && s.LatestImage.JobID == job.ID {
				t.Logf("model=%s imageVersion=%d size=%dx%d elapsed=%s file=%s", s.LatestImage.Model, s.LatestImage.Version, s.LatestImage.Width, s.LatestImage.Height, time.Since(started).Round(time.Millisecond), s.LatestImage.FileName)
				break
			}
			for _, m := range s.Messages {
				if m.JobID == job.ID && (m.Status == "failed" || m.Status == "interrupted") {
					t.Fatal(m.Content)
				}
			}
			if time.Now().After(deadline) {
				t.Fatal("image job timed out")
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}
