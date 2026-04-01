---
status: testing
phase: 02-schema-and-dependency-discovery
source: [02-01-SUMMARY.md, 02-02-SUMMARY.md, 02-03-SUMMARY.md]
started: 2026-03-25T08:50:41.9367156+01:00
updated: 2026-04-01T11:06:49.7239776+02:00
---

## Current Test

number: 3
name: Dependency-Aware Selection Review
expected: |
  In the interactive create or edit flow with reachable source and target schemas, the table-selection step should list discovered source tables, accept explicit selections and optional exclusions, then show a review that names explicit selections, required additions, blocked exclusions, and final table order before save confirmation.
awaiting: user response

## Tests

### 1. Cold Start Smoke Test
expected: From a fresh terminal in the repo root, run `go run ./cmd/db-sync`. The CLI should open the interactive wizard immediately instead of printing help and exiting. Using any phase-2-ready source and target databases you already have available, continue through validation until schema discovery and table selection run. The command should stay interactive, print the discovered source tables, and reach the selection/review flow without requiring any warm state from a previous run.
result: pass

### 2. Blocked Discovery Guidance
expected: In the interactive create or edit flow, if schema discovery cannot resolve endpoint environment variables or lacks metadata visibility, the flow should stop before table selection and show a blocked discovery summary with actionable remediation guidance instead of silently continuing with partial results.
result: pass

### 3. Dependency-Aware Selection Review
expected: In the interactive create or edit flow with reachable source and target schemas, the table-selection step should list discovered source tables, accept explicit selections and optional exclusions, then show a review that names explicit selections, required additions, blocked exclusions, and final table order before save confirmation.
result: [pending]

### 4. Blocked Exclusion Visibility
expected: If you explicitly exclude a table that is still required by a selected table, the review should keep that exclusion visible, mark the selection as blocked, and explain which selected tables require the excluded dependency rather than silently dropping or auto-correcting the exclusion.
result: [pending]

### 5. Edit Flow Selection Round-Trip
expected: After saving a profile with explicit selected tables and exclusions, running `go run ./cmd/db-sync profile edit <name>` should reopen the same selection intent, preserving only the explicit choices and exclusions rather than storing or replaying a precomputed dependency closure.
result: [pending]

## Summary

total: 5
passed: 2
issues: 0
pending: 3
skipped: 0
blocked: 0

## Gaps

[none yet]