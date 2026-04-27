package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Ptr returns a pointer to v.
func Ptr[T any](v T) *T {
	return &v
}

// Deref returns the string value or "" if nil.
func Deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// EmptyToNil converts an empty or whitespace-only *string to nil for SQL.
func EmptyToNil(v *string) interface{} {
	if v == nil {
		return nil
	}
	if strings.TrimSpace(*v) == "" {
		return nil
	}
	return *v
}

// MustJSON marshals v to JSON, ignoring errors.
func MustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Getenv reads an environment variable, returning fallback if unset or empty.
func Getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// Clamp restricts v to the inclusive range [minV, maxV].
func Clamp(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

// Fallback returns fb when value is empty or whitespace.
func Fallback(value, fb string) string {
	if strings.TrimSpace(value) == "" {
		return fb
	}
	return value
}

// ValueOr returns fb when value is empty or whitespace.
func ValueOr(value, fb string) string {
	if strings.TrimSpace(value) == "" {
		return fb
	}
	return value
}

// TruncateCell truncates cell content to 32KB for Excel safety.
func TruncateCell(value string) string {
	value = ValueOr(value, "未提交")
	if len(value) <= 32000 {
		return value
	}
	return "[内容过长已截断] " + value[:32000]
}

// JoinInterfaces joins a slice of interface{} values with "、".
func JoinInterfaces(values []interface{}) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprint(value))
	}
	return strings.Join(parts, "、")
}

// FirstEnabledPart returns the first enabled part number (1-4) or 0.
func FirstEnabledPart(enabled map[int]bool) int {
	for part := 1; part <= 4; part++ {
		if enabled[part] {
			return part
		}
	}
	return 0
}

// URLEncode does a minimal URL encoding for Chinese filenames.
func URLEncode(value string) string {
	replacer := strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29")
	return replacer.Replace(value)
}

// SplitStudentID splits "class_name" from the last "_student_name" segment.
func SplitStudentID(studentID string) (className, studentName string) {
	parts := strings.Split(studentID, "_")
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return "", parts[0]
	}
	return strings.Join(parts[:len(parts)-1], "_"), parts[len(parts)-1]
}

// ResolvePath finds the data directory relative to the working directory.
func ResolvePath(name string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return name
	}
	candidates := []string{
		filepath.Join(cwd, "backend-go", name), // from project root
		filepath.Join(cwd, name),               // from backend-go dir
		filepath.Join(cwd, "..", name),
		filepath.Join(cwd, "..", "..", name),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join(cwd, "backend-go", name)
}

// DecodeJSON decodes the request body into target.
func DecodeJSON(r *http.Request, target interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(target)
}

// ParseRawFlexibleInt parses an int that may be quoted in JSON.
func ParseRawFlexibleInt(raw json.RawMessage) (int, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return 0, nil
	}
	if strings.HasPrefix(text, "\"") && strings.HasSuffix(text, "\"") {
		unquoted, err := strconv.Unquote(text)
		if err != nil {
			return 0, err
		}
		text = strings.TrimSpace(unquoted)
		if text == "" {
			return 0, nil
		}
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return 0, err
	}
	return value, nil
}

// StripHTMLTags removes HTML tags and decodes basic entities.
func StripHTMLTags(raw string) string {
	replacer := strings.NewReplacer(
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
		"</div>", "\n",
		"</p>", "\n",
		"&nbsp;", " ",
	)
	normalized := replacer.Replace(raw)
	var builder strings.Builder
	inTag := false
	for _, ch := range normalized {
		switch ch {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				builder.WriteRune(ch)
			}
		}
	}
	lines := strings.Split(builder.String(), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		text := strings.TrimSpace(HTMLUnescape(line))
		if text != "" {
			cleaned = append(cleaned, text)
		}
	}
	return strings.Join(cleaned, "\n")
}

// HTMLUnescape performs minimal HTML entity decoding.
func HTMLUnescape(raw string) string {
	replacer := strings.NewReplacer(
		"&lt;", "<",
		"&gt;", ">",
		"&amp;", "&",
		"&quot;", "\"",
		"&#39;", "'",
	)
	return replacer.Replace(raw)
}

// CountPart2Annotations counts highlight spans in a part-2 HTML answer.
func CountPart2Annotations(raw string) int {
	return strings.Count(raw, "part2-highlight--green") +
		strings.Count(raw, "part2-highlight--yellow") +
		strings.Count(raw, "part2-highlight--red")
}

