[English](../en/directories-and-access.md) | 简体中文

# 目录与访问控制

Aumbry 以目录树组织所有访问权限，不提供公开全站首页索引，也不支持单篇条目独立设密。网站首页（`/`）仅显示提示页和 **管理员入口**（`/admin`），访客只能打开你分享给他们的目录链接（`/d/<dir-id>`）或条目链接（`/r/<id>`、`/p/<id>`）。

## 权限模型要点

- **未挂载条目（草稿）不可被访客读取**：未挂载到任何目录的条目，访客访问其元数据、XML 正文或关联图片均返回 HTTP `403 forbidden`。管理员仍可在 `/admin` 的 **未挂载条目** 分类下查看。
- **目录树向下继承密码**：为父目录设置密码后，其下所有后代子目录自动受密码保护。
- **不支持单篇条目单独密码**：密码仅绑定在目录上。
- **不支持定时或自动解锁**：访客输入目录密码通过校验后，服务端签发加密签名 Cookie `aumbry_unlocked`（有效期 `30` 天），在 Cookie 过期或目录密码变更/清除前持续有效，不设定时开放窗口。

## 创建目录与管理密码

### 通过管理控制台（`/admin`）

1. 在浏览器打开 `/admin`，使用配置中的 `admin.user` 与管理员密码登录。
2. 创建根目录：点击 **新建根目录**，填写 **名称** 与可选 **描述**；如需设密则填写 **访问密码**，公开目录留空，最后点击 **保存**。
3. 创建子目录：在左侧目录树选中目标父目录，点击当前目录操作栏中的 **子目录**，填写名称、描述与可选密码后点击 **保存**。
4. 为已有目录设置、修改或清除密码：

   > **注意：会话即时失效。** 设置、修改或清除目录密码后，此前使用旧密码签发给访客的 `aumbry_unlocked` Cookie 会立即失效；访客需重新输入新密码，或在清除密码后直接访问公开目录。

   1. 在左侧目录树选中目标目录。
   2. 点击工具栏中的 **密码** 按钮。
   3. 设置或更换密码时，在 **新密码** 输入框填写密码后点击 **保存**；若当前已设密码且要取消保护，点击弹窗左下角的 **清除密码**。

   预期结果：管理树中该目录的锁图标状态实时更新，目录及其未单独设密的子目录按新状态生效。

### 通过管理员 HTTP API

先调用 `POST /api/v1/admin/login` 获取 `admin.jar` 会话 Cookie，再执行以下请求：

1. 新建目录并附带密码（根目录下可省略 `parent_id` 或传 `null`）：

   ```sh
   curl -s -b admin.jar -H "Content-Type: application/json" \
     -d '{"name":"Team Private","description":"Internal team notes","password":"team-secret"}' \
     http://127.0.0.1:8080/api/v1/admin/dirs
   ```

   实测响应（HTTP `201`）：

   ```json
   {"id":"4ho9VKNPRj2h"}
   ```

2. 设置或修改已有目录密码（`PATCH /api/v1/admin/dirs/{id}`）：

   ```sh
   curl -i -b admin.jar -X PATCH -H "Content-Type: application/json" \
     -d '{"password":"new-password"}' \
     http://127.0.0.1:8080/api/v1/admin/dirs/4ho9VKNPRj2h
   ```

   实测响应：HTTP `204 No Content`。

3. 清除目录密码时向同一接口传入空字符串 `""`：

   ```sh
   curl -i -b admin.jar -X PATCH -H "Content-Type: application/json" \
     -d '{"password":""}' \
     http://127.0.0.1:8080/api/v1/admin/dirs/4ho9VKNPRj2h
   ```

   实测响应：HTTP `204 No Content`。

## 子目录继承与 `protected` / `has_password` 的区别

访问权限检查从目标目录一路向上追溯到根节点：

1. 当且仅当路径上所有祖先（含自身）均未设密码，**或者**访客的 `aumbry_unlocked` Cookie 中持有路径上任一受保护节点（含自身）当前有效的解锁凭证时，该目录对访客可见。
2. 访客访问 `GET /api/v1/dirs/{id}` 或提交 `POST /api/v1/dirs/{id}/unlock` 时，服务端校验从该目录向上最近一个设有密码的节点（含自身）：若父目录设密、子目录未单独设密，则用父目录密码解锁并放行父目录及其子树；若子目录自己另设了密码，在未解锁父目录的情况下输入子目录密码仅解锁该子目录，不会顺带解锁父目录。
3. 公开父目录下存在未解锁的加密子目录时：
   - 父目录响应的 `children` 仍列出该子目录，但标记 `"locked": true` 且将 `"description"` 置空 `""`；前端目录卡片显示锁图标。
   - 在父目录内搜索（`GET /api/v1/dirs/{id}/search`）仅检索已解锁的子树，未解锁子目录内的条目不会出现在搜索结果中。
4. 实测受保护父目录 `Team Private`（`4ho9VKNPRj2h`）下的未设密子目录 `Weekly Briefs`（`50h0cpGGYRkM`）访问与解锁过程：

   ```sh
   curl -s http://127.0.0.1:8080/api/v1/dirs/50h0cpGGYRkM
   ```

   未解锁时的实测响应（HTTP `401`）：

   ```json
   {"dir_name":"Weekly Briefs","error":"locked","locked":true,"message":"this directory is password protected"}
   ```

   提交密码解锁（每 IP 每分钟限流 `10` 次）：

   ```sh
   curl -s -c vis.jar -H "Content-Type: application/json" \
     -d '{"password":"team-secret"}' \
     http://127.0.0.1:8080/api/v1/dirs/50h0cpGGYRkM/unlock
   ```

   解锁成功响应（HTTP `200`，签发 `aumbry_unlocked` Cookie）：

   ```json
   {"ok":true}
   ```

### `protected` 与 `has_password` 对比

`aumbry dirs` 对每个目录同时返回两个布尔字段。判断目录对访客是否公开时，始终以 `protected` 为准：

| 字段 | 语义范围 | 父目录加密、子目录未设密码时的取值 | 适用场景 |
| :--- | :--- | :--- | :--- |
| `has_password` | 仅表示当前节点自身是否直接设置了密码 | `false` | 管理界面判断是否显示 **清除密码** 按钮 |
| `protected` | 表示当前节点或其任意祖先是否受密码保护 | `true` | 判断访客打开该目录时是否需要解锁 |

`aumbry dirs --format tree` 实测输出直观反映了三者区分：

```text
- Public Reports (id=DRTxD0OSpDPZ, items=0, public)
- Team Private (id=4ho9VKNPRj2h, items=0, password)
  - Weekly Briefs (id=50h0cpGGYRkM, items=0, protected)
```

## 多目录挂载与条目可见性

同一条目可以同时挂载到多个目录（通过多次传递 `aumbry push --dir` 或在 `/admin` 的条目详情点击 **保存挂载**）。

> **注意：挂载到任意公开目录即等同公开该条目。** 访客对条目的读取权限采用“任一可见”规则：只要条目所挂载的目录中**至少有一个**对当前访客可见，访客持有条目链接就能阅读正文和图片。把原本只在加密目录里的条目再挂进公开目录，会让知道条目链接的任何人都能读取。

多目录挂载时的细粒度过滤行为：

- **只返回访客可见的挂载路径**：`GET /api/v1/items/{id}` 响应的 `dirs` 面包屑列表只保留当前访客有权打开的目录，避免通过条目详情泄露私有目录名称或 ID。
- **相关条目自动过滤**：条目的 `<related-ids>` 会按当前访客的读取权限逐一过滤；位于未解锁目录或处于未挂载状态的关联条目不会出现在访客响应中。
- **图片资源继承条目可见性**：`GET /api/v1/assets/{hash}` 在任一引用该图片哈希的条目对访客可见时允许读取。
- **未挂载条目（草稿）返回 `403`**：删除目录时会级联移除该目录及子目录的挂载记录，但不会删除条目本身；当某个条目挂载数降为 0 时，访客访问立即返回 HTTP `403 forbidden`：

  ```json
  {"error":"forbidden","message":"this item is not accessible"}
  ```

## 目录订阅源、只读 Token 与修订隐私边界

- **RSS / Podcast 订阅源**：每个目录提供 `GET /feeds/{dir_id}/rss.xml` 与 `GET /feeds/{dir_id}/podcast.xml`（返回最多 50 条可见条目）。公开目录可匿名访问，enclosure 直指 `/api/v1/assets/{hash}`；受保护目录**不接受**浏览器管理员或访客 Cookie，必须携带经 `/admin` 为该目录签发的只读 Token（`?token=ad_...`），此时响应携带 `Cache-Control: private, no-store`，且音频 enclosure 经由 `/feeds/{dir_id}/assets/{hash}?token=...` 代理交付。
- **不穿透子锁与凭证解耦**：针对目录 `D` 签发的 `ad_` Token 可以读取 `D` 及其未单独设密的子目录，但**不会穿透**下级独立设密的子目录 `C`（需为 `C` 另行签发 Token）。修改目录的访客密码不会吊销已签发的 `ad_` Token；如需撤销订阅权，可在 `/admin` 执行**轮换**或**吊销**。
- **历史修订双门隔离**：阅读历史快照（`GET /api/v1/items/{id}/revisions` 与 `?rev=N`）及其独占历史资产遵循双门检查——非管理员读取第 `R` 版时，条目当前必须可见，且第 `R` 版发布时的挂载目录集合中至少有一项对当前调用者仍可见（曾发布于私有目录、日后脱敏公开重发的条目，其旧私密版不会随现行条目一并公开；后续即使管理员清空该旧私有目录的密码，曾在该目录受保护期间发布的历史快照也不会因此变成匿名公开可读）。
- **反向引用过滤**：`GET /api/v1/items/{id}` 返回的 `referenced_by` 同样按调用者权限过滤并过滤掉未挂载草稿，不会泄露受保护简报的标题与 ID。

## MCP 端点（`/mcp`）身份与权限矩阵

`/mcp` 仅面向 Agent 与 CLI 生态，**完全忽略浏览器管理员 Cookie 与访客解锁 Cookie**，并且每次 HTTP 请求及每次工具调用都从当前 SQLite 状态实时重算权限（吊销 `ad_` Token 后同会话下一条调用立即失效）：

| 调用凭据 | 可见目录与条目范围 | `push_item` 写入 |
| :--- | :--- | :--- |
| **无凭据（匿名）** | 仅公开目录及公开目录内已发布条目与符合 B1 的公开历史修订（`list_dirs` 对未解锁子目录标 `locked: true` 且清空描述） | 拒绝（工具错误 `unauthorized`） |
| **`Bearer ad_<...>`** | 仅限该目录 Token 按不穿透子锁规则（A1）授权的子树及其中已发布条目/可见历史修订 | 拒绝（工具错误 `forbidden`） |
| **`Bearer <api_token>`**（可选 `X-Aumbry-Dir-Tokens: ad_...,ad_...`） | 公开目录 + `X-Aumbry-Dir-Tokens` 授权子树 + **自己创建（`created_by == digest`）** 的条目与修订（含草稿与自建加密条目）；传入 `dir_id` 时严格按授权集合求交；**不可读其他 Token 的草稿或未授权加密条目** | **唯一允许身份**（拒绝含本地相对路径资源的 XML） |
