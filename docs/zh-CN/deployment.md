[English](../en/deployment.md) | 简体中文

# 部署与运维指南

本文说明如何使用 Docker Compose 或宿主机二进制部署 `aumbry-server`，配置反向代理，检查服务探针，以及执行 WAL 安全备份、存储核验、垃圾回收与搜索索引重建。

> **注意：严格单实例运行。** Aumbry 串行处理数据库写入，在 `data_dir` 中持有文件锁（`aumbry.lock`），并在进程内存中保存限流计数器。部署时请确保同一 `data_dir` 与私有 WebDAV 根目录仅运行一个 `aumbry-server` 实例；同时启动多个实例会引发锁冲突并使限流失效。

## 前置条件

- **私有 WebDAV 存储**：`aumbry-server` 可通过 HTTP/HTTPS 访问的 WebDAV 服务。终端访客不直接连接 WebDAV，所有 XML 与图片下载均经由服务端代理输出。
- **容器主机**：安装 Docker Engine 与 Docker Compose v2（`ghcr.io/nerdneilsfield/aumbry:latest` 提供 `linux/amd64` 与 `linux/arm64` 镜像）。
- **源码编译主机**：Go `1.27.1+`、Node.js `22+` 与 `npm`。GoReleaser 另在 Release 中发布 `linux`、`darwin` 与 `windows`（`amd64`/`arm64`）预编译包。
- **初始凭证**：管理员密码 bcrypt 哈希（`htpasswd -nbBC 10 '' '<your-password>' | cut -d: -f2`）、一个或多个随机 API token，以及不少于 32 字节的 Cookie 密钥（`openssl rand -hex 32`）。

## 使用 Docker Compose 部署

仓库根目录提供两套独立的 Compose 配置，分别对应 `.env` 方式与 TOML 文件挂载方式。两套配置均不包含 WebDAV 容器。

### 方式 A：`.env` 环境变量部署（默认 `docker-compose.yml`）

1. 复制环境变量模板并收紧权限：

   ```sh
   cp .env.example .env
   chmod 600 .env
   ```

2. 编辑 `.env`，填写 `AUMBRY_ADMIN_PASSWORD_HASH`、`AUMBRY_API_TOKENS`、`AUMBRY_COOKIE_SECRET`、`AUMBRY_WEBDAV_URL`、`AUMBRY_WEBDAV_USER` 与 `AUMBRY_WEBDAV_PASS`。含 `$` 的值（尤其是 bcrypt 哈希）须用单引号包裹，例如 `AUMBRY_ADMIN_PASSWORD_HASH='$2y$10$...'`，防止 `docker compose` 将 `$2y` 按环境变量展开；`AUMBRY_DATA_DIR` 在 `docker-compose.yml` 中已固定设为 `/data`。

3. 校验配置语法并启动容器：

   ```sh
   docker compose config > /dev/null
   docker compose up -d --build
   ```

   （`docker compose config` 已配合临时 `.env` 校验通过；`up -d --build` 依据 [docker-compose.yml](../../docker-compose.yml) 与 [Dockerfile](../../Dockerfile) 核对，本机未实际拉起容器守护进程。）

### 方式 B：TOML 配置文件部署（`docker-compose.toml.yml`）

> **警告：容器 UID `65532` 读取权限。** 容器内 `aumbry-server` 以 `nonroot`（UID/GID `65532`）身份运行。宿主机普通用户属主的 `0600` 文件无法被 UID `65532` 读取。请赋予 GID `65532` 组读权限（`640`），不要直接设为 `644`，以免同机其他本地用户读取 `cookie_secret` 与 WebDAV 密码。

1. 复制模板（`config.example.toml` 默认 `data_dir = "/data"`，与容器卷挂载路径一致）并填写全部必填项：

   ```sh
   cp config.example.toml aumbry.toml
   ```

2. 设置 GID `65532` 可读的最小权限：

   ```sh
   sudo chgrp 65532 aumbry.toml
   chmod 640 aumbry.toml
   ```

   （文件对同组可读时，`aumbry-server` 启动会输出一条权限提示日志；若不希望出现该提示，可改用 `sudo chown 65532:65532 aumbry.toml && chmod 600 aumbry.toml`。）

3. 校验并启动 `docker-compose.toml.yml`：

   ```sh
   docker compose -f docker-compose.toml.yml config > /dev/null
   docker compose -f docker-compose.toml.yml up -d --build
   ```

   （`docker compose -f docker-compose.toml.yml config` 已实测退出码 `0`；`up -d --build` 对照 [docker-compose.toml.yml](../../docker-compose.toml.yml) 核对。）

## 使用二进制部署在宿主机

1. 编译前端资源以及 CLI 与服务端可执行文件：

   ```sh
   make setup
   make build
   ```

2. 准备 `aumbry.toml`，把 `data_dir` 改为宿主机可写目录（例如 `data_dir = "./data"`）并设为仅属主读写：

   ```sh
   cp config.example.toml aumbry.toml
   chmod 600 aumbry.toml
   ```

3. 在不连接 WebDAV、不打开 SQLite 的前提下校验并查看生效配置：

   ```sh
   ./aumbry-server check-config --config ./aumbry.toml
   ```

   正常输出示例：

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

   遇到未知 TOML 键、缺失必填项或无效 bcrypt 哈希时，`check-config` 会报错并以退出码 `1` 退出。

4. 启动服务：

   ```sh
   ./aumbry-server --config ./aumbry.toml
   ```

   向进程发送 `SIGINT` 或 `SIGTERM` 可触发最长 `10 s` 的优雅停机。

## 数据目录、容量上限与安全响应头

- **`data_dir` 结构**：服务端以 `0700` 权限创建 `data_dir`，锁定 `aumbry.lock` 防止多实例同时启动，并初始化处于 SQLite WAL 模式的 `aumbry.db`（启动时自动完成数据库结构迁移）。
- **WebDAV 对象布局与超时**：条目正文放在 `/xml/<12位ID>-<64位小写十六进制>.xml`；图片放在 `/assets/<64位小写十六进制>.<ext>`（按文件内容 MIME 识别，仅允许 `.png`、`.jpg`、`.webp`、`.svg`）。WebDAV 连接建立超时为 `5 s`，等待响应头超时为 `30 s`。
- **上传与限流上限**：`POST /api/v1/ingest` 单次请求上限 `100 MiB`，XML 正文上限 `10 MiB`，单张图片上限 `20 MiB`，并发上传上限为 `4`；`/api/v1/ingest` 与 `/api/v1/agent/*` 共享同 IP 每分钟 `120` 次限流。管理员登录与目录密码解锁接口限流为同 IP 每分钟 `10` 次。
- **默认安全响应头**：所有 HTTP 响应均带 `Referrer-Policy: no-referrer`、`X-Content-Type-Options: nosniff` 与 `X-Frame-Options: DENY`；XML 与图片代理响应额外带 `Cache-Control: private` 与 `Content-Security-Policy: default-src 'none'; sandbox`。

## 健康检查与就绪探针

- `GET /healthz`：进程存活探针，返回 HTTP `200` 与 `{"status":"ok"}`。
- `GET /readyz`：以 `2 s` 超时并发检测 SQLite 与 WebDAV（`PROPFIND /`），对并发探针去重并将 WebDAV 检测结果在内存中缓存 `5 s`。两者均为 `"ok"` 时返回 HTTP `200`，任一返回 `"auth"`、`"timeout"` 或 `"unreachable"` 时返回 HTTP `503`（`"status":"unavailable"`）：

  ```sh
  curl -s http://127.0.0.1:8080/readyz
  ```

  ```json
  {"checks":{"sqlite":"ok","webdav":"ok"},"status":"ok"}
  ```

## 反向代理与 `trusted_proxies`

`trusted_proxies` 默认为空列表 `[]`。空列表状态下，服务端忽略 `X-Forwarded-For` 与 `X-Forwarded-Proto`，始终取直接 TCP 对端 IP；会话 Cookie（`aumbry_admin` 有效期 7 天、`aumbry_unlocked` 有效期 30 天）仅在直接 TLS 连接或来自可信代理且携带 `X-Forwarded-Proto: https` 时附带 `Secure` 属性。

在 Nginx、Caddy 或 Traefik 后运行时按下面步骤配置：

1. 将代理服务器 IP 或网段写入 `trusted_proxies`（例如 `trusted_proxies = ["127.0.0.1", "10.0.0.0/8"]`，或环境变量 `AUMBRY_TRUSTED_PROXIES=127.0.0.1,10.0.0.0/8`）。服务端只信任直接 TCP 来源命中该列表的请求，并从右向左跳过可信代理跳数提取真实客户端 IP。
2. 在代理端转发 `X-Forwarded-For`、`X-Forwarded-Proto` 与 `Host`，并将代理最大请求体调至不少于 `100 MiB`（例如 Nginx 设置 `client_max_body_size 100m;`），防止多图推送在代理层被截断。

## 备份、存储核验与维护命令

下列子命令均与服务端主进程遵循同一配置加载顺序（`--config`、`AUMBRY_CONFIG`、`./aumbry.toml`、`/etc/aumbry/config.toml` 或纯环境变量）。

### 在线安全备份（`backup`）

> **注意：不要直接复制 `aumbry.db`。** 在 WAL 模式下，未刷回主库文件的提交仍位于 `aumbry.db-wal` 中，直接文件拷贝会造成快照缺页。请始终使用 `aumbry-server backup --out <path>`：该命令先以 `0600` 创建空目标文件，再通过 SQLite `VACUUM INTO` 在线生成完整快照，失败时自动清理半成品文件。

1. 将快照写入尚不存在的目标路径：

   ```sh
   aumbry-server backup --config ./aumbry.toml --out /var/backups/aumbry-$(date +%Y%m%d).db
   ```

   实测输出：

   ```text
   backup written to /var/backups/aumbry-20261122.db
   ```

2. `backup` 仅备份 SQLite 数据库；条目后续更新时，服务端会删除 WebDAV 中的旧版 XML。如需完整时间点恢复，请将 SQLite 快照与同时刻的 WebDAV 存储快照成对保存。

3. 恢复数据库快照及与之配对的 WebDAV 快照后，先验证恢复结果，再对外提供访问：

   1. 运行 `verify-storage`（见下一节），退出码必须为 `0`：

      ```sh
      aumbry-server verify-storage --config ./aumbry.toml
      ```

   2. 启动 `aumbry-server`，登录管理员页面，打开一篇报告（最好是含图片的报告），确认正文正常渲染且至少有一张图片能显示。`verify-storage` 只检查对象是否存在；这一步确认对象能经服务端读取。

### 核验对象引用完整性（`verify-storage`）

`verify-storage` 逐条检查 `aumbry.db` 中记录的所有条目 XML 文件与引用图片是否仍存在于 WebDAV；发现缺失时以非零退出码返回并列出缺失清单：

```sh
aumbry-server verify-storage --config ./aumbry.toml
```

对象完整时的实测输出：

```text
storage OK: verified 1 xml and 0 asset objects
```

### 清理无引用对象（`gc`）

当 `ingest` 写完对象后事务失败（如并发争用），或旧图片不再被任何条目引用时，孤儿对象会留在 WebDAV 上等待离线回收。`gc` 仅匹配 Aumbry 生成的路径格式（`/xml/<12位ID>-<64位小写十六进制>.xml` 与 `/assets/<64位小写十六进制>.(png|jpg|webp|svg)`），运维手工放置在 `/xml/README.txt` 或 `/assets/manual.png` 等非托管路径不会被列出或删除。

1. 在服务运行期间先用 `--dry-run` 查看待清理清单（只读、安全）：

   ```sh
   aumbry-server gc --config ./aumbry.toml --dry-run
   ```

   实测输出示例：

   ```text
   dry-run: 0 orphan xml, 0 orphan remote assets, 0 orphan db asset rows (0 db-only)
   ```

2. 停止 `aumbry-server`。若服务仍在运行且持有 `aumbry.lock`，破坏性 `gc` 会直接拒绝执行：

   ```text
   refusing to run destructive gc while aumbry-server is running; stop the server first or pass --dry-run
   ```

3. 确认服务已停机后执行正式清理：

   ```sh
   aumbry-server gc --config ./aumbry.toml
   ```

   实测输出示例：

   ```text
   gc complete: deleted 0 xml, 0 remote assets, 0 db asset rows
   ```

4. 重新启动 `aumbry-server`。

### 重建搜索索引（`reindex`）

2 个字及以上的中文、日文、韩文检索词（如 `模型`）使用 SQLite 全文索引 `items_fts_cjk`。其他 3 个字符及以上的检索词使用 `items_fts`。单个 CJK 字符以及其他不足 3 个字符的检索词，对 `items.plain_text` 做子串匹配。每次 `POST /api/v1/ingest` 都在同一事务内写入 `items.plain_text`、`items_fts` 与 `items_fts_cjk`。`reindex` 从 WebDAV 重新读取每个条目已存储的 XML，按当前版本的规则重新计算索引文本，并同时重写这三处，使各条检索路径保持一致。

出现下列情况之一时运行 `reindex`：

- 升级到改变了索引文本范围的版本。此前入库的文档仍保留旧的索引文本，重建后才会按新规则生效。`items_fts_cjk` 这类新表初始为空，在运行 `reindex` 之前，2 个字及以上的中文检索词在早先入库的文档里搜不到结果。
- 检索结果与报告内容不一致，例如恢复数据库快照后，或手工修复过数据库后。

正常的入库、删除和 `backup` 不需要它。

写入式运行与 `gc` 使用同一把 `aumbry.lock`，因此必须先停止 `aumbry-server`。`--dry-run` 只读，服务运行时也可执行。

1. 预览结果。`--dry-run` 会读取并解析每个条目，但不写入任何数据：

   ```sh
   aumbry-server reindex --config ./aumbry.toml --dry-run
   ```

   输出示例：

   ```text
   dry-run: would reindex 12 of 12 items (3 with changed plain_text), 0 failed
   ```

   `with changed plain_text` 统计已存储的 `plain_text` 与重新计算结果不同的条目数。正式运行会重写每个可读条目的 `items_fts` 行，不论 `plain_text` 是否变化。

2. 停止 `aumbry-server`。若服务仍持有 `aumbry.lock`，写入式运行会立即退出：

   ```text
   refusing to run reindex while aumbry-server is running; stop the server first or pass --dry-run
   ```

3. 执行重建。命令每 100 个条目为一批，一批一个事务：

   ```sh
   aumbry-server reindex --config ./aumbry.toml
   ```

   输出示例：

   ```text
   reindexed 12 of 12 items (3 with changed plain_text), 0 failed
   ```

4. 重新启动 `aumbry-server`。

如果某个条目的 XML 无法从 WebDAV 读取，或已不是有效 XML，`reindex` 会输出 `error: item <id> (<xml 路径>): ...`，保持该条目的检索数据不变，并继续处理其余条目。处理完最后一个条目后，命令输出汇总并以退出码 `1` 结束。排除原因后（可用 `verify-storage` 查找缺失对象）再次运行 `reindex` 即可，重复运行是安全的。

## 首次发布：把 GHCR 包设为公开

本节面向维护者，只在首次发布并推送 `ghcr.io/nerdneilsfield/aumbry` 之后执行一次。GitHub 会把新建的容器包默认设为私有，未登录 `ghcr.io` 的主机执行 `docker pull` 会失败，直到修改包的可见性。之后的发布沿用该设置。

1. 发布工作流成功后，在 GitHub 上打开所有者的 **Packages** 标签页，选择 `aumbry`。
2. 确认包页面显示的关联仓库是 `nerdneilsfield/Aumbry`。镜像带有 `org.opencontainers.image.source` 标签，GitHub 通常会据此自动关联。如果没有显示仓库，选择 **Connect repository** 并选中 `nerdneilsfield/Aumbry`。
3. 选择 **Package settings**，滚动到 **Danger Zone**，选择 **Change visibility**。
4. 选择 **Public**，输入包名 `aumbry` 确认，再选择 **I understand the consequences, change package visibility**。
5. 在未登录 `ghcr.io` 的主机上拉取已发布的标签，例如 `docker pull ghcr.io/nerdneilsfield/aumbry:<version>`，应无需凭据即可成功。

以上步骤依据 GitHub 的包设置页面编写，验证期间没有实际操作，界面标签可能被 GitHub 改名。
