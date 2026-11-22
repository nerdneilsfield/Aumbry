# Aumbry 校验与推送排错参考（Troubleshooting）

> `aumbry validate <file.xml>` 默认输出 JSON（`line`、`code`、`field`、`message`、可选 `hint`）；按错误码就地修复后再推送。

## 1. 校验错误码全集（`pkg/schema` 与 `internal/api`）

### XML 与根结构

| 错误码 | 触发原因 | 修复方式 |
| :--- | :--- | :--- |
| `INVALID_XML` | XML 语法错、未知实体（如 `&nbsp;`）或含 `<!DOCTYPE>` | 闭合标签并删 `<!DOCTYPE>`；正文含 `&`/`<T>` 时包进 `<![CDATA[ ... ]]>` |
| `INVALID_ROOT` | 根元素不是 `<document>` | 改为 `<document version="1.1">` |
| `UNSUPPORTED_VERSION` | `document@version` 缺失或不是 `"1.1"`（`1.1` 是唯一受支持的版本） | 设为 `"1.1"` |
| `UNEXPECTED_TEXT` | 容器（`document`/`body`/`section`/`stage`/`collapse`/`tab`/`step`/表格/列表）直接放了非空白文本 | 正文放入 `<md><![CDATA[...]]></md>` |
| `MISSING_META` / `DUPLICATE_META` | 缺 `<meta>` 或存在多个 `<meta>` | 确保 `<document>` 内恰有 1 个 `<meta>` |
| `MISSING_BODY` / `DUPLICATE_BODY` | 缺 `<body>` 或存在多个 `<body>` | 确保 `<document>` 内恰有 1 个 `<body>` |
| `REQUIRED_FIELD` | `<meta>` 缺或空白：`title`/`type`/`date`/`summary`/`source-agent` | 补全 5 个必填 `<meta>` 子元素 |
| `INVALID_TYPE` | `<meta>/<type>` 不是 `report` 或 `post` | 改为 `report` 或 `post` |
| `INVALID_DATE` | `<meta>/<date>`、`event@date`、`news-item@published`、`search-log@date`、`query@date` 格式或日期无效 | `<meta>/<date>` 与 `search-log`/`query` 用 `YYYY-MM-DD`；`event@date` 允许 `YYYY`/`YYYY-MM`/`YYYY-MM-DD`；`news-item@published` 允许 `YYYY-MM-DD` 或 RFC 3339 |
| `INVALID_TAG` | `<meta>/<tags>` 含非 `<tag>` 或空 `<tag>` | 仅保留非空 `<tag>...</tag>` |
| `INVALID_ID` | (1) `related-ids/id`、`paper-card@related-id`、`news-item@detail-id` 不符 `^[A-Za-z0-9_-]{12}$`；(2) 锚点 `@id` 不符 `^[a-z0-9-]{1,32}$` | 条目 ID 必须来自 `push` 或 `aumbry list`；锚点 ID 仅用 1–32 位小写字母、数字和 `-` |
| `UNKNOWN_TAG` | 容器不允许该子标签，或叶子元素内含子 XML 标签 | 对照 [xml-tags.md](xml-tags.md) 修正父子容器；代码/HTML 示例放进 `<![CDATA[...]]>` |
| `MISSING_ATTR` | 缺必填属性或为空（如 `section@title`、`figure@src`、`paper-card@title/url`、`repo-card@name/url`、`source@title/url`、`ref@title`、`term@name`、`event@title`、`figure-group@cols`） | 补齐必填属性 |
| `INVALID_ATTR` | 枚举值非法（`callout@type`、`claim@level`、`cell@tag`、`read-depth`）、`<option>` 为空、或数量/重名超限（`tabs` <2、`steps interactive` <2、`glossary` 重名 `term`、`figure-group` 非 `2..6`、`next-steps` 非 `1..5`） | 对照 [xml-tags.md](xml-tags.md) 修正枚举、正文与上下限 |
| `INVALID_URL` | `canonical-url`、`paper-card@url/code-url`、`repo-card@url`、`source@url`、`ref@url`、`quote@url`、`news-item@url` 不是绝对 `http(s)://` URL | 补全 `http(s)://` URL（页内跳转改用 Markdown 锚点或 `<next-steps>`） |
| `INVALID_ASSET_PATH` | `<figure src>` 为外链、`data:`、绝对路径或 `..` 逃出 **XML 文档自身所在目录** | 改为相对于 XML 文档自身目录（非 CWD）的相对路径，如 `./imgs/fig.png` |
| `INVALID_ASSET_TYPE` | `<figure src>` 扩展名不是 `.png`/`.jpg`/`.jpeg`/`.webp`/`.svg` | 转为支持的图片格式 |
| `MISSING_ASSET` | 按 **XML 文档所在目录** 解析后文件不存在或符号链接逃出根目录（`step`/`tab`/`figure-group` 同样检查） | 确认文件相对 XML 文档存在且未用符号链接指向目录外 |
| `INVALID_ASSET` *(服务端)* | 真实 MIME 内容非 png/jpeg/webp/svg（如改名 HTML） | 上传真实图片文件 |
| `TABLE_NO_COLUMNS` / `TABLE_NO_ROWS` / `TABLE_COL_MISMATCH` | `<compare-table>` 缺 `<columns>/<col>`、缺 `<row>` 或某行 `<cell>` 数与 `<col>` 数不等 | 对齐列头与每行 `<cell>` 数 |
| `MISSING_SOURCE` | `<sources>` 内无 `<source>` | 加至少 1 条 `<source>` 或删除空 `<sources>` |

### 富文本、互动与结构规则

| 错误码 | 触发原因 | 修复方式 |
| :--- | :--- | :--- |
| `SECTION_TOO_DEEP` | `<section>` 嵌套 >3 层（仅计 `section` 祖先，`<stage>` 不计） | 展平章节层级至 ≤3 层 |
| `TOO_MANY_BLOCKS` | 总量超限：`<mermaid>` >10、`<chart>` >10、`<stages>` 组 >8 或全文 `<stage>` >40 | 合并或精简块/阶段数量 |
| `DUPLICATE_ID` | 锚点 `@id` 跨组或跨类型重复 | 使用全文唯一 `@id` |
| `DUPLICATE_TAB` | 同 `<tabs>` 内出现重名 `<tab title>` | 为同组 `<tab>` 设唯一标题 |
| `INVALID_TARGET` | `<next-steps>/<option target>` 不符 `^#[a-z0-9-]+$`、前缀不对、用了 `#sec-N` 或目标不存在；或 `code-block@eq` 无对应 `equation@id` | `target` 用 `#s-`/`#stages-`/`#stage-`/`#quiz-`/`#fig-`/`#eq-`/`#ref-`；`code-block@eq` 填不含 `#` 与前缀的原始 ID |
| `INVALID_CHART` | `type` 非法（`bar`/`line`/`pie`/`scatter`）、JSON 语法或 Schema 错（如多余 `title`/`xAxis`）、长度不等、`NaN`/`Infinity`、总点数 >2000、`pie` 非 1 组 `series`、`pie`/`scatter` 带 `err` 或 `err` 为负/长度不等 | 对照 [xml-tags.md](xml-tags.md#chart-json-schema-chart) 与错误信息里的 JSON 路径修改（标题放 `<chart caption="...">`） |
| `INVALID_EQUATION` | `<equation>` 为空或 >4096 B | 补充非空 LaTeX 或拆分公式 |
| `INVALID_MERMAID` | `<mermaid>` 为空或 >20480 B | 精简源码并放入 CDATA |
| `INVALID_CODE_BLOCK` | `<code-block>` 为空、>32768 B、`diff` 非 `true/false` 或 `highlight` 格式错（如 `9-3`） | 修正长度、`diff` 或 `highlight="3, 5-7"` |
| `INVALID_METRIC` | `label`/`value` 为空、`trend` 非 `up/down/flat` 或数量不在 1..12 | 补齐必填属性并保持 1–12 项 |
| `INVALID_DOI` | `doi` 不匹配 `^10\.\d{4,9}/[-._;()/:A-Za-z0-9]+$` | 仅填裸 DOI（如 `10.48550/arXiv.2611.01234`），去掉 URL 前缀 |
| `INVALID_STAGES` | `<stages>` 嵌套在 `<stage>` 内、缺 `stages@id`/`stage@id`/`stage@title`、`gated` 非布尔或单组非 1..8 个 `<stage>` | 取消嵌套并补全 ID、title 与数量 |
| `INVALID_QUIZ` | 缺 `quiz@id`、`<question>` 非 1 个或为空、`<option>` 非 2..8、少于 1 个 `correct="true"` 或 `<explain>` >1/含子标签 | 补齐规范测验结构，题干含特殊文本时包 CDATA |
| `INVALID_TLDR` | `<tldr>` 不在 `body`/`section`/`stage` 直接子级、同容器 >1 个、`placement` 非 `top/bottom`、`<point>` 非 1..7、`<verdict>` >1/空或内容 >300 runes | 保持每容器 ≤1 个 `<tldr>`、1–7 个非空 `<point>`，每条单行且 ≤300 字符 |
| `INVALID_NEWS` | 缺 `title`、`importance` 非 `high/normal`、摘要 >600 runes、`<detail>` >1 或 >8 KB | 补 `title` 并控制摘要 ≤600 runes、`<detail>` ≤8 KB |
| `INVALID_CHECK` | 缺 `<check>` 或 `status` 非 `done/todo/blocked/risk` | 设为合法状态之一 |
| `INVALID_SEARCH_LOG` | 缺 `<query>`、`<criteria>`/`<stop>` >1 或 `hits`/`kept`（若存在）非非负整数 | 提供 ≥1 条 `<query>` 与合法计数 |

## 2. CLI、配置与服务端排错（Push / Agent API）

| 状态码 / 现象 | 错误标识 / 原因 | 处理方式 |
| :--- | :--- | :--- |
| 未配服务端 / profile 错 | 报 ``no server configured``、`unknown profile`（exit `1`）或连接失败 | `aumbry validate` 可离线先跑；联网命令按 `CLI 参数 > AUMBRY_* 环境变量 > ~/.config/aumbry/config.yaml` 解析，推送前配 `aumbry config set --server ... --token ...` 或设置 `AUMBRY_SERVER`/`AUMBRY_TOKEN` |
| 发布后访客报 `403` | 未带 `--dir` 且无 `default_dirs`/`AUMBRY_DIRS`，条目沦为未挂载草稿 | 先用 `aumbry dirs` 选目录并用同 `--key --dir <id>` 重推；禁止在发布场景无 `--dir` 裸推 |
| 本地参数互斥拒绝 | `--draft` 配 `--dir`；`--create-only` 缺 `--key` 或配 `--id`；`delete` 缺 `--yes` | 按互斥条件拆分命令；注意 `--draft` 自动忽略 `default_dirs`/`AUMBRY_DIRS` |
| `400` | `invalid_request` / `unknown_dirs` | 检查 `external_key`（trim 后 1..200 runes 且无控制字符），或用 `aumbry dirs` 核对目录 ID |
| `401` | `unauthorized` | 缺少或无效 Bearer token（与目录密码无关，须取自服务端 `api_tokens`）；用 `aumbry config show` / `config set --token ...` 核对 |
| `403` | `not_owner`（仅 `unpublish`/`delete`） | 当前 token 非条目创建者（含 `created_by` 为 `NULL`、没有归属记录的条目）；换原创建 token 或交由 `/admin` 管理控制台处理 |
| `409` | `key_exists` | 带 `--create-only --key` 撞库；更新请去掉 `--create-only`，新建请换 `--key` |
| `409` | `already_published` | 目标条目已挂目录，拒绝 `--draft` 覆盖；先 `aumbry unpublish <id>` 再存草稿，或去掉 `--draft` 更新 |
| `413` | `too_large` | 总请求 >100 MiB、XML >10 MiB 或单图 >20 MiB；压缩图片或拆分正文 |
| `422` | 校验拒绝 | 按返回的 JSON `errors` 修改 |
| `429` | `too_many_requests` | `/ingest` 与 `/agent/*` 同 IP 每分钟上限 `120` 次且 `/ingest` 并发上限 `4`；退避重试 |
| `502` | `storage_error` | 服务端私有 WebDAV 不可达或 30 s 响应头超时；检查 WebDAV 与服务端 `/readyz` |
