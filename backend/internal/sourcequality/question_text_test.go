package sourcequality

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestQuestionCorruptionDoesNotTreatProtocolsAndCodeAsNoise(t *testing.T) {
	for _, s := range []string{"死锁避免不会给 TRERREAEED RED，而死锁检测会", "FTP ARS BS iE Wr et 控制连接建立端口为21", "线程的属性�无法辨认"} {
		require.True(t, CorruptQuestionText(s), s)
	}
	for _, s := range []string{"TCP使用ACK、SYN和FIN等标志进行连接管理。", "CPU通过MMU和TLB完成地址转换。", "for (int i=0; i<n; ++i) 可用于遍历数组。", "使用Compare And Swap实现原子操作。", "Dijkstra算法适用于非负权图。"} {
		require.False(t, CorruptQuestionText(s), s)
	}
}
