---
name: aumbry
description: Use when publishing, structuring, validating, or managing research reports, paper readings, comparisons, news digests, or study notes as Aumbry XML documents for an Aumbry server. Do not use when reading papers themselves (qi-read-paper), running literature searches (paper-search), authoring plain prose reports (research-report), or publishing raw standalone HTML interactive guides.
---

# Aumbry XML 发布技能

**核心原则：协议是可自由组合的积木，不是固定模板。** 本技能下所有示例仅作搭配参考，**禁止照抄骨架**——按内容取舍标签，不需要时不凑 `<claim>`、`<metrics>`、`<next-steps>`、`<tldr>` 或 `<section>`，禁止编造空或水词 `<tldr>`/`<metrics>`/`<section>`；短文直接仅用 `<md>`（或 `<md>` + `<figure>`/`<chart>`）完全合法；表达不了的结构先用 `<md>`（含脚注/任务列表/锚点）。

## 核心工作流

1. **按需发现**：发草稿或离线写作时无需连服务端；仅发新条目且不引用 `related-id`/`detail-id` 时跳过 `aumbry list`（查旧条目再用 `aumbry list --key ...`）；准备正式发布前运行 `aumbry dirs` 获取目录 ID 与 `protected` 状态，禁止编造 ID。
2. **组织文稿**：单次短文直接写独立 XML 与同级 `imgs/`；多轮/跨会话长文写入当前工作目录 `./aumbry-drafts/<key-safe>/`（`doc.xml`、`imgs/`、`state.json`）。
3. **本地校验**：运行 `aumbry validate <file.xml>`；若失败，按输出的 JSON（`line`、`code`、`field`、`hint`）就地修复，**校验通过前绝不推送**。
4. **存草稿 / 发布**：**未获授权不自动 push**（仅在用户明确说“存草稿/发布/归档”或已约定每轮保存时执行）。推送成功后向用户返回 `full_url`。

## 硬规则（必须遵守）

1. Markdown 正文放入 `<md><![CDATA[...]]></md>`；XML 属性内禁止出现裸 `&`、`<`。
2. `<figure src>` 仅限相对路径本地图片（`.png`/`.jpg`/`.jpeg`/`.webp`/`.svg`），禁用外链、绝对路径与 `..` 越级路径。`validate` 与 `push` **按 XML 文档所在目录（非 CWD）解析相对路径**；原图能否纳入由用户决定。
3. 禁止编造 `related-id`、`detail-id` 与目录 ID；查看公开性看 `aumbry dirs` 的 `protected`（不看 `has_password`），条目 ID 来自 `aumbry list --key` 或过往 `push` 响应。
4. `<meta>/<date>` 用 `YYYY-MM-DD`；`<meta>/<type>` 为 `report` 或 `post`；根元素必须是 `<document version="1.1">`，这是唯一受支持的版本。
5. 同题更新（含多轮修订、勘误、复测）复用同一个 `--key`（幂等键），**禁止每次换键**。
6. 未核实事实写 `<claim level="unverified">`，对比表缺值写 `<cell tag="unreported">`，不写 0、不编造数据。
7. 校验未通过绝不推送，不绕过校验。
8. 未获授权不推送（含 `--draft`）。说“存一下/保存/存到 Aumbry/先存草稿”按 `--draft` 处理；明确说“发布”但**未指定目录且未配默认目录时必须先问用户，禁止不带 `--dir` 推送**（未挂载条目对访客返回 `403`，用户却会误以为已发布）。
9. 更新已发布条目时，不改、不删已有 `<stages>`/`<stage>`/`<quiz>` 的 `id`（浏览器进度按 `id` 绑定）。

## 默认做法（可按内容与用户要求偏离）

1. **按内容选块**：章节与积木全由实际内容决定，不用的块直接删去（搭配参考见 [scenarios.md](references/scenarios.md)）。
2. **`<meta>` 规范**：`<source-agent>` 填实际产出 agent/技能名（不确定填当前 agent 名或 `aumbry-agent`）；单主题精读/短评/复现/详情用 `post`，周期汇编/日报/简报用 `report`；`<summary>` 填 1–2 句概览并与全文 `<tldr>` 一致；`<tags>` 与 `<canonical-url>` 无依据则留空，禁编 URL。
3. **分层 `<tldr>`**：仅当文档 >= 3 部分或明显较长时配全文及分节 `<tldr>`（只总结已有内容）；一页短文、第 1 轮单 `<stage>` 或精简文档省略。
4. **新闻简报**：`<news-item>` 带 `source` 与 `published`；先推独立详情条目取得 `id` 再填 `detail-id`；未证实消息标 `claim level="unverified"`，或在正文写“据称”，不写成事实。
5. **复现与实验**：目标值与容差写在实测前；缺值标 `unreported`，图读估值标 `approx`，多种子方差放 `<chart>` `series[].err`（`pie`/`scatter` 禁用）；结论用 `复现 / 未复现 / 无法量化 / 环境差异`；环境与指标定义都已排除前，不写“论文有误”。
6. **互动测验**：`<quiz>` 至少 1 个 `correct="true"`（可多选），答案对读者可见，勿放敏感信息，题干考察应用而非复述。
7. **论文证据标注**：论文位置（`§3.2`、`Fig. 4`、`Eq. (5)`、`Table 2`）写进 `<claim source="…">`；`level` 区分 `claim`（作者主张）、`evidence`（论文证据）、`inference`（分析推断）、`unverified`（待核验）。论文未报告的信息写成 `unverified` 的 claim，不留空。

以上规则只管“内容 skill 产出后如何落到 Aumbry”，不改写 `qi-read-paper`、`paper-compare` 等 skill 的流程，也不依赖它们是否安装。

## 意图 → 常用积木速查

| 用户表达 / 内容形态 | 常用积木组合（按需取舍） |
| :--- | :--- |
| 一页结论 / 短评 / 笔记 | 直接 `<body>` 下放 `<md>`，按需配 `<claim>` / `<metrics>` / `<chart>` / `<figure>`（可省 `<section>`） |
| 逐轮精读 / 分阶段指南 | `<stages id>` + `<stage id title>` + 可选 `<quiz id>` / `<next-steps>` / `<tldr>` |
| 多方案横评 / 基准 / 复现 | `<compare-table>`（`cell@tag`）+ `<metrics>` + `<chart>`（多 series/`err`）+ `<checklist>` + `<code-block eq>` |
| 日报 / 周报 / 新闻简报 | `<tldr>` + `<news-item>`（`detail-id` / `<detail>`）+ `<metrics>` + `<timeline>` + `<sources>` |
| 文献调研 / 学术综述 | `<section>` + `<references>`（`#ref-<id>`）+ `<search-log>` + `<glossary>` + `<quote>` |

> 第 1 轮仅有单个 `<stage>` 时不要放指向未建未来 `<stage>` 的 `<next-steps>`（会报 `INVALID_TARGET`）；全量标签、属性与 Chart JSON 字段见 [xml-tags.md](references/xml-tags.md)。

## 发布目录决策与 CLI 认证速查

**选目录决策流程：** `用户指明目录（名称/路径/ID）` > `aumbry config show 里的 default_dirs（或 AUMBRY_DIRS）` > `aumbry dirs 按 path/name 匹配` > `无默认目录且匹配不到或多个时必须先问（禁止不带 --dir 推送“发布”请求）`。`--draft` 一律忽略默认目录。

| 配置方式 / 要素 | 具体说明 | 优先级与边界 |
| :--- | :--- | :--- |
| 认证实质 | Bearer `token` 来自服务端 `api_tokens` | 只管推送与 agent API，与目录密码无关；`unpublish`/`delete` 仅创建该条目的 `token` 可用 |
| 环境变量 | `AUMBRY_SERVER`、`AUMBRY_TOKEN`、`AUMBRY_DIRS`（逗号分隔） | 优先级高于 profile；未建配置文件时可独立工作 |
| Profile 配置 | `aumbry config set [--profile P] --server URL --token TOK [--dir ID]` | 写入 `~/.config/aumbry/config.yaml`（`0600` 明文存 token）；`aumbry config show` / `use <P>` |
| 参数覆盖顺序 | `命令行参数（--dir、--profile） > 环境变量 > 选中 profile` | `--profile` 指向不存在的 profile 直接报 exit `1` |

## 本地草稿与 CLI 命令

- **草稿目录规则**：多轮文档存于 `./aumbry-drafts/<key-safe>/`（建议加入 `.gitignore`），`<key-safe>` 把 `--key` 非 `[A-Za-z0-9._-]` 字符替换为 `-`（如 `paper:2611.01234` → `paper-2611.01234`）。首次建 `state.json` 填 `{"key":"...","id":null,"full_url":null,"updated_at":"<date -u +%Y-%m-%dT%H:%M:%SZ>"}`，推送成功后回填 `id` 与 `full_url`，续写前先读 `state.json` 与 `doc.xml`。

```sh
aumbry schema --format md
aumbry config show
aumbry dirs --format json            # 看 protected 判公开，不看 has_password
aumbry list --key paper:2611.01234 --limit 20
aumbry validate ./doc.xml --format text
aumbry push ./doc.xml --key paper:2611.01234 --dir <dir-id> --dry-run
aumbry push ./doc.xml --key paper:2611.01234 --draft
aumbry push ./doc.xml --key paper:2611.01234 --create-only --dir <dir-id>
aumbry push ./doc.xml --key paper:2611.01234 --dir <dir-id>
aumbry unpublish <item-id>
aumbry delete <item-id> --yes
```

- **离线与错误**：`validate` 完全离线；`dirs`/`list`/`push --dry-run` 未配服务端报 ``no server configured: run `aumbry config set --server URL` or set AUMBRY_SERVER``（exit `1`）。`--draft` 推已发布条目报 `409 already_published`；`unpublish` 清挂载保留条目，`delete --yes` 永久删库与 XML（非创建者 token 报 `403 not_owner`）。完整错误码排查见 [troubleshooting.md](references/troubleshooting.md)，示例见 [assets/examples/](assets/examples/)。
