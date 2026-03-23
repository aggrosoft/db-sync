---
status: diagnosed
phase: 01-foundation-cli
source: [01-01-SUMMARY.md, 01-02-SUMMARY.md, 01-03-SUMMARY.md]
started: 2026-03-23T10:15:00Z
updated: 2026-03-23T10:22:00Z
---

## Current Test

[testing paused — 2 items outstanding]

## Tests

### 1. Wizard Fast-Path New Profile Flow
expected: Run `go run ./cmd/db-sync` in an interactive terminal. The CLI should enter the wizard instead of printing help, move through a short fast-path flow, and reach a review-before-save step that clearly shows the profile name, engines, placeholder-based DSN templates, empty table selection, and sync settings before asking for final save confirmation.
result: issue
reported: "it prints help - no wizard"
severity: major

### 2. Edit Flow Prefill Behavior
expected: After saving a profile, run `go run ./cmd/db-sync profile edit <name>`. The same wizard should reopen with the existing profile name, source engine, target engine, DSN templates, sync mode, and mirror-delete setting already populated.
result: issue
reported: "creating a profile asked me for a dsn template, there is no reason for that - it could ask for a connection string even better just ask for connection details and just write down an .env file - there is no reason for a dynamic dsn template. Just allow the operator to connect his database easily"
severity: major

### 3. Validation Failure Copy And Redaction
expected: Trigger validation failure with missing environment variables or an unreachable database. The CLI should print a structured blocked report that names the failing endpoint and check, while keeping resolved credentials hidden.
result: blocked
blocked_by: prior-phase
reason: "can not validate, must fix connecting first"

### 4. Successful Validate-And-Save Flow
expected: With reachable PostgreSQL or MySQL endpoints and placeholder-backed credentials available, completing the wizard and confirming save should validate both endpoints, persist the profile under the user config directory, and keep `${NAME}` placeholders in the YAML file rather than raw secrets.
result: blocked
blocked_by: prior-phase
reason: "also blocked"

## Summary

total: 4
passed: 0
issues: 2
pending: 0
skipped: 0
blocked: 2

## Gaps
- truth: "Running `go run ./cmd/db-sync` in an interactive terminal enters the wizard instead of printing help and reaches the review-before-save step."
  status: failed
  reason: "User reported: it prints help - no wizard"
  severity: major
  test: 1
  root_cause: "The root command only starts the wizard when `golang.org/x/term` detects stdin as a terminal. In the Windows terminal used for UAT, that probe returned false, so `internal/cli/root.go` intentionally printed help instead of entering the wizard."
  artifacts:
    - path: "internal/cli/root.go"
      issue: "Wizard startup is gated by `term.IsTerminal(0)`, which misclassifies the tested Windows terminal context as non-interactive."
    - path: "internal/cli/app.go"
      issue: "The wizard path exists and is wired, so the gap is in terminal detection rather than missing flow logic."
  missing:
    - "Harden root command interactivity detection for Windows integrated or pseudo terminals so no-arg execution reaches the wizard in normal operator shells."
    - "Add a regression seam or test around root-command startup behavior so the help branch is not taken in expected interactive environments."
  debug_session: ".planning/debug/phase-1-uat-help-no-wizard.md"
- truth: "The profile creation or edit flow should let the operator connect the database easily without forcing DSN template authoring, ideally by accepting a connection string or guided connection details and writing the env-backed config for them."
  status: failed
  reason: "User reported: creating a profile asked me for a dsn template, there is no reason for that - it could ask for a connection string even better just ask for connection details and just write down an .env file - there is no reason for a dynamic dsn template. Just allow the operator to connect his database easily"
  severity: major
  test: 2
  root_cause: "Phase 1 encoded placeholder-backed `dsn_template` as the only supported connection representation in the profile schema, wizard draft, validation layer, and persistence policy, so the operator is forced to author DSN templates instead of supplying a connection string or guided connection details."
  artifacts:
    - path: "internal/wizard/flow.go"
      issue: "Wizard prompts only accept source and target DSN templates."
    - path: "internal/model/profile.go"
      issue: "Endpoint schema only stores `engine` and `dsn_template`, with no higher-level connection input model."
    - path: "internal/validate/service.go"
      issue: "Validation enforces the DSN-template model and rejects non-placeholder connection input."
  missing:
    - "Replace the DSN-template-only wizard inputs with an operator-friendly connection flow that accepts either a connection string or guided connection details."
    - "Generate the env-backed profile representation internally instead of requiring the operator to author placeholder templates."
    - "Update profile schema, validation, and persistence to support the simpler connection model end-to-end."
  debug_session: ".planning/debug/phase-1-uat-dsn-template-gap.md"
