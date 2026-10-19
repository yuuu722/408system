package service

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"net/url"
	"strings"
	"time"
)

func ValidateAsset(a domain.Asset) error {
	if a.Payload.EstimatedMinutes < 0 || a.Payload.EstimatedMinutes > 600 {
		return fmt.Errorf("预计学习时长须在0至600分钟（Invalid duration）")
	}
	invalid := func(s string) error { return fmt.Errorf("%s（Invalid resource）", s) }
	if _, e := uuid.Parse(a.PointID); a.PointID != "" && e != nil {
		return invalid("请选择知识点")
	}
	if strings.TrimSpace(a.Title) == "" || len([]rune(a.Title)) > 200 {
		return invalid("标题须为1至200字")
	}
	if a.Kind != "question" && a.Kind != "video" && a.Kind != "material" {
		return invalid("不支持的资源类型")
	}
	if a.Payload.Difficulty < 1 || a.Payload.Difficulty > 5 {
		return invalid("难度应为1至5")
	}
	if a.Payload.Page < 0 || a.Payload.StartSeconds < 0 || a.Payload.EndSeconds < 0 || a.Payload.EndSeconds > 0 && a.Payload.EndSeconds <= a.Payload.StartSeconds {
		return invalid("页码或视频时间段不正确")
	}
	if a.Payload.URL != "" {
		u, e := url.Parse(a.Payload.URL)
		if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return invalid("链接必须为有效的HTTP或HTTPS地址")
		}
	}

	return nil
}
func ValidateProfile(p domain.Profile) error {
	if p.GoalScore < 0 || p.GoalScore > 150 || p.DailyMinutes < 10 || p.DailyMinutes > 600 {
		return fmt.Errorf("目标分数或每日时间超出范围（Invalid profile）")
	}
	if _, e := time.Parse("2006-01-02", p.ExamDate); e != nil {
		return fmt.Errorf("考试日期格式不正确（Invalid date）")
	}
	if p.Stage != "beginner" && p.Stage != "review" && p.Stage != "sprint" {
		return fmt.Errorf("请选择复习阶段（Invalid stage）")
	}
	if p.Preference != "text" && p.Preference != "video" && p.Preference != "mixed" {
		return fmt.Errorf("请选择学习偏好（Invalid preference）")
	}
	return nil
}
