---
tracker:
  kind: github-pr
  api_key: $SORTIE_TRACKER_API_KEY
  project: $SORTIE_TRACKER_PROJECT
  # Strictly: PRs assigned to the token owner carrying agent:needs-review
  # (assignee scope is appended by sortie-loop config at launch, enforced
  # client-side by the adapter); the label
  # clause rides along for visibility (the github-pr adapter lists via
  # /pulls, which runs no search syntax — active_states below is what
  # actually matches the label). -label:needs-human keeps escalated PRs
  # out, matched client-side by the adapter.
  query_filter: "label:agent:needs-review -label:needs-human"
  # Dispatch claims a taken PR by swapping agent:needs-review -> in-progress
  # (auto, non-fatal), so a second agent never picks it up. Do not add
  # agent:needs-review here: DeriveLabelState is first-match-wins, so the
  # claim label must sort first for the worker's own refresh to see it.
  active_states: [in-progress, agent:needs-review]
  in_progress_state: in-progress
  handoff_state: agent:reviewed
  handoff_evidence: off
  terminal_states: [agent:review-complete]
  comments:
    on_completion: false

polling:
  interval_ms: 60000

db_path: .sortie-review.db

workspace:
  root: $SORTIE_LOOP_WORKSPACES/review
  retention_days: 30

hooks:
  after_create: |
    git init -q . 2>/dev/null || true
    git fetch --depth 1 origin "pull/$SORTIE_ISSUE_IDENTIFIER/head:pr-$SORTIE_ISSUE_IDENTIFIER" 2>/dev/null || git fetch --depth 1 "$SORTIE_LOOP_CLONE_URL" "pull/$SORTIE_ISSUE_IDENTIFIER/head:pr-$SORTIE_ISSUE_IDENTIFIER"
    git checkout -q "pr-$SORTIE_ISSUE_IDENTIFIER"
  timeout_ms: 60000

agent:
  kind: pi
  command: pi
  max_turns: 3
  max_concurrent_agents: 1
  turn_timeout_ms: 3600000
  read_timeout_ms: 120000
  stall_timeout_ms: 300000
  stop_grace_ms: 5000
  max_retry_backoff_ms: 120000

pi:
  model: ""

dispatch:
  default:
    template: ./prompts/review.md
---

{{/* Review loop: label a PR `agent:needs-review` to get a three-pass
     review. The prompt lives in prompts/review.md (via dispatch default
     above): Step 4 applies `agent:build` to the PR when the review is
     not clean, routing it to the review-fix loop. Completion swaps the
     label to `agent:reviewed`; apply `agent:review-complete` when done
     with the feedback. */}}
Review loop routing only — the prompt comes from the dispatch template.
