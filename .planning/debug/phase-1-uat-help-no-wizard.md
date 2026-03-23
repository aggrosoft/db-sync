---
status: diagnosed
trigger: "Diagnose Phase 1 UAT gap: running `go run ./cmd/db-sync` reportedly prints help instead of entering the wizard. Goal: find_root_cause_only."
created: 2026-03-23T00:00:00Z
updated: 2026-03-23T00:00:00Z
---

## Current Focus

hypothesis: Confirmed: the observed UAT failure came from terminal-detection behavior in the execution environment, not from the wizard path being missing.
test: Complete. The exact `term.IsTerminal(0)` probe was run in the same terminal context.
expecting: n/a
next_action: Return diagnosis only.

## Symptoms

expected: Running `go run ./cmd/db-sync` in an interactive terminal should enter the wizard and reach review-before-save.
actual: User reported that it prints help and does not start the wizard.
errors: none reported
reproduction: Test 1 in `.planning/phases/01-foundation-cli/01-UAT.md`
started: discovered during UAT

## Eliminated

## Evidence

- timestamp: 2026-03-23T00:00:00Z
	checked: .planning/phases/01-foundation-cli/01-UAT.md
	found: Test 1 explicitly expects wizard startup only in an interactive terminal, while the report says "it prints help - no wizard."
	implication: The failure condition is specifically about interactive-terminal detection, not generic root command routing.

- timestamp: 2026-03-23T00:00:00Z
	checked: internal/cli/root.go
	found: The root `RunE` calls `cmd.Help()` whenever `term.IsTerminal(0)` is false, and only calls `app.StartInteractiveProfile` when stdin is a terminal.
	implication: Help output is the intended behavior for non-interactive stdin.

- timestamp: 2026-03-23T00:00:00Z
	checked: `go run ./cmd/db-sync` in the tool-managed terminal
	found: The command printed Cobra help instead of starting the wizard.
	implication: This execution context followed the non-interactive branch in `internal/cli/root.go`.

- timestamp: 2026-03-23T00:00:00Z
	checked: .planning/phases/01-foundation-cli/01-VERIFICATION.md and 01-RESEARCH.md
	found: Phase docs consistently define the default no-arg flow as wizard-first only when stdin is interactive, and leave full interactive UX to human verification in a real terminal.
	implication: The implementation and the documented acceptance criteria both depend on true interactive-terminal conditions.

- timestamp: 2026-03-23T00:00:00Z
	checked: PowerShell console state
	found: `[Console]::IsInputRedirected` returned `False` in the same terminal session.
	implication: Simple shell-level input redirection is not the cause; the mismatch is lower-level terminal capability detection.

- timestamp: 2026-03-23T00:00:00Z
	checked: `golang.org/x/term` probe in the same terminal session
	found: `go run ./.planning/debug/isatty_probe.go` printed `false`.
	implication: The exact terminal check used by `internal/cli/root.go` evaluates this session as non-interactive and therefore intentionally prints help.

## Resolution

root_cause: 
The UAT observation was taken in a terminal context that `golang.org/x/term` did not recognize as an interactive TTY on Windows, so `internal/cli/root.go` intentionally executed `cmd.Help()` instead of `app.StartInteractiveProfile`. The gap is an environment-sensitive terminal-detection mismatch, not a missing wizard implementation.
fix: 
verification: 
files_changed: []
