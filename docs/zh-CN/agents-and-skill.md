[English](../en/agents-and-skill.md) | 简体中文

# Agent 与 Skill 集成指南

Aumbry 在仓库根目录随附 [skills/aumbry/](../../skills/aumbry/SKILL.md)，并在每次 GoReleaser Release 压缩包内保留同一套文件，为 AI agent 提供 XML 撰写约束、离线校验流程、幂等键规范与发布边界。

## 安装 Skill

仓库将 Skill 存放在 `skills/aumbry/SKILL.md`，符合官方 `vercel-labs/skills` CLI 的 `skills/` 自动发现规则。

### 推荐方式：使用 `npx skills` 安装

需要 Node.js `22.20` 及以上版本。

1. 仅查看仓库中可安装的 Skill 列表（不执行安装）：

   ```sh
   npx skills add nerdneilsfield/aumbry --list
   ```

2. 安装 `aumbry` Skill（不带 `-g` 时默认安装到当前项目；加 `-g` 则安装到用户级全局目录，供所有项目复用）：

   ```sh
   # 交互式全局安装（自动检测并选择本机 agent）
   npx skills add nerdneilsfield/aumbry --skill aumbry -g

   # 非交互式安装到指定 agent
   npx skills add nerdneilsfield/aumbry --skill aumbry -g -a claude-code -a codex -y
   ```

   常用参数说明：
   - `--skill <name>`：按名称指定要安装的 Skill（此处为 `aumbry`）。
   - `-g` / `--global`：安装到用户级全局目录，而非当前项目目录。
   - `-a` / `--agent <agents...>`：指定目标 agent（如 `claude-code`、`codex`、`cursor`、`opencode`、`gemini-cli`、`github-copilot`）。
   - `--list`：只列出源仓库中发现的 Skill，不写入本地文件。
   - `-y` / `--yes`：跳过全部交互确认提示，适合脚本与 CI 环境。

   常见 agent 的默认安装目录（默认以软链指向统一副本；如需复制文件可附加 `--copy`）：

   | Agent（`-a` 参数值） | 项目级（默认） | 用户全局级（`-g`） |
   | :--- | :--- | :--- |
   | Codex、Cline、Warp、Zed（`codex` 等） | `.agents/skills/` | `~/.agents/skills/` |
   | Claude Code（`claude-code`） | `.claude/skills/` | `~/.claude/skills/` |
   | Cursor（`cursor`） | `.agents/skills/` | `~/.cursor/skills/` |
   | OpenCode（`opencode`） | `.agents/skills/` | `~/.config/opencode/skills/` |
   | Gemini CLI（`gemini-cli`） | `.agents/skills/` | `~/.gemini/skills/` |
   | GitHub Copilot（`github-copilot`） | `.agents/skills/` | `~/.copilot/skills/` |

3. 确保 `aumbry` CLI 可执行文件已加入 `PATH`，并运行 `aumbry version` 确认可用。安装 Skill 仅提供撰写规范与工作流说明，agent 在校验、查询目录和推送时仍需调用 `aumbry` 本体；预编译 Release 压缩包下载或源码编译步骤见[安装](../../README.zh-CN.md#安装)与[部署前置条件](deployment.md#前置条件)。

### 离线或从 Release 压缩包安装

在离线环境或使用 GoReleaser 发布的压缩包（内含 `skills/aumbry/`）时，可直接将目录复制或软链到目标 agent 的 skills 目录（例如 `~/.agents/skills/aumbry` 或 `~/.claude/skills/aumbry`）：

- **从仓库或解压后的 Release 压缩包复制**：适合隔离环境或容器。

  ```sh
  mkdir -p ~/.agents/skills
  cp -R skills/aumbry ~/.agents/skills/aumbry
  ```

- **从本地仓库软链**：适合本地开发环境，随 `git pull` 自动同步更新。

  ```sh
  mkdir -p ~/.agents/skills
  ln -sfn "$(pwd)/skills/aumbry" ~/.agents/skills/aumbry
  ```

完成后确认目标目录内包含 [SKILL.md](../../skills/aumbry/SKILL.md)、`references/` 与 `assets/examples/`，且 `aumbry` CLI 可执行文件位于 `PATH` 中。

## 为 Agent 配置 CLI 凭证与默认目录

建议在服务端 `api_tokens` 中为不同用途的 agent 分配各自独立的 Bearer token，配置细节见[服务端配置参考](configuration.md)，这样就能将 `aumbry unpublish` 与 `aumbry delete` 的创建者归属权隔离到单个 token。请注意：API token 仅用于 `/api/v1/ingest` 与 `/api/v1/agent/*` 接口认证，与访客读取受限目录时输入的目录密码无关。

按 agent 职责选择是否配置默认目录：

- **单一用途 agent**：例如每日自动汇报或固定专栏日报，在专用 profile 或环境中配置 `AUMBRY_DIRS` 或通过 `aumbry config set --profile daily-bot ... --dir <dir-id>` 指定默认目录，定时任务即可直接推送到该目录。
- **通用研究/编码 agent**：留空 `default_dirs` 且不设置 `AUMBRY_DIRS`，让 agent 每次发布前先执行 `aumbry dirs` 并按需求选择或向用户确认目标目录，避免将不同主题的报告误挂到同一目录。

## 发布目录决策流程

在执行正式发布（不带 `--draft`）时，agent 按下面优先级决定挂载目录：

1. **用户明确指定的目录**：根据用户给出的目录名称、路径或 ID，通过 `aumbry dirs` 找到对应 ID 并传入 `--dir <id>`。
2. **默认配置目录**：用户未指定目录时，检查 `aumbry config show` 中的 `default_dirs`，该字段来自 `AUMBRY_DIRS` 或当前激活 profile。若已设置，`aumbry push` 会默认挂载这些目录。
3. **动态查询匹配**：若无默认目录，运行 `aumbry dirs --format json` 或 `aumbry dirs --format tree`，对照 `path` 与 `name` 进行匹配。
4. **未匹配或存在多个候选时先询问**：

> **注意：要求发布但无目标目录时不要省略 `--dir`。** 当用户明确说“发布”、未指定目录且未配置默认目录（也无法唯一匹配）时，**不要不带 `--dir` 直接执行 `aumbry push`**。不传 `--dir` 会产生未挂载条目，访客访问将返回 HTTP `403`，而用户会误以为内容已对外可见。必须先运行 `aumbry dirs` 列出候选目录向用户确认。

5. **草稿例外（`--draft`）**：只要带上 `--draft`，`aumbry push` 一律忽略 `default_dirs` 与 `AUMBRY_DIRS`，将条目以未挂载草稿形式保存（`dirs: []`）。

## 草稿与发布的授权约定

| 用户指令意图 | Agent 应执行的 CLI 动作 | 服务端结果状态 |
| :--- | :--- | :--- |
| “写一份 XML / 本地检查”或未要求上传 | 仅执行 `aumbry validate`，不推送 | 本地通过校验，服务端无改动 |
| “存到 Aumbry / 存一下 / 保存 / 先存草稿” | 执行 `aumbry push <file> --key <k> --draft` | 未挂载草稿（访客 `403`，可在 `/admin` 查看） |
| 明确说“发布”且有明确或默认目录 | 执行 `aumbry push <file> --key <k> --dir <dir-id>` | 已挂载到目标目录，可通过 `/d/<dir-id>` 访问 |
| 意图含糊，或要求发布但无目标/默认目录 | 先向用户确认目录或意图后再执行 `aumbry push` | 避免误公开或生成 `403` 未挂载条目 |

> **注意：挂入目录前核对 `protected` 字段。** 向某个目录挂载私密笔记前，请核对 `aumbry dirs` 返回中的 `"protected"`（不要只看 `"has_password"`）。`has_password: false` 且 `protected: false` 的目录对外持有链接即公开可读。

## 通过 MCP（`/mcp`）接入 Agent

除在终端调用 `aumbry` CLI 外，支持 MCP 的 Agent 还可直接连接服务端的 Streamable HTTP `/mcp` 端点：

- **凭据与权限**：通过 HTTP 头传入 `Authorization: Bearer <api_token>`（可附带 `X-Aumbry-Dir-Tokens: ad_...,ad_...` 访问受保护子树），或传入 `Authorization: Bearer ad_<...>` 仅只读该目录授权子树；`/mcp` 不接受浏览器 Cookie。
- **内置工具**：`list_dirs`、`list_items`、`search_items`、`get_item`、`validate_xml` 与 `push_item`（写入仅对 API Token 开放）。
- **何时改用 CLI `aumbry push`**：MCP `push_item` 仅支持纯文本与 `v1.2` 公网 `http(s)://` 外链 `<figure src>` XML（受 `16 MiB` JSON-RPC 请求体与 `10 MiB` 解码 XML 两道上限约束）；凡需上传本地图片、音频或 `<attachment type="pdf">` 原文附件，或经多轮 JSON 转义超出 `16 MiB` 的文档，请统一调用 CLI `aumbry push`。
- **部署前提**：通过非回环主机名或反向代理访问 `/mcp` 时，服务端必须配置匹配的 `public_url`（详见[部署与运维指南](deployment.md)）。
