[English](../en/configuration.md) | 简体中文

# 服务端配置参考

`aumbry-server` 与维护子命令（`check-config`、`backup`、`verify-storage`、`gc`）共用同一套严格配置加载规则，支持 TOML 文件、环境变量以及两者混合配置。

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
- **首尾空白裁剪**：除 `AUMBRY_COOKIE_SECRET` 与 `AUMBRY_WEBDAV_PASS` 保持原样外，其余所有配置值及逗号分隔项均先裁剪首尾空白再校验。
- **文件权限告警**：当已读取的 TOML 文件对同组或其他用户可读时，服务端输出提示告警（建议 `chmod 600`）。

## 配置项清单

| TOML 键 | 环境变量 | 类型 | 必填 | 默认值 | 说明与约束 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `listen_addr` | `AUMBRY_LISTEN_ADDR` | 字符串 | 是（有默认值） | `":8080"` | HTTP 监听地址（`host:port` 或 `:port`）。裁剪空白后为空时回退至 `":8080"`。 |
| `data_dir` | `AUMBRY_DATA_DIR` | 字符串 | 是 | 无 | 存放 `aumbry.db` 与 `aumbry.lock` 的目录。首次启动以 `0700` 创建；必须可写。 |
| `admin.user` | `AUMBRY_ADMIN_USER` | 字符串 | 是 | 无 | 登录 `POST /api/v1/admin/login` 使用的管理员用户名，裁剪空白后非空。 |
| `admin.password_hash` | `AUMBRY_ADMIN_PASSWORD_HASH` | 字符串 | 是 | 无 | 合法 bcrypt 哈希（支持 `$2$`、`$2a$`、`$2b$`、`$2x$`、`$2y$`），校验开销、分隔符与 53 字符盐+摘要 base64 编码。 |
| `api_tokens` | `AUMBRY_API_TOKENS` | 字符串列表（TOML 数组或逗号分隔环境变量） | 是 | 无 | 允许调用 `/api/v1/ingest` 与 `/api/v1/agent/*` 的 Bearer token 列表，至少 1 个非空项；空项自动过滤。 |
| `cookie_secret` | `AUMBRY_COOKIE_SECRET` | 字符串 | 是 | 无 | 用于签名和加密会话 Cookie 的根密钥。最少 `32` 字节；更换后所有既有访客与管理员会话立即失效。 |
| `webdav.url` | `AUMBRY_WEBDAV_URL` | 字符串 | 是 | 无 | 私有 WebDAV 根目录绝对 URL，必须包含协议（如 `https://`）与主机名。 |
| `webdav.user` | `AUMBRY_WEBDAV_USER` | 字符串 | 是 | 无 | WebDAV HTTP Basic 认证用户名，裁剪空白后非空。 |
| `webdav.password` | `AUMBRY_WEBDAV_PASS` | 字符串 | 是 | 无 | WebDAV HTTP Basic 认证密码；环境变量 `AUMBRY_WEBDAV_PASS` 原样读取，不裁剪空白。 |
| `trusted_proxies` | `AUMBRY_TRUSTED_PROXIES` | 字符串列表（TOML 数组或逗号分隔环境变量） | 否 | `[]` | 允许解析 `X-Forwarded-For` 与 `X-Forwarded-Proto` 的反向代理 IP 或 CIDR 列表（如 `10.0.0.0/8`）。非法地址会导致启动报错。 |

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

运行 `check-config` 校验语法、必填项、bcrypt 哈希、代理 CIDR 与环境变量覆盖结果；该命令不会创建/打开 SQLite，也不会连接 WebDAV：

```sh
aumbry-server check-config --config ./aumbry.toml
```

正常输出以 TOML 格式打印生效配置，所有敏感字段只保留末尾 4 个字符：

```toml
listen_addr = ':8080'
data_dir = '/data'
api_tokens = ['****abcd']
cookie_secret = '****cdef'
trusted_proxies = []

[admin]
user = 'admin'
password_hash = '****lhWy'

[webdav]
url = 'https://dav.example.com/aumbry'
user = 'aumbry'
password = '****9999'
```

若配置存在文件权限过高告警，告警会写入 `stderr`，在全部字段有效时进程仍返回退出码 `0`。
