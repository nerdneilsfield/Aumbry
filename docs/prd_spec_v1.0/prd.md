# Aumbry 产品需求与技术架构详细设计文档（PRD & Tech Spec v1.0）

| 文档属性     | 内容                                                         |
| :----------- | :----------------------------------------------------------- |
| **项目名称** | **Aumbry**（源自中世纪“带锁藏书壁龛”，多 AI Agent 调研报告中枢） |
| **文档版本** | v1.0（MVP 完整基线）                                         |
| **系统形态** | Go 单二进制服务端（内嵌 Web 前端）+ SQLite (FTS5) + 私有 WebDAV + Go 本地 CLI (`aumbry`) |
| **核心理念** | **Agent 只负责产出结构化 XML 与本地图片；页面渲染、排版样式、目录组织、鉴权分享与全文检索全部由站点统一接管。** |

---

## 1. 产品背景与核心目标

### 1.1 业务痛点

1. **排版与投递消耗过高**：多个 AI Agent 每日高频产出多种类型内容（收件箱/论文简报、AI 与自动驾驶日报、vibe coding 日报、Ego 方向周报、单篇论文精读、开源项目拆解等）。当前需由 Agent 自行拼装 HTML 或邮件格式并分发至 Telegram/Gmail/聊天窗口，不仅消耗大量上下文，且排版风格难以统一。
2. **归档与检索困难**：历史调研散落在邮箱与对话记录中，缺乏按主题目录与时间维度的沉淀，也无法进行精准的全文检索。
3. **内外分发冲突**：同一篇深度内容（如单篇论文精读）常需同时归入“个人完整版”与“可对外分享目录”，传统方式需维护两份文件；同时若正文或图片托管在公开 CDN 上，会导致目录密码形同虚设。

### 1.2 核心目标

* **极简写入闭环**：Agent 工作流固化为 `按 Schema 生成 XML + 本地图片` → `aumbry validate`（依据结构化 JSON 报错自修）→ `aumbry push`（一键推送或覆盖更新至指定目录）。
* **严格的目录级隔离**：以目录为权限边界，密码仅设在目录上；访客解锁后通过 Cookie 记住该目录及其整棵子树的会话，树内畅游免密、树外彻底不可见。
* **一文多挂**：内容与分类完全解耦，同一份报告可挂载到多个不同目录，单次更新全站生效。

---

## 2. 信息架构与权限模型

### 2.1 三类匿名资源（URL 去语义化）

所有对外暴露的资源 URL 均采用无业务含义的短随机 ID（12 位 NanoID，如 `V1StGXR8_Z5j`），不在 URL 中暴露分类名、日期或目录层级：

| 资源类型               | URL 路径  | 定位与职责                                                   |
| :--------------------- | :-------- | :----------------------------------------------------------- |
| **目录 (Directory)**   | `/d/{id}` | 容器节点，支持无限嵌套子目录，可挂载日报与详情报告；**密码保护的唯一载体**。 |
| **日报/汇总 (Report)** | `/r/{id}` | 周期性聚合内容（日报、周报、简报），可在正文中关联引用多个 `/p/{id}` 详情条目。 |
| **详情报告 (Post)**    | `/p/{id}` | 深度单篇内容（论文精读、开源项目拆解、长篇分析）。           |

### 2.2 多目录挂载机制

* 业务分类不写死在条目字段中，也不体现在 URL 中，而是通过 `directory_items` 多对多关系表实现。
* 同一篇 `/p/{id}` 可同时挂载于「个人完整知识库」与「对外公开论文精选」两个目录下。

### 2.3 角色与权限矩阵

| 权限维度              | 管理员 (Admin)                           | 访客 (Visitor)                                               |
| :-------------------- | :--------------------------------------- | :----------------------------------------------------------- |
| **认证方式**          | 独立账号密码登录（Admin Session Cookie） | 访问加密目录 `/d/{id}` 输入目录密码，签发签名 Cookie 记录已解锁目录 ID 集合 |
| **目录可见性**        | 查看与管理全站完整目录树                 | 仅可见已解锁目录及其**所有子孙目录**；无全站根目录入口，不可枚举爬取 |
| **子树漫游体验**      | 全站免密                                 | 解锁父目录后，访问其下所有子目录、`/r/{id}`、`/p/{id}` 均无需重复输密；切换至另一加密目录时单独验密 |
| **叶子直连策略 (v1)** | 允许直接访问任意 `/r/{id}` 或 `/p/{id}`  | 若无任何包含该条目的祖先目录有效会话，直接返回 `403 Forbidden`（v1 单篇默认不单独设密） |
| **全文搜索范围**      | 全站无限制搜索                           | 强制限定在**当前已解锁目录及其子树**范围内，绝不泄露树外报告 |
| **静态资源/图片**     | 凭 Admin Cookie 经服务端代理读取         | 凭有效目录子树 Cookie 经服务端代理读取（校验图片所属条目是否在当前会话子树内） |

---

## 3. 系统宏观架构与数据流

**关键安全架构决策**：废弃“前端直连公开 CDN”方案。WebDAV 退居为纯私有后端存储，所有 XML 正文与图片的读取均通过 Go 服务端鉴权后流式代理返回。

```text
                        [本地环境 / AI Agent 运行时]
                                     │
                      1. 产出 XML + 本地相对路径图片
                                     ▼
                     ┌───────────────────────────────┐
                     │       Go CLI (aumbry)         │
                     │  • config (默认目录/Token)     │
                     │  • validate (输出 JSON 报错)   │
                     │  • push (打包上传 & Upsert)    │
                     └───────────────┬───────────────┘
                                     │ 2. HTTP POST multipart/form-data
                                     │    (Authorization: Bearer <API_TOKEN>)
                                     ▼
┌────────────────────────────────────────────────────────────────────────────┐
│                        Aumbry Server (Go 单二进制)                          │
│                                                                            │
│  [Ingest API] ─▶ Schema 二次校验 ─▶ 图片 SHA-256 去重 ─▶ 路径重写 /api/assets/{hash}
│       │                                          │                         │
│       ▼                                          ▼                         │
│  [SQLite (WAL + FTS5)]                  [WebDAV 私有存储]                   │
│  • directories (树结构 & 密码哈希)       (仅服务端凭凭证读写，不对外公开)     │
│  • items (元数据 & 引用关系)                      ▲                         │
│  • directory_items (一文多挂)                     │                         │
│  • item_assets (条目与图片关联，用于图片鉴权)      │                         │
│  • items_fts (FTS5 trigram 全文索引)              │                         │
│       │                                          │                         │
│       ├──▶ [Auth & Query API] ◀──────────────────┘ (校验 Cookie 后流式代理) │
│       │     • 子树递归权限校验 & 两级 FTS5 搜索                              │
│       │     • /api/items/{id}/content (返回鉴权后的 XML)                    │
│       │     • /api/assets/{hash}      (返回图片 + Cache-Control: private)  │
│       ▼                                                                    │
│  [Web SPA (Vue 3 + TS, 通过 go:embed 内嵌进 Go 二进制)]                      │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. 技术栈选型与核心依赖清单

项目采用 **Go Monorepo** 架构，服务端与 CLI 共享 `pkg/schema` 校验代码，彻底杜绝本地校验与服务端校验不一致的问题。

### 4.1 后端与 CLI 技术栈（Go 1.23+）

| 领域                  | 选型 / 依赖包                                   | 版本建议 | 选型理由                                                     |
| :-------------------- | :---------------------------------------------- | :------- | :----------------------------------------------------------- |
| **HTTP 路由与中间件** | `github.com/go-chi/chi/v5`                      | `v5.1+`  | 极简、零外部依赖、完全兼容标准库 `net/http`，适合构建 REST API 与中间件鉴权链。 |
| **数据库驱动**        | `modernc.org/sqlite`                            | `v1.34+` | **纯 Go 实现，彻底免除 CGO 依赖**；支持 Linux/macOS/Windows 一键交叉编译，内置 FTS5。 |
| **SQL 辅助（可选）**  | `github.com/jmoiron/sqlx` 或原生 `database/sql` | `v1.4+`  | 减少结构体字段扫描样板代码，保持 SQL 可控性与透明性。        |
| **WebDAV 客户端**     | `github.com/studio-b12/gowebdav`                | `v0.10+` | 轻量稳定的 Go WebDAV 客户端，支持 `ReadStream`/`WriteStream` 流式读写，避免大文件撑爆内存。 |
| **密码哈希**          | `golang.org/x/crypto/bcrypt` (或 `argon2`)      | latest   | 行业标准慢哈希算法，用于安全存储管理员密码与目录密码。       |
| **安全 Cookie**       | `github.com/gorilla/securecookie`               | `v1.1+`  | 对访客已解锁的目录 ID 集合进行 HMAC-SHA256 签名与 AES 加密，防止客户端伪造篡改。 |
| **匿名短 ID**         | `github.com/matoous/go-nanoid/v2`               | `v2.1+`  | 生成 URL 安全、无语义、抗碰撞的 12 位短 ID。                 |
| **CLI 框架**          | `github.com/spf13/cobra`                        | `v1.8+`  | 组织 `aumbry config` / `validate` / `push` 子命令与 Flag 参数解析。 |
| **配置解析**          | `gopkg.in/yaml.v3`                              | `v3.0+`  | 解析 `~/.config/aumbry/config.yaml` 多 Profile 配置文件。    |
| **XML 解析**          | Go 标准库 `encoding/xml`                        | stdlib   | 原生支持流式解析与精确行号定位（配合 `xml.Decoder.InputOffset()`）。 |
| **日志**              | Go 标准库 `log/slog`                            | stdlib   | 结构化 JSON 日志，便于容器化采集与排查。                     |

### 4.2 Web 前端技术栈（Vue 3 方案）

> 注：前端编译产物（`web/dist`）通过 `go:embed` 直接打包进 Go 服务端二进制中，实现单文件零依赖部署。若团队更偏好 React，可将对应包替换为 `react` + `react-router-dom` + `react-markdown` + `rehype-katex`。

| 领域               | 选型 / 依赖包                                                | 选型理由                                                     |
| :----------------- | :----------------------------------------------------------- | :----------------------------------------------------------- |
| **核心框架与构建** | `vue` (v3.5+) + `typescript` + `vite` (v6+)                  | 响应式写法直观，构建速度快，适合中后台目录管理与文档阅读器。 |
| **路由与状态**     | `vue-router` (v4) + `pinia`                                  | 管理 `/d/:id`、`/r/:id`、`/p/:id`、`/admin` 路由及目录解锁状态。 |
| **样式与图标**     | `tailwindcss` (v4) + `@tailwindcss/typography` + `lucide-vue-next` | 原子化样式 + 开箱即用的高质量排版样式（`prose`）+ 极简线性图标。 |
| **XML 解析**       | 浏览器原生 `DOMParser`                                       | 零体积依赖，直接将服务端返回的 XML 字符串解析为标准 DOM 树，交由 Vue 递归渲染组件处理。 |
| **Markdown 渲染**  | `markdown-it` + `markdown-it-texmath`                        | 高性能渲染 `<md>` 块内的 GFM 表格、列表与文本。              |
| **数学公式**       | `katex`                                                      | 渲染学术论文中的行内公式（`$...$`）与块级公式（`$$...$$`），体积小且渲染极快。 |
| **代码高亮**       | `shiki`（或 `highlight.js`）                                 | 针对 vibe coding 日报与开源项目代码片段提供高精度语法高亮。  |
| **图片预览**       | `medium-zoom`（或自写轻量 Modal）                            | 实现 `<figure>` 图片的点击平滑放大（Lightbox）体验。         |

### 4.3 Go Monorepo 目录结构

```text
aumbry/
├── go.mod
├── go.sum
├── Makefile                      # 一键编译 frontend + server + multi-platform cli
├── Dockerfile                    # 多阶段构建（Node -> Go -> Alpine/Distroless）
├── pkg/
│   └── schema/                   # 【核心共享包】CLI 与 Server 共用
│       ├── types.go              # XML 结构体与元数据定义
│       ├── validator.go          # XML 语法、必填项、组件规范与本地图片校验
│       ├── extractor.go          # 提取纯文本用于 FTS5 索引 + 重写图片路径
│       └── errors.go             # 结构化 JSON 报错定义
├── cmd/
│   ├── aumbry/                   # 本地 CLI 入口
│   │   ├── main.go
│   │   └── commands/             # config.go, validate.go, push.go
│   └── server/                   # 服务端入口
│       └── main.go
├── internal/
│   ├── config/                   # 环境变量加载
│   ├── db/                       # SQLite 连接、WAL 初始化、Migration、FTS5 查询
│   ├── storage/                  # WebDAV 私有存储封装（Upload/Stream/Exists）
│   ├── auth/                     # Admin Session、目录子树 Cookie 签发与校验
│   ├── api/                      # HTTP Handlers (Ingest, Directory, Item, Asset, Search, Admin)
│   └── webembed/                 # go:embed 嵌入 web/dist 静态资源
└── web/                          # Vue 3 + TypeScript + Vite 前端工程
    ├── package.json
    └── src/
        ├── api/                  # Fetch 封装
        ├── views/                # DirectoryView, ReaderView, PasswordGate, AdminView
        └── components/
            └── renderer/         # XmlRenderer.vue + 各自定义富文本组件
```

---

## 5. 数据库设计与两级搜索实现（SQLite + FTS5）

SQLite 启动时需强制开启：`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`。

### 5.1 核心表结构（DDL）

```sql
-- 1. 目录表（支持无限嵌套，密码仅挂在目录上）
CREATE TABLE directories (
    id            TEXT PRIMARY KEY,              -- 12位 NanoID，对应 /d/{id}
    parent_id     TEXT REFERENCES directories(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,                 -- 目录名称（仅展示，不进 URL）
    description   TEXT NOT NULL DEFAULT '',      -- 目录简介
    password_hash TEXT,                          -- NULL 表示继承父目录或公开；非空为 bcrypt 哈希
    sort_order    INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_directories_parent ON directories(parent_id);

-- 2. 报告/详情条目表
CREATE TABLE items (
    id               TEXT PRIMARY KEY,           -- 12位 NanoID，对应 /r/{id} 或 /p/{id}
    external_key     TEXT UNIQUE,                -- 可选唯一键（如 "ego-weekly:2026-W47"），用于 CLI Upsert 覆盖更新
    type             TEXT NOT NULL CHECK (type IN ('report', 'post')), -- report=/r/, post=/p/
    title            TEXT NOT NULL,
    report_date      TEXT NOT NULL,              -- YYYY-MM-DD
    summary          TEXT NOT NULL,
    tags_json        TEXT NOT NULL DEFAULT '[]', -- JSON 字符串数组，如 ["Agent","VLM"]
    source_agent     TEXT NOT NULL,              -- 产出 Agent 标识
    canonical_url    TEXT NOT NULL DEFAULT '',   -- 规范原文链接（arXiv/PDF/GitHub）
    related_ids_json TEXT NOT NULL DEFAULT '[]', -- 关联条目 ID 数组
    xml_storage_path TEXT NOT NULL,              -- WebDAV 私有相对路径，如 /xml/{id}.xml
    plain_text       TEXT NOT NULL,              -- 提取后的纯文本（供 FTS5 同步）
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_items_date ON items(report_date DESC);
CREATE INDEX idx_items_type ON items(type);

-- 3. 目录与条目多对多挂载表（实现一文多挂）
CREATE TABLE directory_items (
    directory_id TEXT NOT NULL REFERENCES directories(id) ON DELETE CASCADE,
    item_id      TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    mounted_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (directory_id, item_id)
);
CREATE INDEX idx_dir_items_item ON directory_items(item_id);

-- 4. 资产映射表（用于图片 SHA-256 去重与访客图片鉴权）
CREATE TABLE assets (
    hash         TEXT PRIMARY KEY,               -- SHA-256 十六进制字符串
    mime_type    TEXT NOT NULL,                  -- 如 image/png, image/webp
    size_bytes   INTEGER NOT NULL,
    storage_path TEXT NOT NULL,                  -- WebDAV 路径，如 /assets/{hash}.png
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE item_assets (
    item_id    TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    asset_hash TEXT NOT NULL REFERENCES assets(hash) ON DELETE RESTRICT,
    PRIMARY KEY (item_id, asset_hash)
);
CREATE INDEX idx_item_assets_hash ON item_assets(asset_hash);

-- 5. SQLite FTS5 全文检索虚拟表（trigram 分词原生支持中英文）
CREATE VIRTUAL TABLE items_fts USING fts5(
    item_id UNINDEXED,
    title,
    summary,
    tags,
    plain_text,
    tokenize = 'trigram'
);
```

### 5.2 两级搜索与子树权限过滤 SQL

* **管理员全站搜索**：直接查询 `items_fts` 并关联 `items` 返回，无目录过滤约束。
* **访客目录内搜索（核心安全边界）**：服务端先验证访客 Cookie 是否包含 `target_dir_id`（或其任一祖先目录），随后使用 `WITH RECURSIVE` 强过滤，确保绝不越权泄露树外内容：

```sql
WITH RECURSIVE subtree(id) AS (
    SELECT id FROM directories WHERE id = :target_dir_id
    UNION ALL
    SELECT d.id FROM directories d
    INNER JOIN subtree s ON d.parent_id = s.id
)
SELECT DISTINCT
    i.id, i.type, i.title, i.report_date, i.summary, i.tags_json, i.source_agent,
    snippet(items_fts, 4, '<mark>', '</mark>', '...', 24) AS match_snippet
FROM items_fts
INNER JOIN items i ON i.id = items_fts.item_id
INNER JOIN directory_items di ON di.item_id = i.id
INNER JOIN subtree s ON s.id = di.directory_id
WHERE items_fts MATCH :query
ORDER BY bm25(items_fts), i.report_date DESC
LIMIT :limit OFFSET :offset;
```

### 5.3 资产（图片）鉴权逻辑

当访客请求 `GET /api/assets/{hash}` 时：

1. 若为 Admin Session，直接从 WebDAV 流式返回。
2. 若为访客，解析其 Cookie 中已解锁的目录 ID 列表 `unlocked_dir_ids`，执行一次轻量 SQL 检查是否存在满足 `item_assets.asset_hash = :hash` 且对应 `item_id` 挂载在 `unlocked_dir_ids` 任一子树下的记录。存在则流式返回并附加 `Cache-Control: private, max-age=86400`，否则返回 `403 Forbidden`。

---

## 6. 内容协议规范：Aumbry XML Schema (v1.0)

采用 **语义化 XML 标签 + `<![CDATA[...]]>`** 混合协议。大模型对标签闭合容错率高，且正文包在 CDATA 内可彻底避免 `<`、`>`、`&` 及代码/公式符号引发的 XML 转义错误。

### 6.1 完整 XML 示例

```xml
<?xml version="1.0" encoding="UTF-8"?>
<document version="1.0">
  <meta>
    <title>2026-11-20 AI 与自动驾驶前沿日报</title>
    <type>report</type> <!-- 枚举: report | post -->
    <date>2026-11-20</date>
    <summary>今日聚焦端到端世界模型在长尾场景的推理加速，以及两篇开源多模态规划框架拆解。</summary>
    <source-agent>auto-drive-daily-agent</source-agent>
    <canonical-url>https://arxiv.org/abs/2611.01234</canonical-url>
    <tags>
      <tag>Autonomous Driving</tag>
      <tag>World Model</tag>
    </tags>
    <related-ids>
      <id>V1StGXR8_Z5j</id> <!-- 关联已入库的单篇精读 /p/V1StGXR8_Z5j -->
    </related-ids>
  </meta>

  <body>
    <section title="今日核心动态">
      <callout type="insight" title="核心判断">
        <![CDATA[端到端规划正从纯视觉隐式表征转向**显式 3D 占用预测 + 轻量化语言推理**的混合架构。]]>
      </callout>

      <md><![CDATA[
今日共筛选 42 篇 arXiv 预印本，其中推理延迟降低公式满足 $T_{total} = T_{enc} + \alpha \cdot T_{dec}$。
详细架构对比见下图与下表。
      ]]></md>

      <!-- 本地相对路径图片，aumbry push 时自动打包并重写为 /api/assets/{hash} -->
      <figure src="./imgs/world_model_arch.png" caption="图 1：混合世界模型推理管线对比" />
    </section>

    <section title="重点论文与项目">
      <paper-card
        title="DriveFlow: Real-Time World Modeling for End-to-End Driving"
        authors="Zhang et al. (MIT / Wayve)"
        date="2026-11-19"
        url="https://arxiv.org/abs/2611.01234"
        related-id="V1StGXR8_Z5j">
        <![CDATA[提出流匹配（Flow Matching）替代传统扩散去噪，将多视角视频预测帧率提升至 30 FPS。]]>
      </paper-card>

      <repo-card
        name="open-drive/driveflow"
        stars="2.4k"
        lang="Python / CUDA"
        url="https://github.com/open-drive/driveflow">
        <![CDATA[官方开源实现，包含 nuScenes 预处理脚本与 TensorRT 导出工具。]]>
      </repo-card>
    </section>

    <section title="主流方案横向对比">
      <compare-table caption="2026 Q4 端到端世界模型对比">
        <columns>
          <col>方案名称</col>
          <col>推理帧率</col>
          <col>开源状态</col>
          <col>核心优势</col>
        </columns>
        <row>
          <cell>DriveFlow</cell>
          <cell>30 FPS</cell>
          <cell>已开源</cell>
          <cell>流匹配一步采样，延迟极低</cell>
        </row>
        <row>
          <cell>Vista-v2</cell>
          <cell>8 FPS</cell>
          <cell>仅权重</cell>
          <cell>长时序一致性更佳</cell>
        </row>
      </compare-table>

      <collapse title="展开查看完整实验超参数与附录">
        <md><![CDATA[
- Batch Size: 64 on 8x H100
- Learning Rate: `1e-4` (Cosine Decay)
        ]]></md>
      </collapse>
    </section>

    <sources>
      <source title="DriveFlow 原文 PDF" url="https://arxiv.org/pdf/2611.01234.pdf" />
      <source title="GitHub Repository" url="https://github.com/open-drive/driveflow" />
    </sources>
  </body>
</document>
```

### 6.2 v1 标签与属性约束清单

| 标签              | 层级位置                      | 必填属性 / 子元素                                         | 说明                                                         |
| :---------------- | :---------------------------- | :-------------------------------------------------------- | :----------------------------------------------------------- |
| `<meta>`          | 根下唯一                      | `title`, `type`, `date`, `summary`, `source-agent` 必填   | `type` 仅允许 `report` 或 `post`；`date` 必须符合 `YYYY-MM-DD`。 |
| `<section>`       | `<body>` 下                   | `title` 属性必填                                          | 前端自动提取所有 `<section>` 生成右侧大纲目录（TOC）。       |
| `<md>`            | `<section>` / `<collapse>` 内 | CDATA 文本                                                | 标准 Markdown、表格、代码块及 KaTeX 公式。                   |
| `<figure>`        | `<section>` / `<collapse>` 内 | `src` 必填，`caption` 可选                                | `src` 必须为本地相对路径，且文件真实存在、格式为 `png/jpg/jpeg/webp/svg`。 |
| `<paper-card>`    | `<section>` 内                | `title`, `url` 必填；`authors`, `date`, `related-id` 可选 | 正文 CDATA 为核心亮点；若填 `related-id`，卡片渲染“查看精读”站内跳转按钮。 |
| `<repo-card>`     | `<section>` 内                | `name`, `url` 必填；`stars`, `lang` 可选                  | 开源项目信息卡，正文 CDATA 为项目简评。                      |
| `<compare-table>` | `<section>` 内                | `<columns>` 与至少一个 `<row>`                            | 校验每行 `<cell>` 数量必须与 `<col>` 数量严格一致。          |
| `<callout>`       | `<section>` 内                | `type` (`info`/`warn`/`insight`) 必填，`title` 可选       | 高亮提示框，正文支持 Markdown。                              |
| `<collapse>`      | `<section>` 内                | `title` 必填                                              | 默认折叠面板，内部可嵌套 `<md>` 与 `<figure>`。              |
| `<sources>`       | `<body>` 或 `<section>` 内    | 包含 1 个以上 `<source title="" url="" />`                | 规范参考来源链接块，URL 必须为合法 `http/https`。            |

---

## 7. CLI 工具 (`aumbry`) 详细设计

### 7.1 配置文件（`~/.config/aumbry/config.yaml`）

支持多 Profile，方便不同 Agent 绑定各自的默认写入目录，彻底省去每次手传目录 ID 的麻烦：

```yaml
active_profile: default
profiles:
  default:
    server: "https://aumbry.example.com"
    token: "amb_live_xxxxxxxxxxxx"
    default_dirs:
      - "DirIdPersonalFull"
  paper_share:
    server: "https://aumbry.example.com"
    token: "amb_live_xxxxxxxxxxxx"
    default_dirs:
      - "DirIdPersonalFull"
      - "DirIdPublicPapers"
```

同时支持通过环境变量 `AUMBRY_SERVER`、`AUMBRY_TOKEN`、`AUMBRY_DIRS` 覆盖配置。

### 7.2 核心子命令

1. **`aumbry config set|show|use`**

   * 快速写入服务端地址、Token 及默认写入目录列表。

2. **`aumbry validate <file.xml> [--format json|text]`**

   * 默认输出 JSON（便于 Agent 直接解析并修复），校验通过退出码为 `0`，失败退出码为 `1`。

   * **JSON 输出示例**：

     ```json
     {
       "valid": false,
       "file": "./daily_20261120.xml",
       "errors": [
         {
           "line": 28,
           "code": "MISSING_ASSET",
           "field": "figure.src",
           "message": "local image file not found: ./imgs/world_model_arch.png"
         },
         {
           "line": 45,
           "code": "TABLE_COL_MISMATCH",
           "field": "compare-table.row[1]",
           "message": "expected 4 cells to match columns, got 3"
         }
       ]
     }
     ```

3. **`aumbry push <file.xml> [flags]`**

   * `--profile <name>`：指定配置 Profile。
   * `--dir <dir_id>`：指定挂载目录（可重复多次传参实现一文多挂，覆盖配置中的 `default_dirs`）。
   * `--id <existing_id>`：显式更新指定条目的内容与图片。
   * `--key <external_key>`：基于幂等键执行 **Upsert**（例如 `--key "vibe-coding:2026-11-20"`，当日重复运行自动覆盖更新而非堆积重复报告）。
   * 推送成功后，标准输出返回 JSON（含生成的 `id`、相对 URL `/r/{id}` 或 `/p/{id}`、挂载的目录列表及上传的图片数量）。

---

## 8. 服务端 REST API 设计

### 8.1 写入接口（API Token 鉴权）

* `POST /api/v1/ingest`
  * Header: `Authorization: Bearer <API_TOKEN>`
  * Content-Type: `multipart/form-data`
  * Fields: `xml` (文件), `assets[]` (图片文件列表), `dirs` (目录 ID 列表), `item_id` (可选), `external_key` (可选)。
  * 行为：二次校验 Schema -> 计算图片 SHA-256 并向 WebDAV 幂等上传 -> 重写 XML 图片路径 -> 写入 WebDAV -> 事务更新 SQLite 与 `items_fts`。

### 8.2 访客与目录鉴权接口

* `GET /api/v1/dirs/{id}`：获取目录元数据、子目录列表及挂载条目列表。若目录受保护且无有效会话，返回 `401 Unauthorized`（附带 `{ "locked": true, "dir_name": "..." }` 供前端渲染密码框）。
* `POST /api/v1/dirs/{id}/unlock`：提交 `{ "password": "..." }`，校验通过后在 HttpOnly 加密 Cookie 中追加该目录 ID，返回 `200 OK`。
* `GET /api/v1/dirs/{id}/search?q=...&limit=20&offset=0`：目录子树内 FTS5 搜索（强依赖目录会话）。
* `GET /api/v1/items/{id}`：获取单篇元数据及关联条目摘要（校验子树会话或 Admin，无祖先会话返回 `403`）。
* `GET /api/v1/items/{id}/content`：鉴权后从私有 WebDAV 流式代理返回原始 XML（`Content-Type: application/xml; charset=utf-8`）。
* `GET /api/v1/assets/{hash}`：校验图片所属条目是否在当前会话子树内，通过后从 WebDAV 流式返回图片，并设置 `Cache-Control: private, max-age=86400`。

### 8.3 管理员接口（Admin Session 鉴权）

* `POST /api/v1/admin/login` / `POST /api/v1/admin/logout`
* `GET /api/v1/admin/tree`：获取全站完整目录树（含密码设置状态与条目计数）。
* `POST /api/v1/admin/dirs` / `PATCH /api/v1/admin/dirs/{id}` / `DELETE /api/v1/admin/dirs/{id}`：目录增删改、移动父节点、设置/清空密码。
* `PUT /api/v1/admin/items/{id}/mounts`：批量修改某篇报告的挂载目录集合。
* `DELETE /api/v1/admin/items/{id}`：删除条目并同步清理 `items_fts`。
* `GET /api/v1/admin/search?q=...&type=...&from=...&to=...`：全站无限制 FTS5 搜索与日期筛选。

---

## 9. Web 前端功能与渲染器设计

### 9.1 路由规划

* `/d/:id`：目录视图（若锁定则展示极简密码门禁页；解锁后展示面包屑、子目录卡片、按日期倒序排列的报告列表与目录内搜索框）。
* `/r/:id` 与 `/p/:id`：阅读视图（顶部元信息条：标题、日期、来源 Agent、标签、原文直达按钮；主区域 XML 富文本正文；右侧浮动章节 TOC；底部关联条目跳转区）。
* `/admin`：管理员控制台（左侧全站目录树操作面板，顶部全站搜索框，主区域条目管理与挂载编辑，支持一键复制任意目录的匿名分享链接）。

### 9.2 XML 动态渲染器机制

1. 进入 `/r/:id` 或 `/p/:id` 后，并行请求 `/api/v1/items/{id}`（元数据）与 `/api/v1/items/{id}/content`（XML 文本）。
2. 使用 `new DOMParser().parseFromString(xmlText, 'application/xml')` 解析出 DOM 树。
3. 遍历 `<body>` 子节点，按标签名分发至对应 Vue 组件：
   * `section` -> `SectionBlock.vue`（注册 TOC 锚点）
   * `md` -> `MarkdownBlock.vue`（`markdown-it` + `katex` + `shiki`）
   * `figure` -> `FigureBlock.vue`（`<img>` 直接指向同源 `/api/v1/assets/{hash}`，自动携带 Cookie，点击触发 Lightbox）
   * `paper-card` / `repo-card` / `compare-table` / `callout` / `collapse` / `sources` -> 对应专属 UI 组件。

---

## 10. 安全性、部署与运维规范

### 10.1 安全基线

* **Cookie 安全**：Admin 与 Visitor Session Cookie 均强制开启 `HttpOnly=true`、`SameSite=Lax`，HTTPS 环境下自动开启 `Secure=true`。
* **防 XSS**：`<md>` 块在 `markdown-it` 中默认关闭原生 HTML 解析（`html: false`）；所有 XML 属性文本经 Vue 模板自动转义；`<source>` / `<paper-card>` 的 `url` 属性在 Schema 校验阶段强制限制仅允许 `http://` 与 `https://` 协议（拦截 `javascript:`）。
* **密码保护**：对 `/api/v1/dirs/{id}/unlock` 与 `/api/v1/admin/login` 接口启用内存级 IP 限流（如每分钟最多 10 次失败尝试），防止暴力破解。

### 10.2 环境变量配置清单

| 环境变量                     | 必填 | 说明                                            |
| :--------------------------- | :--- | :---------------------------------------------- |
| `AUMBRY_LISTEN_ADDR`         | 否   | 监听地址，默认 `:8080`                          |
| `AUMBRY_DATA_DIR`            | 是   | SQLite 数据库文件存放目录（挂载 Docker Volume） |
| `AUMBRY_ADMIN_USER`          | 是   | 管理员用户名                                    |
| `AUMBRY_ADMIN_PASSWORD_HASH` | 是   | 管理员密码的 bcrypt 哈希                        |
| `AUMBRY_API_TOKENS`          | 是   | 允许 CLI 推送的 API Token 列表（逗号分隔）      |
| `AUMBRY_COOKIE_SECRET`       | 是   | 32 字节随机密钥，用于加密签名 Session Cookie    |
| `AUMBRY_WEBDAV_URL`          | 是   | 私有 WebDAV 根地址                              |
| `AUMBRY_WEBDAV_USER`         | 是   | WebDAV 用户名                                   |
| `AUMBRY_WEBDAV_PASS`         | 是   | WebDAV 密码                                     |

### 10.3 单容器部署方案

通过多阶段 `Dockerfile`（Node 构建前端 -> Go `embed` 编译单二进制 -> `alpine` 运行时），最终镜像体积可控制在 **30MB 以内**。运维只需持久化挂载 `AUMBRY_DATA_DIR` 中的单个 `.db` 文件，配合现有 WebDAV 即可实现完整容灾。

---

## 11. 迭代路线图（Roadmap）

* **v1.0（MVP 核心闭环）**
  * 完成 Go Monorepo（`pkg/schema` + `cmd/aumbry` + `cmd/server`）。
  * 完成 SQLite + FTS5 两级搜索与 WebDAV 私有鉴权代理。
  * 完成目录树密码、子树 Cookie 会话、一文多挂与管理员控制台。
  * 完成 v1 XML 富文本组件渲染器与 CLI 校验/推送闭环。
* **v2.0（生态与可视化扩展）**
  * **MCP Server**：直接挂载于 Go 服务端的 `/mcp` 端点，复用目录 Token / Session 权限边界；检索列表默认仅返回元数据与摘要，点开或按 ID 查询时再拉取完整正文。
  * **动态可视化组件**：新增 `<chart>`（ECharts）与 `<mermaid>` 流程图组件支持。
  * **外链图片自动转存**：支持 CLI 或服务端自动抓取 XML 中的外部图片链接并转存至私有 WebDAV，防止原站防盗链或死链。
