package v2

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode"
)

// Explicit opt-in: two student actions (one initial page and one edit), no job retries.
func TestDesignHTMLLive(t *testing.T) {
	if os.Getenv("DESIGN_HTML_LIVE_TEST") != "1" {
		t.Skip("live HTML test disabled")
	}
	_, repo, p := setupDesignImages(t)
	p.Title = "小学生AI安全小测"
	p.Questions = []string{"网页要实现什么？", "如何交互？"}
	p.OutputType = "html"
	p, err := repo.SaveDesignPlan(*p)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 1, 1, []string{"AI安全测验", "三道题，显示分数"}); err != nil {
		t.Fatal(err)
	}
	if err = repo.AdvanceDesignProgress(p.ID, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err = repo.SubmitDesignVersion(p.ID, 1, 1, 2, []string{"小学生AI安全小测", "3道单选题，每题3个选项，提交显示分数和解析，重置清空，手机桌面均可使用，离线运行"}); err != nil {
		t.Fatal(err)
	}
	if err = repo.AdvanceDesignProgress(p.ID, 1, 3); err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("DESIGN_HTML_LIVE_OUTPUT_DIR")
	if root == "" {
		root = t.TempDir()
	}
	h := NewHandler(repo)
	if !h.designHTMLConfigured() {
		t.Fatal("text provider key missing")
	}
	if err = h.StartDesignImages(context.Background(), filepath.Join(root, "images")); err != nil {
		t.Fatal(err)
	}
	defer h.StopDesignImages()
	results := []map[string]any{}
	for n, message := range []string{"验收约定：3组单选题name分别为q1、q2、q3；提交按钮id=submit-quiz，重置按钮id=reset-quiz，结果区域id=result。必须是真正的单文件离线交互测验。", "把主题色改为深蓝色，答完后增加一句鼓励语，保留题目、评分、重置及原有控件id。"} {
		start := time.Now()
		key := []string{"live-html-first", "live-html-edit"}[n]
		job, err := repo.EnqueueDesignHTML(p.ID, 1, 1, key, message)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(270 * time.Second)
		for {
			state, err := repo.DesignHTMLState(p.ID, 1, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if state.LatestVersion != nil && state.LatestVersion.JobID == job.ID {
				v := state.LatestVersion
				reasoningChinese := false
				for _, c := range state.LastJob.Reasoning {
					if unicode.Is(unicode.Han, c) {
						reasoningChinese = true
						break
					}
				}
				reasoningChars := len([]rune(state.LastJob.Reasoning))
				results = append(results, map[string]any{"version": v.Version, "model": v.Model, "bytes": v.SizeBytes, "seconds": time.Since(start).Seconds(), "file": filepath.Join(h.designImages.htmlDir, v.FileName), "reasoningChars": reasoningChars, "reasoningChinese": reasoningChinese})
				t.Logf("HTML version=%d model=%s bytes=%d elapsed=%s reasoningChars=%d reasoningChinese=%t", v.Version, v.Model, v.SizeBytes, time.Since(start).Round(time.Millisecond), reasoningChars, reasoningChinese)
				break
			}
			if state.LastJob != nil && (state.LastJob.Status == "failed" || state.LastJob.Status == "interrupted") {
				t.Fatal("live HTML failed; not retried", state.LastJob.Error)
			}
			if time.Now().After(deadline) {
				t.Fatal("live HTML timed out; not retried")
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	data, _ := json.MarshalIndent(results, "", "  ")
	if err = os.WriteFile(filepath.Join(root, "summary.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("HTML_LIVE_OUTPUT=" + root)
}
