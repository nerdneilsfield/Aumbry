[English](../en/protocol.md) | 简体中文

# XML 协议入门

Aumbry 文档采用基于 UTF-8 的 XML 格式，由前后端共享的 `pkg/schema` 校验包解析。`aumbry validate` 离线检查、服务端 `POST /api/v1/ingest` 写入与 FTS5 索引提取，以及 Vue 3 阅读器全部依据同一套协议规则工作。

## 版本差异（`1.0` 与 `1.1`）

文档根节点必须声明 `<document version="1.0">` 或 `<document version="1.1">`：

| 维度 | `version="1.0"` | `version="1.1"` |
| :--- | :--- | :--- |
| `<body>` 直接子元素 | 仅允许 `<section>` 与 `<sources>` | 可直接放置块级标签（`<md>`、`<claim>`、`<metrics>`、`<chart>`、`<stages>`、`<tldr>`、`<quiz>`、`<news-item>` 等） |
| `<section>` 嵌套深度 | 仅限 1 层（不可嵌套 `<section>`） | 最多 `3` 层 `<section>`（`<stage>` 祖先不计入层数） |
| `<collapse>` 允许位置与子元素 | 仅限 `<section>` 内，且只含 `<md>` 与 `<figure>` | 可放在 `<body>` / `<section>` / `<stage>` 下，容纳各类叶子块与 `<search-log>` |
| `<figure>` 资产收集范围 | 仅限 `/document/body/section/figure` 与 `collapse/figure`（`figureXPathV10`） | 文档内所有 `//figure[@src]`（`figureXPathV11`） |
| 全文块数量上限 | 仅受上传体积限制 | `chart <= 10`、`mermaid <= 10`、`stages <= 8`、`stage <= 40`、`section <= 3 层` |
| 统一锚点与跳转 | 无 | `section`、`stages`、`stage`、`quiz`、`figure`、`equation`、`ref` 共用全局 `^[a-z0-9-]{1,32}$` ID；`<next-steps>` 跳转使用 `#s-` / `#stage-` / `#fig-` / `#eq-` / `#ref-` |
| Markdown 渲染能力 | CommonMark + KaTeX `$` 公式 + Shiki 代码高亮（`html: false`） | 同上，并启用按 `env.docId` 隔离的脚注插件 `markdown-it-footnote`、只读任务列表、下标 `~` 与上标 `^` |
| 版本门控 | 出现任意 1.1 标签或新属性时报错 `TAG_REQUIRES_1_1` | 支持全部 1.0 与 1.1 元素和属性 |

## 查看实时规范与本地校验

通过 CLI 直接打印当前二进制内置的 Markdown 或 JSON 协议规范：

```sh
aumbry schema --format md
aumbry schema --format json
```

`aumbry schema --format md` 开头输出示例：

```markdown
# Aumbry XML Protocol Specification

## Version Comparison (1.0 vs 1.1)

### Version `1.0`

- Section depth: 1 (top-level <section> inside <body> only; no nested <section>)
- `<body>` direct children: section, sources
```

推送前使用 `validate` 在本地离线检查（`--format json` 默认输出行号、`code`、`field`、`hint`）：

```sh
aumbry validate examples/minimal-report.xml --format text
```

校验通过输出：

```text
valid: examples/minimal-report.xml
```

## 完整标签表与示例

协议完整标签表格由 `pkg/schema/spec.go` 自动生成并受 CI（`make skill-check`）校验同步，本页不再重复抄录：

- 全量元素、属性、容器约束与 Chart JSON 字段表：[skills/aumbry/references/xml-tags.md](../../skills/aumbry/references/xml-tags.md)
- 按错误码排查指南：[skills/aumbry/references/troubleshooting.md](../../skills/aumbry/references/troubleshooting.md)
- 结构示例文件：`skills/aumbry/assets/examples/` 下的 `one-pager.xml`、`paper-reading.xml`、`compare-repro.xml`、`news-digest.xml` 与 `news-detail.xml`，以及 [examples/minimal-report.xml](../../examples/minimal-report.xml)、[examples/full-report.xml](../../examples/full-report.xml)
