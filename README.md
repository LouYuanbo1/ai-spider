# ai-spider

一个基于 **Go** 编写、由 **LLM（大语言模型）驱动**的智能网页数据抓取工具。它通过真实浏览器（Chrome）自动化完成用户用自然语言描述的任务——包括页面导航、元素交互、滚动、执行 JavaScript 以及智能正文提取。

区别于传统的爬虫，`ai-spider` 将「理解用户意图 → 自主选择工具 → 执行浏览器操作 → 提取结果」的整个流程交由大模型智能体完成，适合面对 JS 动态渲染、需要模拟真人交互的网页抓取场景。

## 特性

- 🧠 **LLM 智能体驱动**：基于 eino ADK 构建，模型自主决策要调用哪个浏览器工具、如何完成目标。
- 🌐 **真实浏览器环境**：基于 go-rod 启动 Chrome，配合 stealth 反检测，`DisableBlinkFeatures=AutomationControlled` 隐藏自动化痕迹。
- 🛠️ **11 个精细化工具**：导航、可交互元素发现、CSS/XPath/node_id 三种点击与输入、滚动、JS 执行、智能正文提取。
- 🎯 **node_id 精准定位**：通过 `get_interactives_tool` 返回的后端 DOM 节点 ID 精准点击/输入，规避同名元素与相似选择器歧义。
- 📄 **智能正文提取**：基于可读性算法（go-readability）自动剥离广告/导航噪音，也可选择返回完整 HTML。
- 💬 **终端多轮对话**：交互式命令行输入指令，逐轮调用智能体并流式输出思考与结果。
- 🔍 **流式补齐与容错**：完整处理流式工具调用参数累积；工具失败不再中断，而是回传模型让其反思重试。

## 项目结构

```
ai-spider/
├── main.go                 # 程序入口：加载配置，初始化模型/浏览器/智能体，终端多轮对话
├── go.mod / go.sum         # Go 模块与依赖
├── .gitignore              # 忽略规则（含敏感配置与浏览器数据目录）
├── LICENSE                 # MIT 许可证
├── config/
│   ├── config.go           # 配置结构体定义（RodConfig / ModelConfig）
│   ├── config.yaml         # 本地运行配置（含 API Key，已被 .gitignore 忽略，勿提交）
│   ├── config_example.yaml # 配置模板（占位符，可安全提交）
│   └── load.go             # 读取并解析 YAML 配置
├── agent/
│   ├── agent.go            # 智能体构建：角色指令、11 个工具的注册表与流式输出处理
│   └── tools.go            # 每个浏览器工具的封装（含调试日志包装 debugTool）
├── spider/
│   ├── spider.go           # 浏览器封装：导航、点击、输入、滚动、JS、正文提取
│   ├── action.go           # 密封接口 Action 与各动作参数结构体
│   ├── option.go           # Launcher 选项函数（链式配置 Chrome 启动参数）
│   ├── spider_test.go      # 正文截断与 Action 序列化等单元测试
│   └── option_test.go      # Launcher 选项函数单元测试
└── user_data_dir/          # 浏览器用户数据（Cookies/登录态等，已被 .gitignore 忽略）
```

## 模块详解

本项目的代码按职责划分为四个模块，自顶向下为 `main` → `config` → `agent` → `spider`。

### main（程序入口）

[main.go](main.go) 负责整条链路的装配与交互循环，依次完成四件事：

1. 调用 `config.Load("config/config.yaml")` 读取配置，失败即终止。
2. 基于配置的 `api_key` 通过 [eino-ext](https://github.com/cloudwego/eino-ext) 创建 DeepSeek 聊天模型（`model: deepseek-flash`）。
3. 调用 `spider.InitSpider` 启动真实 Chrome 浏览器。
4. 调用 `agent.NewDefaultSpiderAgent` 组装智能体，随后进入**终端多轮对话循环**：每轮读取用户输入，通过 `modelAgent.OutputMessage` 流式执行并打印结果，输入 `exit` / `quit` / `q` 或 `Ctrl+D` / `Ctrl+Z` 退出。

> 说明：当前会话是**无记忆**的，每一轮指令都会作为新请求独立执行。

### config（配置加载）

用于集中管理「浏览器启动参数」与「模型密钥」，避免散落在各模块：

- [config.go](config/config.go)：定义 `RodConfig`（浏览器）、`ModelConfig`（模型）的结构体，每个字段以 `yaml` 标签映射 YAML 键。
- [load.go](config/load.go)：`Load(path)` 读取文件并用 `gopkg.in/yaml.v3` 反序列化，返回 `*Config`。
- `config.yaml` / `config_example.yaml`：真实配置与模板（前者含密钥且已被忽略）。

### spider（浏览器封装层）

封装 go-rod 对 Chrome 的全部底层操作，向上层 agent 提供稳定的动作原语。核心类型为 `Spider`，持有 `browser` / `page` / `logger`：

- [spider.go](spider/spider.go)：导航（`Navigate`）、三种点击（`DoClick` / `DoClickX` / `DoClickNode`）、三种输入（`DoInput` / `DoInputX` / `DoInputNode`）、滚动（`DoScroll`）、JS 执行（`DoJavaScript`）、智能正文提取（`GetArticle`）/ 完整 HTML（`GetHTML`），以及 `ActionSummary` 轻量动作回执与 `PageFingerprint` 页面变化判断。JS 执行自带一次重试，应对 SPA 偶发失败。
- [action.go](spider/action.go)：通过 `isAction()` 私有方法实现 Go 的**密封接口**技巧，定义 `Navigate` / `ClickBy*` / `InputBy*` / `Scroll` / `JavaScript` / `HTML` 等动作参数结构体。
- [option.go](spider/option.go)：`LauncherOption` 函数式选项，将配置映射为 Chrome 启动旗标（如 `WithUserDataDir`、`WithHeadless`、`WithRemoteDebuggingPort`），供 `InitSpider` 链式拼接。

### agent（智能体层）

基于 eino ADK 构建 LLM 智能体，是「意图 → 操作」的决策中枢：

- [agent.go](agent/agent.go)：`NewDefaultSpiderAgent` 内嵌一大段**系统指令**（角色定义、11 个工具的使用准则、工作流、行为边界、输出格式与错误处理策略），并注册全部工具；同时实现流式输出处理 `stream` / `streamMessage`，负责累积跨 chunk 到达的工具调用参数，并处理模型思考过程。
- [tools.go](agent/tools.go)：逐一定义每个工具的实现与 `utils.InferTool` 装配，并通过 `debugTool` 统一包装——在终端打印参数与返回值，且**将工具错误转为普通文本回传模型**，让模型反思后重试而不是中断整次运行。

## 第三方依赖提醒

⚠️ **本项目依赖字节跳动 CloudWeGo 团队开源的 Eino 框架**

`github.com/cloudwego/eino`（含其 ADK，用于构建本项目的 LLM 智能体）以及 `github.com/cloudwego/eino-ext/components/model/deepseek`（DeepSeek 模型接入）均为**字节跳动**开源的第三方项目。使用前请充分了解并权衡以下几点：

- **供应链与合规**：引入字节系开源依赖意味着将其纳入你的软件供应链。请在正式商用前评估其开源许可证条款、维护状态及你所在团队的合规要求。
- **数据流向**：本项目将用户指令与网页抓取内容发送给 DeepSeek 模型（深度求索）的官方 API 以生成决策；若任务输入涉及敏感/机密信息，请评估该第三方调用是否符合你的数据安全规范。
- **版本稳定性**：eino / eino-ext 属于较新的 AI 编排框架，接口演进较快，升级版本可能引入破坏性变更（本项目 `go.mod` 固定了具体版本以规避此类风险）。
- **无担保免责**：Eino 框架及本项目均为“按现状”提供，作者不对其稳定性、安全性或特定用途适用性作任何担保。

> 本项目的智能体能力完全建立在 Eino ADK 之上，若使用其他ADK，可能需要改写 `agent` 层以迁移到其他 AI 编排框架。

## 环境要求

- Go 1.27 或更高版本
- 已安装的 Chrome 浏览器（默认路径 `C:/Program Files/Google/Chrome/Application/chrome.exe`，可通过 `rod.bin` 配置修改）
- 一个可用的 LLM API Key（本项目使用 DeepSeek flash 模型）

## 快速开始

### 1. 安装依赖

```bash
cd ai-spider
go mod download
```

### 2. 配置

将配置模板复制为本地配置文件并填入真实值：

```bash
# Windows
copy config\config_example.yaml config\config.yaml
```

编辑 `config/config.yaml`，至少需要修改：

| 配置项 | 说明 |
|--------|------|
| `model.api_key` | 你的 LLM API Key（必填） |
| `rod.bin` | Chrome 可执行文件路径（按需） |

> ⚠️ **安全提醒**：`config/config.yaml` 包含 API Key 与浏览器登录态，**不要提交到版本库**。项目已在 `.gitignore` 中忽略它（详见下方「安全说明」）。

### 3. 运行

```bash
go run .
```

程序会在终端提示输入指令，输入 `exit` / `quit` / `q` 或按下 `Ctrl+D` / `Ctrl+Z` 退出。

### 4. 交互示例

```
>>> 请输入指令 (输入 exit/quit/q 退出): 打开 https://example.com 并抓取正文
```

智能体会自动完成：导航 → 等待加载 → 提取正文 → 按格式输出（页面信息、抓取内容、操作摘要等）。

## 配置说明

`config/config.yaml` 采用 YAML 格式，分为 `rod`（浏览器）与 `model`（模型）两部分。

### rod（浏览器启动配置）

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `user_data_dir` | `user_data_dir` | 浏览器用户数据目录（其下的 instance_0 为实际实例目录） |
| `user_mode` | `false` | 是否使用本机用户模式浏览器 |
| `headless` | `false` | 是否无头模式运行 |
| `disable_blink_features` | `AutomationControlled` | 禁用的 Blink 特性，用于反自动化检测 |
| `incognito` | `false` | 是否无痕模式 |
| `disable_dev_shm_usage` | `true` | 禁用共享内存（适用于受限容器） |
| `no_sandbox` | `true` | 禁用沙箱 |
| `default_page_width` / `default_page_height` | `300` / `400` | 页面尺寸（当前未直接使用，可扩展） |
| `user_agent` | Edge UA | 自定义 User-Agent |
| `leakless` | `true` | 启用 go-rod leakless 进程保护 |
| `bin` | Chrome 默认路径 | Chrome 可执行文件路径 |
| `disable_background_networking` | `false` | 禁用后台网络 |
| `disable_background_timer_throttling` | `true` | 禁用后台标签页定时器节流 |
| `disable_backgrounding_occluded_windows` | `true` | 禁用后台窗口遮挡处理 |
| `disable_renderer_backgrounding` | `true` | 禁用渲染器后台化 |
| `basic_remote_debugging_port` | `9222` | 远程调试端口 |
| `trace` | `false` | 是否开启 CDP 通信追踪 |

### model（模型配置）

| 配置项 | 说明 |
|--------|------|
| `api_key` | 调用 LLM 的 API Key |

模型在代码中固定为 `deepseek-flash`。

## 智能体工具

智能体（`crawlerAgent`）暴露以下 11 个浏览器自动化工具，模型会按需自主调用：

| 工具名 | 用途 | 推荐时机 |
|--------|------|----------|
| `navigate_tool` | 导航到 URL 并等待页面加载 | 任务第一步，查看/抓取目标页 |
| `get_interactives_tool` | 获取页面所有可交互元素（链接/按钮/输入框）及 `node_id` | 不确定选择器时先获取 `node_id` |
| `click_by_node_id_tool` | 通过 `node_id` 精准点击 | 有新鲜 `node_id` 时最优先 |
| `click_by_css_tool` | 通过 CSS 选择器点击 | 明确特定选择器且无歧义 |
| `click_by_xpath_tool` | 通过 XPath 点击 | 明确特定 XPath 且无歧义 |
| `input_by_node_id_tool` | 通过 `node_id` 向输入框键入文本 | 有 `node_id` 时最优先 |
| `input_by_css_tool` | 通过 CSS 选择器键入文本 | 明确选择器且无歧义 |
| `input_by_xpath_tool` | 通过 XPath 键入文本 | 明确 XPath 且无歧义 |
| `scroll_tool` | 按像素滚动页面 | 触发懒加载/浏览长页 |
| `javascript_tool` | 在页面执行 JS 并取回返回值 | 仅在专用工具无法完成时使用 |
| `html_tool` | 获取完整 HTML 或智能提取正文 | 抓取的最终步骤 |

**关键设计**：
- `node_id` 在整页导航或大规模 DOM 重渲染后会失效，此时需重新调用 `get_interactives_tool` 刷新。
- 点击/输入优先用 `node_id`，其次 CSS/XPath；`html_tool` 默认用可读性算法提取正文，传 `enable_full_info=true` 返回完整 HTML。
- 向已有内容的输入框键入前可设 `clear=true` 先清空。

## 测试

```bash
go test ./...
```

当前覆盖内容：
- `TestTruncateText`：正文截断的边界与中文（rune）处理。
- `TestActionInterface` / `TestActionJSONRoundTrip` / `TestActionZeroValue`：`Action` 密封接口与各类动作参数的 JSON 编解码保真。
- `Test*`（option_test）：各 `With*` Launcher 选项函数的旗标设置行为。

## 安全说明

本项目对敏感信息做了以下保护，请勿移除相关 `.gitignore` 规则：

- **API Key**：真实密钥仅存放在 `config/config.yaml`（`model.api_key`），该文件已被 `.gitignore` 忽略；仓库中的 `config_example.yaml` 使用占位符 `your_api_key`，可安全提交。
- **浏览器登录态**：`user_data_dir/` 目录包含 Cookies、登录数据等浏览器隐私，已被 `.gitignore` 忽略，不会随源码提交。
- **环境变量**：`.env` 已在忽略规则中。
- ⚠️ **注意**：切勿将真实 `config.yaml` 重命名为可被跟踪的文件名，或将 API Key 硬编码进源码后提交。

## 许可证

本仓库基于 [MIT License](./LICENSE) 发布，Copyright (c) 2026 louyuanbo1。