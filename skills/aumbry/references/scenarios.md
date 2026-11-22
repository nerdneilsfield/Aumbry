# Aumbry 场景与积木参考（Scenarios & Drafting）

> **设计原则**：下表仅作“这类内容常用哪些积木”的搭配参考，**不是必填骨架**。结构完全由内容与用户意图决定，不用即删；表达不了先用 `<md>`。

## 1. 场景与 `<meta>` / 幂等键映射表

| 场景 | 来源 / Skill | `meta/type` | 推荐幂等键（`--key`） | 常用积木 | 参考示例 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **A 单篇论文精读** | `qi-read-paper` | `post` | `paper:<arxiv-id \| doi \| slug>` | `stages`/`stage`、`quiz`、`next-steps`、`tldr`、`claim`、`equation`、`figure`、`mermaid`、`chart`、`metrics`、`steps`、`glossary`、`compare-table`、`references`/`sources` | `paper-reading.xml` |
| **B 多方法对比** | `paper-compare` | `post` | `compare:<slug>` | `compare-table`（`cell@tag`）、`claim`、`callout type="warn"`、`chart` | `compare-repro.xml` |
| **C 日报/周报/简报** | 各日报 Agent、`steward-reports` | `report` | `<agent>:<YYYY-MM-DD>` 或 `<agent>:<YYYY>-W<ww>` | `tldr`（全文+分节）、`metrics`、`paper-card`、`repo-card`、`timeline`、`sources`、`related-ids` | `news-digest.xml`（周报结构） |
| **D 研究报告** | `research-report`（+ `paper-search`） | `post` | `report:<slug>` | `section`、`figure`、`mermaid`、`chart`、`references`、`compare-table`、`search-log` | — |
| **E 开源拆解** | 各拆解 Agent | `post` / `report` | `repo:<owner>/<name>` / `<agent>:<YYYY-MM-DD>` | `repo-card`、`mermaid`、`code-block`、`steps`、`checklist`、`metrics` | — |
| **F 周期汇报** | `steward-reports` | `report` | `steward:<YYYY-MM-DD>` / `steward:<YYYY>-W<ww>` | `section`（Facts/Results/Blockers/Evidence/Next steps）、`checklist`、`metrics`、`claim` | — |
| **G 复现 / 实验** | `paper-reproduce`、`algorithm-experiment` | `post` | `repro:<paper-slug>` / `exp:<slug>` | `compare-table`（`unreported`/`approx`）、`metrics`、`chart`（`series[].err`）、`checklist`、`claim`（`unverified`）、`stages` | `compare-repro.xml` |
| **内部基准 / 实验记录** | 性能基准 / 实验 Agent | `post` | `bench:<slug>` 或 `exp:<slug>`（如 `bench:driveflow-h100-fps`、`exp:flow-step-ablation`） | `md`、`metrics`、`chart`（多 series/`err`）、`compare-table`、`figure`、`claim` | `one-pager.xml`、`compare-repro.xml` |
| **H 论文到实现** | `paper-to-code` | `post` | `impl:<paper-slug>` | `glossary`、`code-block eq="..."`、`checklist`、`claim` | `compare-repro.xml` |
| **I 论文审计** | `paper-audit` | `post` | `audit:<paper-slug>` | 分 `section` 隔离自动检查与 LLM 判断、`metrics`、`checklist`、`claim` | — |
| **J 互动指南** | `interactive-guide` | `post` | `guide:<slug>` | `stages`、`quiz`、`steps interactive="true"`、`glossary`、`mermaid`、`next-steps`（仅发 XML，不推 HTML） | `paper-reading.xml` |
| **K 每日新闻简报** | 各新闻 Agent | `report` | `news:<YYYY-MM-DD>` | `tldr`、`news-item`（`detail-id` / `<detail>`）、`metrics`、`timeline`、`sources` | `news-digest.xml` |
| **L 新闻详情** | 各新闻 Agent | `post` | `news:<YYYY-MM-DD>:<slug>` | `tldr`、`section`、`quote`、`timeline`、`claim`、`references`/`sources`、`related-ids`（指回简报） | `news-detail.xml` |

> **幂等键与 `<meta>` 规则**：同题更新复用同一 `--key`（trim 后 `1..200` Unicode 字符，禁控制字符，超限 → `400 invalid_request`）。`<source-agent>` 填产出 agent/技能名（默认当前 agent 或 `aumbry-agent`）；`<type>` 见上表；`<summary>` 必填 1–2 句且与全文 `<tldr>` 一致；`<tags>`/`<canonical-url>` 无真实来源则不填。

## 2. 多轮增量草稿目录与配置速查

- **本地工作区**：单次短文直接用独立 XML + `imgs/`；多轮长文写入 `./aumbry-drafts/<key-safe>/`（`doc.xml`、`imgs/`、`state.json`），本地始终是唯一事实来源（不要回读服务端已重写图片 URL 的 XML）。新会话先读 `state.json` 与 `doc.xml`，续写时**严禁修改或删除已发布 `<stages>`/`<stage>`/`<quiz>` 的 `id`**。
- **`<key-safe>` 与 `state.json`**：非 `[A-Za-z0-9._-]` 字符替换为 `-`；`state.json` 初始为 `{"key":"paper:2611.01234","id":null,"full_url":null,"updated_at":"<date -u +%Y-%m-%dT%H:%M:%SZ>"}`。
- **目录与认证配合**：单一用途定时 agent 可用 `aumbry config set --profile <bot> ... --dir <id>` 或 `AUMBRY_DIRS` 固定目标目录；通用交互 agent 不设默认目录，发布前先按 [SKILL.md](../SKILL.md#发布目录决策与-cli-认证速查) 执行 `aumbry dirs` 匹配或向用户询问，禁止不带 `--dir` 裸推导致访客 `403`。

## 3. 各场景撰写要点

- **对比、基准与复现（B / G / H）**：目标值与容差放在实测前；缺值标 `<cell tag="unreported">`（不写 0、不估算），图读估值标 `tag="approx"`；多种子离散度放 `series[].err`（`pie`/`scatter` 禁用）；代码公式映射用 `<code-block eq="<equation-id>">`。
- **日报 / 周报（C）**：同日重跑复用同 `--key` 覆盖；周报按天分 `<section>`、每天一个 `<tldr>`，整篇再有一个全文 `<tldr>`（见 `news-digest.xml`），用 `related-ids` 关联当周日报，不抄正文。日报写法相同，把“按天分”换成“按主题分”。
- **研究报告（D）**：`[@note-slug]` 转为 `<references>` + `[标签](#ref-<id>)`；只读摘要/片段时在 `<ref>` 或 `<paper-card>` 标 `read-depth="abstract|snippet"`；检索过程用 `<search-log>`。
- **新闻简报与详情（K / L）**：短消息用 `<news-item>` 内联 `<detail>`；需独立检索与分享的详情先推取得 `id` 再填入 `detail-id`（详情可用同 `--key` 更新 `related-ids` 指回简报）；未核实标 `<claim level="unverified">` 或“据称”。
- **分层 `<tldr>`**：仅在文档 >= 3 部分或明显偏长时加全文及分节 `<tldr>`（只概括已有内容，对齐 `meta/summary`）；第 1 轮单 `<stage>` 或一页短文省略，第 1 轮单 stage 禁用指向未来 stage 的 `<next-steps>`。
