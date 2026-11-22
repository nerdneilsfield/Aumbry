English | [简体中文](../zh-CN/agents-and-skill.md)

# Agents and the Aumbry Skill

Aumbry includes an Agent Skill under [skills/aumbry/](../../skills/aumbry/SKILL.md) (also packaged inside GoReleaser release archives) that defines XML authoring rules, validation workflows, idempotency keys, and safe publishing boundaries for AI agents.

## Install the Skill

Link or copy `skills/aumbry` into your agent runner's skills directory (for example `~/.agents/skills/aumbry`):

1. **Symlink from a local checkout** (tracks `git pull` updates automatically):

   ```sh
   mkdir -p ~/.agents/skills
   ln -sfn "$(pwd)/skills/aumbry" ~/.agents/skills/aumbry
   ```

2. **Copy from a checkout or unpacked release archive** (for standalone agent containers):

   ```sh
   mkdir -p ~/.agents/skills
   cp -R skills/aumbry ~/.agents/skills/aumbry
   ```

3. Verify that [SKILL.md](../../skills/aumbry/SKILL.md), `references/`, and `assets/examples/` exist under `~/.agents/skills/aumbry`.

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
