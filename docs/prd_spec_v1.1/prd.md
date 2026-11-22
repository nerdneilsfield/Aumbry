# Aumbry 产品需求与技术架构详细设计文档（PRD & Tech Spec v1.1）

| 文档属性     | 内容                                                         |
| :----------- | :----------------------------------------------------------- |
| **项目名称** | **Aumbry**（多 AI Agent 调研报告中枢）                       |
| **文档版本** | v1.1（在 v1.0 MVP 基线上的增量规格，未提及的部分一律沿用 v1.0） |
| **基线**     | `docs/prd_spec_v1.0/prd.md`，对应已合入 master 的 v1.0 实现 |
| **日期**     | 2026-11-21                                                   |
| **本版主题** | **让 Agent 把“和用户聊出来的内容”可靠地发布上来**：服务端配置文件、可信代理、GHCR 镜像发布、更丰富的学术/日报富文本、随仓库分发的 Agent Skill |

---

## 1. 背景与目标

### 1.1 v1.0 之后暴露的问题

1. **Agent 的真实工作流不是“一次性产出 XML”，而是多轮对话。** 例如 `qi-read-paper` 先讲第一轮（问题与整体认识），用户回复“继续”后讲第二轮（方法与证据），再到第三轮（局限、应用与复现）；`paper-compare` 要求每个单元格区分“作者主张 / 实测证据 / 推断”并标出“不可比”；`research-report` 先写笔记再写章节再组装。v1.0 的 XML 协议只有 9 个标签，承载不了这些结构，Agent 只能把它们压扁成 `<md>`，语义全丢。
2. **Agent 不知道怎么写、怎么发。** 标签约束、图片路径规则、幂等键约定、目录 ID 从哪来、校验报错怎么改，全靠 Prompt 里临时描述，换一个 Agent 就要重写一遍，并且和协议版本脱节。
3. **只能靠环境变量配置服务端。** 目录挂载、多个 Token、受信代理这类结构化配置用 env 表达很别扭，Docker/systemd 之外的部署也不友好。
4. **反向代理后限流退化成全站共用一个桶，`Secure` Cookie 的判断也盲信 `X-Forwarded-Proto`。** v1.0 的 README 只写了“怎么改代码”。
5. **只有二进制压缩包，没有可直接拉取的镜像。**

### 1.2 v1.1 目标

| # | 目标 | 一句话验收口径 |
| :- | :--- | :--- |
| G1 | **服务端配置文件** | `aumbry-server --config x.toml` 即可启动，env 仍可覆盖，旧的纯 env 部署不改动照常运行 |
| G2 | **可信代理（trusted proxies）** | 只信任配置的代理网段；其余来源的 `X-Forwarded-*` 一律忽略 |
| G3 | **GHCR 镜像发布** | 打 `v*` tag 后 GitHub Actions 自动推送多架构镜像到 `ghcr.io`；**不发 Docker Hub** |
| G4 | **更丰富的富文本** | 新增 `<claim>`、`<equation>`、`<mermaid>`、`<chart>`、`<metrics>`、`<steps>` 等标签，以及分阶段阅读 `<stages>`、互动 `<quiz>`/`<next-steps>`、摘要 `<tldr>`、新闻 `<news-item>`，覆盖论文精读/对比/复现/实现记录/日报/周报/新闻简报与详情/研究报告/学习路径 |
| G5 | **Agent 交互场景落地** | 支持“逐轮增量发布同一篇文档”“先存草稿再公开”“Agent 自己发现目录和协议” |
| G6 | **随仓库分发的 Agent Skill** | `skills/aumbry/` 教 Agent 怎么写 XML、怎么校验、怎么发布，示例可通过 `aumbry validate` |
| G7 | **读者侧体验** | 手机能用目录、标签可点击筛选（不越出已解锁子树）、可打印、锚点能跳进折叠内容（第 9 章） |
| G8 | **运维与安全加固** | 就绪探针、安全响应头、可靠备份、孤儿资源清理、WebDAV 超时、写入口限流（第 10 章） |

### 1.3 不做（Out of scope）

* MCP Server、外链图片自动转存（仍属 v2.0 路线图；`<chart>`/`<mermaid>` 已从 v2.0 前移到本版）。
* 多用户、命名/分级 API Token、Token 级目录权限。
* 配置热加载（改配置需重启进程）。
* 在 XML 中放原始 HTML、`<iframe>`、视频、任意 JS，或透传 ECharts 的原始 `option`。
* 镜像签名（cosign）、SBOM、Docker Hub 发布。
* 托管 `interactive-guide` 生成的整页 HTML（它自成一体，不走 Aumbry 渲染器）。
* 评论、协作编辑、UI 多语言。
* Prometheus `/metrics`、结构化审计日志、批量推送 `push --batch`、`push --replace-dirs`（见 14.2，需要时再加）。

---

## 2. Agent 交互场景（本版的设计依据）

本章是 G4/G5/G6 的需求来源：以用户已有的 skill（`qi-read-paper`、`paper-compare`、`research-report`、`paper-search`、`steward-reports`）为准，规定它们的产出怎样落到 Aumbry。Aumbry **不改动这些 skill 的流程**，只提供落点；发布动作由 `aumbry` skill（第 8 章）承接。

### 2.1 通用交互回路

```text
 内容 skill（qi-read-paper 等）产出内容
        │  用户说“存到 Aumbry / 发布 / 归档”，或此前已约定“每轮都存”
        ▼
 aumbry skill：参照场景示例、按内容自行组织结构，写/更新本地草稿 XML（+ 本地图片）
        ▼
 aumbry validate  ──失败──▶ 依据 JSON 报错就地修复，重新校验
        ▼ 通过
 aumbry push --key <幂等键> [--draft | --dir ...]
        ▼
 把 full_url 回复给用户；本地草稿保留，供下一轮继续修改
```

约束：**默认不自动发布**。只有用户明确要求，或会话里已约定“每轮都存”，Agent 才执行 push。

### 2.2 场景与落点

下表是**常见积木组合的参考**，不是必填结构（见 6.0）：同一类内容可以只用其中几个标签，也可以混用别的场景的标签。

| 场景 | 内容 skill | `type` | 幂等键约定 | 主要使用的标签 |
| :--- | :--- | :--- | :--- | :--- |
| A 单篇论文精读 | `qi-read-paper` | `post` | `paper:<arxiv-id 或 doi 或 slug>` | `stages`/`stage`（三轮）、`quiz`、`next-steps`、`claim`、`equation`、`figure`、`mermaid`、`chart`、`metrics`、`steps`、`glossary`、`compare-table`、`sources`/`references` |
| B 多方法对比 | `paper-compare` | `post` | `compare:<slug>` | `compare-table`（带 `tag`）、`claim`、`callout type=warn` |
| C 日报/周报/简报 | 各日报 Agent、`steward-reports` | `report` | `<agent>:<YYYY-MM-DD>` 或 `<agent>:<YYYY>-W<ww>` | `tldr`（全文与每个部分）、`metrics`、`paper-card`、`repo-card`、`timeline`、`sources`，`related-ids` 指向精读与当周各日报 |
| D 研究报告 | `research-report`（+ `paper-search` 的检索结论） | `post` | `report:<slug>` | 章节 → `section`；`figure`、`mermaid`、`chart`、`references`、`compare-table`、`search-log` |
| E 开源项目拆解 / vibe coding 日报 | 各拆解/日报 Agent | `post` / `report` | `repo:<owner>/<name>` / `<agent>:<YYYY-MM-DD>` | `repo-card`、`mermaid`、`code-block`、`steps`、`checklist`、`metrics` |
| F 周期汇报（事实/结果/阻塞/建议四分） | `steward-reports` | `report` | `steward:<YYYY-MM-DD>` / `steward:<YYYY>-W<ww>` | `section`（Facts/Results/Blockers/Evidence/Next steps）、`checklist`、`metrics`、`claim`（区分事实与建议） |
| G 论文复现 / 实验报告 | `paper-reproduce`、`algorithm-experiment` | `post` | `repro:<paper-slug>` / `exp:<slug>` | `compare-table`（目标/实测/容差/结论，含 `unreported`/`approx`）、`metrics`、`chart`（带 `err` 误差棒）、`checklist`（失配归因）、`claim`（含 `unverified`）、`stages` |
| H 论文到代码的实现记录 | `paper-to-code` | `post` | `impl:<paper-slug>` | `glossary`（符号表）、`code-block eq=`（公式↔代码映射）、`checklist`（偏差、未说明超参、论文遗漏的技巧）、`claim` |
| I 论文审计 | `paper-audit` | `post` | `audit:<paper-slug>` | `metrics`、`checklist`、`claim`（区分自动检查与 LLM 判断） |
| J 学习路径 / 互动指南 | `interactive-guide` | `post` | `guide:<slug>` | `stages`（入门→进阶→专家）、`quiz`、`steps interactive`、`glossary`、`mermaid`、`next-steps`；整页 HTML 不推，只发结构化版本 |
| K 每日新闻简报 | 各新闻 Agent | `report` | `news:<YYYY-MM-DD>` | `tldr`（全文 + 每个主题）、`news-item`（可带 `detail-id` 指向详情）、`metrics`、`timeline`、`sources` |
| L 新闻详情 | 各新闻 Agent | `post` | `news:<YYYY-MM-DD>:<slug>` | `tldr`、`section`、`quote`、`timeline`、`claim`、`sources`/`references`；`related-ids` 指回简报 |

`external_key` 沿用 v1.0 的 trim 语义（v1.0 已接受的键升级后仍可继续用于更新），v1.1 只额外要求 trim 后为 1–200 个字符且不含控制字符；不满足（含只传空白）→ `400 invalid_request`；约定见上表，不是服务端强制格式。

### 2.3 场景 A：论文精读的逐轮增量发布

`qi-read-paper` 一次只输出当前一轮。对应到 Aumbry：

1. **同一个 `--key` 反复 push 即可增量更新**（v1.0 的 upsert 语义，无需新接口）。Agent 在本地保留草稿目录（XML + `imgs/`），每轮在原文基础上追加该轮的 `<stage>`（或普通 `<section>`），整篇重新校验并重新推送。
2. **本地草稿目录约定（跨会话续写）**：每篇文档一个目录 `./aumbry-drafts/<key 的文件名安全形式>/`，内含 `doc.xml`、`imgs/` 与 `state.json`（记录 `key`、最近一次 push 返回的 `id`、`full_url`、时间）。第二轮可能隔天由新会话触发，新会话先读 `state.json` 再续写，**不要凭记忆重写已有章节**。
3. **每轮一个 `<stage>`**（v1.1，见 6.7）：把三轮放进 `<stages>`，`stage id` 取稳定的短名（如 `r1`/`r2`/`r3`，匹配 `[a-z0-9-]{1,32}` 即可），标题随内容自定（`qi-read-paper` 默认的三轮是“问题与整体认识”“方法与证据”“局限、应用与复现”，用户另有安排时以用户为准）。逐轮 push 时只追加新 stage，**已发布的 stage `id` 不改**，读者保存在浏览器里的阅读进度据此保留。每个 stage 末尾**可以**放 `<quiz>`（考察应用而非复述）和 `<next-steps>`（“继续：下一轮”“跳到某图/某公式”），对应 `qi-read-paper` 中用户回复“继续”或要求跳转、深入的交互。不用 `<stages>` 的文档仍可用顶层 `<section>`。用户要求跳转或合并轮次时，以用户指令为准，不强行套用。
4. **草稿语义**：`aumbry push --draft` 不挂载任何目录（忽略 `default_dirs`），条目只对管理员可见；对已存在条目，`--draft` 不改动其现有挂载。读完后再次 `push --key 同一个键 --dir <公开目录>` 即“发布”（挂载取并集，v1.0 行为）。
5. **证据语义落到标签**：论文位置（`§3.2`、`Fig. 4`、`Eq. (5)`、`Table 2`）写进 `<claim source="…">`；作者主张、论文证据、分析推断、待核验分别用 `level` 区分；未报告的信息写成 `level="unverified"` 的 claim，不留空。
6. **图表深读**：PDF 页面裁剪出的图片用 `<figure>`（本地相对路径，照旧由 `push` 打包），紧随其后的解读写在 `<md>` 中，引用图号用锚点 `#fig-<id>`（见 6.4）。

### 2.4 各场景的建议写法（参考，非强制；以 6.0 为准）

* **B 对比**：`<compare-table>` 的单元格可带 `tag="claim|evidence|inference|na|unreported|approx"`；`na` 表示“不可比”（灰底）；`unreported` 表示“论文未报告”，渲染为带虚线的“未报告”，**不写 0、不估值**（`paper-compare`：缺失数字不是零）；`approx` 表示数值从图中读取（“≈ 图读”角标）。表格之前先写目的，表格之后写 caveats，与 `paper-compare` 的输出顺序一致。
* **C 日报/周报**：同一天重复运行用同一个键覆盖更新，不堆积；周报键用 ISO 周。日报里引用的精读通过 `meta/related-ids` 和 `paper-card related-id` 关联。**周报通常按天分 `section`，每天可有一个 `<tldr>`，整篇再有一个全文 `<tldr>`；当周各日报用 `related-ids` 关联**，周报正文不重抄日报。
* **D 研究报告**：章节文件在本地合并成一个 XML；`research-report` 的 `[@note-slug]` 引用由 Agent 在装配阶段替换为 `<references>` + 正文中的 `[标签](#ref-<id>)` 锚点链接（见 6.4），**服务端不解析 `[@…]`，也不新增行内引用语法**。章节 Markdown 可原样放进 `<md>`，其中的 Mermaid 与图片要提出为 `<mermaid>`/`<figure>`。只读到摘要或检索片段的来源用 `<ref read-depth="abstract|snippet">` 标明；检索过程用 `<search-log>`（含纳入/排除标准、每条检索式、被排除项及理由、缺口与停止规则）。
* **G 复现/实验**：目标数值、指标定义与**容差应写在结果之前**（`paper-reproduce`），常用 `compare-table` 逐条列出“主张 / 指标定义 / 数据集与划分 / 论文值 / 容差 / 实测 / 结论”（列可按实验需要增减），结论取 复现 / 未复现 / 无法量化 / 环境差异；多随机种子的结果用 `chart` 的 `err` 画误差棒，不只给最优种子（`algorithm-experiment`）；失配归因（环境、指标定义、实现、方差）用 `checklist status="risk|blocked"` 逐项记录。
* **H 实现记录**：符号表用 `glossary`；公式与代码的对应用 `<code-block eq="公式id">`；偏差、未说明的超参数、论文没写的技巧各用一个 `checklist` 分组记录，每条带来源（作者代码 / 自己发现）。
* **I 审计**：自动检查结果与 LLM 判断分开呈现（不同 section），不混合打分。
* **J 学习路径**：每个 `stage` 通常以 `quiz` 收尾，术语先在 `glossary` 或正文里引入再出现在题目里；`steps interactive="true"` 用于需要一步一步看的流程。
* **K/L 新闻简报与详情**：简报通常按主题分 `section`，每个主题一个 `<tldr>`，条目用 `<news-item>`（建议带 `source` 与 `published`，协议只要求 `title`）；短消息可用内联 `<detail>` 展开，需要整篇展开的写成**独立的详情条目**（场景 L），简报里用 `detail-id` 链接。顺序：**先推详情、取得各自 `id`，再推简报**（`detail-id` 只能用 `push` 返回的或 `aumbry list --key` 查到的 id）；详情稿可在简报推送后用同一个键再推一次，把简报 `id` 补进它的 `related-ids`（可选）。未证实的消息用 `<claim level="unverified">` 或在正文标明“据称”，不写成事实。
* **长文档（分成多个部分的内容）**：默认给全文一个 `<tldr>`、每个 `section`/`stage` 一个 `<tldr>`；内容不适合（纯日志、单条短讯）或用户另有要求时可以省略。总结只概括正文已有内容，不引入新事实（见 6.8）。

---

## 3. 服务端配置文件（G1）

### 3.1 行为

* 配置格式为 **TOML**（服务端配置；CLI 的 `~/.config/aumbry/config.yaml` 是 v1.0 已发布的格式，本版不改，见 14.1）。
* **查找顺序**（命中即停）：`--config <path>` → 环境变量 `AUMBRY_CONFIG` → `./aumbry.toml` → `/etc/aumbry/config.toml`。显式指定（`--config` 或 `AUMBRY_CONFIG`）而文件不存在 → 启动失败；隐式查找都不存在 → 视为没有配置文件，继续走纯 env。
* **优先级**：环境变量 > 配置文件 > 内置默认值。环境变量名与 v1.0 完全一致，**纯 env 的部署无需任何改动**。
* **严格解析**：TOML 中出现未知键 → 启动失败并指出键名（防止拼写错误被静默忽略）。
* **校验规则沿用 v1.0**：必填项缺失时一次性列出全部缺失项（同时给出 TOML 键和 env 名）；`cookie_secret` 不少于 32 字节；`admin.password_hash` 必须是合法 bcrypt。
* **权限提示**：配置文件含密钥，若文件对 group/other 可读，启动时打印一条警告日志（不阻止启动）。
* **子命令**：`aumbry-server check-config [--config path]` 校验并打印**生效配置**（密钥与密码掩码，仅保留末 4 位），无效时退出码为 1；不启动监听。

### 3.2 键与环境变量对照

| TOML 键 | 环境变量 | 必填 | 默认 | 说明 |
| :--- | :--- | :--- | :--- | :--- |
| `listen_addr` | `AUMBRY_LISTEN_ADDR` | 否 | `:8080` | 监听地址 |
| `data_dir` | `AUMBRY_DATA_DIR` | 是 | – | SQLite 数据目录 |
| `admin.user` | `AUMBRY_ADMIN_USER` | 是 | – | 管理员用户名 |
| `admin.password_hash` | `AUMBRY_ADMIN_PASSWORD_HASH` | 是 | – | bcrypt 哈希 |
| `api_tokens` | `AUMBRY_API_TOKENS` | 是 | – | TOML 为数组；env 为逗号分隔 |
| `cookie_secret` | `AUMBRY_COOKIE_SECRET` | 是 | – | ≥ 32 字节 |
| `webdav.url` | `AUMBRY_WEBDAV_URL` | 是 | – | 私有 WebDAV 根地址 |
| `webdav.user` | `AUMBRY_WEBDAV_USER` | 是 | – | |
| `webdav.password` | `AUMBRY_WEBDAV_PASS` | 是 | – | |
| `trusted_proxies` | `AUMBRY_TRUSTED_PROXIES` | 否 | 空 | 见第 4 章；TOML 为数组，env 为逗号分隔 |

表中 `admin.user` 这类带点的键在 TOML 里是表（table）：

```toml
listen_addr = ":8080"
data_dir = "/data"
api_tokens = ["amb_live_xxxxxxxx"]
cookie_secret = "至少 32 字节的随机串"
trusted_proxies = ["10.0.0.0/8"]

[admin]
user = "admin"
password_hash = "$2y$10$..."   # bcrypt；TOML 的双引号字符串里 `$` 不需要转义

[webdav]
url = "https://dav.example.com/aumbry"
user = "aumbry"
password = "..."
```

仓库提供 `config.example.toml`（全部键、注释、占位值，无真实密钥）。`docker-compose.yml` 增加“挂载配置文件”的示例，env 方式保留。

### 3.3 实现要求

按“有成熟库就用库”的约束，使用 `pelletier/go-toml/v2` 的 `DisallowUnknownFields` 严格解码，再用仓库已在用的 `caarlos0/env/v11` 做 env 覆盖（只把实际设置过的变量放进 `Environment`），**不新增 `koanf`**（新增依赖且默认 `_`→`.` 的键映射与现有变量名不同构，反而要写映射）。注意：当前 `Config` 里的 `envDefault:":8080"` 会覆盖 TOML 里的 `listen_addr`，实现时要把默认值移出 struct tag、在合并之后再套用。行为以 3.1 为准。`aumbry-server` 目前没有任何命令行参数，本版用与 CLI 一致的 `cobra` 引入：**无子命令时默认启动服务**（因此镜像里 `ENTRYPOINT` 为 `aumbry-server` 不变，`docker run IMAGE check-config` 即可执行校验子命令），并提供 `--config`。第 10 章的 `backup`、`verify-storage`、`gc`、`reindex` 也是该二进制的子命令。`docker`/`systemd` 的 secrets 文件间接引用（`*_FILE`）不在本版（见 14.2）。

**合并与校验的顺序**：先读 TOML，再用**显式设置了的**环境变量逐键覆盖，**合并之后**才统一校验必填项（不能沿用“每个变量都必须在 env 里出现”的 v1.0 加载逻辑，否则 TOML 里已填的 `data_dir` 也会被报缺失）。`check-config` 只做配置的加载与校验，不打开数据库、不执行迁移、不连 WebDAV。

**镜像里的 env 默认值**：v1.0 的 `Dockerfile` 内置了 `AUMBRY_LISTEN_ADDR` 与 `AUMBRY_DATA_DIR`，它们会覆盖 TOML 里的同名键。官方镜像（`Dockerfile.release`）**不再内置任何 `AUMBRY_*` 环境变量**：`listen_addr` 的默认值 `:8080` 由程序内置，`data_dir` 仍是必填项（TOML 或 env 提供）。`docker-compose.yml` 提供两个可独立运行的示例：只挂 TOML（不需要 `.env`），以及纯 env。

---

## 4. 可信代理（G2）

### 4.1 行为

* `trusted_proxies`：IP 或 CIDR 列表，默认**空，即不信任任何转发头**（与 v1.0 行为一致）。
* 当且仅当**直连对端地址（RemoteAddr）命中 `trusted_proxies`** 时：
  * 客户端 IP 取 `X-Forwarded-For` 中**从右向左第一个不在 `trusted_proxies` 内的地址**（防止客户端自带 XFF 伪造）；没有 XFF 则回退为 RemoteAddr。
  * `X-Forwarded-Proto: https` 才会让 Cookie 带上 `Secure`。
* 来源不在 `trusted_proxies` 内的请求，上述两个头一律忽略。
* 生效范围：限流的 IP 键、`Secure` Cookie 判断、请求日志中的客户端地址。
* 不新增“信任哪个头”的开关：只认 `X-Forwarded-For` / `X-Forwarded-Proto`。使用 Cloudflare 等需要其它头的场景见 14.2。

### 4.2 实现要求

客户端 IP 解析使用已在依赖中的 `chi` 的 `middleware.ClientIPFromXFF(trustedPrefixes...)`，但**必须先校验直连对端**：仓库锁定的 chi v5.3.0 的该中间件只遍历 `X-Forwarded-For`，**不检查 `RemoteAddr`**，直接套用时，不可信的直连客户端自带的 XFF 也会生效。实现二选一，并以验收 7–10 为准：① 升级到其实现会校验 TCP 对端的 chi 版本（实现前读该版本源码核实）；② 在其外面包一层按 `RemoteAddr` 分流的小判断（命中 `trusted_proxies` 才走 XFF 中间件，否则 `ClientIPFromRemoteAddr`）。其余要求：`trusted_proxies` 为空时走 `ClientIPFromRemoteAddr`；单个 IP 视为 `/32` 或 `/128`；非法 CIDR 在启动时报错并指出位置（chi 对非法前缀是 panic，不能让 panic 当作配置错误提示）；XFF 缺失或解析失败时回退为 `RemoteAddr`。`Secure` 判断改为读取同一份“可信来源”判定，不再在 `auth` 包里直接读头；请求日志的客户端地址也显式取 `GetClientIP`（chi 不改写 `RemoteAddr`）。

### 4.3 修复的 v1.0 缺陷

* 反代后所有人共用一个限流桶（一个人 1 分钟内失败 10 次即全站 429）。
* 直连 HTTP 时客户端可用自带的 `X-Forwarded-Proto: https` 影响 `Secure` 标记。

---

## 5. GHCR 镜像发布（G3）

### 5.1 产物

* 镜像名：`ghcr.io/nerdneilsfield/aumbry`（owner 小写）。
* 架构：`linux/amd64`、`linux/arm64`。
* 内容：`aumbry-server`（`ENTRYPOINT`）与 `aumbry`（CLI，便于 `docker exec`），基础镜像 `gcr.io/distroless/static-debian13:nonroot`，沿用 v1.0 的 `/data` 目录、`USER nonroot`、`EXPOSE 8080`；**不内置 `AUMBRY_*` 环境变量**（见 3.3），部署时由 TOML 或 env 显式给出 `data_dir`。
* 标签：
  * 正式版 `v1.2.3` → `1.2.3`、`1.2`、`1`、`latest`。
  * 预发布 `v1.2.3-rc.1` → 仅 `1.2.3-rc.1`（不更新 `1.2`、`1`、`latest`）。
* OCI 标准 label/annotation：`source`、`version`、`revision`、`created`、`licenses`、`description`。
* **不推送 Docker Hub**，不新增任何 Docker Hub 凭据。

### 5.2 实现要求

* 使用 GoReleaser 自带的 `dockers_v2`（多架构由 buildx 完成，无需手写 manifest）；仓库已固定 GoReleaser `v2.19.0`，满足该特性。
* 镜像使用 GoReleaser 已构建好的二进制，由独立的 `Dockerfile.release`（只做 `COPY` 与运行时设置）构建，保证 Release 里的二进制与镜像内完全一致；根目录原有的多阶段 `Dockerfile` 保留，用于本地 `docker compose build`。
* GoReleaser 配置要点（直接照默认值会改变产物）：`dockers_v2` 的 `ids` 指定 `[cli, server]`，`Dockerfile.release` 按 `${TARGETPLATFORM}` 子目录复制两个二进制；`/data` 目录属主为 UID 65532 且可写；显式关闭 `sboms`（与 1.3 不做 SBOM 一致）；浮动标签用 `{{ if not .Prerelease }}` 控制，避免 RC 误更新 `latest`/`1.2`/`1`；给 `archives` 设置 `files` 后，原先默认收入的 `LICENSE`、`README*` 必须显式保留，再加 `skills/aumbry/**/*`。
* `release.yml`：权限增加 `packages: write`；在 GoReleaser 之前加 `docker/setup-qemu-action`、`docker/setup-buildx-action`、`docker/login-action`（registry 为 `ghcr.io`，用 `GITHUB_TOKEN`）；所有 action 沿用仓库现有的“固定 commit SHA”写法。
* GitHub Release 的压缩包照旧发布，并**额外把 `skills/aumbry/` 通过 `archives[].files` 打进压缩包**（第 8 章；现有归档同时含 `cli` 与 `server` 两个构建，skill 目录只打一份，随该归档分发）。
* 首次发布后，需在 GitHub 把该 package 的可见性手动设为 public（一次性，写入 README 的发布说明）。

---

## 6. XML 协议 v1.1：更多富文本（G4）

### 6.0 设计原则：积木，不是模板（结构由内容决定）

不同的报告结构不同：精读、对比、日报、新闻、复现记录、研究报告、一页结论……**协议只提供可自由组合的块，不规定“某类文档必须长什么样”。**

* **所有 v1.1 新标签都是可选积木**：校验只检查“结构合法与安全”（`id` 唯一、URL 白名单、大小与数量上限、容器嵌套规则、必填属性），**不要求**任何固定章节、固定标题、固定顺序或最少数量；`tldr`、`stages`、`metrics`、`claim` 等没有一个是必填的。
* **放宽 v1.0 的容器限制（仅 `version="1.1"`）**：`body`、`section`、`stage` 内都可以直接放任意块级标签（不必先包一层 `section`；只含一个 `<md>` 的 `body` 是合法文档）；`section` 可嵌套至 3 层（超过 → `SECTION_TOO_DEEP`），章节标题是自由文本。v1.0 文档仍按 v1.0 原规则校验。下文 6.2 表“位置”列写作“`section` 内”的，一律读作“`body`/`section`/`stage` 内”。
* **场景表（2.2）与 `skills/aumbry/assets/examples/` 只是“这类内容常用哪些积木”的参考**，不是必填结构；用户的要求与内容本身优先于示例。服务端不引入“场景类型”字段，`type` 仍只有 `report`/`post`，也不按场景分流。
* **不用的积木不要硬塞**：没有可量化的指标就不放 `metrics`，内容很短就不加 `tldr`，没有分阶段就不用 `stages`。
* 逃生口：`<md>` 可承载任意 Markdown（含 6.4 的锚点链接、脚注、任务列表），表达不了的结构先用 `<md>`，而不是新增标签。

### 6.1 版本与兼容

* `<document version>` 接受 `1.0` 与 `1.1`。v1.0 文档在 v1.1 服务端上**原样可用**。
* 本章新增的标签与属性（含 `<figure id/alt>`、`<cell tag>`、`<paper-card>`/`<repo-card>` 的新增属性）**只允许出现在 `version="1.1"` 的文档中**；在 `1.0` 文档里使用 → `TAG_REQUIRES_1_1`，提示把版本改为 `1.1`。这样旧技能/旧示例写出的文档不会悄悄混用新标签。
* 其余校验规则、错误结构、CLI/服务端共用 `pkg/schema` 的约定不变。
* **兼容边界**：已入库文档保持可读，索引无需重建；重新上传的 `version="1.0"` 文档按“v1.0 规则 + 新标签/新属性门控”校验，因此可能拒绝 v1.0 服务端当年宽松接受的个别用法（如 1.0 文档里带 `alt` 的 `<figure>`），错误带 `hint` 提示把 `version` 改为 `1.1`，这是有意取舍。6.5 的索引文本投影只对 `version="1.1"` 的新入库文档生效。**反向**：v1.1 CLI 向 v1.0 服务端推 `version="1.1"` 文档，会被其 `UNSUPPORTED_VERSION` 拒绝（HTTP 422，不写对象与数据库），CLI 提示升级服务端。

### 6.2 新增标签一览

优先级：**P0** 为场景 A/B/C 的必需项；**P1** 为场景 D 与体验增强；**P2** 视进度，可顺延到 v1.2。

| 标签 | 位置 | 必填 / 属性 | 校验与渲染 | 优先级 |
| :--- | :--- | :--- | :--- | :--- |
| `<claim>` | `section`/`collapse`/`tab`/`step` 内 | `level` ∈ `claim`(作者主张) / `evidence`(论文证据) / `inference`(分析推断) / `unverified`(待核验)；`source` 可选（如 `§3.2`、`Fig. 4`、`Table 2`）；正文 Markdown | 带色条与等级徽章，`source` 显示为位置标签 | P0 |
| `<compare-table>` 的 `<cell tag>` | 既有标签的扩展 | `tag` ∈ `claim`/`evidence`/`inference`/`na`/`unreported`/`approx`，可选 | `na` 渲染为灰底“不可比”；`unreported` 为虚线“未报告”（不显示 0）；`approx` 为“≈ 图读”角标；其余渲染为角标 | P0 |
| `<equation>` | `section`/`collapse`/`tab`/`step` 内 | LaTeX 文本；`id` 可选；`number` 可选（显示为 `(5)`）；`caption` 可选 | KaTeX displayMode，`throwOnError:false`、`trust:false`，并设置 `maxSize` 与有界的 `maxExpand`，公式容器内自行横向滚动（单个很短的公式也能指定巨大尺寸而破坏布局）；非空、≤ 4 KB | P0 |
| `<mermaid>` | 同上 | Mermaid 源码（CDATA）；`caption` 可选 | 非空、≤ 20 KB；前端按需懒加载，`securityLevel:"strict"`；渲染失败时回退显示源码并提示错误 | P0 |
| `<chart>` | 同上 | `type` ∈ `bar`/`line`/`pie`/`scatter`；`caption` 可选；正文为 JSON（CDATA），见 6.3 | 服务端/CLI 用 JSON Schema 校验；前端用 ECharts（按需引入） | P0 |
| `<metrics>` / `<metric>` | `section` 内 | `<metrics>` 含 1–12 个 `<metric label value>`；`unit`、`delta`、`trend`(`up`/`down`/`flat`) 可选 | 指标卡片网格 | P0 |
| `<steps>` / `<step>` | `section`/`stage` 内 | `<step title>` ≥ 1 个；内部可含 `md`/`figure`/`callout`/`claim`/`equation`；`interactive="true"` 可选（要求 ≥ 2 个 step） | 默认为带编号的纵向步骤（复现路径）；`interactive` 时一次显示一步，带上一步/下一步、步骤点与键盘左右键，打印时全部展开 | P0 / P1 |
| `<section>` 的 `id` | 既有标签的扩展 | 可选，`[a-z0-9-]{1,32}`，文档内全局唯一；锚点 `#s-<id>` | 让 `next-steps`、`<md>` 链接与 TOC 能稳定指向章节。无 `id` 的章节仍有按位置生成的 `#sec-N`，仅供 TOC 与滚动，**不是**合法的 `next-steps` 目标（追加内容会让位置变化） | P1 |
| `<tabs>` / `<tab>` | `section` 内 | `<tab title>` ≥ 2 个且标题不重复；内部同 `step` | Reka UI Tabs；全部 Tab 内容参与全文索引 | P1 |
| `<timeline>` / `<event>` | `section` 内 | `<event date title>`；`date` 形如 `YYYY`、`YYYY-MM` 或 `YYYY-MM-DD`；正文 Markdown | 纵向时间线 | P1 |
| `<glossary>` / `<term>` | `body` 或 `section` 内 | `<term name>` 正文为释义（Markdown 行内）；`name` 不重复 | 定义列表 | P1 |
| `<references>` / `<ref>` | `body` 或 `section` 内 | `<ref id title>` 必填；`authors`、`year`、`venue`、`url`、`doi` 可选；`id` 在文档内唯一 | 编号参考文献，锚点 `#ref-<id>`；`url` 只允许 `http/https` | P1 |
| `<figure-group>` | `section` 内 | `cols` ∈ `2`/`3`；2–6 个 `<figure>`；`caption` 可选 | 多子图并排（论文 (a)(b)(c) 子图） | P1 |
| `<code-block>` | `section`/`collapse`/`tab`/`step` 内 | 代码（CDATA，≤ 32 KB）；`lang`、`file`、`lines`（如 `66-78`）、`commit`、`caption`、`eq`（指向某个 `<equation id>`，显示“对应公式 (n)”并跳转；不存在 → `INVALID_TARGET`）、`highlight`（高亮行，如 `3,5-7`，相对代码块首行）均可选；`diff="true"` 时按 `+`/`-` 着色 | 在 `shiki` 基础上：`diff="true"` 的 unified diff（行首 `+`/`-`）用 shiki 内置的 `diff` 语言着色（仓库已加载 `shiki/langs/diff`）；行高亮用 `@shikijs/transformers`（其 notation 指令只识别 `[!code ++]` 之类注释，不能当 unified diff 用，实现时以实际渲染验证为准），带复制按钮；只索引 `file`/`caption`，不索引代码正文 | P1 |
| `<quote>` | `section`/`collapse`/`tab`/`step` 内 | `source`（如 `§4.1, p.6`）、`author`、`url` 可选；正文 Markdown | 与分析性文字样式区分的“逐字摘录”；`url` 只允许 `http/https`；正文与 `source` 进索引 | P1 |
| `<checklist>` / `<check>` | `section` 内 | `<check status>`，`status` ∈ `done`/`todo`/`blocked`/`risk`，`detail` 可选，正文为一行说明 | 状态图标与徽章，只读展示，不落库；进索引 | P1 |
| `<search-log>` | `section`/`collapse` 内 | `date` 可选；`<criteria>` 含 `<include>`/`<exclude>`；≥ 1 个 `<query source date hits kept>`（正文为检索式）；0..n 个 `<excluded title reason>`；0..n 个 `<gap>`；至多 1 个 `<stop>`（停止规则，如“连续两轮无新增”） | 默认折叠的检索审计卡片，标题栏显示检索式条数/纳入数（`paper-search` 的产出） | P1 |
| `<stages>` / `<stage>` | `body`/`section` 内的块；**一份文档可有多组 `<stages>`**，可放在文档中间或某个主题的 `section` 里，但不能出现在任何 `stage` 内部 | `<stages id>` 必填；含 1–8 个 `<stage id title>`（只有 1 个时不显示步骤条与“继续”，逐轮发布的第一轮即此情形），`level` 可选（如“入门/进阶/专家”“第一轮”）；`gated="false"` 可选；`stage` 内可含任意块级标签与 `section`。每组独立门控与记录进度 | 分阶段阅读：该组开头的步骤条与进度，后续阶段默认折叠，“继续”后展开；详见 6.7 | P1 |
| `<quiz>` | `body`/`section`/`stage` 内 | `id` 必填；`<question>` 一个，`<option>` 2–8 个，其中 `correct="true"` 的有 ≥ 1 个（1 个为单选，多个为多选）；`<explain>` 可选 | 提交后逐项标对错并显示解析，可重答；得分按阶段汇总；状态只存浏览器，不落库；答案在 XML 中对读者可见，**不是保密机制** | P1 |
| `<next-steps>` / `<option>` | `body`/`section`/`stage` 内 | 1–5 个 `<option target="#锚点">`，正文为按钮文字 | 一排选择按钮（“继续：下一轮”“跳到某图”“深入某公式”）；`target` 必须是本文档内已存在的 `id`，不存在 → `INVALID_TARGET` | P1 |
| `<tldr>` | `body` 内任意位置（全文总结，至多 1 个）；或 `section`/`stage` 内（本部分小结，每个至多 1 个） | `title` 可选（默认“全文要点”/“本节要点”）；`placement` ∈ `top`/`bottom`（默认 `top`；`body` 级的全文总结默认显示在正文首屏）；含 1–7 个 `<point>`（一行 Markdown 行内文本，≤ 300 个 Unicode 字符，按 rune 计数而非字节）与 0–1 个 `<verdict>`（一句结论或建议，≤ 300 个 Unicode 字符）；违规 → `INVALID_TLDR` | 要点卡片；章节 TOC 与移动端抽屉显示各节首条要点；是“速读模式”的数据来源（9.7）；进全文索引；要点按行内 Markdown 渲染，TOC 与抽屉取纯文本 | P1 |
| `<news-item>` | `body`/`section`/`stage` 内 | `title` 必填；`source`、`url`（`http/https`）、`published`（`YYYY-MM-DD` 或带时间的 RFC 3339）、`importance` ∈ `high`/`normal`（默认）、`detail-id`（12 位条目 ID，格式同 `related-id`）；正文为 Markdown 行内摘要（≤ 600 个 Unicode 字符）；可含 0–1 个 `<detail>`（Markdown，≤ 8 KB）。缺 `title` → `INVALID_NEWS`；`url` 非法 → `INVALID_URL`；`detail-id` 格式错 → `INVALID_ID` | 新闻卡片；`high` 带强调色条；`<detail>` 点击内联展开；`detail-id` 渲染为“阅读详情 →”，跳转到该条目的阅读页，目标条目的访问仍由目录权限决定 | P1 |

**既有标签的 v1.1 扩展属性**：`<paper-card>` 增加可选 `venue`、`doi`、`code-url`、`read-depth`（`full`/`abstract`/`snippet`，对应 `paper-search`/`research-report` 要求标明“全文精读 / 仅读摘要”）；`<repo-card>` 增加可选 `license`、`commit`。`code-url` 只允许 `http/https`；`doi` **不是 URL**，按 `^10\.\d{4,9}/[-._;()/:A-Za-z0-9]+$` 校验（失败 `INVALID_DOI`），渲染时拼成 `https://doi.org/<doi>`。

**容器与允许的子块（校验器与示例共用同一份定义）**：`body`、`section`、`stage` 内可放全部块级标签（见 6.0）；`collapse`、`tab`、`step` 内只允许叶子内容块：`md`、`figure`、`callout`、`claim`、`equation`、`mermaid`、`chart`、`code-block`、`quote`（`collapse` 沿用 v1.0 时只含 `md`/`figure`，其余在 `version="1.1"` 下放开）。`tabs` 只含 `tab`，`steps` 只含 `step`，`figure-group` 只含 `figure`。`stages` 只含 `stage`；一份文档可有多组 `stages`（至多 8 组、全文 `stage` 合计至多 40 个，超出 → `TOO_MANY_BLOCKS`），每个 `stages` 必须有 `id`；`stages` 不能出现在任何 `stage` 的后代中（即不嵌套），每组 1–8 个 `stage`，否则 `INVALID_STAGES`；`stage` 内可放 `section` 允许的全部块级标签以及 `section`；`stages`、`stage`、`quiz` 的 `id` 与 `#stages-<id>`/`#stage-<id>` 锚点遵守同一套“文档内全局唯一”规则（不同组里的 `stage` 也不能重名）。`tldr` 只能是 `body`、`section`、`stage` 的子元素：`body` 级至多 1 个，`section`/`stage` 级各至多 1 个（否则 `INVALID_TLDR`）；

`<figure>` 增加可选属性 `id`（用于 `#fig-<id>` 锚点）与 `alt`（无障碍；建议填写；缺省时回退为 `caption`）。**图片收集、路径校验与重写覆盖所有可含 `<figure>` 的容器**（`section`、`collapse`、`tab`、`step`、`figure-group`），实现上以 `//figure[@src]` 为准，不再按父子路径枚举。`<equation>`、`<ref>` 的 `id` 同理生成 `#eq-<id>`、`#ref-<id>`。`<equation>` 与 `<mermaid>` 在 Go 侧只校验非空与长度（空 → `INVALID_EQUATION`/`INVALID_MERMAID`，超长同码）。**`<sources>` 与 `<references>` 的分工**：`<sources>` 是日报/简报末尾的外链汇总，不被正文引用；`<references>` 是学术正文的参考文献，带 `id` 并被 `#ref-<id>` 锚点引用。`id` 取值限 `[a-z0-9-]{1,32}`，**文档内全局唯一**，重复 → `DUPLICATE_ID`。

### 6.3 `<chart>` 数据格式

不透传 ECharts 原始 `option`（避免任意配置膨胀和潜在的代码型配置），只接受下面这个最小结构：

```json
{
  "categories": ["DriveFlow", "Vista-v2"],
  "series": [{ "name": "推理帧率 (FPS)", "data": [30, 8] }],
  "x_label": "方案",
  "y_label": "FPS"
}
```

* `bar`/`line`：**必填** `categories`（非空）与 `series`（1–20 个，每个有 `name`），每个 `series[].data` 是数字数组且长度等于 `categories`；可选 `series[].err`（**P2，列入 M5**；首发可先用 `metrics`/`compare-table` 表达均值与离散程度；非负有限数，长度与 `data` 相同，表示对称误差，如多随机种子的标准差），前端用 ECharts 官方的 custom 系列误差棒方案绘制，不手写绘图（`scatter`/`pie` 不允许 `err`）。
* `pie`：**必填** `categories`（扇区名，非空）与**恰好 1 个** `series`（多于 1 个 → 拒绝，不取“第一个”），`data` 长度等于 `categories`。
* `scatter`：**不允许** `categories`；**必填** `series`（1–20 个，每个有 `name`），`data` 为 `[[x, y], …]`。
* 限制：`series` ≤ 20，数据点总数 ≤ 2000，数值必须有限（不含 NaN/Infinity）。
* 结构与类型校验使用嵌入的 JSON Schema 与成熟的 JSON Schema 库（如 `santhosh-tekuri/jsonschema`）。**JSON Schema 无法表达“`data` 长度等于 `categories` 长度”这类跨字段约束**，这部分（等长、点数上限、数值有限、`pie` 只允许一个 series、`categories` 非空、`series[].name` 必填）在 Go 中补充检查；全部报 `INVALID_CHART` 并带 JSON 路径。

### 6.4 引用与锚点（不新增行内语法）

行内引用采用普通 Markdown 锚点链接：`见 [Fig. 4](#fig-arch)`、`如 [Smith 2024](#ref-smith24)`、`由式 [(5)](#eq-ssm) 可得`。前端的 URL 白名单已允许同页 `#` 锚点。`<md>` 块的 Markdown 增强统一使用 markdown-it 现成插件：脚注（`markdown-it-footnote`）、任务列表（`markdown-it-task-lists`，只读复选框）、上标/下标（`markdown-it-sub`、`markdown-it-sup`）。仍然 `html:false`。脚注插件为每个 `<md>` 块使用不同的 `docId`（`env.docId`），否则两个块里的同名脚注会生成相同锚点而互相跳错。**锚点命中隐藏内容时的行为**：`#fig-*`、`#eq-*`、`#ref-*`、章节锚点指向收起的 `<collapse>` 或未激活的 `<tab>` 内时，前端先展开/切换再滚动定位，不能让原生 hash 跳转落空。


**锚点映射与统一的 `id` 命名空间**：XML 里所有 `id`（`stages`、`stage`、`quiz`、`figure`、`equation`、`ref`、`section`）共用**一个**文档内命名空间，重复 → `DUPLICATE_ID`（包括 `stage id="x"` 与 `figure id="x"` 这类跨类型重名）。渲染后的锚点按类型加前缀：

| 元素 | 锚点 | 可作为 `next-steps` 的 `target` |
| :--- | :--- | :--- |
| `section id` | `#s-<id>` | 是 |
| `stages id` | `#stages-<id>` | 是 |
| `stage id` | `#stage-<id>` | 是 |
| `quiz id` | `#quiz-<id>` | 是 |
| `figure id` | `#fig-<id>` | 是 |
| `equation id` | `#eq-<id>` | 是 |
| `ref id` | `#ref-<id>` | 是 |
| 无 `id` 的章节 | `#sec-<N>`（按位置） | 否 |

`next-steps/option@target` 必须形如 `^#[a-z0-9-]+$`，并按上表前缀解析到文档内已存在的元素，否则 `INVALID_TARGET`。前端用 `<button type="button">` 触发“定位”，**不使用 `<a href>`，也不经 `safeUrl`**（现有 `safeUrl` 本来就放行外链与同源路径，不能当作“只允许页内锚点”的校验）。`<code-block eq>` 写的是 `equation` 的**原始 `id`**（不带 `#`、不带前缀），不存在 → `INVALID_TARGET`。

**定位顺序**（深链接、搜索结果跳转、`next-steps`、`<md>` 内的锚点链接共用同一套）：① 把目标所属的 `stages` 组解锁到目标阶段；② 依次展开祖先 `collapse`、切换 `tab`、交互式 `steps` 到目标步；③ 退出速读对目标所在节的折叠（9.7）；④ 等目标挂载后再滚动。实现上在解析 XML 时一次性建立 `anchorId → { stagesId, stageId, tabIds, collapseIds, sectionId }` 的映射表，由阅读页集中驱动这些受控状态（`provide/inject`），不让各容器组件各自监听 `hash`；打印时的全挂载复用 Reka UI 的 `forceMount`，不手写切换 DOM。

### 6.5 全文索引与安全

* **进入索引**：既有文本与 `title/caption/name/authors` 属性；新增的 `claim` 正文、`metric` 的 `label/value`、`term` 的 `name` 与释义、`ref` 的 `title/authors`、`step/tab/event/stage` 的 `title`、`quiz` 的 `question`/`explain`（选项与正误不入索引）、`search-log` 的 `query`、`criteria`、`gap` 与 `excluded`（含属性 `reason`）、`quote` 的 `source`/`author`、`check` 的 `detail`、`tldr` 的 `point`/`verdict`、`news-item` 的 `title`/`source` 与正文（`<detail>` 内文也入索引）。属性值需要在文本投影里显式收入，沿用“只取元素文本”的规则不会命中；`next-steps/option` 的按钮文字不入索引。**搜索只定位到文章，不定位到阶段**：命中第二组第三阶段的词，点击结果只打开文章（所有阶段内容仍可被检索）；要定位到某阶段靠已知锚点的链接。
* **不进入索引**：`<equation>`、`<mermaid>`、`<chart>`、`<code-block>` 的源文本（只索引 `caption`/`file`），避免 LaTeX/JSON/代码噪声污染搜索。注意 v1.0 的 `PlainText()` 用的是全量 `//text()`，需要改为排除这些子树；已入库的 v1.0 文档不含这些标签，无需重建索引，规则变化后的重建见 10.5 `reindex`。
* **版本边界**：本节的文本投影只对 `version="1.1"` 的新入库文档生效；v1.0 已入库文档沿用原投影，不要求重建索引（6.1）。长词（FTS）与 2 字短词（`LIKE`）都要有验收（见 13，#81）。
* **安全**：所有新标签的文本经 Vue 模板转义渲染；Chart 只吃 6.3 的结构化数据；`url`/`code-url` 链接只允许 `http/https`；所有新增 `<md>` 型正文沿用现有的 markdown-it + DOMPurify 流水线；资源大小沿用 v1.0 的上限。
* **渲染层专项防护（新标签引入的新攻击面）**：Mermaid 使用 `securityLevel:"strict"`，且生成的 SVG 再经 DOMPurify（SVG profile）后才挂载；ECharts 的 tooltip 一律使用纯文本渲染（`renderMode:"richText"`），不走 HTML 字符串拼接，防止恶意 `series[].name`/`categories` 触发 XSS；每篇文档最多 10 个 `<mermaid>`、10 个 `<chart>`（超出 → `TOO_MANY_BLOCKS`），防止渲染卡死主线程。

### 6.5.1 中文检索（v1.1，P1）

现状的缺口：`items_fts` 用 `trigram` 分词，只能匹配 3 个及以上字符；中文里最常见的 2 字词（“模型”“检索”）退化为对 `plain_text` 等列的 `LIKE` 全表扫描，没有索引，也没有按相关度排序。本节补上这一块，不引入分词库，不引入 CGO，不依赖词典。

* **新增一张 FTS5 表 `items_fts_cjk`**（goose 迁移）：列与 `items_fts` 相同（`item_id UNINDEXED, title, summary, tags, plain_text`），`tokenize = 'unicode61'`。写入的不是原文，而是**二字组展开**后的文本：把文本中每一段连续的 CJK 字符（汉字、平假名、片假名、谚文）展开成相邻两字的重叠组合，用空格连接（“大模型训练” → `大模 模型 型训 训练`）；标点、空白、拉丁字母与数字会截断这一段，二字组不跨越它们；拉丁字母与数字部分原样交给 `unicode61`。长度为 1 的 CJK 段不产生二字组。
* **展开函数只有一份**，放在服务端的一个共享位置，入库、`reindex`、查询三处共用，避免写入与查询的拆法不一致。
* **查询**：每个空白分隔的查询词再按“CJK 段 / 其他段”切开。长度 ≥ 2 的 CJK 段展开成二字组并作为 FTS5 短语查询（所有用户输入仍整体加引号，不能被解析成 FTS 语法）打在 `items_fts_cjk` 上；其他段沿用现有规则（≥ 3 字符走 `items_fts` 的 `trigram`，否则 `LIKE`）；长度为 1 的 CJK 段走 `LIKE`。各段之间是 AND。
* **排序与片段**：两张表的 `bm25()` 都带列权重，标题 > 标签 > 摘要 > 正文（具体数值由实现定，写进代码注释）。命中片段与高亮必须取自**原文**，不得显示二字组展开后的文本：命中走 `items_fts_cjk` 时，由 Go 在 `plain_text` 里定位查询词并截取上下文，用现有的 `markStart`/`markEnd` 标记。
* **维护**：新增、更新、删除条目时，在**同一个事务**里同步写 `items_fts_cjk`；`aumbry-server reindex` 同时重建两张表。
* **不做**：分词与词典、同义词、简繁转换、拼音；这些都不在本版范围。

### 6.6 示例（精读场景节选）

```xml
<document version="1.1">
  <meta> … </meta>
  <body>
    <section title="第二轮：方法与证据">
      <claim level="claim" source="§1 Abstract">推理延迟较扩散式方法降低一个数量级。</claim>
      <claim level="evidence" source="Table 2">在 nuScenes 上 30 FPS，对比 Vista-v2 为 8 FPS（同为单卡 H100，batch=1）。</claim>
      <claim level="unverified">训练预算与随机种子数：论文未报告。</claim>

      <equation id="loss" number="5" caption="流匹配目标"><![CDATA[\mathcal{L}=\mathbb{E}_{t,x}\lVert v_\theta(x_t,t)-u_t(x_t)\rVert^2]]></equation>

      <mermaid caption="整体流程"><![CDATA[flowchart LR
  A[多视角视频] --> B[编码器] --> C[世界模型] --> D[规划头]]]></mermaid>

      <compare-table caption="与相关工作对比">
        <columns><col>方案</col><col>帧率</col><col>代码</col></columns>
        <row><cell>DriveFlow</cell><cell tag="evidence">30 FPS</cell><cell>已开源</cell></row>
        <row><cell>Vista-v2</cell><cell tag="na">8 FPS（协议不同）</cell><cell>仅权重</cell></row>
      </compare-table>
    </section>
  </body>
</document>
```

### 6.7 分阶段阅读、测验与阅读进度（v1.1，P1）

来源：`qi-read-paper` 每次只讲一轮、用户说“继续”才进入下一轮并可随时跳转深入；`interactive-guide` 的“阶段 + 级别 + 小测验”。Aumbry 只提供**阅读节奏与互动**，不改变内容与权限。

* **门控是节奏控制，不是访问控制**：所有阶段的内容都在同一份 XML 里并随条目一起下发，全文索引覆盖所有阶段，管理员/访客的可见性只由目录权限决定。
* **多组、各自独立**：一份文档可以在中间多次使用 `<stages>`，也可以每个主题各自分阶段（例如一篇周报里每个主题一组）。每组有自己的步骤条、进度与“继续”按钮；一组前进不影响另一组。`stages` 不嵌套：`stage` 内部可以有 `section`、`quiz`、`tabs` 等，但不能再放 `stages`（否则状态、锚点与进度的含义变得不确定）。
* **默认门控**：每组含 ≥ 2 个 `stage` 且该组 `gated` 未设为 `false` 时，只展开已到达的阶段；阶段末尾出现“继续：<下一阶段标题>”按钮；页头有一个对所有组生效的“展开全部”开关。`gated="false"` 的组默认全部展开。只有 1 个 `stage` 的组（如逐轮发布的第一轮）不显示步骤条与“继续”，直接展开；追加第二个 `stage` 后自动出现。
* **进度存储**：只存浏览器（`@vueuse/core` 的 `useStorage`，键含条目 `id`），存储结构为 `{ stages: Record<stagesId, stageId>, quizzes: Record<quizId, { fp, picked: number[] }> }`（`stages` 存每组“已到达的最后一个 `stage id`”；`quizzes` 只存选中选项的下标，`fp` 是题干、选项顺序与正确答案的指纹，用 Web Crypto 的 SHA-256 计算）。题目内容变化（指纹不同）时只清除该题作答，阶段进度保留；`localStorage` 里的 JSON 损坏或结构不符时静默重置，不让阅读页初始化抛错；`localStorage` 不可用（如部分 `file://` 场景）时静默退化为会话内状态。按 `stages`/`stage`/`quiz` 的 `id` 记录，所以逐轮追加新阶段后旧进度保留；被删除或改名的 `id` 的进度被忽略（该组回到第 1 个阶段）。**因此已发布的 `id` 不得改名。**
* **深链接**：`#stage-<id>` 直达某阶段；`#fig-*`、`#eq-*`、`#ref-*` 等锚点落在未解锁阶段时，先把**所属那一组**解锁到目标阶段（含此前的阶段，保持顺序）再定位，不影响其他组（与 6.4 的折叠展开规则一致）。搜索结果跳转同理。
* **测验**：1 个正确选项为单选（Reka UI `RadioGroup`），多个为多选（`CheckboxGroup`）；提交后逐项显示对错与 `<explain>`；每个阶段显示“答对 x/y”。题干、选项、解析全部按文本渲染，不经 HTML 解析。
* **`<next-steps>`**：是按钮而不是链接：`target` 只接受 6.4 的页内锚点，走 6.4 的“定位顺序”，不经 `safeUrl`，不会整页跳转。
* **打印**：`@media print` 展开全部阶段、`tab`、`collapse`（与 9.3 同一机制，需强制挂载）。
* **TOC**：有 `stage` 时，每组按阶段分组并标出当前阶段，位置跟随该组所在的章节，移动端抽屉同样（9.2）。
* **不做**：服务端按时间/按轮次解锁、答题统计上报、成绩排行。需要时另议（见 14.2 第 2 条）。

### 6.8 摘要分层、速读模式与新闻简报（v1.1，P1）

* **三层摘要，各管一件事**：`meta/summary`（v1.0，一句话，用于卡片与搜索结果）→ 全文 `<tldr>`（`body` 内至多 1 个，要点 + 结论，默认显示在正文首屏）→ 各部分 `<tldr>`（`section`/`stage` 内的本部分小结）。三者互不替代；`meta/summary` 应与全文 `<tldr>` 的结论一致，但**校验器不强制**，由 skill 的默认做法约束（避免引入新的 warning 机制）。
* **总结只概括正文已有内容**，不引入正文没有的事实、数字或结论（skill 规则）。
* **速读模式**见 9.7：只显示各部分标题与 `<tldr>`，需要时再展开某部分全文；适合很长的论文精读、研究报告、周报与新闻简报。
* **新闻简报与详情**：简报（场景 K）是轻量的卡片列表，详情（场景 L）是独立条目，因此可以被单独检索、分享、设置目录权限，也不会让简报膨胀。简报与详情之间只用 `detail-id`（简报 → 详情）与 `meta/related-ids`（详情 → 简报）互指，**不新增服务端反向索引**（见 14.2 第 10 条）。`<news-item>` 的 `detail-id` 指向无权限的条目时，访客只会在跳转后看到 403，卡片上显示的标题来自简报自身，不泄露目标条目信息。
* **周报**：不是新标签，只是约定：按天分 `section`，每天一个 `<tldr>`，整篇一个全文 `<tldr>`，当周各日报写进 `related-ids`。

### 6.9 对现有 `pkg/schema` 的改动（实现说明）

* **现状**：`protocol.sch` 的块级规则几乎都写死为 `/document/body/section/...` 或 `/document/body/section/collapse/...`（`containers`、`leaves`、`blocks`、`compare-table`、`sources`），Go 侧的 `figureXPath`、`urlXPath` 也一样，且 `urlXPath` 只覆盖 `paper-card`/`repo-card`/`source`。按 6.0 放宽容器后，原样保留这些路径会**静默漏检**（例如 `<compare-table>` 放进 `body` 或 `stage` 后列数检查失效）。
* **做法**：块级规则的 `context` 改为按元素名匹配（`//compare-table`、`//paper-card`、`//figure` 等）；容器关系单独成组并按版本隔离：`/document[@version='1.0']` 下保持现有容器规则，`/document[@version='1.1']` 下用新规则。Go 侧 `figureXPath` 改为 `//figure[@src]`，`urlXPath` 扩到 `ref`、`quote`、`news-item` 的 `url` 与 `paper-card/@code-url`，`checkURLs` 不再硬编码只读 `url` 属性。
* **版本门控不重复报错**：`1.0` 容器规则的 `UNKNOWN_TAG` 要排除 1.1 保留标签名，使一个标签只报一条 `TAG_REQUIRES_1_1`；v1.0 规则本来不拦多余属性，新属性（`figure@id/alt`、`cell@tag`、`paper-card@venue|doi|code-url|read-depth`、`repo-card@license|commit`、`section@id`、`steps@interactive`、`ref@read-depth` 等）要在 `1.0` 文档里显式拦截。
* **深度与性能**：`count(ancestor::section) <= 3` 一类谓词与 `//` 轴在 helium 的 Schematron 上的性能是**推断未验证**，实现前用单测和最大尺寸文档验证；若不满足，深度检查移到 Go 侧。
* **`aumbry schema`**：Markdown 与 JSON 共用同一张**按版本区分的规则表**（允许的父子关系、数量上下限、属性、位置例外、错误码），skill 参考文档与校验器同源，避免漂移。

---

## 7. CLI 与 API 增强（G5）

### 7.1 `aumbry push`

* 新增 `--draft`：不挂载任何目录，忽略配置里的 `default_dirs` 与环境变量 `AUMBRY_DIRS`；与 `--dir` 同时出现 → 参数错误。
* **`--draft` 绝不能碰已发布的条目**：服务端 ingest 新增可选表单字段 `draft=1`；若目标条目已存在且当前有任何挂载 → `409 already_published`，**不写入任何内容或存储**（原正文与挂载保持不变）。这样“草稿”提交不会把含内部批注的新版直接暴露给访客。继续修改已发布内容就不带 `--draft` 直接 `push`（保持挂载）；要撤回先 `aumbry unpublish`。草稿阶段的逐轮提交（条目尚无挂载）可反复使用 `--draft`。响应的 `dirs` 为推送后条目的**全部**当前挂载，并新增 `published`（挂载数 > 0）。
* 输出 JSON 新增 `full_url`（由 **CLI** 用当前 profile 的 `server` 与服务端返回的 `url` 拼接，服务端不返回）、`draft`（布尔，即是否使用了 `--draft`）与 `published`。其余字段不变。
* 新增 `--create-only`：服务端 ingest 新增可选表单字段 `create_only=1`，**由服务端原子裁决**：`external_key` 已存在 → `409 key_exists`；两个请求并发创建同一个新键时，由数据库唯一约束决出唯一赢家，输家得到 `409 key_exists`（可能已上传的孤儿对象留给 `gc`）；`created_by` 随首次 INSERT 一并写入。不能由 CLI 先查后推（会有竞态）。与 `--id`（`item_id`）同时出现 → 参数错误。
* 新增 `--dry-run`：本地完整校验，并调用 `GET /api/v1/agent/dirs` 确认服务地址、Token 与目录 ID 有效；不上传、不写库、不写存储。
* 服务端 `POST /api/v1/ingest` 只**新增**两个可选表单字段 `draft`、`create_only`；不带它们的请求（包括 v1.0 的旧 CLI）行为完全不变。新 CLI 使用 `--draft`/`--create-only` 时，先调用 `GET /api/v1/agent/dirs` 确认服务端是 v1.1（旧服务端返回 404），否则在上传前失败，避免旧服务端忽略未知字段而悄悄覆盖。

### 7.2 `aumbry dirs`

* 作用：让 Agent 自己找到目录 ID，而不是靠用户手抄。
* 输出：JSON 数组（默认）或 `--format tree` 的缩进文本；每项 `{id, name, path, parent_id, has_password, protected, item_count}`，`path` 形如 `个人知识库 / 论文精读`。`has_password` 只表示该目录**自己**设了密码；`protected` 表示它自己或任一祖先设了密码（访客需要解锁才能看）。Agent 判断“这个目录是否公开”必须看 `protected`，不能看 `has_password`。
* 服务端新增 `GET /api/v1/agent/dirs`：**API Token 鉴权**，返回全部目录的上述字段。API Token 本就是受信写入方，因此可见目录名与 ID，**不含密码哈希**，也不返回条目内容。

### 7.3 `aumbry schema`

* `aumbry schema [--format md|json]` 输出当前二进制所支持协议的标签参考（标签、属性、约束、错误码，含一份最小示例）。
* 内容来自内嵌在 `pkg/schema` 的单一参考源，Skill 的 `references/xml-tags.md` 由同一来源生成（见 8.3），保证“Agent 读到的说明”与“二进制实际校验的规则”不会各说各话。

### 7.4 `aumbry validate`

输出结构不变；新增错误码：`TAG_REQUIRES_1_1`、`DUPLICATE_ID`、`INVALID_CHART`（带 JSON 路径）、`INVALID_EQUATION`、`INVALID_MERMAID`（仅检查非空与长度，语法无法在 Go 侧校验）、`INVALID_METRIC`、`INVALID_DOI`、`TOO_MANY_BLOCKS`、`DUPLICATE_TAB`；v1.1 又新增 `SECTION_TOO_DEEP`、`INVALID_STAGES`、`INVALID_QUIZ`、`INVALID_TARGET`、`INVALID_TLDR`、`INVALID_NEWS`、`INVALID_CHECK`（`status` 非法）、`INVALID_SEARCH_LOG`（缺 `<query>`）、`INVALID_DATE`（`timeline` 的 `date`）、`INVALID_CODE_BLOCK`（超 32 KB 等）。`INVALID_METRIC` 的触发条件是 `label`/`value` 为空或一组超过 12 个 `<metric>`；`TOO_MANY_BLOCKS` 只用于全文级上限（`mermaid`/`chart` 各 10、`stages` 8 组、`stage` 合计 40）。`TAG_REQUIRES_1_1` 与旧容器的 `UNKNOWN_TAG` 不重复报告（见 6.9）。

**自修复提示**：`ValidationError` 增加可选字段 `hint`（一句可直接照改的修法）。大模型最常见的错误是在 `<md>`、`<callout>`、`<mermaid>`、`<chart>` 里忘记包 `<![CDATA[…]]>` 而直接写 `<`、`&`、泛型尖括号，底层解析器只会报“标签不匹配/意外 EOF”。当 `INVALID_XML` 发生在这些元素内部时，`hint` 为“把正文放进 `<![CDATA[ … ]]>`”；`TAG_REQUIRES_1_1`、`INVALID_CHART` 等也附带 `hint`。

### 7.5 `aumbry list` / `unpublish` / `delete`

解决两个具体问题：新会话里 Agent 查不到昨天那篇精读的 `id`，只能让用户手抄或编造 `related-id`；误把草稿推到公开目录后，Agent 没法撤回。

* 服务端新增（均为 **API Token 鉴权**）：`GET /api/v1/agent/items`（参数 `key`（精确匹配）、`q`（**只匹配 `title` 与 `external_key`**，不搜正文与摘要，避免 Token 借全文搜索探测别的 Agent 的草稿内容）、`dir`、`limit`、`offset`；返回 `id`、`type`、`title`、`date`、`external_key`、`url`、`dirs`，**不返回 XML 正文**）、`DELETE /api/v1/agent/items/{id}/mounts`（**只撤回**：清空该条目的全部挂载；不提供“替换为任意目录列表”，那留给管理员接口）、`DELETE /api/v1/agent/items/{id}`。`list` 的每项带 `owned`（是否由当前 Token 创建）。
* CLI：`aumbry list [--key K] [--q Q] [--dir D]`；`aumbry unpublish <id>`（清空挂载，回到草稿）；`aumbry delete <id> --yes`（无 `--yes` 拒绝执行）。
* **所有权：Agent 只能下线或删除自己写的条目。** 条目首次创建时，服务端记录创建它的 API Token 的指纹（**完整的 `SHA-256(规范化 token)` 十六进制串**，不存明文 Token；认证成功时由鉴权层返回匹配到的规范化 Token 的摘要，不得另对 `Authorization` 头原文做摘要），保存在新增列 `items.created_by`；之后的更新不改该列。`DELETE …/mounts` 与 `DELETE …` 只在条目的 `created_by` 等于当前 Token 指纹时执行，否则 `403 not_owner`。
* v1.0 已有的条目没有所有者（`created_by` 为 `NULL`），Agent 无法下线或删除它们，只能由管理员处理；管理员接口不受所有权限制。
* 同一个 Token 被多个 Agent 共用时视为同一所有者；要让各 Agent 互相不能动对方的条目，给每个 Agent 配一个独立 Token（`AUMBRY_API_TOKENS` 本就支持多个）。轮换（更换）Token 字符串后，旧条目的所有权随之失效，需管理员处理。
* 所有权**只约束下线与删除**，不改变 v1.0 的 `push` 语义：用同一个 `external_key` 推送仍可更新任何条目的内容（是否把所有权扩展到内容覆盖见 14.2）。
* **`unpublish` 的保证范围**：清除挂载，使新访客的条目与正文请求返回 403。它**不保证**撤回已被浏览器缓存的图片（`Cache-Control: private, max-age=86400`，最多一天），也不影响被另一篇仍可访问的文章共用的同哈希图片。
* 数据库迁移：新增一个 goose 迁移（`ALTER TABLE items ADD COLUMN created_by TEXT`），旧库升级后零数据变动。

---

## 8. Agent Skill：`skills/aumbry/`（G6）

### 8.1 目标

任何带 skill 机制的 Agent，读完它就应当能：**知道什么时候该发布；按场景写出能通过校验的 XML；用 CLI 校验、修复、推送；把链接回复给用户**。它承接内容 skill 的产出，不重复它们的流程。

### 8.2 目录结构

```text
skills/aumbry/
├── SKILL.md                      # 触发条件、核心回路、硬规则（精简）
├── references/
│   ├── xml-tags.md               # 由 `aumbry schema --format md` 生成，不手改
│   ├── scenarios.md              # 第 2 章的场景 → 标签 → 幂等键映射，含逐轮发布与本地草稿目录约定（2.3）；
│   │                             # （参考，不是必填结构）归档 interactive-guide 会话时只提炼结构化摘要 + mermaid/steps，不推整页 HTML
│   └── troubleshooting.md        # 校验错误码 → 修法；常见 push 失败（401/403/422/413）
└── assets/examples/
    ├── one-pager.xml             # 无章节短文：body 直接放 md + 图表（展示“不必有 section”）
    ├── paper-reading.xml         # 论文精读：多组 stages + quiz + next-steps + tldr（场景 A、J）
    ├── compare-repro.xml         # 对比与复现混排：compare-table(tag) + metrics + chart + checklist（场景 B、G、H）
    ├── news-digest.xml           # 新闻简报：按主题 section，news-item，部分带 detail-id；日报/周报同构（场景 C、K）
    └── news-detail.xml           # 新闻详情：tldr + 时间线 + 引述 + sources，related-ids 指回简报（场景 L）
```

### 8.3 内容要求

**`SKILL.md`**（风格同用户现有 skill：英文 frontmatter 的 `description` 以 “Use when …” 写触发与排除条件；正文中文）：

* **何时用**：用户要求把论文精读/对比/日报/研究报告发布、归档、存到 Aumbry，或要求写 Aumbry XML。**不用于**阅读论文本身（`qi-read-paper`）、检索文献（`paper-search`）、写报告正文（`research-report`）。
* **核心回路**：写（参照示例、按内容自行组织）→ `aumbry validate` → 按 JSON 错误修复 → `aumbry push` → 回复 `full_url`。
* **硬规则**（只管发布授权、协议合法性与真实性；每条都对应一个已知的失败方式）：
  1. 普通 Markdown 正文放 `<md><![CDATA[…]]></md>`；XML 属性里不要出现裸 `&`、`<`。
  2. 图片只用相对路径的本地文件，格式 `png/jpg/jpeg/webp/svg`，不引用外链、不用绝对路径、不用 `..` 跳出文档目录。
  3. 不编造 `related-id`、`detail-id` 与目录 ID；目录用 `aumbry dirs` 查（判断是否公开看 `protected`），条目 ID 用 `aumbry list --key` 查或只用 `push` 返回过的。
  4. 日期用 `YYYY-MM-DD`；`type` 只能是 `report`/`post`；使用新标签或新属性必须声明 `version="1.1"`。
  5. 幂等键按场景约定；同一内容重复发布用同一个键，**不要**每次换键。
  6. 不确定的证据用 `<claim level="unverified">`，不要写成事实。
  7. 校验未通过绝不 push；不绕过校验。
  8. 用户没有要求发布前不 push（包括草稿）；用户说“存一下/先存草稿”才用 `--draft`，说“发布”才带 `--dir`。
  9. 已发布文档再 push 时，不改、不删已有 `stages`/`stage`/`quiz` 的 `id`（读者进度按 `id` 保存）。
* **默认做法**（可按内容与用户要求偏离，**不是规则**）：
  1. **结构由内容决定，不是填表**：示例只是参考；章节数量、标题、顺序、用哪些标签都按内容与用户要求取舍，不为“凑齐示例里的章节”而写空话；表达不了的结构先用 `<md>`，不要为此改协议或绕过校验。
  2. 长文档（≥ 3 个 `section`/`stage`，或明显超过 2000 字）默认给全文 `<tldr>` 与每个部分的 `<tldr>`；用户另有要求、或内容不适合（纯日志、单条短讯）时省略。总结只概括正文已有内容；`meta/summary` 与全文 `<tldr>` 的结论一致。
  3. 新闻：每条 `<news-item>` 默认带 `source` 与 `published`；先推详情再推简报；未证实消息写“据称”或用 `claim level="unverified"`。
  4. 复现/实验类文档：目标数值与容差写在结果之前；缺失的数字用 `tag="unreported"`，不写 0、不估算；从图中读出的数用 `tag="approx"`；结论取 复现 / 未复现 / 无法量化 / 环境差异，不写“论文有误”除非环境与指标定义都已排除（`paper-reproduce`）。
  5. 互动内容：`<quiz>` 至少有一个正确选项（校验会拦），答案对读者可见，不要放敏感信息；测验考察应用而非复述。
* **与其它 skill 的衔接**：只描述“内容 skill 产出后如何落到 Aumbry”，并列出各场景对应的内容 skill；不改写它们的流程，也不依赖它们安装。

**`references/xml-tags.md`**：`make skill-refs` 调用 `aumbry schema --format md` 生成；CI 里 `make skill-check` 重新生成并 `git diff --exit-code`，漂移即失败。

**`assets/examples/*.xml`**：每个示例都是**可直接通过 `aumbry validate` 的完整文档**（图片用 `examples/imgs/` 同款占位 SVG），用来示范“这类内容常用哪些积木怎样搭配”。**它们不是必须遵守的骨架**：Agent 可以增删章节、调换顺序、混用其他标签，只要通过校验；CI 对全部示例执行 `aumbry validate`，保证示例本身不过期。只放 5 个**结构差异明显**的示例（无章节短文、多组互动、对比复现混排、新闻简报、新闻详情）；其余场景（研究报告、项目拆解、周期汇报、审计、实现记录等）只在 `scenarios.md` 里给出积木映射，不再各配一份示例，避免示例变成模板。

### 8.4 分发

* 仓库内路径固定为 `skills/aumbry/`；GitHub Release 压缩包包含该目录，安装 CLI 时可一并取得。
* README 增加“安装 Skill”一节：复制或软链到 Agent 的 skills 目录（如 `~/.agents/skills/aumbry`）。
* 建议（不在本仓库内执行）：用户在自己的 `qi-read-paper` 等 skill 里各加一句“用户要求归档/发布时交给 `aumbry`”。

---

## 9. 读者侧体验增强（G7）

均为前端为主的改动；凡涉及搜索/列表的，**一律限定在访客当前已解锁的子树内**，不新增任何全站枚举入口（不做 `/tags/{tag}` 之类的全局页面）。

### 9.1 目录内按标签与类型筛选（P1）

* 问题：条目卡片与阅读页的标签芯片目前不可点击；分享目录里有几十篇日报/精读时，访客无法只看精读（`post`）或某个标签下的内容。
* `GET /api/v1/dirs/{id}/search` 新增可选参数 `type`（`report`/`post`）与 `tag`；`q` 允许为空（此时等价于带筛选的列表，按日期倒序）。标签过滤在已有的“可见目录集合”之内用 SQLite 内置 `json_each(tags_json)` 完成。
* 前端：点击标签芯片 → 目录页带 `?tag=` 查询；类型筛选用现有的 `SelectBox`。

### 9.2 移动端目录抽屉与章节链接复制（P1）

* 问题：阅读页右侧 TOC 在窄屏上被整体隐藏，手机上无法在分阶段精读（见 6.7）或长篇报告里跳章节；含 `<stages>` 时 TOC 按阶段分组并标出当前阶段；嵌套 `section`（至多 3 层，深度只数 `section` 祖先，`stages`/`stage` 不计）在 TOC 中缩进显示，标题级别随深度为 `h2`/`h3`/`h4`。
* 窄屏下提供浮动按钮，用 Reka UI 的 `Dialog` 打开章节抽屉；桌面端与移动端的 `<section>` 标题悬停/长按时出现“复制章节链接”按钮（`@vueuse/core` 的 `useClipboard`）。

### 9.3 打印（P1）与导出 Markdown（P2），均晚于 9.6 的深链接展开

* `@media print`：白底黑字，隐藏站头与 TOC，图片不跨页断裂。**“打印时展开所有 `<collapse>`、`<tab>` 与未解锁的 `<stage>`”需要让这些内容在从未打开过时也已挂载**（现有 `<collapse>` 默认不挂载正文，非活动 Tab 同理），用 Reka UI 的强制挂载能力实现，并用“从未打开过的 Tab”做验收；否则只写 CSS 不会让未打开的内容出现在打印稿里。
* “导出 Markdown”：纯前端把已渲染内容转成 GFM Markdown 下载（`turndown`），不新增服务端接口。

### 9.4 浅色/深色主题（P2）

当前强制暗色，`shiki` 固定 `github-dark`。用 `@vueuse/core` 的 `useColorMode` 切换，`shiki` 使用 `github-light`/`github-dark` 双主题，Mermaid/ECharts 跟随主题重绘。

### 9.5 同目录上一篇/下一篇与 `/` 聚焦搜索（P2）

`GET /api/v1/items/{id}` 增加 `neighbors`：在**访客可见**的挂载目录内按日期取前一篇/后一篇，不跨出可见范围。`/` 快捷键聚焦目录页与管理端的搜索框（`@vueuse/core` 的 `onKeyStroke`）。

### 9.6 锚点命中隐藏内容（P1，见 6.4）

`#fig-*`、`#eq-*`、`#ref-*`、章节锚点落在收起的 `<collapse>` 或未激活的 `<tab>` 内时，前端先展开/切换再滚动。

### 9.7 速读模式（P1，见 6.8）

问题：精读、研究报告、周报与新闻简报往往分成很多部分，读者想先看每部分的结论，再决定读哪一部分。阅读页头部在文档含至少一个 `<tldr>` 时显示“速读”开关：开启后只显示各 `section`/`stage` 的标题与 `tldr`，其余内容折叠，点“展开本节”再显示全文；状态由 `?skim=1` 与 `@vueuse/core` 的 `useStorage` 保存。没有 `tldr` 的部分只显示标题与“展开本节”。速读只折叠每个容器自己的**正文块**（`md`、`figure`、`chart` 等），子 `section`/`stage` 的标题与 `tldr` 仍然显示；`body` 直接放置的正文块归入一个“正文”折叠区，提供“展开正文”。偏好优先级：URL 的 `?skim=` 优先于本机记忆；记忆按条目 `id` 分别保存，不是全站开关；定位到某个锚点时自动展开目标所在节（6.4 定位顺序第③步）。与 6.7 的阶段门控相互独立：速读折叠的是同一阶段内的细节，门控决定阶段是否可见。`@media print` 忽略速读，展开全部。

---

## 10. 运维与安全加固（G8）

### 10.1 `/readyz` 就绪探针（P1）

`/healthz` 保持轻量存活探针不变。新增 `GET /readyz`：对 SQLite 执行 `PingContext`，对 WebDAV 执行带 2 秒超时的 `Stat("/")`（结果做 5 秒短缓存，避免探针打爆 WebDAV）；任一失败返回 `503` 与 `{ "status": "unavailable", "checks": { "sqlite": "ok", "webdav": "unreachable" } }`——对外只返回固定的失败类别（`ok`/`unreachable`/`timeout`/`auth`），详细错误只写脱敏日志，避免公开探针泄露内部地址或路径。两个探针都带截止时间（SQLite 的 `PingContext` 会等待唯一连接，不能无限等），并发探针合并为一次检查。服务启动时同样做一次检查并打印日志，WebDAV 凭证或网络有误时能在启动日志里立刻看到。

### 10.2 WebDAV 客户端超时（P1）

现在的 WebDAV 客户端没有 HTTP 超时且忽略 `ctx`，WebDAV 挂起会让请求一直阻塞。为 `gowebdav` 客户端注入带拨号超时（5 秒）与响应头超时（30 秒）的 `http.RoundTripper`；大文件流式传输不设整体超时。**保证范围**：这只保证“等待响应头”有界；WebDAV 发完响应头后卡在响应体、或接收上传时停止读请求体，不在此保证之内（`gowebdav` 不支持 context）。就绪探针使用单独的、带完整 2 秒请求期限（`SetTimeout`）的探针客户端。本版**不做自动重试**（避免重复写入的副作用）。

### 10.3 安全响应头（P1）

全站（SPA 与 API）统一下发 `Referrer-Policy: no-referrer`（匿名分享链接 `/d/{id}` 不经 `Referer` 泄露给外链站点）、`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`（防 `/admin` 与密码门禁被点击劫持）。使用现成的中间件（如 `unrolled/secure`）。`/items/{id}/content` 与 `/assets/{hash}` 已有的沙箱 CSP 保持不变。

### 10.4 写入口限流（P1）

`/api/v1/ingest` 与 `/api/v1/agent/*` 目前没有任何限流：Token 可被无限次试探，也可被泛洪拖慢。使用已有的 `httprate`，按（可信代理解析后的）客户端 IP **固定 120 次/分钟**（`httprate` 是滑动窗口近似，临界时刻的精确计数以其实现为准），超限 `429`；`/ingest` 与 `/agent/*` 共用同一个桶。另用 chi 自带的 `Throttle` 给 `/ingest` 设**固定的并发上限（4）**，防止少量合法 Token 同时上传接近 100 MB 的请求而耗尽内存与磁盘暂存。不新增配置项。

### 10.5 备份、对账与重建索引（P1/P2）

* v1.0 的“直接拷贝单个 `.db` 文件容灾”在 `journal_mode=WAL` 下**不安全**（会漏掉 `-wal` 中未合并的页）。README 与 PRD v1.0 10.3 的说法在本版修正。
* `aumbry-server backup --out <path>`：用 SQLite 内置 `VACUUM INTO` 做在线、WAL 安全的**数据库快照**；服务运行中可执行。**它只备份数据库**：快照之后更新条目时，被替换的旧 XML 会从 WebDAV 删除，恢复旧快照会得到指向已不存在对象的条目。完整恢复需要把数据库快照与 WebDAV 的同时刻快照（或停写窗口内的导出）配对，PRD 与 README 明确写出这一点；恢复后必须执行 `verify-storage`，并实际打开一篇报告和一张图验证。
* `aumbry-server verify-storage`：核对数据库引用的所有 XML 与图片对象在 WebDAV 上都存在，输出缺失清单（用于恢复后验证）。
* `aumbry-server reindex`（P2）：从存储中的 XML 按当前 `PlainText()` 规则重建 `items_fts`。

### 10.6 孤儿资源清理 `aumbry-server gc [--dry-run]`（P1）

v1.0 没有垃圾回收：删除条目、更新替换图片、上传中途失败都会在 WebDAV 与 `assets` 表中留下无人引用的对象。`aumbry-server gc [--dry-run]` 清理这些对象，**但破坏性的 `gc`（不带 `--dry-run`）只允许在服务停止写入时执行**（离线维护）：ingest 在数据库事务之前先上传对象，运行中的 `gc` 可能把“刚上传、尚未被引用”的对象当孤儿删掉，随后 ingest 提交了指向它的引用，造成永久断图/断正文（外键只能阻止删 `assets` 行，挡不住已删的远端对象）。`--dry-run` 任何时候都可以执行。扫描范围只限 Aumbry 自己管理的对象名（`/xml/`、`/assets/`），并包含“对象已上传但从未进入 `assets` 表”的失败上传残留；远端枚举用 `gowebdav` 的 `ReadDir`（存储接口为此增加 `List`）。清理顺序：先删对象，再在事务里删数据库行。

### 10.7 单实例约束（P1，文档）

SQLite 单连接与内存限流决定了**只能单实例运行**；在 README 与 `config.example.toml` 中明确写出，误部署多副本会互相锁库且限流失效。

---

## 11. 非功能与兼容

* **向后兼容**：升级无需**人工**迁移：goose 自动新增 nullable 列 `items.created_by`（SQLite `ADD COLUMN`，不重建表，旧行为 `NULL`）；v1.0 的 env 配置、数据库、XML 文档、以及不带新字段的旧 CLI 请求继续可用。**唯一的可见变化**：官方镜像不再内置 `AUMBRY_*` 环境变量（见 3.3），直接 `docker run` 的用户需显式传 `AUMBRY_DATA_DIR`（compose 示例已包含）。
* **性能**：新增前端依赖（Mermaid、ECharts）必须按需动态加载，不得进入首屏 chunk；不含这些标签的文章不加载它们。
* **依赖原则**：沿用“有成熟开源库就用库，不手写”：配置（go-toml/v2 + 已有的 caarlos0/env）、XFF（chi）、图表（ECharts）、流程图（Mermaid）、Markdown 增强（markdown-it 插件）、JSON Schema 校验、镜像构建（GoReleaser `dockers_v2`）。
* **构建约束**：`CGO_ENABLED=0` 不变。

---

## 12. 迭代里程碑

| 里程碑 | 内容 | 依赖 |
| :--- | :--- | :--- |
| **M1** | 配置文件与 `check-config`、`cobra` 子命令骨架；可信代理；安全响应头（10.3）；写入口限流（10.4）；WebDAV 超时（10.2）；`/readyz`（10.1）；README/`config.example.toml`/compose 更新（含单实例约束 10.7） | – |
| **M2** | GHCR 镜像（`dockers_v2`、`Dockerfile.release`、workflow）可构建并通过验收 11–14；`backup`/`verify-storage`/`gc`（10.5、10.6）。**正式 `v1.1.0` Release 的压缩包需包含 M4 才完成的 Skill，因此正式发布等 M4 完成**；M2 阶段只发 RC | M1 的子命令骨架 |
| **M3** | 协议 v1.1 P0 标签（校验 + 渲染 + 索引规则 + 渲染层防护）；`aumbry push --draft/--create-only/--dry-run`；`aumbry dirs`、`list`、`unpublish`、`delete`（含 `items.created_by` 所有权迁移）与对应 `/api/v1/agent/*`；`aumbry schema`；`validate` 的 `hint` | – |
| **M4** | P1 标签（含 `stages`、`quiz`、`next-steps`、交互式 `steps`、`search-log`、`tldr`、`news-item`）；`skills/aumbry/` 全部内容与五个示例；`make skill-refs`/`skill-check` 与 CI；读者侧 9.1、9.2、9.3（打印）、9.6、9.7 | M3 |
| **M5（可选）** | P1：6.5.1 中文检索；P2：`series[].err`、9.3 导出 Markdown、9.4 主题、9.5 邻近条目、10.5 `reindex` | M4 |

M1、M3 互不依赖，可并行；M2 依赖 M1 的子命令骨架；M4 依赖 M3 的协议与 `aumbry schema`。

---

## 13. 验收标准

每一条都可以由他人执行并判定。

**G1 配置文件**
1. 仅用 env（v1.0 的 9 个变量）启动 → 与 v1.0 行为一致。
2. `aumbry-server check-config --config config.example.toml`（填入占位合法值）退出码 0，输出中密钥只显示末 4 位。
3. 同一键在 TOML 与 env 中取值不同 → 生效的是 env（单测）。
4. TOML 含未知键 `listn_addr` → 退出码 1，错误信息指出该键。
5. `--config /不存在.toml` → 退出码 1；不传 `--config` 且默认路径都不存在 → 正常按 env 启动。
6. 缺少多个必填项 → 一次性列出全部缺失项（含 TOML 键与 env 名）。

**G2 可信代理**
7. 默认配置下，请求带 `X-Forwarded-For: 1.2.3.4` 不改变限流键（单测）。
8. `trusted_proxies: ["10.0.0.0/8"]`：来自 `10.0.0.5` 且 `X-Forwarded-For: 203.0.113.7` → 限流键为 `203.0.113.7`；来自 `198.51.100.9`（不可信）且带同样头 → 限流键为 `198.51.100.9`。
9. 来自可信代理且 `X-Forwarded-Proto: https` → 登录响应的 Cookie 带 `Secure`；不可信来源带同样头 → 不带 `Secure`。
10. 同一可信代理后两个不同客户端 IP，各自失败 10 次互不影响对方（不再共用一个桶）。

**G3 GHCR**
11. `goreleaser check` 通过；`release.yml` 含 `packages: write` 与 ghcr 登录步骤，且 `git grep -n -E 'docker\.io|DOCKERHUB' -- .github .goreleaser.yml 'Dockerfile*' docker-compose.yml` 无结果（`docs/` 里 PRD 的否定性说明不计）。
12. 推送 `v1.1.0-rc.1` 后：`docker manifest inspect ghcr.io/nerdneilsfield/aumbry:1.1.0-rc.1` 同时包含 `linux/amd64` 与 `linux/arm64`；**不存在** `latest`、`1.1`、`1` 标签的更新。
13. 推送正式 `v1.1.0` 后：`1.1.0`、`1.1`、`1`、`latest` 均可拉取；`docker run --rm --entrypoint /usr/local/bin/aumbry ghcr.io/nerdneilsfield/aumbry:1.1.0 version` 输出 `aumbry 1.1.0`（`ENTRYPOINT` 是 `aumbry-server`，必须覆盖才能运行 CLI）。
14. 镜像以非 root 运行；`docker run --rm -v $PWD/config.toml:/etc/aumbry/config.toml:ro ghcr.io/nerdneilsfield/aumbry:1.1.0 check-config` 退出码 0（`ENTRYPOINT` 为 `aumbry-server`，无子命令时才启动服务）。

**G4 富文本**
15. 6.6 的示例文档 `aumbry validate` 退出码 0，推送后在阅读页逐项渲染（人工检查清单：claim 徽章、公式、流程图、对比表 `na` 灰底）。
16. `version="1.0"` 文档中使用 `<claim>` → `validate` 退出码 1，错误码 `TAG_REQUIRES_1_1`；所有 v1.0 既有测试与示例文档仍通过。
17. 逐项负例（单测）：重复 `id` → `DUPLICATE_ID`；`<chart>` 含 NaN 或 `series` 长度不一致 → `INVALID_CHART` 且指出 JSON 路径；`<tabs>` 只有 1 个 tab → 校验失败；`<ref url="javascript:…">` → `INVALID_URL`。
18. 全文搜索：搜索 `<equation>` 内的 LaTeX 片段命中 0 条；搜索 `<claim>` 正文命中该文章。
19. 含 `<mermaid>` 的文章：首屏网络请求中不含 mermaid 的 chunk；打开该文章才加载（浏览器网络面板检查）。语法错误的 Mermaid 显示源码与错误提示，不让页面白屏。
20. `<md>` 中 `[x](#fig-arch)` 跳转到对应图；`[x](/\evil.com)` 仍被拒绝（沿用 v1.0 的 `safeUrl`）。

**G5 交互场景**
21. 同一 `--key paper:2611.01234` 先后 push 三次（第一轮文档只含一个 `<stage id="r1">`，之后同键逐轮追加 `r2`、`r3`）：返回同一个 `id`，`created` 依次为 `true/false/false`；第一次的文档通过校验且不出现“继续”按钮；TOC 依次出现 1、2、3 个阶段，读者已有的进度保留。另测“逐轮追加顶层 `section`”同样可行。
22. `aumbry push --draft`：返回 `draft:true`、`published:false`、`dirs:[]`；访客访问 `/r|p/{id}` 为 403，管理员可见；再 `push --key 同键 --dir <公开目录>` 后访客可见，且 `id` 不变。
23. `aumbry push --draft --dir X` → 参数错误，退出码非 0，且不联网。
24. `aumbry dirs` 返回的 `id` 可直接用于 `--dir`；响应中不含密码哈希（单测断言）；无 Token → 401。
25. `aumbry schema --format json` 输出可被 JSON 解析，且包含全部 6.2 所列标签，以及 6.2 所列每个新增属性（单测按协议定义逐项比对，防漏）。

**G6 Skill**
26. `skills/aumbry/assets/examples/*.xml` 五个示例逐个 `aumbry validate` 退出码 0（CI 执行）。
27. `make skill-check`：重新生成 `references/xml-tags.md` 后 `git diff --exit-code` 无差异；协议改动但没更新参考文件时该检查失败。
28. `SKILL.md` frontmatter 含 `name: aumbry` 与以 “Use when” 开头的 `description`，并写明排除条件；正文包含 8.3 的 9 条硬规则，以及 5 条标明“可按内容偏离”的默认做法。
29. Release 压缩包内含 `skills/aumbry/SKILL.md`（`tar tzf` 检查，且只出现一份）。
30. 演练：把 `skills/aumbry/` 和 `assets/examples/paper-reading.xml` 交给一个没有上下文的 Agent，要求“把这篇论文的第一轮精读发布到 Aumbry”，其产物一次或在按 `validate` 报错修复后通过校验并成功 push（记录一次演练结果即可，不作为 CI 项）。

**G4 补充（富文本边界）**
31. 位于 `<step>`、`<tab>`、`<figure-group>` 内的 `<figure>`：本地文件缺失时 `validate` 报 `MISSING_ASSET`；推送后路径被重写为 `/api/v1/assets/{hash}`（与位于 `section` 内的图片行为一致，单测覆盖每种容器）。
32. `<chart>`：`data` 长度与 `categories` 不一致、`pie` 含多个 series、`categories` 为空、数值为 NaN → 均 `INVALID_CHART` 且指出 JSON 路径。
33. `<ref doi="10.48550/arXiv.2611.01234">` 通过；`doi="https://doi.org/x"` → `INVALID_DOI`；渲染出的链接为 `https://doi.org/10.48550/arXiv.2611.01234`。
34. 一篇文档含 11 个 `<mermaid>` → `TOO_MANY_BLOCKS`。
35. 带恶意 `series[].name`（如 `<img src=x onerror=alert(1)>`）的 `<chart>`，悬停 tooltip 时不执行脚本、以文本显示（jsdom 或浏览器检查）。
36. `<code-block diff="true">` 的 `+`/`-` 行着色且有复制按钮；`<quote url="javascript:…">` → `INVALID_URL`；`<check status="maybe">` → `INVALID_CHECK`。
37. 在 `version="1.0"` 文档中使用 `<figure id="a">`、`<cell tag="evidence">` 或 `<paper-card doi="…">` → 均报 `TAG_REQUIRES_1_1`。

**G5 补充（Agent 命令）**
38. `aumbry list --key paper:2611.01234` 返回该条目的 `id` 与当前 `dirs`，响应中不含 XML 正文；无 Token → 401。
39. `aumbry unpublish <id>` 后访客访问该条目为 403、管理员可见、`id` 不变；`aumbry delete <id>`（无 `--yes`）退出码非 0 且不联网；加 `--yes` 后条目与其 `items_fts` 行均删除。
39a. 所有权：Token A 创建的条目，Token A 执行 `unpublish`/`delete` 成功；Token B 执行 → `403 not_owner` 且条目不变；管理员删除不受限；v1.0 遗留条目（`created_by` 为 `NULL`）对任何 Token 都是 `403 not_owner`；Token A 用同一 `external_key` 更新条目后，`created_by` 仍是创建者。数据库中不出现 Token 明文（单测检查）。
40. `aumbry push --dry-run`：不写存储与数据库（推送前后对象数与条目数不变）；Token 无效或目录 ID 不存在时退出码非 0。
41. `aumbry push --create-only --key K`：键已存在 → `409 key_exists` 且内容未被覆盖；两个并发请求同时 `--create-only` 创建同一个新键 → 恰好一个成功、另一个 `409 key_exists`（并发单测，依赖数据库唯一约束而非先查后写）。
42. 已发布条目执行 `push --draft`：服务端 `409 already_published`，条目内容与挂载均不变；CLI 退出码非 0，并提示改用 `unpublish` 或去掉 `--draft`。
43. `aumbry dirs` 对“自己无密码、但祖先有密码”的目录返回 `has_password:false, protected:true`（单测）。
44. `<md>` 内未包 CDATA 且含裸 `&` 或泛型尖括号的文档：`validate` 的 `INVALID_XML` 带 `hint`，内容提示使用 CDATA。

**G7 读者侧**
45. 访客在目录 A（含受保护子目录 B，未解锁）里按 `tag=World Model` 筛选：结果不含 B 内条目；`q` 为空 + `type=post` 返回该目录可见范围内的精读列表（单测）。
46. 视口宽度 375px 时，阅读页能通过浮动按钮打开章节抽屉并跳转（浏览器检查）。
47. 打印预览（`@media print`）：白底、无 TOC 与站头，所有 `<collapse>`/`<tab>` 内容均展开。
48. 打开 `…/p/{id}#fig-x`，而 `fig-x` 位于收起的 `<collapse>` 内：页面自动展开并滚动到该图。

**G8 运维与安全**
49. WebDAV 不可达时：`/healthz` 为 200，`/readyz` 为 503 且 `checks.webdav` 含错误信息；恢复后 `/readyz` 为 200。
50. 任一响应（含 404、429、`/api/v1/assets/*`、`/items/{id}/content`、SPA 回退页）都带 `Referrer-Policy: no-referrer`、`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`；资源与内容端点原有的沙箱 CSP 不被覆盖。
51. `/api/v1/ingest` 同一 IP 第 121 次请求（1 分钟内）→ 429；默认配置下伪造 `X-Forwarded-For` 轮换来源**不能**绕过该限流（键仍是 TCP 对端）。
52. 服务运行并持续写入时执行 `aumbry-server backup --out b.db`：`sqlite3 b.db "PRAGMA integrity_check"` 输出 `ok`，且 `items` 行数不少于备份开始时的行数；用该快照加配对的 WebDAV 内容启动新实例，`verify-storage` 退出码 0。
53. 人为删除一个 WebDAV 上的图片对象后 `aumbry-server verify-storage` 列出该缺失项，退出码非 0。
54. 构造一个无 `item_assets` 引用的资产：`gc --dry-run` 列出它而不删；`gc` 删除其对象与 `assets` 行；仍被引用的资产不受影响；对象已上传但从未进入 `assets` 表的残留同样被列出。服务正在运行时执行不带 `--dry-run` 的 `gc` 会被拒绝或要求显式确认离线（按 10.6 的实现选择，测试断言其一）。
55. WebDAV 响应挂起超过 30 秒时，`POST /api/v1/ingest` 以 5xx 返回而不是无限阻塞（用延迟响应的测试 WebDAV 服务验证）。
56. 升级：用 v1.0 构建产生的数据库与对象（固定 fixture，含无 `created_by` 的遗留条目、带受保护目录的树）启动 v1.1：迁移成功，旧条目仍可按原权限访问，`push --key` 更新旧条目成功且 `created_by` 仍为 `NULL`，既有 Cookie 解锁状态不失效。
57. 默认配置下，来自任意 TCP 对端、带 `X-Forwarded-For`/`X-Forwarded-Proto` 的请求：登录限流键为对端地址，Cookie 不带 `Secure`；`trusted_proxies` 非空但对端不在其中时同样忽略这些头（覆盖 4.2 的 TCP 对端校验，不依赖 chi 自身行为）。

**G9 分阶段阅读、互动与更多场景**
58. 3 个 `stage` 的文档：首次打开只展开第 1 阶段；点击“继续”后第 2 阶段出现并滚动到位；刷新后保持；`#stage-r3` 深链接直接解锁并展开第 3 阶段；“展开全部”可关闭门控；`gated="false"` 默认全部展开。
59. 锚点 `#fig-x` 位于未解锁的第 3 阶段：自动解锁并定位；`@media print` 下全部阶段、`tab`、`collapse` 均展开（含从未打开过的）。
60. 向已发布条目追加第 4 个 `stage`（同键再 push）：读者此前的阶段进度与 `quiz` 作答保留；改名已有 `stage id` 后，对应进度被忽略而不报错。只调换某道 `quiz` 的选项顺序或改正确答案后，该题的旧作答被清除（指纹不同），阶段进度保留；`localStorage` 里是损坏的 JSON 时页面照常打开。
61. `<quiz>`：无正确选项、选项少于 2 个、缺 `id` → `INVALID_QUIZ`；单选与多选分别用 radio 与 checkbox；提交后错选/漏选逐项标红、显示解析；阶段得分“答对 x/y”；题干以转义文本或 CDATA 写入 `<img onerror=…>` 时只显示为文本（真正的子元素按非法结构校验失败）。
62. `<next-steps>` 的 `target="#不存在"`、`target="#sec-3"`（按位置的锚点）、`target="https://…"`、`javascript:…`、`target="/admin"` → `INVALID_TARGET`；合法 `target` 为 6.4 表中的前缀锚点；点击指向未解锁阶段的选项会先解锁，页面不发生整页跳转（`<button>`，无 `href`）。
63. 嵌套 `<stages>`、缺 `id` 的 `<stages>`、单组 9 个或 0 个 `stage` → `INVALID_STAGES`；跨组或跨类型重名的 `id`（如 `stage id="x"` 与 `figure id="x"`）→ `DUPLICATE_ID`；9 组各 2 个 `stage`（超组数）与 8 组合计 41 个 `stage`（超总数）分别 → `TOO_MANY_BLOCKS`；`version="1.0"` 文档使用上述任一标签 → `TAG_REQUIRES_1_1`。
63a. 同一文档里两组独立的 `<stages>`（一组在 `body` 开头，一组在某个 `section` 内）：各自有步骤条与进度，一组点“继续”不影响另一组；锚点落在另一组的未解锁阶段时，只把那一组解锁到目标阶段；两组进度各自在刷新后保持。
64. `<steps interactive="true">`：一次只显示一步，键盘左右键与上一步/下一步可用；只有 1 个 step → 校验失败；打印时全部展开。
65. `<cell tag="unreported">` 渲染为“未报告”且不含数字 0；`<cell tag="approx">` 带“≈ 图读”角标；非法 `tag` → 校验失败。
66. （P2/M5）`<chart>` 的 `series[].err`：长度与 `data` 不同、含负数或 NaN、用于 `pie`/`scatter` → `INVALID_CHART` 且指出 JSON 路径；合法时图上有误差棒。
67. `<code-block eq="loss">`：存在 `<equation id="loss">` 时显示“对应公式”并跳转；不存在 → `INVALID_TARGET`。
68. `<search-log>`：缺少 `<query>` → 校验失败；渲染为默认折叠卡片，标题栏显示检索式条数与纳入数（纳入数为各查询 `kept` 之和，标注“各查询合计”，不去重）；`excluded` 的理由进入全文索引。
69. `aumbry schema --format json|md` 输出一张按 `version` 区分的规则表：含所有新增容器的允许父子关系、数量上下限、属性、位置例外与错误码，`1.0` 与 `1.1` 的差异可直接读出（单测）。

**G10 摘要、速读与新闻**
70. `<tldr>`：`body` 级 2 个、`section` 级 2 个、`point` 为 0 个或超过 7 个、单条超过 300 字符 → `INVALID_TLDR`；`version="1.0"` 文档使用 → `TAG_REQUIRES_1_1`。
71. 渲染与索引：全文 `tldr` 在标题下方首屏；`section` 的 `tldr` 默认在标题后，`placement="bottom"` 时在末尾；TOC 抽屉显示各节首条要点；搜索某条 `point` 的文字命中该文档。
72. 速读模式：开启后只剩各节标题与 `tldr`，点“展开本节”才显示全文；`?skim=1` 刷新后保持；文档里没有任何 `tldr` 时不显示该开关；打印时忽略速读、全部展开；`body` 直接放置的正文块归入“展开正文”区；`?skim=0/1` 优先于本机记忆；锚点定位自动展开目标所在节；嵌套 `stage > section` 时子节的标题与 `tldr` 仍可见。
73. `<news-item>`：缺 `title` → `INVALID_NEWS`；`url="javascript:…"` → `INVALID_URL`；`detail-id` 不是 12 位 → `INVALID_ID`；`importance="high"` 有强调样式；`<detail>` 点击展开且其内文可被搜索。
74. `detail-id` 点击后进入该条目阅读页；目标条目对当前访客无权限时显示 403，且页面不出现目标条目的标题或摘要。
75. 演练：先推两篇新闻详情、再推带 `detail-id` 的简报；用同一个键重复推简报只覆盖不新增；周报示例推送后，每天一个 `tldr` 与全文 `tldr` 均正常渲染。

**G11 结构自由度**
76. 自由结构（`version="1.1"`，单测）：只含一个 `<md>` 的 `body` 通过；`body` 直接混排 `claim`、`chart`、`metrics` 通过；`section` 嵌套 3 层通过、4 层 → `SECTION_TOO_DEEP`；一份不含 `tldr`/`stages`/`metrics` 等任何可选积木的文档通过。同样的 `body` 直接放 `claim` 在 `version="1.0"` 下仍按 v1.0 原规则失败；`version="1.0"` 下 `body > md` 也失败（只违反旧容器规则，与新标签门控分开测），行为不变。`quiz`、`next-steps`、`news-item`、`tldr` 直接放在 `body` 下通过。
77. 示例之外的结构：用一份与所有示例都不同的文档（章节名自拟、顺序颠倒、混用 `news-item`、`claim`、`chart`）推送并正确渲染，TOC 按自拟章节生成。
78. 演练（记录结果，不进 CI）：给没有上下文的 Agent 一个与示例结构不同的需求（如“只要一页结论加两张图”），产物不得硬塞示例里没用到的章节或空的 `tldr`/`metrics`，且通过校验并成功 push。另加一份篇幅达到 `tldr` 默认阈值、但用户明确只要正文和图的演练：产物不被硬塞 `tldr`。
79. 兼容（单测加集成）：v1.0 构建产生的已入库文档，读取与搜索不变且无需重建索引；v1.1 CLI 向 v1.0 服务端推 `version="1.1"` 文档 → 422 `UNSUPPORTED_VERSION`，不写对象与数据库；v1.1 服务端对重新上传的 `version="1.0"` 文档（带 `<figure alt>`）→ `TAG_REQUIRES_1_1` 且带 `hint`（有意取舍，见 6.1）。
80. 锚点：`#stage-r3`、`#fig-x`、`#s-method`、`#quiz-q1` 都能被 `next-steps` 命中；位置锚点 `#sec-3` 不能。目标位于未解锁阶段的第 2 个 `tab` 内的收起 `collapse` 中时，按 6.4 的“定位顺序”完成后再滚动并可见（浏览器检查，含跨组用例）。
81. 搜索：只出现在 `<quiz>` 选项或 `next-steps` 按钮里的词搜不到；只出现在另一组第三阶段正文里的词命中文章（但点击只打开文章）；`search-log excluded@reason`、`quote@author`、`check@detail` 里的词可命中；长词（FTS）与 2 字短词（`LIKE`）各有一条。
82. 中文检索：入库正文含“大模型训练与评测”的文档，搜“模型”命中且该查询打在 `items_fts_cjk` 上（单元测试断言生成的 SQL 用了该表，而不是 `LIKE`）；搜“训练”“练与”“大模型”均命中；搜“型训”“与评”也命中；不含这些字的文档不命中。
83. 二字组不跨标点：正文为“模型，训练”时，搜“型训”不命中；搜“模型”“训练”各自命中。
84. 单个汉字（如“型”）仍可通过 `LIKE` 找到；中英混排的查询“GPT 模型”要求两段同时命中。
85. 命中片段取自原文：搜“模型”得到的 `snippet` 含原文上下文与高亮标记，不含二字组展开后的重复文本（如不出现“大模 模型 型训”）。
86. 排序：标题含“模型”的文档排在只在正文出现“模型”的文档之前。
87. `aumbry-server reindex` 能在手工清空 `items_fts_cjk` 后恢复它，恢复后 82 的查询重新命中；新增、更新、删除条目后该表同步变化。
88. 阅读体验：用真实的中文报告，在搜索页输入“模型”，列表显示命中、高亮且无乱码（浏览器检查）。

---

## 14. 决策记录与未决问题

### 14.1 已定决策

| 决策 | 理由 | 否决的备选 |
| :--- | :--- | :--- |
| 逐轮增量发布靠“同一 `--key` 重复 push” | v1.0 已有 upsert，零新接口；本地草稿是唯一事实来源，Agent 无需回读服务端 | 新增“追加 section”接口；提供 `aumbry get` 回拉 XML（回拉出的图片 URL 已被重写，反而更难改） |
| 草稿 = 不挂载目录 | 复用 v1.0 的“未挂载仅管理员可见”，不加状态字段 | 给 item 增加 `status: draft/published` 列（要迁移、要改所有查询） |
| 新标签要求 `version="1.1"` | 旧模板不会悄悄混用；报错能直接教会 Agent 升版本 | 不分版本、新标签随时可用（旧服务端会渲染成未知标签） |
| `<chart>` 只收最小 JSON 结构 | 可用 JSON Schema 完整校验，无代码型配置，Agent 容易写对 | 透传 ECharts `option`（校验不了、能力过大） |
| 引用用 `#ref-<id>` 普通锚点 | 不新增 Markdown 语法，`safeUrl` 已支持同页锚点，Agent 零学习成本 | 自定义 `[@key]` 行内语法（需手写 markdown-it 规则，见 14.2） |
| `aumbry dirs` 走 API Token | Token 持有者本就是受信写入方；避免让 Agent 去用管理员 Cookie | 让用户手抄目录 ID；开放管理员接口给 Agent |
| 可信代理只认 XFF/XFwdProto | 覆盖 Nginx/Caddy/Traefik/Docker 常见反代，配置只有一个键 | 做“信任哪个头”的开关（Cloudflare 等另议） |
| 镜像用 GoReleaser `dockers_v2` + `Dockerfile.release` | 二进制与 Release 同源；无需手写 manifest | 沿用多阶段 `Dockerfile` 在 CI 里 buildx（二进制被重复编译，两处不一致） |
| 服务端配置用 TOML，CLI 配置仍是 YAML | 用户偏好 TOML；`~/.config/aumbry/config.yaml` 是 v1.0 已发布的格式，改它会破坏已有用户 | 两者都改成 TOML（需要 CLI 迁移读取旧 YAML 的兼容逻辑，见 14.2） |
| 只发 GHCR | 用户明确要求不发 Docker Hub | – |
| 标签/类型筛选只在已解锁子树内，不做全站 `/tags` | 守住“树外不可枚举”的权限边界 | 全局标签页（会泄露树外条目存在与标签） |
| `--draft` 不会让已发布条目下线，另设 `unpublish` | 动词语义单一，响应里的 `published` 让后果可见 | 让 `--draft` 隐式撤回挂载（一次误用就让公开内容消失，或相反） |
| Agent 只能删除/下线自己创建的条目，所有权 = 创建者 Token 的指纹（`items.created_by`） | 满足“Agent 可删自己写的”，又不让一个被注入的 Agent 删光别人的内容；复用 v1.0 已有的多 Token 配置，无需命名 Token | 用 XML 里的 `source-agent`（自报，可伪造）；命名/分级 Token（范围外）；只许管理员删 |
| Agent 专用 API 用 `/api/v1/agent/*` 且仍是单一 API Token | Token 持有者本就是受信写入方，不引入多级权限 | 给 Agent 开放管理员接口；命名/分级 Token（已在范围外） |
| 备份用 SQLite 原生 `VACUUM INTO` | 在线、WAL 安全、零额外依赖 | 直接拷贝 `.db`（WAL 下会丢数据）；引入第三方复制工具 |
| 写入口限流固定 120 次/分钟，不做配置项 | 防 Token 试探与泛洪，又不增加配置面 | 可配置限额（用户倾向简单） |
| ECharts tooltip 用纯文本、Mermaid SVG 再过 DOMPurify | 新标签带来的渲染层 XSS 是 v1.1 新增风险 | 只依赖 Mermaid 的 `strict` 级别 |
| Skill 参考文档由 `aumbry schema` 生成并在 CI 校验 | 协议与说明不漂移 | 手写参考文档 |
| `<stages>` 门控只做阅读节奏，不做访问控制；进度存浏览器 | 所有阶段本就随条目下发并被索引，做服务端门控要改权限模型和搜索；进度用 `id` 记录，逐轮追加不丢 | 服务端按轮次/时间解锁（见 14.2 第 2 条）；把进度存服务端（需要读者身份） |
| `<quiz>` 进 v1.1（P1），答案可见、状态只存浏览器 | 用户的 `interactive-guide`/`qi-read-paper` 工作流需要“每阶段小测验 + 选择下一步”；答案保密没有场景价值 | 推迟到 v1.2；服务端判分与统计 |
| 复现/实验/实现记录用现有标签加少量扩展属性（`unreported`/`approx`、`err`、`code-block eq`），不新增“复现”专用标签 | `paper-reproduce`、`algorithm-experiment`、`paper-to-code` 的产出本质是表格+指标+图+清单，扩展属性能覆盖，避免标签爆炸 | 新增 `<repro-table>`、`<deviation>` 等专用标签 |
| `<stages>` 允许一份文档有多组、可在文档中间或某个主题内使用，每组独立；但不允许嵌套 | 用户要“文档中间多次分阶段、每个主题各自分阶段”；嵌套门控会让进度键、锚点解锁顺序、TOC 分组的语义含混，而独立多组已覆盖这些用例 | 限制为全文一个 `stages`；允许 `stages` 嵌套在 `stage` 内 |
| 摘要用独立标签 `<tldr>`，不是 `section` 的属性或“约定第一段” | 要点/结论有结构，可被 TOC、速读模式、索引直接使用，校验器能数条数与长度 | `section summary="…"` 属性（只能一句话、无结构）；约定第一段是摘要（无法校验、无法折叠） |
| 协议是可自由组合的积木，不按场景规定必填骨架；示例只是参考 | 不同报告结构差异大，固定骨架会逼 Agent 为填表而塞空话、校验也会误杀合理文档 | “场景类型”字段 + 每类固定模板与必填章节；v1.0 的“`body` 只能放 `section`、`section` 只能放少数几种块”的僵硬容器规则（v1.1 放宽） |
| 单个 `stage` 也是合法的 `<stages>` | 逐轮发布的第一轮只有一个阶段；要求 ≥ 2 个会逼 Agent 写空占位或改结构 | 要求至少 2 个 `stage`（第二评审指出会阻断第一轮发布） |
| `<next-steps>` 是按钮，`target` 只接受页内前缀锚点，不经 `safeUrl` | `safeUrl` 本来就放行外链与同源路径，不能当“只允许页内锚点”的校验 | 沿用 `safeUrl` 渲染成链接 |
| `section` 增加可选 `id`，锚点 `#s-<id>`；位置锚点 `#sec-N` 不可作为 `next-steps` 目标 | 追加内容会让位置变化；稳定目标需要 `id` | 只用位置锚点 |
| 示例精简为 5 个结构差异明显的文件，其余场景只写映射 | 示例太多等于模板，且要在 CI 里逐个维护 | 每个场景一个示例（12 个） |
| 新闻详情是独立条目，简报用 `detail-id` 链接，不做内联长文 | 详情可单独检索、分享、设权限，简报保持轻量 | 全部内联成 `collapse`（简报膨胀、无法单独分享）；服务端按 key 解析链接（要新接口） |

### 14.2 未决问题（不在本版擅自决定）

1. **行内引用语法 `[@key]`**：是否值得为 `research-report` 的 `[@note-slug]` 习惯做一个 markdown-it 规则？现方案用锚点链接已够用；需要时再评估是否有成熟插件。**决定人：用户。**
2. **是否需要服务端层面的“按轮次/按时间解锁”**：目前 `<stages>` 门控只在浏览器里做节奏控制（6.7）。若分享给学员/同事时要求“未发布的轮次读者根本拿不到”，现有做法就是**只推已完成的轮次**（逐轮 push 本来就是增量），不需要新机制；只有“同一份内容按时间自动开放”才需要服务端支持，本版不做。**决定人：用户。**
3. **配置文件里的 secrets 间接引用（`*_FILE`）**：Docker/K8s secrets 的常见做法，本版不做；若部署需要再加。
4. **Cloudflare 等需要读取非 XFF 的真实 IP 头**：本版不支持；有需要时加 `client_ip_header`。
5. **Mermaid 语法在发布前无法由 Go 侧校验**：目前只检查非空与长度；是否要求 Skill 提示 Agent 本地用 `mmdc` 预渲染，待演练后决定。
6. **是否默认开启“每轮自动存草稿”**：现为“用户要求才发布”；是否给 skill 增加用户级开关，视使用习惯再定。
7. **所有权是否也约束内容覆盖**：现在只约束下线与删除，`push --key` 仍可更新他人条目的内容（v1.0 行为）。若多个 Agent 共用同一服务且互不信任，应把所有权扩展到更新（对 `created_by` 为 `NULL` 的遗留条目需要迁移策略）。**决定人：用户。**
8. **暂不做的项**：`push --batch`、`push --replace-dirs`、Prometheus `/metrics`、结构化审计日志、访客解锁 Cookie 的容量上限（约 80–150 个目录后会超 4 KB）。需要时再评估。
9. **CLI 配置是否也改 TOML**：服务端用 TOML 而 CLI 仍是 YAML，格式不统一。若要统一，CLI 需要同时读取旧的 `config.yaml`（兼容）并改写为 `config.toml`。**决定人：用户。**
10. **新闻详情页是否自动显示“来自哪份简报”**：现在靠详情稿的 `meta/related-ids` 手动指回简报（需在简报推送后补推一次详情）。要自动显示，服务端需要从 `detail-id` 建反向索引，本版不做。**决定人：用户。**

### 14.3 评审记录（外部评审的处理）

v1.1 草稿经一次外部评审（`faed5dfe…`，只读），逐条对照当前代码核实后的处理：

| 评审意见 | 处理 |
| :--- | :--- |
| `/items/{id}/content` 缺 CSP、`accessible` fail-open、`/\evil.com` 放行、`caarlos0/env` 不存在、`safeUrl` 不存在 | **不采纳**：均为读了修复前的旧文件，这些已在 v1.0 的修复提交里解决（当前代码已核实） |
| 渲染层 XSS（Mermaid SVG、ECharts tooltip）、每篇块数上限 | 采纳 → 6.5 |
| 安全头（`Referrer-Policy`、`X-Frame-Options`）、`/ingest` 限流 | 采纳 → 10.3、10.4 |
| `code-block`、`quote`、`checklist`、`paper-card`/`repo-card` 扩展属性、`search-log` | 采纳 → 6.2（`search-log` 为 P2） |
| 标签/类型筛选、移动端 TOC、打印/导出、主题、邻近条目 | 采纳 → 第 9 章（导出/主题/邻近为 P2） |
| `list`/`unpublish`/`delete`、`--create-only`、`--dry-run`、`hint`、两个新模板、本地草稿目录约定 | 采纳 → 7.1、7.4、7.5、8.2、2.3；`delete`/`unpublish` 按用户要求限定为“Agent 只能处理自己创建的条目”（所有权见 7.5） |
| `--replace-dirs`、`push --batch`、Prometheus `/metrics`、审计日志、Cookie 容量上限 | 暂不做 → 14.2 第 8 条 |
| `/readyz`、WebDAV 超时、`VACUUM INTO` 备份、`gc`、`reindex`、单实例说明 | 采纳 → 第 10 章 |
| PRD 缺口：`figureXPath` 未覆盖新容器、`PlainText()` 与 6.5 矛盾、`doi` 不是 URL、`chart` 等长约束 JSON Schema 无法表达、`protected` 继承字段、`--draft` 语义含糊、`defineAsyncComponent` 拆分、压缩包只放一份 skill、`external_key` 状态码、`aumbry-server` 无命令行 | 采纳并已补入对应章节 |
| 建议数据库迁移改用 goose | 已在 v1.0 修复提交中完成 |

v1.1 草稿又经第二次外部评审（`0b047a7c…`，Codex，只读），同样逐条对照当前代码与依赖源码核实：

| 评审意见 | 处理 |
| :--- | :--- |
| chi v5.3.0 的 `ClientIPFromXFF` 不校验直连对端，伪造 XFF 可绕过限流 | **采纳**（已读 `middleware/client_ip.go` 确认：只遍历 `X-Forwarded-For`，不看 `RemoteAddr`）→ 4.2 增加 TCP 对端前置校验；验收 7–10、57 |
| `--draft` 可让已发布条目的语义含糊 | 采纳 → 已发布条目返回 `409 already_published`；验收 42 |
| `--create-only` 应由服务端原子裁决，不能先 list 再 push | 采纳 → 7.1 的 `create_only` 表单字段，唯一约束保证并发；验收 41 |
| `created_by` 存 Token 明文或短指纹的风险 | 采纳 → 存规范化 Token 的完整 SHA-256；验收 39a 检查库中无明文 |
| 镜像内置 `AUMBRY_*` 会覆盖 TOML | 采纳 → 3.3、5.1：`Dockerfile.release` 不内置 env；修正了 5.1 中残留的 `AUMBRY_DATA_DIR=/data` |
| 运行中 `gc` 会误删刚上传的对象；`backup` 不含 WebDAV | 采纳 → 10.5、10.6：破坏性 `gc` 仅限离线，备份只含数据库并要求配对恢复与 `verify-storage` |
| GoReleaser 默认值会改变产物（`ids`、平台子目录、SBOM、`archives.files` 覆盖默认文件、RC 浮动标签） | 采纳 → 5.2；验收 11–14 |
| 验收缺升级路径、并发、XFF 伪造、全响应安全头 | 采纳 → 新增验收 56、57，并收紧 41、50、51、54 |
| 把所有权扩展到内容覆盖 | **未擅自决定** → 14.2 第 7 条交用户 |

**用户对 PoC 的反馈（2026-11-21）**：第一版 PoC 只覆盖静态富文本，没有“多阶段”和“交互选项”；并指出相关 skill 不止 `qi-read-paper`/`interactive-guide`。逐个核对 `research-report`、`paper-compare`、`paper-search`、`paper-reproduce`、`paper-to-code`、`algorithm-experiment`、`paper-audit` 的产出结构后的处理：

| 发现的缺口 | 处理 |
| :--- | :--- |
| 没有分阶段阅读与“继续/跳转/深入”（`qi-read-paper`、`interactive-guide`） | 采纳 → 6.7 `<stages>`、`<next-steps>`；2.3 改为每轮一个 `stage`；验收 58–60、62、63 |
| 没有测验（`interactive-guide`） | 采纳，从 P2 提到 P1 → `<quiz>`；验收 61 |
| `paper-compare` 要求“未报告 ≠ 0”“图中读数需标注” | 采纳 → `cell tag` 增 `unreported`/`approx`；验收 65 |
| `algorithm-experiment` 要求报告多种子的离散程度 | 采纳 → `chart` 增 `series[].err`；验收 66 |
| `paper-to-code` 的公式↔代码映射、偏差记录 | 采纳 → `code-block eq`、场景 H（`compare-repro.xml` 示例）；验收 67 |
| `paper-search` 的检索日志含纳入/排除标准、排除理由、停止规则 | 采纳，从 P2 提到 P1 → `<search-log>` 扩展；验收 68 |
| `paper-reproduce` 的“目标与容差在结果之前、逐条结论” | 采纳 → 场景 G（`compare-repro.xml` 示例）、skill 默认做法 4；暂不新增专用标签 |
| `research-report` 的 `[@note-slug]` 引用 | 不新增语法：由 Agent 装配时替换为 `#ref-` 锚点（2.4 D） |
| `paper-audit` | 场景 I：只用现有标签，不设专门示例 |
| 想加 `<flow>` 调用流动画与 `<graph>` 知识图谱（`interactive-guide`） | **暂不做**：`<mermaid>` 与交互式 `steps` 已覆盖需求；图谱需要整套新依赖，另议 |
| 每日/每周简报、新闻与新闻详情（用户补充） | 采纳 → 场景 K/L、`<news-item>`（`detail-id`、内联 `<detail>`）、示例 `news-digest`/`news-detail` 两个示例（周报同构于简报）；验收 73–75 |
| 内容分成很多部分，每部分要有总结，全文也要有总结（用户补充） | 采纳 → `<tldr>`（全文 + 每部分）、6.8、速读模式 9.7、skill 默认做法 2；验收 70–72 |
| 要给 Agent 自定义空间，不要做成只能填空的模板（用户补充） | 采纳 → 新增 6.0 设计原则；放宽容器规则（`body` 可直接放块、`section` 嵌套 3 层）；“模板”改称“示例”（`assets/examples/`）并声明不是必填骨架；`stage id`/标题、`tldr`、`quiz`、`next-steps` 等由“必须/固定”改为“可以/默认”；skill 原则（默认做法 1）；验收 76–81 |
| `stages` 只能在 `body` 下且至多一个（用户要求放宽） | 采纳 → 可有多组、可放在 `body`/`section` 内，每组独立；仍不嵌套；验收 63、63a |

### 14.3b 第二轮外部评审（针对 v1.1 新增内容）

`faed5dfe…`（gemini）与 `0b047a7c…`（codex）各出一份只读评审，逐条对照当前代码核实后的处理（均已读 `protocol.sch`、`validator.go`、`url.ts`、`xml.ts`、`ReaderView.vue`、`config.go` 确认）：

| 评审意见 | 处理 |
| :--- | :--- |
| `<stages>` 要求 ≥ 2 个 stage，第一轮单独发布会被拒（两方一致） | 采纳 → 1–8 个；验收 21、63 |
| `protocol.sch` 块级规则写死 `/document/body/section/...`，放宽容器后会静默漏检；`urlXPath` 覆盖不全；1.0 拦截与 `UNKNOWN_TAG` 重复 | 采纳（已核实）→ 新增 6.9 |
| `next-steps` 写“经 `safeUrl`”，而 `safeUrl` 放行外链与同源路径 | 采纳（已核实）→ 按钮 + 前缀锚点校验；验收 62 |
| 锚点前缀与“已存在的 `id`”表述不一致；`section` 没有 `id`；`id` 命名空间 | 采纳 → 6.4 锚点映射表、`section@id`；验收 63、80 |
| 2.3/2.4 与 8.3 仍有“必须/每个”类措辞，与 6.0 冲突；硬规则含场景布局 | 采纳 → 2.3/2.4 改为建议写法；8.3 拆成 9 条硬规则 + 5 条默认做法 |
| `quiz`/`next-steps` 不能放 `body` 下，与 6.0 矛盾 | 采纳 → `body`/`section`/`stage` 同一套块集合 |
| 速读对 `body` 直属正文、嵌套章节、锚点跳转、`?skim` 与本机记忆的优先级没定义；TOC 无层级 | 采纳 → 9.7、9.2；验收 72 |
| 错误码清单与触发条件缺失、字符上限按字节还是 rune、`INVALID_METRIC` 条件 | 采纳 → 7.4、6.2 |
| 搜索只能定位文章而非阶段；属性值未入文本投影；`next-steps` 按钮文字是否入索引 | 采纳 → 6.5；验收 81 |
| v1.0 兼容边界：已存文档、重新上传的门控差异、v1.1 CLI → v1.0 服务端 | 采纳 → 6.1 兼容边界；验收 79 |
| `quiz id` 保留但内容变化时作答错位；存储结构与损坏 JSON | 采纳 → 6.7 题目指纹、存储结构 |
| 里程碑缺 9.7；打印 P2 与 M4 矛盾；#69 与 #26 重复 | 采纳 → 打印提到 P1、M4 增 9.7；#69 改为 `schema` 规则表验收 |
| 精简首发：后移 `series[].err`；12 个示例收成 5 个 | 采纳 `err` → P2/M5；示例 → 5 个。**不采纳**砍掉 `err` 本身：`algorithm-experiment` 要求报告多种子离散程度，P2 保留 |
| `koanf` 与现有 `caarlos0/env` 的键映射不同构；`envDefault` 会覆盖 TOML | 采纳（已核实 `config.go`）→ 3.3 改为 go-toml/v2 + 已有 env，不新增 koanf |
| `@shikijs/transformers` 不处理 unified diff | 部分采纳：库行为为推断；6.2 改为 unified diff 用已加载的 shiki `diff` 语言、行高亮用 transformers，实现时以实际渲染验证 |
| 锚点注册表与 `forceMount`、避免各容器自行监听 `hash` | 采纳 → 6.4 定位顺序与实现说明 |
| 验收编号含 `39a`/`63a` | 保留：可引用，不重排 |

### 14.4 v1.0 路线图的调整

* `<chart>`、`<mermaid>` 从 v2.0 前移到本版（P0）。
* v2.0 保留：MCP Server、外链图片自动转存。
