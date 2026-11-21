---
name: aumbry
description: Use when publishing, structuring, validating, or managing research reports, paper readings, comparisons, news digests, or study notes as Aumbry XML documents for an Aumbry server. Do not use when reading papers themselves (qi-read-paper), running literature searches (paper-search), authoring plain prose reports (research-report), or publishing raw standalone HTML interactive guides.
---

# Aumbry XML 发布技能

**核心原则（PRD 6.0）：协议是可自由组合的积木，不是固定模板。** 按内容需要挑选标签与章节，绝不套骨架填空；不要硬凑用不到的章节，不要编造空的 `<tldr>`、`<metrics>` 或 `<section>`；只用 `<md>` 写短文完全合法；表达不了的结构先用 `<md>`（含脚注/任务列表/锚点）。本技能下所有示例只是参考。

## 核心工作流

1. **按需发现**：发布或引用前，用 `aumbry dirs` 查目录 ID 与 `protected` 状态，用 `aumbry list --key ...` 查已有条目 ID；禁止编造 ID。
2. **组织草稿**：按内容需求写 XML 与本地相对路径图片；多轮长文保留 `./aumbry-drafts/<key-safe>/`。
3. **本地校验**：运行 `aumbry validate <file.xml>`；若失败，依据输出的 JSON（含 `line`、`code`、`field`、`hint`）就地修复，**校验通过前绝不 push**。
4. **发布 / 存草稿**：**默认不自动 push**（仅在用户明确要求“存草稿/发布/归档”或已约定每轮保存时执行）。推送成功后把返回的 `full_url` 回复给用户。

## 硬规则（必须遵守）

1. 普通 Markdown 放入 `<md><![CDATA[...]]></md>`；XML 属性内禁止出现裸 `&`、`<`。
2. 图片只用相对路径本地文件（`.png`/`.jpg`/`.jpeg`/`.webp`/`.svg`），禁用外链、绝对路径及 `..` 越级路径。
3. 绝不编造 `related-id`、`detail-id` 与目录 ID；目录看 `aumbry dirs`（判断公开看 `protected`，不看 `has_password`），条目 ID 来自 `aumbry list --key` 或过往 `push` 返回值。
4. `<meta>/<date>` 用 `YYYY-MM-DD`；`<meta>/<type>` 只能是 `report` 或 `post`；使用任何 v1.1 标签或新属性必须声明 `<document version="1.1">`。
5. 幂等键按场景约定，同一篇内容逐轮或修正更新复用同一个 `--key`，**禁止每次换键**。
6. 不确定或缺失证据写成 `<claim level="unverified">`，对比表中缺失数值用 `<cell tag="unreported">`，不写 0、不编造事实。
7. 校验未通过绝不推送，不绕过校验。
8. 用户未授权前不推（含 `--draft`）；用户说“先存草稿”才加 `--draft`，明确说“发布”才传 `--dir`。
9. 更新已发布文档时，不改、不删已有 `<stages>`/`<stage>`/`<quiz>` 的 `id`（读者进度按 `id` 绑定）。

## 默认做法（可按内容与用户要求偏离）

1. **结构由内容决定，不是填表**：章节数量、标题、顺序与积木种类全随内容和用户意图调整；示例（见 [scenarios.md](references/scenarios.md)）仅作搭配参考。
2. **分层摘要**：≥3 部分或长文档默认配全文 `<tldr>` 与每节 `<tldr>`；单页短文、纯日志或用户明确精简时省略。总结仅概括正文已有内容，结论与 `<meta>/<summary>` 一致。
3. **新闻简报**：`<news-item>` 默认带 `source` 与 `published`；先推独立详情条目取得 `id`，再填入简报 `detail-id`；未证实消息写“据称”或标 `claim level="unverified"`。
4. **复现与实验**：目标数值与容差放在实测之前；缺值标 `unreported`；图读标 `approx`；多种子离散度放 `<chart>` `series[].err`；结论取 `复现 / 未复现 / 无法量化 / 环境差异`。
5. **互动测验**：`<quiz>` 至少 1 个 `correct="true"`（多选亦可），答案对读者可见勿放敏感内容，题干优先考察应用理解。

## 意图 → 常用积木速查

| 用户表达 / 内容形态 | 常用积木组合 |
| :--- | :--- |
| 一页结论 / 短评 / 笔记 | 直接 `<body>` 下放 `<md>` + `<claim>` / `<chart>` / `<figure>`（无需 `<section>`） |
| 逐轮精读 / 分阶段指南 | `<stages id>` + `<stage id title>` + `<quiz id>` + `<next-steps>` + `<tldr>` |
| 多方案横评 / 复现记录 | `<compare-table>`（`cell@tag`）+ `<metrics>` + `<chart>`（含 `err`）+ `<checklist>` + `<code-block eq>` |
| 日报 / 周报 / 新闻简报 | `<tldr>` + `<news-item>`（`detail-id` / `<detail>`）+ `<metrics>` + `<timeline>` + `<sources>` |
| 文献调研 / 学术综述 | `<section>` + `<references>`（`#ref-<id>`）+ `<search-log>` + `<glossary>` + `<quote>` |

> 全量标签、属性、容器约束与 `1.0` vs `1.1` 差异见 [xml-tags.md](references/xml-tags.md)；各场景推荐幂等键与拆解规范见 [scenarios.md](references/scenarios.md)。

## 本地草稿与 CLI 命令速查

逐轮撰写把稿件保存在工作目录 `./aumbry-drafts/<key-safe>/`（`doc.xml`、`imgs/`、`state.json`），新会话先读 `state.json` 与 `doc.xml` 再续写：

```json
{ "key": "paper:2611.01234", "id": "abc123456789", "full_url": "https://hub.example/p/abc123456789", "updated_at": "2026-11-20T12:00:00Z" }
```

常用实测 CLI 命令（只使用真实存在的参数，首次配置用 `aumbry config set` / 查状态用 `aumbry config show`）：

```sh
# 查协议参考或配置状态
aumbry schema --format md
aumbry config show

# 发现目录（看 protected 判公开，不看 has_password）与已有条目
aumbry dirs --format json
aumbry list --key paper:2611.01234 --limit 20

# 本地校验（默认 JSON；排错见 references/troubleshooting.md）
aumbry validate ./doc.xml
aumbry validate ./doc.xml --format text

# 预检：只校本地 XML/资源并核对服务端/Token/目录，不上传
aumbry push ./doc.xml --key paper:2611.01234 --dir <dir-id> --dry-run

# 存草稿（忽略 default_dirs 与 AUMBRY_DIRS；不可与 --dir 同用；对已发布条目会报 409）
aumbry push ./doc.xml --key paper:2611.01234 --draft

# 首次防覆盖创建（必须配 --key，不可与 --id 同用）或发布到目录
aumbry push ./doc.xml --key paper:2611.01234 --create-only --dir <dir-id>
aumbry push ./doc.xml --key paper:2611.01234 --dir <dir-id>

# 撤回与删除（Agent 仅可操作自己 Token 创建的条目；别人或 v1.0 遗留条目报 403 not_owner）
aumbry unpublish <item-id>
aumbry delete <item-id> --yes
```

- `--draft` 只允许新建或更新**尚未挂载目录**的草稿；若条目已公开，须先 `aumbry unpublish <id>` 再存草稿，或直接去掉 `--draft` 更新。
- `unpublish` 仅清空挂载使访客返回 `403`（保留 ID 与数据）；`delete --yes` 永久删库、删 `items_fts` 与正文 XML（无 `--yes` 会在本地直接拒绝执行）。
- 错误码对照、Chart JSON 路径排查与服务端 401/403/409/422 处理详见 [troubleshooting.md](references/troubleshooting.md)；完整示例见 [assets/examples/](assets/examples/)。
