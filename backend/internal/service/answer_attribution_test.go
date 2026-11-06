package service

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"testing"
)

func TestUnsafeAttribution(t *testing.T) {
	for _, text := range []string{"程序在运行时具有一次性及驻留性特征。", "若确认号为Y，则到序号N- !的数据已收到。"} {
		if !unsafeAttribution(text) {
			t.Errorf("accepted %s", text)
		}
	}
	for _, text := range []string{"传统存储管理方式具有一次性和驻留性。", "程序运行具有局部性。", "确认号为Y，表示序号Y-1及以前的数据已收到。"} {
		if unsafeAttribution(text) {
			t.Errorf("rejected %s", text)
		}
	}
}

func TestVirtualMemoryPropertiesCannotBeReversed(t *testing.T) {
	for _, text := range []string{"虚拟存储器的多次性特征使得作业在运行前必须一次性地全部装入内存后方能开始运行。", "虚拟存储器的驻留性特征允许将暂时不用的程序调至磁盘。", "虚拟内存具有一次性。"} {
		if !unsafeAttribution(text) {
			t.Errorf("accepted reversal %s", text)
		}
	}
	for _, text := range []string{"虚拟存储器的多次性意味着作业不需要一次性全部装入内存。", "虚拟内存打破传统存储管理的一次性和驻留性。", "虚拟内存不具有驻留性。"} {
		if unsafeAttribution(text) {
			t.Errorf("rejected supported distinction %s", text)
		}
	}
}

func TestKnownDefinitionReversalRejectedEvenIfReviewerWouldApprove(t *testing.T) {
	source := "虚拟内存允许作业分多次调入内存，不必一次性全部装入。"
	raw := `{"claims":[{"text":"虚拟存储器的多次性要求作业必须一次性全部装入内存。","kind":"explanation","evidence":[{"source":1,"quote":"虚拟内存允许作业分多次调入内存，不必一次性全部装入。"}]}]}`
	rag := RAGService{LLM: comparisonReviewModel{raw: `{"supported":true}`}}
	_, _, ok, why := rag.composedAnswerDetail(context.Background(), raw, "为什么需要虚拟内存？", []string{source}, []domain.Citation{{DocumentID: "d"}}, nil)
	if ok || why != "virtual_memory_definition" {
		t.Fatalf("accepted reversal or misclassified: %v %s", ok, why)
	}
}
