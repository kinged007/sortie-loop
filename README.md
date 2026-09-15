# sortie-loop

One command to run autonomous coding-agent loops against any GitHub repo's issues.
GitHub is the only supported tracker: workflows use the `github` and
`github-pr` tracker kinds, setup syncs labels with the `gh` CLI, and the
prompts shell out to `gh` throughout. No other tracker is configured to work.

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
sortie-loop          # run every WORKFLOW.*.md loop (Ctrl-C stops all)
```

`setup` detects `owner/name` from the git `origin` remote
(`--repo=owner/name` overrides). The loop re-detects it at every run, so
moving the checkout or forking needs no reconfiguration.

## Layout in the target repo

```
.sortie/
  config.yaml      # repo:, token:, milestone: (all optional — see below)
  workflows/       # installed by setup from the binary; edit freely, loop runs these
  .env.loop        # generated each run (resolved tracker, token, clone URL)
  workspaces/      # per-item agent checkouts (one subdir per loop)
```

`.sortie/config.yaml`:

```yaml
repo: ""        # empty = detect from git remote; or pin owner/name
token: ""       # empty = GITHUB_TOKEN / GH_TOKEN env
milestone: ""   # optional milestone title to restrict all loops to
#assignee: ""   # unset = @me (token owner only); "" = shared backlog
# per-loop query-filter overrides, keyed by WORKFLOW.*.md stem
# (read on every run — no setup re-run needed):
#filters:
#  triage: "label:agent:triage -label:needs-human"
# GitHub labels setup ensures exist (create-or-edit; others left alone).
# A custom loop adds its labels here, then setup is re-run to create them.
labels:
- {name: "agent:quick", color: "fbca04", description: "Track: small change, merged to base, no PR"}
# ... (full default list seeded by setup)
```

Env overrides: `SORTIE_LOOP_REPO`, `SORTIE_LOOP_TOKEN`,
`SORTIE_LOOP_MILESTONE` (plus `GH_MILESTONE` as a legacy alias),
`SORTIE_LOOP_ASSIGNEE` (overrides `assignee:`).
Flags: `--no-server` disables the per-loop HTTP debug ports (from 7678 up).

## Loops

All loops default to items assigned to the token owner
(`assignee:@me` appended to each query filter). Set `assignee: ""` in
config (or `SORTIE_LOOP_ASSIGNEE=""`) for a shared backlog. The
startup set is every `WORKFLOW.*.md` in `.sortie/workflows/` — drop in
a new file (e.g. `WORKFLOW.triage.md`) and it starts as a loop named
by its stem (`triage`), port `7678+index`. New loops get the default
filter (`-label:needs-human` plus scope) until narrowed via the
`filters:` map in `.sortie/config.yaml` (keyed by loop name).

Five sortie loops: `plan` writes an implementation plan as
an issue comment (a human removes `agent:plan-needed` to approve);
`dev` builds `agent:quick` issues onto the base branch and `agent:build` issues via PR;
`review` reviews PRs labeled `agent:needs-review` (when the review is not
clean it labels the PR `agent:build`, routing it to review-fix);
`review-fix` applies posted review feedback on PRs labeled `agent:build`
and routes them back with `agent:needs-review` for re-review;
`merge` merges PRs labeled `agent:merge` (reads the PR plus all comments,
files follow-up issues for remaining findings, merges into the PR's base
branch; on conflicts it labels `needs-human` and drops the PR). `setup` installs the
workflow files into `.sortie/workflows/` (from `cmd/sortie-loop/workflows/`,
embedded in the binary) and the loop runs those copies — edit them per repo.
Re-run `setup` after upgrading sortie-loop to refresh them (local edits are
overwritten); the loop itself only restores files deleted since setup.
The label scheme is documented in the old `.pi-workflows/ARCHITECTURE.md`,
kept in project history.

## Ad hoc: auto-merge on clean review

To fully automate dev → review → merge on one repo (not a template
default), edit that repo's `.sortie/workflows/prompts/review.md` Step 4
clean branch to also apply `agent:merge`:

```
gh pr edit {{ .issue.identifier }} --repo $SORTIE_TRACKER_PROJECT --remove-label "agent:needs-review,in-progress" --add-label "agent:reviewed,agent:merge"
```

The merge loop already watches `label:agent:merge`, so it picks the PR
up on the next poll (~60s), triages comments, merges, and lands on
`agent:merged`. Blocking findings and conflicts still stop the merge
and escalate via `needs-human` — automation holds everywhere except
where judgment is required. Caveat: the merge agent uses
`git merge --ff-only`, so a branch drifted behind base stalls and
escalates instead of rebasing. Note: re-running `setup` after a
sortie-loop upgrade overwrites local workflow edits, so keep a copy of
the one-liner.

## Building

```sh
go build ./... && go vet ./...
```

## Roadmap

- Enforce `milestone:` on the `github-pr` tracker path. Config already
  appends `milestone:"..."` to every loop's query filter, but sortie's
  github-pr adapter parses only `label:` / `assignee:` / `-label:`
  clauses — the milestone clause is silently ignored and `domain.Issue`
  carries no milestone field. Until the adapter parses it, milestone
  scoping does not filter PR loops.
