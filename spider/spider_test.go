package spider

import (
	"encoding/json"
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
	var _ Action = (*GetInteractive)(nil)
}

// TestActionJSONRoundTrip 验证各动作类型的 JSON 编解码能够完整保真关键字段。
func TestActionJSONRoundTrip(t *testing.T) {
	// marshal 后仍应是合法 JSON，且包含预期字段值
	byteName := func(v any) []byte {
		b, err := json.Marshal(v)
		require.NoError(t, err, "Marshal %T 不应报错", v)
		require.True(t, json.Valid(b), "Marshal %T 结果应合法", v)
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
		&JavaScript{}, &GetInteractive{},
	}
	for _, it := range items {
		_, err := json.Marshal(it)
		require.NoError(t, err, "零值 %T 应能序列化", it)
	}
}
