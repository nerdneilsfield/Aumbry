# Aumbry 产品需求与技术架构详细设计文档（PRD & Tech Spec v1.2）

| 文档属性     | 内容                                                         |
| :----------- | :----------------------------------------------------------- |
| **项目名称** | **Aumbry**（多 AI Agent 调研报告中枢）                       |
| **文档版本** | v1.2（在 v1.1 基线上的增量规格，未提及的部分一律沿用 v1.1）  |
| **基线**     | `docs/prd_spec_v1.1/prd.md`，对应 master 上已发布的 v0.1.1   |
| **日期**     | 2026-11-22                                                   |
| **本版主题** | **对象存储不再只有 WebDAV**：新增本地目录与 S3 兼容存储（含 Cloudflare R2、MinIO、AWS S3 等） |

---

## 1. 背景与目标

Aumbry 把报告的 XML 与图片存在私有对象存储里，目前只有 WebDAV 一种实现。WebDAV 服务挂掉时，所有读写都失败（`/readyz` 返回 503），没有 WebDAV 的人则无法部署。

目标：新增 **本地目录**（`local`）与 **S3 兼容**（`s3`，另有 R2 便捷类型 `r2`）两种后端；WebDAV（`webdav`）保持可用；由配置选择其一。

**这是个小需求，只做三件事：选后端、两个新后端、让现有运维命令与就绪探针适配。** 不做：

* 同时使用多个后端；后端之间的迁移命令（做法见 7.2）。
* IAM 角色、`AWS_*` 环境变量链等动态凭证；只支持配置里的静态密钥。
* 预签名 URL、让浏览器直连存储桶：读取仍由 Aumbry 代理，桶不需要公开。
* 同一个桶/目录由多套数据库共用：**每套 Aumbry 数据库独占一个桶或一个目录**（否则对其中一套运行 `gc` 会把另一套的有效对象当孤儿删除）。因此没有 `prefix` 配置。
* 服务端加密、桶策略、生命周期规则（在存储服务侧自行设置）。
* 多实例部署；单实例约束不变。
* 本项目没有存量用户，**不做任何向后兼容处理**，以本文为准。

---

## 2. 配置

### 2.1 选择后端

配置键 `storage`（环境变量 `AUMBRY_STORAGE`），取值 `local`（**默认**）、`webdav`、`s3`、`r2`。`r2` 是 `s3` 的便捷写法：由账户 ID 拼出端点、区域固定为 `auto`，之后与 `s3` 是同一份实现。

只校验被选中的后端所需的键（沿用 v1.1：缺失项一次性全部列出，同时给出 TOML 键和 env 名）。**防误配**：`storage` 未设置、却填写了 `webdav`、`s3` 或 `r2` 的任何配置项时，启动失败并提示设置 `storage`（避免悄悄用空的本地目录）。v1.1 中 `webdav.*` 不再是全局必填，只在 `storage = "webdav"` 时必填。

### 2.2 键与环境变量

| TOML 键 | 环境变量 | 后端 | 必填 | 说明 |
| :--- | :--- | :--- | :--- | :--- |
| `storage` | `AUMBRY_STORAGE` | 全部 | 否（默认 `local`） | `local` / `webdav` / `s3` / `r2` |
| `local.path` | `AUMBRY_LOCAL_PATH` | local | 否 | 对象根目录，默认 `<data_dir>/objects`；允许相对路径，与 `data_dir` 一样相对进程工作目录解析 |
| `webdav.url` / `user` / `password` | `AUMBRY_WEBDAV_URL` / `_USER` / `_PASS` | webdav | 是 | 不变 |
| `s3.endpoint` | `AUMBRY_S3_ENDPOINT` | s3 | 否 | 形如 `http://minio:9000`；空表示 AWS 官方端点；协议头决定是否使用 TLS |
| `s3.region` | `AUMBRY_S3_REGION` | s3 | 端点为空时必填 | 端点非空且未设置时默认 `auto`（R2）；MinIO 填 `us-east-1` |
| `s3.bucket` | `AUMBRY_S3_BUCKET` | s3 | 是 | 必须已存在，Aumbry 不创建桶 |
| `s3.access_key_id` / `s3.secret_access_key` | `AUMBRY_S3_ACCESS_KEY_ID` / `AUMBRY_S3_SECRET_ACCESS_KEY` | s3 | 是 | `check-config` 对两者都掩码，仅保留末 4 位 |
| `s3.path_style` | `AUMBRY_S3_PATH_STYLE` | s3 | 否 | 默认 `false`；MinIO 等自建服务通常要 `true` |
| `r2.account_id` | `AUMBRY_R2_ACCOUNT_ID` | r2 | 是 | 32 位十六进制；端点为 `https://<account_id>.r2.cloudflarestorage.com` |
| `r2.bucket` / `access_key_id` / `secret_access_key` | `AUMBRY_R2_BUCKET` / `_ACCESS_KEY_ID` / `_SECRET_ACCESS_KEY` | r2 | 是 | 含义同 `s3`，掩码规则相同 |

校验补充：`s3.endpoint` 非空时必须是带 `http://` 或 `https://` 的合法 URL，不含路径、查询或用户信息；R2 欧盟等司法辖区端点（`<account>.eu.r2.cloudflarestorage.com`）用 `storage = "s3"` 手写端点。`check-config` 只加载与校验配置，不连接存储。

### 2.3 示例（每个代码块独立可解析）

```toml
# 本地目录（默认，可省略）
storage = "local"
```

```toml
# Cloudflare R2
storage = "r2"
[r2]
account_id = "<32 位账户 ID>"
bucket = "aumbry"
access_key_id = "..."
secret_access_key = "..."
```

```toml
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

`internal/storage.Store` 保持六个方法与路径约定（绝对路径、斜杠分隔，如 `/xml/<id>-<hash>.xml`、`/assets/<hash>.<ext>`），调整三处：

* `Put(ctx, name, body io.ReadSeeker) error`：把 `io.Reader` 改为 `io.ReadSeeker`。现有两个调用点（`internal/api/ingest.go` 的 XML 与图片）传的都是 `bytes.NewReader`，这样 S3 能得到长度并安全重试。
* 方法在可行范围内遵守 `ctx` 的取消与截止时间；S3 的请求与响应体读取都要传 `ctx`。WebDAV 底层库不支持 `context` 的部分维持现状。
* 错误语义对新后端与 WebDAV 一致：`Open` 对不存在的对象返回 `ErrNotFound`，**且必须在返回流之前就报告**（不能等到第一次 `Read`；正文与图片接口依赖它返回 404）；`Exists` 对不存在返回 `(false, nil)`；`Delete` 对不存在不是错误；`List(dir)` 只返回 `dir` 直接下的对象（不递归），对分页服务取完所有分页，目录不存在时返回空列表且无错误，不返回目录占位对象和临时文件；`Probe` 把失败归类为 `ErrAuth`、`ErrTimeout`、`ErrUnreachable` 之一。

**就绪与启动**（沿用 v1.1 10.1）：`/readyz` 的 `checks.webdav` 改名为 `checks.storage`，取值仍是固定的 `ok`/`unreachable`/`timeout`/`auth`，响应不含地址、桶名、路径；探测 2 秒期限、5 秒缓存不变。启动时探测失败**只打警告、继续监听**（与现状一致）；配置本身无效则启动失败。服务端日志不得出现访问密钥与签名。

---

## 4. 本地目录后端

* 对象 `/xml/a.xml` 存为 `<root>/xml/a.xml`；`<root>` 即 `local.path`。**只有服务启动会自动创建根目录**（权限 `0700`）；`verify-storage`、`gc`、`reindex`（含 `--dry-run`）遇到根目录不存在要报错，不创建，避免路径拼错后对着空目录报「一切正常」。
* **根目录打不开**（路径不可访问、权限不足、挂载缺失）属于配置错误：服务启动直接失败并给出原因，不进入「监听但 `/readyz` 为 503」的状态。
* **越界防护**：对象名必须是规范化的绝对路径，且在 `/xml/`、`/assets/` 之下；含 `..`、空段、`\`、NUL 的名字拒绝。文件访问使用 Go 标准库 `os.Root`，指向根目录之外的符号链接不会被跟随。
* **原子写入**：写入同目录下以 `.tmp-` 开头的临时文件，`fsync` 文件、关闭、`rename` 到最终名；写入失败时删除临时文件。`List` 不返回 `.tmp-*`。不做目录 `fsync`（Windows 不支持），也不做遗留临时文件的自动回收（进程被强杀可能留下，停服后手工删除即可）。
* 文件权限 `0600`，目录 `0700`。容器镜像以 UID/GID `65532` 运行，挂载的卷要对该用户可写。
* `Probe`：根目录存在且可写（创建并删除一个临时文件）。

---

## 5. S3 兼容后端

* **客户端库**：用成熟的开源 S3 客户端，不手写签名与协议。候选 `minio-go` 与 `aws-sdk-go-v2`；实现者读取所选库当前版本的文档与源码，确认三点并写进最终报告：① 能 `CGO_ENABLED=0` 构建；② 与 R2 兼容（`aws-sdk-go-v2` 新版本默认为上传计算 CRC32 校验和，部分 S3 兼容服务不支持，用它就要把校验和策略设为「仅在必要时」，见 [AWS 文档](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/s3-checksums.html)）；③ 维护活跃、许可证允许。
* **对象键**：对象名去掉前导 `/`，例如 `/xml/a.xml` 对应键 `xml/a.xml`。
* **`Open`**：直接发一次流式 GET，在返回流之前判断错误：`NoSuchKey`/404 → `ErrNotFound`。不要用「先 HEAD 再 GET」，也不要用会延迟请求的高层接口（`minio-go` 的 `Client.GetObject` 会延迟到第一次读取，需用 `Core.GetObject` 或等价的立即请求）。
* **权限**：AWS 上缺少 `s3:ListBucket` 时，对不存在的对象 GET/HEAD 返回 403 而非 404；文档要写明所需权限（读、写、删、列举），并把权限错误映射为 `ErrAuth`，只有确认对象缺失才映射为 `ErrNotFound`。
* `Put` 用已知长度的单次上传；`Exists` 用元数据查询；`Delete` 用单对象删除；`List` 按前缀 `xml/`、`assets/` 与分隔符 `/` 列举并取完所有分页。
* **超时**：沿用现有的拨号 5 秒、响应头 30 秒，大对象流式传输不设整体超时；允许 SDK 内置的有限次重试（内容寻址的对象可重复写入）。错误映射：认证失败 → `ErrAuth`；超时 → `ErrTimeout`；桶不存在、网络不可达 → `ErrUnreachable`。
* **R2**（文档里写，不做实测）：`storage = "r2"` 由账户 ID 拼出端点、区域 `auto`；API 令牌需要对该桶的读写权限；Aumbry 用到的操作都在 R2 的 S3 兼容范围内，但本版没有在真实 R2 上验证，以 [Cloudflare 文档](https://developers.cloudflare.com/r2/api/s3/api/)为准。

---

## 6. 运维命令

`verify-storage`、`gc`、`reindex` 都通过 `Store` 访问，对各后端行为一致，离线锁与 `--dry-run` 语义不变。

**备份（需要写进文档，纠正 v1.1 的说法）**：`backup` 仍只备份 SQLite 快照。数据库快照之后更新条目会删除被替换的旧 XML，所以**不能**「先快照、再复制对象目录」。完整备份的做法：停止服务（或停止写入并暂停 `gc`），保存 SQLite 快照以及对象存储的对应内容（`local` 复制目录，`s3`/`webdav` 用存储服务自带的复制或 `rclone`），恢复后运行 `verify-storage`，再打开一篇含图报告确认。

---

## 7. 文档与示例（只改必须改的）

### 7.1 要改的文件

* `config.example.toml`、`.env.example`、`examples/deploy/*/.env.example`：默认示例为 `local`，WebDAV 相关项改为备选并注明需要 `AUMBRY_STORAGE=webdav`。`docker-compose*.yml` 已经挂载 `/data` 卷，默认 `local` 下不需要改动。
* `docs/{en,zh-CN}/configuration.md`：配置表与每种后端的示例（含 R2 与 MinIO）、S3 所需权限；`docs/{en,zh-CN}/deployment.md`：`/readyz` 示例改为 `checks.storage`、第 6 章的备份说明、`local` 在容器里的卷权限（UID `65532`）、更换后端的一句话（见 7.2）；`docs/{en,zh-CN}/cli.md` 与 `README.md`、`README.zh-CN.md`、`skills/aumbry/references/troubleshooting.md` 里提到 WebDAV 的地方改为存储后端的通用说法（先用 `rg` 限定在这些路径下确认）。
* 中英文档内容一致；R2 示例标明「依据官方文档核对，未在真实 R2 上验证」。

### 7.2 更换后端

不提供迁移命令。停服务，用 `rclone` 把旧后端的 `xml/` 与 `assets/` 复制到新后端，改配置，先运行 `verify-storage` 再启动。

---

## 8. 验收标准（不涉及 docker 与真实浏览器）

S3 的测试使用进程内的 S3 兼容测试服务（实现时选定一个成熟的开源 Go 测试库，如 `gofakes3`；说明选择理由），`go test ./...` 默认不依赖外部服务。

1. **local 端到端**：仅默认配置（不设 `storage`）启动，推送含图片的报告后正文与图片可读回；对象落在 `<data_dir>/objects/xml/` 与 `objects/assets/`；文件 `0600`、目录 `0700`。
2. **local 安全**：对象名 `/xml/../../x`、含 NUL、含 `\` 的读写被拒；根目录内指向根目录外的符号链接不被跟随；写入中途失败不留下不完整的最终文件；`List` 不返回 `.tmp-*`；`verify-storage`/`gc --dry-run` 对不存在的根目录报错且不创建它。
3. **共用一致性测试**：对 `local` 与进程内 `s3`（WebDAV 沿用现有测试）覆盖 `Put`（含覆盖同名）、`Open`、`Exists`、`Delete`（不存在不报错）、`List`（非递归；用小页长构造出分页，如每页 2 个、放 5 个对象；不存在的目录返回空）、`Probe`，以及 `Open` 不存在时立即返回 `ErrNotFound`。
4. **s3 端到端**：推送含图报告、更新、删除；`verify-storage` 退出码 0；删掉一个图片对象后它被列出且退出码非 0；`gc --dry-run` 列出孤儿；`reindex` 正常。
5. **s3 超时、取消与错误映射**：用短超时注入测试（沿用 `internal/storage/storage_test.go` 与 `internal/api/middleware_test.go` 的方式）验证慢响应导致 ingest 以 5xx 返回，并断言默认常量仍是 5 秒/30 秒；`ctx` 取消后请求返回；访问密钥错误 → `/readyz` 的 `checks.storage` 为 `auth`，端点不可达 → `unreachable`；`/readyz` 响应与日志中不含访问密钥或签名，响应中不含地址、桶名、路径。
6. **配置与就绪**：`storage` 未设置且设置了 `webdav`/`s3`/`r2` 任一项 → 启动失败并提示；所选后端缺少必填项 → 一次性列出全部缺失项（含 TOML 键与 env 名）；`check-config` 对 `access_key_id` 与 `secret_access_key`（`s3` 与 `r2`）掩码到末 4 位；`r2.account_id` 非 32 位十六进制 → 失败；`storage = "r2"` 解析出的端点与区域由单元测试直接断言，不连网；`s3.endpoint` 带路径 → 失败；`config.example.toml` 通过 `check-config`（退出码 0）；`/readyz` 在各后端下都输出 `checks.storage`，不再有 `checks.webdav`。
7. **工程检查**：`CGO_ENABLED=0 go build ./...`、`go vet ./...`、`go test ./...`、`golangci-lint`（v2.15.0，与 CI 同版本）、`gofumpt`、`goimports` 全部通过；本需求不动前端，不跑前端检查。

---

## 9. 决策记录

* **`storage` 默认 `local`**，但「没设 `storage` 却配了其它后端」会启动失败，避免漏配后悄悄使用空目录。
* **`r2` 作为便捷类型保留**（使用者要求）：只填账户 ID，内部与 `s3` 同一份实现；欧盟等端点与其他 S3 兼容服务用 `s3`。
* **没有 `prefix`**：每套数据库独占桶或目录，避免 `gc` 跨库误删。
* **`Put` 改 `io.ReadSeeker`**，不新增 `size` 参数。
* **不做 `.tmp-*` 自动回收，也不做目录 `fsync`**。
* **`checks.webdav` 改名为 `checks.storage`**；`[webdav]` 表与 `AUMBRY_WEBDAV_*` 名字不变。
* **S3 客户端库**在 `minio-go` 与 `aws-sdk-go-v2` 之间由实现者按第 5 章三点核对后选定。
* **里程碑**：一次完成，不分阶段。
