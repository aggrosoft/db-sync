---
phase: 01-foundation-cli
plan: 05
subsystem: wizard
tags: [go, huh, wizard, connection-first, env-backed-secrets]
requires:
  - phase: 01-02
    provides: shared wizard service and edit prefill pattern
  - phase: 01-03
    provides: validated profile save flow to integrate with later persistence changes
provides:
  - Connection-first endpoint contract for connection-string, details, and legacy-template modes
  - Shared create/edit wizard flow for connection-first endpoint input
  - Review rendering that explains env-backed secret storage without printing raw secrets
affects: [operator-ux, persistence, validation]
tech-stack:
  added: []
  patterns: [connection-first drafts, env-var convention generation, redacted review summaries]
key-files:
  created: []
  modified: [internal/model/profile.go, internal/profile/schema.go, internal/secrets/template.go, internal/wizard/draft.go, internal/wizard/flow.go, internal/wizard/messages.go, internal/wizard/review.go, internal/wizard/wizard_test.go]
key-decisions:
  - "Represent connection input explicitly in the profile model instead of forcing operators to author DSN templates."
  - "Keep legacy-template support in the model so earlier Phase 1 profiles remain editable and valid during the gap closure."
patterns-established:
  - "Wizard drafts preserve env-var naming conventions so later save and validation logic can resolve secrets predictably."
  - "Review output explains env-file storage plans while redacting passwords and connection strings."
requirements-completed: [PROF-01, PROF-03]
duration: 35 min
completed: 2026-03-23
---

# Phase 01 Plan 05: Connection-First Wizard Summary

**Connection-first profile modeling and create/edit wizard flow that replaces template authoring with operator-friendly input paths**

## Performance

- **Duration:** 35 min
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Added a connection-first endpoint model with `connection-string`, `details`, and `legacy-template` modes.
- Refactored draft hydration and wizard flow so new and edit operations share the same connection-first path.
- Reworked review rendering so env-file storage is explicit while raw DSNs and passwords stay redacted.

## Task Commits

This inline execution did not create per-task git commits.

## Files Created/Modified
- `internal/model/profile.go` and `internal/profile/schema.go` - connection-first endpoint contract and normalization rules.
- `internal/wizard/draft.go` and `internal/wizard/flow.go` - create/edit draft mapping and connection-first interactive flow.
- `internal/wizard/review.go` and `internal/wizard/messages.go` - operator-facing review and copy for env-backed secrets.
- `internal/wizard/wizard_test.go` - coverage for defaults, edit prefill, and secret-safe review output.

## Decisions Made
- Kept a `legacy-template` mode instead of hard-cutting older profiles to avoid stranding Phase 1 artifacts.
- Derived deterministic env-var names from the profile slug and endpoint role so review, save, and validation stay aligned.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required

None.

## Next Phase Readiness
- Profiles can now express operator-friendly connection input without hand-authored DSN templates.
- Persistence and validation can now move secrets into sibling env files while preserving edit prefill behavior.

---
*Phase: 01-foundation-cli*
*Completed: 2026-03-23*