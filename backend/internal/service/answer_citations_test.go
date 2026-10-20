package service

import "testing"

func TestAnswerCitationIntegrity(t *testing.T) {
	for _, tc := range []struct {
		answer string
		count  int
		valid  bool
	}{
		{"页表保存映射关系。[1]", 1, true},
		{"缺少引用的答案", 1, false},
		{"伪造来源[2]", 1, false},
		{"有效引用[1]和无效引用[99]", 2, false},
		{"零编号[0]", 2, false},
	} {
		if got := validAnswerCitations(tc.answer, tc.count); got != tc.valid {
			t.Errorf("%q: got %v", tc.answer, got)
		}
	}
}
