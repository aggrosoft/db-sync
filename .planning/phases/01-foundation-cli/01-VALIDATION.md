---
phase: 01
slug: foundation-cli
status: draft
nyquist_compliant: true
wave_0_complete: true
created: 2026-03-23
---

# Phase 01 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` with `go-cmp` for structural comparisons and `testcontainers-go` for DB integration coverage |
| **Config file** | none — standard `go test` conventions are sufficient |
| **Quick run command** | `go test ./internal/profile/... ./internal/secrets/... ./internal/wizard/... ./internal/validate/...` |
| **Full suite command** | `go test ./... -tags=integration` |
| **Estimated runtime** | ~25 seconds quick run, ~45 seconds full suite |

---

## Sampling Rate

 **After every task commit:** Run `go test` only for the touched internal packages, using `go test ./internal/profile/... ./internal/secrets/... ./internal/wizard/... ./internal/validate/...` as the default quick path
- **After every plan wave:** Run `go test ./... -tags=integration`
- **Before `/gsd-verify-work`:** Full suite must be green
 **Max feedback latency:** 30 seconds

---

## Plan-Level Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01 | 1 | PROF-01 | unit | `go test ./internal/profile/... ./internal/secrets/...` | ❌ planned in 01-01 | ⬜ pending |
| 01-02-01 | 02 | 2 | PROF-02 | unit | `go test ./internal/wizard/... ./internal/cli/...` | ❌ planned in 01-02 | ⬜ pending |
| 01-03-01 | 03 | 3 | PROF-03 | integration | `go test ./internal/validate/... -tags=integration` | ❌ planned in 01-03 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

This map is intentionally plan-level. Task-specific verification commands remain defined inside each `*-PLAN.md` task block and should be used for execution-time sampling.

---

## Wave 0 Requirements

- [ ] `go.mod` — module initialized so `go test` can run
- [ ] `internal/profile/profile_test.go` — coverage for YAML round-trip and placeholder-safe persistence
- [ ] `internal/wizard/wizard_test.go` — coverage for new/edit draft reuse and review-step behavior
- [ ] `internal/validate/validate_integration_test.go` — adapter integration coverage for PostgreSQL and MySQL/MariaDB
- [ ] `internal/testkit/` — shared Docker-backed test helpers for supported databases
- [ ] `go get github.com/google/go-cmp/cmp github.com/testcontainers/testcontainers-go` — test dependencies installed during Wave 0 setup
- [x] No standalone Wave 0 is required for this phase — test infrastructure is introduced directly within Plans 01-01 through 01-03.
- [x] Plan 01-01 introduces `go.mod`, baseline dependencies, and profile or placeholder unit tests.
- [x] Plan 01-02 introduces wizard and CLI-focused unit coverage.
- [x] Plan 01-03 introduces integration-tag validation tests and shared container helpers.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Wizard flow feels fast-path first but still includes a review-before-save step | PROF-02 | Prompt flow and operator ergonomics are easier to confirm interactively than with unit assertions alone | Start the CLI, create a new profile, confirm the path reaches review in a short sequence and edit mode reopens with existing values |
| Validation error messaging makes missing env placeholders and failed DB checks understandable | PROF-03 | Human-readable CLI copy quality needs operator review | Run validation with missing env vars and with an unreachable database; confirm the output names the failure cause without leaking secrets |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [x] Feedback latency < 30s for task-level quick checks
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-03-23