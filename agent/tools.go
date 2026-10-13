package agent

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"log"
	"strings"

	"github.com/LouYuanbo1/ai-spider/spider"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/go-rod/rod/lib/proto"
)

// debugTool 包装工具执行函数：将调用参数与返回值打印到终端，便于调试。
// 不影响工具本身的行为，仅在执行前后输出日志。
func debugTool[T any](name string, fn func(ctx context.Context, params *T) (string, error)) func(ctx context.Context, params *T) (string, error) {
	return func(ctx context.Context, params *T) (string, error) {
		argJSON, _ := json.Marshal(params)
		out, err := fn(ctx, params)
		if err != nil {
			// 记录原始错误到终端，但不再向上抛 err。
			// 改为把错误作为“普通结果文本”回传给模型，
			// 让模型反思后重试 / 调整参数 / 换方案，而不是中断整次运行。
			log.Printf("[工具失败] %s\n  参数: %s\n  错误: %v", name, argJSON, err)
			return fmt.Sprintf("【%s 执行失败】%v\n请分析失败原因，调整参数或换一种方式重新尝试。", name, err), nil
		}
		view := out
		if len(view) > 500 {
			view = view[:500] + "\n  ...(输出过长已截断)"
		}
		log.Printf("[工具调用] %s\n  参数: %s\n  返回: %s", name, argJSON, view)
		return out, nil
	}
}

func NavigateFunc(ctx context.Context, spd *spider.Spider) func(ctx context.Context, params *spider.Navigate) (string, error) {
	return func(ctx context.Context, params *spider.Navigate) (string, error) {
		cleanURL := strings.TrimSpace(params.URL)
		// 移除首尾的反引号（解决LLM流式输出时带Markdown反引号的问题）
		cleanURL = strings.Trim(cleanURL, "`")
		// 再次Trim掉反引号移除后可能残留的空格
		cleanURL = strings.TrimSpace(cleanURL)
		err := spd.Navigate(cleanURL)
		if err != nil {
			return "", err
		}
		// 导航后到达新页面，视为内容已变化；仅返回轻量确认，需内容时由 LLM 调用 html_tool
		return spd.ActionSummary("navigate", "ok", true)
	}
}

func NewNavigateTool(ctx context.Context, spider *spider.Spider) (tool.InvokableTool, error) {
	navigateTool, err := utils.InferTool(
		"navigate_tool", // tool name
		`Navigate the browser to a specified URL and wait for the page to fully load;
		use this to visit any web page, e.g.: https://www.example.com`, // tool description
		debugTool("navigate_tool", NavigateFunc(ctx, spider)))
	if err != nil {
		return nil, err
	}
	return navigateTool, nil
}

func ClickByCSSFunc(ctx context.Context, spd *spider.Spider) func(ctx context.Context, params *spider.ClickByCSS) (string, error) {
	return func(ctx context.Context, params *spider.ClickByCSS) (string, error) {
		before := spd.PageFingerprint()
		err := spd.DoClick(params.Selector)
		if err != nil {
			return "", err
		}
		return spd.ActionSummary("click_by_css", "ok", before != spd.PageFingerprint())
	}
}

func NewClickByCSSTool(ctx context.Context, spider *spider.Spider) (tool.InvokableTool, error) {
	return utils.InferTool(
		"click_by_css_tool",
		`Click an element on the page using a CSS selector; use this when you know the target element's CSS selector, e.g. button.load-more`,
		debugTool("click_by_css_tool", ClickByCSSFunc(ctx, spider)))
}

func ClickByXPathFunc(ctx context.Context, spd *spider.Spider) func(ctx context.Context, params *spider.ClickByXPath) (string, error) {
	return func(ctx context.Context, params *spider.ClickByXPath) (string, error) {
		before := spd.PageFingerprint()
		err := spd.DoClickX(params.Selector)
		if err != nil {
			return "", err
		}
		return spd.ActionSummary("click_by_xpath", "ok", before != spd.PageFingerprint())
	}
}

func NewClickByXPathTool(ctx context.Context, spider *spider.Spider) (tool.InvokableTool, error) {
	return utils.InferTool(
		"click_by_xpath_tool",
		`Click an element on the page using an XPath selector; use this when you know the target element's XPath, e.g. //div[@data-id="42"]/button`,
		debugTool("click_by_xpath_tool", ClickByXPathFunc(ctx, spider)))
}

func ClickByNodeIDFunc(ctx context.Context, spd *spider.Spider) func(ctx context.Context, params *spider.ClickByNodeID) (string, error) {
	return func(ctx context.Context, params *spider.ClickByNodeID) (string, error) {
		before := spd.PageFingerprint()
		err := spd.DoClickNode(proto.DOMBackendNodeID(params.NodeID))
		if err != nil {
			return "", err
		}
		return spd.ActionSummary("click_by_node_id", "ok", before != spd.PageFingerprint())
	}
}

func NewClickByNodeIDTool(ctx context.Context, spider *spider.Spider) (tool.InvokableTool, error) {
	return utils.InferTool(
		"click_by_node_id_tool",
		`Click a clickable element by its node_id returned from get_interactives_tool; this is the most accurate way to click and avoids ambiguity from duplicate text/selectors. Prefer this over css/xpath when a fresh node_id is available. NOTE: node_id becomes stale after a full page navigation or large DOM re-render, re-run get_interactives_tool to refresh it.`,
		debugTool("click_by_node_id_tool", ClickByNodeIDFunc(ctx, spider)))
}

func InputByCSSFunc(ctx context.Context, spd *spider.Spider) func(ctx context.Context, params *spider.InputByCSS) (string, error) {
	return func(ctx context.Context, params *spider.InputByCSS) (string, error) {
		effective, err := spd.DoInput(params.Selector, params.Text, params.Clear)
		if err != nil {
			return "", err
		}
		return spd.ActionSummary("input_by_css", "ok", effective)
	}
}

func NewInputByCSSTool(ctx context.Context, spider *spider.Spider) (tool.InvokableTool, error) {
	return utils.InferTool(
		"input_by_css_tool",
		`Type text into an input field on the page using a CSS selector; use this when you know the input field's CSS selector, e.g. input.search-input or #keyword. Set clear=true to first erase existing text. Prefer input_by_node_id_tool when a node_id is available.`,
		debugTool("input_by_css_tool", InputByCSSFunc(ctx, spider)))
}

func InputByXPathFunc(ctx context.Context, spd *spider.Spider) func(ctx context.Context, params *spider.InputByXPath) (string, error) {
	return func(ctx context.Context, params *spider.InputByXPath) (string, error) {
		effective, err := spd.DoInputX(params.Selector, params.Text, params.Clear)
		if err != nil {
			return "", err
		}
		return spd.ActionSummary("input_by_xpath", "ok", effective)
	}
}

func NewInputByXPathTool(ctx context.Context, spider *spider.Spider) (tool.InvokableTool, error) {
	return utils.InferTool(
		"input_by_xpath_tool",
		`Type text into an input field on the page using an XPath selector; use this when you know the input field's XPath, e.g. //input[@placeholder="搜索"]. Set clear=true to first erase existing text.`,
		debugTool("input_by_xpath_tool", InputByXPathFunc(ctx, spider)))
}

func InputByNodeIDFunc(ctx context.Context, spd *spider.Spider) func(ctx context.Context, params *spider.InputByNodeID) (string, error) {
	return func(ctx context.Context, params *spider.InputByNodeID) (string, error) {
		effective, err := spd.DoInputNode(proto.DOMBackendNodeID(params.NodeID), params.Text, params.Clear)
		if err != nil {
			return "", err
		}
		return spd.ActionSummary("input_by_node_id", "ok", effective)
	}
}

func NewInputByNodeIDTool(ctx context.Context, spider *spider.Spider) (tool.InvokableTool, error) {
	return utils.InferTool(
		"input_by_node_id_tool",
		`Type text into an input field by its node_id listed under "inputs" from get_interactives_tool; this is the most accurate way to input and avoids ambiguity. Prefer this over css/xpath when a fresh node_id is available. NOTE: node_id becomes stale after a full page navigation or large DOM re-render, re-run get_interactives_tool to refresh it.`,
		debugTool("input_by_node_id_tool", InputByNodeIDFunc(ctx, spider)))
}

func ScrollFunc(ctx context.Context, spd *spider.Spider) func(ctx context.Context, params *spider.Scroll) (string, error) {
	return func(ctx context.Context, params *spider.Scroll) (string, error) {
		before := spd.PageFingerprint()
		err := spd.DoScroll(params.ScrollY)
		if err != nil {
			return "", err
		}
		return spd.ActionSummary("scroll", "ok", before != spd.PageFingerprint())
	}
}

func NewScrollTool(ctx context.Context, spider *spider.Spider) (tool.InvokableTool, error) {
	scrollTool, err := utils.InferTool(
		"scroll_tool", // tool name
		`Web browser are used to scroll the page; 
		they can be used to scroll the page by a specified amount of pixels;`, // tool description
		debugTool("scroll_tool", ScrollFunc(ctx, spider)))
	if err != nil {
		return nil, err
	}
	return scrollTool, nil
}

func JavaScriptFunc(ctx context.Context, spd *spider.Spider) func(ctx context.Context, params *spider.JavaScript) (string, error) {
	return func(ctx context.Context, params *spider.JavaScript) (string, error) {
		result, err := spd.DoJavaScript(ctx, params.JS, params.Args...)
		if err != nil {
			return "", err
		}
		return result, nil
	}
}

func NewJavaScriptTool(ctx context.Context, spider *spider.Spider) (tool.InvokableTool, error) {
	jsTool, err := utils.InferTool(
		"javascript_tool", // tool name
		`Web browser are used to execute JavaScript code on the page; 
		they can be used to execute JavaScript code on the page, eg: document.querySelector('button').click()`, // tool description
		debugTool("javascript_tool", JavaScriptFunc(ctx, spider)))
	if err != nil {
		return nil, err
	}
	return jsTool, nil
}

func HTMLFunc(ctx context.Context, spd *spider.Spider) func(ctx context.Context, params *spider.HTML) (string, error) {
	return func(ctx context.Context, params *spider.HTML) (string, error) {
		mode := spider.DefaultHTMLMode(params.Mode)
		switch mode {
		case spider.HTMLModeStructured:
			return spd.GetStructured()
		case spider.HTMLModeFull:
			return spd.GetHTML()
		case spider.HTMLModeAccessibility:
			return spd.GetAXContent()
		default:
			return "", fmt.Errorf("未知的 mode=%q，可选：structured|accessibility|full", mode)
		}
	}
}

func NewHTMLTool(ctx context.Context, spider *spider.Spider) (tool.InvokableTool, error) {
	htmlTool, err := utils.InferTool(
		"html_tool", // tool name
		`Get the content of the current page in one of three modes: accessibility (default, preferred), structured, full. accessibility builds compact Markdown from the browser Accessibility/AX tree, which naturally excludes hidden/decorative/script nodes, restores headings/lists/tables and parent-child nesting, and attaches a node_id marker to every interactive element (links/buttons/inputs) so you can directly click or type at them without re-fetching the interactives list -- this is the most token-efficient and interactive-friendly choice, use by default; structured converts the pruned DOM to readable Markdown (headings/lists/tables/links with absolute URLs) and preserves rich inline formatting like bold/inline code, use when content is content-rich reading material; full returns the complete raw HTML with precise DOM structure and CSS attributes (expensive, only when you need the original source, code snippets, data tables, or CSS-attribute/xpath-based selectors).`, // tool description
		debugTool("html_tool", HTMLFunc(ctx, spider)))
	if err != nil {
		return nil, err
	}
	return htmlTool, nil
}

func GetInteractivesFunc(ctx context.Context, spd *spider.Spider) func(ctx context.Context, params *spider.GetInteractive) (string, error) {
	return func(ctx context.Context, params *spider.GetInteractive) (string, error) {
		return spd.GetInteractives()
	}
}

func NewGetInteractivesTool(ctx context.Context, spider *spider.Spider) (tool.InvokableTool, error) {
	interactivesTool, err := utils.InferTool(
		"get_interactives_tool", // tool name
		`Web browser are used to get all interactive elements on this page, including clickable links, buttons and text input fields, each with its node_id`, // tool description
		debugTool("get_interactives_tool", GetInteractivesFunc(ctx, spider)))
	if err != nil {
		return nil, err
	}
	return interactivesTool, nil
}
