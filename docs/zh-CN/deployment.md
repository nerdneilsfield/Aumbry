[English](../en/deployment.md) | 简体中文

# 部署与运维指南

本文说明如何使用 Docker Compose 或宿主机二进制部署 `aumbry-server`，配置反向代理，检查服务探针，以及执行 WAL 安全备份、存储核验、垃圾回收与搜索索引重建。

> **注意：严格单实例运行。** Aumbry 串行处理数据库写入，在 `data_dir` 中持有文件锁（`aumbry.lock`），并在进程内存中保存限流计数器。部署时请确保同一 `data_dir` 与对象存储仅运行一个 `aumbry-server` 实例；同时启动多个实例会引发锁冲突并使限流失效。

## 前置条件

- **对象存储后端**：默认使用本地目录（`storage = "local"`，默认 `<data_dir>/objects`），无需额外依赖；也可按需接入私有 WebDAV 服务（`webdav`）、S3 兼容存储桶（`s3`）或 Cloudflare R2 存储桶（`r2`）。终端访客不直接连接对象存储，所有 XML 与图片下载均经由 `aumbry-server` 代理输出。
- **容器主机**：安装 Docker Engine 与 Docker Compose v2（`ghcr.io/nerdneilsfield/aumbry:latest` 提供 `linux/amd64` 与 `linux/arm64` 镜像）。
- **源码编译主机**：Go `1.27.1+`、Node.js `22+` 与 `npm`。GoReleaser 另在 Release 中发布 `linux`、`darwin` 与 `windows`（`amd64`/`arm64`）预编译包。
- **初始凭证**：管理员密码 bcrypt 哈希（`htpasswd -nbBC 10 '' '<your-password>' | cut -d: -f2`）、一个或多个随机 API token，以及不少于 32 字节的 Cookie 密钥（`openssl rand -hex 32`）。

## 使用 Docker Compose 部署

仓库根目录提供两套独立的 Compose 配置，分别对应 `.env` 方式与 TOML 文件挂载方式。两套配置均不启动外部存储容器；默认 `local` 后端会把对象直接保存在 `/data` 卷中的 `/data/objects`。

### 方式 A：`.env` 环境变量部署（默认 `docker-compose.yml`）

1. 复制环境变量模板并收紧权限：

   ```sh
   cp .env.example .env
   chmod 600 .env
   ```

2. 编辑 `.env`，填写 `AUMBRY_ADMIN_PASSWORD_HASH`、`AUMBRY_API_TOKENS` 与 `AUMBRY_COOKIE_SECRET`（如将 `AUMBRY_STORAGE` 从 `local` 改为 `webdav`、`s3` 或 `r2`，同时填写对应后端字段）。含 `$` 的值（尤其是 bcrypt 哈希）须用单引号包裹，例如 `AUMBRY_ADMIN_PASSWORD_HASH='$2y$10$...'`，防止 `docker compose` 将 `$2y` 按环境变量展开；`AUMBRY_DATA_DIR` 在 `docker-compose.yml` 中已固定设为 `/data`。

3. 校验配置语法并启动容器：

   ```sh
   docker compose config > /dev/null
   docker compose up -d --build
   ```

   （`docker compose config` 已配合临时 `.env` 校验通过；`up -d --build` 依据 [docker-compose.yml](../../docker-compose.yml) 与 [Dockerfile](../../Dockerfile) 核对，本机未实际拉起容器守护进程。）

### 方式 B：TOML 配置文件部署（`docker-compose.toml.yml`）

> **警告：容器 UID `65532` 配置文件与卷权限。** 容器内 `aumbry-server` 以 `nonroot`（UID/GID `65532`）身份运行。宿主机普通用户属主的 `0600` 文件无法被 UID `65532` 读取。请赋予 GID `65532` 组读权限（`640`），不要直接设为 `644`，以免同机其他本地用户读取 `cookie_secret` 与存储凭证。使用 `storage = "local"` 时，宿主机挂载到 `/data` 或自定义 `local.path` 的卷目录同样必须对 UID/GID `65532` 可写（目录权限为 `0700`，对象文件权限为 `0600`）。

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

3. 在不连接对象存储、不打开 SQLite 的前提下校验并查看生效配置：

   ```sh
   ./aumbry-server check-config --config ./aumbry.toml
   ```

   正常输出示例：

   ```toml
   listen_addr = ':8080'
   data_dir = './data'
   storage = 'local'
   api_tokens = ['****abcd']
   cookie_secret = '****cdef'
   trusted_proxies = []

   [admin]
   user = 'admin'
   password_hash = '****lhWy'

   [local]
   path = 'data/objects'
   ```

   遇到未知 TOML 键、缺失必填项或无效 bcrypt 哈希时，`check-config` 会报错并以退出码 `1` 退出。

4. 启动服务：

   ```sh
   ./aumbry-server --config ./aumbry.toml
   ```

   向进程发送 `SIGINT` 或 `SIGTERM` 可触发最长 `10 s` 的优雅停机。

## 数据目录、容量上限与安全响应头

- **`data_dir` 结构**：服务端以 `0700` 权限创建 `data_dir`，锁定 `aumbry.lock` 防止多实例同时启动，并初始化处于 SQLite WAL 模式的 `aumbry.db`（启动时自动完成数据库结构迁移）。
- **对象布局、权限与超时**：条目正文放在 `/xml/<12位ID>-<64位小写十六进制>.xml`（`aumbry validate` / `ingest` 每次更新都会生成新快照路径，删除条目和淘汰历史不再直接 `DELETE` 存储对象，统一由离线 `gc` 清理）；媒体资产放在 `/assets/<64位小写十六进制>.<ext>`（按文件内容与容器校验，图片支持 `.png`/`.jpg`/`.webp`/`.svg`，音频支持 `.mp3`/`.wav`/`.m4a`，`/api/v1/assets/{hash}` 支持 HTTP `Range` 分片与 `HEAD` 请求）。使用 `storage = "local"` 时，服务端在启动时创建 `local.path`（默认 `<data_dir>/objects`，权限 `0700`），对象文件以原子写入方式落盘并设为 `0600`。远程存储后端（`webdav`、`s3`、`r2`）建立 TCP 连接超时为 `5 s`，等待响应头超时为 `30 s`，大对象流式读取不设整体超时。WebDAV 上传先写入 `.tmp-*` 临时对象再通过 HTTP `MOVE` 原子切换。
- **上传、临时盘与限流上限**：`POST /api/v1/ingest` 默认总请求体上限为 `200 MiB`（通过 `ingest.max_body_mb` / `AUMBRY_INGEST_MAX_BODY_MB` 可调，最小 `20 MiB`），XML 正文上限 `10 MiB`，单图上限 `20 MiB`，单音频上限 `100 MiB`，并发上传上限为 `4`。超过 `32 MiB` 的 multipart 媒体部分会流式落盘到临时目录，请为服务器临时目录预留至少 `4 × max_body_mb`（默认至少 `800 MiB`）磁盘空间。`/api/v1/ingest` 与 `/api/v1/agent/*` 共享同 IP 每分钟 `120` 次限流；管理员登录与目录密码解锁限流为同 IP 每分钟 `10` 次。
- **默认安全响应头与订阅源隐私**：所有 HTTP 响应均带 `Referrer-Policy: no-referrer`、`X-Content-Type-Options: nosniff` 与 `X-Frame-Options: DENY`；XML 与资产代理响应带 `Content-Security-Policy: default-src 'none'; sandbox`；受保护目录的 `/feeds/{dir_id}/rss.xml`、`podcast.xml`、`/feeds/{dir_id}/assets/{hash}` 以及后台 Token 生成/轮换接口一律返回 `Cache-Control: private, no-store`，Go HTTP 日志中间件会在到达 `httplog` 前将 `?token=...` 脱敏为 `REDACTED`。
- **Podcast 客户端兼容性提示**：WAV 格式在 Apple Podcasts 中可能不被支持（面向 Apple Podcasts 订阅建议优先推送 `.mp3` 或 `.m4a`）。使用 Pocket Casts / Overcast 等云端拉取型播客客户端订阅受保护目录时，其云端服务器会持有 URL 中的 `?token=`，且须配置外部可访问的 `public_url`。

## 健康检查与就绪探针

- `GET /healthz`：进程存活探针，返回 HTTP `200` 与 `{"status":"ok"}`。
- `GET /readyz`：以 `2 s` 超时并发检测 SQLite 与当前配置的对象存储后端（`local` 在根目录创建并删除临时文件探针、`webdav` 发送 `PROPFIND /`、`s3`/`r2` 发送 `HeadBucket`），对并发探针去重并将存储检测结果在内存中缓存 `5 s`。两者均为 `"ok"` 时返回 HTTP `200`，任一返回 `"auth"`、`"timeout"` 或 `"unreachable"` 时返回 HTTP `503`（`"status":"unavailable"`）；响应中不含端点 URL、桶名、本地路径、访问密钥与签名：

  ```sh
  curl -s http://127.0.0.1:8080/readyz
  ```

  ```json
  {"checks":{"sqlite":"ok","storage":"ok"},"status":"ok"}
  ```

## 反向代理与 `trusted_proxies`

`trusted_proxies` 默认为空列表 `[]`。空列表状态下，服务端忽略 `X-Forwarded-For` 与 `X-Forwarded-Proto`，始终取直接 TCP 对端 IP；会话 Cookie（`aumbry_admin` 有效期 7 天、`aumbry_unlocked` 有效期 30 天）仅在直接 TLS 连接或来自可信代理且携带 `X-Forwarded-Proto: https` 时附带 `Secure` 属性。

在 Nginx、Caddy 或 Traefik 后运行时按下面步骤配置：

1. 将代理服务器 IP 或网段写入 `trusted_proxies`（例如 `trusted_proxies = ["127.0.0.1", "10.0.0.0/8"]`，或环境变量 `AUMBRY_TRUSTED_PROXIES=127.0.0.1,10.0.0.0/8`）。服务端只信任直接 TCP 来源命中该列表的请求，并从右向左跳过可信代理跳数提取真实客户端 IP。
2. 在代理端转发 `X-Forwarded-For`、`X-Forwarded-Proto`、`X-Forwarded-Host` 与 `Host`，将代理最大请求体调至大于 `ingest.max_body_mb`（默认 `200 MiB`，例如 Nginx 设置 `client_max_body_size 201m;`），确保 **绝不缓存 `/feeds/`**（受保护目录 Feed 内含 `?token=` 凭据），并在代理访问日志中移除或脱敏 `?token=` 查询参数（以及关闭对 `/api/v1/assets/*` 与 `/feeds/*/assets/*` 的 Range 压缩破坏）。

`examples/deploy/traefik`、`examples/deploy/nginx` 与 `examples/deploy/caddy` 目录提供了这三种代理的 Compose 示例，可直接复制使用。三个示例中，Aumbry 与代理位于同一个 Docker 网络，Aumbry 不发布任何端口。每个 Compose 文件把该网络固定为子网 `172.31.250.0/24`，并把同一个 CIDR 作为 `AUMBRY_TRUSTED_PROXIES` 传给 Aumbry，因此 Aumbry 只信任来自该网络内容器的转发头。如果宿主机已经使用该网段，在 `.env` 中设置 `PROXY_SUBNET` 即可换成其他网段，子网与 `AUMBRY_TRUSTED_PROXIES` 会同时跟随。不要在 `.env` 中设置 `AUMBRY_TRUSTED_PROXIES`，因为 Compose 文件中的值优先。

不配置 `trusted_proxies` 时，Aumbry 看到的客户端地址只有代理的容器地址。此时所有访客共用同一个限流桶（登录限流每分钟 10 次由全体访客合计），签发的会话 Cookie 也不带 `Secure`。

示例使用已发布的镜像 `ghcr.io/nerdneilsfield/aumbry:latest`，生产环境请固定为某个发布标签。示例只运行单个 Aumbry 实例；镜像中没有 `curl` 或 `wget`，所以 Aumbry 服务不定义容器 healthcheck。如果想用 `aumbry.toml` 代替 `.env` 配置 Aumbry，请像 `docker-compose.toml.yml` 那样挂载该文件，设置 `trusted_proxies = ["172.31.250.0/24"]`（或你的 `PROXY_SUBNET`），并在 Compose 文件的 Aumbry 服务里同时删除 `env_file` 和 `AUMBRY_TRUSTED_PROXIES` 一行，因为环境变量会覆盖 TOML 配置项，`.env` 里残留的任何 `AUMBRY_*` 值仍会生效。

要确认 `trusted_proxies` 已生效，请通过代理登录并查看响应头。命令中的域名和密码需要替换：

```sh
curl -si -X POST https://aumbry.example.com/api/v1/admin/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"<your-password>"}' | grep -i '^set-cookie'
```

`aumbry_admin` Cookie 必须带有 `Secure` 属性。如果没有，说明代理地址不在 `trusted_proxies` 内，或者代理没有发送 `X-Forwarded-Proto: https`。

### 用 Traefik 部署

相关文件是 `examples/deploy/traefik/docker-compose.yml` 与 `.env.example`。示例使用 Traefik v3，通过 HTTP-01 挑战申请 Let's Encrypt 证书，并把 HTTP 重定向到 HTTPS。

1. 确认域名的 DNS 记录指向 Docker 主机，并且公网可以访问 `80` 与 `443` 端口。
2. 在 `examples/deploy/traefik` 目录中复制模板并收紧权限：

   ```sh
   cp .env.example .env
   chmod 600 .env
   ```

3. 编辑 `.env`。设置 `PUBLIC_HOST`（域名）、`ACME_EMAIL`（Let's Encrypt 账号邮箱）以及 Aumbry 的配置项（`AUMBRY_ADMIN_PASSWORD_HASH`、`AUMBRY_API_TOKENS`、`AUMBRY_COOKIE_SECRET`；若不使用默认的 `local`，再填写对应远程存储字段）。bcrypt 哈希外面的单引号必须保留。
4. 启动：

   ```sh
   docker compose up -d
   ```

Traefik 把证书保存在 `traefik-letsencrypt` 卷中。它每 10 s 探测一次 `GET /readyz`；SQLite 或对象存储不可用期间，Traefik 停止向 Aumbry 转发请求，并自行返回 `503`。如果希望对象存储故障时仍由 Aumbry 返回响应，把 healthcheck 路径标签改为 `/healthz`。示例还把入口点的 `readTimeout` 调到 `300s`：Traefik v3 默认的 `60s` 覆盖读取整个请求体的时间，会中断最大 100 MiB 的慢速上传。Traefik 会丢弃客户端自带的 `X-Forwarded-*` 头，所以 Aumbry 收到的只有 Traefik 设置的值。

### 用 nginx 部署

相关文件是 `examples/deploy/nginx/docker-compose.yml`、`aumbry.conf` 与 `.env.example`。nginx 使用你提供的证书文件终止 TLS，并把 HTTP 重定向到 HTTPS。示例不包含 certbot，请用 certbot 或 acme.sh 获取证书。

1. 在 `examples/deploy/nginx` 目录中把 `.env.example` 复制为 `.env`，用 `chmod 600 .env` 收紧权限，并填写 Aumbry 的配置项。
2. 在 `aumbry.conf` 中，把两处 `server_name` 里的 `aumbry.example.com` 都替换为你的域名。
3. 把证书链和私钥分别以 `fullchain.pem` 和 `privkey.pem` 为文件名放入 `examples/deploy/nginx/certs/`。
4. 启动：

   ```sh
   docker compose up -d
   ```

每次证书续期后，把新文件复制到 `certs/`，再执行 `docker compose exec nginx nginx -s reload`。

`aumbry.conf` 已经在 HTTP（80）和 HTTPS（443）两个入口启用了 `aumbry_no_query` 日志格式（使用 `$uri` 而非 `$request_uri`，避免 `?token=ad_...` 写入 nginx 访问日志），并设置了 `Host`、`X-Forwarded-For` 与 `X-Forwarded-Proto`。`client_max_body_size` 设为 `201m`（nginx 的 `m` 表示 MiB），比 Aumbry 默认拒绝的 `200 MiB` 请求体上限（`ingest.max_body_mb`）大 1 MiB，因此超限推送会收到 Aumbry 返回的 JSON `413` 响应，而不是 nginx 的错误页。若开启反向代理缓存，请勿缓存 `/feeds/`。`proxy_send_timeout` 与 `proxy_read_timeout` 均为 `120s`。这两个超时只计算相邻两次写入或读取之间的间隔，而 Aumbry 在把 ingest 请求的对象上传到对象存储之前不会返回任何响应。

### 用 Caddy 部署

相关文件是 `examples/deploy/caddy/docker-compose.yml`、`Caddyfile` 与 `.env.example`。这是最小配置：Caddy 自动申请并续期证书，并把 HTTP 重定向到 HTTPS，无需其他设置。

1. 确认域名的 DNS 记录指向 Docker 主机，并且公网可以访问 `80` 与 `443` 端口（TCP；HTTP/3 另需 UDP）。
2. 在 `examples/deploy/caddy` 目录中把 `.env.example` 复制为 `.env`，用 `chmod 600 .env` 收紧权限，并设置 `PUBLIC_HOST`、`ACME_EMAIL` 与 Aumbry 的配置项。Compose 会把前两项传给 Caddy 容器，`Caddyfile` 以 `{$PUBLIC_HOST}` 与 `{$ACME_EMAIL}` 读取它们。
3. 启动：

   ```sh
   docker compose up -d
   ```

请保留 `caddy-data` 卷，证书保存在其中。Caddy 自己设置 `X-Forwarded-For`、`X-Forwarded-Proto` 与 `X-Forwarded-Host`，并忽略客户端发来的同名头。Caddy 默认不限制请求体大小，因此生效的是 Aumbry 自身的 `ingest.max_body_mb`（默认 `200 MiB`；`/mcp` JSON-RPC 单次请求上限 `16 MiB`）。

### `/mcp` 反向代理与外链抓取运行注意

- **`Host`、`Origin` 与 `public_url`**：`/mcp` 在服务端逐请求校验 HTTP `Host` 与浏览器 `Origin`（防止 DNS Rebinding 与跨站请求）。当 `public_url` 留空时，**`/mcp` 仅放行回环 `Host`（`localhost` / `127.0.0.1` / `::1`）**，任何内网或公网非回环 `Host` 一律返回 `403`。因此只要通过非回环网卡或反向代理接入 `/mcp`，必须设置 `public_url = "https://<你的对外域名>[:端口]"`，且反代务必透传含端口的原始 Host（例如 nginx 使用 `proxy_set_header Host $http_host;`，而非会丢弃非标准端口的 `$host`）。
- **SSE 与两道体积上限**：若使用 `GET /mcp` 独立 SSE 长连接，在 nginx/反代上保持 `proxy_http_version 1.1` 并对 `/mcp` 关闭代理缓冲（`proxy_buffering off;`）。`/mcp` 同时执行两道独立大小上限：`16 MiB` JSON-RPC 原始请求体上限（超限 HTTP `413`）与 `10 MiB` 解码后 XML 上限；反代 `client_max_body_size` 至少设为 `16m`（示例 nginx 已设为 `201m`）。接近 `10 MiB` 且包含大量需 JSON 转义字符（换行、双引号、尖括号）的文档若编码后超过 `16 MiB`，请改用 CLI `aumbry push`。
- **外链图片转存并发与直连策略**：`ingest.allow_remote_images = true`（默认）开启时，服务端最多使用 4 并发、单图 15 s、整篇 **60 s 总预算** 抓图；该过程占用全局 **4 个 `ingest` 并发槽位**之一，因此在大量并发抓取慢速外链时可能短暂触发后来请求的 `429`。专用抓图 HTTP 客户端**显式禁用代理（`Proxy: nil`）**并在每次 Socket建连（含 5 跳以内重定向）前校验 `.Unmap()` 后的真实远端 IP，**不会继承或走系统 `HTTP_PROXY`/`HTTPS_PROXY`**；若部署在必须经出口 HTTP 代理才能访问公网的隔离环境，可设为 `allow_remote_images = false` 并由推送端本地上传图片。

## 备份、存储核验与维护命令

下列子命令均与服务端主进程遵循同一配置加载顺序（`--config`、`AUMBRY_CONFIG`、`./aumbry.toml`、`/etc/aumbry/config.toml` 或纯环境变量）。

> **Docker Compose 下执行维护命令**：镜像 `ENTRYPOINT` 就是 `/usr/local/bin/aumbry-server`，所有配置卷（`/data` 及 TOML 模式下只读挂载的 `/etc/aumbry/config.toml`）与 `.env` 都会自动复用。直接通过一次性容器调用子命令即可（若使用 `docker-compose.toml.yml`，在 `compose` 后加上 `-f docker-compose.toml.yml`）：
>
> ```sh
> # 在线检查或只读演练（无需停机）
> docker compose run --rm --no-deps aumbry verify-storage
> docker compose run --rm --no-deps aumbry gc --dry-run
> docker compose run --rm --no-deps aumbry reindex --dry-run
>
> # 破坏性写入 / 重建前必须先停服释放 aumbry.lock
> docker compose stop aumbry
> docker compose run --rm --no-deps aumbry reindex
> docker compose up -d aumbry
> ```
>
> 执行 `backup` 导出到宿主机时，请预先创建目录并授予容器 `nonroot` 用户（UID/GID `65532`）写权限：`mkdir -p ./backups && sudo chown 65532:65532 ./backups && chmod 700 ./backups`。

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

2. `backup` 仅备份 SQLite 数据库；不能在写入持续期间「先快照 SQLite、再事后复制对象目录」，因为之后运行的 `gc` 会删除快照仍然引用的对象。完整备份的做法是：停止服务（或停止写入并暂停 `gc`），同时保存 SQLite 快照以及对象存储的对应内容（`local` 复制对象目录，`s3`、`r2` 或 `webdav` 使用存储服务自带的复制功能或 `rclone`）。

3. 恢复数据库快照及与之配对的对象存储后，先验证恢复结果，再对外提供访问：

   1. 运行 `verify-storage`（见下一节），退出码必须为 `0`：

      ```sh
      aumbry-server verify-storage --config ./aumbry.toml
      ```

   2. 启动 `aumbry-server`，登录管理员页面，打开一篇含图片的报告，确认正文正常渲染且至少有一张图片能显示。`verify-storage` 只检查对象是否存在；这一步确认对象能经服务端读取。

### 核验对象引用完整性（`verify-storage`）

`verify-storage` 逐条检查 `aumbry.db` 中记录的所有现行条目、历史修订（`item_revisions`）的 XML 文件以及全部当前与历史引用资产（图片、音频与 PDF）是否仍存在于对象存储（使用 `local` 时，根目录必须已存在，命令不会自动创建缺失目录；否则以非零退出码返回）：

```sh
aumbry-server verify-storage --config ./aumbry.toml
```

对象完整时的实测输出（缺失对象时非零退出并列出缺失路径）：

```text
storage OK: verified 1 xml and 0 asset objects
```

### 清理无引用对象（`gc`）

当 `ingest` 写完对象后事务失败、更新/淘汰修订版本或删除条目使旧 XML 与旧媒体不再被 `items` 或 `item_revisions` 引用时，孤儿对象会留在对象存储等待离线回收。`gc` 仅匹配 Aumbry 生成的路径格式（`/xml/<12位ID>-<64位小写十六进制>.xml` 与 `/assets/<64位小写十六进制>.(png|jpg|webp|svg|mp3|wav|m4a|pdf)`），运维手工放置在 `/xml/README.txt` 或 `/assets/manual.png` 等非托管路径不会被列出或删除。使用 `local` 时，配置的对象根目录必须已存在（`gc` 与 `gc --dry-run` 均不会创建缺失目录，遇到不存在时直接报错）。

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

2 个字及以上的中文、日文、韩文检索词（如 `模型`）使用 SQLite 全文索引 `items_fts_cjk`。其他 3 个字符及以上的检索词使用 `items_fts`。单个 CJK 字符以及其他不足 3 个字符的检索词，对标题、摘要、标签和 `items.plain_text` 做子串匹配。每次 `POST /api/v1/ingest` 都在同一事务内写入 `items.plain_text`、`items_fts` 与 `items_fts_cjk`。`reindex` 从对象存储重新读取每个条目已存储的 XML，按当前版本的规则重新计算 `plain_text`（含 `<audio>` 标题/章节/逐字稿）、重写 `items_fts` / `items_fts_cjk`，并同步重建反向引用 `item_links`、首音频投影 `primary_audio_*`，以及为**已挂载但尚无历史记录（0 revision）的存量条目自动回填基线 `rev = 1`**（含当时挂载与资产关联）。

> **从 `v0.1.3` / `v0.1.4` 升级必读顺序**：由于 `reindex` / `verify-storage` / `gc` 使用不执行迁移的只读 schema 打开逻辑，升级后请**先启动新版 `aumbry-server`**（由 `db.Open` 自动应用 `00004`–`00008` 迁移），**随后停服执行一次 `aumbry-server reindex`**，完成反向索引、首音频投影与存量 `rev = 1` 快照回填后再恢复对外服务。Docker Compose 完整升级命令见下方 [升级版本](#升级版本支持从-v013-直接升至-v020) 一节。

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

   `with changed plain_text` 统计已存储的 `plain_text` 与重新计算结果不同的条目数。正式运行会重写每个可读条目的 `items_fts` 与 `items_fts_cjk` 行，不论 `plain_text` 是否变化。

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

如果某个条目的 XML 无法从对象存储读取，或已不是有效 XML，`reindex` 会输出 `error: item <id> (<xml 路径>): ...`，保持该条目的检索数据不变，并继续处理其余条目。处理完最后一个条目后，命令输出汇总并以退出码 `1` 结束。排除原因后（可用 `verify-storage` 查找缺失对象）再次运行 `reindex` 即可，重复运行是安全的。

### 更换存储后端

Aumbry 不提供内置的跨后端迁移命令：停机后使用 `rclone` 把旧后端的 `xml/` 与 `assets/` 目录复制到新后端，修改配置，先运行 `aumbry-server verify-storage` 核验完整性，确认无误后再启动服务。

## 升级版本（支持从 `v0.1.3` 直接升至 `v0.2.0`）

无需经过 `v0.1.4` 中转，可从 `v0.1.3` 直接升级到 `v0.2.0`：

- 新版 `aumbry-server` 启动时会自动按顺序补齐 `00004`–`00008` 数据库迁移（`v0.1.4` 升级只需补跑 `00008`）。
- `v0.2.0` 新增配置项（`public_url`、`[revisions].max_per_item`、`[ingest].max_body_mb`、`[ingest].allow_remote_images`）均有默认值，旧 `.env` 或 `aumbry.toml` 无需改动即可直接启动。
- **核心原则**：由于离线子命令 `reindex` / `verify-storage` / `gc` 不会运行数据库迁移，升级时必须遵循「**先启动新版 `aumbry-server` 自动迁库 → 停服执行 `reindex` → 再恢复运行**」的顺序。

### Docker Compose 升级步骤

以下命令在 Compose 文件所在目录执行（例如仓库根目录或 `examples/deploy/nginx` 目录）；若采用 `docker-compose.toml.yml`，将每条 `docker compose` 替换为 `docker compose -f docker-compose.toml.yml`：

1. **升级前在线备份 SQLite**（如使用 `local` 存储，建议同时备份 `aumbry-data` 卷或停机拷贝对象目录）：

   ```sh
   mkdir -p ./backups
   sudo chown 65532:65532 ./backups && chmod 700 ./backups
   docker compose run --rm --no-deps -v "$PWD/backups:/backups" \
     aumbry backup --out "/backups/aumbry-pre-v020-$(date +%Y%m%d-%H%M%S).db"
   ```

2. **拉取（或重新构建）`v0.2.0` 并启动一次主服务以应用迁移 `00004`–`00008`**：

   - 若使用 GHCR 预构建镜像（如 `examples/deploy/{traefik,nginx,caddy}`，建议把 `image:` 固定为 `ghcr.io/nerdneilsfield/aumbry:v0.2.0`）：

     ```sh
     docker compose pull aumbry
     docker compose up -d aumbry
     ```

   - 若使用源码本地构建（根目录 `docker-compose.yml` / `docker-compose.toml.yml`）：

     ```sh
     git fetch --tags && git checkout v0.2.0
     docker compose up -d --build aumbry
     ```

   查看日志确认新版容器已正常启动并完成监听：

   ```sh
   docker compose logs --tail 50 aumbry
   ```

3. **停服执行一次 `reindex` 完成存量回填**：

   该步骤会为升级前已挂载的条目重建 `items_fts` / `items_fts_cjk`、反向链接 `item_links`、首音频投影 `primary_audio_*` 与基线 `rev = 1` 快照（旧快照保留 `author_verified = 0`，在 MCP 中统一按历史挂载 B1 鉴权）：

   ```sh
   docker compose stop aumbry
   docker compose run --rm --no-deps aumbry reindex
   ```

4. **恢复服务并核验存储完整性**：

   ```sh
   docker compose up -d aumbry
   docker compose run --rm --no-deps aumbry verify-storage
   ```

5. **按需启用 `v0.2.0` 新能力（可选）**：

   - 若通过域名/反向代理公开 `/mcp` 或使用 `/feeds/`，在 `.env` 中补充 `AUMBRY_PUBLIC_URL=https://<你的域名>`（或在 `aumbry.toml` 设置 `public_url`）；否则 `/mcp` 只放行本机回环 `Host`。
   - 若使用 Nginx，请对照更新版 [examples/deploy/nginx/aumbry.conf](../../examples/deploy/nginx/aumbry.conf) 同步 `/mcp` 长连接参数（`proxy_set_header Host $http_host;`、`proxy_http_version 1.1;`、`proxy_buffering off;`）及脱敏日志格式，然后执行 `docker compose exec nginx nginx -s reload`。

### 宿主机二进制升级简述

步骤顺序完全一致：**先使用新版 `./aumbry-server backup --out ...` 备份 → 替换二进制并启动一次新版 `./aumbry-server` 应用迁移 → 停服运行一次 `./aumbry-server reindex` → 重新启动服务并跑一次 `verify-storage`**。同时把客户端机器上的 `aumbry` CLI 升级到 `v0.2.0` 即可使用 `v1.2` PDF、外链转存与 MCP 相关命令。

## 首次发布：把 GHCR 包设为公开

本节面向维护者，只在首次发布并推送 `ghcr.io/nerdneilsfield/aumbry` 之后执行一次。GitHub 会把新建的容器包默认设为私有，未登录 `ghcr.io` 的主机执行 `docker pull` 会失败，直到修改包的可见性。之后的发布沿用该设置。

1. 发布工作流成功后，在 GitHub 上打开所有者的 **Packages** 标签页，选择 `aumbry`。
2. 确认包页面显示的关联仓库是 `nerdneilsfield/Aumbry`。镜像带有 `org.opencontainers.image.source` 标签，GitHub 通常会据此自动关联。如果没有显示仓库，选择 **Connect repository** 并选中 `nerdneilsfield/Aumbry`。
3. 选择 **Package settings**，滚动到 **Danger Zone**，选择 **Change visibility**。
4. 选择 **Public**，输入包名 `aumbry` 确认，再选择 **I understand the consequences, change package visibility**。
5. 在未登录 `ghcr.io` 的主机上拉取已发布的标签，例如 `docker pull ghcr.io/nerdneilsfield/aumbry:<version>`，应无需凭据即可成功。

以上步骤依据 GitHub 的包设置页面编写，验证期间没有实际操作，界面标签可能被 GitHub 改名。
