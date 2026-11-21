---
name: aumbry
description: Use when publishing, structuring, validating, or managing research reports, paper readings, comparisons, news digests, or study notes as Aumbry XML documents for an Aumbry server. Do not use when reading papers themselves (qi-read-paper), running literature searches (paper-search), authoring plain prose reports (research-report), or publishing raw standalone HTML interactive guides.
---

# Aumbry XML 发布技能

**核心原则：协议是可自由组合的积木，不是固定模板。** 本技能下所有示例只是积木搭配参考，**绝不要照抄示例里的块或骨架**——按实际内容取舍标签与章节：不需要时不要多加 `<claim>`、`<metrics>`、`<next-steps>`、`<tldr>` 或 `<section>`，绝不编造空或水词 `<tldr>`/`<metrics>`/`<section>`；短文直接仅用 `<md>`（或 `<md>` + `<figure>`/`<chart>`）完全合法；表达不了的结构先用 `<md>`（含脚注/任务列表/锚点）。

## 核心工作流

1. **按需发现**：仅在需要发布到目录（`--dir`）时用 `aumbry dirs` 查目录 ID 与 `protected` 状态；**只发新文档且不引用已有条目（`related-id`/`detail-id`）时，可跳过 `aumbry list`**（需去重查旧条目再用 `aumbry list --key ...`），禁止编造 ID。若当前未配置服务端或 `dirs`/`list` 失败，本地撰写与 `aumbry validate` 仍可离线继续，留待推送前再处理服务端配置。
2. **组织文稿**：按内容需求写 XML 与本地相对路径图片（图片建议放在同目录 `imgs/` 下）；单次短文可直接写独立 XML 文件，无需建草稿目录；多轮/跨会话长文保留在当前工作目录 `./aumbry-drafts/<key-safe>/`。
3. **本地校验**：运行 `aumbry validate <file.xml>`；若失败，依据输出的 JSON（含 `line`、`code`、`field`、`hint`）就地修复，**校验通过前绝不 push**。
4. **存草稿 / 发布**：**默认不自动 push**（仅在用户明确要求“存草稿/发布/归档”或已约定每轮保存时执行）。推送成功后把返回的 `full_url` 回复给用户。

## 硬规则（必须遵守）

1. 普通 Markdown 放入 `<md><![CDATA[...]]></md>`；XML 属性内禁止出现裸 `&`、`<`。
2. 图片只用相对路径本地文件（`.png`/`.jpg`/`.jpeg`/`.webp`/`.svg`），禁用外链、绝对路径及 `..` 越级路径。`aumbry validate` 与 `aumbry push` **均以 XML 文档自身所在目录（而非当前工作目录）为基准解析 `<figure src>`**；建议将图片放到文档同级 `imgs/`（如 `./imgs/fig1.png`）。论文或资料原图能否纳入文档由用户决定，不要替用户判断版权。
3. 绝不编造 `related-id`、`detail-id` 与目录 ID；目录看 `aumbry dirs`（判断公开看 `protected`，不看 `has_password`），条目 ID 来自 `aumbry list --key` 或过往 `push` 返回值。
4. `<meta>/<date>` 用 `YYYY-MM-DD`；`<meta>/<type>` 只能是 `report` 或 `post`；使用任何 v1.1 标签或新属性必须声明 `<document version="1.1">`。
5. 幂等键按场景约定，同一篇内容（含多轮更新、勘误或同题基准复测）复用同一个 `--key`，**禁止每次换键**。
6. 不确定或缺失证据写成 `<claim level="unverified">`，对比表中缺失数值用 `<cell tag="unreported">`，不写 0、不编造事实。
7. 校验未通过绝不推送，不绕过校验。
8. 用户未授权前不推（含 `--draft`）。用户说“存到 Aumbry / 存一下 / 保存 / 先存草稿”**不等于发布**，按草稿处理（`--draft`）；只有用户明确说发布或指定发布目录才传 `--dir`；意图含糊时先问一句确认。
9. 更新已发布文档时，不改、不删已有 `<stages>`/`<stage>`/`<quiz>` 的 `id`（读者进度按 `id` 绑定）。

## 默认做法（可按内容与用户要求偏离）

1. **结构由内容决定，不是填表**：章节数量、标题、顺序与积木种类全随内容和用户意图调整；示例（见 [scenarios.md](references/scenarios.md)）仅作搭配参考，不用的块直接删去。
2. **`<meta>` 填写规范**：`<source-agent>` 填实际产出文档的 agent 或技能名（如 `qi-read-paper`、`paper-compare`、`research-agent`），不确定时默认填当前 agent 名或 `aumbry-agent`；`<type>` 单主题分析/一页结论/论文精读/对比/基准/实验记录/独立详情用 `post`，周期性或多项汇编简报（日报、周报、周期汇报、新闻简报）用 `report`；`<summary>` 必填（1–2 句概览，用于列表卡片与搜索；若正文有全文 `<tldr>` 需与其结论一致）；`<tags>` 与 `<canonical-url>` 均为可选，有明确标签或真实可靠来源再填，严禁编造链接。
3. **分层摘要 `<tldr>`**：仅在文档 ≥3 个部分或明显较长时默认配全文 `<tldr>` 与每节 `<tldr>`；一页短文、第 1 轮单 `<stage>` 精读、纯日志或用户要求精简时省略，绝不为了凑结构编造空或水词 `<tldr>`（总结只概括正文已有内容）。
4. **新闻简报**：`<news-item>` 默认带 `source` 与 `published`；先推独立详情条目取得 `id`，再填入简报 `detail-id`；未证实消息写“据称”或标 `claim level="unverified"`。
5. **复现与实验**：目标数值与容差放在实测之前；缺值标 `unreported`；图读标 `approx`；多种子离散度放 `<chart>` `series[].err`（`pie`/`scatter` 禁用 `err`）；结论取 `复现 / 未复现 / 无法量化 / 环境差异`。
6. **互动测验**：`<quiz>` 至少 1 个 `correct="true"`（多选亦可），答案对读者可见勿放敏感内容，题干优先考察应用理解。

## 意图 → 常用积木速查

| 用户表达 / 内容形态 | 常用积木组合（按需取舍） |
| :--- | :--- |
| 一页结论 / 短评 / 笔记 | 直接 `<body>` 下放 `<md>`，按需搭配 `<claim>` / `<metrics>` / `<chart>` / `<figure>`（无需 `<section>`） |
| 逐轮精读 / 分阶段指南 | `<stages id>` + `<stage id title>` + 可选 `<quiz id>` / `<next-steps>` / `<tldr>` |
| 多方案横评 / 基准 / 复现 | `<compare-table>`（`cell@tag`）+ `<metrics>` + `<chart>`（含多 series/`err`）+ `<checklist>` + `<code-block eq>` |
| 日报 / 周报 / 新闻简报 | `<tldr>` + `<news-item>`（`detail-id` / `<detail>`）+ `<metrics>` + `<timeline>` + `<sources>` |
| 文献调研 / 学术综述 | `<section>` + `<references>`（`#ref-<id>`）+ `<search-log>` + `<glossary>` + `<quote>` |

> - **逐轮精读第 1 轮提示**：第 1 轮仅写单个 `<stage>` 时，**不要**提前放置指向尚未创建之未来 `<stage>` 的 `<next-steps>`（目标不存在会报 `INVALID_TARGET`），等后续轮次新增目标 `<stage>` 后再补充；第 1 轮单 stage 若内容较短可省略 `<tldr>`。
> - 全量标签、属性、Chart JSON 字段表、容器约束与 `1.0` vs `1.1` 差异见 [xml-tags.md](references/xml-tags.md)；各场景推荐幂等键与拆解规范见 [scenarios.md](references/scenarios.md)。

## 本地草稿与 CLI 命令速查

- **何时建草稿目录**：单次发布的短文（如一页结论）可直接写独立 XML 并校验推送，**无需建草稿目录**。多轮续写文档建议保存在**当前工作目录**下的 `./aumbry-drafts/<key-safe>/`（`doc.xml`、`imgs/`、`state.json`）；若在 Git 仓库内工作，建议将 `aumbry-drafts/` 加入 `.gitignore`。
- **`<key-safe>` 转换规则**：把 `--key` 中所有不在 `[A-Za-z0-9._-]` 的字符替换为 `-`（例如 `paper:2611.01234` → `paper-2611.01234`，`repo:open-drive/driveflow` → `repo-open-drive-driveflow`）。
- **`state.json` 格式**：首次创建、尚未 `push` 时 `id` 与 `full_url` 设为 `null`，`push` 成功后从响应回填；`updated_at` 每次写入由 `date -u +%Y-%m-%dT%H:%M:%SZ` 从系统时钟生成（不要手写占位时间）。新会话先读 `state.json` 与 `doc.xml` 再续写：

```json
{ "key": "paper:2611.01234", "id": null, "full_url": null, "updated_at": "<由 date -u +%Y-%m-%dT%H:%M:%SZ 生成>" }
```

常用实测 CLI 命令（只使用真实存在的参数，首次配置用 `aumbry config set` / 查状态用 `aumbry config show`）：

```sh
# 查协议参考或配置状态
aumbry schema --format md
aumbry config show

# 发现目录（看 protected 判公开，不看 has_password）与已有条目（离线未配服务端时可先跳过，推送前再配）
aumbry dirs --format json
aumbry list --key paper:2611.01234 --limit 20

# 本地校验（完全离线，按 XML 文档所在目录解析 figure src；排错见 references/troubleshooting.md）
aumbry validate ./doc.xml
aumbry validate ./doc.xml --format text

# 预检：先校本地 XML/资源，再请求服务端核对配置/Token/目录（需要已配置且可达的服务端；不上传）
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

- **离线与 `--dry-run` 说明**：`aumbry dirs`、`aumbry list` 与 `aumbry push --dry-run` 均依赖服务端配置（未配置时直接报错 ``no server configured: run `aumbry config set --server URL` or set AUMBRY_SERVER`` 并以退出码 `1` 退出；不可达时返回连接错误）。未配服务端或离线写作期间请先用 `aumbry validate` 完成全部本地检查，推送前再配置服务端与 Token。
- `--draft` 只允许新建或更新**尚未挂载目录**的草稿；若条目已公开，须先 `aumbry unpublish <id>` 再存草稿，或直接去掉 `--draft` 更新。
- `unpublish` 仅清空挂载使访客返回 `403`（保留 ID 与数据）；`delete --yes` 永久删库、删 `items_fts` 与正文 XML（无 `--yes` 会在本地直接拒绝执行）。
- 错误码对照、Chart JSON 路径排查与服务端 401/403/409/422 处理详见 [troubleshooting.md](references/troubleshooting.md)；完整示例见 [assets/examples/](assets/examples/)。
