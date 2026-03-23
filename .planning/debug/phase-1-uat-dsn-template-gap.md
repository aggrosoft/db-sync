---
status: diagnosed
trigger: "Diagnose the Phase 1 UAT gap in c:\\work\\db-sync. Goal is find_root_cause_only, not apply fixes. Gap: profile creation or edit flow should let the operator connect the database easily without forcing DSN template authoring. Actual: creating a profile asked for a dsn template instead of accepting a connection string or guided connection details and writing env-backed config. Severity: major. Reproduction: Test 2 in .planning/phases/01-foundation-cli/01-UAT.md. Timeline: discovered during UAT."
created: 2026-03-23T00:00:00Z
updated: 2026-03-23T00:00:00Z
---

## Current Focus

hypothesis: Confirmed: Phase 1 hard-coded DSN-template authoring into the product contract and interactive UX, so operators are forced into placeholder/template thinking instead of simple connection setup.
test: Completed by tracing CLI -> wizard -> model -> store -> validation and running focused tests that exercise placeholder DSN behavior.
expecting: N/A
next_action: return structured root-cause diagnosis to the user

## Symptoms

expected: The profile creation or edit flow should let the operator connect the database easily without forcing DSN template authoring, ideally by accepting a connection string or guided connection details and writing env-backed config for them.
actual: Creating a profile asked for a DSN template; the user had to think in terms of dynamic template authoring instead of simply entering a connection string or connection details.
errors: No runtime crash reported; the gap is a UX and capability mismatch surfaced during UAT.
reproduction: Test 2 in .planning/phases/01-foundation-cli/01-UAT.md.
started: Discovered during Phase 1 UAT.

## Eliminated

## Evidence

- timestamp: 2026-03-23T00:00:00Z
	checked: .planning/phases/01-foundation-cli/01-UAT.md
	found: Test 2 and the corresponding gap explicitly document the failure as being asked for a DSN template instead of a connection string or guided connection details.
	implication: The UAT issue is specifically about the setup model, not a validation error or later persistence bug.

- timestamp: 2026-03-23T00:00:00Z
	checked: .planning/phases/01-foundation-cli/01-CONTEXT.md
	found: Decision D-05 states that credential handling should prefer DSN templates with placeholders so runtime resolution can pull values from the environment.
	implication: The requirement/context itself biased implementation toward placeholder-backed DSN templates rather than direct connection input.

- timestamp: 2026-03-23T00:00:00Z
	checked: internal/wizard/flow.go, internal/wizard/draft.go, internal/wizard/messages.go
	found: The wizard prompts only for `Source DSN template` and `Target DSN template`, the draft stores only those fields, and helper copy explains placeholder usage as the intended operator experience.
	implication: The interactive profile creation/edit flow has no branch for connection strings or guided host/user/password capture.

- timestamp: 2026-03-23T00:00:00Z
	checked: internal/model/profile.go
	found: Each endpoint contains only `engine` and `dsn_template`; there are no fields for connection string, host, port, database, username, or env-file metadata.
	implication: The persisted profile schema itself cannot represent the easier connection flow requested by UAT.

- timestamp: 2026-03-23T00:00:00Z
	checked: internal/secrets/template.go, internal/validate/service.go
	found: Validation requires template parsing/resolution and enforces that each DSN template includes at least one `${NAME}` placeholder for secret values.
	implication: Even a raw connection string is intentionally rejected by the current validation layer, confirming the DSN-template abstraction is enforced end-to-end.

- timestamp: 2026-03-23T00:00:00Z
	checked: internal/cli/app.go, internal/profile/filesystem_store.go, internal/profile/schema.go
	found: The CLI new/edit commands only call the wizard, saving routes through `ValidateAndSave`, and the filesystem store re-applies placeholder policy before writing the YAML profile.
	implication: There is no alternate command or persistence path that could accept simpler connection input; the DSN-template requirement is structural.

- timestamp: 2026-03-23T00:00:00Z
	checked: internal/wizard/review.go, internal/wizard/wizard_test.go, internal/profile/filesystem_store_test.go, internal/validate/service_test.go
	found: Review rendering highlights placeholders, wizard tests assert DSN-template-prefill behavior, persistence tests reject raw-secret DSNs, and validation tests use placeholder-backed DSNs as the supported happy path.
	implication: The codebase's expected behavior and tests were written around DSN templates, so the UAT gap comes from the chosen implementation model rather than an accidental regression.

## Resolution

root_cause: Phase 1 encoded a placeholder-backed `dsn_template` model as the only supported connection representation, then wired the wizard, review UX, validation, and filesystem store around that abstraction. Because the profile schema has no fields for direct connection strings or guided connection details, the create/edit flow can only ask operators to author DSN templates.
fix:
verification: Required files inspected end-to-end; CLI/profile path traced; focused tests passed in internal/profile/filesystem_store_test.go, internal/wizard/wizard_test.go, and internal/validate/service_test.go.
files_changed: []