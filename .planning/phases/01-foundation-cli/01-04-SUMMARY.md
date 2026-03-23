---
phase: 01-foundation-cli
plan: 04
subsystem: cli
tags: [go, cobra, windows, terminal-detection]
requires:
  - phase: 01-01
    provides: root command shell and app wiring
  - phase: 01-02
    provides: wizard-backed interactive profile flow
provides:
  - Windows-tolerant no-arg root interactivity gate
  - Regression coverage for interactive and redirected stdin startup paths
affects: [operator-ux, root-command-entry]
tech-stack:
  added: []
  patterns: [injectable terminal probe, root-only interactivity gating]
key-files:
  created: [internal/cli/root_test.go]
  modified: [internal/cli/root.go]
key-decisions:
  - "Treat character-device stdin plus known terminal signals as interactive even when golang.org/x/term reports false in Windows pseudo-terminals."
  - "Keep headless and redirected stdin strict so non-interactive automation still falls back to help output."
patterns-established:
  - "Root startup policy is test-driven through an interactivity probe rather than hard-coded x/term behavior."
requirements-completed: [PROF-01]
duration: 15 min
completed: 2026-03-23
---

# Phase 01 Plan 04: Root Startup Gap Summary

**Windows-tolerant root command startup detection with explicit regression coverage for wizard-first and non-interactive paths**

## Performance

- **Duration:** 15 min
- **Tasks:** 1
- **Files modified:** 2

## Accomplishments
- Replaced the brittle `term.IsTerminal(0)`-only check with a layered interactivity probe.
- Preserved strict help fallback for redirected or clearly headless execution contexts.
- Added tests that lock in wizard startup, non-interactive fallback, and unaffected explicit subcommands.

## Task Commits

This inline execution did not create per-task git commits.

## Files Created/Modified
- `internal/cli/root.go` - layered interactivity detection for root no-arg execution.
- `internal/cli/root_test.go` - table-driven coverage for Windows terminal signals, redirected stdin, and subcommand behavior.

## Decisions Made
- Used stdin file-mode inspection plus terminal environment signals to handle Windows pseudo-terminals more safely.
- Kept the change scoped to root-command startup so explicit `profile ...` commands remain unchanged.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required

None.

## Next Phase Readiness
- The root command now reliably enters the wizard-first flow in the tested Windows terminal scenarios.
- The connection-first profile work can build on a stable default entrypoint without relying on ad hoc UAT behavior.

---
*Phase: 01-foundation-cli*
*Completed: 2026-03-23*