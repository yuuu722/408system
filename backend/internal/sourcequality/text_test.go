package sourcequality

import (
	"strings"
	"testing"
)

func TestTextbookCleanupPreservesFormulaAndContent(t *testing.T) {
	input := "\ufeff虚 拟 内 存\u00a0是按需装入。\r\nSeas 唯一官方淘宝店:\nhttps://jc408.taobao.com\n第 225 页\n地址 = x + y\n1/2\n12\nTCP window\n&#x20;内 存保护\n"
	got := Clean(input)
	for _, bad := range []string{"淘宝", "http", "第 225", "&#x20;"} {
		if strings.Contains(got, bad) {
			t.Fatal(got)
		}
	}
	for _, keep := range []string{"虚拟内存是按需装入。", "地址 = x + y", "1/2", "12", "TCP window", "内存保护"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("lost %s: %s", keep, got)
		}
	}
	if Clean(got) != got {
		t.Fatal("not idempotent")
	}
}
func TestReportedGraphPollutionRejected(t *testing.T) {
	for _, v := range []string{"Seas 唯一官方淘宝店", "买纸质版认准竟成唯一官方淘宝店", "第一趟排序结果", "计算机一般有两种存储器", "e JP 电话的通话质量主要由两个因素决定", "文件A AFAR", "此题要注意 3 个细节", "WES"} {
		if EntityReason(v) == "" {
			t.Errorf("accepted %q", v)
		}
	}
	for _, v := range []string{"虚拟内存", "页表", "Cache", "TCP/IP", "LRU", "CSMA/CD", "IEEE 754", "请求分页存储管理"} {
		if EntityReason(v) != "" {
			t.Errorf("rejected %q", v)
		}
	}
}

func TestSentenceLabelsAndAmbiguousFormula(t *testing.T) {
	for _, label := range []string{"一个结点包括三部分内容", "服务器之间要建立两个并行的TCP连接", "我们可以把二维数组看成这样"} {
		if EntityReason(label) == "" {
			t.Errorf("accepted sentence %s", label)
		}
	}
	for _, label := range []string{"线性表", "TCP滑动窗口", "存储系统"} {
		if EntityReason(label) != "" {
			t.Errorf("rejected term %s", label)
		}
	}
	if !SuspectFormula("到序号N- !为止") || SuspectFormula("序号N-1") {
		t.Fatal("formula ambiguity guard")
	}
}
