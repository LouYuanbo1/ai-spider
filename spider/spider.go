package spider

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"codeberg.org/readeck/go-readability/v2"
	"github.com/LouYuanbo1/ai-spider/config"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

type Spider struct {
	browser *rod.Browser
	page    *rod.Page
	logger  *slog.Logger
}

func InitSpider(cfg *config.RodConfig) (*Spider, error) {
	logger := slog.Default().With("component", "spider")

	dataDir := fmt.Sprintf("%s/instance_0", cfg.UserDataDir)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建实例数据目录失败: %v", err)
	}

	launcher := CreateLauncher(cfg.UserMode,
		WithBin(cfg.Bin),
		WithUserDataDir(dataDir),
		WithHeadless(cfg.Headless),
		WithDisableBlinkFeatures(cfg.DisableBlinkFeatures),
		WithIncognito(cfg.Incognito),
		WithDisableDevShmUsage(cfg.DisableDevShmUsage),
		WithNoSandbox(cfg.NoSandbox),
		WithUserAgent(cfg.UserAgent),
		WithLeakless(cfg.Leakless),
		WithDisableBackgroundNetworking(cfg.DisableBackgroundNetworking),
		WithDisableBackgroundTimerThrottling(cfg.DisableBackgroundTimerThrottling),
		WithRemoteDebuggingPort(cfg.BasicRemoteDebuggingPort),
	)
	url, err := launcher.Launch()
	if err != nil {
		return nil, fmt.Errorf("启动浏览器失败: %v", err)
	}
	logger.Info("浏览器就绪", "instance", 1, "url", url)

	browser := rod.New().ControlURL(url).Trace(cfg.Trace)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("连接浏览器失败: %v", err)
	}

	page := stealth.MustPage(browser)

	return &Spider{
		browser: browser,
		page:    page,
		logger:  logger,
	}, nil
}

// Close 关闭爬虫，释放所有浏览器资源
func (s *Spider) Close() error {
	s.logger.Info("关闭爬虫")
	if err := s.browser.Close(); err != nil {
		return fmt.Errorf("关闭浏览器失败: %v", err)
	}
	return nil
}

func (s *Spider) ClosePage() error {
	if err := s.page.Close(); err != nil {
		return fmt.Errorf("关闭页面失败: %v", err)
	}
	return nil
}

// Navigate 导航到目标URL并等待页面加载与网络空闲
func (s *Spider) Navigate(url string) error {
	if err := s.page.Navigate(url); err != nil {
		return fmt.Errorf("导航失败: %v", err)
	}
	if err := s.page.WaitLoad(); err != nil {
		if strings.Contains(err.Error(), "navigated or closed") {
			return nil
		}
		return fmt.Errorf("页面加载等待失败: %v", err)
	}
	s.page.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)
	return nil
}

// DoClick CSS选择器点击，点击后等待网络空闲
func (s *Spider) DoClick(selector string) error {
	el, err := s.page.Element(selector)
	if err != nil {
		return fmt.Errorf("未找到元素 %q: %v", selector, err)
	}
	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	s.page.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)
	return nil
}

// DoClickX XPath点击，点击后等待网络空闲
func (s *Spider) DoClickX(selector string) error {
	el, err := s.page.ElementX(selector)
	if err != nil {
		return fmt.Errorf("未找到XPath元素 %q: %v", selector, err)
	}
	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	s.page.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)
	return nil
}

// DoClickNode 通过后端 DOM 节点 ID 精准点击，避免同名元素/选择器歧义
func (s *Spider) DoClickNode(backendNodeID proto.DOMBackendNodeID) error {
	el, err := s.page.ElementFromNode(&proto.DOMNode{BackendNodeID: backendNodeID})
	if err != nil {
		return fmt.Errorf("解析节点 %d 失败: %v", backendNodeID, err)
	}
	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	s.page.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)
	return nil
}

// typeText 将文本输入到元素；clearFlag 为 true 时先清空原有内容。
// 使用 rod 原生 SelectAllText + Input 触发真实键盘事件，对 React/Vue 受控组件友好。
func typeText(el *rod.Element, text string, clearFlag bool) error {
	if clearFlag {
		// 全选原有内容，随后 Input(text) 会直接覆盖选中文本
		if err := el.SelectAllText(); err != nil {
			return err
		}
	}
	return el.Input(text)
}

// DoInput CSS选择器定位输入框输入文本
func (s *Spider) DoInput(selector, text string, clearFlag bool) error {
	el, err := s.page.Element(selector)
	if err != nil {
		return fmt.Errorf("未找到输入框 %q: %v", selector, err)
	}
	return typeText(el, text, clearFlag)
}

// DoInputX XPath定位输入框输入文本
func (s *Spider) DoInputX(selector, text string, clearFlag bool) error {
	el, err := s.page.ElementX(selector)
	if err != nil {
		return fmt.Errorf("未找到XPath输入框 %q: %v", selector, err)
	}
	return typeText(el, text, clearFlag)
}

// DoInputNode 通过后端 DOM 节点 ID 精准定位输入框并输入文本
func (s *Spider) DoInputNode(backendNodeID proto.DOMBackendNodeID, text string, clearFlag bool) error {
	el, err := s.page.ElementFromNode(&proto.DOMNode{BackendNodeID: backendNodeID})
	if err != nil {
		return fmt.Errorf("解析节点 %d 失败: %v", backendNodeID, err)
	}
	return typeText(el, text, clearFlag)
}

// DoScroll 页面滚动（瞬时完成），滚动后等待网络空闲以捕获懒加载内容
func (s *Spider) DoScroll(scrollY int) error {
	_, err := s.page.Eval(`(dy) => window.scrollBy({top: dy, behavior: 'smooth'})`, scrollY)
	if err != nil {
		return err
	}
	s.page.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)
	return nil
}

// doJavaScript 执行JS并回调处理结果
func (s *Spider) DoJavaScript(ctx context.Context, js string, args ...any) (string, error) {
	// 用 async IIFE 包裹，兼容上层 await 与顶层 return；
	// rod 的 Eval 默认 awaitPromise=true，会自动等待 Promise 完成。
	var source string
	if len(args) > 0 {
		source = js
	} else {
		source = "(async () => { " + js + "\n})()"
	}

	run := func() (*proto.RuntimeRemoteObject, error) {
		return s.page.Eval(source, args...)
	}

	result, err := run()
	if err != nil {
		// 模型生成的 JS 在 SPA 页面（如 B 站）上偶发失败，
		// 重试一次，避免一次偶发异常就中断整次抓取。
		s.logger.Warn("JS执行失败，重试中", "js", js, "err", err)
		time.Sleep(300 * time.Millisecond)
		result, err = run()
		if err != nil {
			s.logger.Error("JS执行失败", "js", js, "err", err)
			return "", fmt.Errorf("JS执行失败: %v", err)
		}
	}

	// 脚本未 return / 返回 undefined 时给一个明确结果，而不是后续序列化崩溃。
	if result == nil || result.Type == proto.RuntimeRemoteObjectTypeUndefined || result.Value.Nil() {
		return "undefined", nil
	}

	data, err := result.Value.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("JSON序列化失败: %v", err)
	}

	return string(data), nil
}

// Link 页面链接
type Link struct {
	Name   string `json:"name"`
	Href   string `json:"href"`
	NodeID int64  `json:"node_id,omitempty"` // 后端 DOM 节点 ID，用于精准点击/定位
}

// Button 页面按钮
type Button struct {
	Name   string `json:"name"`
	Value  string `json:"value,omitempty"`
	NodeID int64  `json:"node_id,omitempty"` // 后端 DOM 节点 ID，用于精准点击
}

// Input 页面文本输入框
type Input struct {
	Name   string `json:"name"`
	Value  string `json:"value,omitempty"`
	NodeID int64  `json:"node_id,omitempty"` // 后端 DOM 节点 ID，用于精准定位输入
}

// Interactives 页面上的可交互元素：链接、按钮与文本输入框
type Interactives struct {
	Links   []Link   `json:"links"`
	Buttons []Button `json:"buttons"`
	Inputs  []Input  `json:"inputs"` // 页面上的文本输入框，可通过 node_id 定位用于 input_tool
}

// ActionSummary 动作执行后的轻量确认结果，避免每次操作都向 LLM 返回全文
type ActionSummary struct {
	Action  string `json:"action"`
	Status  string `json:"status"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Changed bool   `json:"changed"`
	Snippet string `json:"snippet"`
}

// ActionSummary 组装动作确认结果，snippet 为正文文本截断片段
func (s *Spider) ActionSummary(action, status string, changed bool) (string, error) {
	info, _ := s.page.Info()
	title, url := "", ""
	if info != nil {
		title, url = info.Title, info.URL
	}
	summary := ActionSummary{
		Action:  action,
		Status:  status,
		Title:   title,
		URL:     url,
		Changed: changed,
		Snippet: s.bodySnippet(200),
	}
	data, err := json.Marshal(summary)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// bodySnippet 截取页面正文文本片段，用作轻量摘要
func (s *Spider) bodySnippet(maxLen int) string {
	el, err := s.page.Element("body")
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
func (s *Spider) PageFingerprint() string {
	txt := s.bodySnippet(2000)
	info, _ := s.page.Info()
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

// GetInteractives 遍历可访问性树，采集链接、按钮与文本输入框
func (s *Spider) GetInteractives() (string, error) {
	err := proto.AccessibilityEnable{}.Call(s.page)
	if err != nil {
		return "", err
	}

	result, err := proto.AccessibilityGetFullAXTree{}.Call(s.page)
	if err != nil {
		return "", err
	}

	var interactives Interactives
	for _, node := range result.Nodes {
		// 过滤掉无可访问性名、以及没有对应 DOM 节点(无法用于交互/location)的节点
		if node.Role == nil || node.Name == nil || node.BackendDOMNodeID == 0 {
			continue
		}

		switch node.Role.Value.String() {
		case "link":
			interactives.Links = append(interactives.Links, Link{
				Name:   node.Name.Value.String(),
				Href:   s.resolveHref(node.BackendDOMNodeID),
				NodeID: int64(node.BackendDOMNodeID),
			})
		case "button":
			btn := Button{
				Name:   node.Name.Value.String(),
				NodeID: int64(node.BackendDOMNodeID),
			}
			// 注意：button 的跳转动作通常是 JS 点击触发，静态 DOM/AX 里没有
			if node.Value != nil {
				btn.Value = node.Value.Value.String()
			}
			interactives.Buttons = append(interactives.Buttons, btn)
		case "textbox", "searchbox", "combobox", "input":
			// 文本输入框：用于 input_by_node_id_tool 精准定位
			inp := Input{
				Name:   node.Name.Value.String(),
				NodeID: int64(node.BackendDOMNodeID),
			}
			if node.Value != nil {
				inp.Value = node.Value.Value.String()
			}
			interactives.Inputs = append(interactives.Inputs, inp)
		}
	}

	data, err := json.Marshal(interactives)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// resolveHref 通过 DOM 节点读取 href 属性，并补全协议相对地址
func (s *Spider) resolveHref(backendNodeID proto.DOMBackendNodeID) string {
	href, err := s.attrOfNode(backendNodeID, "href")
	if err != nil {
		return ""
	}
	// 协议相对地址 (//host/path) 补成绝对地址
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	return href
}

// attrOfNode 从 DOM 节点中按属性名取值。Attributes 是扁平数组: [名称1, 值1, 名称2, 值2, ...]
func (s *Spider) attrOfNode(backendNodeID proto.DOMBackendNodeID, want string) (string, error) {
	res, err := proto.DOMDescribeNode{BackendNodeID: backendNodeID}.Call(s.page)
	if err != nil || res.Node == nil {
		return "", err
	}

	attrs := res.Node.Attributes
	for i := 0; i+1 < len(attrs); i += 2 {
		if attrs[i] == want {
			return attrs[i+1], nil
		}
	}
	return "", nil
}

func (s *Spider) GetHTML() (string, error) {
	return s.page.HTML()
}

// minArticleLen 判定 readability 提取结果是否“过短”而需要触发兜底的最小字符长度。
const minArticleLen = 30

func (s *Spider) GetArticle() (string, error) {
	html, err := s.page.HTML()
	if err != nil {
		return "", err
	}
	info, err := s.page.Info()
	if err != nil {
		return "", err
	}
	parsedURL, err := url.Parse(info.URL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析 URL 失败: %v\n", err)
		return "", err
	}
	article, err := readability.FromReader(strings.NewReader(html), parsedURL)
	// 兜底：readability 解析失败、节点为空，或提取到的正文过短（基本没内容）时，
	// 回退到用 JS 直接抓取 document.body.innerText，适配百度等结构复杂的页面。
	if err != nil || article.Node == nil || len(strings.TrimSpace(article.Node.Data)) < minArticleLen {
		s.logger.Info("readability 提取效果不佳，回退到 body.innerText", "err", err)
		return s.fallbackBodyText()
	}
	return article.Node.Data, nil
}

// fallbackBodyText 通过 JS 抓取页面 body 的可见文本，作为 readability 提取失败时的兜底方案。
func (s *Spider) fallbackBodyText() (string, error) {
	const js = `document.body ? document.body.innerText : ''`
	result, err := s.page.Eval("() => { return " + js + " }")
	if err != nil {
		return "", fmt.Errorf("JS获取正文失败: %v", err)
	}
	data, err := result.Value.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("JSON序列化正文失败: %v", err)
	}
	// Eval 返回的是 JSON 字符串字面量，需要 strip 掉两端的引号还原为原始文本
	return strings.Trim(strings.TrimSpace(string(data)), `"`), nil
}
