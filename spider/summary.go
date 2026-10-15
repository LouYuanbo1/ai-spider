package spider

import (
	"context"
	"encoding/json/v2"
	"strings"

	"github.com/go-rod/rod"
)

// ActionSummary 动作执行后的轻量确认结果，避免每次操作都向 LLM 返回全文。
// Effective 表示本步"操作是否真正生效"，而非笼统的"页面是否有差异"：
//   - navigate：到达目标页面即视为生效
//   - input*：读回输入框当前值，确认已包含目标文本
//   - click* / scroll：页面发生了实质响应（URL/标题跳转或正文内容变化）
type ActionSummary struct {
	Action    string `json:"action"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Effective bool   `json:"effective"`
	Snippet   string `json:"snippet"`
}

// ActionSummary 组装动作确认结果，snippet 为正文文本截断片段
func (s *Spider) ActionSummary(ctx context.Context, action, status string, effective bool) (string, error) {
	p := s.page.Context(ctx)
	info, _ := p.Info()
	title, url := "", ""
	if info != nil {
		title, url = info.Title, info.URL
	}
	summary := ActionSummary{
		Action:    action,
		Status:    status,
		Title:     title,
		URL:       url,
		Effective: effective,
		Snippet:   s.bodySnippet(p, 200),
	}
	data, err := json.Marshal(summary)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// bodySnippet 截取页面正文文本片段，用作轻量摘要
func (s *Spider) bodySnippet(p *rod.Page, maxLen int) string {
	el, err := p.Element("body")
	if err != nil {
		return ""
	}
	txt, err := el.Text()
	if err != nil {
		return ""
	}
	txt = strings.Join(strings.Fields(txt), " ")
	return truncateText(txt, maxLen)
}

// PageFingerprint 计算页面轻量指纹(URL+标题+正文前段)，用于判断动作前后内容是否变化
func (s *Spider) PageFingerprint(ctx context.Context) string {
	p := s.page.Context(ctx)
	txt := s.bodySnippet(p, 2000)
	info, _ := p.Info()
	if info == nil {
		return txt
	}
	return info.URL + "|" + info.Title + "|" + txt
}

// truncateText 按 rune 截断字符串，超出部分以 … 结尾。
// maxLen<=0 时视为 0，避免负数导致切片越界 panic。
func truncateText(s string, maxLen int) string {
	if maxLen <= 0 {
		return "…"
	}
	rs := []rune(s)
	if len(rs) <= maxLen {
		return s
	}
	return string(rs[:maxLen]) + "…"
}
