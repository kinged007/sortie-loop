---
tracker:
  kind: github-pr
  api_key: $SORTIE_TRACKER_API_KEY
  project: $SORTIE_TRACKER_PROJECT
  active_states: [agent:needs-review]
  in_progress_state: agent:needs-review
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
  turn_timeout_ms: 1800000
  read_timeout_ms: 10000
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
     above). Completion swaps the label to `agent:reviewed`; apply
     `agent:review-complete` when done with the feedback. */}}
Review loop routing only — the prompt comes from the dispatch template.
