package service

import "testing"

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
