# Aumbry 校验与推送排错参考（Troubleshooting）

> 使用 `aumbry validate <file.xml>` 默认输出 JSON（含 `line`、`code`、`field`、`message` 及可选 `hint`）；逐条按 `code` 与 `hint` 就地修改后再校验。

## 1. 校验错误码全集（来自 `pkg/schema` 与 `internal/api`）

### XML 与根结构（v1.0+）

| 错误码 | 触发原因 | 修复方式 |
| :--- | :--- | :--- |
| `INVALID_XML` | XML 不良格式、未知实体（如 `&nbsp;`）或含 `DOCTYPE` 声明 | 检查标签闭合与移除 `<!DOCTYPE>`；若 `hint` 提示 `CDATA`（如 `<md>`/`<mermaid>`/`<chart>`/`<code-block>` 内有裸 `&` 或泛型 `<T>`），把正文包进 `<![CDATA[ ... ]]>` |
| `INVALID_ROOT` | 根元素不是 `<document>` | 根节点改为 `<document version="1.1">`（或 `"1.0"`） |
| `UNSUPPORTED_VERSION` | `document@version` 不是 `"1.0"` 或 `"1.1"` | 设为 `"1.1"`（或旧版 `"1.0"`） |
| `UNEXPECTED_TEXT` | 结构容器（`document`/`body`/`section`/`stage`/`collapse`/`tab`/`step`/表格/列表容器）直接放了非空白文本 | 把正文放进 `<md><![CDATA[...]]></md>` |
| `MISSING_META` / `DUPLICATE_META` | `<document>` 缺少 `<meta>` 或存在多个 `<meta>` | 确保 `<document>` 内恰好有 1 个 `<meta>` |
| `MISSING_BODY` / `DUPLICATE_BODY` | `<document>` 缺少 `<body>` 或存在多个 `<body>` | 确保 `<document>` 内恰好有 1 个 `<body>` |
| `REQUIRED_FIELD` | `<meta>` 缺少或空白：`title`/`type`/`date`/`summary`/`source-agent` | 补全全部 5 个必填 `<meta>` 子元素 |
| `INVALID_TYPE` | `<meta>/<type>` 不是 `report` 或 `post` | 改为 `report` 或 `post` |
| `INVALID_DATE` | `<meta>/<date>`、`event@date`、`news-item@published`、`search-log@date` 或 `query@date` 格式或日历日期无效 | `<meta>/<date>` 与 `search-log`/`query` 用 `YYYY-MM-DD`；`event@date` 可用 `YYYY`/`YYYY-MM`/`YYYY-MM-DD`；`news-item@published` 可用 `YYYY-MM-DD` 或 RFC 3339 |
| `INVALID_TAG` | `<meta>/<tags>` 含非 `<tag>` 或空 `<tag>` | 只放非空 `<tag>...</tag>` |
| `INVALID_ID` | 两类 ID 违规：(1) `related-ids/id`、`paper-card@related-id`、`news-item@detail-id` 不符合 12 位条目 ID（`^[A-Za-z0-9_-]{12}$`）；(2) `section`/`stages`/`stage`/`quiz`/`figure`/`equation`/`ref` 的 `@id` 不符合 `^[a-z0-9-]{1,32}$` | 条目 ID 必须来自真实 `push` 或 `aumbry list`；锚点 `id` 仅用 1–32 位小写字母、数字和 `-` |
| `UNKNOWN_TAG` | 当前容器不允许该子标签，或叶子块内包含子 XML 元素 | 参照 [xml-tags.md](xml-tags.md) 修正父子容器；若想写代码/泛型/HTML 示例，放入 `<![CDATA[...]]>` |
| `MISSING_ATTR` | 缺少必填属性或属性为空（如 `section@title`、`figure@src`、`paper-card@title/url`、`repo-card@name/url`、`source@title/url`、`ref@title`、`term@name`、`event@title`、`figure-group@cols`） | 补齐对应必填属性 |
| `INVALID_ATTR` | 属性枚举值不合法（如 `callout@type`、`claim@level`、`cell@tag`、`read-depth`）或数量/重复限制（`<tabs>` <2 个、`<steps interactive="true">` <2 个、`<glossary>` 重名 `term`、`figure-group` 非 `2..6` 图、`<next-steps>` 超出 `1..5` 项） | 对照 [xml-tags.md](xml-tags.md) 的枚举值与数量上下限修正 |
| `INVALID_URL` | `canonical-url`、`paper-card@url/code-url`、`repo-card@url`、`source@url`、`ref@url`、`quote@url`、`news-item@url` 不是绝对 `http://` 或 `https://` URL | 补全为 `http://` 或 `https://` 绝对 URL（页内跳转请用 Markdown 锚点或 `<next-steps>`） |
| `INVALID_ASSET_PATH` | `<figure src>` 为外链、`data:`、绝对路径或用 `..` 逃逸出 **XML 文档自身所在目录** | 改为相对于 XML 文档自身目录（而非当前工作目录）的路径，如 `./imgs/fig.png` |
| `INVALID_ASSET_TYPE` | `<figure src>` 扩展名不是 `.png`/`.jpg`/`.jpeg`/`.webp`/`.svg` | 转换为支持的图片格式 |
| `MISSING_ASSET` | 按 **XML 文档自身所在目录**解析 `<figure src>` 后本地文件不存在（`step`/`tab`/`figure-group` 内同样检查） | 将图片放在文档同级相对路径（如 `./imgs/...`）并核对文件名；注意 `validate` 不以当前工作目录（CWD）为基准 |
| `INVALID_ASSET` *(服务端)* | 上传文件真实 MIME 内容非 png/jpeg/webp/svg（如把 HTML 改名 `.png`） | 上传真实的图片文件内容 |
| `TABLE_NO_COLUMNS` / `TABLE_NO_ROWS` / `TABLE_COL_MISMATCH` | `<compare-table>` 缺 `<columns>/<col>`、缺 `<row>` 或某行 `<cell>` 数与 `<col>` 数不一致 | 对齐列头与每行 `<cell>` 数量 |
| `MISSING_SOURCE` | `<sources>` 内没有 `<source>` | 至少加一条 `<source title="..." url="..."/>` 或移除空的 `<sources>` |

### v1.1 版本门控与富文本/互动/结构规则

| 错误码 | 触发原因 | 修复方式 |
| :--- | :--- | :--- |
| `TAG_REQUIRES_1_1` | 在 `version="1.0"` 文档中使用了 v1.1 标签或新属性（如 `figure@id/alt`、`cell@tag`、`paper-card@doi`、`section@id`） | 将根元素改为 `<document version="1.1">` |
| `SECTION_TOO_DEEP` | `<section>` 嵌套超过 3 层（仅计 `section` 祖先，`<stage>` 不计层数） | 展平章节层级至 ≤ 3 层 |
| `TOO_MANY_BLOCKS` | 文档总量超上限：`<mermaid>` >10、`<chart>` >10、`<stages>` 组 >8 或全文 `<stage>` 合计 >40 | 合并或精简超出上限的块/阶段 |
| `DUPLICATE_ID` | `section`/`stages`/`stage`/`quiz`/`figure`/`equation`/`ref` 的 `@id` 跨组或跨类型重名 | 使用文档内全局唯一的 `@id` |
| `DUPLICATE_TAB` | 同一个 `<tabs>` 内出现同名 `<tab title>` | 为每个 `<tab>` 起唯一标题 |
| `INVALID_TARGET` | `<next-steps>/<option target="#...">` 不匹配 `^#[a-z0-9-]+$`、前缀与元素不符、用了位置锚点 `#sec-N` 或目标不存在；或 `<code-block eq="...">` 无对应 `<equation id>` | `target` 使用有效前缀 `#s-`/`#stages-`/`#stage-`/`#quiz-`/`#fig-`/`#eq-`/`#ref-`；`code-block@eq` 写不带 `#` 和前缀的原始 `<equation id>` |
| `INVALID_CHART` | `type` 非法（`bar`/`line`/`pie`/`scatter`）、JSON 语法错、Schema 校验失败（如写了未定义字段 `title`/`xAxis`）、长度不等、`NaN`/`Infinity`（含未加引号 `NaN`）、总点数 >2000、`pie`/`scatter` 带 `err` 或 `err` 为负/长度不等 | 对照 [xml-tags.md](xml-tags.md#chart-json-schema-chart) 的字段表与错误中的 JSON 路径（如 `/series/0/data/1`、`/series/0/err`）修复 JSON 数据（图表标题请放到 `<chart caption="...">`） |
| `INVALID_EQUATION` | `<equation>` 内容为空或超出 4 KB（4096 字节） | 补充非空 LaTeX 或拆分公式 |
| `INVALID_MERMAID` | `<mermaid>` 内容为空或超出 20 KB（20480 字节） | 精简 Mermaid 源码并包入 CDATA |
| `INVALID_CODE_BLOCK` | `<code-block>` 内容为空、超 32 KB、`diff` 非 `true/false` 或 `highlight` 行号格式错（如 `9-3`） | 修正内容长度或 `highlight="3,5-7"` 格式 |
| `INVALID_METRIC` | `<metric>` 的 `label`/`value` 为空、`trend` 非 `up/down/flat`，或 `<metrics>` 非 1..12 个 `<metric>` | 补齐 `label`/`value`，保持 1–12 项 |
| `INVALID_DOI` | `paper-card@doi` 或 `ref@doi` 不匹配 `^10\.\d{4,9}/[-._;()/:A-Za-z0-9]+$`（例如误写 `https://doi.org/...`） | 仅填裸 DOI（如 `10.48550/arXiv.2611.01234`） |
| `INVALID_STAGES` | `<stages>` 嵌套在 `<stage>` 祖先中（含 `<stage><section><stages>`）、缺 `stages@id`/`stage@id`/`stage@title`、`gated` 非布尔，或单组不在 1..8 个 `<stage>` | 取消嵌套并补全 `id`/`title` 与数量 |
| `INVALID_QUIZ` | 缺 `quiz@id`、`<question>` 不是恰好 1 个或为空、`<option>` 不在 2..8 个、少于 1 个 `correct="true"`、`<explain>` >1 个或内含子 XML 标签 | 修正测验结构，若题目含 `<img ...>` 等文本请用 CDATA 或转义 |
| `INVALID_TLDR` | `<tldr>` 不在 `body`/`section`/`stage` 直接子级、同容器多于 1 个、`placement` 非 `top/bottom`、`<point>` 不在 1..7 个、`<verdict>` >1 个，或内容 >300 Unicode 字符（按 rune 计数） | 保持每容器最多 1 个 `<tldr>`、1..7 个 `<point>`，每条 ≤300 字符 |
| `INVALID_NEWS` | `<news-item>` 缺 `title`、`importance` 非 `high/normal`、摘要 >600 Unicode 字符（按 rune 计数）、`<detail>` >1 个或 `<detail>` >8 KB | 补齐 `title` 并控制摘要 ≤600 runes、`<detail>` ≤8 KB |
| `INVALID_CHECK` | `<checklist>` 无 `<check>` 或 `<check status>` 不在 `done/todo/blocked/risk` | 设为合法状态之一 |
| `INVALID_SEARCH_LOG` | `<search-log>` 缺 `<query>`、多于 1 个 `<criteria>`/`<stop>` 或 `hits`/`kept` 为负数/非整数 | 至少提供 1 个 `<query>` 及非负整数计数 |

---

## 2. CLI 与服务端错误排错（Push / Agent API）

| 状态码 / 场景 | 错误标识 / 现象 | 原因与处理 |
| :--- | :--- | :--- |
| 未配置服务端 / 离线写作 | `dirs`/`list`/`push --dry-run` 报 ``no server configured: run `aumbry config set --server URL` or set AUMBRY_SERVER``（exit 1）或连接失败 | `aumbry dirs`/`list`/`push --dry-run` 均需已配置且可达的服务端（`--dry-run` 先做本地校验，再请求 `GET /api/v1/agent/dirs`）；但撰写 XML 与 `aumbry validate` **完全离线可用**，可先完成本地编写与校验，推送前再执行 `aumbry config set` |
| 本地参数校验失败 | `--draft` 与 `--dir` 冲突；`--create-only` 与 `--id` 冲突或缺少 `--key`；`delete` 未带 `--yes` | 按互斥条件拆开命令；`--draft` 会自动忽略配置中的 `default_dirs`/`AUMBRY_DIRS`；删除必须显式 `--yes` |
| `400` | `invalid_request` / `unknown_dirs` | 检查 `external_key`（trim 后 1..200 字符且无控制字符）、或用 `aumbry dirs` 重新确认有效目录 ID |
| `401` | `unauthorized` | 未配置 Token、Token 无效或格式非 Bearer；运行 `aumbry config show` / `aumbry config set --token ...` |
| `403` | `not_owner` | 仅在 `unpublish` / `delete` 时发生：当前 Token 不是该条目创建者（含 v1.0 遗留条目 `created_by` 为 `NULL`）；改用原创建 Token 或请管理员在 Web 管理后台处理 |
| `404` *(推 `--draft` / `--create-only` 时)* | 预检 `/api/v1/agent/dirs` 返回 404 | 目标服务端仍是 v1.0，不支持这些参数；请先升级服务端再使用新参数 |
| `409` | `key_exists` | 带 `--create-only --key K` 时该键已存在；若需更新请去掉 `--create-only`，或更换唯一 `--key` |
| `409` | `already_published` | 目标条目已有挂载目录，拒绝用 `--draft` 覆盖；先执行 `aumbry unpublish <id>` 转回草稿，或直接不带 `--draft` 推送更新 |
| `413` | `too_large` | 请求体 >100 MB、XML >10 MB 或单张图片 >20 MB；压缩或裁剪资产体积 |
| `422` *(含对 v1.0 服务端推 1.1)* | `ValidationFailedError`；若返回 `UNSUPPORTED_VERSION` 且 stderr 提示升级服务端 | 本地/服务端校验不通过按 JSON `errors` 修复；若是 v1.0 服务端拒收 `version="1.1"`，需升级服务端 |
| `429` | `too_many_requests` | `/ingest` 与 `/agent/*` 共享同 IP `120 次/分钟` 限流，且 `/ingest` 并发上限为 4；等待退避后重试 |
| `502` | `storage_error` | 服务端私有 WebDAV 存储不可达或响应头超时（30s）；等待存储恢复并检查服务端 `/readyz` |
