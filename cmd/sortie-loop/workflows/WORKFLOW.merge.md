---
tracker:
  kind: github-pr
  api_key: $SORTIE_TRACKER_API_KEY
  project: $SORTIE_TRACKER_PROJECT
  query_filter: "assignee:@me -label:needs-human"
  active_states: [agent:merge]
  in_progress_state: agent:merge
  handoff_state: agent:merged
  handoff_evidence: off
  terminal_states: [agent:review-complete]
  comments:
    on_completion: false

polling:
  interval_ms: 60000

db_path: .sortie-merge.db

workspace:
  root: $SORTIE_LOOP_WORKSPACES/merge
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
  max_turns: 10
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
    template: ./prompts/merge.md
---

{{/* Merge loop: label an approved PR `agent:merge` to have the agent
     read the PR plus all comments/reviews, file follow-up issues for
     remaining findings, and merge the branch. The prompt lives in
     prompts/merge.md (via dispatch default above). Completion swaps the
     label to `agent:merged`; on conflicts the agent labels `needs-human`
     and the loop drops the PR via its `-label:needs-human` exclusion. */}}
Merge loop routing only — the prompt comes from the dispatch template.
