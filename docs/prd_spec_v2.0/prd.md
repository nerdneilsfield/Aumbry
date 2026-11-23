# Aumbry 产品需求与技术架构详细设计文档（PRD & Tech Spec v2.0）

| 文档属性 | 内容 |
| :--- | :--- |
| **项目名称** | **Aumbry**（多 AI Agent 调研报告中枢） |
| **文档版本** | v2.0（已冻结规格；在 v1.1 与 v1.2 基线上的增量规格，未提及部分沿用 v1.1 / v1.2） |
| **基线** | [docs/prd_spec_v1.1/prd.md](../prd_spec_v1.1/prd.md)、[docs/prd_spec_v1.2/prd.md](../prd_spec_v1.2/prd.md)，对应已发布 `v0.1.3`（`5e7cbee`）及 `master` 上的 `55814f2` |
| **日期** | 2026-11-23 |
| **本版主题** | **让报告能被订阅和收听、补齐枢纽阅读断点，并落地 MCP 与外链图片自动转存**：分 M1（`v0.1.4`）与 M2（`v0.2.0`）两个可独立发版的里程碑交付 |

---

## 1. 背景、目标与非目标

### 1.1 背景

v1.1 与 v1.2 已建立结构化 XML 协议（`version="1.1"`）、本地与 S3/R2/WebDAV 私有存储、全文与中文双字索引，以及面向 Agent 的 `skills/aumbry/` 技能分发。日常使用与需求评审中暴露出两类待解决的问题：

1. **订阅与多媒体的缺失**：当前资产接口与 `pkg/schema` 仅接受 `.png` / `.jpg` / `.jpeg` / `.webp` / `.svg` 图像。本地播客技能已能生成 `mp3` / `wav` 与 `HH:MM:SS Title` 章节，却无法随报告一同托管、鉴权、拖拽播放（HTTP `Range`）或作为播客 RSS 订阅源对外输出；目录页也缺少文字订阅源（RSS）。
2. **枢纽阅读与写作断点**：
   - 简报可用 `<news-item detail-id>` 指向详情（`<paper-card related-id>` 同理），但详情页不会自动列出指向自己的简报，只能手工维护 `meta/related-ids`。
   - 复用 `--key` 覆盖更新后，读者无法回看旧版本，也难以看清相比上一版改动了什么。
   - 精读稿仅能贴裁图，无法随文挂载受鉴权的本地 PDF 原文并在站内安全翻页与按页跳转。
   - v1.0 / v1.1 路线图留给 v2.0 的 **MCP Server** 与 **外链图片自动转存** 尚未形成冻结可验收规格。
3. **公式分隔符基线状态（G1 已完成）**：`<md>` 渲染引擎此前只开启 `texmath` 的 `dollars` 分隔符。该问题已在基线提交 `55814f2`（`fix(web): support LaTeX bracket math delimiters in markdown`）完成修复并合入 `master`（通过 [markdownEngine.ts](../../web/src/lib/markdownEngine.ts) 与 [markdown.test.mjs](../../web/test/markdown.test.mjs)），已同时支持 `$...$`、`$$...$$`、`\(...\)`、`\[...\]`（含无空行紧跟段落/标题的 `\[...\]`），并保持 `\[-1.4; +4.3\]` 等转义方括号按原文显示；该修复将随 `v0.1.4` 一并发版，本版不再重复排期。

### 1.2 目标与里程碑映射

| 目标 | 名称 | 核心验收口径 | 状态 / 里程碑 |
| :--- | :--- | :--- | :--- |
| G1 | 公式分隔符 | `$...$`、`$$...$$`、`\(...\)`、`\[...\]`（含无空行紧邻段落的 `\[...\]`）正常渲染；行中与行首 `\[-1.4; +4.3\]` 保持普通文本；不改 XML 协议。 | **已完成**（`55814f2`，随 `v0.1.4` 发版） |
| G4 | 简报与详情反向索引 | 详情页通过服务端反向索引自动展示指向自己的已发布、且当前调用者有权查看的简报/报告，无需手工回填 `related-ids`；下线/删去引用后入口消失。 | **M1（`v0.1.4`）** |
| G5 | 条目修订历史 | 每次非草稿正式发布留存完整历史版本及发布目录边界（默认保留 20 版，可配置），授权读者只读查看并获块级变更摘要与文本差异高亮；支持管理员删除历史修订。 | **M1（`v0.1.4`）** |
| G2 | 播客与音频（含流式 ingest 与 Range） | `<audio>` 支持 `mp3` / `wav` / `m4a`、章节与锚点跳转、逐字稿入全文索引；ingest 流式写入并提升默认上限；存储 `OpenRange` 与标准库 `http.ServeContent` 支撑 `Range`。 | **M1（`v0.1.4`）** |
| G3 | 目录订阅源 | 每目录提供 RSS 2.0 文字源（`rss.xml`）与播客源（`podcast.xml`）；公开目录可匿名订阅；保护目录通过「目录级可撤销只读 Token」订阅与拉取 enclosure。 | **M1（`v0.1.4`）** |
| G6 | 可选 PDF 原文附件 | `<attachment type="pdf">` 支持本地 PDF 与 `#att-<id>:p=<N>` 页码锚点；前端懒加载 `pdfjs-dist` 纯 Canvas 渲染，禁用脚本；服务端校验真实页数。 | **M2（`v0.2.0`）** |
| G8 | 外链图片自动转存 | `version="1.2"` 的 `<figure src>` 允许公网 `http(s)://`；CLI `validate` 仍离线；服务端 `ingest` 禁用代理并做连接级 SSRF 防护，原子转存为私有资产。 | **M2（`v0.2.0`）** |
| G7 | MCP Server | 同进程提供 Streamable HTTP `/mcp`；支持读（`list_dirs`、`list_items`、`search_items`、`get_item`）与校验/发布（`validate_xml`、`push_item`）；支持匿名、`ad_` 与 API Token 三类身份并逐请求验权。 | **M2（`v0.2.0`）** |

### 1.3 非目标（Out of Scope）

沿用 `docs/prd_spec_v1.1/prd.md` §1.3 与 `v2.0` 边界：

1. 在 XML 中放置原始 HTML、`<iframe>`、视频或任意 JavaScript，或透传 ECharts 的原始 `option`。
2. 评论、协作编辑、UI 多语言切换。
3. 多用户账号体系、分级 API Token、面向单个 API Token 的独立目录 ACL 表。
4. 服务端语音合成（TTS）、音频/视频转码或生成波形图（音频在站外生成后作为本地文件上传）。
5. 通用三维点云或 Gaussian Splatting 查看器。
6. 站内与 Zotero 的双向同步。
7. v1.1 §14.2 第 8 条已标明暂缓的项：`push --batch`、`push --replace-dirs`、Prometheus `/metrics`、结构化审计日志。
8. 不保证多段 Range（`multipart/byteranges`）的定制优化（Range 统一交由 Go 标准库 `http.ServeContent` 处理）。
9. 不自动下载外链 PDF（如 arXiv PDF）；外部文献链接继续放在 `meta/canonical-url` 或 `<sources>/<source>`。
10. 不允许任何音频、PDF 或转存图片绕过鉴权放入公开 CDN。

---

## 2. XML 协议 v1.2 与向后兼容

### 2.1 版本声明与向后兼容规则

* 既有 **`<document version="1.1">`** 文档继续被 `aumbry validate`、`POST /api/v1/ingest`、`aumbry-server reindex` 与 Web 阅读器完整接受，规则与 v1.1 保持一致。
* 在 `pkg/schema/protocol.sch` 中将根断言从仅接受 `'1.1'` 扩展为接受 `'1.1'` 与 `'1.2'`；各容器（`body`、`section`、`stage`、`collapse`、`tab`、`step`）白名单按 `@version` 分支：
  * 在 `version="1.1"` 文档中出现 `<audio>`、`<attachment>` 时，**明确报出带升级提示的错误**（`UNSUPPORTED_VERSION` 或专属 `INVALID_AUDIO` / `INVALID_ATTACHMENT`，Hint 明确说明需升级到 `<document version="1.2">`），不得只报普通 `UNKNOWN_TAG`；同理 `<figure src>` 在 `version="1.1"` 下出现 `http(s)://` 时报出“外链转存需要 `version="1.2"`”。
  * 同步更新 [pkg/schema/spec.go](../../pkg/schema/spec.go) 中硬编码为 `"only 1.1"` 的版本说明文本。
* 新增 **`<document version="1.2">`**：在 `version="1.1"` 全套元素与约束上**严格增量扩展**，不移除、不破坏任何 v1.1 元素语义：
  * **M1 冻结交付**：`<audio>` 及其子元素 `<chapter>`、`<transcript>`。
  * **M2 冻结交付**：`<attachment type="pdf">`、页引用 `#att-<id>:p=<N>`，以及 `<figure src>` 支持外链 `http(s)://`。
* 旧版（仅支持 `1.1` 的）客户端或未升级的验证器遇到 `version="1.2"` 时按既有规则返回 `UNSUPPORTED_VERSION`，不得静默丢弃未知标签后当成旧文档发布。

### 2.2 统一本地资产路径、重写与锚点扩展

* **本地资产发现与重写**：扩展 `pkg/schema` 的本地资产收集、路径规范化与 `RewriteAssets` 逻辑，统一覆盖：
  * M1：`//figure[@src]` 与 `//audio[@src]`
  * M2：增补 `//attachment[@src]`
  * 本地路径继续复用 `schema.NormalizeAssetPath` 与 `schema.DirAssetChecker`（基于 `os.OpenRoot`，禁止外链以外的越级、绝对路径、`..` 与目录外符号链接）。
  * `StoredAssets: true` 模式（供 `reindex` 与读取历史 XML 时使用）必须同步识别已重写为 `/api/v1/assets/<64-hex>` 的 `<figure>`、`<audio>` 与 `<attachment>` 节点，不误报 `INVALID_ASSET_PATH`。
* **统一锚点命名空间与后缀**：
  * 全局 `^[a-z0-9-]{1,32}$` 共享 ID 命名空间（`DUPLICATE_ID` 检查）新增两种类型：

| 元素 | `id` 命名空间前缀 | 目标引用格式（`<next-steps>` 与 `<audio><chapter target>` 由 Go 校验；`<md>` 内链由前端按同一规则解析跳转） | 交付阶段 |
| :--- | :--- | :--- | :--- |
| `<audio id="...">` | `#audio-<id>` | `#audio-<id>` | M1 |
| `<attachment id="...">` | `#att-<id>` | `#att-<id>` 或 `#att-<id>:p=<N>`（`N` 为 `>= 1` 的十进制正整数） | M2 |

* **目标校验与页码剥离（Go 与前端双向对齐）**：
  * Go 侧（`pkg/schema/rules.go` 的 `targetRe` 与 `pkg/schema/blocks.go` 的 `checkPendingRefs`）与前端侧（`web/src/lib/anchor.ts` 的 `AnchorMap.find` 与 `AnchorMap.findStrict`）**在过 `^#[a-z0-9-]+$` 正则与查表前，都必须先解析并剥离可选的 `:p=<N>` 后缀**：
    * 若目标含 `:p=` 但非 `<att-id>:p=<1..9><0..9>*`（例如 `p=0`、负数或非数字）或应用在非 `#att-` 目标上，直接报 `INVALID_TARGET`。
    * 剥离 `:p=<N>` 后的基础锚点（`#att-<id>`）必须命中同文档 `<attachment id="<id>">`；Go 侧 `pendingTarget` 保留解析出的页号 `N`，供服务端 `ingest` 在解析完 PDF 实际 `page_count` 后复核 `N <= page_count`（超界报 `INVALID_TARGET`）。
  * `<audio><chapter target="...">` 与 `<next-steps><option target="...">` 共用同一套严格目标校验规则；`<md>` 内的 `#...` Markdown 链接沿用现有约定仅在前端解析与跳转，不做 Go 侧校验。

### 2.3 新增校验错误码与 Schema / Skill 同步

| 错误码 | 触发场景 | 阶段 |
| :--- | :--- | :--- |
| `INVALID_AUDIO` | `<audio>` 缺失 `src`/`title`、超出每篇上限、`<chapter start>` 非 `HH:MM:SS` 或不满足非严格单调递增（`start[i] < start[i-1]`）、`<transcript>` 为空白或超出大小上限 | M1 |
| `INVALID_ATTACHMENT` | `<attachment>` `type` 非 `pdf`、缺失 `src`/`title`、超出每篇上限、或上传文件不是有效可读的 PDF | M2 |
| `REMOTE_ASSET_BLOCKED` | `<figure src>` 外链触发 SSRF 安全规则（非 `http`/`https`、含 userinfo、字面内网/回环 IP 或 DNS 解析命中非公网 IP），或 `allow_remote_images = false` | M2 |
| `REMOTE_ASSET_FETCH_FAILED` | 允许外链抓取时，远程请求超时、HTTP 非 2xx、超过大小限制、重定向超限或内容校验与图片格式不符 | M2 |

实现 M1 与 M2 时须同步更新 `pkg/schema/spec.go`、`pkg/schema/protocol.sch`，并重新生成 `skills/aumbry/references/xml-tags.md`、`skills/aumbry/references/troubleshooting.md` 及对应示例，保证 `make skill-check` 通过。

---

## 3. 简报与详情的反向索引（G4 · M1）

### 3.1 数据模型与构建

在 SQLite 新增迁移表 `item_links`：

```sql
CREATE TABLE IF NOT EXISTS item_links (
    source_item_id TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    target_item_id TEXT NOT NULL,
    kind           TEXT NOT NULL CHECK (kind IN ('news_detail', 'paper_related')),
    PRIMARY KEY (source_item_id, target_item_id, kind)
);
CREATE INDEX IF NOT EXISTS idx_item_links_target ON item_links(target_item_id, source_item_id);
```

* 不为 `target_item_id` 设外键约束：允许先发引用再发目标（或目标先被删除），并在删去源条目时由 `ON DELETE CASCADE` 自动清理反向记录。
* **抽取范围**：在 `pkg/schema.Document` 增设只读投影提取方法 `OutgoingLinks()`，从当前文档中收集两类出边并去重：
  * `<news-item detail-id="<id>">` → `kind = 'news_detail'`
  * `<paper-card related-id="<id>">` → `kind = 'paper_related'`
* **写入时机**：在 `internal/api/ingest.go` 的 `saveItem` 事务中原子执行：先 `DELETE FROM item_links WHERE source_item_id = ?`，再插入本次最新出边集合。当源条目更新并删掉某个 `detail-id`、或执行 `unpublish`/`delete` 时，链接自动更新或从公开列表消失。
* **历史存量回填**：扩展 `aumbry-server reindex`，重建全文索引时一并重建每个条目的 `item_links` 行（受离线锁约束，仅写入模式写入）。

### 3.2 API 与权限过滤（同事务一致读取）

扩展 `GET /api/v1/items/{id}` 响应结构 `itemDetail`（[internal/api/items.go](../../internal/api/items.go)），保留原有 `related`（来自 `meta/related-ids`），新增字段 `referenced_by`：

```json
{
  "referenced_by": [
    {
      "id": "abc123def456",
      "type": "report",
      "title": "2026-11-23 AI 日报",
      "date": "2026-11-23",
      "summary": "...",
      "tags": ["daily"],
      "source_agent": "news-digest",
      "link_kinds": ["news_detail"]
    }
  ]
}
```

**强约束（无权限零泄露 + 单事务一致性）**：

1. **同一短只读事务内读取**：`GET /api/v1/items/{id}`（以及第 4 章修订列表与正文路径读取）**必须在同一短只读 SQLite 事务 `tx` 内**完成目录树加载 `loadTreeFrom(ctx, tx)`、当前条目挂载检查、`referenced_by` 候选集与挂载集合读取及 `canViewItem` 权限判断，避免“先查到私密简报旧摘要、在并发重新挂载后套用新权限放行旧摘要”的竞态。事务提交/结束后才可打开对象存储流（遵照 `SetMaxOpenConns(1)` 约定，事务内所有查询全部走 `tx`）。
2. 查询条件必须过滤掉 **未挂载**（草稿）的源条目（`EXISTS (SELECT 1 FROM directory_items di WHERE di.item_id = i.id)`），未发布的简报不得出现在反向引用列表。
3. 在 Go 层按当前请求的 `viewer` 执行权限过滤：若简报 A 所在目录对当前调用者均不可见（如受密码保护且未解锁），`referenced_by` **不得返回 A 的 ID、标题或任何摘要字段**。
4. 排序规则：`i.report_date DESC, i.created_at DESC, i.id ASC`；同一简报同时用多个 `<news-item>` 引用同一详情时去重合并为一条。

### 3.3 阅读页呈现

在 [ReaderView.vue](../../web/src/views/ReaderView.vue) 增加「收录于 / 被以下简报引用」区块（位于正文底部、`related` 上方或紧邻处，仅当 `item.referenced_by.length > 0` 时展示），复用既有条目卡片链接样式（跳转 `/r/<id>` 或 `/p/<id>`）。

---

## 4. 条目修订历史（G5 · M1）

### 4.1 保留策略、B1 历史范围隔离与数据模型

遵循已定决策：**仅正式发布状态记录历史（草稿不入）；默认每条目保留最近 20 版（可配置），超限按最旧 `rev` 顺序淘汰；采用 B1 方案记录每版发布目录边界并在读取历史与历史独占资产时同时满足“现行可读 + 该版目录边界可读”；支持管理员单版删除与差异高亮。**

#### 历史触发与存量基线规则

1. **草稿不产生历史版本**：带 `--draft`（即 `draft=true` 且当前没有任何挂载）推送或覆盖未挂载草稿时，**不写入** `item_revisions`。非草稿 `ingest` 若 `dirs` 为空且条目此前没有任何挂载（即仍处于未挂载状态），同样不建历史快照，直到首次被挂载发布为止。
2. **所有未挂载到已挂载的转换与正式覆盖均按哈希去重记录快照**：
   * 非草稿 `ingest` 提交后条目拥有挂载目录（`len(mounted) > 0`），**或** 管理员通过 `PUT /api/v1/admin/items/{id}/mounts` 将条目挂载变为 `len(dirs) > 0`（不论此前是否已有历史版本，从而彻底覆盖 `已发布 -> unpublish -> --draft 覆盖 -> 后台重新挂载` 路径，防止 `current_rev` 与现行内容错位）：
   * 先检查是否存在历史记录；若当前要写入的新 `xml_sha256` 与最新一条 `item_revisions.xml_sha256` 不同（或此前 0 条历史），在**同一数据库事务内**分配 `rev = COALESCE(MAX(rev), 0) + 1`，同步写入：
     * `item_revisions` 行（快照元数据与不可变 `xml_storage_path`）；
     * `item_revision_assets` 行（该版引用的所有资产哈希）；
     * `item_revision_mounts` 行（该次提交时的挂载目录集合 `mounted`）。
   * 若同一次后台 `PUT /mounts` 仅改变已有挂载集合且 `xml_sha256` 未变（例如把第 N 版从私密移入/追加其它目录），按下面原则更新规则：追加公开目录会同时把 `dirs` 并入还是新建历史？注意 **B1 防泄露核心规则**：第 1 版在私密目录发布，脱敏修改并公开重发产生第 2 版；如果**只是**重新挂载同一份未改动过的私密草稿去公开目录，它本身就是当前版本，正常对外开放。而只要 `xml_sha256` 改变了，`item_revision_mounts` 就锁定旧版第 1 版只曾挂载在私密目录！
3. **存量已发布条目（升级迁移兼容）**：
   * 升级后首次停服执行 `aumbry-server reindex` 时，为所有当前已挂载但 `item_revisions` 数为 0 的存量条目**自动回填 `rev = 1`**（快照其当前 XML、`item_assets` 与 `directory_items`）。
   * 若升级后尚未跑 `reindex` 就对一条存量已发布（0 条历史）条目执行覆盖推送：`saveItem` 在更新 `items` 前，先在同一事务内将该条目**更新前的旧行**（旧 `xml_storage_path`、旧 `item_assets` 与当前挂载 `directory_items`）写入为 `rev = 1`，随后再把新推送版本写入为 `rev = 2`，确保旧版永不丢失。
4. **幂等空推送去重**：同一 `--key` 携带完全一致内容（重写后 `xml_sha256` 与最新一条 `item_revisions.xml_sha256` 相等，且条目已为当前最新快照）重复 `push` 时，不生成重复的空修订。
5. **淘汰上限与原子裁剪**：
   * 新增配置 `revisions.max_per_item`（`AUMBRY_REVISIONS_MAX_PER_ITEM`，默认值 `20`，`0` 表示不限制，负值校验报错）。
   * 每次插入新 revision 后，若该条目历史总数超过 `max_per_item`，在同一事务内按 `rev ASC` 删除超出的最旧记录（通过 `ON DELETE CASCADE` 自动清理对应的 `item_revision_assets` 与 `item_revision_mounts` 行）。
6. **存储对象安全与统一离线 GC**：
   * 因历史淘汰、单版删除或整篇删除条目留下的旧 XML 与资产路径，**一律不在 HTTP 请求事务中直接删除对象存储**。
   * 同步更新 [admin.go](../../internal/api/admin.go) 第 392 行与 [agent.go](../../internal/api/agent.go) 第 277 行的删条目处理器，取消提交后立即 `s.store.Delete(ctx, path)`，统一交给离线 `aumbry-server gc` 清理（消除与并发读旧版或同哈希复用的竞态）。
7. **ID 不可变既有约束**：同题更新仍遵守 v1.1 hard rule——不得改、删已发布 `<stages>` / `<stage>` / `<quiz>` 的 `id`，历史视图与阅读进度不冲突。

#### 数据库迁移（`item_revisions`、`item_revision_assets` 与 `item_revision_mounts`）

```sql
CREATE TABLE IF NOT EXISTS item_revisions (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id          TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    rev              INTEGER NOT NULL,
    title            TEXT NOT NULL,
    summary          TEXT NOT NULL,
    report_date      TEXT NOT NULL,
    xml_storage_path TEXT NOT NULL,
    xml_sha256       TEXT NOT NULL,
    created_by       TEXT,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (item_id, rev)
);
CREATE INDEX IF NOT EXISTS idx_item_revisions_item_rev ON item_revisions(item_id, rev DESC);

CREATE TABLE IF NOT EXISTS item_revision_assets (
    revision_id INTEGER NOT NULL REFERENCES item_revisions(id) ON DELETE CASCADE,
    asset_hash  TEXT NOT NULL REFERENCES assets(hash) ON DELETE RESTRICT,
    PRIMARY KEY (revision_id, asset_hash)
);
CREATE INDEX IF NOT EXISTS idx_item_revision_assets_hash ON item_revision_assets(asset_hash);

-- No FK on directory_id: deleting a historical private directory leaves an unknown directory_id,
-- which dirTree.accessible fails closed on (never treats an unresolvable dir as open).
CREATE TABLE IF NOT EXISTS item_revision_mounts (
    revision_id  INTEGER NOT NULL REFERENCES item_revisions(id) ON DELETE CASCADE,
    directory_id TEXT NOT NULL,
    PRIMARY KEY (revision_id, directory_id)
);
```

#### 与已有鉴权/运维子系统的耦合更新

* **B1 双门历史鉴权（修订元数据、修订正文与 MCP `rev` 共用）**：
  * 除管理员（`v.admin`）外，调用者阅读条目 `id` 的历史版本 `R` 时必须**同时满足两道门槛**（在同一个只读事务 `tx` 内查询判定）：
    1. 当前条目本身对调用者可见（`len(t.visibleDirs(v, currentMounts)) > 0`，防止已下线 `unpublish` 条目仍从历史接口泄漏）；
    2. 该修订 `R` 在 `item_revision_mounts` 中至少存在 1 个 `directory_id` 当前对调用者可见（`len(t.visibleDirs(v, revMounts)) > 0`）。
  * 即使旧私密目录日后被删除，`directory_id` 在目录树中不存在，`dirTree.accessible` 会 fail closed 返回 `false`，保证旧私密版本永远不会意外转为公开。
* **`GET /api/v1/assets/{hash}` 鉴权**：
  * 非管理员读取资产时，可访问条件为以下两者满足其一（同只读事务内检查）：
    1. 存在当前 `item_assets` 关联的条目 `item_id`，调用者当前可查看该条目；**或**
    2. 存在 `item_revision_assets` 关联的修订 `R`（所属条目 `item_id`），调用者**同时满足上述 B1 双门条件**（可查看现行条目 `item_id` 且可查看修订 `R` 的 `item_revision_mounts`）。
* **`aumbry-server verify-storage`**：检查 `items.xml_storage_path UNION item_revisions.xml_storage_path` 全部存在。
* **`aumbry-server gc`**：
  * `liveXML` = `SELECT xml_storage_path FROM items UNION SELECT xml_storage_path FROM item_revisions`
  * `liveAssets` = `SELECT DISTINCT a.storage_path FROM assets a WHERE EXISTS (SELECT 1 FROM item_assets ia WHERE ia.asset_hash = a.hash) OR EXISTS (SELECT 1 FROM item_revision_assets ira WHERE ira.asset_hash = a.hash)`
  * 删除孤儿库行前同时检查 `item_assets` 与 `item_revision_assets` 均无引用（匹配表外键 `ON DELETE RESTRICT`）。先查出完整 `liveXML` 与 `liveAssets` 后才执行对象删除，严禁仅改删库行条件却保留旧对象存活查询。

### 4.2 API 契约

1. **`GET /api/v1/items/{id}/revisions`**：
   * 鉴权：先检查 `authorizeItem`（现行条目无权 403，不存在 404）。
   * 过滤：非管理员按 **B1 双门规则**过滤，只返回 caller 有权访问 `item_revision_mounts` 的修订版本（防止公开条目的旧私密修订标题、摘要在列表中泄漏）。
   * 响应（按 `rev DESC` 排列）：
     ```json
     {
       "item_id": "abc123def456",
       "current_rev": 3,
       "revisions": [
         {
           "rev": 3,
           "title": "...",
           "summary": "...",
           "date": "2026-11-23",
           "created_at": "2026-11-23T06:00:00Z"
         }
       ]
     }
     ```
2. **`GET /api/v1/items/{id}/revisions/{rev}/content`**：
   * 鉴权：在只读事务内校验现行条目可见且 **B1 双门规则**对第 `rev` 版成立；`{rev}` 非正整数返回 `400`；不存在返回 `404`；现行可读但该修订属于无权限旧目录时返回 `403`。
   * 响应头与 `/items/{id}/content` 完全一致：`Content-Type: application/xml; charset=utf-8`、`Cache-Control: private, no-cache`、`X-Content-Type-Options: nosniff`、`Content-Security-Policy: default-src 'none'; sandbox`。
3. **`DELETE /api/v1/admin/items/{id}/revisions/{rev}`**（新增，需 `requireAdmin`）：
   * 用于撤回误发过敏感内容的某个历史快照。
   * 校验：`{rev}` 非正整数返回 `400`；修订不存在返回 `404`；若该修订为当前最新现行版本（`rev == MAX(rev)` 且条目仍对应它），拒绝返回 `409 conflict`（提示先发布新覆盖版本或直接删除整篇条目）。
   * 删除事务：`DELETE FROM item_revisions WHERE item_id = ? AND rev = ?`（级联清理 `item_revision_assets` 与 `item_revision_mounts`），返回 `204 No Content`，对象存储旧 XML 与独占资产留给离线 `gc` 回收。后台管理界面对应提供「查看/删除旧修订」操作。
4. **`GET /api/v1/items/{id}` 增补字段**：追加按调用者可见范围过滤后的 `"revision_count": <int>` 与最新可回看版本号 `"current_rev": <int>`。

### 4.3 阅读页历史模式与差异高亮

* **只读历史阅读**：阅读路由 `/r/:id` 与 `/p/:id` 支持查询参数 `?rev=<N>`（可与 `?dir=` 并存）。
  * 确认 [useUrlParams.ts](../../web/src/composables/useUrlParams.ts) 中 `syncUrlParams` 保留 `rev` 参数而不将其丢弃（扩展参数泛型类型与保留列表）。
  * 当 `?rev=N` 存在时：顶部展示醒目横幅标明「正在查看第 N 版历史快照（发布于 YYYY-MM-DD HH:mm），内容只读」，并提供「返回最新版」与「切换版本 / 查看与上一版差异」按钮。
  * **状态全面只读隔离**：历史视图不仅隔离 `aumbry:read:<id>`（stage 解锁与 quiz 作答），还要隔离 [useReading.ts](../../web/src/composables/useReading.ts#L215) 中的 `aumbry:skim:<id>` 偏好——传入 `?rev=N` 时所有阅读状态与 skim 状态使用内存态（不读写 `localStorage`），stage/collapse 可直接查看而不污染现行阅读进度；若 `?rev=N` 已被淘汰或无权访问（404 / 403），前端明确提示原因并提供「返回最新版」按钮，不静默替换成另一版本。
* **客户端版本对比面板（块级变更摘要 + 文本差异高亮）**：
  * 拉取两版可访问的 XML 后在客户端纯内存比对：
  * **结构化块级摘要**：按 `<meta>` 字段（`title`、`summary`、`tags`）及正文结构节点（优先按共享 `id` 配对 `section`/`stage`/`quiz`/`equation`/`figure`/`audio`，无 ID 块按类型+序号匹配），统计「新增 / 删除 / 修改」列表并支持点击跳转。
  * **文本差异高亮**：对匹配到的文本块使用开源轻量级库 `diff`（如 npm 包 `diff` 的词/行级比对）在模板中转义渲染 `<ins>` / `<del>` 差异高亮。

---

## 5. 播客与音频、流式上传与 HTTP Range（G2 · M1）

### 5.1 XML 协议规范（`<audio>`）

在 `<document version="1.2">` 中新增块级元素 `<audio>`：

* **允许容器**：`body`、`section`、`stage`、`collapse`、`tab`、`step`（每篇文档最多 `4` 个 `<audio>` 块，超出报 `INVALID_AUDIO`）。若 `<audio>` 放在 `<tab>` 内，切换标签页时由 Reka UI 默认卸载行为停止播放（已确认接受，不在本版改用全保活挂载）。
* **属性**：
  * `src`（**必填**）：本地相对音频路径（规则同 `<figure src>`），禁止外链、绝对路径与 `..`。
  * `title`（**必填**）：非空标题（最大 200 Unicode 字符，不合报 `INVALID_AUDIO`）。
  * `id`（可选）：符合 `^[a-z0-9-]{1,32}$` 且纳入全文共享 ID 命名空间（锚点 `#audio-<id>`）。
* **子元素**：
  * `<chapter>`（0 到 100 个）：
    * `start`（**必填**）：`HH:MM:SS` 格式（正则 `^\d{2,}:\d{2}:\d{2}$`，分钟与秒限 `00`–`59`）。**同 `<audio>` 内所有 `<chapter>` 的 `start` 必须满足非严格单调递增**（`start[i] >= start[i-1]`，允许同秒连续章节；前一条严格大于后一条时报 `INVALID_AUDIO`）。
    * `title`（**必填**）：非空章节标题。
    * `target`（可选）：同 `<next-steps><option target>`，遵循全局严格锚点校验（必须命中文档内已有锚点，否则报 `INVALID_TARGET`）。
  * `<transcript>`（0 到 1 个）：
    * 允许纯文本或 `<![CDATA[...]]>`（加入 [pkg/schema/parse.go](../../pkg/schema/parse.go) 的 `cdataTextElements`），不允许嵌套子元素。
    * 字节上限：`512 KB`（`512 * 1024` bytes），超出报 `INVALID_AUDIO`；存在该标签但内容为空或纯空白时拒绝（`INVALID_AUDIO`）。
* **全文索引规则**：
  * `Document.PlainText()` 收录 `<audio title>`、每个 `<chapter title>` 以及 `<transcript>` 的文本内容；**绝对不把音频二进制内容、音频 MIME 或文件名计入正文索引**。

### 5.2 支持封装、MIME 识别、纯 Go 时长解析与首音频投影

* **接受的封装格式（规范化校验）**：
  * `.mp3`：MIME 嗅探为 `audio/mpeg`；须通过纯 Go MPEG 帧解析器（`github.com/tcolgate/mp3`，限定解析步长与最大帧头搜索预算）验证包含有效音频帧，损坏文件报 `422 INVALID_ASSET`。
  * `.wav`：MIME 嗅探为 `audio/wav`、`audio/x-wav` 或 `audio/vnd.wave`（入库与响应统一为 `audio/wav`）；通过 Go 标准库只读解析 RIFF / WAVE `fmt ` 与 `data` chunk，校验采样率与块对齐有效并计算时长，结构损坏报 `422 INVALID_ASSET`。
  * `.m4a`：扩展名须为 `.m4a`，MIME 嗅探为 `audio/mp4`、`audio/x-m4a` 或 `video/mp4` 时均需通过纯 Go MP4 解析器（`github.com/abema/go-mp4`，限定 box 遍历上限防畸形嵌套）实际检查 `moov/trak/mdia/hdlr` Box：**必须存在至少一条音频轨（`handler_type == "soun"`），并从 `mdhd` / `mvhd` 读取精确时长**；仅含视频轨或结构损坏的文件一律报 `422 INVALID_ASSET`；合法 `.m4a` 入库 `mime_type` 统一为 `audio/mp4`。
  * 上述封装以外的音频文件（如 `.ogg`、`.flac` 等）一律以 `INVALID_ASSET_TYPE` / `INVALID_AUDIO` 拒绝。有效音频文件仅因缺少 Xing/VBRI 或编码方式特殊导致无法确定精确时长时，允许 `duration_seconds` 存 `NULL`（不编造 0 或估数）。
* **数据库扩展（资产元数据 + `items` 首音频投影）**：
  ```sql
  ALTER TABLE assets ADD COLUMN duration_seconds INTEGER;
  ALTER TABLE assets ADD COLUMN page_count INTEGER;

  ALTER TABLE items ADD COLUMN primary_audio_hash TEXT;
  ALTER TABLE items ADD COLUMN primary_audio_chapters_json TEXT NOT NULL DEFAULT '[]';
  ```
  * **`primary_audio_hash` 与 `primary_audio_chapters_json`**：在 `saveItem` 事务及 `reindex` 时直接从已解析 XML 的**文档序首个 `<audio>`** 投影入库（无 `<audio>` 时 `primary_audio_hash = NULL`）。这样 `/feeds/{dir_id}/podcast.xml` 与 `rss.xml` 完全通过 SQL + `JOIN assets` 直接取首音频与时长，**单次刷 Feed 对对象存储 0 读取**。
* 扩展 [cmd/server/maintenance.go](../../cmd/server/maintenance.go#L185) `managedAssetPathRe` 为：`^/assets/[0-9a-f]{64}\.(png|jpg|webp|svg|mp3|wav|m4a|pdf)$`。

### 5.3 流式 Ingest、WebDAV 原子上传与体积上限

* **分类体积上限**：
  * 图片（`.png`/`.jpg`/`.jpeg`/`.webp`/`.svg`）：`20 MiB`
  * PDF（M2）：`50 MiB`
  * 音频（`.mp3`/`.wav`/`.m4a`）：`100 MiB`
  * 整次 `POST /api/v1/ingest` 请求体上限：由 `100 MiB` 调整为 **默认 `200 MiB`**，并通过配置 `ingest.max_body_mb`（`AUMBRY_INGEST_MAX_BODY_MB`，最小值 20）可配；`formMemory` 保持 `32 MiB`（超出部分落临时文件），`ingestConcurrencyLimit` 保持 `4`。
* **流式校验与写入**：
  * 资产上传时直接打开 `multipart.File`（实现 `io.ReadSeeker` / `io.ReaderAt`）：流式计算 SHA-256 与读取头部进行 MIME/媒体解析，**不把整份音频/PDF 一次性 `io.ReadAll` 进 `[]byte`**；完成后 `Seek(0, io.SeekStart)` 并直接交给 `s.store.Put(ctx, path, f)`。
* **WebDAV 原子上传修复**：
  * 现有 `WebDAV.Put` 直接写最终路径，断流会留下半截文件并在重推时被 `Exists` 误认为已存在。改为：先以随机临时路径 `/xml/.tmp-<rand>` 或 `/assets/.tmp-<rand>`（同目录）写入流，写入并关闭成功后调用 `gowebdav.Rename`（HTTP `MOVE`）原子切换到目标路径，失败则尽最大努力 `Remove` 临时文件；`WebDAV.List` 过滤所有 `.tmp-*` 文件。
* **代理与客户端配置同步**：
  * CLI 默认上传超时（[internal/client/client.go](../../internal/client/client.go)）由 `2 * time.Minute` 调整为 `5 * time.Minute`。
  * 同步更新 [examples/deploy/nginx/aumbry.conf](../../examples/deploy/nginx/aumbry.conf#L25) 的 `client_max_body_size` 为 `201m`（并补充部署文档提醒：反代上限需始终大于 `ingest.max_body_mb`、临时目录需预留至少 `4 × max_body_mb` 磁盘空间）。

### 5.4 `Store.OpenRange` 与标准库 `http.ServeContent` Range 服务

采用评审采纳的 **简化方案 #1**：`OpenRange` 不返回 `totalSize`，直接以数据库权威 `assets.size_bytes` 搭配 Lazy `io.ReadSeeker` 交由 Go 标准库 `http.ServeContent` 处理，一举解决 S3 `206 ContentLength` 被误当总长及手写 Range 边界遗漏。

1. **扩展 `storage.Store` 接口**（[internal/storage/storage.go](../../internal/storage/storage.go)）：
   ```go
   // OpenRange reads an inclusive byte range [start, end] where 0 <= start <= end.
   OpenRange(ctx context.Context, name string, start, end int64) (io.ReadCloser, error)
   ```
   * **`local` 后端**：`root.Open` 后 `Seek(start, io.SeekStart)` 并以 `io.LimitReader(f, end-start+1)` 封装关闭器。
   * **`s3` / `r2` 后端**：调用 `GetObject` 传入 `Range: fmt.Sprintf("bytes=%d-%d", start, end)`，校验响应若为 `206` 或合法范围返回截断限长流（如遇服务端误返回全量 `200` 则按需跳读与限长，防止返回超长字节）。
   * **`webdav` 后端**：**不调用 `gowebdav.ReadStreamRange`**（该库未传 `ctx` 且上游忽略 Range 返回 200 时在开放区间会出错）。改由内置可取消 `http.NewRequestWithContext` 发起带 `Range: bytes=<start>-<end>` 头的 GET 请求：
     * 上游返回 `206 Partial Content`：直接限长 `end-start+1` 返回流；
     * 上游忽略 `Range` 返回 `200 OK`：在尊重 `ctx` 的前提下丢弃前 `start` 字节（`io.CopyN(io.Discard, ...)` 分块检查 `ctx.Err()`），再以 `io.LimitReader(body, end-start+1)` 返回。
2. **基于 `http.ServeContent` 的 `GET` / `HEAD /api/v1/assets/{hash}`**：
   * 权限检查与 `assets(mime_type, size_bytes, storage_path, created_at)` 查询在短只读事务中完成，**先发送确定性安全头与缓存头**：`Content-Type`、`X-Content-Type-Options: nosniff`、`Content-Security-Policy: default-src 'none'; sandbox`、`Cache-Control: private, max-age=86400`、`Accept-Ranges: bytes`；确认若经反代也不对音频/PDF `206` 做 gzip 压缩（可在头中显式避免中间层破坏字节范围，如仅在二进制类型下由应用直出且不主动压缩）。
   * 构造轻量适配器 `storeRangeSeeker` 实现 `io.ReadSeeker`（`Size() = asset.SizeBytes`，`Seek` 仅维护逻辑偏移量，`Read` 在首次读取当前连续窗口时按剩余区间或分块边界懒调用 `store.OpenRange` 缓冲拉流），然后直接调用 `http.ServeContent(w, r, "", asset.CreatedAt, seeker)`：
     * 标准库自动正确处理 `HEAD`（0 存储字节读取，精准返回 `Content-Length`）、单段 `Range: bytes=start-end` / `bytes=start-` / `bytes=-suffix`、`206 Partial Content`、`Content-Range`、`416 Range Not Satisfiable` 与完整 `GET`（含 `Content-Length`）。

### 5.5 阅读页音频播放器（`AudioBlock.vue`）

* HTML5 原生 `<audio controls preload="metadata">` 播放器，绑定 DOM 锚点 `#audio-<id>`，渲染标题、时间进度及章节目录。
* 点击章节跳转时间并开始播放；当章节带 `target` 时，显示「定位到正文」按钮并调用 `anchorMap` 的统一 `navigate`/`reveal`（支持自动解锁 `gated` 阶段与展开折叠容器）。
* 可折叠 `<transcript>` 面板；`@media print` 与 `exportMarkdown`（通过 `data-export-md`）隐藏 `<audio>` 控件，保留标题、章节列表与逐字稿内容。

---

## 6. 目录订阅源与目录级可撤销只读 Token（G3 · M1）

### 6.1 `public_url` 配置

RSS 2.0 中的 `<link>`、`<guid>` 与 `<enclosure url="...">` 中的地址需要稳定基址：

* 新增可选配置 `public_url`（环境变量 `AUMBRY_PUBLIC_URL`）。
* 非空时校验必须为合法 `http://` 或 `https://` URL，**不含路径、查询参数、片段或用户信息**（末尾 `/` 自动去除）。
* 为空时结合既有 `trusted_proxies` 信任链从请求的 `Scheme` 与 `Host` 推导。

### 6.2 目录级可撤销只读 / 订阅 Token（已定决策：A1 + 简化 #4）

采纳 **A1**（独立 `accessibleByDirToken` 判定，不穿透子目录独立密码）与 **简化 #4**（移除 `last_used_at` 列以消除单连接 SQLite 写入放大）：

```sql
CREATE TABLE IF NOT EXISTS directory_tokens (
    id           TEXT PRIMARY KEY,
    directory_id TEXT NOT NULL REFERENCES directories(id) ON DELETE CASCADE,
    label        TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_directory_tokens_dir ON directory_tokens(directory_id);
```

* **Token 格式与存储**：前缀固定 `ad_` + 32 字符随机 URL 安全字母数字串；数据库**仅存 SHA-256 Hex 摘要**（`token_hash`）。创建/轮换响应须附带 `Cache-Control: private, no-store`，明文仅在该次响应中返回一次。
* **管理接口（均需 `requireAdmin`）**：
  * `GET /api/v1/admin/dirs/{id}/tokens`
  * `POST /api/v1/admin/dirs/{id}/tokens`
  * `POST /api/v1/admin/dirs/{id}/tokens/{token_id}/rotate`（更新 `token_hash`，旧 Token 立即失效）
  * `DELETE /api/v1/admin/dirs/{id}/tokens/{token_id}`（立即吊销）
  * 后台 [AdminView.vue](../../web/src/views/AdminView.vue) 增加目录订阅 Token 管理弹窗/面板，方便一键复制带 `?token=` 的 RSS / Podcast 地址。
* **A1 严格 Fail-Closed 子树判定函数 `accessibleByDirToken(D, X)`**：
  * 为目录 Token 编写**独立遍历判定**（禁止复用穿透祖先解锁的 `dirTree.accessible`）：
    1. 若 `X` 不存在或父链有环/缺失父节点 → 返回 `false`；
    2. 从 `X` 沿 `ParentID` 向上走到 `D`：
       * 如果在到达 `D` 之前（即 `X` 自身或 `D` 与 `X` 之间的任何中间节点）遇到 `PasswordHash != ""`（含设了独立密码的子目录，以及与 `D` 自身无关但在下面的锁）→ **注意边界**：
         * 当 `X == D` 时，允许（Token 正是为 `D` 签发）；
         * 当 `X` 是 `D` 的严格后代（`X != D`）时，路径 `(X -> ... -> D)`（含 `X`，**不含 `D`**）上的**任何节点均不可有 `PasswordHash != ""`**，否则返回 `false`（严格不穿透 `D` 下单独加锁的子目录，子目录须使用自己的 Token）；
       * 如果一直走到根仍未遇到 `D`（说明 `X` 不在 `D` 子树内，如父目录或兄弟目录）→ 返回 `false`；
    3. 到达 `D` 后，**继续验证 `D -> root` 整条上游父链完整无环无缺失节点**（与既有 fail-closed 不变式一致），全部完整才返回 `true`。
  * **密码解耦语义**：修改目录 `D` 的访客密码（或清空密码）**不会自动吊销**已签发的 `ad_` Token；两套凭据解耦，管理员需停用某订阅者时在后台轮换或撤销对应 Token 即可。
* **全链路日志脱敏与禁缓存**：
  * **Go 服务端外层中间件**：在 `httplog.RequestLogger` 之前包装脱敏中间件——先从 `r.URL.Query().Get("token")` 读取原始 Token 存入私有 `context` key，随后构造浅拷贝请求将 `URL.RawQuery` 中的 `token` 参数替换为 `REDACTED` 传给 `httplog`（因为 `httplog/v3` 在 `middleware.go:162` 会把整条 `r.URL` 打印进主日志消息体），并向后续 handler 传递从 context 提取 Token 的干净值，确保任何 HTTP 请求日志与 panic 恢复日志绝不出现 Token 明文。
  * **响应头禁缓存**：受保护目录的 `rss.xml` / `podcast.xml`（因响应内含带 `?token=` 的 enclosure 链接）、`/feeds/{dir_id}/assets/{hash}`、以及后台 Token 创建/轮换接口统一设置 `Cache-Control: private, no-store`。
  * **Nginx 示例同步**：更新 [examples/deploy/nginx/aumbry.conf](../../examples/deploy/nginx/aumbry.conf) 注释与脱敏建议，提示反代不要在访问日志原样落盘 `?token=` query 或缓存 `/feeds/`。

### 6.3 RSS 2.0 订阅源与授权 Enclosure 交付（简化 #2 + RSS 合规修复）

采纳 **简化 #2**：Feed 接口**只认匿名（公开目录）与 `?token=ad_...`（保护目录）两种身份**，不读取管理员/访客浏览器 Cookie（防止管理员误把含 Cookie 私有条目的链接复制给外部阅读器），也不做播客客户端不使用的 `Authorization: Bearer` 头分支。

1. **`GET /feeds/{dir_id}/rss.xml[?token=...]`**：
   * `Content-Type: application/rss+xml; charset=utf-8`
   * 范围与去重：收集 `dir_id` 下经当前授权（匿名仅公开路径；`ad_` 走 `accessibleByDirToken`）可见的所有目录 ID 集合，在 SQL 中以 **`SELECT DISTINCT`** 按 `i.report_date DESC, i.created_at DESC, i.id ASC` 取最近 **50** 条已发布条目（防止同一条目挂在父子两个目录时在 Feed 中重复出现）。
   * RSS 2.0 标准字段与命名空间：
     * 声明 `xmlns:dc="http://purl.org/dc/elements/1.1/"`（及含音频时声明 `xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"`）。
     * 频道元数据：`<title>`、`<link>`、`<description>`、`<language>zh-CN</language>`。
     * 每条 `<item>`：
       * `<title>`、`<link>`（`{public_url}/r/<id>` 或 `/p/<id>`）；
       * **`<guid isPermaLink="false">aumbry:item:<id></guid>`**（与 `public_url` 域名变更解耦，更换域名不会导致所有阅读器把历史条目当成新条目重推）；
       * **`<pubDate>`**：结合 `report_date`（日期部分）与 `created_at` 的 UTC 时分秒合成确定性 `RFC 1123Z` 时间戳，避免同天多篇文章顺序错乱；
       * **`<dc:creator>`**：输出 `source_agent`（不使用要求邮箱的 `<author>` 与要求源 Feed URL 的 `<source>`）；
       * `<category>`（来自 `tags`）、`<description>`（`summary`）；
       * 当 `primary_audio_hash` 非空时附加 `<enclosure url="..." length="<size_bytes>" type="<mime_type>"/>`。
2. **`GET /feeds/{dir_id}/podcast.xml[?token=...]`**：
   * 相同目录子树与 `DISTINCT` 去重，**通过 `WHERE i.primary_audio_hash IS NOT NULL` 在 SQL 直接过滤并取最近 50 条**（零额外存储对象读取）。
   * 频道级默认标签：`<itunes:explicit>false</itunes:explicit>`、`<language>zh-CN</language>`；每条 `<item>` 带 `<enclosure>`、`<dc:creator>`、`<guid isPermaLink="false">`、`<pubDate>`，当 `assets.duration_seconds` 非空时输出 `<itunes:duration>`（**为 NULL 时省略该标签**，不编造），并在 `<description>` 中附带转义安全的章节列表（`HH:MM:SS — 标题`）。
   * 文档说明：WAV 在 Apple Podcasts 客户端可能不被支持（建议面向 Apple Podcasts 订阅时优先使用 MP3 / M4A）；使用 Pocket Casts / Overcast 等云端拉取型播客客户端订阅受保护目录时，其云端服务器会持有订阅 URL 中的 `?token=` 且依赖可公网访问的 `public_url`。
3. **鉴权与资产代理 `/feeds/{dir_id}/assets/{hash}?token=<tok>`**：
   * 公开目录（`dir_id` 无密码且到根链路全公开）：`rss.xml` 与 `podcast.xml` 可匿名访问，enclosure 直接指向 `{public_url}/api/v1/assets/{hash}`（匿名直接读取并支持 `Range`）。若此时误传 `?token=` 到纯公开目录且其下方存在保护子目录，仅在 `token` 对应 `dir_id` 且有效时放行该子目录（注意不以任何 Cookie 放行）。
   * 保护目录（`dir_id` 本身或任一祖先带密码）：未携带有效绑定到 `dir_id` 的 `?token=ad_<...>` 一律返回 `401 Unauthorized`（仅 JSON 错误，不返回 `WWW-Authenticate: Bearer`，不泄露受保护标题/内容）。有效请求返回的 `<enclosure>` 指向 `{public_url}/feeds/{dir_id}/assets/{hash}?token=<tok>`，且 Feed 响应携带 `Cache-Control: private, no-store`。
   * **`GET` / `HEAD /feeds/{dir_id}/assets/{hash}?token=<tok>`**：在短只读事务内同时验证 ① Token 合法且属于 `dir_id`，② `hash` 当前关联着挂载在 `accessibleByDirToken(dir_id, X)` 可见目录内的至少一个已发布条目（防止跨目录猜哈希越权盗图/盗音）；随后共享 §5.4 同一套 `http.ServeContent` Range 流逻辑，响应携带 `Cache-Control: private, no-store`、`Content-Security-Policy: default-src 'none'; sandbox`。
4. **前端入口**：公开目录在 [DirectoryView.vue](../../web/src/views/DirectoryView.vue) 顶部展示「RSS / Podcast 订阅地址」一键复制；保护目录仅在后台管理面板复制含 `?token=` 的地址。

---

## 7. 可选 PDF 原文附件（G6 · M2）

### 7.1 XML 协议规则与服务端校验

在 `<document version="1.2">` 中新增块级元素 `<attachment>`：

* **允许容器**：`body`、`section`、`stage`（每篇文档最多 `4` 个 `<attachment>`）。
* **属性**：
  * `type`（**必填**）：本版仅接受 `"pdf"`，其他值报 `INVALID_ATTACHMENT`。
  * `src`（**必填**）：本地相对路径（`.pdf`），规则同 `<figure src>` / `<audio src>`。
  * `title`（**必填**）：非空附件标题。
  * `id`（可选）：符合 `^[a-z0-9-]{1,32}$` 且纳入全文共享 ID 命名空间（锚点 `#att-<id>`）。
  * **不接受手填 `pages` 属性**（交由服务端以文件为准）。
* **纯 Go 解析与页数校验**：
  * 选定纯 Go（`CGO_ENABLED=0`）库 `github.com/pdfcpu/pdfcpu`（宽松只读模式读取页数并限制解析资源预算）校验上传资产：
    * **允许仅有 owner password（空 user password，arXiv 常见）的 PDF**正常读取页数与托管；
    * **必须拒绝**需要 user password 解锁的加密 PDF、结构损坏文件或页数为 `0` 的 PDF，均以 `422 INVALID_ATTACHMENT` 返回。
  * 真实总页数存入 `assets.page_count`。
  * 文档中任何 `<next-steps><option target="#att-<id>:p=<N>">` 或 `<audio><chapter target="#att-<id>:p=<N>">`：Go `pkg/schema` 校验语法有效并将页码存入 `pendingTarget`，服务端 `ingest` 在写库前校验 `N <= page_count`，越界报 `422 INVALID_TARGET`。
* **全文索引**：`PlainText()` 收录 `<attachment title>`，不抽取或索引 PDF 内文。

### 7.2 站内安全渲染与页跳转（`AttachmentBlock.vue`，采纳简化 #5）

* **严禁 `<iframe>` / `<embed>` / `<object>`**：匹配既有 `FrameDeny: true` 与 `CSP sandbox`，彻底杜绝内嵌 PDF JS 执行。
* **纯 Canvas 渲染方案（简化 #5：不做 TextLayer 与 AnnotationLayer）**：
  * 按需动态 `import()` 引入 `pdfjs-dist` 核心库（独立打包 chunk，仅当页面出现 `<attachment type="pdf">` 时加载）。
  * 仅导入核心 `getDocument` 与 `page.render` 至 `<canvas>`，**不引入 `pdf_viewer` 的 scripting / annotation / text-layer 模块**；调用参数**显式设置**：
    * `disableAutoFetch: true`（禁止拿到首页后后台擅自拖完 50 MB 全文）
    * `disableStream: true`（强制走 HTTP `Range` 分片按需请求当前页）
    * `isEvalSupported: false`（禁止 JS `eval`/函数构造）
  * 仅渲染每页 `<canvas>`，配上一页/下一页、页码跳转输入、缩放及直接下载按钮（`<a download>`）。
* 与 `anchorMap` 集成：剥离 `#att-<id>:p=<N>` 后定位到 `#att-<id>` 并展开所在 `stage`/容器，同时把目标页码 `N` 传递至对应 `AttachmentBlock.vue` 立即切换并渲染第 `N` 页。
* 打印与 `exportMarkdown` 输出附件标题与资产占位引用，不输出多页 Canvas 位图。

---

## 8. 外链图片自动转存（G8 · M2）

### 8.1 协议口径与离线校验边界

* 仅在 **`<document version="1.2">`** 中，`//figure[@src]` 允许取值为相对本地路径 **或** 符合规范的 `http://` / `https://` URL（`version="1.1"` 保持禁止外链）。
* **CLI `aumbry validate` 保持 100% 离线**：校验 URL 合法性（只允许 `http`/`https`、非空 `Host`、不含 `userinfo`），并离线拒绝字面回环/私网/链路本地主机或 IP（如 `localhost`、`127.*`、`::1`、`169.254.*`、`10.*`、`172.16-31.*`、`192.168.*`、`0.0.0.0`），返回 `REMOTE_ASSET_BLOCKED`；**严禁发起网络请求**。
* **`aumbry push`**：对 `http(s)://` 的 `<figure src>` 跳过本地 `os.OpenRoot` 打开与上传，将其保留在 XML 中交给服务端 `ingest` 转存。

### 8.2 服务端原子转存与 SSRF 安全防线

新增配置 `ingest.allow_remote_images`（环境变量 `AUMBRY_INGEST_ALLOW_REMOTE_IMAGES`，默认值 `true`）。设为 `false` 时，任何含外链 `<figure src>` 的 `ingest` 一律返回 `422 REMOTE_ASSET_BLOCKED`。

1. **SSRF 强制防御边界（禁用代理 + `Dialer.Control` + Redirect 重检）**：
   * 专用抓图 `http.Transport` **必须显式设置 `Proxy: nil`**（绝对不克隆或继承 `HTTP_PROXY` / `HTTPS_PROXY` 环境变量，否则 `Control` 仅看到代理服务器 IP 而绕过目标拦截）。
   * 在自定义 `net.Dialer.Control(network, address, rawConn)` 中对实际建立连接前的 `netip.AddrPort` 做 `.Addr().Unmap()` 解包（消除 IPv4-mapped IPv6 `::ffff:127.0.0.1` 绕过）并拒接一切非公网地址：回环（`IsLoopback`）、未指定（`IsUnspecified`）、私网与 ULA（`IsPrivate`）、链路本地单播/组播（`IsLinkLocalUnicast`/`IsLinkLocalMulticast` 含 `169.254.0.0/16` 与 `fe80::/10`）、组播（`IsMulticast`）、CGNAT（`100.64.0.0/10`）、以及保留网段（`192.0.0.0/24`、`198.18.0.0/15` 等非全球单播地址）。
   * 仅允许 `http`/`https` 且禁止 `userinfo`；`CheckRedirect` 最多 5 跳，每跳重验 URL 并经过上述 `Control` 检查，任何跳转至内网均立刻中止报 `REMOTE_ASSET_BLOCKED`。
2. **资源、格式与 SVG XML 安全校验**：
   * 并发控制：`4`（注：在服务全局 4 个 ingest 并发槽位下，一次外链抓取最多占用 60 秒预算，运维文档中提醒此并发占用特征）。单图超时 `15s`，单篇总抓取预算 `60s`，单篇最多 `20` 个独立远程图 URL（去重），单图体积硬上限 `20 MiB`。
   * 发请求时携带 `Accept: image/png,image/jpeg,image/webp,image/svg+xml`（避免 CDN 返回 `avif` 等不支持格式）；**不做严格的 URL 扩展名与 MIME 一致性拒绝**（避免误杀将 `.jpg` 响应为 `webp` 的常见 CDN）。
   * 对响应体执行 `mimetype.Detect`，仅允许 `image/png`、`image/jpeg`、`image/webp`、`image/svg+xml`。若嗅探为 `image/svg+xml`，还必须用 `helium` 解析为良构 XML（禁用 DTD，根元素必须是 `svg`），拒绝断截残缺的恶意或损坏 SVG（继续配合下发时的 `Content-Security-Policy: default-src 'none'; sandbox`）。
3. **原子提交保障**：
   * 任意外链触发拦截、失败或格式不符，整次 `ingest` 立刻失败返回 `422`，数据库与对外版本零变化。
   * 转存成功后，**仅针对被转存的 `<figure src>` 属性**重写为 `/api/v1/assets/<hash>`（不触碰 `<meta><canonical-url>` 等其它可能出现同一外链 URL 的语义元素）。

---

## 9. MCP Server（G7 · M2）

### 9.1 传输、依赖与路由挂载

* 在既有 HTTP Router（[internal/api/router.go](../../internal/api/router.go)）挂载 Streamable HTTP MCP 端点 **`/mcp`**（支持 `POST /mcp`、`GET /mcp` SSE、`DELETE /mcp`）。
* 采用官方纯 Go SDK `github.com/modelcontextprotocol/go-sdk`（核对基线 `v1.8.0`，`CGO_ENABLED=0` 兼容）。

### 9.2 三类身份鉴权与逐请求/会话加固（采纳简化 #3 + Codex #5/#8/#16）

采纳 **简化 #3**：MCP 仅服务于 Agent 与 CLI 生态，**不接受浏览器管理员或访客 Cookie**（仅识别匿名、`ad_` 目录 Token、API Token 三类身份）。

| 身份来源 | 可读目录范围 | 可读条目（`list` / `search` / `get_item` / 历史 `rev`） | `push_item` 写入 |
| :--- | :--- | :--- | :--- |
| **1. 目录级只读 Token**（`Authorization: Bearer ad_<...>`） | 仅按 A1 规则 `accessibleByDirToken(D, X)` 允许的 `D` 下可见子树 | 仅上述授权子树内**已挂载发布**的条目（含符合 B1 双门条件的历史修订） | **拒绝**（工具错误 `forbidden`） |
| **2. API Token**（`Authorization: Bearer <api_token>`）+ 可选头 `X-Aumbry-Dir-Tokens: ad_...,ad_...` | 全部**公开目录** + 通过 `X-Aumbry-Dir-Tokens` 显式授权的子树（同样走 A1） | ① 上述可见目录内已发布条目；② **自己创建（`created_by == digest`）的条目（含自建草稿及自己创建并挂载的受保护条目与自建修订）**；**绝不可读取其它 Token 的草稿或未获 `ad_` 授权的受保护目录条目** | **唯一允许写入的身份** |
| **3. 无凭据（匿名）** | 仅公开目录（`protected == false`） | 仅公开目录内**已挂载发布**的条目（含公开修订） | **拒绝**（`401` 或工具错误 `unauthorized`） |

**MCP 安全与传输加固（逐请求重新鉴权，不信任初始化态）**：

1. **会话与主体绑定**：对每个进来的 `POST`、`GET` SSE 与 `DELETE` 请求在进入/处理 MCP 前**重新验证凭据**，并在新建会话时把经规范化验权的身份主体写入非空 `TokenInfo.UserID`（防不同调用方猜中或复用他人 `Mcp-Session-Id`），匿名会话按客户端 IP 限制并发与速率，设置合理空闲超时（如 30 分钟）防无限累积内存。
2. **工具调用实时重算权限**：每次工具执行都依据当前请求重新验证所有 `ad_` / API Token（一旦目录 Token 在后台被撤销或轮换，已有 MCP 会话在下一条工具调用立即失效，绝不在会话上下文里缓存永久权限）。
3. **Host 与 Origin 校验**：显式配置 `Origin` 与受信 `Host` 校验（不依赖 SDK 默认仅保护 loopback 监听的默认检查，在反代绑定 `0.0.0.0:8080` 场景同样安全）。
4. **请求体上限对齐与错误模型区分**：
   * 传输层对 HTTP JSON-RPC 请求体设置 **`16 MiB` 上限**（避免 SDK 默认 `4 MiB` 上限误杀带 JSON 转义的 `10 MiB` XML），超限返回 HTTP `413`；鉴权缺失/非法 Token 或非法 Origin 在 HTTP 层按需返回 `401` / `403`。
   * 通过 MCP 工具通道触发的业务拒绝（如用 `ad_` Token 或匿名调用 `push_item`、无权访问指定 `dir_id` / `item_id`、XML 校验不通过），按 MCP 协议返回 `CallToolResult{IsError: true}` 与稳定可断言的错误标识（`forbidden`、`not_found`、`invalid_request`、`validation_failed`）。

### 9.3 工具清单、过滤语义与输出投影

1. **`list_dirs`**：可传 `parent_id`；只返回当前调用方可见目录。当某个公开目录下存在保护子目录且未获授权时，子目录在列表中保持与既有 `DirectoryView` 一致的 `locked: true` 且**清空受保护 `description` 细节**；跨目录查询 `parent_id` 时若对父目录无权则隐藏或返回未授权错误，仅投影可见层级字段。
2. **`list_items`**：支持 `dir_id`、`key`、`type`、`tag`、`limit`（默认 20，最大 100）、`offset`：
   * **权限谓词先于分页并独立与显式条件相交**：例如 API Token 未传 `dir_id` 时可包含自己创建的未挂载草稿；**传入 `dir_id` 时，授权集合必须与 `dir_id` 子树严格求交（自建条目若不在该 `dir_id` 下绝不可绕过 `dir_id` 返回）**。
   * **多挂载目录投影过滤**：条目的 `dirs` 输出数组**只保留当前调用者有权看到的挂载目录 ID**，严禁把条目兼挂的受保护秘密目录 ID 放进公开调用方的 `dirs` 字段。
   * **输出字段硬约束**：仅返回元数据（`id`、`type`、`title`、`date`、`summary`、`tags`、`source_agent`、`dirs`、`url`、`revision_count`），严禁包含 `xml` 或 `plain_text`。
3. **`search_items`**：参数 `query`（必填）、`dir_id`、`type`、`tag`、`from`、`to`、`limit`、`offset`：
   * 在 SQL 分页前应用统一权限条件 `((i.id IN 可见公开或授权子树条目) OR (i.created_by == callerAPITokenDigest))`，并与 `dir_id`、日期、标签条件 `AND` 相交；当可见目录集合为空且无权查自建条目时直接返回空结果而不报错。
   * 仅返回元数据、`snippet` 与可见 `dirs`，严禁包含完整 XML 正文。
4. **`get_item`**：参数 `id`（必填）、`rev`（可选正整数）、`format`（`"xml"` | `"text"`，默认 `"xml"`）：
   * 读最新正文需满足对应身份可读；带 `rev` 时**同样经过 B1 双门校验**（对于 API Token 读自己创建的非公开版本，当其作为 `created_by == digest` 读取自建版本或授权读其它版本时明确按矩阵判定）。
5. **`validate_xml`**：参数 `xml`（必填，解码后字节校验遵循 `<= 10 MiB` 即 `maxXMLSize`）；调用离线 `schema.Parse`，返回 `schema.ValidationResult`。
6. **`push_item`（API Token 专属）**：
   * 鉴权：非 API Token 一律返回工具错误 `forbidden`。
   * 限流与上限：校验 `xml` 解码后 `<= maxXMLSize`（`10 MiB`）；受 `writeLimiter` 与 `ingestConcurrencyLimit` 保护。
   * 校验与转存：走与 `POST /api/v1/ingest` 一致的目录校验、schema 解析、幂等键逻辑、历史版本快照与 §8.2 外链图片自动转存。若 XML 内含任何需要本地二进制上传的相对路径资源（`<figure src>`、`<audio src>` 或 `<attachment src>`），拒绝并返回明确 `invalid_request` 工具错误，指引改用 CLI `aumbry push`。

---

## 10. 配置、数据库迁移与运维汇总

### 10.1 新增配置键与环境变量（沿用 `check-config` 校验规则）

| TOML 键 | 环境变量 | 类型与默认值 | 说明与校验 | 阶段 |
| :--- | :--- | :--- | :--- | :--- |
| `public_url` | `AUMBRY_PUBLIC_URL` | string，默认空 | 订阅源绝对 URL 基址；非空须为 `http(s)://host[:port]`（无路径/查询/userinfo） | M1 |
| `revisions.max_per_item` | `AUMBRY_REVISIONS_MAX_PER_ITEM` | int，默认 `20` | 单条目最大保留修订数；`0` 不限制；`< 0` 启动失败 | M1 |
| `ingest.max_body_mb` | `AUMBRY_INGEST_MAX_BODY_MB` | int，默认 `200` | 单次 `ingest` 总大小上限（MiB）；最小允许 `20`，`<= 0` 使用默认，`1..19` 报错 | M1 |
| `ingest.allow_remote_images` | `AUMBRY_INGEST_ALLOW_REMOTE_IMAGES` | bool，默认 `true` | 是否允许服务端转存 `v1.2` XML 中的 `http(s)://` 外链图片 | M2 |

### 10.2 数据库迁移与升级顺序

延续 [internal/db/migrations/](../../internal/db/migrations/) 编号（当前已至 `00003`），所有新表仅在 **M1** 一次性迁移：

* `00004_item_links.sql`：新建 `item_links` 表及索引。
* `00005_item_revisions.sql`：新建 `item_revisions`、`item_revision_assets` 与 `item_revision_mounts` 表及索引。
* `00006_assets_and_items_metadata.sql`：`assets` 添加 `duration_seconds`、`page_count`；`items` 添加 `primary_audio_hash`、`primary_audio_chapters_json`。
* `00007_directory_tokens.sql`：新建 `directory_tokens` 表及索引（不含 `last_used_at`）。
* **升级部署步骤（必须在运维文档写明）**：由于离线 CLI 命令 `verify-storage` / `gc` / `reindex` 使用不跑迁移的 `db.OpenExisting`（[db.go](../../internal/db/db.go#L51)），从 `v0.1.3` 升级到 `v0.1.4` 时必须：**先启动新版 `aumbry-server`（让 `db.Open` 自动应用 `00004`–`00007` 迁移），随后停服运行一次 `aumbry-server reindex`**，完成存量条目的 `item_links`、`primary_audio_*` 与首个基线 `rev = 1` 回填后恢复对外服务。

### 10.3 运维、文档与 Agent Skill 同步

* M1 与 M2 合入时同步更新 [docs/en/](../../docs/en/)、[docs/zh-CN/](../../docs/zh-CN/)（`protocol.md`、`configuration.md`、`cli.md`、`deployment.md`）、[config.example.toml](../../config.example.toml)、[examples/deploy/nginx/aumbry.conf](../../examples/deploy/nginx/aumbry.conf)（`client_max_body_size 201m` 与不记/脱敏 query token 说明）以及 [skills/aumbry/SKILL.md](../../skills/aumbry/SKILL.md)，确保 `make skill-check` 绿灯。

---

## 11. 里程碑拆分

### Baseline（已合入 `master`，随 `v0.1.4` 一并发版）

* **G1**：`<md>` 支持 `\(...\)` 与 `\[...\]` 公式（commit `55814f2`）。

### M1（目标版本：`v0.1.4`）

1. **XML `version="1.2"` 基础与 `<audio>` 扩展**：双版本校验提示、`PlainText()` 收录逐字稿与章节、`#audio-<id>` 锚点、`<chapter start>` 非严格递增校验。
2. **流式 Ingest + `WebDAV` 原子写入 + `Store.OpenRange` + `http.ServeContent`**：流式 `multipart.File`、纯 Go `.mp3`/`.wav`/`.m4a` 解析与音轨校验、`primary_audio_*` 投影、全后端 `OpenRange`、标准库 Range 服务、客户端 5 分钟超时、扩展 `gc` 正则、Nginx `201m` 配置与前端 `AudioBlock.vue`。
3. **反向索引（G4）**：`item_links` 表、`saveItem` 原子更新、`reindex` 回填、`GET /items/{id}` 短只读事务 `referenced_by` 过滤与前端展示。
4. **修订历史（G5）**：`item_revisions` / `item_revision_assets` / `item_revision_mounts`（B1 双门权限）、未挂载转挂载/重新挂载全覆盖快照、存量回填与升级前更新保护、管理员单版删除、取消 `deleteItem` 直删 XML、前端 `?rev=N` 状态隔离 + 块级摘要 + `diff` 高亮。
5. **目录订阅源与目录 Token（G3）**：`public_url` 配置、`directory_tokens`（A1 fail-closed 子树判定）、日志 `?token=` 脱敏中间件、`private, no-store` 禁缓存头、RSS/Podcast 合规输出、`/feeds/{dir_id}/assets/{hash}` 代理与后台/目录 UI。
6. **文档与 Skill 同步**：M1 对应双语文档、示例配置与 `skills/aumbry/`。

### M2（目标版本：`v0.2.0`）

1. **可选 PDF 附件（G6）**：XML `v1.2` 增补 `<attachment type="pdf">`、双端剥离 `:p=<N>` 前缀、纯 Go `pdfcpu` 解析（放行仅 owner password 加密）与越界复核、前端懒加载 `pdfjs-dist` 纯 Canvas 渲染（`disableAutoFetch: true`、`disableStream: true`、`isEvalSupported: false`，不引入脚本/文本/注解层）。
2. **外链图片自动转存（G8）**：XML `v1.2` `<figure src>` 允许 `http(s)://`、`validate` 离线拦截、`push` 跳过远程上传、服务端禁用代理 `Proxy: nil` + `Control` `.Unmap()` SSRF 防护 + SVG 良构校验 + 仅重写 `<figure src>`。
3. **MCP Server（G7）**：引入 `github.com/modelcontextprotocol/go-sdk` 挂载 `/mcp`、3 类身份逐请求重验权、Host/Origin 校验、16 MiB 传输上限 / 10 MiB XML 上限、6 个工具与工具级错误语义。
4. **文档与 Skill 同步**：补全 PDF、外链转存与 MCP 指南及双语文档。

---

## 12. 验收标准

全部测试均须在 `CGO_ENABLED=0 go test -parallel=1 ./...` 与 `cd web && npm test && npm run typecheck && npm run build` 中全自动通过（不依赖 Docker 或真实外网；外链图片、SSRF 与代理禁用测试使用本地 `httptest.Server` 与受控测试 Dialer）。

### 12.1 M1（`v0.1.4`）验收标准

1. **公式回归（G1）**：`web/test/markdown.test.mjs` 全部用例通过，已有 `version="1.1"` 报告不改稿即可渲染 `\(...\)` 与紧跟段落后的 `\[...\]`，置信区间 `\[-1.4; +4.3\]` 不误渲染。
2. **反向索引（G4）**：
   * 先发详情 B 再发含 `detail-id="B"` 简报 A → `GET /items/B` 返回 `referenced_by` 含 A；支持多简报；`reindex` 对旧库回填 `item_links`；A 下线/删除/删去 `detail-id` 后入口消失。
   * A 所在目录未解锁时打开 B，`referenced_by` 完全隐藏 A（无标题/ID）；验证短只读事务内挂载快照一致性（不套用旧摘要+新权限）。
3. **修订历史（G5）**：
   * 推草稿（`--draft`）多次覆盖不增 rev；首次发布建 `rev = 1`；幂等相同内容不重复建 rev。
   * 覆盖 `已发布 v1 -> unpublish -> --draft 覆盖 v2 -> admin PUT /mounts 重新挂载` 路径：`current_rev` 更新指向 `v2`，旧 `v1` 仍可读。
   * 对模拟迁移前 0-revision 已发布条目执行 `reindex` 回填 `rev = 1`；并在“未先 `reindex` 直接推新版”的场景下验证旧版先入库为 `rev = 1`、新版为 `rev = 2`。
   * **B1 隐私边界**：在保护目录发布含私密图片的第 1 版 → 下线 → 推送脱敏第 2 版至公开目录。匿名访客可读第 2 版，但请求 `/revisions` 不返回第 1 版标题/条目，请求 `/revisions/1/content` 返回 `403`，请求第 1 版独占图片返回 `403`。
   * 管理员 `DELETE /api/v1/admin/items/{id}/revisions/1` 成功；删除 `current_rev` 返回 `409`；超 `max_per_item` 按 `rev ASC` 淘汰后经 `gc` 清理孤儿，在淘汰前独占资产受 `verify-storage` 与 `gc --dry-run` 保护；删除整篇条目不再提交后直删 XML。
   * 前端 `npm test` 覆含客户端块级/文本 `diff` 逻辑与 `?rev=N` 状态隔离不写入 `localStorage` 且保留 `rev` 参数。
4. **音频、流式 Ingest、WebDAV 原子写与 Range（G2）**：
   * 在保护目录上传有效 `mp3`/`wav`/`m4a`（校验非 0 时长入 `assets.duration_seconds` 与 `items.primary_audio_hash`）；拒绝仅含视频轨的 `.mp4`（伪装 `.m4a`）与损坏文件（`422`）；允许 `<chapter start>` 同秒递增，拒绝前大后小（`INVALID_AUDIO`）；`<chapter target>` 无效时 `aumbry validate` 报校验失败退出码非 `0` 且 `ingest` 返回 `422 INVALID_TARGET`；在 `version="1.1"` 中出现 `<audio>` 返回明确带升级提示的错误；`<transcript>` 入搜而音频二进制/纯文件名不误中。
   * `Store.OpenRange` 与 `/api/v1/assets/{hash}` 覆盖：`HEAD`（含 `Content-Length` 且不读正文）、`Range: bytes=0-1023` 返回 `206`（覆盖 `local`、进程内 `s3` 与测试 WebDAV 服务端返回 `206` 与忽略 Range 返回 `200` 两种模式及 `ctx` 取消）、后缀 `bytes=-512`、越界 `416`、多段 fallback 与完整 `GET` 带 `Content-Length`。
   * 上传 `> 32 MiB` 落盘 Multipart 文件并覆盖取消与临时文件回收；模拟 WebDAV `Put` 中途断流不留最终路径，重试相同内容重推仍完整可读。
5. **目录订阅与 Token（G3）**：
   * 公开目录匿名拉取 `rss.xml` 与 `podcast.xml`，校验 XML 转义（标题含 `&`）、`<dc:creator>`、`<guid isPermaLink="false">`、`pubDate`、`DISTINCT` 去重、enclosure `length == size_bytes`、首音频匹配及 `duration=NULL` 时省略 `<itunes:duration>`。
   * **A1 子树规则**：对保护父目录 `D` 与设了独立密码的子目录 `C`：持 `ad_D` 访问 `D` 正常、访问 `C` 及 `C` 下资产被拒绝（`401`/`403`）；持 `ad_C` 访问 `C` 正常、访问 `D` / 兄弟目录 / 父目录被拒；在 `testTree` 构造损坏祖先链（缺父/环）拒绝访问；Cookie 不穿透 Feed。
   * 保护 Feed、代理资产与后台 Token 响应均带 `Cache-Control: private, no-store`；旋转/吊销后旧 Token 立即失效；抓取 `httplog` 完整输出字节确认明文 `ad_` Token 零残留。
6. **M1 工程检查**：`make check`（`fmt-check`、`lint`、`vet`、`test`、`skill-check`、`release-check`）、`make setup test-web frontend build` 与 `CGO_ENABLED=0` 多架构交叉构建验证（如 `GOOS=linux/darwin/windows GOARCH=amd64/arm64 go build ./...`）全部通过。

### 12.2 M2（`v0.2.0`）验收标准

1. **PDF 原文附件（G6）**：
   * 多页有效 PDF（含仅带 owner password 的测试样本）上传成功并正确记录 `page_count`；user-password 加密或损坏 PDF 报 `422 INVALID_ATTACHMENT`；`#att-<id>:p=0` 与 `N > page_count` 在 `validate`/`ingest` 分别返回对应错误与 `422 INVALID_TARGET`；前端构建与测试核对 `anchor` 剥离 `:p=<N>` 解析、`pdfjs-dist` 调用显式配置 `disableAutoFetch: true` / `disableStream: true` / `isEvalSupported: false` 且未打入 `pdf_viewer` scripting 模块。
2. **外链图片转存（G8）**：
   * 本地 `httptest` 覆盖：设置环境变量 `HTTP_PROXY` / `HTTPS_PROXY` 指向探针代理，断言远程抓图不向代理发任何请求且触发连接级检查；`127.0.0.1`、`169.254.169.254`、`[::ffff:127.0.0.1]` 与 `302 -> 127.0.0.1`（及 5 跳重定向上限）均返回 `422 REMOTE_ASSET_BLOCKED`；`text/html` 假图、损坏 SVG（非良构 XML 或非 `<svg>` 根）、20 URL 上限与超时/60s 总预算均原子拒绝；允许 CDN 将 `.jpg` URL 返回合法 `image/webp` 并正确按实际 MIME 命名转存；保留同链接的 `<meta><canonical-url>` 不受误改而 `<figure src>` 变为 `/api/v1/assets/<hash>`；`aumbry validate` 离线不联网。
3. **MCP Server（G7）**：
   * 断言传输层 `401` / `403` / `413`（含 `Origin` 检查）与工具层 `CallToolResult.IsError = true`（`forbidden` / `not_found` / `invalid_request`）；`list_items`/`search_items` 不含正文且过滤 `dirs` 只保留调用方可见挂载目录；`list_dirs` 对未解锁子目录标记 `locked: true` 且清空描述；API Token 自建草稿不越过显式 `dir_id` 过滤，且无权搜读其它人的受保护条目或草稿；撤销 `ad_` Token 后同一 MCP 会话下一条调用立拒；支持 `> 4 MiB`（`<= 10 MiB`）纯文本 XML 走 `push_item`；`push_item` 拒绝含本地相对路径资产的文档。
4. **M2 工程检查**：`make check`、`make setup test-web frontend build` 与 `CGO_ENABLED=0` 全目标平台编译验证通过。

---

## 13. 决策记录

| 议题 | 冻结决策 | 依据与影响 |
| :--- | :--- | :--- |
| XML 协议版本 | 新增 **`version="1.2"`**；既有 `version="1.1"` 继续兼容并在含新块时提示升级 | 不混淆产品版本 `v2.0` 与协议版本；旧文档零迁移成本。 |
| 音频封装与解析 | 接受 **`mp3`、`wav`、`m4a`**；纯 Go 结构+音轨+时长校验 | 拒绝仅视频轨 `.mp4` 与坏文件；零 CGO。 |
| 体积上限与流式写入 | 图 `20 MiB` / PDF `50 MiB` / 音频 `100 MiB` / 总请求默认 `200 MiB`；`multipart.File` 流式写入与 WebDAV `.tmp-` MOVE 原子传 | 防 OOM 与断流残留半截文件；Nginx 示例同步调为 `201m`。 |
| 保护目录订阅与子树边界（**A1**） | 目录只读 Token（`ad_`）采用**独立 `accessibleByDirToken`**（严禁穿透子目录独立密码；改密码不自动吊销）并移除 `last_used_at`（**简化 #4**） | fail-closed 防越权读子锁，且避免 Range 请求高频写单连接 SQLite。 |
| 订阅源格式、合规与身份（**简化 #2**） | `rss.xml` + `podcast.xml`（RSS 2.0 + `<dc:creator>` + `isPermaLink="false"` + 首音频 SQL 投影）；Feed 只认匿名与 `ad_` | 播客客户端稳定兼容；拒绝 Cookie 泄漏与 50 次存储扇出读取。 |
| Range 实现方式（**简化 #1**） | `Store.OpenRange` 仅读 `[start, end]`，以 `assets.size_bytes` 驱动 Go 标准库 `http.ServeContent` | 消除 S3 `206 ContentLength` 混淆并免去手写边界分歧。 |
| 修订历史隐私与触发（**B1**） | 仅正式发布记录（含 unpublish 后草稿再挂载）；增加 `item_revision_mounts` 双门鉴权与管理员单版删除，存量 `reindex` 补 `rev=1` | 彻底堵住保护目录重发至公开目录后旧私密稿与旧图外泄漏洞。 |
| 修订差异展示 | 块级变更摘要 + 开源 `diff` 前端高亮；`?rev=N` 隔离读写进度与 skim 偏好 | 读者高效查改且绝不污染现行阅读进度。 |
| PDF 解析与渲染（**简化 #5**） | 纯 Go `pdfcpu`（放行 owner-password）；前端仅 `pdfjs-dist` Canvas 核心（`disableAutoFetch`/`disableStream`/`isEvalSupported: false`）+ 下载 | 不后台拖全文件、零脚本攻击面、免去 TextLayer/AnnotationLayer 包体与 CSS 复杂度。 |
| MCP 范围、身份与传输（**简化 #3**） | 读 + 校验 + `push_item`（仅 API Token）；仅留匿名/`ad_`/API Token；逐请求验权、16 MiB 请求限与标准工具错误语义 | 绝无 Cookie 会话绕过或跨目录情报泄漏。 |
| 外链图片转存 | 服务端禁用代理（`Proxy: nil`）+ `Dialer.Control` `.Unmap()` SSRF 防线 + 仅重写 `<figure src>`；放宽 CDN 扩展名但强检 MIME 与良构 SVG | 防住代理与 DNS Rebinding 绕过，不误杀 CDN WebP 优化，不误伤 `<canonical-url>`。 |
