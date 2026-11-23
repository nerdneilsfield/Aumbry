[English](../en/protocol.md) | 简体中文

# XML 协议入门

Aumbry 文档采用基于 UTF-8 的 XML 格式，由统一的协议校验规则解析。`aumbry validate` 离线检查、服务端 `POST /api/v1/ingest` 写入与全文索引提取，以及 Web 阅读器全部依据同一套协议规则工作。

## 协议版本

文档根节点必须声明 `<document version="1.1">`。Aumbry 不接受其他版本：`version` 取其他值或缺失时，校验报错 `UNSUPPORTED_VERSION`。

| 维度 | 规则 |
| :--- | :--- |
| `<body>` 直接子元素 | 可直接放置块级标签（`<md>`、`<claim>`、`<metrics>`、`<chart>`、`<stages>`、`<tldr>`、`<quiz>`、`<news-item>` 等） |
| `<section>` 嵌套深度 | 最多 `3` 层 `<section>`（`<stage>` 祖先不计入层数） |
| `<collapse>` 允许位置与子元素 | 可放在 `<body>` / `<section>` / `<stage>` 下，容纳各类叶子块与 `<search-log>` |
| `<figure>` 资产收集范围 | 文档内任意层级的带 `src` 属性 `<figure>` |
| 全文块数量上限 | `chart <= 10`、`mermaid <= 10`、`stages <= 8`、`stage <= 40`、`section <= 3 层` |
| 统一锚点与跳转 | `section`、`stages`、`stage`、`quiz`、`figure`、`equation`、`ref` 共用全局 `^[a-z0-9-]{1,32}$` ID；`<next-steps>` 跳转使用 `#s-` / `#stages-` / `#stage-` / `#quiz-` / `#fig-` / `#eq-` / `#ref-` |
| Markdown 渲染能力 | CommonMark（禁用原始 HTML）、KaTeX 公式（行内 `$...$` / `\(...\)`，块级 `$$...$$` / `\[...\]`）、代码语法高亮，并支持文档间隔离的脚注、只读任务列表、下标（`~`）与上标（`^`） |

## 查看实时规范与本地校验

通过 CLI 直接打印当前二进制内置的 Markdown 或 JSON 协议规范：

```sh
aumbry schema --format md
aumbry schema --format json
```

`aumbry schema --format md` 开头输出示例：

```markdown
# Aumbry XML Protocol Specification

Protocol version: `1.1` (the only accepted value of `<document version>`).

## Structure Rules

- Section depth: 3 (at most 3 nested <section> levels counting only <section> ancestors (<stage> does not count))
- `<body>` direct children: section, sources, md, figure, paper-card, repo-card, compare-table, ...
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

协议完整标签表格由 CLI 内置规范自动生成并受 CI（`make skill-check`）校验同步，本页不再重复抄录：

- 全量元素、属性、容器约束与 Chart JSON 字段表：[skills/aumbry/references/xml-tags.md](../../skills/aumbry/references/xml-tags.md)
- 按错误码排查指南：[skills/aumbry/references/troubleshooting.md](../../skills/aumbry/references/troubleshooting.md)
- 结构示例文件：`skills/aumbry/assets/examples/` 下的 `one-pager.xml`、`paper-reading.xml`、`compare-repro.xml`、`news-digest.xml` 与 `news-detail.xml`，以及 [examples/minimal-report.xml](../../examples/minimal-report.xml)、[examples/full-report.xml](../../examples/full-report.xml)
