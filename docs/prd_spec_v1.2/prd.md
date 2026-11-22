# Aumbry 产品需求与技术架构详细设计文档（PRD & Tech Spec v1.2）

| 文档属性     | 内容                                                         |
| :----------- | :----------------------------------------------------------- |
| **项目名称** | **Aumbry**（多 AI Agent 调研报告中枢）                       |
| **文档版本** | v1.2（在 v1.1 基线上的增量规格，未提及的部分一律沿用 v1.1）  |
| **基线**     | `docs/prd_spec_v1.1/prd.md`，对应 master 上已发布的 v0.1.1   |
| **日期**     | 2026-11-22                                                   |
| **本版主题** | **对象存储不再只有 WebDAV**：新增本地目录与 S3 兼容存储（含 AWS S3、Cloudflare R2、MinIO 等） |

---

## 1. 背景与目标

Aumbry 把报告的 XML 与图片存在私有对象存储里，目前只有 WebDAV 一种实现。WebDAV 服务不可用时，整个服务无法启动（启动探测失败），也无法读写任何报告；没有 WebDAV 的用户则根本没法部署。

目标：

* **G1 本地目录后端**：对象直接存在本机目录，零外部依赖，单机部署最简单。
* **G2 S3 兼容后端**：一个实现覆盖 AWS S3、Cloudflare R2、MinIO、Backblaze B2、Wasabi 等所有 S3 协议服务。
* **G3 保留 WebDAV**：现有 WebDAV 后端继续可用，行为不变。
* **G4 后端可选**：由配置选择其中之一，运维命令（`backup`、`verify-storage`、`gc`、`reindex`）与就绪探针对三种后端行为一致。

### 1.1 不做（Out of scope）

* 同时使用多个后端、按条目分后端、后端之间的自动迁移命令（换后端的做法见 7.3）。
* IAM 角色、实例元数据、`AWS_*` 环境变量链等动态凭证；只支持配置里写明的静态密钥。
* 预签名 URL、让浏览器直连存储桶：所有读取仍由 Aumbry 服务端代理，存储桶不需要公开。
* 服务端加密、桶策略、生命周期规则的配置与管理（由使用者在存储服务侧自行设置）。
* GCS、Azure Blob 的原生协议（它们的 S3 兼容入口可以用 S3 后端）。
* 多实例部署：单实例约束不变。
* 本项目没有存量用户，**不做任何向后兼容处理**：与旧版本的配置键、`/readyz` 响应字段的差异见 11.1，直接以本文为准。

---

## 2. 配置（G4）

### 2.1 选择后端

新增配置键 `storage`（环境变量 `AUMBRY_STORAGE`），取值 `local`、`webdav`、`s3`。**必填，没有默认值**：为避免漏配后悄悄指向一个空的新存储，缺失时启动失败并列出全部缺失项（沿用 v1.1 的「一次性列出全部缺失项，同时给出 TOML 键和 env 名」）。

只校验被选中的后端所需的键。未被选中的后端的键如果被设置，`check-config` 与启动日志给出一条警告（`storage = "local" ignores [webdav] settings`），不报错。

### 2.2 键与环境变量

| TOML 键 | 环境变量 | 适用 | 必填 | 默认 | 说明 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `storage` | `AUMBRY_STORAGE` | 全部 | 是 | – | `local` / `webdav` / `s3` |
| `local.path` | `AUMBRY_LOCAL_PATH` | local | 否 | `<data_dir>/objects` | 对象根目录，必须是绝对路径；不存在时自动创建（目录权限 `0700`） |
| `webdav.url` | `AUMBRY_WEBDAV_URL` | webdav | 是 | – | 不变 |
| `webdav.user` | `AUMBRY_WEBDAV_USER` | webdav | 是 | – | 不变 |
| `webdav.password` | `AUMBRY_WEBDAV_PASS` | webdav | 是 | – | 不变 |
| `s3.endpoint` | `AUMBRY_S3_ENDPOINT` | s3 | 否 | AWS 官方端点 | 形如 `https://<account>.r2.cloudflarestorage.com` 或 `http://minio:9000`；协议头决定是否使用 TLS；AWS S3 可省略 |
| `s3.region` | `AUMBRY_S3_REGION` | s3 | 是 | – | AWS 用真实区域；R2 填 `auto`；MinIO 填 `us-east-1` |
| `s3.bucket` | `AUMBRY_S3_BUCKET` | s3 | 是 | – | 存储桶名，必须已存在，Aumbry 不创建桶 |
| `s3.access_key_id` | `AUMBRY_S3_ACCESS_KEY_ID` | s3 | 是 | – | |
| `s3.secret_access_key` | `AUMBRY_S3_SECRET_ACCESS_KEY` | s3 | 是 | – | 属于密钥，`check-config` 掩码，仅保留末 4 位 |
| `s3.prefix` | `AUMBRY_S3_PREFIX` | s3 | 否 | 空 | 桶内的前缀，如 `aumbry/`；Aumbry 只读写该前缀之下的对象，两个实例用不同前缀可以共用一个桶 |
| `s3.path_style` | `AUMBRY_S3_PATH_STYLE` | s3 | 否 | `false` | 使用路径式寻址（`endpoint/bucket/key`）；MinIO 等自建服务通常需要 `true`，R2 与 AWS 用默认即可 |

校验补充：

* `local.path` 不是绝对路径 → 报错；`storage = "local"` 且 `data_dir` 与 `local.path` 都不可用时一并报出。
* `s3.endpoint` 非空时必须是带 `http://` 或 `https://` 的合法 URL，不得含路径、查询或用户信息。
* `s3.prefix` 规范化为「无前导 `/`，非空时以 `/` 结尾」；含 `..` 段 → 报错。
* `check-config` 不连接任何存储，只做配置的加载与校验；密钥一律掩码。
* `config.example.toml` 同时给出三种后端的示例，默认示例是 `storage = "local"`。

### 2.3 示例

```toml
# 本地目录
storage = "local"

# Cloudflare R2
storage = "s3"
[s3]
endpoint = "https://<account-id>.r2.cloudflarestorage.com"
region = "auto"
bucket = "aumbry"
access_key_id = "..."
secret_access_key = "..."

# 自建 MinIO
storage = "s3"
[s3]
endpoint = "http://minio:9000"
region = "us-east-1"
bucket = "aumbry"
path_style = true
access_key_id = "..."
secret_access_key = "..."
```

---

## 3. 存储接口

`internal/storage.Store` 保持现有的六个方法与路径约定（绝对路径、斜杠分隔，如 `/xml/<id>-<hash>.xml`、`/assets/<hash>`），只做两处调整：

* `Put(ctx, name, body io.Reader, size int64) error`：新增已知长度 `size`。所有调用方（`internal/api/ingest.go` 的 XML 与图片）上传的都是内存里的字节切片，长度已知；S3 需要它做单次上传，WebDAV 与本地后端忽略它。
* 所有方法**必须遵守 `ctx` 的取消与截止时间**（含读取响应体阶段）。WebDAV 实现因底层库不支持 `context` 而做不到的部分维持现状，不在本版修复。

错误语义对三种后端一致，沿用 `ErrNotFound`、`ErrAuth`、`ErrTimeout`、`ErrUnreachable`：

* `Open`、`Exists` 对不存在的对象，分别返回 `ErrNotFound` 与 `(false, nil)`；**`Open` 必须在返回时就报告不存在**，不能等到第一次 `Read`（读取条目正文的接口依赖这一点返回 404）。
* `Delete` 对不存在的对象不是错误。
* `List(dir)` 只返回 `dir` 直接下的对象（不递归），返回完整对象路径；对分页服务要取完全部分页；不返回目录占位对象和后端内部的临时对象。
* `Probe` 对 `local` 检查根目录存在且可写，对 `s3` 检查桶可访问，对 `webdav` 沿用现状；2 秒期限。

`/readyz` 响应里的 `checks.webdav` 改名为 `checks.storage`（见 11.1），取值仍是固定的 `ok`/`unreachable`/`timeout`/`auth`，不输出地址、桶名或路径。启动时同样做一次探测并打印脱敏日志。

---

## 4. 本地目录后端（G1）

* 对象 `/xml/a.xml` 存为 `<root>/xml/a.xml`，`<root>` 即 `local.path`。
* **目录越界防护**：对象名必须是规范化的绝对路径且只包含 `/xml/`、`/assets/` 两个顶层目录之一；含 `..`、空段、`\`、NUL 的名字拒绝。文件系统访问使用 Go 标准库的 `os.Root`（项目已在 CLI 里使用），因此指向根目录之外的符号链接不会被跟随。
* **原子写入**：先写入同目录下的临时文件（以 `.tmp-` 开头），`fsync` 后 `rename` 到最终名，再 `fsync` 所在目录；写入中途失败或进程被杀，最终名要么不存在、要么是完整内容，绝不出现半个文件。`List` 不返回 `.tmp-*`；`gc` 可以清理超过 1 小时的遗留 `.tmp-*`。
* 同名对象的内容寻址路径已经保证相同名字内容相同，覆盖写入安全。
* 文件权限 `0600`，目录 `0700`。容器镜像以 UID/GID `65532` 运行，卷需要对该用户可写。
* 磁盘已满、权限不足等 I/O 错误按 5xx 返回并写日志，不吞掉。
* 对象文件名里不嵌入原始路径以外的信息，不引入额外元数据文件。

---

## 5. S3 兼容后端（G2）

* **客户端库**：使用成熟的开源 S3 客户端，不手写签名与协议。候选：`minio-go`（轻量，对各类 S3 兼容服务宽容）与 `aws-sdk-go-v2`。实现前必须读取该库当前版本的文档与源码核对三点，并把结论写进提交说明：① 能用 `CGO_ENABLED=0` 构建；② 与 R2 兼容（`aws-sdk-go-v2` 的新版本默认会为上传计算 CRC32 校验和，部分 S3 兼容服务不支持，用它就必须把校验和策略设为「仅在必要时」，见 [AWS 文档](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/s3-checksums.html)）；③ 维护活跃、许可证允许。
* **对象键**：`<prefix>` + 对象名去掉前导 `/`，例如前缀 `aumbry/` 下 `/xml/a.xml` 的键是 `aumbry/xml/a.xml`。
* **读写**：`Put` 用已知长度的单次上传；`Open` 先确认对象存在（得到 `ErrNotFound` 的时机见第 3 章）再流式返回响应体；`Exists` 用对象元数据查询；`Delete` 用单对象删除。
* **`List`**：按前缀加分隔符 `/` 列举并取完所有分页；只返回 Aumbry 自己前缀之下、`dir` 直接下的对象。多个前缀共用一个桶时互不可见。
* **超时**：拨号 5 秒、响应头 30 秒，大对象流式传输不设整体超时；全部请求受 `ctx` 约束。上传的是内容寻址的对象，可重复写入而无副作用，因此允许客户端库内置的有限次重试，但总时间受 `ctx` 限制。
* **错误映射**：认证失败（访问密钥错误、签名不匹配、被拒绝）→ `ErrAuth`；桶不存在或网络不可达 → `ErrUnreachable`；超时 → `ErrTimeout`。错误文本写入日志前去掉密钥与签名。
* **TLS 与地址**：`https://` 端点用系统根证书校验；`http://` 允许，用于内网与本地自建服务。
* **R2 说明**（写进文档，不做实测）：端点是账户专属的 `https://<account-id>.r2.cloudflarestorage.com`，区域填 `auto`；API 令牌需要对该桶的读写权限；Aumbry 用到的操作（单对象读写删、按前缀列举、元数据查询）都在 R2 的 S3 兼容范围内，但本版没有在真实 R2 上验证，以 [Cloudflare 的 S3 兼容文档](https://developers.cloudflare.com/r2/api/s3/api/)为准。

---

## 6. 运维命令（G4）

* `backup`：仍只备份 SQLite 快照，行为不变。文档按后端说明如何做配对备份：`local` 在数据库快照**之后**复制对象目录（对象内容寻址且不可变，晚复制得到的是超集）；`s3` 靠存储服务自身的版本控制、复制或 `rclone`；`webdav` 同现状。
* `verify-storage`、`gc`、`reindex`：通过 `Store` 访问，对三种后端行为一致；`gc` 与 `reindex` 的离线锁、`--dry-run` 语义不变。
* `gc` 清理 `local` 后端遗留的 `.tmp-*`（见第 4 章）。
* 恢复后验证（`verify-storage`，再打开一篇含图报告）的要求不变。

---

## 7. 文档与示例

### 7.1 文档（中英双语，默认英文，按 technical-writing 写，用 deai 检查）

* `docs/{en,zh-CN}/configuration.md` 与 `deployment.md`：新增「选择存储后端」一节，分别写 `local`、`webdav`、`s3`（含 R2、MinIO 的完整示例）的配置、需要的权限、容器部署时的卷与 UID `65532` 权限、各后端的备份做法。
* `README.md`、`README.zh-CN.md`：把「需要一个可访问的 WebDAV 服务」改为「选择一种存储后端」，快速上手以 `local` 为例。
* `config.example.toml`、`.env.example`、`docker-compose.yml`、`docker-compose.toml.yml`、`examples/deploy/*`：默认示例改成 `local`（对象放在已有的 `/data` 卷里），WebDAV 与 S3 作为备选写在注释或独立示例里；反向代理示例里的 `AUMBRY_STORAGE` 等必需项补齐。
* `skills/aumbry` 里提到 WebDAV 的地方改为存储后端的通用说法（先 `rg` 确认有没有）。

### 7.2 就绪探针与日志

见第 3 章。文档里的 `/readyz` 示例响应同步改成 `checks.storage`。

### 7.3 更换后端

不提供迁移命令。更换后端时：停服务，用 `rclone` 把旧后端的 `xml/` 与 `assets/` 两个目录复制到新后端（本地目录、S3 桶或前缀），改配置，启动前运行 `verify-storage`。写进部署文档的「更换存储后端」一节。

---

## 8. 非功能

* `CGO_ENABLED=0` 不变；新依赖只引入 S3 客户端一个库。
* 单实例约束不变。
* 日志与 `check-config` 不输出密钥；`readyz` 不输出地址、桶名、路径。
* 沿用「有成熟开源库就用库」：S3 协议用库，本地文件访问用标准库 `os.Root`，不手写任何协议或签名。

---

## 9. 里程碑

| 里程碑 | 内容 | 依赖 |
| :--- | :--- | :--- |
| **M1** | `Store` 接口调整（`Put` 加 `size`、`ctx` 约束）；`storage` 配置键与校验；本地目录后端；`/readyz` 改名；启动探测；运维命令适配；`config.example.toml`、compose、README 的 `local` 示例；文档的 `local` 与 `webdav` 部分 | – |
| **M2** | S3 兼容后端；三种后端共用的存储一致性测试；S3 的文档（含 R2、MinIO）；`gc` 清理 `.tmp-*` | M1 |

---

## 10. 验收标准

以下验收不涉及 docker 与真实浏览器。S3 的一致性测试使用进程内的 S3 兼容测试服务（实现时选定成熟的开源测试库；若找不到，改为用本机的 `rclone serve s3` 并在测试里按环境变量跳过，说明原因）。

**G1 本地目录**
1. 仅设置 `AUMBRY_STORAGE=local` 与 v1.1 已有的必填项，服务启动；推送一篇含图片的报告后，正文与图片都能读回；文件分别落在 `<data_dir>/objects/xml/` 与 `objects/assets/`，文件权限 `0600`、目录 `0700`。
2. 越界防护：对象名 `/xml/../../x`、含 NUL、含 `\` 的写入与读取全部被拒；根目录内指向根目录外的符号链接不被跟随（单元测试）。
3. 原子写入：在写入中途注入失败，最终名下不出现不完整文件；`List` 不返回 `.tmp-*`；`gc` 清理超过 1 小时的遗留临时文件。
4. `local.path` 为相对路径 → 配置校验失败并指出键名；根目录不可写 → 启动失败并给出脱敏日志，`/readyz` 返回 503。

**G2 S3 兼容**
5. 共用一致性测试（`Put`/`Open`/`Exists`/`Delete`/`List`/`Probe`）对 `local`、`s3`（进程内服务）全部通过，WebDAV 沿用现有测试：覆盖重复写入同名对象、`Delete` 不存在的对象、`List` 的非递归语义、超过 1000 个对象时的分页、`Open` 不存在的对象立即返回 `ErrNotFound`。
6. S3 端到端：推送含图片的报告、更新、删除；`verify-storage` 退出码 0；人为删除一个图片对象后 `verify-storage` 列出缺失项且退出码非 0；`gc --dry-run` 列出孤儿对象；`reindex` 正常。
7. 前缀隔离：同一个桶、两个不同前缀，两个实例的 `List`、`gc`、`verify-storage` 互不可见。
8. 超时与取消：测试服务延迟响应超过 30 秒时，`POST /api/v1/ingest` 以 5xx 返回而不是一直阻塞；`ctx` 被取消时 `Put`、`Open`、`List` 立即返回。
9. 错误映射：错误的访问密钥 → `ErrAuth`，`/readyz` 的 `checks.storage` 为 `auth`；端点不可达 → `unreachable`；日志与响应里都不出现访问密钥、签名或端点地址。

**G4 配置与运维**
10. 缺少 `storage` → 启动失败；缺少所选后端的必填项 → 一次性列出全部缺失项，同时给出 TOML 键和 env 名；未选中的后端的键被设置 → 一条警告，不报错。
11. `check-config` 对 `s3.secret_access_key` 与 `s3.access_key_id` 掩码，仅保留末 4 位；`s3.endpoint` 带路径或用户信息 → 校验失败；`s3.prefix` 含 `..` → 校验失败。
12. `/readyz` 在三种后端下都返回 `checks.storage`，正常为 `ok`；不再出现 `checks.webdav`；响应不含地址、桶名或路径。
13. `CGO_ENABLED=0 go build ./...`、`go vet`、`go test`、`golangci-lint`（CI 同版本）全部通过。

**文档**
14. 中英文的 `configuration.md`、`deployment.md`、README 都描述三种后端；R2 与 MinIO 有完整示例；R2 示例标明「依据官方文档核对，未在真实 R2 上验证」；有「更换存储后端」一节；`config.example.toml` 通过 `check-config`（退出码 0）。
15. 文档用 deai `--mode check` 检查（中英分别），`markdown-it` 渲染后没有残留的 `**`。

---

## 11. 决策记录与未决问题

### 11.1 已定决策

* **R2 不单设类型**：R2 是 S3 兼容服务，用 `storage = "s3"` 加端点与 `region = "auto"` 配置，文档里单独给出示例。
* **`storage` 必填**：宁可让漏配的人启动失败，也不要悄悄指向一个空的新存储。
* **只支持静态密钥**：不引入凭证链，减少依赖与行为差异。
* **`checks.webdav` 改名为 `checks.storage`**：探针不再只检查 WebDAV；项目没有存量用户，不保留旧字段。
* **WebDAV 的 `[webdav]` 表与 `AUMBRY_WEBDAV_*` 名字不变**：它们不是兼容层，只是现有后端的正常配置。
* **不提供后端迁移命令**：用 `rclone` 复制两个目录即可，见 7.3。

### 11.2 未决问题（实现前由使用者确认）

1. 配置键名：`storage` 加 `[local]`/`[s3]` 两张表与 `AUMBRY_S3_*` 环境变量，这个命名是否可以。
2. S3 客户端库在 `minio-go` 与 `aws-sdk-go-v2` 之间的选择，由实现者按第 5 章的三点核对后决定，并说明理由。
3. 是否需要 `type = "r2"` 这样的便捷别名（自动由账户 ID 拼出端点）。当前决定不要。
4. 是否需要支持 AWS 的 IAM 角色等动态凭证。当前决定不要。
