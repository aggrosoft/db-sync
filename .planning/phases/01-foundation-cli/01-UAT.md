---
status: partial
phase: 01-foundation-cli
source: [01-01-SUMMARY.md, 01-02-SUMMARY.md, 01-03-SUMMARY.md, 01-04-SUMMARY.md, 01-05-SUMMARY.md, 01-06-SUMMARY.md]
started: 2026-03-23T10:15:00Z
updated: 2026-03-23T11:31:10Z
---

## Current Test

number: none
name: Manual Retest Pending
expected: |
  Re-run Test 1 and Test 4 after the validation-recovery fix is verified manually. The wizard should stay interactive on blocked validation and offer retry, modify, or cancel before the successful save path is rechecked.
awaiting: next iteration

## Tests

### 1. Wizard Fast-Path New Profile Flow
expected: Run `go run ./cmd/db-sync` in an interactive terminal. The CLI should enter the wizard instead of printing help, move through a short fast-path flow, and reach a review-before-save step that clearly shows the profile name, engines, placeholder-based DSN templates, empty table selection, and sync settings before asking for final save confirmation.
result: issue
reported: "program exits hard when wrong connection details are entered; it must ask to retry / modify / cancel."
severity: blocker

### 2. Edit Flow Prefill Behavior
expected: After saving a profile, run `go run ./cmd/db-sync profile edit <name>`. The same wizard should reopen with the existing profile name, source engine, target engine, DSN templates, sync mode, and mirror-delete setting already populated.
result: pass

### 3. Validation Failure Copy And Redaction
expected: Trigger validation failure with missing environment variables or an unreachable database. The CLI should print a structured blocked report that names the failing endpoint and check, while keeping resolved credentials hidden.
result: pass

### 4. Successful Validate-And-Save Flow
expected: With reachable PostgreSQL or MySQL endpoints and placeholder-backed credentials available, completing the wizard and confirming save should validate both endpoints, persist the profile under the user config directory, and keep `${NAME}` placeholders in the YAML file rather than raw secrets.
result: blocked
reason: "Blocked by Test 1; recheck in next iteration."
blocked_by: 1

## Summary

total: 4
passed: 2
issues: 1
pending: 0
skipped: 0
blocked: 1

## Gaps
- truth: "Running `go run ./cmd/db-sync` in an interactive terminal enters the wizard instead of printing help and reaches the review-before-save step."
  status: resolved
  reason: "Plan 01-04 replaced the brittle terminal probe with a layered Windows-aware interactivity check and automated regression coverage."
  severity: major
  test: 1
  root_cause: "The root command only started the wizard when `golang.org/x/term` detected stdin as a terminal. In the Windows terminal used for UAT, that probe returned false, so `internal/cli/root.go` intentionally printed help instead of entering the wizard."
  artifacts:
    - path: "internal/cli/root.go"
      issue: "Root startup now uses a layered interactivity probe instead of `term.IsTerminal(0)` alone."
    - path: "internal/cli/app.go"
      issue: "The wizard path remains wired through the root command once the terminal session is classified as interactive."
  missing: []
  debug_session: ".planning/debug/phase-1-uat-help-no-wizard.md"
- truth: "When validation fails because connection details are wrong, the interactive flow lets the operator retry, modify the details, or cancel instead of hard-stopping the program."
  status: failed
  reason: "User reported: program exits hard when wrong connection details are entered; it must ask to retry / modify / cancel."
  severity: blocker
  test: 1
  root_cause: "The interactive create/edit commands returned blocked `ValidateAndSave` errors directly from `internal/cli/app.go`, so Cobra exited the command after printing the validation report instead of keeping the operator inside the wizard with a recovery choice."
  artifacts:
    - path: "internal/cli/app.go"
      issue: "Interactive save now loops on blocked validation and offers retry, modify, or cancel instead of returning the blocked error directly."
    - path: "internal/cli/app_test.go"
      issue: "Focused CLI coverage proves retry and modify stay inside the interactive flow after blocked validation."
  missing: []
  debug_session: ""
- truth: "The profile creation or edit flow should let the operator connect the database easily without forcing DSN template authoring, ideally by accepting a connection string or guided connection details and writing the env-backed config for them."
  status: resolved
  reason: "Plans 01-05 and 01-06 replaced DSN-template authoring with a connection-first wizard, env-backed persistence, and mode-aware validation."
  severity: major
  test: 2
  root_cause: "Phase 1 encoded placeholder-backed `dsn_template` as the only supported connection representation in the profile schema, wizard draft, validation layer, and persistence policy, so the operator was forced to author DSN templates instead of supplying a connection string or guided connection details."
  artifacts:
    - path: "internal/wizard/flow.go"
      issue: "Wizard prompts now accept connection strings, guided details, or legacy templates, then route secrets into env-backed storage."
    - path: "internal/model/profile.go"
      issue: "Endpoint schema now supports `connection-string`, `details`, and `legacy-template` modes."
    - path: "internal/validate/service.go"
      issue: "Validation now resolves connection-string and details modes directly while preserving legacy template compatibility."
  missing: []
  debug_session: ".planning/debug/phase-1-uat-dsn-template-gap.md"
