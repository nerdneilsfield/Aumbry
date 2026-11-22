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
