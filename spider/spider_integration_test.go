package spider

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LouYuanbo1/ai-spider/config"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testFixturePage 一个自包含的可测试页面：含标题、正文、链接、按钮与输入框。
// content 段落长度需超过 linkWallMinLen(40)，以保证结构化剪枝时不被当作链接墙剔除、
// 正文内容能被 prune 保留。
const testFixturePage = `<!DOCTYPE html>
<html lang="zh">
<head><meta charset="utf-8"><title>入口页</title></head>
<body>
  <h1>网站标题</h1>
  <p class="post">这里是一段用于正文内容提取的段落，篇幅设置在四十个字符之上以保证其在剪枝清理阶段不会因链接密度过高或过短而被整体丢弃，从而作为有效正文保留下来。</p>
  <a href="/page2" id="nav">跳转页链接</a>
  <button id="go">更新结果</button>
  <div id="result">初始结果</div>
  <input id="kw" type="text" value="" aria-label="kw">
  <script>
    document.getElementById('go').onclick = function () {
      document.getElementById('result').textContent = '按钮已点击';
    };
  </script>
</body>
</html>`

// newFixtureServer 起一个本地 HTTP 服务，提供入口页与跳转页两条路由，全部离线访问。
func newFixtureServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, testFixturePage)
	})
	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html lang="zh"><head><meta charset="utf-8"><title>跳转页</title></head>
<body><h1>第二页</h1><p>从入口页点击链接跳转后到达本页。</p></body></html>`)
	})
	return httptest.NewServer(mux)
}

// requireBrowserSpider 启动一个真实(无头)Spider 用于集成测试。
// 若环境无法拉起浏览器（未安装/路径不可用）则跳过，避免 CI 上误报失败。
func requireBrowserSpider(t *testing.T) *Spider {
	t.Helper()
	cfg := &config.RodConfig{
		Headless:    true,        // 无头运行，更快且不干扰桌面
		Incognito:   true,        // 无痕，避免污染默认档案
		NoSandbox:   true,        //
		Leakless:    true,        //
		UserDataDir: t.TempDir(), // 独立干净档案目录（Chrome 136+ 必需）
		Humanize:    false,       // 关闭真人化：输入/点击/滚动更确定、更快
	}
	s, err := InitSpider(cfg)
	if err != nil {
		t.Skipf("环境无法启动浏览器，跳过集成测试: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// mustElement 是测试内的辅助：定位失败即终止当前子测试。
func mustElement(t *testing.T, page *rod.Page, selector string) *rod.Element {
	t.Helper()
	el, err := page.Element(selector)
	require.NoError(t, err, "按 %q 定位元素失败", selector)
	return el
}

// TestSpiderCoreIntegration 用真实浏览器覆盖全部浏览器相关方法。
// 单个测试函数复用同一个 Spider 与本地服务，只起一次浏览器；
// 每个子测试都先回到基础页，保证彼此独立、可单独运行。
func TestSpiderCoreIntegration(t *testing.T) {
	s := requireBrowserSpider(t)
	srv := newFixtureServer()
	t.Cleanup(srv.Close)
	base := srv.URL

	t.Run("Navigate 到达页面并读取有效信息", func(t *testing.T) {
		require.NoError(t, s.Navigate(base))
		info, err := s.page.Info()
		require.NoError(t, err)
		assert.Equal(t, "入口页", info.Title, "应导航到入口页并得到正确标题")
	})

	t.Run("GetHTML 返回包含页面标记的完整 HTML", func(t *testing.T) {
		require.NoError(t, s.Navigate(base))
		html, err := s.GetHTML()
		require.NoError(t, err)
		assert.Contains(t, html, "<title>入口页</title>")
		assert.Contains(t, html, `id="go"`)
	})

	t.Run("GetInteractives 采集链接/按钮/输入框", func(t *testing.T) {
		require.NoError(t, s.Navigate(base))
		out, err := s.GetInteractives()
		require.NoError(t, err)
		assert.Contains(t, out, `"跳转页链接"`)
		assert.Contains(t, out, `"更新结果"`)
		assert.Contains(t, out, `"kw"`)
	})

	t.Run("DoInput 注入文本并判定为已生效", func(t *testing.T) {
		require.NoError(t, s.Navigate(base))
		ok, err := s.DoInput("#kw", "Go 爬虫测试", false)
		require.NoError(t, err)
		assert.True(t, ok, "输入目标文本后应判定为已生效")
		got, err := s.elementValue(mustElement(t, s.page, "#kw"))
		require.NoError(t, err)
		assert.Contains(t, got, "Go 爬虫测试")
	})

	t.Run("DoJavaScript 执行 JS 并取回返回值", func(t *testing.T) {
		require.NoError(t, s.Navigate(base))
		val, err := s.DoJavaScript(context.Background(), "return document.title")
		require.NoError(t, err)
		assert.Contains(t, val, "入口页", "JS 应取回页面标题")
	})

	t.Run("DoClick 触发脚本更新 #result", func(t *testing.T) {
		require.NoError(t, s.Navigate(base))
		require.NoError(t, s.DoClick("#go"))
		txt, err := mustElement(t, s.page, "#result").Text()
		require.NoError(t, err)
		assert.Equal(t, "按钮已点击", txt, "点击按钮后 #result 应被脚本更新")
	})

	t.Run("DoClickNode 通过 node_id 再次点击", func(t *testing.T) {
		require.NoError(t, s.Navigate(base))
		out, err := s.GetInteractives()
		require.NoError(t, err)
		// 从采集结果中解析出按钮 node_id，走 node_id 精准点击路径
		var its Interactives
		require.NoError(t, json.Unmarshal([]byte(out), &its), "采集结果应能反序列化")
		require.NotEmpty(t, its.Buttons, "页面应采集到按钮")
		nodeID := proto.DOMBackendNodeID(its.Buttons[0].NodeID)
		require.NoError(t, s.DoClickNode(nodeID), "DoClickNode 应能按 node_id 点击")
		txt, err := mustElement(t, s.page, "#result").Text()
		require.NoError(t, err)
		assert.Equal(t, "按钮已点击", txt, "node_id 点击后 #result 应更新")
	})

	t.Run("DoScroll 正常返回", func(t *testing.T) {
		require.NoError(t, s.DoScroll(300))
	})

	t.Run("ActionSummary 返回有效回执与摘要", func(t *testing.T) {
		out, err := s.ActionSummary("navigate", "ok", true)
		require.NoError(t, err)
		assert.Contains(t, out, `"action":"navigate"`)
		assert.Contains(t, out, `"effective":true`)
		assert.Contains(t, out, `"title":"入口页"`)
	})

	t.Run("PageFingerprint 稳定且随输入变化", func(t *testing.T) {
		require.NoError(t, s.Navigate(base))
		a := s.PageFingerprint()
		b := s.PageFingerprint()
		assert.NotEmpty(t, a, "指纹不应为空")
		assert.Equal(t, a, b, "页面未变时前后两次指纹应一致")

		// input 的 value 不会体现在 innerText 中，因此用点击按钮触发可读文本
		// 更新（#result: 初始结果 -> 按钮已点击），更贴近真实用户行为。
		require.NoError(t, s.DoClick("#go"))
		assert.NotEqual(t, a, s.PageFingerprint(), "内容变化后指纹应不同")
	})

	t.Run("链接点击后跳转到第二页", func(t *testing.T) {
		require.NoError(t, s.Navigate(base))
		require.NoError(t, s.DoClick("#nav"))
		// 锚点点击触发的导航是异步的，立刻读 Info 可能还停留在上一文档，
		// 轮询等待标题真正变为第二页，避免偶发竞态。
		require.Eventually(t, func() bool {
			info, err := s.page.Info()
			return err == nil && info.Title == "跳转页"
		}, 5*time.Second, 100*time.Millisecond, "点击链接后应跳到 page2")
	})
}
