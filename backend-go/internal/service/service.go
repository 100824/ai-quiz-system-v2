package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"ai-quiz-system-v2/backend-go/internal/models"
	"ai-quiz-system-v2/backend-go/internal/repository"
	"ai-quiz-system-v2/backend-go/internal/utils"
)

const part2UnderstandingQuestionText = "你对人工智能生成内容的理解程度是？"

// Service encapsulates business logic, delegating persistence to Repository.
type Service struct {
	repo *repository.Repository
}

// NewService creates a Service.
func NewService(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

// ---------------------------------------------------------------------------
// Statistics
// ---------------------------------------------------------------------------

// BuildStats aggregates completion and answer statistics for a course/class.
func (s *Service) BuildStats(courseID int, className string) (models.StatsPayload, error) {
	var stats models.StatsPayload
	var err error
	if stats.Part1, err = s.repo.GetCompletionStats(courseID, 1, className); err != nil {
		return stats, err
	}
	if stats.Part2, err = s.repo.GetCompletionStats(courseID, 2, className); err != nil {
		return stats, err
	}
	if stats.Part3, err = s.repo.GetCompletionStats(courseID, 3, className); err != nil {
		return stats, err
	}
	if stats.Part4, err = s.repo.GetCompletionStats(courseID, 4, className); err != nil {
		return stats, err
	}
	students, err := s.repo.GetAllStudentSurveys(courseID, className)
	if err != nil {
		return stats, err
	}
	stats.Students = students
	stats.Part1Stats.PredictionScoreDistribution = map[string]int{}
	stats.Part1Stats.LearningMethodsDistribution = map[string]int{}
	stats.Part2Stats.TotalCount = len(students)
	stats.Part2Stats.UnderstandingDistribution = map[string]int{}
	stats.Part3Stats.ScoreDistribution = map[string]int{"0": 0, "1": 0, "2": 0, "3": 0, "4": 0, "5": 0}

	part3Questions, err := s.repo.GetQuestionsByPart(courseID, 3)
	if err != nil {
		return stats, err
	}
	for i, q := range part3Questions {
		stats.Part3Stats.QuestionCorrectRate = append(stats.Part3Stats.QuestionCorrectRate, models.QuestionRate{
			QuestionID:   q.ID,
			QuestionText: q.QuestionText,
			SortOrder:    i + 1,
		})
	}

	for _, st := range students {
		if len(st.Part1) > 0 {
			var answers models.Part1Answers
			if err := json.Unmarshal(st.Part1, &answers); err == nil {
				stats.Part1Stats.PredictionScoreDistribution[strconv.Itoa(answers.PredictionScore)]++
				for _, method := range answers.LearningMethods {
					stats.Part1Stats.LearningMethodsDistribution[method]++
				}
				if strings.TrimSpace(answers.CustomMethod) != "" {
					stats.Part1Stats.LearningMethodsDistribution["自定义: "+answers.CustomMethod]++
				}
			}
		}
		if st.Part2 != nil && strings.TrimSpace(*st.Part2) != "" {
			stats.Part2Stats.FilledCount++
			for _, item := range GetPart2ResponseItems(*st.Part2) {
				if strings.TrimSpace(item.QuestionText) == part2UnderstandingQuestionText {
					label := FormatPart2ResponseLabel(item)
					if strings.TrimSpace(label) != "" {
						stats.Part2Stats.UnderstandingDistribution[label]++
					}
				}
			}
		}
		if st.Part3Score != nil {
			score := utils.Clamp(*st.Part3Score, 0, 5)
			stats.Part3Stats.ScoreDistribution[strconv.Itoa(score)]++
		}
		if len(st.Part3) > 0 {
			var answers map[string]string
			if err := json.Unmarshal(st.Part3, &answers); err == nil {
				for i := range stats.Part3Stats.QuestionCorrectRate {
					key := fmt.Sprintf("q%d", i+1)
					answer, ok := answers[key]
					if !ok {
						continue
					}
					stats.Part3Stats.QuestionCorrectRate[i].TotalCount++
					if i < len(part3Questions) && part3Questions[i].CorrectAnswer != nil && answer == *part3Questions[i].CorrectAnswer {
						stats.Part3Stats.QuestionCorrectRate[i].CorrectCount++
					}
				}
			}
		}
	}
	for i := range stats.Part3Stats.QuestionCorrectRate {
		q := &stats.Part3Stats.QuestionCorrectRate[i]
		if q.TotalCount > 0 {
			q.CorrectRate = int(float64(q.CorrectCount) / float64(q.TotalCount) * 100)
		}
	}
	return stats, nil
}

// ---------------------------------------------------------------------------
// Export
// ---------------------------------------------------------------------------

// BuildExportRow converts a student survey into a flat string slice for Excel.
func BuildExportRow(s models.StudentSurvey) []string {
	status := "未开始"
	if len(s.Part1) > 0 {
		status = "完成到第一部分"
	}
	if s.Part2 != nil && strings.TrimSpace(*s.Part2) != "" {
		status = "完成到第二部分"
	}
	if len(s.Part3) > 0 {
		status = "完成到第三部分"
	}
	if len(s.Part4) > 0 {
		status = "已完成全部"
	}

	predictionScore := "未提交"
	learningMethods := "未提交"
	customLearningMethod := "无"
	part2Understanding := "未提交"
	part2OpenAnswer := "未提交"
	if len(s.Part1) > 0 {
		var answers models.Part1Answers
		if err := json.Unmarshal(s.Part1, &answers); err == nil {
			predictionScore = fmt.Sprintf("%d分", answers.PredictionScore)
			if len(answers.LearningMethods) > 0 {
				learningMethods = strings.Join(answers.LearningMethods, "、")
			} else {
				learningMethods = "无"
			}
			if strings.TrimSpace(answers.CustomMethod) != "" {
				customLearningMethod = answers.CustomMethod
			}
		}
	}
	if s.Part2 != nil && strings.TrimSpace(*s.Part2) != "" {
		items := GetPart2ResponseItems(*s.Part2)
		if len(items) == 1 {
			part2OpenAnswer = FormatPart2ResponseLabel(items[0])
		} else {
			for _, item := range items {
				if strings.TrimSpace(item.QuestionText) == part2UnderstandingQuestionText {
					part2Understanding = utils.ValueOr(FormatPart2ResponseLabel(item), "未提交")
					continue
				}
				if item.QuestionType == "text" || part2OpenAnswer == "未提交" {
					part2OpenAnswer = utils.ValueOr(FormatPart2ResponseLabel(item), "未提交")
				}
			}
		}
	}

	part3Answers := map[string]string{}
	if len(s.Part3) > 0 {
		_ = json.Unmarshal(s.Part3, &part3Answers)
	}
	part4 := map[string]interface{}{}
	if len(s.Part4) > 0 {
		_ = json.Unmarshal(s.Part4, &part4)
	}
	actualScore := "未提交"
	predictedScore := "未提交"
	part4Q2Answer := "未提交"
	part4Q2Custom := "无"
	part4Q3Answer := "未提交"
	part4Q3Custom := "无"
	if scoreCompare, ok := part4["scoreCompare"].(map[string]interface{}); ok {
		if v, ok := scoreCompare["actual"]; ok {
			actualScore = fmt.Sprint(v)
		}
		if v, ok := scoreCompare["predicted"]; ok {
			predictedScore = fmt.Sprint(v)
		}
	}
	if q2, ok := part4["q2"].(map[string]interface{}); ok {
		if answers, ok := q2["answers"].([]interface{}); ok {
			part4Q2Answer = utils.JoinInterfaces(answers)
		}
		if v, ok := q2["custom"].(string); ok && strings.TrimSpace(v) != "" {
			part4Q2Custom = v
		}
	}
	if q3, ok := part4["q3"].(map[string]interface{}); ok {
		if answers, ok := q3["answers"].([]interface{}); ok {
			part4Q3Answer = utils.JoinInterfaces(answers)
		}
		if v, ok := q3["custom"].(string); ok && strings.TrimSpace(v) != "" {
			part4Q3Custom = v
		}
	}

	scoreText := "未完成"
	if s.Part3Score != nil {
		scoreText = fmt.Sprintf("%d/5", *s.Part3Score)
	}

	lastSubmitted := s.UpdatedAt
	return []string{
		utils.TruncateCell(s.ClassName),
		utils.TruncateCell(s.StudentName),
		utils.TruncateCell(status),
		utils.TruncateCell(predictionScore),
		utils.TruncateCell(learningMethods),
		utils.TruncateCell(customLearningMethod),
		utils.TruncateCell(part2Understanding),
		utils.TruncateCell(part2OpenAnswer),
		utils.TruncateCell(utils.ValueOr(part3Answers["q1"], "未提交")),
		utils.TruncateCell(utils.ValueOr(part3Answers["q2"], "未提交")),
		utils.TruncateCell(utils.ValueOr(part3Answers["q3"], "未提交")),
		utils.TruncateCell(utils.ValueOr(part3Answers["q4"], "未提交")),
		utils.TruncateCell(utils.ValueOr(part3Answers["q5"], "未提交")),
		utils.TruncateCell(scoreText),
		utils.TruncateCell(actualScore),
		utils.TruncateCell(predictedScore),
		utils.TruncateCell(part4Q2Answer),
		utils.TruncateCell(part4Q2Custom),
		utils.TruncateCell(part4Q3Answer),
		utils.TruncateCell(part4Q3Custom),
		utils.TruncateCell(lastSubmitted),
	}
}

// ---------------------------------------------------------------------------
// Part 2 answer builders
// ---------------------------------------------------------------------------

// BuildPart2StoredAnswers assembles the final stored JSON from legacy or new request shapes.
func BuildPart2StoredAnswers(questions []models.Question, legacyAnswer string, legacyAnswers []string, responses []models.Part2ResponseInput) (stored string, display string, explanation string, err error) {
	if len(questions) == 1 && len(responses) == 0 {
		single := questions[0]
		stored, display, err = BuildPart2StoredAnswer(single, strings.TrimSpace(legacyAnswer), legacyAnswers)
		if err != nil {
			return "", "", "", err
		}
		if single.Explanation != nil {
			explanation = *single.Explanation
		}
		return stored, display, explanation, nil
	}

	responseMap := make(map[int]models.Part2ResponseInput, len(responses))
	for _, item := range responses {
		responseMap[item.QuestionID] = item
	}

	payload := models.Part2AnswerPayload{
		Version:   2,
		Responses: make([]models.Part2ResponseItem, 0, len(questions)),
	}
	displayParts := make([]string, 0, len(questions))
	explanations := make([]string, 0, len(questions))

	for index, q := range questions {
		input := responseMap[q.ID]
		if index == 0 && input.Answer == "" && len(input.Answers) == 0 && strings.TrimSpace(legacyAnswer) != "" {
			input.Answer = legacyAnswer
			input.Answers = legacyAnswers
		}
		storedSingle, displaySingle, singleErr := BuildPart2StoredAnswer(q, strings.TrimSpace(input.Answer), input.Answers)
		if singleErr != nil {
			return "", "", "", singleErr
		}
		item := ParsePart2Payload(storedSingle)
		item.QuestionID = q.ID
		item.QuestionText = q.QuestionText
		payload.Responses = append(payload.Responses, item)
		displayParts = append(displayParts, displaySingle)
		if q.Explanation != nil && strings.TrimSpace(*q.Explanation) != "" {
			explanations = append(explanations, *q.Explanation)
		}
	}

	raw, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return "", "", "", marshalErr
	}
	return string(raw), strings.Join(displayParts, " | "), strings.Join(explanations, "\n\n"), nil
}

// BuildPart2StoredAnswer validates and formats a single part-2 answer.
func BuildPart2StoredAnswer(q models.Question, answer string, answers []string) (stored string, display string, err error) {
	switch q.QuestionType {
	case "text":
		if strings.TrimSpace(answer) == "" {
			return "", "", errors.New("请填写你的答案")
		}
		annotationCount := utils.CountPart2Annotations(answer)
		if q.AnnotationEnabled && annotationCount == 0 {
			return "", "", errors.New("请至少对一处内容进行颜色标注后再提交")
		}
		plainText := utils.StripHTMLTags(answer)
		payload := models.Part2AnswerPayload{
			QuestionType:    q.QuestionType,
			Value:           answer,
			Label:           plainText,
			AnnotationCount: annotationCount,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", "", err
		}
		return string(raw), plainText, nil
	case "multi":
		cleaned := make([]string, 0, len(answers))
		for _, item := range answers {
			item = strings.TrimSpace(item)
			if item != "" {
				cleaned = append(cleaned, item)
			}
		}
		if len(cleaned) == 0 {
			return "", "", errors.New("请至少选择一个答案")
		}
		payload := models.Part2AnswerPayload{
			QuestionType: q.QuestionType,
			Values:       cleaned,
			Labels:       cleaned,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", "", err
		}
		return string(raw), strings.Join(cleaned, "、"), nil
	default:
		if strings.TrimSpace(answer) == "" {
			return "", "", errors.New("请选择一个答案")
		}
		payload := models.Part2AnswerPayload{
			QuestionType: q.QuestionType,
			Value:        answer,
			Label:        answer,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", "", err
		}
		return string(raw), answer, nil
	}
}

// ParsePart2Payload parses a single stored answer string into a response item.
func ParsePart2Payload(raw string) models.Part2ResponseItem {
	var payload models.Part2AnswerPayload
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		if len(payload.Responses) > 0 {
			return payload.Responses[0]
		}
		return models.Part2ResponseItem{
			QuestionType:    payload.QuestionType,
			Value:           payload.Value,
			Values:          payload.Values,
			Label:           payload.Label,
			Labels:          payload.Labels,
			AnnotationCount: payload.AnnotationCount,
		}
	}
	return models.Part2ResponseItem{
		QuestionType: "text",
		Value:        raw,
		Label:        raw,
	}
}

// GetPart2ResponseItems extracts all response items from a stored answer string.
func GetPart2ResponseItems(raw string) []models.Part2ResponseItem {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var payload models.Part2AnswerPayload
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		if len(payload.Responses) > 0 {
			return payload.Responses
		}
		return []models.Part2ResponseItem{{
			QuestionType:    payload.QuestionType,
			Value:           payload.Value,
			Values:          payload.Values,
			Label:           payload.Label,
			Labels:          payload.Labels,
			AnnotationCount: payload.AnnotationCount,
		}}
	}
	return []models.Part2ResponseItem{{
		QuestionType: "text",
		Value:        raw,
		Label:        raw,
	}}
}

// FormatPart2Answer produces a human-readable display string from stored part-2 JSON.
func FormatPart2Answer(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	items := GetPart2ResponseItems(raw)
	if len(items) == 0 {
		return raw
	}
	if len(items) == 1 {
		return FormatPart2ResponseLabel(items[0])
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		label := FormatPart2ResponseLabel(item)
		if strings.TrimSpace(item.QuestionText) != "" {
			parts = append(parts, item.QuestionText+": "+label)
		} else {
			parts = append(parts, label)
		}
	}
	return strings.Join(parts, "；")
}

// FormatPart2ResponseLabel extracts the best display label from a response item.
func FormatPart2ResponseLabel(item models.Part2ResponseItem) string {
	if len(item.Labels) > 0 {
		return strings.Join(item.Labels, "、")
	}
	if strings.TrimSpace(item.Label) != "" {
		return item.Label
	}
	if len(item.Values) > 0 {
		return strings.Join(item.Values, "、")
	}
	if strings.TrimSpace(item.Value) != "" {
		return item.Value
	}
	return ""
}
