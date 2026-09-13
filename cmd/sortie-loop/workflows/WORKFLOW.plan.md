---
tracker:
  kind: github
  api_key: $SORTIE_TRACKER_API_KEY
  project: $SORTIE_TRACKER_PROJECT
  query_filter: "label:agent:plan-needed assignee:@me"
  # ponytail: `backlog` must stay first. Open issues carrying no state
  # label derive active_states[0] as their state, so without a leading
  # non-target entry the dispatch-time in-progress transition is a silent
  # no-op and the `in-progress` label never lands on the issue.
  active_states: [backlog, in-progress]
  in_progress_state: in-progress
  handoff_state: review
  # Plan writes no code, so work-evidence is always undeterminable;
  # `off` skips the capture and the per-run log line. Handoff proceeds.
  handoff_evidence: off
  terminal_states: [done]
  comments:
    on_completion: false
    on_failure: false

polling:
  interval_ms: 60000

db_path: .sortie-plan.db

workspace:
  root: $SORTIE_LOOP_WORKSPACES/plan
  retention_days: 30

# ponytail: plan-gating is enforced by the dev loop's query_filter
# (`-label:agent:plan-needed`), not here — dispatch.match has no negation.
# The plan loop stays a separate file so it can carry its own model.

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
    template: ./prompts/plan.md
---

{{/* Plan loop: issues labeled `agent:plan-needed`. Prompt lives in
     prompts/plan.md (via dispatch default above): research the issue,
     post the plan as an issue comment, write no code. A human removes
     `agent:plan-needed` after review, which makes the issue eligible
     for the dev loop. The body below never renders; it documents the
     routing. */}}
Plan loop routing only — the prompt comes from the dispatch template.
