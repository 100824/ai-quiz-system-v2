package v2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const designHTMLPromptTarget = 1 << 20 // Prompt guidance only; never a response/file rejection threshold.
const designHTMLReasoningLimit = 12000
const designHTMLPolicy = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; media-src data: blob:; connect-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; worker-src 'none'"
const designHTMLSystem = `你为小学学生生成可直接使用的离线交互网页。若提供可展示的生成思路(reasoning_content)，请用面向小学生的简短中文说明页面内容与交互的设计要点，不要只复述任务，不要使用英文思路。
最终回答(content)只返回一份完整HTML文档，以<!doctype html>开头并以</html>结束，不要Markdown、开场白、设计说明或结语。公开思路由接口独立的reasoning_content字段提供，不要在content中再次输出思路，不要把思路放进网页。
HTML、CSS、JavaScript全部内联，使用系统字体、CSS或内联SVG绘图。不能使用外链、CDN、网络请求、模块导入、iframe、eval、new Function、弹窗、跳转、Cookie或localStorage。按钮与测验使用DOM更新和内存状态，避免alert和表单提交。页面必须有真实可见内容和可用交互，适配手机与桌面，防止无界循环。
请将完整单文件HTML控制在1MiB以内，使用精简CSS、JavaScript和SVG，避免大段重复内容或巨大内联素材。
仅把学生要求呈现的内容放入网页，不显示问卷说明、示例和版本标记。若有上一版HTML，按本次要求修改并返回完整新文档，保留未要求改变的功能。
遇到与设计无关或含义不清的输入，不猜测学生想要删除哪些功能，不添加说教或说明，保留原方案或上一版功能并返回完整网页。`

var cssURLs = regexp.MustCompile(`(?is)url\s*\(\s*([^)]*)\)`)
var networkJS = regexp.MustCompile(`(?i)\b(fetch|XMLHttpRequest|WebSocket|EventSource|importScripts|import)\s*\(|\bnavigator\s*\.\s*sendBeacon\s*\(`)
var unsafeDesignJS = regexp.MustCompile(`\b(eval|Function|Worker|SharedWorker|alert|confirm|prompt)\s*\(|\bwindow\s*\.\s*open\s*\(|\bdocument\s*\.\s*write\s*\(|\bwhile\s*\(\s*(true|1)\s*\)|\bfor\s*\(\s*;\s*;\s*\)`)

func inlineDesignURL(value string) bool {
	v := strings.ToLower(strings.Trim(strings.TrimSpace(value), `"'`))
	return v == "" || strings.HasPrefix(v, "#") || strings.HasPrefix(v, "data:image/") || strings.HasPrefix(v, "data:font/") || strings.HasPrefix(v, "data:audio/") || strings.HasPrefix(v, "data:video/")
}

func validateDesignHTML(source string) (string, error) {
	source = strings.TrimSpace(source)
	source = strings.TrimPrefix(source, "\ufeff")
	var err error
	source, err = extractDesignHTMLDocument(source)
	if err != nil {
		return "", err
	}
	if !utf8.ValidString(source) || strings.ContainsRune(source, '\x00') || strings.ContainsRune(source, utf8.RuneError) {
		return "", fmt.Errorf("网页编码无效，包含损坏字符")
	}
	if strings.HasPrefix(strings.ToLower(source), "<html") {
		source = "<!doctype html>\n" + source
	}
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", fmt.Errorf("网页解析失败")
	}
	var head *html.Node
	var visit func(*html.Node) error
	visit = func(n *html.Node) error {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "head":
				head = n
			case "iframe", "frame", "frameset", "object", "embed", "base", "link", "portal":
				return fmt.Errorf("网页包含不支持的外部资源或嵌套内容")
			}
			for _, a := range n.Attr {
				key, value := strings.ToLower(a.Key), strings.TrimSpace(a.Val)
				if strings.HasPrefix(key, "on") && (networkJS.MatchString(value) || unsafeDesignJS.MatchString(value)) {
					return fmt.Errorf("网页事件包含不支持的脚本")
				}
				switch key {
				case "src", "href", "poster", "background", "action", "formaction", "data":
					if !inlineDesignURL(value) || (n.Data == "script" && key == "src" && value != "") {
						return fmt.Errorf("网页必须使用内联资源")
					}
				case "srcset":
					if value != "" {
						return fmt.Errorf("网页不支持外部图片列表")
					}
				case "http-equiv":
					if strings.EqualFold(value, "refresh") {
						return fmt.Errorf("网页不能自动跳转")
					}
				}
				if key == "style" {
					if err := validateDesignCSS(value); err != nil {
						return err
					}
				}
			}
		}
		if n.Type == html.TextNode && n.Parent != nil {
			if n.Parent.Data == "style" {
				if err := validateDesignCSS(n.Data); err != nil {
					return err
				}
			}
			if n.Parent.Data == "script" {
				if networkJS.MatchString(n.Data) || unsafeDesignJS.MatchString(n.Data) {
					return fmt.Errorf("网页包含不支持的网络、弹窗或高风险脚本")
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err = visit(doc); err != nil {
		return "", err
	}
	if head == nil {
		return "", fmt.Errorf("网页缺少head")
	}
	// Replace model-provided CSPs with the platform policy before any executable content.
	for n := head.FirstChild; n != nil; {
		next := n.NextSibling
		if n.Type == html.ElementNode && n.Data == "meta" {
			for _, a := range n.Attr {
				if a.Key == "http-equiv" && strings.EqualFold(a.Val, "Content-Security-Policy") {
					head.RemoveChild(n)
					break
				}
			}
		}
		n = next
	}
	meta := &html.Node{Type: html.ElementNode, Data: "meta", Attr: []html.Attribute{{Key: "http-equiv", Val: "Content-Security-Policy"}, {Key: "content", Val: designHTMLPolicy}}}
	head.InsertBefore(meta, head.FirstChild)
	charset := &html.Node{Type: html.ElementNode, Data: "meta", Attr: []html.Attribute{{Key: "charset", Val: "utf-8"}}}
	head.InsertBefore(charset, head.FirstChild)
	var out bytes.Buffer
	if err = html.Render(&out, doc); err != nil {
		return "", err
	}
	return out.String(), nil
}

// Extract the document from optional model notes/fences. Only real HTML tokens
// define its boundaries: text inside script/textarea/comment is never a closing
// root. Inline references to <html> in explanatory prose are not documents.
func extractDesignHTMLDocument(source string) (string, error) {
	tokens := html.NewTokenizer(strings.NewReader(source))
	offset, doctype, root, bestStart, bestEnd := 0, -1, -1, -1, -1
	for {
		kind := tokens.Next()
		if kind == html.ErrorToken {
			if tokens.Err() != io.EOF {
				return "", fmt.Errorf("网页解析失败")
			}
			if root >= 0 {
				return "", designArtifactError("模型返回的网页没有完整结束，请重试。")
			}
			if bestStart >= 0 {
				return strings.TrimSpace(source[bestStart:bestEnd]), nil
			}
			return "", designArtifactError("模型未返回HTML网页，请填写具体的设计或修改要求后重试。")
		}
		start := offset
		offset += len(tokens.Raw())
		if kind == html.DoctypeToken && root < 0 {
			fields := strings.Fields(tokens.Token().Data)
			if len(fields) > 0 && strings.EqualFold(fields[0], "html") {
				doctype = start
			}
			continue
		}
		if kind == html.StartTagToken {
			name, _ := tokens.TagName()
			if string(name) == "html" && root < 0 {
				lineStart := strings.LastIndexByte(source[:start], '\n') + 1
				if doctype >= 0 {
					root = doctype
				} else if strings.TrimSpace(source[lineStart:start]) == "" {
					root = start
				}
				doctype = -1
			} else if root < 0 {
				doctype = -1
			}
		} else if kind == html.EndTagToken {
			name, _ := tokens.TagName()
			if string(name) == "html" && root >= 0 {
				bestStart, bestEnd = root, offset
				root = -1
				doctype = -1
			}
		} else if kind == html.TextToken && root < 0 && strings.TrimSpace(string(tokens.Text())) != "" {
			doctype = -1
		}
	}
}

func validateDesignCSS(css string) error {
	if strings.Contains(strings.ToLower(css), "@import") {
		return fmt.Errorf("网页样式必须内联")
	}
	for _, match := range cssURLs.FindAllStringSubmatch(css, -1) {
		if !inlineDesignURL(match[1]) {
			return fmt.Errorf("网页样式不能依赖外部资源")
		}
	}
	return nil
}

func (s *DesignImageService) htmlPath(name string) (string, error) {
	if name == "" || filepath.Base(name) != name {
		return "", fmt.Errorf("网页路径无效")
	}
	return filepath.Join(s.htmlDir, name), nil
}

func (s *DesignImageService) generateHTML(ctx context.Context, j *DesignHTMLJob) (*DesignHTMLVersion, error) {
	if err := checkDesignStorage(s.htmlDir, designHTMLPromptTarget); err != nil {
		return nil, err
	}
	if _, err := normalizeDesignText(j.Prompt, "生成要求", designPromptLimit, true); err != nil {
		return nil, err
	}
	h := s.htmlHandler
	if h == nil {
		h = NewHandler(s.repo)
	}
	primary, err := s.repo.GetPrimaryAIProvider()
	if err != nil {
		return nil, err
	}
	providers := h.aiProviders()
	if primary == "glm" {
		providers[0], providers[1] = providers[1], providers[0]
	}
	history := []AIChatMessage{}
	if j.SourceVersionID != "" {
		previous, err := scanDesignHTMLVersion(s.repo.db.QueryRow(`SELECT `+designHTMLVersionColumns+` FROM design_html_versions WHERE id=? AND session_id=?`, j.SourceVersionID, j.SessionID))
		if err != nil {
			return nil, err
		}
		path, err := s.htmlPath(previous.FileName)
		if err != nil {
			return nil, err
		}
		data, err := readDesignArtifact(path, 0)
		if err != nil {
			return nil, err
		}
		history = append(history, AIChatMessage{Role: "assistant", Content: string(data)})
	}
	history = append(history, AIChatMessage{Role: "user", Content: j.Prompt})
	var lastError error
	for _, provider := range providers {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if !provider.Configured {
			continue
		}
		timeout := 120 * time.Second
		if h.aiProviderTimeout > 0 {
			timeout = h.aiProviderTimeout
		}
		attempt, cancel := context.WithTimeout(ctx, timeout)
		// Persist a bounded preview at most once per second (plus real phase changes).
		// Each provider attempt starts fresh so fallback never mixes different models' reasoning.
		var reasoning []rune
		truncated, progressErrorLogged := false, false
		var lastWrite time.Time
		phase := ""
		publish := func(next string, force bool) {
			if !force && next == phase && time.Since(lastWrite) < time.Second {
				return
			}
			phase, lastWrite = next, time.Now()
			text := string(reasoning)
			if truncated {
				text += "\n（后续思路已省略）"
			}
			if err := s.repo.UpdateDesignHTMLProgress(j.ID, provider.Model, phase, text); err != nil && !progressErrorLogged {
				log.Printf("design HTML progress update failed: %s", j.ID)
				progressErrorLogged = true
			}
		}
		publish("requesting", true)
		answer, e := h.streamProviderOutput(attempt, provider, designHTMLSystem, history, func(string) error {
			publish("writing", false)
			return nil
		}, func(delta string) error {
			remaining := designHTMLReasoningLimit - len(reasoning)
			if remaining > 0 {
				chunk := []rune(delta)
				if len(chunk) > remaining {
					chunk, truncated = chunk[:remaining], true
				}
				reasoning = append(reasoning, chunk...)
			} else {
				truncated = true
			}
			publish("thinking", false)
			return nil
		}, 16384, -1)
		cancel()
		responseBytes := len(answer)
		if e == nil {
			publish("validating", true)
			answer, e = validateDesignHTML(answer)
		}
		if e != nil {
			lastError = e
			publish(phase, true)
			log.Printf("design HTML provider %s failed: %v (job=%s bytes=%d)", provider.ID, e, j.ID, responseBytes)
			continue
		}
		publish("saving", true)
		v := &DesignHTMLVersion{ID: designRandomID(), Model: provider.Model, SizeBytes: len(answer)}
		v.FileName = v.ID + ".html"
		if e = writeDesignHTMLFile(s.htmlDir, v.FileName, []byte(answer)); e != nil {
			return nil, e
		}
		return v, nil
	}
	var bounded designArtifactError
	if errors.As(lastError, &bounded) {
		return nil, bounded
	}
	return nil, fmt.Errorf("网页模型调用或文档校验失败")
}

func writeDesignHTMLFile(dir, name string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".pending-*.html")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

func (s *DesignImageService) runHTML(parent context.Context, j *DesignHTMLJob) {
	defer func() {
		if failure := recover(); failure != nil {
			log.Printf("design HTML job %s panic (%T)", j.ID, failure)
			_ = s.repo.FailDesignHTMLJob(j, "网页生成出现异常，请稍后重试。")
		}
	}()
	ctx, cancel := context.WithTimeout(parent, 260*time.Second)
	defer cancel()
	v, err := s.generateHTML(ctx, j)
	if err == nil {
		err = s.repo.CompleteDesignHTMLJob(j, v)
		if err != nil {
			if p, e := s.htmlPath(v.FileName); e == nil {
				_ = os.Remove(p)
			}
		}
	}
	if err != nil {
		log.Printf("design HTML job %s failed: %v", j.ID, err)
		text := "网页生成失败，请重试。"
		var bounded designArtifactError
		if errors.As(err, &bounded) {
			text = bounded.Error()
		}
		if ctx.Err() != nil {
			text = "网页生成超时或中断，请重试。"
		}
		if e := s.repo.FailDesignHTMLJob(j, text); e != nil {
			log.Printf("design HTML job state update failed: %s", j.ID)
		}
	}
}

func (s *DesignImageService) claimNextGeneration() (*DesignImageJob, *DesignHTMLJob, error) {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()
	var kind string
	err := s.repo.db.QueryRow(`SELECT kind FROM (SELECT 'image' AS kind,created_at,id FROM design_image_jobs WHERE status='queued' UNION ALL SELECT 'html' AS kind,created_at,id FROM design_html_jobs WHERE status='queued') ORDER BY created_at,id,kind LIMIT 1`).Scan(&kind)
	if err != nil {
		return nil, nil, err
	}
	if kind == "html" {
		j, e := s.repo.ClaimDesignHTMLJob()
		return nil, j, e
	}
	j, e := s.repo.ClaimDesignImageJob()
	return j, nil, e
}
