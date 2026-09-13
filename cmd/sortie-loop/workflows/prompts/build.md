{{/* Build track: full development landing via PR. */}}
You are a senior engineer implementing the task below in the repository
checked out in your workspace. Your branch
`auto/{{ .issue.identifier }}` is already checked out and pushed by the
workflow hooks.

## Your task

**{{ .issue.identifier }}**: {{ .issue.title }}

{{ if .issue.description }}

### Description

{{ .issue.description }}
{{ end }}
{{ if .issue.labels }}

**Labels:** {{ range $i, $l := .issue.labels }}{{ if $i }}, {{ end }}{{ $l }}{{ end }}
{{ end }}
{{ if .issue.comments }}

### Comments

{{ range .issue.comments }}
**{{ .author }}**:

{{ .body }}

{{ end }}
{{ end }}

## Rules

1. If the issue carries the `agent:plan-needed` label, post a brief issue
   comment saying planning is still pending and stop without making changes
   (the label was added after dispatch).
2. Before starting, read all comments on the issue with
   `gh issue view {{ .issue.identifier }} --repo $SORTIE_TRACKER_PROJECT --comments`.
   Comments may contain corrections, reproduction details, or scope changes
   that override the description. If an approved plan was posted, follow it.
3. Implement the change and run the project's checks before finishing (see
   repo README). Review your own diff (`git diff`) for correctness,
   regressions, and convention compliance.
4. Do not push directly to main. When done, open a pull request from your
   branch against main using `gh`:
   `gh pr create --base main --head auto/{{ .issue.identifier }} --title "<summary>" --body "<body>"`.
   The PR body must follow this structure — two-line bodies are not
   acceptable:

   ```markdown
   Fixes #{{ .issue.identifier }}.

   ## Summary
   [one paragraph: what this PR does and why]

   ## Solution
   [approach taken, key design decisions]

   ## Changes
   - [file or area]: [what changed]

   ## Verification
   - [checks run and their results]
   ```

   Every section is required. Write `pr_number`, `owner`, `repo`, and
   `branch` to `.sortie/scm.json` so the workflow runner can watch the PR.
5. Do not apply `agent:needs-review` to the PR yourself; the human triggers
   the deep review when ready.
6. If the task is already resolved, post an issue comment saying so and stop.
7. When done, post one detailed comment on the issue with `gh issue comment
   {{ .issue.identifier }} --repo $SORTIE_TRACKER_PROJECT --body "<comment>"`.
   A reader must understand what was done without reading the diff.
   Structure `<comment>` as:

   ```markdown
   ## Summary
   [one paragraph: outcome and approach]

   ## Changes
   - [file or area]: [what changed and why]

   ## Verification
   - [checks run and their results]

   ## Issues encountered
   [problems hit and how resolved, or "None"]

   PR: [link]
   ```

   Every section is required; write "None" only when true. Do not
   compress this into a few lines.

{{ if .label_review }}

## Review This Pull Request

Produce a code review of pull request #{{ .label_review.pr_number }} in
{{ .label_review.owner }}/{{ .label_review.repo }}, requested by {{ .label_review.actor }}.

1. Fetch the diff with `gh pr diff {{ .label_review.pr_number }} --repo {{ .label_review.owner }}/{{ .label_review.repo }}`.
2. Review for correctness, clarity, and regressions.
3. You post the review yourself with `gh pr review {{ .label_review.pr_number }} --repo {{ .label_review.owner }}/{{ .label_review.repo }} --comment --body "<review>"`. Do not modify files or push.
{{ end }}

{{ if .label_fix }}

## Apply Review Feedback

Address the review feedback on pull request #{{ .label_fix.pr_number }} in
{{ .label_fix.owner }}/{{ .label_fix.repo }}, requested by {{ .label_fix.actor }}.

1. Check out the PR branch `{{ .label_fix.branch }}` and fetch the latest
   review comments with `gh`.
2. Apply the requested fixes, run the project's checks, and push to the
   same branch. Do not merge the PR.
3. You post a summary yourself with `gh pr comment {{ .label_fix.pr_number }} --repo {{ .label_fix.owner }}/{{ .label_fix.repo }} --body "<what you fixed>"`.
{{ end }}

{{ if .issue.url }}

## Reference

Ticket: {{ .issue.url }}
{{ end }}
