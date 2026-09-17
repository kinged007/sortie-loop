# sortie-loop

Run autonomous coding agents against your GitHub issues with one command.

`sortie-loop` watches a repo's issues and pull requests, and for each
labeled item spins up an AI coding agent that reads the issue, writes the
code, and moves the work forward — planning, implementing, reviewing, and
merging — mostly on its own. You manage the backlog by adding labels and
assigning issues; the agents do the rest. GitHub is the only supported
tracker.

## What is what

**sortie-loop** (this repo) is a small Go wrapper. It does setup (config,
labels, workflow files) and then launches one loop process per workflow,
plus a web dashboard. It contains no agent itself.

**Sortie** (the engine, from the
[kinged007/sortie](https://github.com/kinged007/sortie) fork) does the
heavy lifting inside each loop: it polls GitHub on a timer, claims a
matching issue, clones the repo into a scratch workspace, hands the issue
text to an agent, and records the run in a local SQLite DB. The fork
exists because the loops need two pieces upstream lacks: the `pi` agent
adapter and the `github-pr` tracker (which treats pull requests as
work items).

**The agent** is `pi`, a coding agent that runs in a terminal the same
way you would: it reads files, edits code, runs checks, and shells out
to the `gh` CLI to read comments, open PRs, and move labels.

## How the agents pick up issues

There is no queue and no webhook. Every loop polls GitHub roughly every
60 seconds with a search query built from three parts:

1. **Label filter** — each loop watches its own labels. `dev` looks for
   `label:agent:quick,agent:build`, `review` for
   `label:agent:needs-review`, and so on (see Loops below). Adding a
   label enqueues the item; the agent's label moves dequeue it.
2. **Assignee scope** — by default every query ends in `assignee:@me`,
   meaning only items assigned to the token owner are picked up. This
   keeps several people (or bots) from fighting over one backlog. Set
   `assignee: ""` in config for a shared backlog where anyone's issue
   is fair game.
3. **Escalation guard** — every query excludes `-label:needs-human`,
   so anything an agent flagged for a person stays parked until a
   human removes the label.

When a loop claims an issue it clones the repo into
`.sortie/workspaces/<loop>/`, checks out a branch named
`auto/<issue-number>`, and renders a prompt template
(`.sortie/workflows/prompts/*.md`) with the issue title, body, labels,
and comments. The agent follows that prompt: it re-reads the full
comment thread with `gh` (comments can override the description), makes
the change, runs the project's checks, pushes, and posts a structured
summary comment (Summary / Changes / Verification / Issues encountered).
State moves forward through labels — e.g. `dev` opens a PR, a human
adds `agent:needs-review`, `review` reviews it, `merge` merges it.

## The loops

Five workflows ship by default (each is one `WORKFLOW.*.md` file, each
runs as its own loop process):

| Loop | Watches | Does |
|------|---------|------|
| `plan` | issues with `agent:plan-needed` | Writes an implementation plan as an issue comment. A human removes `agent:plan-needed` to approve and unblock `dev`. |
| `dev` | issues with `agent:quick` or `agent:build` | `agent:quick`: small fix, merged straight to the base branch, no PR. `agent:build`: full change, opened as a PR. Issues still carrying `agent:plan-needed` are skipped. |
| `review` | PRs with `agent:needs-review` | Reviews the PR. Clean reviews get `agent:reviewed`; anything else gets `agent:build`, routing it to review-fix. |
| `review-fix` | PRs with `agent:build` / `agent:pr-fix` | Applies posted review feedback, pushes, and puts `agent:needs-review` back on for re-review. |
| `merge` | PRs with `agent:merge` | Reads the PR plus all comments, files follow-up issues for leftovers, and merges into the PR's base branch. Conflicts get `needs-human` and stop. Uses `git merge --ff-only`, so a branch that drifted behind base stalls and escalates instead of rebasing. |

The loop set is just every `WORKFLOW.*.md` in `.sortie/workflows/` —
drop in a new file (e.g. `WORKFLOW.triage.md`) and it starts as a loop
named by its stem on the next run. Per-loop label queries can be
narrowed via the `filters:` map in `.sortie/config.yaml` without
re-running setup.

## Unified dashboard

Every run serves a small status page (default
`http://127.0.0.1:7677`, auto-refreshes every 5s). Each loop also gets
its own debug port starting at 7678. The page shows what is running
right now (issue, loop, turns, tokens, model) alongside all-time totals
read from each loop's SQLite DB, so numbers survive restarts.

Running loops in several repos at once? Pass `--unite`:

```sh
sortie-loop --unite          # in each repo
```

The first `--unite` run starts the dashboard; later ones detect it and
join instead of starting a second page. Every run registers its loops
in a shared registry file, so the one page rolls up all repos: a
per-repo summary plus the same running-now and token tables with a Repo
column. Without `--unite` the dashboard shows only its own repo.

## Install

Requires: **go >= 1.24**, **git**, and the **gh CLI** (`setup` shells
out to `gh`; the agent prompts use it throughout).

```sh
git clone https://github.com/kinged007/sortie-loop.git
cd sortie-loop
./install.sh                    # -> ~/.local/bin/sortie-loop
```

`install.sh` builds the Go binary, then resolves the `sortie` engine
binary: it links one already on `PATH` if present, otherwise downloads
the matching release from the
[kinged007/sortie](https://github.com/kinged007/sortie) fork
(`--prefix DIR` and `--sortie-bin PATH` override the install dir and
the engine binary). Only build the engine from source if the download
fails:

```sh
git clone https://github.com/kinged007/sortie.git
cd sortie && go build -o ~/.local/bin/sortie ./cmd/sortie
```

### GitHub token

The token is the identity of both the loop and the agent: it
authenticates GitHub API polling, label changes, clones of private
repos, and every `gh` command the agent runs (reading issues, posting
comments, opening PRs). `@me` in the assignee scope means whoever owns
this token. Use a token for the account you want the work attributed
to — a separate bot account keeps agent commits, comments, and PRs off
your personal name.

1. Create it: `gh auth login`, or generate a token at
   https://github.com/settings/tokens with the `repo` scope (classic
   token is simplest; a fine-grained token needs Contents, Issues, and
   Pull requests read/write on the repos you loop over).
2. Expose it with one of (first set wins: `SORTIE_LOOP_TOKEN`, then
   `token:` in `.sortie/config.yaml`, then `GITHUB_TOKEN`, then
   `GH_TOKEN`):
   ```sh
   export GITHUB_TOKEN="ghp_..."     # or GH_TOKEN; or SORTIE_LOOP_TOKEN to override just the loop
   ```
   The `gh` CLI reads `GITHUB_TOKEN`/`GH_TOKEN` itself, so exporting
   one of those covers both the loop and the agent's `gh` calls. Never
   commit the token — `token:` in config is for local-only setups.
3. Verify: `gh auth status` should show the right account before you
   run `setup` (which syncs labels) or the loop.

## Use (inside any repo)

```sh
cd your-repo
sortie-loop setup    # writes .sortie/config.yaml, updates .gitignore, creates the labels
sortie-loop          # run every WORKFLOW.*.md loop (Ctrl-C stops all)
```

`setup` detects `owner/name` from the git `origin` remote
(`--repo=owner/name` overrides). The loop re-detects it at every run, so
moving the checkout or forking needs no reconfiguration.

Flags: `--unite` joins (or starts) the shared dashboard,
`--dashboard-port=N` forces the dashboard onto port N,
`--no-dashboard` / `--no-server` disable the dashboard page and the
per-loop debug ports respectively. A positional arg selects the repo
root (default: current directory).

## Layout in the target repo

```
.sortie/
  config.yaml      # repo:, token:, milestone: (all optional — see below)
  workflows/       # installed by setup from the binary; edit freely, loop runs these
  .env.loop        # generated each run (resolved tracker, token, clone URL; mode 0600)
  workspaces/      # per-item agent checkouts (one subdir per loop)
```

`.sortie/config.yaml`:

```yaml
repo: ""        # empty = detect from git remote; or pin owner/name
token: ""       # empty = SORTIE_LOOP_TOKEN, then GITHUB_TOKEN / GH_TOKEN env
milestone: ""   # optional milestone title to restrict all loops to
assignee: "@me" # token owner only; "" = shared backlog
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
where judgment is required. Note: re-running `setup` after a
sortie-loop upgrade overwrites local workflow edits, so keep a copy of
the one-liner.

## Agent identity (name/avatar on GitHub)

The commit author name/email is what GitHub displays. Point the loop's git
identity at the agent — in the target repo or globally:

```sh
git config --global user.name "sortie-agent"
git config --global user.email "agent@example.com"
```

`user.name` is display-only. GitHub links the commit (and its avatar) to the
GitHub account whose **verified email** matches the author email. One account
= one avatar, so every agent using your emails shows your personal avatar.
Options for a distinct face: a separate bot account per agent (own email +
avatar, verified there); or no GitHub account at all plus a per-email avatar
on gravatar.com (GitHub falls back to the Gravatar for unclaimed emails);
or publishing the agent as a GitHub App (`name[bot]`, its own avatar).

## Building

```sh
go build ./... && go vet ./...
```

## Customizing agents and workflows

The shipped default is the `pi` agent. Every `WORKFLOW.*.md` also
documents a commented-out `claude-code` alternative (dev/review-fix/
merge on `sonnet`, plan/review on `opus`) — swap the `agent:` block
for the commented one, keeping the same `max_turns` and timeouts
(dev's dispatch rules must name `claude-code` too). The `model:` value
passes through to `claude --model` unvalidated, so only use aliases
your installed CLI accepts. The pinned engine only knows `pi` and
`claude-code` (plus a few others); other adapters need a newer or
upstream engine binary. The dashboard's token/model columns populate
for either agent.

Sortie upstream references:

- [Claude Code adapter](https://docs.sortie-ai.com/reference/adapter-claude-code/) —
  `agent:` fields and the `claude-code:` extension block
  (`model`, `fallback_model`, `permission_mode`, costs)
- [WORKFLOW.md configuration reference](https://docs.sortie-ai.com/reference/workflow-config/) —
  full front-matter schema
- [Configure dispatch rules](https://docs.sortie-ai.com/guides/configure-dispatch-rules/) —
  routing plus the `dispatch.agent.missing_block` rule (every kind a
  rule names needs its own settings block)
- [Write a prompt template](https://docs.sortie-ai.com/guides/write-prompt-template/) —
  template variables and helpers for `prompts/*.md`
- [Control agent costs](https://docs.sortie-ai.com/guides/control-costs/) —
  turn caps, session caps, concurrency limits

## Roadmap

- Enforce `milestone:` on the `github-pr` tracker path. Config already
  appends `milestone:"..."` to every loop's query filter, but sortie's
  github-pr adapter parses only `label:` / `assignee:` / `-label:`
  clauses — the milestone clause is silently ignored and `domain.Issue`
  carries no milestone field. Until the adapter parses it, milestone
  scoping does not filter PR loops.
