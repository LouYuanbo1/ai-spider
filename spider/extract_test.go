package spider

import (
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/assert"
	"github.com/ysmood/gson"
	"golang.org/x/net/html"
)

// parseElement 将一段 HTML 片段解析为单个元素节点（自动包裹 <html><body>）。
func parseElement(t *testing.T, raw string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(`<html><body>` + raw + `</body></html>`))
	requireNoErr(t, err)
	var find func(n *html.Node) *html.Node
	find = func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode && n.Data == "body" && n.FirstChild != nil {
			return n.FirstChild
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if got := find(c); got != nil {
				return got
			}
		}
		return nil
	}
	el := find(doc)
	if el == nil {
		t.Fatal("元素解析失败，未找到 body 子节点")
	}
	return el
}

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// ---- collapseWS ----

func TestCollapseWS(t *testing.T) {
	t.Run("空串不变", func(t *testing.T) {
		assert.Equal(t, "", collapseWS(""))
	})
	t.Run("首尾空白被去除", func(t *testing.T) {
		assert.Equal(t, "abc", collapseWS("  abc  "))
	})
	t.Run("连续空白压成单空格", func(t *testing.T) {
		assert.Equal(t, "Go 爬虫 测试", collapseWS("Go \n \t 爬虫　 测试"))
	})
	t.Run("全角空格被归一", func(t *testing.T) {
		assert.Equal(t, "你好 世界", collapseWS("你好\u3000世界"))
	})
	t.Run("纯空白返回空串", func(t *testing.T) {
		assert.Equal(t, "", collapseWS("\n\t  \u3000"))
	})
}

// ---- isHiddenElement ----

func TestIsHiddenElement(t *testing.T) {
	t.Run("text 节点不视为隐藏", func(t *testing.T) {
		assert.False(t, isHiddenElement(&html.Node{Type: html.TextNode, Data: "文本"}))
	})
	t.Run("hidden 属性命中", func(t *testing.T) {
		assert.True(t, isHiddenElement(parseElement(t, `<div hidden>t</div>`)))
	})
	t.Run("aria-hidden=true 命中", func(t *testing.T) {
		assert.True(t, isHiddenElement(parseElement(t, `<span aria-hidden="true">t</span>`)))
	})
	t.Run("aria-hidden=false 不命中", func(t *testing.T) {
		assert.False(t, isHiddenElement(parseElement(t, `<span aria-hidden="false">t</span>`)))
	})
	t.Run("display:none 命中(忽略空白大小写)", func(t *testing.T) {
		assert.True(t, isHiddenElement(parseElement(t, `<div style="DISPLAY:none">t</div>`)))
	})
	t.Run("visibility:hidden 命中", func(t *testing.T) {
		assert.True(t, isHiddenElement(parseElement(t, `<div style="color:red; visibility:hidden">t</div>`)))
	})
	t.Run("display:block 不命中", func(t *testing.T) {
		assert.False(t, isHiddenElement(parseElement(t, `<div style="display:block">t</div>`)))
	})
	t.Run("无隐藏属性不命中", func(t *testing.T) {
		assert.False(t, isHiddenElement(parseElement(t, `<p>可见</p>`)))
	})
}

// ---- isSkippable ----

func TestIsSkippable(t *testing.T) {
	t.Run("script 命中", func(t *testing.T) {
		assert.True(t, isSkippable(parseElement(t, `<script>var a=1</script>`)))
	})
	t.Run("style 命中", func(t *testing.T) {
		assert.True(t, isSkippable(parseElement(t, `<style>body{}</style>`)))
	})
	t.Run("head 命中", func(t *testing.T) {
		assert.True(t, isSkippable(&html.Node{Type: html.ElementNode, Data: "head"}))
	})
	t.Run("隐藏元素命中", func(t *testing.T) {
		assert.True(t, isSkippable(parseElement(t, `<div hidden></div>`)))
	})
	t.Run("普通元素不命中", func(t *testing.T) {
		assert.False(t, isSkippable(parseElement(t, `<p>正文</p>`)))
	})
	t.Run("text 节点不命中", func(t *testing.T) {
		assert.False(t, isSkippable(&html.Node{Type: html.TextNode, Data: "text"}))
	})
}

// ---- subtreeStats ----

func TestSubtreeStats(t *testing.T) {
	t.Run("纯文本计数", func(t *testing.T) {
		textLen, linkLen := subtreeStats(parseElement(t, `<p>hello world</p>`))
		assert.Equal(t, 11, textLen)
		assert.Equal(t, 0, linkLen)
	})
	t.Run("链接文本同时计入 textLen 与 linkLen", func(t *testing.T) {
		textLen, linkLen := subtreeStats(parseElement(t, `<p>aa<a>xyz</a></p>`))
		assert.Equal(t, 5, textLen) // aa(2) + xyz(3)
		assert.Equal(t, 3, linkLen) // 仅链接部分计入链接占比
	})
	t.Run("跳过 script 噪音", func(t *testing.T) {
		textLen, _ := subtreeStats(parseElement(t, `<div><script>let longVar=123</script>ok</div>`))
		assert.Equal(t, 2, textLen)
	})
	t.Run("注释与空白不计数", func(t *testing.T) {
		textLen, _ := subtreeStats(parseElement(t, `<div><!-- 注释 -->  <span>ab</span>  </div>`))
		assert.Equal(t, 2, textLen)
	})
}

// ---- isLinkWall ----

// linkA 生成 <a>链接</a>，text 为锚文本。
func linkA(text string) string { return `<a href="/x">` + text + `</a>` }

func TestIsLinkWall(t *testing.T) {
	text40 := "一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十" // 40 字
	t.Run("短块不判定(文本不足阈值)", func(t *testing.T) {
		assert.False(t, isLinkWall(parseElement(t, `<nav>`+linkA("短")+`</nav>`)))
	})
	t.Run("纯链接墙命中", func(t *testing.T) {
		raw := `<nav>` + linkA(text40+"甲") + linkA(text40+"乙") + linkA(text40+"丙") + `</nav>`
		assert.True(t, isLinkWall(parseElement(t, raw)))
	})
	t.Run("正文主导不命中", func(t *testing.T) {
		// 大量正文 + 少量链接，链接占比远低于 0.7
		body := strings.Repeat("这是很长的正文内容，用于稀释链接占比，让块被判定为正文而非链接墙。", 12)
		raw := `<div>` + linkA("短") + `<p>` + body + `</p></div>`
		assert.False(t, isLinkWall(parseElement(t, raw)))
	})
	t.Run("空节点不命中", func(t *testing.T) {
		assert.False(t, isLinkWall(parseElement(t, `<div></div>`)))
	})
}

// ---- prune ----

func TestPrune(t *testing.T) {
	t.Run("空文本节点返回 false", func(t *testing.T) {
		assert.False(t, prune(&html.Node{Type: html.TextNode, Data: "   "}))
	})
	t.Run("script 子树被剔除", func(t *testing.T) {
		el := parseElement(t, `<div><script>var a=1</script><p>正文</p></div>`)
		assert.True(t, prune(el))
		render := renderOf(t, el)
		assert.Contains(t, render, "<p>正文</p>")
		assert.NotContains(t, render, "script")
	})
	t.Run("隐藏外壳被剔除", func(t *testing.T) {
		el := parseElement(t, `<div><div hidden>隐藏</div><p>可见</p></div>`)
		assert.True(t, prune(el))
		render := renderOf(t, el)
		assert.Contains(t, render, "可见")
		assert.NotContains(t, render, "隐藏")
	})
	t.Run("语义标签即使无子内容也保留", func(t *testing.T) {
		el := parseElement(t, `<div><p>正文</p><img src="/a.png" alt="图"></div>`)
		assert.True(t, prune(el))
		render := renderOf(t, el)
		assert.Contains(t, render, `img src="/a.png"`)
		assert.Contains(t, render, "<p>正文</p>")
	})
	t.Run("属性白名单过滤", func(t *testing.T) {
		el := parseElement(t, `<a href="/x" class="nav" style="color:red">链接</a>`)
		prune(el)
		render := renderOf(t, el)
		assert.Contains(t, render, `href="/x"`)
		assert.NotContains(t, render, "class=")
		assert.NotContains(t, render, "style=")
	})
	t.Run("纯布局外壳无有效内容被剔除", func(t *testing.T) {
		el := parseElement(t, `<div><span></span></div>`)
		assert.False(t, prune(el))
	})
	t.Run("链接墙外壳被剔除", func(t *testing.T) {
		text40 := strings.Repeat("导航", 20)
		raw := `<nav>` + linkA(text40+"一") + linkA(text40+"二") + linkA(text40+"三") + `</nav>`
		el := parseElement(t, `<div>`+raw+`</div>`)
		// 外壳 div 内的 nav 因链接墙被剔除 → div 自身无有效内容
		assert.False(t, prune(el))
	})
}

// renderOf 把解析的元素节点重渲染为 HTML 字符串。
func renderOf(t *testing.T, el *html.Node) string {
	t.Helper()
	var b strings.Builder
	if err := html.Render(&b, el); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// ---- roleName / axString / axPropertyValue ----

func axValue(s string) *proto.AccessibilityAXValue {
	return &proto.AccessibilityAXValue{Value: gson.New(s)}
}

func TestRoleName(t *testing.T) {
	t.Run("nil 返回空串", func(t *testing.T) {
		assert.Equal(t, "", roleName(nil))
	})
	t.Run("有值时返回字符串", func(t *testing.T) {
		assert.Equal(t, "link", roleName(axValue("link")))
	})
}

func TestAxString(t *testing.T) {
	t.Run("nil 返回空串", func(t *testing.T) {
		assert.Equal(t, "", axString(nil))
	})
	t.Run("有值时返回裁剪后字符串", func(t *testing.T) {
		assert.Equal(t, "按钮", axString(axValue("  按钮  ")))
	})
}

func TestAxPropertyValue(t *testing.T) {
	mk := func(level string) *proto.AccessibilityAXNode {
		return &proto.AccessibilityAXNode{
			Properties: []*proto.AccessibilityAXProperty{
				{Name: "level", Value: axValue(level)},
				{Name: "busy", Value: axValue("false")},
			},
		}
	}
	t.Run("命中返回对应值", func(t *testing.T) {
		assert.Equal(t, "3", axPropertyValue(mk("3"), "level"))
	})
	t.Run("未命中返回空串", func(t *testing.T) {
		assert.Equal(t, "", axPropertyValue(mk("3"), "selected"))
	})
	t.Run("nil 属性被跳过不 panic", func(t *testing.T) {
		n := &proto.AccessibilityAXNode{
			Properties: []*proto.AccessibilityAXProperty{nil},
		}
		assert.Equal(t, "", axPropertyValue(n, "level"))
	})
}

// ---- buildAXTree ----

func TestBuildAXTree(t *testing.T) {
	mk := func(id, parent string) *proto.AccessibilityAXNode {
		return &proto.AccessibilityAXNode{
			NodeID:   proto.AccessibilityAXNodeID(id),
			ParentID: proto.AccessibilityAXNodeID(parent),
		}
	}
	t.Run("重建父子关系", func(t *testing.T) {
		forest := buildAXTree([]*proto.AccessibilityAXNode{
			mk("a", ""),
			mk("b", "a"),
			mk("c", "a"),
		})
		if assert.Len(t, forest, 1) {
			assert.Equal(t, proto.AccessibilityAXNodeID("a"), forest[0].node.NodeID)
			assert.Len(t, forest[0].children, 2)
		}
	})
	t.Run("父节点缺失时回退为根", func(t *testing.T) {
		forest := buildAXTree([]*proto.AccessibilityAXNode{mk("orphan", "ghost")})
		assert.Len(t, forest, 1)
		assert.Equal(t, proto.AccessibilityAXNodeID("orphan"), forest[0].node.NodeID)
	})
	t.Run("nil 节点被忽略", func(t *testing.T) {
		assert.Len(t, buildAXTree([]*proto.AccessibilityAXNode{nil}), 0)
	})
	t.Run("多根共存", func(t *testing.T) {
		forest := buildAXTree([]*proto.AccessibilityAXNode{mk("r1", ""), mk("r2", "")})
		assert.Len(t, forest, 2)
	})
	t.Run("根节点与父子结构按输入顺序稳定保留", func(t *testing.T) {
		// AX NodeID 无顺序语义，构建必须保留浏览器 GetFullAXTree 返回的
		// 文档/遍历顺序（父在前、子在后），且不随 Go map 迭代而随机打乱。
		forest := buildAXTree([]*proto.AccessibilityAXNode{
			mk("30", ""), mk("5", ""), mk("100", "30"), mk("42", "30"), mk("9", "5"),
		})
		if assert.Len(t, forest, 2) {
			// 根节点保持输入出现顺序。
			assert.Equal(t, proto.AccessibilityAXNodeID("30"), forest[0].node.NodeID)
			assert.Equal(t, proto.AccessibilityAXNodeID("5"), forest[1].node.NodeID)
		}
		// 子节点保持输入出现顺序（100 在 42 之前）。
		if assert.Len(t, forest[0].children, 2) {
			assert.Equal(t, proto.AccessibilityAXNodeID("100"), forest[0].children[0].node.NodeID)
			assert.Equal(t, proto.AccessibilityAXNodeID("42"), forest[0].children[1].node.NodeID)
		}
		if assert.Len(t, forest[1].children, 1) {
			assert.Equal(t, proto.AccessibilityAXNodeID("9"), forest[1].children[0].node.NodeID)
		}
	})
}

// ---- renderAXTable ----

func TestRenderAXTable(t *testing.T) {
	// tree 帮助构造：nodeID → role；childIDs 下的子节点
	mk := func(id, role string, name string) *proto.AccessibilityAXNode {
		var nm *proto.AccessibilityAXValue
		if name != "" {
			nm = axValue(name)
		}
		return &proto.AccessibilityAXNode{
			NodeID: proto.AccessibilityAXNodeID(id),
			Role:   axValue(role),
			Name:   nm,
		}
	}
	t.Run("无表头普通表", func(t *testing.T) {
		root := &axTreeNode{node: mk("t", "table", "")}
		root.children = []*axTreeNode{
			{node: mk("r1", "row", ""), children: []*axTreeNode{
				{node: mk("c1", "cell", "甲")},
				{node: mk("c2", "cell", "乙")},
			}},
			{node: mk("r2", "row", ""), children: []*axTreeNode{
				{node: mk("c3", "cell", "1")},
				{node: mk("c4", "cell", "2")},
			}},
		}
		got := renderAXTable(root)
		assert.Equal(t, "| 甲 | 乙 |\n| 1 | 2 |", got)
	})
	t.Run("首行表头生成分隔线", func(t *testing.T) {
		root := &axTreeNode{node: mk("t", "table", "")}
		root.children = []*axTreeNode{
			{node: mk("r1", "row", ""), children: []*axTreeNode{
				{node: mk("h1", "columnheader", "姓名")},
				{node: mk("h2", "columnheader", "年龄")},
			}},
			{node: mk("r2", "row", ""), children: []*axTreeNode{
				{node: mk("d1", "cell", "张三")},
				{node: mk("d2", "cell", "28")},
			}},
		}
		got := renderAXTable(root)
		assert.Equal(t, "| 姓名 | 年龄 |\n| --- | --- |\n| 张三 | 28 |", got)
	})
	t.Run("穿透 rowgroup 容器", func(t *testing.T) {
		root := &axTreeNode{node: mk("t", "table", "")}
		rg := &axTreeNode{node: mk("rg", "rowgroup", "")}
		rg.children = []*axTreeNode{{node: mk("r", "row", ""), children: []*axTreeNode{
			{node: mk("c", "cell", "仅此一格")},
		}}}
		root.children = []*axTreeNode{rg}
		assert.Equal(t, "| 仅此一格 |", renderAXTable(root))
	})
	t.Run("空表或无 row 返回空串", func(t *testing.T) {
		assert.Equal(t, "", renderAXTable(&axTreeNode{node: mk("t", "table", "")}))
	})
}

// ---- writeAXLine ----

func TestWriteAXLine(t *testing.T) {
	t.Run("按缩进深度输出两空格每层", func(t *testing.T) {
		var b strings.Builder
		writeAXLine(&b, 0, "正文", 0)
		writeAXLine(&b, 2, "子项", 0)
		assert.Equal(t, "正文\n    子项\n", b.String())
	})
	t.Run("附带 node_id", func(t *testing.T) {
		var b strings.Builder
		writeAXLine(&b, 1, "链接", proto.DOMBackendNodeID(7))
		assert.Equal(t, "  链接 (node:7)\n", b.String())
	})
	t.Run("空行不输出", func(t *testing.T) {
		var b strings.Builder
		writeAXLine(&b, 3, "", 1)
		assert.Equal(t, "", b.String())
	})
	t.Run("行内空白压缩且 node_id 追加", func(t *testing.T) {
		var b strings.Builder
		writeAXLine(&b, 0, " 多  行 ", proto.DOMBackendNodeID(2))
		assert.Equal(t, "多 行 (node:2)\n", b.String())
	})
}

// ---- isAXStructural / isInputRole ----

func TestStructuralAndInputRole(t *testing.T) {
	t.Run("结构节点组成独立行", func(t *testing.T) {
		for _, r := range []string{"heading", "headingWithoutLevel", "link", "button",
			"image", "img", "listitem", "textbox", "searchbox", "combobox", "input"} {
			assert.True(t, isAXStructural(r), "角色 %s 应为结构节点", r)
		}
		for _, r := range []string{"generic", "paragraph", "list", "text", "StaticText", ""} {
			assert.False(t, isAXStructural(r), "角色 %s 不应为结构节点", r)
		}
	})
	t.Run("输入角色", func(t *testing.T) {
		for _, r := range []string{"textbox", "searchbox", "combobox", "input"} {
			assert.True(t, isInputRole(r), "角色 %s 应为输入框", r)
		}
		assert.False(t, isInputRole("button"))
		assert.False(t, isInputRole("link"))
	})
}

// ---- headingDepth ----

func TestHeadingDepth(t *testing.T) {
	t.Run("空值回退到二级", func(t *testing.T) {
		assert.Equal(t, 2, headingDepth(""))
	})
	t.Run("数字字符串正常解析", func(t *testing.T) {
		assert.Equal(t, 1, headingDepth("1"))
		assert.Equal(t, 3, headingDepth("3"))
		assert.Equal(t, 6, headingDepth("6"))
	})
	t.Run("越界归一到二级", func(t *testing.T) {
		assert.Equal(t, 2, headingDepth("0"))
		assert.Equal(t, 2, headingDepth("7"))
		assert.Equal(t, 2, headingDepth("999"))
	})
	t.Run("非数字字符混入时提取数字", func(t *testing.T) {
		assert.Equal(t, 2, headingDepth("h2"))
	})
	t.Run("多位数正常解析", func(t *testing.T) {
		assert.Equal(t, 4, headingDepth("4"))
	})
}

// ---- renderAXContent / walkAX ----

// mkAX 构造 AX 节点：role/name/backendID 均来自字符串。
func mkAX(id, role, name string) *proto.AccessibilityAXNode {
	var nm *proto.AccessibilityAXValue
	if name != "" {
		nm = axValue(name)
	}
	return &proto.AccessibilityAXNode{
		NodeID:           proto.AccessibilityAXNodeID(id),
		BackendDOMNodeID: proto.DOMBackendNodeID(parseBackend(id)),
		Role:             axValue(role),
		Name:             nm,
	}
}

func parseBackend(s string) int64 {
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			continue
		}
		n = n*10 + int64(r-'0')
	}
	return n
}

func axNode(node *proto.AccessibilityAXNode, children ...*axTreeNode) *axTreeNode {
	return &axTreeNode{node: node, children: children}
}

func TestRenderAXContent_Dedup(t *testing.T) {
	t.Run("容器聚合 name 与文本叶子去重,只输出一次", func(t *testing.T) {
		// bio: generic 容器 name 是聚合后的整段,子节点被 CSS 拆成逐字符碎片。
		// 期望:仅输出聚合 name 一行,不再下钻输出碎片。
		bio := axNode(
			mkAX("10", "generic", "A student in the Department of Computational Mathematics"),
			axNode(mkAX("11", "StaticText", "A student")),
			axNode(mkAX("12", "StaticText", "in the")),
			axNode(mkAX("13", "StaticText", "Department")),
		)
		got := renderAXContent([]*axTreeNode{bio}, nil)
		assert.Equal(t, "A student in the Department of Computational Mathematics", got)
	})

	t.Run("结构节点聚合 name 后不再输出子文本", func(t *testing.T) {
		// link 的 name 已含完整文本,子 StaticText 不应再输出 → 只保留一行 [Follow](...)。
		link := axNode(
			mkAX("5", "link", "Follow"),
			axNode(mkAX("6", "StaticText", "Follow")),
		)
		got := renderAXContent([]*axTreeNode{link}, func(id proto.DOMBackendNodeID) string {
			return "https://github.com/follow"
		})
		assert.Equal(t, "[Follow](https://github.com/follow) (node:5)", got)
	})

	t.Run("容器内同时含文本与结构时,文本输一行后下钻结构", func(t *testing.T) {
		// paragraph: "收藏" + link "star" → 正文一行 + 链接行。
		p := axNode(
			mkAX("20", "paragraph", ""),
			axNode(mkAX("21", "StaticText", "收藏")),
			axNode(mkAX("22", "link", "star"),
				axNode(mkAX("23", "StaticText", "star"))),
		)
		got := renderAXContent([]*axTreeNode{p}, func(id proto.DOMBackendNodeID) string {
			return "https://github.com/star"
		})
		assert.Equal(t, "收藏\n[star](https://github.com/star) (node:22)", got)
	})
}

func TestRenderAXContent_Noise(t *testing.T) {
	t.Run("contentinfo(footer) 整棵跳过", func(t *testing.T) {
		root := axNode(
			mkAX("30", "contentinfo", "Copyright 2026"),
			axNode(mkAX("31", "StaticText", "Copyright 2026")),
		)
		assert.Equal(t, "", renderAXContent([]*axTreeNode{root}, nil))
	})
	t.Run("banner 地标整棵跳过", func(t *testing.T) {
		root := axNode(mkAX("40", "banner", "Site banner"))
		assert.Equal(t, "", renderAXContent([]*axTreeNode{root}, nil))
	})
	t.Run("skip-link 跳过", func(t *testing.T) {
		root := axNode(mkAX("50", "link", "Skip to content"))
		assert.Equal(t, "", renderAXContent([]*axTreeNode{root}, nil))
	})
	t.Run("普通 link 不受影响", func(t *testing.T) {
		root := axNode(mkAX("51", "link", "Documentation"))
		got := renderAXContent([]*axTreeNode{root}, func(id proto.DOMBackendNodeID) string {
			return "/docs"
		})
		assert.Equal(t, "[Documentation](/docs) (node:51)", got)
	})
}

func TestRenderAXContent_Structural(t *testing.T) {
	t.Run("heading 输出 # 层级", func(t *testing.T) {
		h := axNode(mkAX("60", "heading", "Profile"))
		h.node.Properties = []*proto.AccessibilityAXProperty{{
			Name: "level", Value: axValue("2"),
		}}
		got := renderAXContent([]*axTreeNode{h}, nil)
		assert.Equal(t, "## Profile", got)
	})
	t.Run("listitem 输出 - 前缀", func(t *testing.T) {
		li := axNode(mkAX("70", "listitem", "go-knocker"))
		got := renderAXContent([]*axTreeNode{li}, nil)
		assert.Equal(t, "- go-knocker", got)
	})
	t.Run("button 保留 node_id", func(t *testing.T) {
		b := axNode(mkAX("80", "button", "Follow"))
		got := renderAXContent([]*axTreeNode{b}, nil)
		assert.Equal(t, "[BUTTON] Follow (node:80)", got)
	})
	t.Run("input 保留 node_id", func(t *testing.T) {
		in := axNode(mkAX("90", "textbox", "Search"))
		got := renderAXContent([]*axTreeNode{in}, nil)
		assert.Equal(t, "[INPUT] Search (node:90)", got)
	})
}

func TestSubtreeHasStructural(t *testing.T) {
	t.Run("纯文本块无结构", func(t *testing.T) {
		leaf := axNode(mkAX("1", "StaticText", "x"))
		assert.False(t, subtreeHasStructural(leaf))
	})
	t.Run("含 link 即视为有结构", func(t *testing.T) {
		container := axNode(mkAX("2", "generic", ""),
			axNode(mkAX("3", "StaticText", "a")),
			axNode(mkAX("4", "link", "b")),
		)
		assert.True(t, subtreeHasStructural(container))
	})
	t.Run("heading 自身即结构", func(t *testing.T) {
		h := axNode(mkAX("5", "heading", "Title"))
		assert.True(t, subtreeHasStructural(h))
	})
}
