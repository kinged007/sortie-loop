---
tracker:
  kind: github
  api_key: $SORTIE_TRACKER_API_KEY
  project: $SORTIE_TRACKER_PROJECT
  # ponytail: comma is GitHub search OR. The explicit
  # `(label:a OR label:b)` form returns zero results from /search/issues.
  query_filter: "label:agent:quick,agent:build -label:agent:plan-needed assignee:@me"
  # ponytail: `backlog` must stay first. Open issues carrying no state
  # label derive active_states[0] as their state, so without a leading
  # non-target entry the dispatch-time in-progress transition is a silent
  # no-op and the `in-progress` label never lands on the issue.
  active_states: [backlog, in-progress]
  in_progress_state: in-progress
  handoff_state: review
  terminal_states: [done]
  comments:
    on_completion: false
    on_failure: false

polling:
  interval_ms: 60000

db_path: .sortie-dev.db

workspace:
  root: $SORTIE_LOOP_WORKSPACES/dev
  retention_days: 30

hooks:
  # ponytail: hooks run with a restricted env (PATH/HOME + SORTIE_* only),
  # so the repo URL must arrive as SORTIE_-prefixed vars set by the CLI.
  after_create: |
    git clone --depth 1 "$SORTIE_LOOP_CLONE_URL" .
  before_run: |
    git fetch origin main
    git checkout -B "auto/$SORTIE_ISSUE_IDENTIFIER" origin/main
  after_run: |
    git add -A
    git diff --cached --quiet || git commit -m "$SORTIE_ISSUE_IDENTIFIER: automated changes"
    git push -u origin "auto/$SORTIE_ISSUE_IDENTIFIER"
  before_remove: |
    git push origin --delete "auto/${SORTIE_ISSUE_IDENTIFIER}" 2>/dev/null || true
  timeout_ms: 120000

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

# ponytail: one shared dev loop. Split into WORKFLOW.quick/build.md when
# quick vs build need different models — dispatch cannot vary `pi.model`.
dispatch:
  rules:
    - name: quick
      match:
        labels: ["agent:quick"]
      agent: pi
      template: ./prompts/quick.md
    - name: build
      match:
        labels: ["agent:build"]
      agent: pi
      template: ./prompts/build.md
  default:
    template: ./prompts/build.md

reactions:
  review_comments:
    provider: github
    max_retries: 2
    escalation: label
    escalation_label: needs-human
  label_commands:
    provider: github
    review_label: agent:review
    fix_label: agent:fix
---

{{/* Dev loop: issues labeled `agent:quick` or `agent:build`
     (optionally plus `agent:plan-needed` while planning). The query
     excludes `agent:plan-needed`, so planning gates development.
     Prompts live in prompts/: dispatch routes quick to quick.md,
     build to build.md, and anything unrouted to build (PR = safe).
     The body below never renders; it documents the routing.
     The {{ if .label_review }} and {{ if .label_fix }} branches live in
     the per-rule templates (label dispatches reuse the frozen dispatch
     template, not this body). */}}
Dev loop routing only — the prompt comes from the dispatch template.
