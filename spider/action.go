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

type HTML struct {
	EnableFullInfo bool `json:"enable_full_info" jsonschema:"description=是否返回完整信息，false=只提取关键信息"` // true=返回完整信息，false=只提取关键信息
}

type GetInteractive struct {
}

func (g *GetInteractive) isAction() {
}
