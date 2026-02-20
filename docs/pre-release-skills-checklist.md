# Pre-Release Skills Checklist

Use this checklist before cutting a Lisa release tag.

Goal: ensure `skills/lisa/SKILL.md` in repo is current and install flows propagate the exact file.

## 1) Build Local CLI

```bash
go build -o lisa .
./lisa version
```

## 2) Sync Skill Into Repo (Canonical Source)

If Codex is canonical for this release:

```bash
./lisa skills sync --from codex --repo-root "$(pwd)" --json
```

If Claude is canonical instead, use:

```bash
./lisa skills sync --from claude --repo-root "$(pwd)" --json
```

## 3) Confirm Repo Skill Matches Source

```bash
diff -u ~/.codex/skills/lisa/SKILL.md skills/lisa/SKILL.md
```

If Claude is used in your flow, align it too:

```bash
./lisa skills install --to claude --repo-root "$(pwd)" --json
```

## 4) Validate Project Install Propagation

```bash
TMP_PROJECT="$(mktemp -d /tmp/lisa-skill-check-XXXXXX)"
./lisa skills install --to project --project-path "$TMP_PROJECT" --repo-root "$(pwd)" --json
shasum -a 256 skills/lisa/SKILL.md "$TMP_PROJECT/skills/lisa/SKILL.md"
```

Expected: both hashes are identical.

## 5) Pre-Tag Git Gate

```bash
git status --short
```

Expected before tagging:
- `skills/lisa/SKILL.md` is committed if changed
- docs updates are committed if process changed
- working tree clean

## 6) Post-Tag Remote Sanity (Optional, Recommended)

After pushing tag `vX.Y.Z`:

```bash
TAG="vX.Y.Z"
curl -fsSL "https://raw.githubusercontent.com/bma-d/lisa/$TAG/skills/lisa/SKILL.md" >/tmp/lisa-skill-tag.md
diff -u skills/lisa/SKILL.md /tmp/lisa-skill-tag.md
```

Expected: no diff.
