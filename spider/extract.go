package spider

import (
	"context"
	"fmt"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/strikethrough"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"github.com/go-rod/rod/lib/proto"
	"golang.org/x/net/html"
)

// GetHTML 返回当前页面的完整原始 HTML。
func (s *Spider) GetHTML(ctx context.Context) (string, error) {
	return s.page.Context(ctx).HTML()
}

// nonContentTags 是从结果中整体剔除的"无正文内容/技术噪音"标签。
// 它们对 LLM 理解正文语义几乎无帮助，却占用大量 token。
// 不做 nav/header/footer/aside 的静态剔除：这些常含正文级内容
// （如 header 里的 h1 标题），交给 prune 的内容取舍与链接密度判断更准。
var nonContentTags = map[string]struct{}{
	"script": {}, "style": {}, "link": {}, "meta": {}, "noscript": {},
	"svg": {}, "template": {}, "iframe": {}, "object": {}, "embed": {},
	"canvas": {}, "audio": {}, "video": {}, "picture": {}, "source": {},
	"head": {}, "title": {}, "base": {},
}

// attrWhitelist 在剪枝时仅保留的"语义属性"。
// 丢弃 class/id/style/事件属性等对语义理解无益的冗余，大幅压缩体积。
var attrWhitelist = map[string]struct{}{
	"href": {}, "src": {}, "srcset": {}, "alt": {}, "title": {},
	"type": {}, "value": {}, "placeholder": {}, "name": {}, "target": {},
	"rel": {}, "headers": {}, "scope": {}, "colspan": {}, "rowspan": {},
	"datetime": {}, "abbr": {},
}

// semanticTags 是"自身即承载语义"的标签：只要存在（哪怕没文本）也予以保留，
// 因为它们是链接、图片、表单控件、代码等对 LLM 有用的最小语义单元。
var semanticTags = map[string]struct{}{
	"a": {}, "p": {}, "h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {},
	"li": {}, "ul": {}, "ol": {}, "dl": {}, "dt": {}, "dd": {},
	"table": {}, "thead": {}, "tbody": {}, "tfoot": {}, "tr": {}, "td": {}, "th": {},
	"caption": {}, "colgroup": {}, "code": {}, "pre": {}, "blockquote": {},
	"img": {}, "figure": {}, "figcaption": {}, "picture": {},
	"strong": {}, "em": {}, "b": {}, "i": {}, "u": {}, "small": {}, "mark": {},
	"label": {}, "button": {}, "input": {}, "select": {}, "textarea": {}, "option": {},
	"time": {}, "address": {}, "progress": {}, "meter": {}, "details": {}, "summary": {},
	// 补齐的表单/内联语义标签。
	"form": {}, "fieldset": {}, "legend": {}, "abbr": {}, "acronym": {},
	"cite": {}, "q": {}, "samp": {}, "var": {}, "dfn": {}, "kbd": {},
	"ins": {}, "del": {}, "sub": {}, "sup": {}, "main": {}, "article": {},
}

// isHiddenElement 依据静态属性/内联样式判断元素是否隐藏。
// 说明：不交给浏览器计算（开销大），仅做启发式判断，命中率已足够高。
func isHiddenElement(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	for _, a := range n.Attr {
		switch a.Key {
		case "hidden":
			return true
		case "aria-hidden":
			if a.Val == "true" {
				return true
			}
		case "style":
			norm := strings.ReplaceAll(strings.ToLower(a.Val), " ", "")
			if strings.Contains(norm, "display:none") || strings.Contains(norm, "visibility:hidden") {
				return true
			}
		}
	}
	return false
}

// isSkippable 判定一个节点是否应整棵从结果中剔除。
func isSkippable(n *html.Node) bool {
	if n.Type == html.ElementNode {
		if _, drop := nonContentTags[n.Data]; drop {
			return true
		}
		return isHiddenElement(n)
	}
	return false
}

// subtreeStats 统计 n 子树的可见文本长度与其中 <a> 链接锚文本长度。
// 返回 (文本长度, 链接长度)。跳过脚本/样式等已剔除的噪音标签。
func subtreeStats(n *html.Node) (textLen, linkLen int) {
	switch n.Type {
	case html.TextNode:
		t := len(strings.TrimSpace(n.Data))
		return t, 0
	case html.ElementNode:
		if _, drop := nonContentTags[n.Data]; drop {
			return 0, 0
		}
		isLink := n.Data == "a"
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			tt, ll := subtreeStats(c)
			textLen += tt
			if isLink {
				linkLen += tt
			} else {
				linkLen += ll
			}
		}
		return textLen, linkLen
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			tt, ll := subtreeStats(c)
			textLen += tt
			linkLen += ll
		}
		return textLen, linkLen
	}
}

// isLinkWall 依据链接密度判断一段修剪后的块是否属于"链接墙"（导航/标签云/目录）。
// 参考 justext 的链接密度启发式：当块内可见文本几乎全部来自链接锚文本、且体量
// 足够时，说明它更像一组导航链接而非正文，对 LLM 语义价值低，值得整体剔除。
// 阈值：至少 linkWallMinLen(40) 字符的文本，且链接文本占比 >= linkWallLinkRatio(0.7)。
const (
	linkWallMinLen    = 40
	linkWallLinkRatio = 0.7
)

func isLinkWall(n *html.Node) bool {
	textLen, linkLen := subtreeStats(n)
	if textLen < linkWallMinLen {
		return false
	}
	return float64(linkLen)/float64(textLen) >= linkWallLinkRatio
}

// collapseWS 将连续的空白（含换行、全角空格）压成单个空格，用于文本归一。
func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// prune 就地修剪 n 的子树，并返回该节点是否携带"有效内容"。
// 策略三层，自底向上：
//   - 文本节点：空白归一；非空文本才算有效内容。
//   - 元素节点：先剔除 Chrome/隐藏节点（黑名单见 nonContentTags），过滤出语义属性
//     （attrWhitelist），再递归修剪子节点；白名单标签（semanticTags，a/p/button/…
//     自身即语义）无论有无子内容都保留；其余外壳（div/span/section…）仅当子树含
//     有效内容且非链接墙（isLinkWall）才保留。
//   - 其余（document 等）：仅递归清理子节点并视为有效。
func prune(n *html.Node) bool {
	switch n.Type {
	case html.TextNode:
		n.Data = collapseWS(n.Data)
		return n.Data != ""
	case html.ElementNode:
		tag := n.Data
		if isSkippable(n) {
			return false
		}
		// 就地过滤属性：只回写 whitelist 内的属性（kept 索引 <= 当前索引，安全）。
		kept := n.Attr[:0]
		for _, a := range n.Attr {
			if _, ok := attrWhitelist[a.Key]; ok {
				kept = append(kept, a)
			}
		}
		n.Attr = kept

		var content bool
		for c := n.FirstChild; c != nil; {
			nxt := c.NextSibling
			if prune(c) {
				content = true
			} else {
				n.RemoveChild(c)
			}
			c = nxt
		}
		if _, sem := semanticTags[tag]; sem {
			return true
		}
		// 外壳若修剪后几乎全是链接（导航/标签云/目录墙），对正文理解价值低，整体剔除。
		if content && isLinkWall(n) {
			return false
		}
		return content
	default:
		// document / ruby / raw 等节点：仅递归清理子节点，不做自身取舍。
		for c := n.FirstChild; c != nil; {
			nxt := c.NextSibling
			if !prune(c) {
				n.RemoveChild(c)
			}
			c = nxt
		}
		return true
	}
}

// GetStructured 返回把当前页面转成**可读 Markdown** 的结构化文本：
// 先对整页做内容级剪枝（去 Chrome/空壳，见 prune），再用 html-to-markdown(v2) 转换为
// Markdown（表格/任务列表/删除线用 GFM 扩展）。相比原始 HTML 更省 token、对 LLM 更友好，
// 同时保留标题、列表、表格、链接等结构。
func (s *Spider) GetStructured(ctx context.Context) (string, error) {
	p := s.page.Context(ctx)
	raw, err := p.HTML()
	if err != nil {
		return "", err
	}
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return "", err
	}
	prune(doc)

	clean := &strings.Builder{}
	if err := html.Render(clean, doc); err != nil {
		return "", err
	}

	// domain 用于把相对链接（/path、./x）转成绝对链接。
	domain := ""
	if info, err := p.Info(); err == nil {
		domain = info.URL
	}

	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(),
			strikethrough.NewStrikethroughPlugin(),
		),
	)

	mdText, err := conv.ConvertString(clean.String(), converter.WithDomain(domain))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(mdText), nil
}

// roleName 从 AX 角色节点提取角色名字符串。
func roleName(v *proto.AccessibilityAXValue) string {
	if v == nil {
		return ""
	}
	return v.Value.String()
}

// axString 从 AX 属性节点提取文本值（Name/Value/Description 通用）。
func axString(v *proto.AccessibilityAXValue) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(v.Value.String())
}

// axPropertyValue 在节点属性列表中按名查找值（如 heading 的 level）。
func axPropertyValue(n *proto.AccessibilityAXNode, want string) string {
	for _, p := range n.Properties {
		if p == nil || string(p.Name) != want {
			continue
		}
		return p.Value.Value.String()
	}
	return ""
}

// axTreeNode AX 树节点的树形包装，用于重建父子宫级与表格结构。
type axTreeNode struct {
	node     *proto.AccessibilityAXNode
	children []*axTreeNode
}

// buildAXTree 用 ParentID 把扁平 AX 节点列表重建为森林（多根）。
// ignored 节点保留在树上（用于定位层级），但在输出时会跳过。
//
// 注意：AX 节点的 NodeID 只保证上下文内的稳定与唯一，不具备顺序语义，
// 绝不能按它排序。真正的文档顺序是浏览器 GetFullAXTree 返回时的树遍历
// 顺序（父在前、子在后）。Go 的 map 迭代顺序是随机的，若直接按 map 遍历
// 构建，父子顺序会被打乱且每次运行不一致；因此这里按输入顺序收集节点，
// 再在构建时按它填充父子关系，从而稳定保留浏览器给出的树结构。
func buildAXTree(nodes []*proto.AccessibilityAXNode) []*axTreeNode {
	byID := make(map[proto.AccessibilityAXNodeID]*axTreeNode, len(nodes))
	ordered := make([]*axTreeNode, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		tn := &axTreeNode{node: n}
		byID[n.NodeID] = tn
		ordered = append(ordered, tn)
	}
	var roots []*axTreeNode
	for _, tn := range ordered {
		if tn.node.ParentID == "" {
			roots = append(roots, tn)
			continue
		}
		if parent, ok := byID[tn.node.ParentID]; ok {
			parent.children = append(parent.children, tn)
		} else {
			// 父节点缺失（未返回），当作根，避免节点丢失。
			roots = append(roots, tn)
		}
	}
	return roots
}

// renderAXTable 从 table/grid 节点重建 Markdown 表格。
// 收集 row 及每行的 cell 单元格文本，第一行若为表头（含 columnheader/rowheader）则加分隔线。
func renderAXTable(root *axTreeNode) string {
	var rows []*axTreeNode
	var collect func(n *axTreeNode)
	collect = func(n *axTreeNode) {
		for _, c := range n.children {
			if r := roleName(c.node.Role); r == "row" {
				rows = append(rows, c)
			} else {
				collect(c) // 穿透 rowgroup 等容器
			}
		}
	}
	collect(root)

	var out strings.Builder
	first := true
	for _, row := range rows {
		var cells []string
		isHeader := false
		for _, c := range row.children {
			r := roleName(c.node.Role)
			switch r {
			case "cell", "columnheader", "rowheader":
				txt := axString(c.node.Name)
				if txt == "" {
					txt = " "
				}
				cells = append(cells, txt)
				if r == "columnheader" || r == "rowheader" {
					isHeader = true
				}
			}
		}
		if len(cells) == 0 {
			continue
		}
		if first && isHeader {
			out.WriteString("| ")
			out.WriteString(strings.Join(cells, " | "))
			out.WriteString(" |\n")
			out.WriteString("| ")
			out.WriteString(strings.TrimSuffix(strings.Repeat("--- | ", len(cells)), " | "))
			out.WriteString(" |\n")
			first = false
			continue
		}
		out.WriteString("| ")
		out.WriteString(strings.Join(cells, " | "))
		out.WriteString(" |\n")
	}
	return strings.TrimRight(out.String(), "\n")
}

// renderAXContent 把 AX 森林渲染为紧凑 Markdown。
//
// 去重策略（解决聚合 name 与文本叶子重复输出的问题）：
//   - 结构节点（heading/link/button/input/image/listitem/table）作为独立语义行输出，
//     其聚合 name 已完整表达文本；若其子树再无嵌套结构，则不再下钻子节点；
//   - 其他容器/文本节点：若其子树无任何结构后代（纯文本块），直接输出聚合 name 一行，
//     天然合并被 CSS 打断的文本碎片，且不重复输出子叶子；
//   - 仅当子树含有结构后代（如 listitem 内含链接）时才递归下钻。
//
// 噪音过滤：contentinfo(footer)/banner 地标与 skip-link 整棵跳过，
// navigation 等导航墙按链接密度启发式剔除。
func renderAXContent(forest []*axTreeNode, resolve func(id proto.DOMBackendNodeID) string) string {
	var b strings.Builder
	for _, root := range forest {
		walkAX(root, 0, &b, resolve)
	}
	return strings.TrimRight(b.String(), "\n")
}

// walkAX 深度优先遍历单个 AX 子树并写入 b。
func walkAX(tn *axTreeNode, depth int, b *strings.Builder, resolve func(id proto.DOMBackendNodeID) string) {
	if tn == nil || tn.node == nil {
		return
	}
	n := tn.node
	role := roleName(n.Role)

	// 噪音节点（footer/banner/skip-link）：整棵跳过，不输出也不下钻。
	if isNoiseAX(n) {
		return
	}
	// 隐藏/装饰节点不输出，但保留其在树上的位置，子节点继续前进。
	if n.Ignored {
		for _, c := range tn.children {
			walkAX(c, depth, b, resolve)
		}
		return
	}

	name := axString(n.Name)

	// 表格：整体还原为 Markdown 表格，子节点不再单独遍历。
	if role == "table" || role == "grid" {
		if tbl := renderAXTable(tn); tbl != "" {
			for _, line := range strings.Split(tbl, "\n") {
				writeAXLine(b, depth, line, 0)
			}
		}
		return
	}

	// 结构节点：作为独立语义行输出。聚合 name 已完整表达其文本，
	// 仅当子树中还有别的结构节点（如 listitem 内的链接）时才下钻。
	if isAXStructural(role) {
		writeAXStructural(n, depth, role, name, b, resolve)
		if hasStructuralDescendant(tn) {
			for _, c := range tn.children {
				walkAX(c, depth+1, b, resolve)
			}
		}
		return
	}

	// 非结构容器/文本：整棵无结构后代时，聚合 name 即全部文本，输出一次并停止下钻。
	if hasStructuralDescendant(tn) {
		for _, c := range tn.children {
			walkAX(c, depth, b, resolve)
		}
		return
	}
	if name != "" {
		writeAXLine(b, depth, name, 0)
	}
}

// isNoiseAX 判定节点是否属于应整棵剔除的噪音：footer/banner 地标与 skip-link。
func isNoiseAX(n *proto.AccessibilityAXNode) bool {
	switch roleName(n.Role) {
	case "contentinfo", "banner":
		return true
	}
	// skip-link：跳到主内容的锚点链接，对内容提取无价值。
	if roleName(n.Role) == "link" {
		if href := strings.ToLower(axString(n.Name)); strings.Contains(href, "skip to") {
			return true
		}
	}
	return false
}

// isAXStructural 判断该 role 是否构成独立语义行（需输出聚合 name 并带 node_id）。
func isAXStructural(role string) bool {
	switch role {
	case "heading", "headingWithoutLevel", "link", "button", "image", "img", "listitem",
		"textbox", "searchbox", "combobox", "input":
		return true
	}
	return false
}

// writeAXStructural 按 role 输出结构节点的一行 Markdown。
func writeAXStructural(n *proto.AccessibilityAXNode, depth int, role, name string, b *strings.Builder, resolve func(id proto.DOMBackendNodeID) string) {
	switch {
	case role == "heading" || role == "headingWithoutLevel":
		level := headingDepth(axPropertyValue(n, "level"))
		writeAXLine(b, depth, strings.Repeat("#", level)+" "+name, 0)

	case role == "link":
		href := ""
		if resolve != nil {
			href = resolve(n.BackendDOMNodeID)
		}
		txt := name
		if txt == "" {
			txt = href
		}
		// 链接数量通常远多于真正可交互控件，不叠加 [CLICK] 标记以控制输出体积，但仍保留 node_id 供按节点点击。
		writeAXLine(b, depth, fmt.Sprintf("[%s](%s)", txt, href), n.BackendDOMNodeID)

	case role == "button":
		// 可点击动作标识，与 click_by_node_id_tool 对应。
		label := name
		if v := axString(n.Value); v != "" {
			label += " (value:" + v + ")"
		}
		writeAXLine(b, depth, "[BUTTON] "+label, n.BackendDOMNodeID)

	case isInputRole(role):
		// 可输入动作标识，与 input_by_node_id_tool 对应。
		label := name
		if v := axString(n.Value); v != "" {
			label += " (value:" + v + ")"
		}
		writeAXLine(b, depth, "[INPUT] "+label, n.BackendDOMNodeID)

	case role == "image" || role == "img":
		writeAXLine(b, depth, "![image: "+name+"]", 0)

	case role == "listitem":
		writeAXLine(b, depth, "- "+name, 0)
	}
}

// hasStructuralDescendant 判断 tn 的子树中是否存在"结构节点（含表格/噪音）"。
// 注意仅检查后代、不含当前节点自身：用于决定一个容器如何下钻——
// 对结构节点判断其子是否还有需独立输出的结构，对纯容器判断其下是否有结构。
func hasStructuralDescendant(tn *axTreeNode) bool {
	if tn == nil {
		return false
	}
	for _, c := range tn.children {
		if subtreeHasStructural(c) {
			return true
		}
	}
	return false
}

// subtreeHasStructural 判断以 tn 为根的子树（含自身）是否存在结构节点或噪音。
func subtreeHasStructural(tn *axTreeNode) bool {
	if tn == nil || tn.node == nil {
		return false
	}
	if isAXStructural(roleName(tn.node.Role)) || isNoiseAX(tn.node) {
		return true
	}
	for _, c := range tn.children {
		if subtreeHasStructural(c) {
			return true
		}
	}
	return false
}

// GetAXContent 基于浏览器可访问性树（Accessibility/AX）提取页面内容，输出紧凑 Markdown。
//
// AX 树是浏览器渲染后的语义投影，本身已剔除 script、隐藏
// （display:none / visibility:hidden / aria-hidden）与纯装饰节点（node.Ignored=true）。
// 渲染逻辑（含去重/碎片合并/噪音过滤）由 renderAXContent 承担，本函数仅负责取回 AX 树。
//   - 保留父子宫级结构，正文/列表按嵌套深度缩进；
//   - 按 role 输出 Markdown：heading→# 层级、list→- 、link→[txt](href)、button/input 保留 node_id；
//   - table/grid 还原为 Markdown 表格（表头/分隔线）；
//   - 保留 node_id（即 BackendDOMNodeID，与 get_interactives 的 node_id 同源），
//     使 LLM 拿到内容后可直接用 click_by_node_id / input_by_node_id 交互。
func (s *Spider) GetAXContent(ctx context.Context) (string, error) {
	p := s.page.Context(ctx)
	err := proto.AccessibilityEnable{}.Call(p)
	if err != nil {
		return "", err
	}
	result, err := proto.AccessibilityGetFullAXTree{}.Call(p)
	if err != nil {
		return "", err
	}

	forest := buildAXTree(result.Nodes)
	return renderAXContent(forest, func(id proto.DOMBackendNodeID) string {
		return s.resolveHref(p, id)
	}), nil
}

// writeAXLine 按缩进深度写入一行内容（缩进 2 空格/层），并附加 node_id 标记。
func writeAXLine(b *strings.Builder, depth int, line string, nodeID proto.DOMBackendNodeID) {
	if line == "" {
		return
	}
	line = strings.Join(strings.Fields(line), " ")
	if nodeID != 0 {
		line += fmt.Sprintf(" (node:%d)", nodeID)
	}
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString(line)
	b.WriteString("\n")
}

// isInputRole 判断该 role 是否为文本输入类控件。
func isInputRole(role string) bool {
	switch role {
	case "textbox", "searchbox", "combobox", "input":
		return true
	}
	return false
}

// headingDepth 解析 heading 层级字符串为缩进级数（越界归一到安全区间）。
func headingDepth(level string) int {
	if level == "" {
		return 2
	}
	var depth int
	for _, r := range level {
		if r < '0' || r > '9' {
			continue
		}
		depth = depth*10 + int(r-'0')
	}
	if depth < 1 || depth > 6 {
		return 2
	}
	return depth
}
