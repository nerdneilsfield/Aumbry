[English](../en/agents-and-skill.md) | 简体中文

# Agent 与 Skill 集成指南

Aumbry 在仓库根目录随附 [skills/aumbry/](../../skills/aumbry/SKILL.md)，并在每次 GoReleaser Release 压缩包内保留同一套文件，为 AI agent 提供 XML 撰写约束、离线校验流程、幂等键规范与发布边界。

## 安装 Skill

将仓库或 Release 解压包中的 `skills/aumbry` 放入 agent 运行环境的 skills 目录，常见路径为 `~/.agents/skills/aumbry`：

1. **仓库软链安装：**适合开发环境使用，随 `git pull` 自动同步更新。

   ```sh
   mkdir -p ~/.agents/skills
   ln -sfn "$(pwd)/skills/aumbry" ~/.agents/skills/aumbry
   ```

2. **独立目录复制：**适合隔离容器或从 Release 压缩包安装。

   ```sh
   mkdir -p ~/.agents/skills
   cp -R skills/aumbry ~/.agents/skills/aumbry
   ```

3. 确认 `~/.agents/skills/aumbry` 下包含 [SKILL.md](../../skills/aumbry/SKILL.md)、`references/` 与 `assets/examples/`。

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
