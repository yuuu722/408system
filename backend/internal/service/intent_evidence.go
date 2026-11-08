package service

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
	"time"
)

func virtualMemoryWhy(q string) bool {
	return whyQuestion(q) && (strings.Contains(q, "虚拟内存") || strings.Contains(q, "虚拟存储"))
}
func textbookIntentTerms(q string) ([]string, []string) {
	if spec := comparisonTargets(q); spec != nil {
		return []string{spec.Left, spec.Right}, []string{"比较", "区别", "调度", "资源", "地址空间", "开销", "共享", "通信", "基本单位"}
	}
	if virtualMemoryWhy(q) {
		return []string{"虚拟内存", "虚拟存储"}, []string{"一次性", "驻留性", "全部装入", "无法运行", "内存不足", "局部性", "暂时不用", "浪费", "并发度"}
	}
	if tcpReliabilityQuestion(q) {
		return []string{"tcp"}, []string{"序号", "确认", "超时重传", "累计确认", "重传", "校验和"}
	}
	if protocolWhy(q) {
		needs := []string{"用于", "用来", "目的", "功能", "提供", "支持", "解决", "要求", "建立", "配置", "协商"}
		if sharesProtocol([]string{"PPP"}, q) {
			needs = append(needs, "点到点", "点对点", "LCP", "NCP")
		}
		return protocolFocus(q), needs
	}
	return nil, nil
}
func (s RAGService) intentEvidence(ctx context.Context, uid string, kids []string, q string) []domain.Chunk {
	store, ok := s.Store.(interface {
		SearchTextbookEvidence(context.Context, string, string, []string, []string, int) ([]domain.Chunk, error)
	})
	if !ok {
		return nil
	}
	topics, needs := textbookIntentTerms(q)
	if len(topics) == 0 {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out := []domain.Chunk{}
	for _, kid := range kids {
		chunks, e := store.SearchTextbookEvidence(bounded, uid, kid, topics, needs, 12)
		if e != nil {
			s.warn("intent textbook retrieval unavailable", "error", e)
			continue
		}
		out = append(out, chunks...)
	}
	return out
}
func virtualMemoryMotivation(text string) bool {
	for _, term := range []string{"驻留性", "作业很大", "作业过大", "超过物理内存", "超过了物理内存", "超过内存实际容量", "内存资源浪费", "内存空间浪费", "并发度下降", "并发度的下降"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	if strings.Contains(text, "一次性") && !strings.Contains(text, "不必一次性") && !strings.Contains(text, "无需一次性") {
		return true
	}
	if strings.Contains(text, "全部装入") && (strings.Contains(text, "必须") || strings.Contains(text, "要求")) {
		return true
	}
	if strings.Contains(text, "暂时不用") && (strings.Contains(text, "占据") || strings.Contains(text, "占用") || strings.Contains(text, "浪费")) {
		return true
	}

	return false
}

func generationFailureLabel(code string) string {
	switch code {
	case "virtual_memory_term", "virtual_memory_overclaim", "virtual_memory_coverage":
		return "虚拟内存讲解存在术语错误、绝对化结论或遗漏本轮证据中的必要方面"
	case "virtual_memory_definition":
		return "多次性、一次性或驻留性的定义发生反转"
	case "subject_mismatch":
		return "概念特征被错误地归给了其他对象"
	case "suspect_formula":
		return "回答存在疑似错误的数字或公式符号"
	case "ambiguous_comparison":
		return "比较对象不明确，需先补充双方名称"
	case "invalid_comparison_structure", "incomplete_comparison":
		return "比较结果未分别说明双方的具体差异"
	case "comparison_side_unsupported":
		return "比较一方的结论缺少该方的原文依据"
	case "repair_request_failed":
		return "模型曾返回文本，但整理重试请求失败"
	case "no_complete_facts":
		return "相关片段缺少可引用的完整讲解句，未发起模型整理"
	case "model_abstained":
		return "模型返回了空整理结果，认为当前事实不足以回答"
	case "excess_claims":
		return "模型返回的段落数量超出要求"
	case "no_usable_evidence":
		return "没有取得可用的讲解依据"
	case "provider_error":
		return "模型请求没有返回可用文本"
	case "invalid_json", "invalid_claim_schema", "missing_fact_ids", "empty_or_excess_claims":
		return "模型返回内容的格式或事实编号不符合要求"
	case "unknown_fact_id", "unknown_source":
		return "模型引用了本轮不存在的事实编号"
	case "incomplete_claim":
		return "模型返回的正文不完整或超出长度要求"
	case "quote_not_in_source":
		return "引用原句未能与来源匹配"
	case "missing_premise":
		return "整理结果缺少必要前提"
	case "missing_protocol_purpose":
		return "整理结果没有解释协议用途，只提供了格式、字段或长度等信息"
	case "insufficient_coverage":
		return "整理结果遗漏了问题要求的关键方面"
	case "support_review_unavailable":
		return "模型已完成整理，但依据核验请求失败"
	case "invalid_review_format":
		return "模型已完成整理，但核验结果格式无效"
	case "unsupported_claim":
		return "模型已完成整理，但核验认为正文存在原文未支持的结论"
	case "unsupported_or_invalid_review":
		return "依据核验未通过，或核验输出格式无效"
	default:
		return "未生成通过核验的整理结果"
	}
}

func repairAnswerGuidance(code, q string) string {
	switch code {
	case "virtual_memory_term", "virtual_memory_overclaim", "virtual_memory_coverage":
		return "使用本轮事实中的准确术语：传统存储管理的一次性不能写成单一性。不要声称完全避免内存浪费。按原因到机制解释：本轮事实若包含容量限制、一次性驻留性、局部性、按需调入，应分别说明其联系；未提供的方面不编造。引用实际支持这些解释的事实编号。"
	case "virtual_memory_definition":
		return "核对本轮事实的肯定否定和特征名称，不得把多次性解释成必须一次性全部装入，不得把驻留性与对换性混同。只改写原文支持的内容。"
	case "subject_mismatch":
		return "恢复事实中的原始主语和属性归属，不把存储管理方式的性质写成程序的性质。"
	case "suspect_formula":
		return "删除本轮事实无法核实的乱码数字或公式，不要猜改符号或变量。"
	case "invalid_comparison_structure", "incomplete_comparison", "comparison_side_unsupported":
		return "严格返回comparisons数组；每项给出dimension、left、right、left_facts、right_facts，分别说明双方具体情况并引用支持各自结论的事实。没有双方证据的维度不写。"
	case "insufficient_coverage", "missing_protocol_purpose":
		if protocolWhy(q) {
			return "解释所问协议的用途、功能或解决的问题，只用本轮事实。帧字段、MTU数值不是必须重复的原因；证据只有格式时明确说明用途依据不足，不编造内容。"
		}
		if virtualMemoryWhy(q) {
			return "必须先解释事实中传统管理的一次性、驻留性或容量限制导致的困难，再说明局部性和按需装入如何缓解；不能只讲换页步骤。只写事实支持的内容。"
		}
		return "补齐本题要求的原因、机制或比较双方；不要重复定义代替解释。"
	case "missing_premise":
		return "保留所用事实的全部适用条件和必要前提。"
	case "unsupported_claim":
		return "删除事实中没有支持的结论，不扩写模型常识，允许只回答有依据的部分并说明范围。"
	case "incomplete_claim":
		return "每段写成完整句，删除半句，缩短段落，不续写缺失原文。"
	default:
		return "按要求返回有效JSON和本轮存在的facts编号。"
	}
}
