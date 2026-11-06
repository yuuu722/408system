package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
)

// Public textbooks precede personal notes; unknown synthetic stores retain compatibility.
func sourcePriority(c domain.Chunk) int {
	kind, _ := c.Metadata["resource_type"].(string)
	public, _ := c.Metadata["is_public"].(bool)
	rank := 4
	switch kind {
	case "textbook":
		rank = 0
	case "courseware":
		rank = 2
	case "solution":
		rank = 3
	case "notes":
		rank = 4
	case "past_exam", "exercise", "mock_exam":
		rank = 6
	}
	if !public {
		rank += 1
	}
	return rank
}

func questionFocusScore(question string, c domain.Chunk) float64 {
	score := c.Score
	if spec := comparisonTargets(question); spec != nil && mentions(c.Content, spec.Left) && mentions(c.Content, spec.Right) {
		score += 1.2
	}
	if protocolWhy(question) && protocolPurpose(c.Content) {
		score += 1.2
	}
	if virtualMemoryWhy(question) && virtualMemoryMotivation(c.Content) {
		score += 1.2
	}
	if tcpReliabilityQuestion(question) && tcpMechanismCoverage(c.Content) {
		score += 1
	}
	if virtualMemoryWhy(question) {
		for _, term := range []string{"不足", "无法运行", "不能全部", "局部性", "暂时不用", "驻留性", "并发度"} {
			if strings.Contains(c.Content, term) {
				score += .15
			}
		}
	}
	return score
}
