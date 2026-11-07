package domain

import "testing"

func TestBatchPreservesCompleteAndConflictingSource(t *testing.T) {
	a := Asset{Payload: AssetPayload{Options: []string{"a", "b"}, Answer: 0, AnswerOrigin: "source", Explanation: "1.A"}}
	if KeepSourceAnswer(a, 0) {
		t.Fatal("answer list must allow filling explanation")
	}
	if !KeepSourceAnswer(a, 1) {
		t.Fatal("conflicting reliable answer must be preserved")
	}
	a.Payload.Explanation = "因为进程拥有独立的资源，所以选A。"
	if !KeepSourceAnswer(a, 0) {
		t.Fatal("complete source record must be preserved")
	}
	a.Payload.AnswerOrigin = "ai_pending"
	if KeepSourceAnswer(a, 0) {
		t.Fatal("unverified answer is not source evidence")
	}
}
func TestSubstantiveExplanation(t *testing.T) {
	for _, s := range []string{"", "1.A 2.B 3.C", "答案：A", "【解析】"} {
		if HasQuestionExplanation(s) {
			t.Fatalf("not an explanation: %q", s)
		}
	}
	for _, s := range []string{"解析：因为线程共享进程的地址空间，故开销较小。", "2+2=4"} {
		if !HasQuestionExplanation(s) {
			t.Fatalf("missing explanation: %q", s)
		}
	}
}
