[English](README.md) | 简体中文

# Aumbry

Aumbry 是一个面向 AI agent 与人工研究产出的自托管私有报告库，提供 XML 校验、目录分权分享与全文检索。

[![CI](https://github.com/nerdneilsfield/Aumbry/actions/workflows/ci.yml/badge.svg)](https://github.com/nerdneilsfield/Aumbry/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/github/license/nerdneilsfield/Aumbry)](LICENSE)

## 核心特性

- **单二进制部署与内嵌阅读器**：`aumbry-server` 内嵌 Web 阅读器与管理控制台，单个二进制直接提供 `/d/<id>`、`/r/<id>`、`/p/<id>` 与 `/admin` 页面。
- **前后端共享的 XML 校验器**：`aumbry` CLI 与 `aumbry-server` 执行同一套 XML 校验规则（`version="1.1"`），在上传前拦截未知标签、图表错误、超限代码块及越界符号链接图片。
- **SQLite 元数据与全文检索**：目录关系、条目元数据与全文搜索索引保存在 `<data_dir>/aumbry.db`（SQLite WAL 模式），支持 2 个字及以上的中文词直接检索（单字也可以）、其他 3 个字符及以上的全文检索，以及更短检索词的子串匹配。
- **目录级密码继承与私有 WebDAV 代理分发**：未挂载目录的条目对访客返回 HTTP `403`；目录密码沿子树继承；WebDAV 中的 XML 与图片仅通过服务端代理输出，并附加 `Content-Security-Policy: default-src 'none'; sandbox`。
- **随库分发的 Agent Skill**：[skills/aumbry/](skills/aumbry/SKILL.md) 随仓库与 Release 归档一起提供，规范 agent 发现目录、离线校验、保存草稿及幂等更新流程。

## 概述

Aumbry 把“报告编写/校验”与“托管/权限控制”分开。AI agent 在本地生成结构化 XML 报告与相对路径配图，先运行 `aumbry validate` 通过本地校验，再使用 `api_tokens` 中的 Bearer token 推送到服务端 `POST /api/v1/ingest`。人类读者通过获分享的目录链接（`/d/<dir-id>`）或条目链接（`/r/<id>`、`/p/<id>`）访问内容，网站首页（`/`）不列出任何公开目录。

项目基于 MIT 许可证开源（`github.com/nerdneilsfield/aumbry`），当前 `master` 分支实现 `1.1` 协议及配套功能。Aumbry 定位为单实例私有研究库，而不是多人公开 Wiki 或多副本集群，每个 `data_dir` 与私有 WebDAV 存储同一时刻仅允许连接一个服务端进程。

## 安装

可使用预构建容器镜像或从源码编译。GitHub Release 同样通过 GoReleaser 提供 `linux`、`darwin` 与 `windows`（`amd64` / `arm64`）的 `aumbry` 与 `aumbry-server` 预编译压缩包。

**容器镜像**（`linux/amd64`、`linux/arm64`）：需本机已安装 Docker Engine、Docker Compose v2，以及一个可访问的私有 WebDAV 服务：

```sh
docker pull ghcr.io/nerdneilsfield/aumbry:latest
```

镜像以非特权用户 `nonroot`（UID/GID `65532`）运行，默认入口为 `aumbry-server`，同时将 `aumbry` 安装在 `/usr/local/bin/aumbry`。

**Homebrew**：同时安装 `aumbry` 与 `aumbry-server`：

```sh
brew install --cask nerdneilsfield/apps/aumbry
```

**Scoop（Windows）**：从同一个 bucket 安装两个可执行文件：

```powershell
scoop bucket add nerdneilsfield https://github.com/nerdneilsfield/homebrew-apps
scoop install nerdneilsfield/aumbry
```

**从源码编译**：需 Go `1.27.1` 及以上版本、Node.js `22` 及以上版本与 `npm`：

```sh
git clone https://github.com/nerdneilsfield/Aumbry.git
cd Aumbry
make setup
make build
```

编译完成后，仓库根目录会生成 `./aumbry-server` 与 `./aumbry`。

## 快速上手

下面步骤通过 `.env` 配合 Docker Compose 启动服务端，并用 CLI 本地校验示例报告。

> **前置条件：** 先准备好一个私有 WebDAV 服务，并提前用 `htpasswd -nbBC 10 '' '<your-password>' | cut -d: -f2` 生成管理员密码的 bcrypt 哈希。

1. 复制环境变量模板并填写全部必填项：

   ```sh
   cp .env.example .env
   chmod 600 .env
   ```

   `.env` 中的 `AUMBRY_ADMIN_PASSWORD_HASH` 必须用单引号包裹（如 `AUMBRY_ADMIN_PASSWORD_HASH='$2y$10$...'`），避免 `docker compose` 把 `$` 片段误解析为变量替换；`AUMBRY_DATA_DIR` 在容器中保持 `/data`。

2. 构建并启动服务：

   ```sh
   docker compose up -d --build
   ```

   （命令已对照 `docker-compose.yml` 与 `Dockerfile` 核对；请在运行 Docker 守护进程的环境执行。）

3. 检查存活探针与依赖就绪状态：

   ```sh
   curl -s http://127.0.0.1:8080/healthz
   curl -s http://127.0.0.1:8080/readyz
   ```

   正常响应如下：

   ```json
   {"status":"ok"}
   ```

   ```json
   {"checks":{"sqlite":"ok","webdav":"ok"},"status":"ok"}
   ```

4. 在本地离线校验仓库自带示例（未配置服务端也可运行）：

   ```sh
   ./aumbry validate examples/minimal-report.xml --format text
   ```

   预期输出：

   ```text
   valid: examples/minimal-report.xml
   ```

5. 在浏览器打开 `http://127.0.0.1:8080/admin`，使用管理员账号登录后点击 **新建根目录** 创建首个目录（记录其 12 位目录 ID，或随后通过 `aumbry dirs` 查看）。

若使用 TOML 挂载部署容器（`docker-compose.toml.yml`）、二进制直接运行、在线备份或离线清理，请参阅[部署与运维指南](docs/zh-CN/deployment.md)。在 Traefik、nginx 或 Caddy 后运行的 Compose 示例位于 [`examples/deploy/`](examples/deploy/)。

## 常见用法

**配置 CLI 并推送报告：**

> **警告：** `aumbry config set` 会把 Bearer `token` 以明文写入 `$XDG_CONFIG_HOME/aumbry/config.yaml`（默认 `~/.config/aumbry/config.yaml`，权限 `0600`）。在临时 CI 环境中请优先使用 `AUMBRY_SERVER` 与 `AUMBRY_TOKEN` 环境变量。

```sh
aumbry config set --server http://127.0.0.1:8080 --token <your-api-token>
aumbry dirs --format tree
aumbry push examples/minimal-report.xml --key demo:report --dir <dir-id>
```

`aumbry push` 实测输出示例：

```json
{
  "assets": 0,
  "created": true,
  "dirs": [
    "DRTxD0OSpDPZ"
  ],
  "draft": false,
  "full_url": "http://127.0.0.1:8080/r/MDUnpJpiAjMo",
  "id": "MDUnpJpiAjMo",
  "published": true,
  "type": "report",
  "url": "/r/MDUnpJpiAjMo"
}
```

**安装 Agent Skill**：

需要 Node.js `22.20` 及以上版本（使用 `vercel-labs/skills` 官方 CLI）。安装到当前用户的全局目录，供所有项目使用：

```sh
npx skills add nerdneilsfield/aumbry --skill aumbry -g
```

安装 Skill 后，还需确保 `aumbry` CLI 可执行文件位于 `PATH` 中（预编译 Release 压缩包或源码编译见[安装](#安装)）。指定目标 agent、非交互安装、离线或压缩包安装及目录选择规则见[Agent 与 Skill 集成指南](docs/zh-CN/agents-and-skill.md)。

## 配置摘要

服务端按 `--config` -> `AUMBRY_CONFIG` -> `./aumbry.toml` -> `/etc/aumbry/config.toml` 查找 TOML 文件，环境变量会覆盖同名 TOML 配置，未知 TOML 键在启动时直接报错。启动前可运行 `aumbry-server check-config` 验证脱敏后的生效配置。

| TOML 键 | 环境变量 | 必填 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- | :--- |
| `listen_addr` | `AUMBRY_LISTEN_ADDR` | 是（有默认值） | `":8080"` | HTTP 监听地址 |
| `data_dir` | `AUMBRY_DATA_DIR` | 是 | 无 | 保存 `aumbry.db`（WAL）的数据目录 |
| `admin.user` | `AUMBRY_ADMIN_USER` | 是 | 无 | 管理员登录用户名 |
| `admin.password_hash` | `AUMBRY_ADMIN_PASSWORD_HASH` | 是 | 无 | 管理员密码 bcrypt 哈希 |
| `api_tokens` | `AUMBRY_API_TOKENS` | 是 | 无 | 供 CLI/agent 使用的 Bearer token 列表 |
| `cookie_secret` | `AUMBRY_COOKIE_SECRET` | 是 | 无 | Cookie 签名密钥（不少于 32 字节） |
| `webdav.url` | `AUMBRY_WEBDAV_URL` | 是 | 无 | 私有 WebDAV 根 URL |
| `webdav.user` | `AUMBRY_WEBDAV_USER` | 是 | 无 | WebDAV 用户名 |
| `webdav.password` | `AUMBRY_WEBDAV_PASS` | 是 | 无 | WebDAV 密码 |
| `trusted_proxies` | `AUMBRY_TRUSTED_PROXIES` | 否 | `[]` | 可信反向代理 IP 或 CIDR 列表 |

完整字段约束、脱敏规则与容器 UID `65532` 权限配置见[服务端配置参考](docs/zh-CN/configuration.md)。

## 文档导航

| 文档 | 内容范围 |
| :--- | :--- |
| [部署与运维指南](docs/zh-CN/deployment.md) | Docker Compose（`.env` 与 TOML）、二进制运行、反向代理、WAL 安全备份、`verify-storage`、`gc` 与单实例限制 |
| [服务端配置参考](docs/zh-CN/configuration.md) | 配置查找顺序、环境变量覆盖规则、全部配置键类型与约束、`htpasswd` 转义、`check-config` |
| [目录与访问控制](docs/zh-CN/directories-and-access.md) | 目录树、密码设置/清除与继承、`protected` 与 `has_password` 差异、多目录可见性、未挂载草稿 |
| [CLI 使用与配置参考](docs/zh-CN/cli.md) | profile 与环境变量、配置优先级、`validate`/`push`/`dirs`/`list`/`unpublish`/`delete` 命令与实测输出 |
| [Agent 与 Skill 集成指南](docs/zh-CN/agents-and-skill.md) | Skill 安装、目标目录决策流程、草稿与发布授权约定、Token 与归属权 |
| [XML 协议入门](docs/zh-CN/protocol.md) | 协议定位、`1.1` 结构规则、`aumbry schema` 与完整参考入口 |

问题反馈与讨论见 GitHub 仓库（`github.com/nerdneilsfield/Aumbry`）。

## 安全与运维要点

- **严格单实例运行**：服务端对 SQLite 仅开放 1 条连接，并在进程内存中统计限流（密码解锁与管理员登录每 IP 每分钟 10 次；`/ingest` 与 `/agent/*` 每 IP 每分钟 120 次且同时最多 4 个 `/ingest` 上传请求）。同一 `data_dir` 与 WebDAV 只能启动一个实例。
- **反向代理与 Cookie**：`trusted_proxies` 默认为空，服务端忽略所有 `X-Forwarded-For` 与 `X-Forwarded-Proto`。配置代理网段后，按从右到左还原真实客户端 IP，并在到达 `X-Forwarded-Proto: https` 时为 Cookie（`aumbry_admin` 7 天、`aumbry_unlocked` 30 天，`HttpOnly`、`SameSite=Lax`）启用 `Secure` 标志。
- **在线备份与离线 GC**：请使用 `aumbry-server backup --out <path>`（通过 SQLite `VACUUM INTO` 在线生成一致快照），不要在 WAL 模式下直接复制 `aumbry.db`。执行破坏性 `aumbry-server gc` 前必须先停止服务端；`gc` 仅清理未被引用的 Aumbry 命名规则对象。

## 开发与验证

运行格式化检查、静态检查、测试与 Skill 示例校验：

```sh
make fmt-check
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
make skill-check
```

## 许可证

本项目基于 MIT 许可证开源，详情见 [LICENSE](LICENSE)。
