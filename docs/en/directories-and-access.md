English | [简体中文](../zh-CN/directories-and-access.md)

# Directories and access control

Aumbry organizes access around a directory tree instead of a public homepage or per-item passwords. The root URL (`/`) shows a static entry page with a link to **管理员入口** (`/admin`), so visitors can browse only the directory (`/d/<dir-id>`) or item (`/r/<id>`, `/p/<id>`) links you share with them.

## Visibility rules at a glance

- **Unmounted items (drafts):** An item mounted in zero directories returns HTTP `403 forbidden` to every visitor on metadata, XML content, and item assets. Admins still see unmounted items under **未挂载条目** in `/admin`.
- **Single-directory inheritance:** Setting a password on a directory protects that directory and all of its descendants automatically.
- **No per-item passwords:** Passwords exist only on directories; Aumbry has no standalone per-report password feature.
- **No scheduled unlocks:** Directory unlock state persists in the visitor's encrypted, signed `aumbry_unlocked` cookie (`30 days` TTL) until the cookie expires or the directory password changes. There is no timed or scheduled unlock window.

## Create directories and manage passwords

### Via the Admin console (`/admin`)

1. Open `/admin` in your browser and sign in with `admin.user` and your admin password.
2. To create a top-level directory, select **新建根目录**, enter a **名称** (and optional **描述**), leave **访问密码** empty for a public directory or enter a password to protect the subtree, then select **保存**.
3. To create a child directory, select the parent directory in the left tree, select **子目录**, complete the form, and select **保存**.
4. To set, replace, or remove a password on an existing directory:

   > **Caution — Session invalidation:** Setting, changing, or clearing a directory password computes a new password hash fingerprint (`Fingerprint`). Every visitor `aumbry_unlocked` cookie previously issued for that directory stops matching immediately, requiring visitors to unlock with the new password (or reloading freely if cleared).

   1. Select the target directory in the left tree.
   2. Select **密码** (`DirDialog` password mode).
   3. To set or rotate the password, enter **新密码** and select **保存**. To remove protection, select **清除密码** (shown when `has_password` is true).

   Expected result: The directory lock badge updates in the admin tree and unlocked subtrees become immediately accessible or locked according to the new password state.

### Via the Admin HTTP API

Authenticate first (`POST /api/v1/admin/login`), then manage directory passwords programmatically (requests below use cookie jar `admin.jar`):

1. Create a directory with an initial password (`parent_id` optional; omit or set `null` for root):

   ```sh
   curl -s -b admin.jar -H "Content-Type: application/json" \
     -d '{"name":"Team Private","description":"Internal team notes","password":"team-secret"}' \
     http://127.0.0.1:8080/api/v1/admin/dirs
   ```

   Expected response (HTTP `201`):

   ```json
   {"id":"4ho9VKNPRj2h"}
   ```

2. Set or change the password on an existing directory (`PATCH /api/v1/admin/dirs/{id}`):

   ```sh
   curl -i -b admin.jar -X PATCH -H "Content-Type: application/json" \
     -d '{"password":"new-password"}' \
     http://127.0.0.1:8080/api/v1/admin/dirs/4ho9VKNPRj2h
   ```

   Expected response: HTTP `204 No Content`.

3. Clear the password by passing an empty string `""` (`password_hash = NULL` in SQLite):

   ```sh
   curl -i -b admin.jar -X PATCH -H "Content-Type: application/json" \
     -d '{"password":""}' \
     http://127.0.0.1:8080/api/v1/admin/dirs/4ho9VKNPRj2h
   ```

   Expected response: HTTP `204 No Content`.

## Inheritance and `protected` vs `has_password`

Permission evaluation walks upward from the requested directory to the tree root (`internal/api/perm.go`):

1. A directory is **accessible** to a visitor only if no ancestor (including itself) has a password, **or** the visitor holds a valid unlock entry whose hash fingerprint matches any ancestor (including itself).
2. Opening `GET /api/v1/dirs/{id}` or unlocking via `POST /api/v1/dirs/{id}/unlock` checks the **nearest protected ancestor (or the directory itself)**. Unlocking an ancestor unlocks its entire subtree (`accessible` stops walking upward at the first unlocked ancestor). When a child directory defines its own password while the parent remains locked (or unlocked separately), calling `/unlock` on the child verifies that child's own password and unlocks only the child subtree without unlocking the parent.
3. When viewing a public parent directory that contains a locked child directory:
   - The child appears in `children` with `"locked": true` and an empty `"description": ""`.
   - Directory search (`GET /api/v1/dirs/{id}/search`) searches only the unlocked subset (`t.visibleDirs(v, t.subtree(d.ID))`), hiding items inside locked child directories until unlocked.
4. Visitor unlock example against a child directory `Weekly Briefs` (`50h0cpGGYRkM`) under password-protected `Team Private` (`4ho9VKNPRj2h`):

   ```sh
   curl -s http://127.0.0.1:8080/api/v1/dirs/50h0cpGGYRkM
   ```

   Expected response (HTTP `401`):

   ```json
   {"dir_name":"Weekly Briefs","error":"locked","locked":true,"message":"this directory is password protected"}
   ```

   Unlock with the ancestor password:

   ```sh
   curl -s -c vis.jar -H "Content-Type: application/json" \
     -d '{"password":"team-secret"}' \
     http://127.0.0.1:8080/api/v1/dirs/50h0cpGGYRkM/unlock
   ```

   Expected response (HTTP `200`, sets `aumbry_unlocked` cookie; limited to `10 requests/min` per IP):

   ```json
   {"ok":true}
   ```

### Difference between `protected` and `has_password`

`aumbry dirs` returns both boolean fields on every directory. Do not rely on `has_password` alone to decide whether a directory is public:

| Field | Scope | Value for a child under a locked parent (no password of its own) | When to inspect |
| :--- | :--- | :--- | :--- |
| `has_password` | Direct setting on this node only | `false` | Admin UI showing whether **清除密码** applies to this node |
| `protected` | Effective access status across node + ancestors | `true` | Checking whether visitors need a password to open the directory |

Example output from `aumbry dirs --format tree`:

```text
- Public Reports (id=DRTxD0OSpDPZ, items=0, public)
- Team Private (id=4ho9VKNPRj2h, items=0, password)
  - Weekly Briefs (id=50h0cpGGYRkM, items=0, protected)
```

## Multi-directory mounting and item visibility

An item can be mounted in multiple directories at the same time (via `aumbry push --dir A --dir B` or **保存挂载** in `/admin` `ItemDialog`).

> **Caution — Mounting to any public directory exposes the item:** Visitor access (`canViewItem` in `internal/api/items.go`) uses **OR** semantics: a visitor can read an item and its assets if **at least one** of its mounted directories is accessible (`len(t.visibleDirs(v, mounts)) > 0`). Mounting a protected item into an additional public directory makes the item readable to anyone who holds the item link.

Behavior when viewing a multi-mounted item:

- **Visible mounts only:** `GET /api/v1/items/{id}` lists only the subset of mounted directories the current visitor is allowed to open (`dirs`). Locked directories stay omitted so visitors cannot discover private directory names or IDs.
- **Related items filtering:** Each `<related-ids>/<id>` returned in `related` is filtered through `canViewItem`; items mounted exclusively in locked directories or unmounted drafts are omitted.
- **Assets follow items:** `GET /api/v1/assets/{hash}` allows access whenever any item referencing that asset hash is readable by the caller.
- **Unmounted items (drafts) return `403`:** Deleting a directory removes mounts on that directory (cascading to child directories) without deleting items; any item left with zero mounts immediately returns HTTP `403 forbidden` to visitors:

  ```json
  {"error":"forbidden","message":"this item is not accessible"}
  ```
