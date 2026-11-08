package v2

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	designRequestLimit   = 128 << 10
	designTitleLimit     = 120
	designQuestionLimit  = 1000
	designQuestionCount  = 20
	designQuestionsTotal = 4000
	designAnswerLimit    = 4000
	designAnswersTotal   = 12000
	designMessageLimit   = 4000
	designPromptLimit    = 24000
	designJobsPerMinute  = 10
	designPendingLimit   = 200
)

var designRequestKey = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

// Keep punctuation, code, newlines and emoji. Reject malformed text and invisible
// controls, rather than attempting to blacklist words or SQL/HTML punctuation.
func normalizeDesignText(text, label string, limit int, required bool) (string, error) {
	if !utf8.ValidString(text) {
		return "", fmt.Errorf("%s包含无效字符，请重新输入", label)
	}
	if utf8.RuneCountInString(text) > limit {
		return "", fmt.Errorf("%s最多%d字", label, limit)
	}
	visible := false
	for _, c := range text {
		if c == utf8.RuneError || (unicode.IsControl(c) && c != '\n' && c != '\r' && c != '\t') || c == '\u200b' || c == '\ufeff' || (c >= '\u202a' && c <= '\u202e') || (c >= '\u2066' && c <= '\u2069') {
			return "", fmt.Errorf("%s包含不可见控制字符，请删除后重试", label)
		}
		if !unicode.IsSpace(c) && !unicode.Is(unicode.Cf, c) && !unicode.Is(unicode.Mn, c) && !unicode.Is(unicode.Me, c) {
			visible = true
		}
	}
	if !visible && required {
		return "", fmt.Errorf("%s不能为空", label)
	}
	if !visible {
		return "", nil
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")), nil
}

func normalizeDesignAnswers(answers []string) ([]string, error) {
	if len(answers) == 0 || len(answers) > designQuestionCount {
		return nil, fmt.Errorf("问题数量无效，最多%d道", designQuestionCount)
	}
	result, total := make([]string, len(answers)), 0
	for n, answer := range answers {
		value, err := normalizeDesignText(answer, fmt.Sprintf("第%d题内容", n+1), designAnswerLimit, true)
		if err != nil {
			return nil, err
		}
		result[n] = value
		total += utf8.RuneCountInString(value)
	}
	if total > designAnswersTotal {
		return nil, fmt.Errorf("全部内容合计最多%d字，请精简后重试", designAnswersTotal)
	}
	return result, nil
}

func validateDesignSnapshot(v DesignVersion) error {
	if len(v.Questions) == 0 || len(v.Answers) != len(v.Questions) || len(v.Questions) > designQuestionCount {
		return fmt.Errorf("方案内容不完整，请联系老师")
	}
	if _, err := normalizeDesignText(v.Title, "方案标题", designTitleLimit, true); err != nil {
		return err
	}
	total := 0
	for n, q := range v.Questions {
		if _, err := normalizeDesignText(q, fmt.Sprintf("第%d道问题", n+1), designQuestionLimit, true); err != nil {
			return err
		}
		total += utf8.RuneCountInString(q)
	}
	if total > designQuestionsTotal {
		return fmt.Errorf("问题总长度最多%d字，请联系老师", designQuestionsTotal)
	}
	_, err := normalizeDesignAnswers(v.Answers)
	return err
}

func readDesignArtifact(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, designArtifactError("原作品无法读取，请联系老师")
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || (limit > 0 && stat.Size() > int64(limit)) {
		return nil, designArtifactError("原作品内容超限或无效，请联系老师")
	}
	var reader io.Reader = f
	if limit > 0 {
		reader = io.LimitReader(f, int64(limit)+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil || (limit > 0 && len(data) > limit) {
		return nil, designArtifactError("原作品内容超限或读取失败，请联系老师")
	}
	return data, nil
}

func checkDesignStorage(dir string, required uint64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return designArtifactError("作品存储暂不可用，请联系老师")
	}
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	const reserve = 64 << 20 // Leave space for SQLite, logs and atomic file writes.
	if available < reserve || required > available-reserve {
		return designArtifactError("作品存储空间不足，请联系老师")
	}
	return nil
}

// Read the entire bounded body so a valid first object cannot hide trailing JSON,
// oversized whitespace or invalid UTF-8 beyond it.
func readDesignJSON(w http.ResponseWriter, r *http.Request, value any) error {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, designRequestLimit))
	if err != nil {
		return fmt.Errorf("请求内容过大或读取失败，请精简后重试")
	}
	if !utf8.Valid(data) || json.Unmarshal(data, value) != nil {
		return fmt.Errorf("请求内容格式无效，请检查后重试")
	}
	return nil
}

func checkDesignGenerationBudget(tx *sql.Tx, studentID int) error {
	cutoff := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	var recent, pending int
	err := tx.QueryRow(`SELECT (SELECT COUNT(*) FROM design_image_jobs j JOIN design_image_sessions s ON s.id=j.session_id WHERE s.student_id=? AND j.created_at>=?)+(SELECT COUNT(*) FROM design_html_jobs j JOIN design_html_sessions s ON s.id=j.session_id WHERE s.student_id=? AND j.created_at>=?)`, studentID, cutoff, studentID, cutoff).Scan(&recent)
	if err != nil {
		return err
	}
	if recent >= designJobsPerMinute {
		return fmt.Errorf("操作过于频繁，请稍等一分钟后重试")
	}
	err = tx.QueryRow(`SELECT (SELECT COUNT(*) FROM design_image_jobs WHERE status IN ('queued','running'))+(SELECT COUNT(*) FROM design_html_jobs WHERE status IN ('queued','running'))`).Scan(&pending)
	if err != nil {
		return err
	}
	if pending >= designPendingLimit {
		return fmt.Errorf("生成任务较多，请稍后重试")
	}
	return nil
}
