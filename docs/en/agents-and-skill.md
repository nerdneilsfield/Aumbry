English | [简体中文](../zh-CN/agents-and-skill.md)

# Agents and the Aumbry Skill

Aumbry includes an Agent Skill under [skills/aumbry/](../../skills/aumbry/SKILL.md) (also packaged inside GoReleaser release archives) that defines XML authoring rules, validation workflows, idempotency keys, and safe publishing boundaries for AI agents.

## Install the Skill

The repository places the skill at `skills/aumbry/SKILL.md`, which the official `vercel-labs/skills` CLI discovers automatically under `skills/`.

### Recommended: install with `npx skills`

Requires Node.js `22.20` or later.

1. Preview available skills in the repository without installing:

   ```sh
   npx skills add nerdneilsfield/aumbry --list
   ```

2. Install the `aumbry` skill (omit `-g` to install only for the current project, or pass `-g` for user-level global installation across projects):

   ```sh
   # Interactive user-level install (lets you pick detected agents)
   npx skills add nerdneilsfield/aumbry --skill aumbry -g

   # Non-interactive install for specific agents
   npx skills add nerdneilsfield/aumbry --skill aumbry -g -a claude-code -a codex -y
   ```

   Common options:
   - `--skill <name>`: select a specific skill (`aumbry`).
   - `-g` / `--global`: install to user-level directories instead of the current project.
   - `-a` / `--agent <agents...>`: target specific agents (for example `claude-code`, `codex`, `cursor`, `opencode`, `gemini-cli`, `github-copilot`).
   - `--list`: list discoverable skills in the source without installing.
   - `-y` / `--yes`: skip confirmation prompts (useful in scripts and CI).

   Default target directories installed by `npx skills` (symlinks to a canonical copy by default; pass `--copy` to copy files instead):

   | Agent (`-a` value) | Project scope (default) | Global scope (`-g`) |
   | :--- | :--- | :--- |
   | Codex, Cline, Warp, Zed (`codex`, ...) | `.agents/skills/` | `~/.agents/skills/` |
   | Claude Code (`claude-code`) | `.claude/skills/` | `~/.claude/skills/` |
   | Cursor (`cursor`) | `.agents/skills/` | `~/.cursor/skills/` |
   | OpenCode (`opencode`) | `.agents/skills/` | `~/.config/opencode/skills/` |
   | Gemini CLI (`gemini-cli`) | `.agents/skills/` | `~/.gemini/skills/` |
   | GitHub Copilot (`github-copilot`) | `.agents/skills/` | `~/.copilot/skills/` |

3. Ensure the `aumbry` CLI binary is installed on `PATH` and verify `aumbry version` succeeds. Installing the skill provides instructions only; the agent invokes the `aumbry` binary for validation, directory lookup, and uploads. Download prebuilt Release archives or build from source as described in [Installation](../../README.md#installation) and [Deployment prerequisites](deployment.md#prerequisites).

### Offline or release-archive installation

If you are working offline or from an unpacked GoReleaser archive (which includes `skills/aumbry/`), copy or symlink the folder directly into your agent's skill directory (for example `~/.agents/skills/aumbry` or `~/.claude/skills/aumbry`):

- **Copy from a checkout or unpacked Release archive** (for isolated environments or containers):

  ```sh
  mkdir -p ~/.agents/skills
  cp -R skills/aumbry ~/.agents/skills/aumbry
  ```

- **Symlink from a local checkout** (tracks `git pull` updates automatically):

  ```sh
  mkdir -p ~/.agents/skills
  ln -sfn "$(pwd)/skills/aumbry" ~/.agents/skills/aumbry
  ```

Verify that [SKILL.md](../../skills/aumbry/SKILL.md), `references/`, and `assets/examples/` exist inside the target directory, and ensure the `aumbry` CLI binary is available on `PATH`.

## Provision CLI credentials for an agent

Give each distinct agent role its own Bearer token inside `api_tokens` on the server so item ownership for `aumbry unpublish` and `aumbry delete` is scoped per token (see [Server configuration reference](configuration.md)). Remember that API tokens authenticate against `/api/v1/ingest` and `/api/v1/agent/*`; they have no relationship to directory passwords.

Choose a directory-default strategy based on the agent's scope:

- **Single-purpose agent** (for example a daily research digest job): set `AUMBRY_DIRS` (or `default_dirs` via `aumbry config set --profile digest-bot ... --dir <dir-id>`) so scheduled runs publish deterministically without prompting.
- **General-purpose agent** (interactive coding or paper-reading assistant): omit `AUMBRY_DIRS` and leave `default_dirs` empty so the agent queries `aumbry dirs`, matches against the user's request, and asks when unsure instead of silently publishing to an unintended directory.

## How an agent chooses the target directory

When pushing a published report (`--draft` is **not** set), the agent follows this precedence order:

1. **Explicit user target:** Use the directory specified by the user (matching name/path or ID from `aumbry dirs`, passed via `--dir <id>`).
2. **Configured default directory:** If the user did not name a directory, inspect `aumbry config show` for `default_dirs` (from `AUMBRY_DIRS` or the active profile). If present, `aumbry push` mounts those directories unless `--draft` is passed.
3. **Interactive discovery:** If neither is set, run `aumbry dirs --format json` (or `--format tree`) and match the user's topic against `path` / `name`.
4. **Ask before publishing when ambiguous or unmatched:**

> **Caution — Do not omit `--dir` when publishing:** If the user asks to publish, no default directory is configured, and `aumbry dirs` has no unique match, **do not run `aumbry push` without `--dir`**. Pushing with zero directories creates an unmounted item that returns HTTP `403` to visitors while leaving the user believing the report was published. Ask the user which directory ID to mount first.

5. **Draft exception (`--draft`):** `aumbry push --draft` always ignores `default_dirs` and `AUMBRY_DIRS` and saves the item unmounted (`dirs: []`).

## Authorization conventions for drafts vs publishing

| User wording | Required CLI action | Resulting state |
| :--- | :--- | :--- |
| "Write locally" / no push instruction | Run `aumbry validate` only; do not push | XML checked offline; server untouched |
| "Save to Aumbry" / "Store this" / "Save as draft" | Run `aumbry push <file> --key <k> --draft` | Unmounted draft (`403` to visitors; visible under `/admin`) |
| "Publish to `<dir>`" / "Publish" (with unambiguous target) | Run `aumbry push <file> --key <k> --dir <dir-id>` | Mounted and readable via `/d/<dir-id>` |
| Ambiguous request ("Save/push it") or missing target directory | Ask the user before calling `aumbry push` | Prevents accidental public exposure or unmounted `403` |

> **Caution — Check `protected` before publishing:** Inspect `"protected"` (not `"has_password"`) in `aumbry dirs` output before mounting internal notes. A child directory with `"has_password": false` and `"protected": false` is publicly accessible to anyone who receives its link.

## Connecting AI agents via MCP (`/mcp`)

In addition to invoking the `aumbry` CLI, MCP-capable agents can connect directly to the server's Streamable HTTP endpoint at `/mcp`:

- **Credentials and scoping:** Pass `Authorization: Bearer <api_token>` (plus optional `X-Aumbry-Dir-Tokens: ad_...,ad_...` for protected directory subtrees) or `Authorization: Bearer ad_<...>` for read-only access to a single directory subtree. Browser cookies are ignored on `/mcp`.
- **Available tools:** `list_dirs`, `list_items`, `search_items`, `get_item`, `validate_xml`, and `push_item`.
- **When to use CLI `aumbry push` instead of MCP `push_item`:** MCP `push_item` handles text-only XML and `v1.2` public `http(s)://` remote `<figure src>` URLs (`16 MiB` raw JSON-RPC request cap / `10 MiB` decoded XML cap). Use CLI `aumbry push` whenever uploading local relative binary assets (`.png`/`.jpg`/`.webp`/`.svg` figures, audio, or `<attachment type="pdf">`) or heavily JSON-escaped documents exceeding the `16 MiB` transport limit.
- **Deployment prerequisite:** Non-loopback clients connecting to `/mcp` require `public_url` on the server (see [Deployment guide](deployment.md)).
