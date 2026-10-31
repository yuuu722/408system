package service

import "github.com/local/exam408-assistant/backend/internal/domain"

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
