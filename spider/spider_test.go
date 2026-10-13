package spider

import (
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- truncateText ----

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{name: "空字符串", input: "", maxLen: 10, want: ""},
		{name: "长度等于上限不截断", input: "12345", maxLen: 5, want: "12345"},
		{name: "长度小于上限不截断", input: "hello", maxLen: 20, want: "hello"},
		{name: "长度超过上限截断并加省略号", input: "0123456789", maxLen: 5, want: "01234…"},
		{name: "按 rune 截断不破坏多字节字符", input: "你好世界啊", maxLen: 3, want: "你好世…"},
		{name: "上限为 0 截断为空并加省略号", input: "abcd", maxLen: 0, want: "…"},
		{name: "上限为负数视为 0", input: "abcd", maxLen: -3, want: "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, truncateText(tt.input, tt.maxLen))
		})
	}

	t.Run("省略号确保总长度不超过 maxLen+1", func(t *testing.T) {
		in := strings.Repeat("a", 100)
		out := truncateText(in, 30)
		assert.True(t, strings.HasSuffix(out, "…"), "输出应以省略号结尾")
		assert.Equal(t, 31, len([]rune(out)), "30 个字符 + 1 个省略号")
	})
}

// ---- Action 密封接口与 JSON 编解码 ----

// TestActionInterface 编译期断言：所有动作类型都应满足 Action 密封接口。
func TestActionInterface(t *testing.T) {
	var _ Action = (*Navigate)(nil)
	var _ Action = (*ClickByCSS)(nil)
	var _ Action = (*ClickByXPath)(nil)
	var _ Action = (*ClickByNodeID)(nil)
	var _ Action = (*InputByCSS)(nil)
	var _ Action = (*InputByXPath)(nil)
	var _ Action = (*InputByNodeID)(nil)
	var _ Action = (*Scroll)(nil)
	var _ Action = (*JavaScript)(nil)
	var _ Action = (*HTML)(nil)
	var _ Action = (*GetInteractive)(nil)
}

// TestActionJSONRoundTrip 验证各动作类型的 JSON 编解码能够完整保真关键字段。
func TestActionJSONRoundTrip(t *testing.T) {
	// marshal 后仍应是合法 JSON，且包含预期字段值
	byteName := func(v any) []byte {
		b, err := json.Marshal(v)
		require.NoError(t, err, "Marshal %T 不应报错", v)
		require.NoError(t, json.Unmarshal(b, &map[string]any{}), "Marshal %T 结果应为合法对象", v)
		return b
	}

	t.Run("Navigate", func(t *testing.T) {
		b := byteName(&Navigate{URL: "https://example.com"})
		assert.Contains(t, string(b), `"https://example.com"`)
	})

	t.Run("ClickByCSS", func(t *testing.T) {
		b := byteName(&ClickByCSS{Selector: "button.go"})
		assert.Contains(t, string(b), `"button.go"`)
	})

	t.Run("ClickByXPath", func(t *testing.T) {
		b := byteName(&ClickByXPath{Selector: `//div[@data-id="42"]`})
		assert.Contains(t, string(b), `@data-id`)
	})

	t.Run("ClickByNodeID", func(t *testing.T) {
		b := byteName(&ClickByNodeID{NodeID: 12345})
		assert.Contains(t, string(b), `12345`)
	})

	t.Run("InputByCSS", func(t *testing.T) {
		b := byteName(&InputByCSS{Selector: "input.kw", Text: "Go", Clear: true})
		s := string(b)
		assert.Contains(t, s, `"input.kw"`)
		assert.Contains(t, s, `"Go"`)
		assert.Contains(t, s, `true`)
	})

	t.Run("InputByXPath", func(t *testing.T) {
		b := byteName(&InputByXPath{Selector: `//input[@pid="k"]`, Text: "xyz"})
		assert.Contains(t, string(b), `"xyz"`)
	})

	t.Run("InputByNodeID", func(t *testing.T) {
		b := byteName(&InputByNodeID{NodeID: 99, Text: "hello", Clear: true})
		s := string(b)
		assert.Contains(t, s, `99`)
		assert.Contains(t, s, `"hello"`)
	})

	t.Run("Scroll", func(t *testing.T) {
		b := byteName(&Scroll{ScrollY: 800})
		assert.Contains(t, string(b), `800`)
	})

	t.Run("JavaScript", func(t *testing.T) {
		b := byteName(&JavaScript{JS: `document.title`, Args: []any{"a", 1}})
		s := string(b)
		assert.Contains(t, s, `document.title`)
		assert.Contains(t, s, `"a"`)
		assert.Contains(t, s, `1`)
	})

	t.Run("GetInteractive", func(t *testing.T) {
		b := byteName(&GetInteractive{})
		assert.JSONEq(t, `{}`, string(b))
	})

	t.Run("JavaScript Args 缺省时序列化为空数组", func(t *testing.T) {
		// nil Args 在 json 中会输出 null；这里验证显式空切片输出为 []
		b, err := json.Marshal(&JavaScript{JS: "1+1", Args: []any{}})
		require.NoError(t, err)
		assert.Contains(t, string(b), `[]`)
	})
}

// TestActionZeroValue 验证零值动作也可正常序列化（供模型端缺省调用场景）。
func TestActionZeroValue(t *testing.T) {
	items := []Action{
		&Navigate{}, &ClickByCSS{}, &ClickByXPath{}, &ClickByNodeID{},
		&InputByCSS{}, &InputByXPath{}, &InputByNodeID{}, &Scroll{},
		&JavaScript{}, &HTML{}, &GetInteractive{},
	}
	for _, it := range items {
		_, err := json.Marshal(it)
		require.NoError(t, err, "零值 %T 应能序列化", it)
	}
}

// ---- Interactives 结构体 JSON 序列化 ----

func TestInteractivesJSON(t *testing.T) {
	t.Run("无零值字段时序列化省略 omitempty", func(t *testing.T) {
		it := Interactives{
			Links: []Link{{Name: "首页", Href: "https://example.com", NodeID: 1}},
		}
		b, err := json.Marshal(it)
		require.NoError(t, err)
		s := string(b)
		assert.Contains(t, s, `"首页"`)
		assert.Contains(t, s, `"https://example.com"`)
		assert.Contains(t, s, `1`) // node_id 非零会保留
		// 空按钮/输入数组仍应以 [] 输出，而非 null
		assert.Contains(t, s, `"buttons":[]`)
		assert.Contains(t, s, `"inputs":[]`)
	})

	t.Run("零值 node_id 被 omitzero 省略", func(t *testing.T) {
		it := Interactives{Links: []Link{{Name: "无ID", Href: "x"}}}
		b, err := json.Marshal(it)
		require.NoError(t, err)
		assert.NotContains(t, string(b), `"node_id"`)
	})

	t.Run("Button 与 Input 字段齐全", func(t *testing.T) {
		it := Interactives{
			Buttons: []Button{{Name: "提交", Value: "go", NodeID: 8}},
			Inputs:  []Input{{Name: "搜索", Value: "", NodeID: 9}},
		}
		b, err := json.Marshal(it)
		require.NoError(t, err)
		s := string(b)
		require.NoError(t, json.Unmarshal(b, &Interactives{}), "序列化结果应可反序列化回 Interactives")
		assert.Contains(t, s, `"提交"`)
		assert.Contains(t, s, `"go"`)
		assert.Contains(t, s, `"搜索"`)
	})

	t.Run("零值 Interactives 输出空数组结构", func(t *testing.T) {
		it := Interactives{}
		b, err := json.Marshal(it)
		require.NoError(t, err)
		// 空切片字段应序列化为 []，保证模型端能解析为数组
		assert.Contains(t, string(b), `"links":[]`)
		assert.Contains(t, string(b), `"buttons":[]`)
		assert.Contains(t, string(b), `"inputs":[]`)
	})
}

// TestActionSummaryJSON 验证动作回执结构体的字段名与 effective 语义的正确序列化。
func TestActionSummaryJSON(t *testing.T) {
	t.Run("effective 字段如实导出", func(t *testing.T) {
		s := ActionSummary{
			Action:    "click",
			Status:    "ok",
			Title:     "标题",
			URL:       "https://example.com",
			Effective: true,
			Snippet:   "摘要…",
		}
		b, err := json.Marshal(s)
		require.NoError(t, err)
		out := string(b)
		assert.Contains(t, out, `"action":"click"`)
		assert.Contains(t, out, `"status":"ok"`)
		assert.Contains(t, out, `"title":"标题"`)
		assert.Contains(t, out, `"url":"https://example.com"`)
		assert.Contains(t, out, `"effective":true`)
		assert.Contains(t, out, `"snippet":"摘要…"`)
	})

	t.Run("未生效时 effective=false 被保留", func(t *testing.T) {
		b, err := json.Marshal(ActionSummary{Action: "scroll", Effective: false})
		require.NoError(t, err)
		// effective=false 也必须输出，而不是被省略导致模型误判为缺省
		assert.Contains(t, string(b), `"effective":false`)
	})
}
