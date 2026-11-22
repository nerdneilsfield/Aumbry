[English](../en/configuration.md) | 简体中文

# 服务端配置参考

`aumbry-server` 与维护子命令（`check-config`、`backup`、`verify-storage`、`gc`、`reindex`）共用同一套严格配置加载规则，支持 TOML 文件、环境变量以及两者混合配置。

## 查找顺序与覆盖规则

加载器在应用环境变量覆盖前，按以下顺序定位至多一个 TOML 文件：

1. 命令行 `--config <path>`（显式指定时文件必须存在，缺失则启动失败）。
2. `AUMBRY_CONFIG` 环境变量（显式指定时文件必须存在，缺失则启动失败）。
3. 当前工作目录下的 `./aumbry.toml`（可选）。
4. `/etc/aumbry/config.toml`（可选）。
5. 以上均未命中时进入纯环境变量模式。

配置项同时出现在多处时，优先级严格按以下顺序生效：

```text
环境变量  >  TOML 配置文件  >  内置默认值
```

加载器行为约束：

- **严格 TOML 字段检查**：出现未定义 TOML 键名时启动直接报错（`unknown configuration key(s) ...`）。
- **一次性报告错误**：所有缺失必填项或格式错误合并收集后一次性输出。
- **防误配检查**：未设置 `storage`（`AUMBRY_STORAGE`）却填写了 `webdav`、`s3` 或 `r2` 任一配置项时，启动失败并提示设置 `storage`，避免在漏设后端时直接使用空的本地目录。
- **按所选后端校验**：只校验当前选中后端所需的字段。
- **首尾空白裁剪**：除 `AUMBRY_COOKIE_SECRET` 与 `AUMBRY_WEBDAV_PASS` 保持原样外，其余所有配置值及逗号分隔项均先裁剪首尾空白再校验。
- **文件权限告警**：当已读取的 TOML 文件对同组或其他用户可读时，服务端输出提示告警（建议 `chmod 600`）。

## 配置项清单

### 通用配置

| TOML 键 | 环境变量 | 类型 | 必填 | 默认值 | 说明与约束 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `listen_addr` | `AUMBRY_LISTEN_ADDR` | 字符串 | 是（有默认值） | `":8080"` | HTTP 监听地址（`host:port` 或 `:port`）。裁剪空白后为空时回退至 `":8080"`。 |
| `data_dir` | `AUMBRY_DATA_DIR` | 字符串 | 是 | 无 | 存放 `aumbry.db` 与 `aumbry.lock` 的目录。首次启动以 `0700` 创建；必须可写。 |
| `storage` | `AUMBRY_STORAGE` | 字符串 | 否 | `"local"` | 对象存储后端：`local`、`webdav`、`s3` 或 `r2`。 |
| `admin.user` | `AUMBRY_ADMIN_USER` | 字符串 | 是 | 无 | 登录 `POST /api/v1/admin/login` 使用的管理员用户名，裁剪空白后非空。 |
| `admin.password_hash` | `AUMBRY_ADMIN_PASSWORD_HASH` | 字符串 | 是 | 无 | 合法 bcrypt 哈希（支持 `$2$`、`$2a$`、`$2b$`、`$2x$`、`$2y$`），校验开销、分隔符与 53 字符盐+摘要 base64 编码。 |
| `api_tokens` | `AUMBRY_API_TOKENS` | 字符串列表（TOML 数组或逗号分隔环境变量） | 是 | 无 | 允许调用 `/api/v1/ingest` 与 `/api/v1/agent/*` 的 Bearer token 列表，至少 1 个非空项；空项自动过滤。 |
| `cookie_secret` | `AUMBRY_COOKIE_SECRET` | 字符串 | 是 | 无 | 用于签名和加密会话 Cookie 的根密钥。最少 `32` 字节；更换后所有既有访客与管理员会话立即失效。 |
| `trusted_proxies` | `AUMBRY_TRUSTED_PROXIES` | 字符串列表（TOML 数组或逗号分隔环境变量） | 否 | `[]` | 允许解析 `X-Forwarded-For` 与 `X-Forwarded-Proto` 的反向代理 IP 或 CIDR 列表（如 `10.0.0.0/8`）。非法地址会导致启动报错。 |

### 对象存储后端配置

| TOML 键 | 环境变量 | 后端 | 必填 | 默认值与约束 |
| :--- | :--- | :--- | :--- | :--- |
| `local.path` | `AUMBRY_LOCAL_PATH` | `local` | 否 | 对象根目录，默认 `<data_dir>/objects`；允许相对路径，按进程工作目录解析。 |
| `webdav.url` | `AUMBRY_WEBDAV_URL` | `webdav` | 是 | 私有 WebDAV 根目录绝对 URL，必须包含协议（如 `https://`）与主机名。 |
| `webdav.user` | `AUMBRY_WEBDAV_USER` | `webdav` | 是 | WebDAV HTTP Basic 认证用户名，裁剪空白后非空。 |
| `webdav.password` | `AUMBRY_WEBDAV_PASS` | `webdav` | 是 | WebDAV HTTP Basic 认证密码；环境变量 `AUMBRY_WEBDAV_PASS` 原样读取，不裁剪空白。 |
| `s3.endpoint` | `AUMBRY_S3_ENDPOINT` | `s3` | 否 | 可选 `http://` 或 `https://` 端点（如 `http://minio:9000`），不含路径、查询或用户信息；留空表示 AWS 官方端点。 |
| `s3.region` | `AUMBRY_S3_REGION` | `s3` | 端点为空时必填 | 端点非空且留空时默认 `auto`（适合 R2）；自建 MinIO 可填 `us-east-1`。 |
| `s3.bucket` | `AUMBRY_S3_BUCKET` | `s3` | 是 | 已存在且专供当前数据库使用的存储桶；Aumbry 不会自动创建桶。 |
| `s3.access_key_id` | `AUMBRY_S3_ACCESS_KEY_ID` | `s3` | 是 | 静态 Access Key ID（`check-config` 会掩码保留末 4 位）。 |
| `s3.secret_access_key` | `AUMBRY_S3_SECRET_ACCESS_KEY` | `s3` | 是 | 静态 Secret Access Key（`check-config` 会掩码保留末 4 位）。 |
| `s3.path_style` | `AUMBRY_S3_PATH_STYLE` | `s3` | 否 | 默认 `false`；MinIO 等自建服务通常需要设为 `true`。 |
| `r2.account_id` | `AUMBRY_R2_ACCOUNT_ID` | `r2` | 是 | 32 位十六进制 Cloudflare 账户 ID，自动推导端点 `https://<account_id>.r2.cloudflarestorage.com` 与区域 `auto`。若使用欧盟等司法辖区端点（`.eu.r2.cloudflarestorage.com`），请改用 `storage = "s3"` 并填写 `s3.endpoint`。 |
| `r2.bucket` | `AUMBRY_R2_BUCKET` | `r2` | 是 | 已存在且专供当前数据库使用的 R2 存储桶。 |
| `r2.access_key_id` | `AUMBRY_R2_ACCESS_KEY_ID` | `r2` | 是 | R2 S3 API Access Key ID（`check-config` 会掩码保留末 4 位）。 |
| `r2.secret_access_key` | `AUMBRY_R2_SECRET_ACCESS_KEY` | `r2` | 是 | R2 S3 API Secret Access Key（`check-config` 会掩码保留末 4 位）。 |

## 存储后端配置示例与权限要求

每套 Aumbry 数据库必须独占一个目录或一个存储桶；不要让两套数据库共用同一存储桶或目录，否则在一套数据库上执行 `gc` 会把另一套的有效对象当作孤儿删除。Aumbry 只使用配置中的静态凭证，不读取 IAM 角色或 `AWS_*` 环境变量链。

**本地目录（默认）：**

```toml
storage = "local"

[local]
path = "/data/objects" # 可选，默认为 <data_dir>/objects
```

**Cloudflare R2** *（依据官方文档核对，未在真实 R2 上验证）*：

```toml
storage = "r2"

[r2]
account_id = "0123456789abcdef0123456789abcdef"
bucket = "aumbry"
access_key_id = "..."
secret_access_key = "..."
```

**自建 MinIO（`s3`）：**

```toml
storage = "s3"

[s3]
endpoint = "http://minio:9000"
region = "us-east-1"
bucket = "aumbry"
path_style = true
access_key_id = "..."
secret_access_key = "..."
```

**AWS S3：**

```toml
storage = "s3"

[s3]
region = "us-east-1"
bucket = "aumbry"
access_key_id = "..."
secret_access_key = "..."
```

**私有 WebDAV：**

```toml
storage = "webdav"

[webdav]
url = "https://dav.example.com/aumbry"
user = "aumbry"
password = "replace-with-webdav-password"
```

### S3 与 R2 所需权限

为静态凭证授予目标存储桶的读、写、删、列举权限：

- **AWS S3 / MinIO IAM 权限**：对 `arn:aws:s3:::<bucket>/*` 授予 `s3:GetObject`、`s3:PutObject`、`s3:DeleteObject`，并对 `arn:aws:s3:::<bucket>` 授予 `s3:ListBucket`。
  在 AWS S3 上缺少 `s3:ListBucket` 时，对不存在对象的 `GET`/`HEAD` 会返回 `403 Forbidden` 而非 `404 Not Found`；Aumbry 会把 `403` 归类为 `auth` 鉴权错误，不会误判为对象不存在。
- **Cloudflare R2**：创建作用域限于目标桶且具备 **Object Read & Write** 权限的 S3 API 令牌 *（依据官方文档核对，未在真实 R2 上验证）*。

## 生成密钥与 bcrypt 哈希的 `$` 转义规则

生成 32 字节随机十六进制 Cookie 密钥及管理员密码 bcrypt 哈希：

```sh
openssl rand -hex 32
htpasswd -nbBC 10 '' '<your-password>' | cut -d: -f2
```

（`openssl rand` 已在本机执行；`htpasswd` 来自 Apache `apache2-utils` / `httpd-tools`，命令格式依据 `config.example.toml` 与 `.env.example` 核对。）

不同载体中对 `$` 的转义要求不同：

- **TOML 双引号字符串（`"..."`）**：直接填写原串（`password_hash = "$2y$10$..."`），TOML 不解析 `$` 变量。
- **`.env` 文件（`docker compose` / `make run`）**：必须用单引号包裹（`AUMBRY_ADMIN_PASSWORD_HASH='$2y$10$...'`），避免 `$2y`、`$10` 被 shell 或 Compose 按环境变量替换为空。
- **`docker-compose.yml` 内的 `environment:` 段**：需把每个 `$` 写成 `$$`（`$$2y$$10$$...`）。

## 无副作用配置校验（`check-config`）

运行 `check-config` 校验语法、必填项、bcrypt 哈希、代理 CIDR 与环境变量覆盖结果；该命令不会创建/打开 SQLite，也不会连接对象存储：

```sh
aumbry-server check-config --config ./aumbry.toml
```

正常输出以 TOML 格式打印生效配置，所有敏感字段（含 `s3.access_key_id`、`s3.secret_access_key`、`r2.access_key_id`、`r2.secret_access_key` 与 `webdav.password`）均掩码保留末 4 个字符：

```toml
listen_addr = ':8080'
data_dir = '/data'
storage = 'local'
api_tokens = ['****abcd']
cookie_secret = '****cdef'
trusted_proxies = []

[admin]
user = 'admin'
password_hash = '****lhWy'

[local]
path = '/data/objects'
```

若配置存在文件权限过高告警，告警会写入 `stderr`，在全部字段有效时进程仍返回退出码 `0`。
