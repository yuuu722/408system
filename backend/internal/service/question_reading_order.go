package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"math"
	"regexp"
	"sort"
	"strings"
)

var isolatedQuestionNumber = regexp.MustCompile(`^[（(]?\s*[0-9０-９]{1,3}\s*[.．、)）]\s*$`)

// Rejoin a detached question-number gutter only when saved geometry proves
// which body line shares each number's baseline. No sequential-number guessing
// is permitted when boxes are missing, overlapping, or in multiple columns.
func questionReadingPages(pages []TextPage) []TextPage {
	out := append([]TextPage(nil), pages...)
	for i, p := range out {
		if repaired, ok := repairQuestionReadingPage(p); ok {
			out[i] = repaired
		}
	}
	return out
}
func repairQuestionReadingPage(p TextPage) (TextPage, bool) {
	validBox := func(b domain.DocumentBlock) bool {
		return len(b.Box) == 4 && b.Box[0] >= 0 && b.Box[1] >= 0 && b.Box[2] <= 1 && b.Box[3] <= 1 && b.Box[2] > b.Box[0] && b.Box[3] > b.Box[1]
	}
	anchors := []int{}
	column := -1.0
	for i, b := range p.Blocks {
		if !isolatedQuestionNumber.MatchString(questionLine(b.Text)) {
			continue
		}
		if !validBox(b) {
			return p, false
		}
		if column < 0 {
			column = b.Box[0]
		}
		if math.Abs(b.Box[0]-column) > .035 || b.Box[0] > .2 {
			return p, false
		}
		anchors = append(anchors, i)
	}
	if len(anchors) < 2 {
		return p, false
	}
	// Require a unique nearest same-baseline body for at least two anchors.
	used := map[int]bool{}
	for _, index := range anchors {
		a := p.Blocks[index]
		best := -1
		distance := math.Inf(1)
		ambiguous := false
		for j, b := range p.Blocks {
			if j == index || !validBox(b) || b.Image != "" || strings.TrimSpace(b.Text) == "" || isolatedQuestionNumber.MatchString(questionLine(b.Text)) {
				continue
			}
			if b.Box[0] < a.Box[2]-.002 || b.Box[0]-a.Box[2] > .14 {
				continue
			}
			overlap := math.Min(a.Box[3], b.Box[3]) - math.Max(a.Box[1], b.Box[1])
			if overlap < .6*math.Min(a.Box[3]-a.Box[1], b.Box[3]-b.Box[1]) {
				continue
			}
			d := b.Box[0] - a.Box[2]
			if d < distance-.001 {
				distance = d
				best = j
				ambiguous = false
			} else if math.Abs(d-distance) < .001 {
				ambiguous = true
			}
		}
		if best < 0 || ambiguous || used[best] {
			return p, false
		}
		used[best] = true
	}
	blocks := []domain.DocumentBlock{}
	for _, b := range p.Blocks {
		if b.Kind == "paragraph_break" {
			continue
		}
		if b.Text == "" && b.Image == "" {
			continue
		}
		if !validBox(b) {
			return p, false
		}
		blocks = append(blocks, b)
	}
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].Box[1] != blocks[j].Box[1] {
			return blocks[i].Box[1] < blocks[j].Box[1]
		}
		return blocks[i].Box[0] < blocks[j].Box[0]
	})
	rows := [][]domain.DocumentBlock{}
	for _, b := range blocks {
		if len(rows) == 0 {
			rows = append(rows, []domain.DocumentBlock{b})
			continue
		}
		row := rows[len(rows)-1]
		a := row[0]
		overlap := math.Min(a.Box[3], b.Box[3]) - math.Max(a.Box[1], b.Box[1])
		if b.Image == "" && a.Image == "" && overlap >= .65*math.Min(a.Box[3]-a.Box[1], b.Box[3]-b.Box[1]) {
			rows[len(rows)-1] = append(row, b)
		} else {
			rows = append(rows, []domain.DocumentBlock{b})
		}
	}
	result := []domain.DocumentBlock{}
	for _, row := range rows {
		sort.SliceStable(row, func(i, j int) bool { return row[i].Box[0] < row[j].Box[0] })
		a := row[0]
		a.Box = append([]float64(nil), a.Box...)
		for _, b := range row[1:] {
			if b.Box[0] < a.Box[2]-.002 {
				return p, false
			}
			a.Text += " " + b.Text
			a.ID += "+" + b.ID
			a.Box[1] = math.Min(a.Box[1], b.Box[1])
			a.Box[2] = b.Box[2]
			a.Box[3] = math.Max(a.Box[3], b.Box[3])
			if b.Confidence != nil && (a.Confidence == nil || *b.Confidence < *a.Confidence) {
				c := *b.Confidence
				a.Confidence = &c
			}
		}
		result = append(result, a)
	}
	p.Blocks = result
	CanonicalPage(&p)
	return p, true
}

func QuestionReadingOrderStats(pages []TextPage) map[string]int {
	stats := map[string]int{"PagesWithDetachedNumberLines": 0, "CoordinateRepairablePages": 0, "MissingCoordinatePages": 0, "AmbiguousCoordinatePages": 0}
	for _, p := range pages {
		numbers := 0
		for _, line := range strings.Split(p.Text, "\n") {
			if isolatedQuestionNumber.MatchString(questionLine(line)) {
				numbers++
			}
		}
		if numbers < 2 {
			continue
		}
		stats["PagesWithDetachedNumberLines"]++
		if _, ok := repairQuestionReadingPage(p); ok {
			stats["CoordinateRepairablePages"]++
			continue
		}
		boxes := 0
		for _, b := range p.Blocks {
			if len(b.Box) == 4 {
				boxes++
			}
		}
		if boxes == 0 {
			stats["MissingCoordinatePages"]++
		} else {
			stats["AmbiguousCoordinatePages"]++
		}
	}
	return stats
}
