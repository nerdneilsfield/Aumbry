# Aumbry 场景与积木参考（Scenarios & Drafting）

> **设计原则（PRD 6.0）**：下表只是“这类内容常用哪些积木”的参考，**不是必填骨架**。永远由内容和用户需求决定结构：无需强凑章节，不用的积木不要硬塞；表达不了的先用 `<md>`。

## 1. 场景 A–L 映射表（PRD 2.2）

| 场景 | 内容来源 / Skill | `meta/type` | 推荐幂等键（`--key`） | 常用积木 | 参考示例 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **A 单篇论文精读** | `qi-read-paper` | `post` | `paper:<arxiv-id \| doi \| slug>` | `stages`/`stage`、`quiz`、`next-steps`、`tldr`、`claim`、`equation`、`figure`、`mermaid`、`chart`、`metrics`、`steps`、`glossary`、`compare-table`、`references`/`sources` | `paper-reading.xml` |
| **B 多方法对比** | `paper-compare` | `post` | `compare:<slug>` | `compare-table`（`cell@tag`）、`claim`、`callout type="warn"`、`chart` | `compare-repro.xml` |
| **C 日报/周报/简报** | 各日报 Agent、`steward-reports` | `report` | `<agent>:<YYYY-MM-DD>` 或 `<agent>:<YYYY>-W<ww>` | `tldr`（全文 + 各节）、`metrics`、`paper-card`、`repo-card`、`timeline`、`sources`、`related-ids` | `news-digest.xml` |
| **D 研究报告** | `research-report`（+ `paper-search`） | `post` | `report:<slug>` | `section`、`figure`、`mermaid`、`chart`、`references`、`compare-table`、`search-log` | — |
| **E 开源拆解 / vibe coding** | 各拆解 Agent | `post` / `report` | `repo:<owner>/<name>` / `<agent>:<YYYY-MM-DD>` | `repo-card`、`mermaid`、`code-block`、`steps`、`checklist`、`metrics` | — |
| **F 周期汇报** | `steward-reports` | `report` | `steward:<YYYY-MM-DD>` / `steward:<YYYY>-W<ww>` | `section`（Facts/Results/Blockers/Evidence/Next steps）、`checklist`、`metrics`、`claim` | — |
| **G 复现 / 实验报告** | `paper-reproduce`、`algorithm-experiment` | `post` | `repro:<paper-slug>` / `exp:<slug>` | `compare-table`（含 `unreported`/`approx`）、`metrics`、`chart`（`series[].err`）、`checklist`、`claim`（`unverified`）、`stages` | `compare-repro.xml` |
| **H 论文到代码实现** | `paper-to-code` | `post` | `impl:<paper-slug>` | `glossary`、`code-block eq="..."`、`checklist`、`claim` | `compare-repro.xml` |
| **I 论文审计** | `paper-audit` | `post` | `audit:<paper-slug>` | 分 `section` 隔离自动检查与 LLM 判断、`metrics`、`checklist`、`claim` | — |
| **J 学习路径 / 互动指南** | `interactive-guide` | `post` | `guide:<slug>` | `stages`、`quiz`、`steps interactive="true"`、`glossary`、`mermaid`、`next-steps`（只发结构化 XML，不推整页 HTML） | `paper-reading.xml` |
| **K 每日新闻简报** | 各新闻 Agent | `report` | `news:<YYYY-MM-DD>` | `tldr`（全文 + 分区）、`news-item`（`detail-id` 或内联 `<detail>`）、`metrics`、`timeline`、`sources` | `news-digest.xml` |
| **L 新闻详情** | 各新闻 Agent | `post` | `news:<YYYY-MM-DD>:<slug>` | `tldr`、`section`、`quote`、`timeline`、`claim`、`references`/`sources`、`related-ids`（指回简报） | `news-detail.xml` |

> **服务端对 `external_key` 的唯一格式约束**：`--key` trim 后长度为 `1..200` 个 Unicode 字符且不含控制字符（超限或纯空白 → `400 invalid_request`）。上表仅为建议命名约定。

---

## 2. 逐轮增量发布与本地草稿目录（PRD 2.3）

多轮对话产出的长文档（如三轮论文精读）使用**同一 `--key` 反复 `aumbry push`** 完成增量更新，本地稿件始终是唯一事实来源（不要回读服务端已重写图片 URL 的 XML）：

```text
./aumbry-drafts/<key 的文件名安全形式>/
├── doc.xml      # 当前完整 XML，下一轮直接在原文基础上增补 <stage> 或 <section>
├── imgs/        # 本地相对路径图片，如 ./imgs/fig1.png
└── state.json   # 记录跨会话上下文，例：{"key":"paper:2611.01234","id":"abc123456789","full_url":"https://...","updated_at":"2026-11-20T12:00:00Z"}
```

- 新会话续写前先读 `state.json` 与 `doc.xml`，不要凭记忆重写已有章节。
- 每轮可追加新 `<stage id="...">` 或顶层 `<section>`；**严禁修改或删除已发布 `<stages>`/`<stage>`/`<quiz>` 的 `id`**（读者浏览器进度按 `id` 持久化）。
- 第一轮仅有 1 个 `<stage>` 时合法且不显示步骤条与“继续”；追加第 2 个 `<stage>` 后自动激活分阶段步骤条。

---

## 3. 各场景撰写要点（PRD 2.4，默认参考）

- **对比与复现（B / G / H）**：目标数值与容差放在实测结果之前；缺失指标用 `<cell tag="unreported">`，不写 0、不估算；图读估值标 `tag="approx"`；多随机种子离散度用 `<chart>` 的 `series[].err`（`pie`/`scatter` 不可带 `err`）；公式与代码映射用 `<code-block eq="<equation-id>">`。
- **日报 / 周报（C）**：同一日重跑用同一 `--key` 覆盖不堆积；周报按天分 `<section>`，可每天配 `<tldr>` 并附整篇 `<tldr>`，用 `related-ids` 关联当周各日报，不抄正文。
- **研究报告（D）**：内部 `[@note-slug]` 引用在本地装配时转为 `<references>` + `[标签](#ref-<id>)` 锚点；只读到摘要/检索片段时在 `<ref>` 或 `<paper-card>` 标 `read-depth="abstract|snippet"`；检索过程用 `<search-log>`（纳入/排除标准、`query`、`excluded` 理由、`gap`、`stop`）。
- **新闻简报与详情（K / L）**：短消息用 `<news-item>` 内联 `<detail>`；需独立检索与分享的详情写成独立条目先推，取 `id` 后填入简报的 `detail-id`（详情可再用同 `--key` 更新 `related-ids` 指回简报）。无法核实的消息用 `<claim level="unverified">` 或“据称”。
- **长文档摘要（6.8）**：≥ 3 个部分或明显偏长时默认给全文 `<tldr>` 与每节 `<tldr>`（总结只概括正文已有内容，与 `meta/summary` 结论一致）；简短文档或用户另有指令时省略。
