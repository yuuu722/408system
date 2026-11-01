// Package sourcequality separates searchable textbook text from layout noise.
// It never guesses missing OCR characters or changes mathematical operators.
package sourcequality

import (
	"regexp"
	"strings"
	"unicode"
)

var spaces = regexp.MustCompile(`[\t\p{Zs}]+`)
var hanGap = regexp.MustCompile(`([\p{Han}]) +([\p{Han}])`)
var pageNumber = regexp.MustCompile(`^[·•\-— ]*(?:第\s*\d+\s*页|\d+\s*页|[·•]\s*\d+)[·•\-— ]*$`)
var garbage = regexp.MustCompile(`[�\x00]`)
var englishName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 /+._-]*$`)
var knownLatin = map[string]bool{}

func init() {
	knownLatin["ieee 754"] = true
	for _, v := range strings.Fields("CPU GPU ALU MMU TLB RAM ROM SRAM DRAM SDRAM DDR SRAM EPROM EEPROM Flash Cache SSD HDD RAID DMA I/O IO PCI PCIe USB BIOS UEFI SIMD MIMD SISD MISD RISC CISC TCP IP UDP HTTP HTTPS FTP SMTP DNS DHCP ARP ICMP NAT CIDR VLAN LAN WAN WLAN MAC CSMA/CD CSMA/CA OSI RIP OSPF BGP PPP Ethernet IPv4 IPv6 TCP/IP PCB PID IPC API FIFO LRU LFU OPT NRU CLOCK FCFS SJF SRTF RR HRRN MLFQ SCAN CSCAN C-SCAN LOOK C-LOOK SSTF FCB FAT NTFS EXT AVL BST B树 B+树 BFS DFS Dijkstra Floyd Prim Kruskal KMP AOE AOV ASCII Unicode UTF-8 IEEE754 IEEE IEEE 754 FFT ACK SYN FIN RTT MSS MTU TTL SSL TLS RPC NFS SQL VM VFS UNIX Linux Windows inode Semaphore Monitor Peterson Banker") {
		knownLatin[strings.ToLower(v)] = true
	}
}
func Advertising(s string) bool {
	s = strings.ToLower(strings.Join(strings.Fields(s), ""))
	for _, v := range []string{"淘宝", "taobao", "公众号", "官方店", "官方唯一", "唯一官方", "购买纸质", "买纸质", "扫码关注", "扫码购买", "加微信", "加qq群", "交流群", "资料领取", "领取资料", "微信号", "版权声明", "禁止转载", "网盘链接", "提取码"} {
		if strings.Contains(s, v) {
			return true
		}
	}
	return false
}

// Normalize preserves line structure, Latin word boundaries and formula spacing.
func Normalize(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	s = strings.NewReplacer("&#x20;", " ", "&#32;", " ", "&nbsp;", " ", "\u00ad", "", "\u200b", "", "\ufeff", "").Replace(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		l = strings.TrimSpace(spaces.ReplaceAllString(l, " "))
		for {
			v := hanGap.ReplaceAllString(l, "$1$2")
			if v == l {
				break
			}
			l = v
		}
		r := []rune(l)
		if len(r) > 1 && r[len(r)-1] == '\\' && r[len(r)-2] != '\\' && (unicode.Is(unicode.Han, r[len(r)-2]) || strings.ContainsRune("，。；：", r[len(r)-2])) {
			l = string(r[:len(r)-1])
		}
		lines[i] = l
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// Clean drops only recognizable boilerplate lines. Equations/tables stay intact.
func Clean(s string) string {
	lines := strings.Split(Normalize(s), "\n")
	out := []string{}
	blank := false
	for _, l := range lines {
		if (Advertising(l) && len([]rune(l)) <= 200) || pageNumber.MatchString(l) || strings.HasPrefix(strings.ToLower(l), "https://") || strings.HasPrefix(strings.ToLower(l), "http://") {
			continue
		}
		if garbage.MatchString(l) && strings.Count(l, "�")*5 > len([]rune(l)) {
			continue
		}
		if l == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// EntityReason rejects advertising, sentence fragments and unexplained OCR codes.
// Unknown labels are excluded from the active graph, not erased from source files.
func EntityReason(name string) string {
	n := strings.TrimSpace(name)
	r := []rune(n)
	if Advertising(n) {
		return "advertising"
	}
	if len(r) < 2 || len(r) > 24 || garbage.MatchString(n) || strings.ContainsAny(n, "\n\r。！？?！：:=<>\\|{}") {
		return "invalid_label"
	}
	for _, v := range []string{"的特点是", "可分为", "主要有", "是一个", "此题", "本题", "答案", "解析", "第一趟", "第一个", "如下", "一般有", "主要由", "要注意", "计算结果", "填空", "通话质量", "文件A ", "第几", "考虑以下", "由图可知"} {
		if strings.Contains(n, v) {
			return "sentence_or_question"
		}
	}
	if strings.HasSuffix(n, "是") || strings.HasSuffix(n, "为") {
		return "sentence_fragment"
	}
	if englishName.MatchString(n) && !knownLatin[strings.ToLower(n)] {
		return "unverified_code"
	}
	han := 0
	for _, c := range r {
		if unicode.Is(unicode.Han, c) {
			han++
		}
	}
	if han == 0 && !englishName.MatchString(n) {
		return "invalid_label"
	}
	return ""
}

// A colon alone does not establish a definition (shop titles and question
// headings also use it). Require a readable explanatory predicate as well.
func Definition(name, description string) bool {
	if EntityReason(name) != "" || Advertising(description) || strings.ContainsRune(description, '�') {
		return false
	}
	for _, v := range []string{"是", "指", "使用", "记录", "称为", "通过", "实现", "采用", "负责", "表示", "用于", "保存", "能够", "允许", "分配", "组成", "包含", "存储"} {
		if strings.Contains(description, v) {
			return true
		}
	}
	return false
}
