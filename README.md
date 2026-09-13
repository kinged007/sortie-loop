# sortie-loop

One command to run autonomous coding-agent loops against any GitHub repo's issues.

## Install

```sh
git clone https://github.com/kinged007/sortie-loop.git
cd sortie-loop
./install.sh                    # -> ~/.local/bin/sortie-loop
```

`install.sh` builds the Go binary and links a `sortie` binary if one is on
`PATH` (`--prefix DIR` and `--sortie-bin PATH` overrides exist). The
workflows need sortie's `pi` agent and `github-pr` tracker, which no sortie
release ships yet (v1.24.0 lacks both) — until one does, build sortie from
source and point the loop at it:

```sh
git clone https://github.com/sortie-ai/sortie.git
cd sortie && go build -o ~/.local/bin/sortie ./cmd/sortie
```

Requires: go >= 1.24, git. `sortie-loop setup` also needs `gh`.

## Use (inside any repo)

```sh
sortie-loop setup    # writes .sortie/config.yaml, updates .gitignore, creates the labels
sortie-loop          # run plan + dev + review loops (Ctrl-C stops all)
```

`setup` detects `owner/name` from the git `origin` remote
(`--repo=owner/name` overrides). The loop re-detects it at every run, so
moving the checkout or forking needs no reconfiguration.

## Layout in the target repo

```
.sortie/
  config.yaml      # repo:, token:, milestone: (all optional — see below)
  .env.loop        # generated each run (resolved tracker, token, clone URL)
  workspaces/      # per-issue agent checkouts (dev/plan/review share one dir)
```

`.sortie/config.yaml`:

```yaml
repo: ""        # empty = detect from git remote; or pin owner/name
token: ""       # empty = GITHUB_TOKEN / GH_TOKEN env
milestone: ""   # optional milestone title to restrict all loops to
```

Env overrides: `SORTIE_LOOP_REPO`, `SORTIE_LOOP_TOKEN`,
`SORTIE_LOOP_MILESTONE` (plus `GH_MILESTONE` as a legacy alias).
Flags: `--no-server` disables the per-loop HTTP debug ports (7678-7680).

## Loops

Three sortie loops: `plan` writes an implementation plan as
an issue comment (a human removes `agent:plan-needed` to approve);
`dev` builds `agent:quick` issues onto main and `agent:build` issues via PR;
`review` reviews PRs labeled `agent:needs-review`. The workflow files live
in `cmd/sortie-loop/workflows/` (embedded into the binary); the label
scheme is documented in the old `.pi-workflows/ARCHITECTURE.md`, kept in
project history.

## Building

```sh
go build ./... && go vet ./...
```
