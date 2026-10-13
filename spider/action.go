package spider

//Go 语言实现密封类技巧
type Action interface {
	isAction()
}

type Navigate struct {
	URL string `json:"url" jsonschema:"description=用于指定要导航的目标 URL"`
}

func (n *Navigate) isAction() {
}

type ClickByCSS struct {
	Selector string `json:"selector" jsonschema:"description=用于指定要点击的元素CSS选择器"`
}

func (c *ClickByCSS) isAction() {
}

type ClickByXPath struct {
	Selector string `json:"selector" jsonschema:"description=用于指定要点击的元素XPath选择器"`
}

func (c *ClickByXPath) isAction() {
}

type ClickByNodeID struct {
	NodeID int64 `json:"node_id" jsonschema:"description=优先使用：可点击元素的节点ID(来自 get_interactives_tool)，比选择器更准"`
}

func (c *ClickByNodeID) isAction() {
}

type InputByCSS struct {
	Selector string `json:"selector" jsonschema:"description=用于指定要输入文本的输入框CSS选择器"`
	Text     string `json:"text" jsonschema:"description=要输入到输入框中的文本内容"`
	Clear    bool   `json:"clear" jsonschema:"description=是否先清空输入框原有内容再输入，默认为false"`
}

func (c *InputByCSS) isAction() {
}

type InputByXPath struct {
	Selector string `json:"selector" jsonschema:"description=用于指定要输入文本的输入框XPath选择器"`
	Text     string `json:"text" jsonschema:"description=要输入到输入框中的文本内容"`
	Clear    bool   `json:"clear" jsonschema:"description=是否先清空输入框原有内容再输入，默认为false"`
}

func (c *InputByXPath) isAction() {
}

type InputByNodeID struct {
	NodeID int64  `json:"node_id" jsonschema:"description=优先使用：输入框元素的节点ID(来自 get_interactives_tool)，比选择器更准"`
	Text   string `json:"text" jsonschema:"description=要输入到输入框中的文本内容"`
	Clear  bool   `json:"clear" jsonschema:"description=是否先清空输入框原有内容再输入，默认为false"`
}

func (c *InputByNodeID) isAction() {
}

type Scroll struct {
	ScrollY int `json:"scroll_y" jsonschema:"description=用于指定要滚动的像素数"`
}

func (s *Scroll) isAction() {
}

type JavaScript struct {
	JS   string `json:"js" jsonschema:"description=用于指定要执行的JavaScript代码"`
	Args []any  `json:"args" jsonschema:"description=用于指定JavaScript代码的参数"`
}

func (j *JavaScript) isAction() {
}

// HTMLMode 定义 html_tool 的取数模式。
type HTMLMode string

const (
	HTMLModeAccessibility HTMLMode = "accessibility" // 默认，首选。基于浏览器可访问性树(AX)输出紧凑Markdown：天然剔除隐藏/装饰/脚本节点，还原标题/列表/表格与父子宫级，且每条可交互元素附 node_id 可直接点击/输入，最省 token 且图文并貌兼备
	HTMLModeStructured    HTMLMode = "structured"    // 把已剪枝页面转为可读 Markdown（含标题/列表/表格/链接），保留富文本内联格式（粗体/代码等），适合内容丰富的阅读场景
	HTMLModeFull          HTMLMode = "full"          // 完整原始HTML（体积大，token 昂贵，当需要精确 DOM 结构/CSS 属性/选择器定位或原始源码时使用）
)

type HTML struct {
	Mode HTMLMode `json:"mode,omitempty" jsonschema:"description=取数模式,enum=accessibility,enum=structured,enum=full,default=accessibility" jsonschema_description:"取数模式。默认accessibility(首选)：基于浏览器可访问性树AX输出紧凑Markdown，天然剔除隐藏装饰及脚本节点并还原标题列表表格与层级，每条可交互元素附node_id可点击或输入，最省token且图文兼备。structured=已剪枝页面转为可读Markdown，保留粗体行内代码等富文本内联格式，适合内容丰富的阅读。full=完整原始HTML，保留精确DOM结构与CSS属性，仅在需要原始源码代码数据表格或精确选择器定位时使用，token昂贵"` // accessibility(默认)|structured|full
}

func (h *HTML) isAction() {
}

// DefaultHTMLMode 返回实际生效的取数模式：未指定（空串）时回退到默认档 accessibility。
func DefaultHTMLMode(mode HTMLMode) HTMLMode {
	if mode == "" {
		return HTMLModeAccessibility
	}
	return mode
}

type GetInteractive struct {
}

func (g *GetInteractive) isAction() {
}
