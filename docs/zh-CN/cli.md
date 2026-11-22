[English](../en/cli.md) | 简体中文

# CLI 使用与配置参考

`aumbry` CLI 负责本地 XML 离线校验、推送报告与图片、查询服务端目录和条目，以及撤回或删除当前 token 名下的条目。CLI 认证使用服务端 `api_tokens` 列表里的一个 Bearer token；它只控制推送与 agent API 权限，与目录密码及管理员 Web 登录无关。

## 认证与配置方式

根据运行环境选择纯环境变量或本地 YAML profile 文件：

> **警告：Token 明文存储。** 执行 `aumbry config set` 时，CLI 会把 Bearer `token` 以明文写入 `$XDG_CONFIG_HOME/aumbry/config.yaml`（未设置 `XDG_CONFIG_HOME` 时各平台均为 `~/.config/aumbry/config.yaml`）。程序在保存时以 `0700` 创建父目录，并在每次保存时将配置文件权限设为 `0600`。请妥善保管该文件，避免纳入版本库；临时 CI 或容器环境改用环境变量传入。

### 方式 A：环境变量

当未选中或指定不存在的 profile 时，仅凭环境变量即可运行远程命令（无需配置文件）：

```sh
export AUMBRY_SERVER="http://127.0.0.1:8080"
export AUMBRY_TOKEN="amb_demo_token_abcd1234"
export AUMBRY_DIRS="DRTxD0OSpDPZ"
```

`AUMBRY_DIRS` 支持以英文逗号分隔多个目录 ID，各条目首尾空白会自动裁剪。

### 方式 B：`aumbry config set` 与 profile

首次执行未指定 `--profile` 时默认写入 `default` profile；只更新命令中显式传入的参数：

```sh
aumbry config set --server http://127.0.0.1:8080 --token amb_demo_token_abcd1234 --dir DRTxD0OSpDPZ
```

预期输出：

```text
profile "default" saved
```

查看当前生效配置（`token` 自动脱敏为末尾 4 位）：

```sh
aumbry config show
```

实测输出：

```yaml
profile: default
server: http://127.0.0.1:8080
token: '****1234'
default_dirs:
    - DRTxD0OSpDPZ
```

需要对接多套环境时，创建并切换命名 profile（`--dir` 可多次传入；传入后会整体替换该 profile 的 `default_dirs`）：

```sh
aumbry config set --profile staging --server https://staging.aumbry.example --token amb_staging_9999 --dir 4ho9VKNPRj2h
aumbry config use staging
```

### 配置覆盖优先级

所有远程命令（`push`、`dirs`、`list`、`unpublish`、`delete`）按以下优先级解析连接参数：

```text
CLI 命令行参数（--dir、--profile） > 环境变量（AUMBRY_SERVER、AUMBRY_TOKEN、AUMBRY_DIRS） > 当前选中 profile
```

`--profile <name>` 仅对当次命令生效，不修改 `active_profile`，且环境变量仍会覆盖该 profile 内的对应字段；若指定了配置文件中不存在的 profile 名称，命令直接报错退出（退出码 `1`）。

## 全局参数与退出码

- `-v` / `--verbose`：将调试日志输出到 `stderr`。
- 退出码 `0`：命令成功执行（或文档通过校验）。
- 退出码 `1`：参数冲突、本地/服务端校验失败、网络异常或服务端返回错误状态码。

## 命令参考

### `aumbry validate`

完全离线校验 Aumbry XML（`version="1.0"` 或 `"1.1"`）。`<figure src>` 按 XML 文件自身所在目录解析（而非当前工作目录），并拒绝指向该目录之外的符号链接。

```sh
aumbry validate <file.xml> [--format json|text]
```

默认 JSON 输出：

```sh
aumbry validate examples/minimal-report.xml
```

```json
{
  "valid": true,
  "file": "examples/minimal-report.xml",
  "errors": []
}
```

可读文本输出：

```sh
aumbry validate examples/minimal-report.xml --format text
```

```text
valid: examples/minimal-report.xml
```

### `aumbry push`

先在本地完成与服务端同源的 XML 与资产校验，再打包上传到 `POST /api/v1/ingest`。

```sh
aumbry push <file.xml> [--dir <dir-id>]... [--key <external_key>] [--id <item-id>] [--draft] [--create-only] [--dry-run] [--profile <name>]
```

常用参数与互斥关系：

- `--dir <id>`：指定挂载目录 ID（可多次传入）；一旦显式传入，覆盖配置中的 `default_dirs` 与 `AUMBRY_DIRS`。
- `--draft`：作为未挂载草稿上传（`dirs: []`），**自动忽略** `default_dirs` 与 `AUMBRY_DIRS`。该参数与 `--dir` 互斥；若目标条目已挂载在任意目录，服务端返回 `409 already_published`，须先执行 `aumbry unpublish <id>` 再保存草稿。
- `--create-only`：仅在 `external_key` 尚不存在时创建，已存在返回 `409 key_exists`。必须搭配 `--key`，不可与 `--id` 同用。
- `--dry-run`：执行完整的本地 XML 与图片检查，并请求 `GET /api/v1/agent/dirs` 核对服务端可达性、token 合法性与目标目录是否存在，但不上传任何对象。
- `--key <external_key>`：幂等键（新键去除首尾空白后须为 `1` 到 `200` 个 Unicode 字符且不含控制字符）。同一篇报告多轮修订复用同一个 `--key`。
- `--id <item-id>`：按服务端 12 位条目 ID 定向更新已有条目。

> **注意：无默认目录且不传 `--dir` 推送的后果。** 若未设置 `default_dirs` / `AUMBRY_DIRS`，也未显式传 `--dir`（且未带 `--draft`），推送会把条目保存为挂载列表为空的状态，访客无法读取（HTTP `403`）。推送发布前务必确认目标目录 ID。

预检示例（`--dry-run`）：

```sh
aumbry push examples/minimal-report.xml --key demo:report --dir DRTxD0OSpDPZ --dry-run
```

```json
{
  "assets": 0,
  "create_only": false,
  "dirs": [
    "DRTxD0OSpDPZ"
  ],
  "draft": false,
  "dry_run": true,
  "file": "examples/minimal-report.xml",
  "type": "report",
  "valid": true
}
```

保存为草稿（`--draft`）：

```sh
aumbry push examples/minimal-report.xml --key demo:draft --draft
```

```json
{
  "assets": 0,
  "created": true,
  "dirs": [],
  "draft": true,
  "full_url": "http://127.0.0.1:8080/r/VFlSwVxYYaCN",
  "id": "VFlSwVxYYaCN",
  "published": false,
  "type": "report",
  "url": "/r/VFlSwVxYYaCN"
}
```

防覆盖首次创建（`--create-only --key`）与按 `--id` 更新：

```sh
aumbry push examples/minimal-report.xml --key demo:report --create-only --dir DRTxD0OSpDPZ
```

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

```sh
aumbry push examples/minimal-report.xml --id MDUnpJpiAjMo --dir DRTxD0OSpDPZ
```

```json
{
  "assets": 0,
  "created": false,
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

### `aumbry dirs`

列出当前 API token 可见的目录树（`GET /api/v1/agent/dirs`）。判断目录是否对访客公开时查看 `protected` 字段（而不是 `has_password`）。

```sh
aumbry dirs [--format json|tree] [--profile <name>]
```

JSON 实测输出：

```sh
aumbry dirs
```

```json
[
  {
    "id": "DRTxD0OSpDPZ",
    "name": "Public Reports",
    "path": "Public Reports",
    "parent_id": null,
    "has_password": false,
    "protected": false,
    "item_count": 0
  },
  {
    "id": "4ho9VKNPRj2h",
    "name": "Team Private",
    "path": "Team Private",
    "parent_id": null,
    "has_password": true,
    "protected": true,
    "item_count": 0
  },
  {
    "id": "50h0cpGGYRkM",
    "name": "Weekly Briefs",
    "path": "Team Private / Weekly Briefs",
    "parent_id": "4ho9VKNPRj2h",
    "has_password": false,
    "protected": true,
    "item_count": 0
  }
]
```

树状实测输出：

```sh
aumbry dirs --format tree
```

```text
- Public Reports (id=DRTxD0OSpDPZ, items=0, public)
- Team Private (id=4ho9VKNPRj2h, items=0, password)
  - Weekly Briefs (id=50h0cpGGYRkM, items=0, protected)
```

### `aumbry list`

查询条目列表（`GET /api/v1/agent/items`）。`--key` 精确匹配 `external_key`，`--q` 仅对 `title` 与 `external_key` 做子串匹配（不检索正文内容），`--dir` 按挂载目录 ID 过滤，`--limit` / `--offset` 分页（服务端默认 `20`、上限 `100`）。返回中的 `"owned": true` 表示该条目由当前 token 创建。

```sh
aumbry list [--key <key>] [--q <query>] [--dir <dir-id>] [--limit <n>] [--offset <n>] [--profile <name>]
```

实测示例：

```sh
aumbry list --key demo:report --limit 20
```

```json
{
  "items": [
    {
      "id": "MDUnpJpiAjMo",
      "type": "report",
      "title": "Aumbry example report",
      "date": "2026-11-20",
      "external_key": "demo:report",
      "url": "/r/MDUnpJpiAjMo",
      "dirs": [
        "DRTxD0OSpDPZ"
      ],
      "owned": true
    }
  ],
  "limit": 20,
  "offset": 0
}
```

### `aumbry unpublish`

清空指定条目的全部目录挂载（`DELETE /api/v1/agent/items/{id}/mounts`），使其退回草稿状态（访客读取返回 `403`），条目 ID、元数据与 WebDAV XML 正文保留。仅允许创建该条目的 token 执行（其他 token 及 v1.0 遗留的无归属记录条目返回 HTTP `403 not_owner`）。

```sh
aumbry unpublish MDUnpJpiAjMo
```

```json
{
  "id": "MDUnpJpiAjMo",
  "published": false
}
```

### `aumbry delete`

> **注意：不可恢复删除。** `aumbry delete <id> --yes` 永久删除该条目的元数据、全文搜索索引、目录挂载记录以及 WebDAV 中的 XML 正文对象（`DELETE /api/v1/agent/items/{id}`），操作无法撤销。未传入 `--yes` 时 CLI 在本地直接拒绝执行；同样仅创建者 token 有权删除（非创建者返回 `403 not_owner`）。

```sh
aumbry delete MDUnpJpiAjMo --yes
```

```json
{
  "deleted": true,
  "id": "MDUnpJpiAjMo"
}
```

### `aumbry schema` 与 `aumbry version`

- `aumbry schema [--format md|json]`：输出内置 XML 协议完整规范（默认为 `md`）。
- `aumbry version`：输出版本号、提交哈希、构建时间、Go 版本及平台架构：

  ```sh
  aumbry version
  ```

  ```text
  aumbry dev
  commit: none
  built: unknown
  go: go1.27.4 linux/amd64
  ```
