# ai-spider

一个基于 **Go** 编写、由 **LLM（大语言模型）驱动**的智能网页数据抓取工具。它通过真实浏览器（Chrome）自动化完成用户用自然语言描述的任务——包括页面导航、元素交互、滚动、执行 JavaScript 以及智能正文提取。

区别于传统的爬虫，`ai-spider` 将「理解用户意图 → 自主选择工具 → 执行浏览器操作 → 提取结果」的整个流程交由大模型智能体完成，适合面对 JS 动态渲染、需要模拟真人交互的网页抓取场景。

## 特性

- 🧠 **LLM 智能体驱动**：基于 eino ADK 构建，模型自主决策要调用哪个浏览器工具、如何完成目标。
- 🌐 **真实浏览器环境**：基于 go-rod 启动 Chrome，配合 stealth 在运行时隐藏 `navigator.webdriver` 等自动化痕迹。
- 🛠️ **11 个精细化工具**：导航、可交互元素发现、CSS/XPath/node_id 三种点击与输入、滚动、JS 执行、结构化内容提取。
- 🎯 **node_id 精准定位**：通过 `get_interactives_tool` 返回的后端 DOM 节点 ID 精准点击/输入，规避同名元素与相似选择器歧义。
- 📄 **结构化取数**：`html_tool` 三种模式——默认 `accessibility` 基于可访问性树输出紧凑 Markdown（天然剔除隐藏/装饰/脚本节点、还原标题/列表/表格并附可交互 node_id，最省 token 且对 LLM 友好），`structured` 保留富文本内联格式，`full` 返回完整原始 HTML（保留精确结构与属性——需要 CSS/XPath 定位时使用）。
- 💬 **终端多轮对话**：交互式命令行输入指令，逐轮调用智能体并流式输出思考与结果。
- 🔍 **流式补齐与容错**：完整处理流式工具调用参数累积；工具失败不再中断，而是回传模型让其反思重试。
- 🧠 **持久化会话记忆（mnemo）**：基于 eino ADK 的会话存储，将多轮对话历史以 JSONL 持久化到本地（`user_data/sessions`），并在每轮自动回填作为上下文，支持跨轮次连续记忆；输入 `n`/`new` 可切换到全新会话。

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
│   ├── agent.go            # 智能体构建：角色指令、11 个工具的注册表、流式输出处理与会话记忆注入
│   ├── tools.go            # 每个浏览器工具的封装（含调试日志包装 debugTool）
│   ├── agent_test.go       # 智能体/工具缺省等单元测试
│   └── mnemo/              # 会话持久化记忆层（mnemo）
│       └── fs/             # 基于文件系统（JSONL）的会话存储
│           ├── store.go    # Store：会话的创建/加载/列举/删除，JSONL 读写
│           └── session.go  # Session：单会话内存态（消息/标题/标题索引/续跑槽位）
├── spider/
│   ├── spider.go           # 结构体定义 + InitSpider（启动/连接 Chrome） + Close + Navigate
│   ├── click.go            # 三种点击：DoClick / DoClickX / DoClickNode + clickElement
│   ├── input.go            # 三种输入 + 生效校验：DoInput×3 / typeText / elementValue
│   ├── scroll.go           # DoScroll 页面滚动
│   ├── javascript.go       # DoJavaScript 执行JS（含一次自动重试）
│   ├── interact.go         # GetInteractives 采集可交互元素 + Link/Button/Input 结构体
│   ├── extract.go          # 内容提取：GetHTML / GetStructured / AX 内容 GetAXContent
│   ├── summary.go          # 动作回执 ActionSummary + PageFingerprint + truncateText
│   ├── humanize.go         # "真人化"行为层：逐字打字、点击悬停、分段滚动（由 spider.humanize 控制）
│   ├── action.go           # 密封接口 Action 与各动作参数结构体
│   ├── option.go           # Launcher 选项函数（链式配置 Chrome 启动参数）
│   ├── extract_test.go     # 内容剪枝（prune/isLinkWall/isHiddenElement）与 AX 树渲染还原等单元测试
│   ├── spider_test.go      # 正文截断、Action 序列化、Interactives/Summary JSON 等单元测试
│   ├── spider_integration_test.go # 真实浏览器端到端集成测试（导航/取数/输入/滚动/JS 等）
│   └── option_test.go      # Launcher 选项函数单元测试
└── user_data/              # 本地数据根目录（已被 .gitignore 忽略）
    ├── sessions/           # 会话历史（JSONL，由 mnemo/fs 管理）
    └── browser/            # 浏览器用户数据（Cookies/登录态等）
```

## 模块详解

本项目的代码按职责划分为四个模块，自顶向下为 `main` → `config` → `agent` → `spider`（其中 `agent` 层内嵌了会话记忆子模块 `mnemo`）。

### main（程序入口）

[main.go](main.go) 负责整条链路的装配与交互循环，依次完成五件事：

1. 调用 `config.Load("config/config.yaml")` 读取配置，失败即终止。
2. 基于配置的 `api_key` 通过 [eino-ext](https://github.com/cloudwego/eino-ext) 创建 DeepSeek 聊天模型（`model: deepseek-flash`）。
3. 调用 `spider.InitSpider` 启动真实 Chrome 浏览器。
4. 调用 `agent.NewDefaultSpiderAgent` 组装智能体。
5. 通过 `fs.NewStore[*schema.Message]` 在 `user_data/sessions` 下创建会话存储，并调用 `modelAgent.SetMemoryStore` 开启持久化记忆，随后进入**终端多轮对话循环**：每轮读取用户输入，通过 `modelAgent.OutputMessage`（按会话 id 回填历史）流式执行并打印结果，输入 `n`/`new` 新建会话，输入 `exit` / `quit` / `q` 或 `Ctrl+D` / `Ctrl+Z` 退出。

> 说明：会话是**持久化且有记忆**的。每轮对话会按会话 id（时间前缀+随机后缀）写入 `user_data/sessions/*.jsonl`，下一轮自动把历史消息作为上下文重新注入；输入 `n`/`new` 会生成一个新会话 id，从而开始一个上下文全新的对话。

### config（配置加载）

用于集中管理「浏览器启动参数」与「模型密钥」，避免散落在各模块：

- [config.go](config/config.go)：定义 `RodConfig`（浏览器）、`ModelConfig`（模型）的结构体，每个字段以 `yaml` 标签映射 YAML 键。
- [load.go](config/load.go)：`Load(path)` 读取文件并用 `gopkg.in/yaml.v3` 反序列化，返回 `*Config`。
- `config.yaml` / `config_example.yaml`：真实配置与模板（前者含密钥且已被忽略）。

### spider（浏览器封装层）

封装 go-rod 对 Chrome 的全部底层操作，向上层 agent 提供稳定的动作原语。核心类型为 `Spider`，持有 `browser` / `page` / `logger`：

- [spider.go](spider/spider.go)：定义 `Spider` 结构体与生命周期——`InitSpider` 组装 Chrome 启动参数并连接浏览器、`Close` / `ClosePage` 释放资源、`Navigate` 导航并等待页面加载与网络空闲。是浏览器封装层的入口骨架。
- [click.go](spider/click.go)：三种点击原语 `DoClick` / `DoClickX` / `DoClickNode`（CSS / XPath / node_id），统一走 `clickElement` 完成真人化点击并等待网络空闲。
- [input.go](spider/input.go)：三种输入原语 `DoInput` / `DoInputX` / `DoInputNode`，经 `typeText` 执行输入，并通过 `elementValue` / `inputEffective` 读回控件值校验输入**是否真正生效**，返回 `(bool, error)`。
- [scroll.go](spider/scroll.go)：`DoScroll` 页面滚动，滚后等待网络空闲以捕获懒加载内容。
- [javascript.go](spider/javascript.go)：`DoJavaScript` 执行 JS 并取回返回值，自带一次重试以应对 SPA 页面偶发失败。
- [interact.go](spider/interact.go)：`GetInteractives` 通过可访问性树采集链接/按钮/输入框（含 `node_id`），并定义 `Link` / `Button` / `Input` / `Interactives` 结构体。
- [extract.go](spider/extract.go)：内容提取——`GetHTML` 返回完整原始 HTML；`GetStructured` 对整页做内容级剪枝（`prune`）后用 html-to-markdown(v2) 转为可读 Markdown；`GetAXContent` 基于浏览器可访问性树(AX)输出紧凑 Markdown（还原标题/列表/表格与层级，并内联可交互 `node_id`）。
- [summary.go](spider/summary.go)：轻量动作回执 `ActionSummary`（`effective` 表示操作是否真正生效）与 `PageFingerprint` 页面变化判断、`truncateText` 截断工具。
- [humanize.go](spider/humanize.go)：`"真人化"`行为辅助层。开启时（`spider.humanize: true`）将输入改为逐字打字（字符间随机停顿并偶发打错退格）、点击前置悬停并随机停留、滚动改为分段式并穿插随机"阅读"停顿，降低被识别为机器人的概率；关闭时回退为一次性输入、瞬时点击与瞬时滚动。开关保存在 `Spider.humanize` 字段，由 `InitSpider` 从配置注入，每个实例相互独立。
- [action.go](spider/action.go)：通过 `isAction()` 私有方法实现 Go 的**密封接口**技巧，定义 `Navigate` / `ClickBy*` / `InputBy*` / `Scroll` / `JavaScript` / `HTML` 等动作参数结构体。
- [option.go](spider/option.go)：`LauncherOption` 函数式选项，将配置映射为 Chrome 启动旗标（如 `WithUserDataDir`、`WithHeadless`、`WithRemoteDebuggingPort`），供 `InitSpider` 链式拼接。

### agent（智能体层）

基于 eino ADK 构建 LLM 智能体，是「意图 → 操作」的决策中枢：

- [agent.go](agent/agent.go)：`NewDefaultSpiderAgent` 内嵌一大段**系统指令**（角色定义、11 个工具的使用准则、工作流、行为边界、输出格式与错误处理策略），并注册全部工具；`OutputMessage` 在启用记忆时按会话 id 获取/创建 `Session`、回填历史消息，并在本轮结束后把生成的用户消息/助手回答/工具调用与结果追加持久化（剔除思考过程 `ReasoningContent`）；同时实现流式输出处理 `stream` / `streamMessage`，负责累积跨 chunk 到达的工具调用参数，并处理模型思考过程。
- [tools.go](agent/tools.go)：逐一定义每个工具的实现与 `utils.InferTool` 装配，并通过 `debugTool` 统一包装——在终端打印参数与返回值，且**将工具错误转为普通文本回传模型**，让模型反思后重试而不是中断整次运行。

### mnemo（会话持久化记忆）

mnemo 是建立在 eino ADK `MessageType` 之上的**会话记忆层**，负责把多轮对话历史持久化到磁盘并在后续轮次自动恢复：

- [fs/session.go](agent/mnemo/fs/session.go)：`Session[M]` 表示单次会话的内存态，按会话保存消息切片，并提供 `Append`（追加并持久化到 JSONL）、`GetMessages`（快照取回全部消息）、`Title`（由首个用户消息派生出会话标题，超 60 字符截断，用于会话列表展示）、`Set/GetPendingInterruptID` 与 `Set/GetMsgIdx`（供中断续跑使用的槽位/续跑标记）；`UserText` 从用户消息中抽取纯文本（兼容 `schema.Message` 与 `schema.AgenticMessage`）。
- [fs/store.go](agent/mnemo/fs/store.go)：`Store[M]` 统一管理会话，`NewStore` 在指定目录下创建（目录不存在则自动创建）；`GetOrCreate(id)` 按会话 id 缓存并懒加载，不存在则新建成 JSONL 文件（首行写入含 `message_kind` 的 header，后续每行一条消息）；`List` 遍历目录返回所有会话元信息（id/标题/创建时间）；`Delete` 删除文件并清缓存；`KindOf`/`ValidateKind` 根据 `message_kind`（`message`/`agentic`）校验文件与当前运行类型是否匹配，逐行容错跳过损坏记录。

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
- 已安装的 Chrome 浏览器（默认路径 `C:/Program Files/Google/Chrome/Application/chrome.exe`，可通过 `spider.bin` 配置修改）
- 一个可用的 LLM API Key（本项目使用 DeepSeek flash 模型）

## 快速开始

### 1. 安装依赖

```bash
cd ai-spider
go mod download
```

### 2. 配置

👉 **推荐方式：通过环境变量注入 API Key**（避免密钥进入磁盘文件，也杜绝误提交风险）

```bash
# Linux / macOS
export AI_SPIDER_API_KEY="sk-你的key"

# Windows PowerShell（当前终端有效）
$env:AI_SPIDER_API_KEY = "sk-你的key"

# Windows（永久生效，需重开终端）
[Environment]::SetEnvironmentVariable("AI_SPIDER_API_KEY","sk-你的key","User")
```

程序启动时会优先读取环境变量 `AI_SPIDER_API_KEY`，仅当它为空时才回退到 `config.yaml` 中 `model.api_key` 的值。

若你仍需使用配置文件，将模板复制为本地配置文件并填入真实值：

```bash
# Windows
copy config\config_example.yaml config\config.yaml
```

编辑 `config/config.yaml`，至少需要修改：

| 配置项 | 说明 |
|--------|------|
| `model.api_key` | 你的 LLM API Key（当环境变量未设置时作为兜底，必填） |
| `spider.bin` | Chrome 可执行文件路径（按需） |

> ⚠️ **安全提醒**：若把 API Key 写入 `config/config.yaml`，该文件包含密钥与浏览器登录态，**不要提交到版本库**。项目已在 `.gitignore` 中忽略它（详见下方「安全说明」）。只要设置了 `AI_SPIDER_API_KEY` 环境变量，可以安全地在 `config.yaml` 中留空 `api_key`。

### 3. 运行

```bash
go run .
```

程序会在终端提示输入指令，输入 `n` / `new` 新建对话（切换全新会话上下文），输入 `exit` / `quit` / `q` 或按下 `Ctrl+D` / `Ctrl+Z` 退出。

### 4. 交互示例

```
>>> 请输入指令 (输入 n/new 新建对话, exit/quit/q 退出): 打开 https://example.com 并抓取正文
```

智能体会自动完成：导航 → 等待加载 → 提取正文 → 按格式输出（页面信息、抓取内容、操作摘要等）。

> 在同一会话内连续输入多条指令时，智能体会记住之前的上下文并据此继续操作；想要开始一段全新的、无历史的任务时，输入 `n`/`new` 即可。

## 配置说明

`config/config.yaml` 采用 YAML 格式，分为 `spider`（浏览器）与 `model`（模型）两部分。

### spider（浏览器启动配置）

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `user_data_dir` | `user_data/browser` | 浏览器用户数据目录（`user_mode:false` 时作为干净隔离档案目录使用） |
| `user_mode` | `true` | 复用本机真实浏览器档案（登录态/Cookies）|
| `headless` | `false` | 是否无头模式运行 |
| `disable_blink_features` | 缺省即留空 | 缺省（不写）时不向 Chrome 传该 flag，隐藏 webdriver 交由 stealth 运行时处理 |
| `incognito` | `false` | 是否无痕模式 |
| `disable_dev_shm_usage` | `true` | 禁用共享内存（适用于受限容器） |
| `no_sandbox` | `true` | 禁用沙箱 |
| `default_page_width` / `default_page_height` | `1920` / `1080` | 窗口尺寸（经 `WithWindowSize` 实际生效） |
| `user_agent` | 缺省即留空 | 缺省（不写）时使用浏览器自报的真实 UA，最小化指纹差异 |
| `leakless` | `true` | 启用 go-rod leakless 进程保护 |
| `bin` | Chrome 默认路径 | Chrome 可执行文件路径 |
| `disable_background_networking` | `false` | 禁用后台网络 |
| `disable_background_timer_throttling` | `true` | 禁用后台标签页定时器节流 |
| `disable_backgrounding_occluded_windows` | `true` | 禁用后台窗口遮挡处理 |
| `disable_renderer_backgrounding` | `true` | 禁用渲染器后台化 |
| `basic_remote_debugging_port` | `9222` | 远程调试端口 |
| `trace` | `false` | 是否开启 CDP 通信追踪 |
| `humanize` | `true` | 是否启用"真人化"行为（逐字打字+偶发打错、点击悬停停顿、分段滚动+阅读停顿），用于降低被识别为机器人的概率 |

### model（模型配置）

| 配置项 | 说明 |
|--------|------|
| `api_key` | 调用 LLM 的 API Key（仅作**兜底**；优先使用环境变量 `AI_SPIDER_API_KEY`，见下） |

模型在代码中固定为 `deepseek-flash`。

> **API Key 读取优先级**：程序启动时先读取环境变量 `AI_SPIDER_API_KEY`，非空则覆盖 `config.yaml` 中的 `api_key`；环境变量为空时才会使用 yaml 中的值。因此强烈建议用环境变量注入密钥，避免写入配置文件带来的泄露风险。

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
| `html_tool` | 以 `mode` 三种模式取页面内容（默认 `accessibility`） | 抓取的最终步骤 |

**关键设计**：
- `node_id` 在整页导航或大规模 DOM 重渲染后会失效，此时需重新调用 `get_interactives_tool` 刷新。
- 点击/输入优先用 `node_id`，其次 CSS/XPath；`html_tool` 默认 `mode=accessibility`（基于可访问性树输出紧凑 Markdown，天然剔除隐藏/装饰/脚本节点并还原标题/列表/表格、附可交互 node_id，最省 token），`mode=structured` 保留富文本内联格式（粗体/行内代码），`mode=full` 返回完整原始 HTML（保留精确 DOM 结构与 CSS 属性）。
- 向已有内容的输入框键入前可设 `clear=true` 先清空。

`html_tool` 的 `mode=accessibility` 输出中，可交互节点采用统一的前缀标记并附 `(node:N)`，便于模型识别动作与直接交互：

| 标记 | 语义 | 示例 | 可用的交互工具 |
|------|------|------|----------------|
| `[BUTTON]` | 可点击的按钮 | `[BUTTON] Follow (node:80)` | `click_by_node_id_tool` |
| `[INPUT]` | 可输入的文本框 | `[INPUT] Search (node:90)` | `input_by_node_id_tool` |
| 普通链接 | 带 href 的链接（自带可点击语义，不加前缀） | `[Follow](/login) (node:7)` | `click_by_node_id_tool` 或直接按 href 导航 |
| `![image: …]` | 图片（仅内容，不标记） | `![image: @user]` | — |

> 约定要点：仅**有确定交互动作**的控件才打标记——`button`→`[BUTTON]`（点击）、输入框→`[INPUT]`（输入）；`link` 因 `[文本](href)` 已自表达可点击，不再叠加前缀，但仍保留 `(node:)` 供按节点点击；`image` 属纯内容，不标记。标题 `#…`、列表 `- …`、表格按 Markdown 还原。

## 测试

```bash
go test ./...
```

当前覆盖内容：
- `TestTruncateText`：正文截断的边界与中文（rune）处理。
- `TestActionInterface` / `TestActionJSONRoundTrip` / `TestActionZeroValue`：`Action` 密封接口与各类动作参数的 JSON 编解码保真。
- `TestInteractivesJSON` / `TestActionSummaryJSON`：可交互元素与动作回执的 JSON 编码。
- `Test*`（extract_test）：内容级剪枝（`prune` / `isLinkWall` / `isHiddenElement` / `isSkippable`）与 AX 树渲染（表格还原、`heading` 层级、结构/去重/噪音过滤）等用例。
- `TestHTMLModeDefault`：`html_tool` 缺省 mode 的取值行为。
- `Test*`（option_test）：各 `With*` Launcher 选项函数的旗标设置行为。
- `TestSpiderCoreIntegration`（spider_integration_test）：真实浏览器端到端集成测试，覆盖导航、取数、点击/输入、滚动、JS 执行等核心方法。

> 注：`TestSpiderCoreIntegration` 需要本机可用的 Chrome 才能运行；`go test ./...` 在执行时若没有浏览器会跳过该用例。

## 安全说明

本项目对敏感信息做了以下保护，请勿移除相关 `.gitignore` 规则：

- ✅ **推荐：API Key 通过环境变量注入**。设置 `AI_SPIDER_API_KEY` 即优先生效，密钥不落磁盘、不进版本库，最安全。用法见「快速开始 → 配置」。
- **API Key（配置文件方案）**：若将真实密钥写入 `config/config.yaml`（`model.api_key`），该文件已被 `.gitignore` 忽略，不会随源码提交；仓库中的 `config_example.yaml` 使用占位符 `your_api_key`，可安全提交。
- **浏览器登录态**：`user_data/browser/` 目录包含 Cookies、登录数据等浏览器隐私，已被 `.gitignore` 忽略，不会随源码提交。
- **会话历史**：`user_data/sessions/` 目录（由 `mnemo` 写入的 JSONL 会话记忆）可能与对话中的网页内容一并保存在本地，已随整个 `user_data/` 目录一并被 `.gitignore` 忽略，不会提交到版本库；如需清除历史，删除该目录即可。
- **环境变量**：`.env` 已在忽略规则中。
- ⚠️ **注意**：切勿将真实 `config.yaml` 重命名为可被跟踪的文件名，或将 API Key 硬编码进源码后提交。

## 许可证

本仓库基于 [MIT License](./LICENSE) 发布，Copyright (c) 2026 louyuanbo1。