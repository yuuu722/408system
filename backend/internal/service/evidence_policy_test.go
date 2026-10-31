package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEvidenceRetainsExplanationButNeverUnresolvedOptions(t *testing.T) {
	q := "为什么需要虚拟内存？"
	require.Empty(t, answerEvidence(q, "虚拟内存概念\n页面置换算法\n内存分配与回收"))
	source := "虚拟内存管理中哪项正确？\nA. 每个用户占用M/N内存\nB. 全部装入\n【解析】虚拟内存利用程序的局部性，允许仅将当前需要的部分装入内存。\n(2) 下一题\nA. 虚拟内存不存在"
	got := answerEvidence(q, source)
	require.Contains(t, got, "局部性")
	require.NotContains(t, got, "M/N")
	require.NotContains(t, got, "下一题")
	require.NotEmpty(t, answerEvidence(q, "虚拟内存通过按需调入，使程序无需全部装入物理内存即可运行。"))
	require.Empty(t, answerEvidence("请计算该选择题的答案", "虚拟内存题目\nA. 一次性 B. 多次性"))
}
