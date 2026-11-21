# Aumbry 场景与积木参考（Scenarios & Drafting）

> **设计原则**：下表只是“这类内容常用哪些积木”的参考，**不是必填骨架**。永远由内容和用户需求决定结构：无需强凑章节，不用的积木不要硬塞；表达不了的先用 `<md>`。

## 1. 场景与 `<meta>` / 幂等键映射表

| 场景 | 内容来源 / Skill | `meta/type` | 推荐幂等键（`--key`） | 常用积木 | 参考示例 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **A 单篇论文精读** | `qi-read-paper` | `post` | `paper:<arxiv-id \| doi \| slug>` | `stages`/`stage`、`quiz`、`next-steps`、`tldr`、`claim`、`equation`、`figure`、`mermaid`、`chart`、`metrics`、`steps`、`glossary`、`compare-table`、`references`/`sources` | `paper-reading.xml` |
| **B 多方法对比** | `paper-compare` | `post` | `compare:<slug>` | `compare-table`（`cell@tag`）、`claim`、`callout type="warn"`、`chart` | `compare-repro.xml` |
| **C 日报/周报/简报** | 各日报 Agent、`steward-reports` | `report` | `<agent>:<YYYY-MM-DD>` 或 `<agent>:<YYYY>-W<ww>` | `tldr`（全文 + 各节）、`metrics`、`paper-card`、`repo-card`、`timeline`、`sources`、`related-ids` | `news-digest.xml` |
| **D 研究报告** | `research-report`（+ `paper-search`） | `post` | `report:<slug>` | `section`、`figure`、`mermaid`、`chart`、`references`、`compare-table`、`search-log` | — |
| **E 开源拆解 / vibe coding** | 各拆解 Agent | `post` / `report` | `repo:<owner>/<name>` / `<agent>:<YYYY-MM-DD>` | `repo-card`、`mermaid`、`code-block`、`steps`、`checklist`、`metrics` | — |
| **F 周期汇报** | `steward-reports` | `report` | `steward:<YYYY-MM-DD>` / `steward:<YYYY>-W<ww>` | `section`（Facts/Results/Blockers/Evidence/Next steps）、`checklist`、`metrics`、`claim` | — |
| **G 复现 / 实验报告** | `paper-reproduce`、`algorithm-experiment` | `post` | `repro:<paper-slug>` / `exp:<slug>` | `compare-table`（含 `unreported`/`approx`）、`metrics`、`chart`（`series[].err`）、`checklist`、`claim`（`unverified`）、`stages` | `compare-repro.xml` |
| **内部基准 / 实验记录** | 性能基准 / 内部实验 Agent | `post` | `bench:<slug>` 或 `exp:<slug>`（如 `bench:driveflow-h100-fps`、`exp:flow-step-ablation`） | `md`、`metrics`、`chart`（多 series / `err`）、`compare-table`、`figure`、`claim` | `one-pager.xml`、`compare-repro.xml` |
| **H 论文到代码实现** | `paper-to-code` | `post` | `impl:<paper-slug>` | `glossary`、`code-block eq="..."`、`checklist`、`claim` | `compare-repro.xml` |
| **I 论文审计** | `paper-audit` | `post` | `audit:<paper-slug>` | 分 `section` 隔离自动检查与 LLM 判断、`metrics`、`checklist`、`claim` | — |
| **J 学习路径 / 互动指南** | `interactive-guide` | `post` | `guide:<slug>` | `stages`、`quiz`、`steps interactive="true"`、`glossary`、`mermaid`、`next-steps`（只发结构化 XML，不推整页 HTML） | `paper-reading.xml` |
| **K 每日新闻简报** | 各新闻 Agent | `report` | `news:<YYYY-MM-DD>` | `tldr`（全文 + 分区）、`news-item`（`detail-id` 或内联 `<detail>`）、`metrics`、`timeline`、`sources` | `news-digest.xml` |
| **L 新闻详情** | 各新闻 Agent | `post` | `news:<YYYY-MM-DD>:<slug>` | `tldr`、`section`、`quote`、`timeline`、`claim`、`references`/`sources`、`related-ids`（指回简报） | `news-detail.xml` |

> - **幂等键复用原则与格式约束**：**同一篇内容（含多轮更新、勘误或同题基准复测）必须复用同一个 `--key`，禁止每次换键**。服务端对 `external_key` 的唯一格式要求为 trim 后 `1..200` 个 Unicode 字符且不含控制字符（超限或纯空白 → `400 invalid_request`）；上表前缀均为约定建议。
> - **`<meta>` 字段指引**：`<source-agent>` 填产出文档的 agent/技能名（不确定时默认填当前 agent 名或 `aumbry-agent`）；`<type>` 单主题分析/基准/实验/详情用 `post`，周期汇总/简报用 `report`；`<summary>` 必填（1–2 句卡片与搜索摘要，与全文 `<tldr>` 结论保持一致）；`<tags>` 与 `<canonical-url>` 均为可选，有明确标签或真实可靠来源再填，严禁编造链接。

---

## 2. 逐轮增量发布与本地草稿目录

- **单次短文可不建草稿目录**：一页结论、短评等一次性编写推送的文档直接写独立 XML 文件（配图放在同目录 `imgs/`）即可，无需创建草稿目录。
- **多轮长文用草稿目录**：多轮对话产出的长文档（如三轮论文精读）保存在**当前工作目录**的 `./aumbry-drafts/<key-safe>/`，使用**同一 `--key` 反复 `aumbry push`** 完成增量更新。本地稿件始终是唯一事实来源（不要回读服务端已重写图片 URL 的 XML）；若在 Git 仓库内工作，建议将 `aumbry-drafts/` 加入 `.gitignore`。
- **`<key-safe>` 转换规则**：把 `--key` 中所有不在 `[A-Za-z0-9._-]` 的字符替换为 `-`（例如 `paper:2611.01234` → `paper-2611.01234`，`repo:open-drive/driveflow` → `repo-open-drive-driveflow`）。

```text
./aumbry-drafts/<key-safe>/
├── doc.xml      # 当前完整 XML，下一轮直接在原文基础上增补 <stage> 或 <section>
├── imgs/        # 本地相对路径图片，如 ./imgs/fig1.png（validate/push 均按 doc.xml 所在目录解析；原图使用由用户决定）
└── state.json   # 跨会话上下文：未 push 时 id/full_url 为 null，push 后回填；updated_at 用 date -u +%Y-%m-%dT%H:%M:%SZ 生成
```

`state.json` 首次创建（尚未 `push`）示例（`push` 成功后由服务端响应回填 `id` 与 `full_url`）：

```json
{ "key": "paper:2611.01234", "id": null, "full_url": null, "updated_at": "<由 date -u +%Y-%m-%dT%H:%M:%SZ 生成>" }
```

- 新会话续写前先读 `state.json` 与 `doc.xml`，不要凭记忆重写已有章节。
- 每轮可追加新 `<stage id="...">` 或顶层 `<section>`；**严禁修改或删除已发布 `<stages>`/`<stage>`/`<quiz>` 的 `id`**（读者浏览器进度按 `id` 持久化）。
- 第 1 轮仅有 1 个 `<stage>` 时合法且不显示步骤条与“继续”：**不要在此时放入指向尚不存在之未来 `<stage>` 的 `<next-steps>`**（目标不存在会报 `INVALID_TARGET`），等后续轮次新增目标 `<stage>` 后再补充；第 1 轮单 stage 的短内容也可省略 `<tldr>`。

---

## 3. 各场景撰写要点（默认参考）

- **对比、基准与复现（B / G / H）**：目标数值与容差放在实测结果之前；缺失指标用 `<cell tag="unreported">`，不写 0、不估算；图读估值标 `tag="approx"`；多随机种子离散度用 `<chart>` 的 `series[].err`（`pie`/`scatter` 不可带 `err`）；公式与代码映射用 `<code-block eq="<equation-id>">`。
- **日报 / 周报（C）**：同一日重跑用同一 `--key` 覆盖不堆积；周报按天分 `<section>`，可每天配 `<tldr>` 并附整篇 `<tldr>`，用 `related-ids` 关联当周各日报，不抄正文。
- **研究报告（D）**：内部 `[@note-slug]` 引用在本地装配时转为 `<references>` + `[标签](#ref-<id>)` 锚点；只读到摘要/检索片段时在 `<ref>` 或 `<paper-card>` 标 `read-depth="abstract|snippet"`；检索过程用 `<search-log>`（纳入/排除标准、`query`、`excluded` 理由、`gap`、`stop`）。
- **新闻简报与详情（K / L）**：短消息用 `<news-item>` 内联 `<detail>`；需独立检索与分享的详情写成独立条目先推，取 `id` 后填入简报的 `detail-id`（详情可再用同 `--key` 更新 `related-ids` 指回简报）。无法核实的消息用 `<claim level="unverified">` 或“据称”。
- **分层摘要 `<tldr>`**：仅在文档 ≥ 3 个部分或明显偏长时默认配全文 `<tldr>` 与每节 `<tldr>`（总结只概括正文已有内容，与 `meta/summary` 结论一致）；短文、一页结论、第 1 轮单 `<stage>` 精读或用户要求精简时省略，绝不为了凑结构编造空或水词 `<tldr>`。
