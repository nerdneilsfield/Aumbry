# Known issues

Problems that are understood and not fixed yet. Each entry says what happens, when you can hit it, and what to do about it.

## Search loads the full text of every result on the page

Searches that contain Chinese, Japanese or Korean text read the whole `plain_text` column of each result row, because the snippet is cut from the original text in Go (`internal/api/search.go`). A page is at most `limit` rows, so memory and I/O grow with `limit` times the report length. With long reports and a large `limit`, these searches are slower than plain Latin-script searches. Keep `limit` at its default if this matters.

## Previous and next links can fail while an item is being changed

The neighbor lookup (`internal/api/neighbors.go`) reads the window of ids in one query and the two neighbor summaries in separate queries, without a shared transaction. If a neighbor is deleted or republished between those queries, the request returns a 500 instead of a response without that neighbor. Reloading the page fixes it. The window is small, and only concurrent writes can trigger it.

## WebDAV requests ignore context cancellation

The WebDAV client library (`gowebdav`) does not take a `context`. A cancelled request keeps running until the transport timeouts end it (dial 5 s, response header 30 s). The `local` and `s3` backends honor cancellation.

## Superseded XML objects stay until `gc`

Updating an item writes a new content-hashed XML object and leaves the previous one in storage. Run `aumbry-server gc` (see [Deployment](deployment.md)) to remove them. This is intentional: deleting the old object right after the commit could race a concurrent ingest that reuses the same path.

## A missing S3 bucket can look like missing objects

`Open` and `Exists` map an HTTP 404 to "object not found". For `Open` (a GET), a missing bucket comes with a `NoSuchBucket` body and is reported as an unreachable store. `Exists` uses a HEAD request, which has no body, so a missing bucket cannot be told apart from a missing object, and `verify-storage` lists every object as missing. Nothing is deleted in this case. When `verify-storage` reports everything missing, check the endpoint and bucket name first.

## A FIFO in the local object directory blocks reads

`local` accepts any directory entry as an object. If a restored object directory contains a FIFO where an object should be, `Exists` returns true and reading that object blocks. Aumbry never creates FIFOs, so this only happens when files are placed by hand.

## `s3.path_style = false` alone does not trigger the `storage` check

When `storage` is unset, Aumbry fails to start if any `webdav`, `s3` or `r2` setting is present. An explicit `s3.path_style = false` counts as "not set" because `false` is also the default, so a config that contains only that line starts with the `local` backend.

## WebDAV `List` on a missing directory returns `ErrNotFound`

The other backends return an empty list for a directory that does not exist. The WebDAV backend returns `ErrNotFound`, and `gc` handles it. No command is affected.
