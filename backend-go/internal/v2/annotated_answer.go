package v2

import (
	"encoding/json"
	"fmt"
	stdhtml "html"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var openTextColorLabels = map[string]string{
	"part2-highlight--green":  "绿色",
	"part2-highlight--yellow": "黄色",
	"part2-highlight--red":    "红色",
}

type annotatedAnswerContent struct {
	HTML           string
	PlainText      string
	ExportText     string
	HighlightCount int
}

func normalizeOpenTextQuestion(q Question) Question {
	if q.Type != "open_text" {
		return q
	}
	rules := map[string]interface{}{}
	if len(q.Rules) > 0 {
		_ = json.Unmarshal(q.Rules, &rules)
	}
	rules["annotationEnabled"] = true
	rules["annotationRequired"] = true
	q.Rules, _ = json.Marshal(rules)
	q.CorrectAnswer = json.RawMessage("null")
	return q
}

func normalizeOpenTextAnswer(raw json.RawMessage) (json.RawMessage, error) {
	var answer string
	if err := json.Unmarshal(raw, &answer); err != nil {
		return nil, fmt.Errorf("开放题答案格式无效")
	}
	content, err := sanitizeAnnotatedAnswer(answer)
	if err != nil {
		return nil, fmt.Errorf("开放题答案格式无效")
	}
	if strings.TrimSpace(content.PlainText) == "" {
		return nil, fmt.Errorf("请填写开放题答案")
	}
	if content.HighlightCount == 0 {
		return nil, fmt.Errorf("开放题至少需要标注一处颜色")
	}
	encoded, err := json.Marshal(content.HTML)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func sanitizeAnnotatedAnswer(value string) (annotatedAnswerContent, error) {
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := xhtml.ParseFragment(strings.NewReader(value), context)
	if err != nil {
		return annotatedAnswerContent{}, err
	}
	var content annotatedAnswerContent
	for _, node := range nodes {
		part := sanitizeAnnotatedNode(node, false)
		content.HTML += part.HTML
		content.PlainText += part.PlainText
		content.ExportText += part.ExportText
		content.HighlightCount += part.HighlightCount
	}
	content.HTML = trimTrailingBreaks(content.HTML)
	content.PlainText = normalizeAnswerText(content.PlainText)
	content.ExportText = normalizeAnswerText(content.ExportText)
	return content, nil
}

func sanitizeAnnotatedNode(node *xhtml.Node, insideHighlight bool) annotatedAnswerContent {
	if node == nil {
		return annotatedAnswerContent{}
	}
	if node.Type == xhtml.TextNode {
		return annotatedAnswerContent{
			HTML:       stdhtml.EscapeString(node.Data),
			PlainText:  node.Data,
			ExportText: node.Data,
		}
	}
	if node.Type != xhtml.ElementNode {
		return annotatedAnswerContent{}
	}
	tag := strings.ToLower(node.Data)
	if tag == "script" || tag == "style" {
		return annotatedAnswerContent{}
	}
	if tag == "br" {
		return annotatedAnswerContent{HTML: "<br>", PlainText: "\n", ExportText: "\n"}
	}

	className, colorLabel := allowedHighlightClass(node)
	childInsideHighlight := insideHighlight || className != ""
	var children annotatedAnswerContent
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		part := sanitizeAnnotatedNode(child, childInsideHighlight)
		children.HTML += part.HTML
		children.PlainText += part.PlainText
		children.ExportText += part.ExportText
		children.HighlightCount += part.HighlightCount
	}
	if className != "" && !insideHighlight && strings.TrimSpace(children.PlainText) != "" {
		children.HTML = `<span class="part2-highlight ` + className + `">` + children.HTML + `</span>`
		children.ExportText = "[" + colorLabel + "]" + children.ExportText + "[/" + colorLabel + "]"
		children.HighlightCount++
	}
	if (tag == "div" || tag == "p") && strings.TrimSpace(children.PlainText) != "" {
		children.HTML += "<br>"
		children.PlainText += "\n"
		children.ExportText += "\n"
	}
	return children
}

func allowedHighlightClass(node *xhtml.Node) (string, string) {
	if node == nil || strings.ToLower(node.Data) != "span" {
		return "", ""
	}
	for _, attr := range node.Attr {
		if strings.ToLower(attr.Key) != "class" {
			continue
		}
		for _, className := range strings.Fields(attr.Val) {
			if label, ok := openTextColorLabels[className]; ok {
				return className, label
			}
		}
	}
	return "", ""
}

func trimTrailingBreaks(value string) string {
	value = strings.TrimSpace(value)
	for strings.HasSuffix(strings.ToLower(value), "<br>") {
		value = strings.TrimSpace(value[:len(value)-4])
	}
	return value
}

func normalizeAnswerText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	for strings.Contains(value, "\n\n\n") {
		value = strings.ReplaceAll(value, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(value)
}

func openTextDisplayAnswer(value string) string {
	content, err := sanitizeAnnotatedAnswer(value)
	if err != nil {
		return stdhtml.EscapeString(value)
	}
	return content.HTML
}

func openTextExportAnswer(value string) string {
	content, err := sanitizeAnnotatedAnswer(value)
	if err != nil {
		return normalizeAnswerText(value)
	}
	return content.ExportText
}
