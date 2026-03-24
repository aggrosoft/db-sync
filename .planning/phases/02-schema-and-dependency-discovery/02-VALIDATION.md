---
phase: 02
slug: schema-and-dependency-discovery
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-03-24
---

# Phase 02 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` with `go test` |
| **Config file** | none |
| **Quick run command** | `go test ./internal/cli ./internal/model ./internal/profile ./internal/validate ./internal/wizard` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~90 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/...`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 120 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 02-01-01 | 01 | 0 | SCMA-01 | integration + unit | `go test -tags=integration ./internal/db/... ./internal/validate/...` | ❌ W0 | ⬜ pending |
| 02-01-02 | 01 | 0 | SCMA-01 | unit | `go test ./internal/schema/... -run TestNormalizeSnapshot` | ❌ W0 | ⬜ pending |
| 02-02-01 | 02 | 1 | SCMA-02 | unit | `go test ./internal/schema/... -run TestCompareSnapshots` | ❌ W0 | ⬜ pending |
| 02-02-02 | 02 | 1 | SCMA-03 | unit | `go test ./internal/schema/... -run TestClassifyTableDrift` | ❌ W0 | ⬜ pending |
| 02-02-03 | 02 | 1 | SCMA-04, SCMA-05 | unit | `go test ./internal/schema/... -run 'TestSkippedColumnsProduceWarnings|TestTargetOptionalColumns'` | ❌ W0 | ⬜ pending |
| 02-03-01 | 03 | 1 | TABL-01 | unit | `go test ./internal/schema/... -run TestBuildDependencyGraph` | ❌ W0 | ⬜ pending |
| 02-03-02 | 03 | 1 | TABL-03 | unit | `go test ./internal/schema/... -run TestDependencyClosure` | ❌ W0 | ⬜ pending |
| 02-03-03 | 03 | 1 | TABL-02 | unit | `go test ./internal/cli/... ./internal/wizard/... -run TestDependencySelection` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/schema/snapshot_test.go` — stubs for canonical table/column metadata normalization and qualified table ID coverage
- [ ] `internal/schema/compare_test.go` — shared drift comparison and classification cases for `SCMA-02` through `SCMA-05`
- [ ] `internal/schema/graph_test.go` — dependency graph and minimal closure coverage for `TABL-01` through `TABL-03`
- [ ] `internal/db/postgres/adapter_integration_test.go` — PostgreSQL discovery query verification
- [ ] `internal/db/mysql/adapter_integration_test.go` — MySQL discovery query verification
- [ ] `internal/db/mysql/mariadb_integration_test.go` or equivalent smoke coverage — MariaDB confidence gap
- [ ] `internal/cli/table_selection_test.go` — dependency-aware selection flow and profile round-trip coverage

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Large-schema interactive selection remains understandable in a real terminal | TABL-02 | Prompt ergonomics and terminal readability are hard to prove through unit tests alone | Run the interactive profile flow against a fixture schema with many tables, verify search/filtering and dependency summaries remain readable. |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [x] Feedback latency < 120s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending