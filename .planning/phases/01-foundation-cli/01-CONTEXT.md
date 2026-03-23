# Phase 1: Foundation CLI - Context

**Gathered:** 2026-03-23
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 1 establishes the core CLI application, the saved profile workflow, and connection validation basics. This phase is about how operators create, edit, and validate reusable sync profiles, not about schema analysis, scan mode, or live sync execution.

</domain>

<decisions>
## Implementation Decisions

### CLI command shape
- **D-01:** V1 should be wizard-first. Operators should have a guided setup entrypoint for creating profiles, with scan and run behavior added as separate commands in later phases.
- **D-02:** The CLI should optimize for human-operated workflows first rather than a flags-only interface.

### Profile storage and secrets
- **D-03:** Profiles should be stored per user in the OS config directory as YAML files.
- **D-04:** Profiles must not store raw secrets directly.
- **D-05:** Credential handling should prefer DSN templates with placeholders so runtime resolution can pull values from the environment.

### Interactive setup flow
- **D-06:** The setup experience should optimize for a fast path first rather than a long explicit wizard.
- **D-07:** Editing an existing profile should reopen the same wizard flow with current values prefilled.
- **D-08:** The workflow should still support a review-before-save step even though the general interaction style is fast-path oriented.

### Connection validation behavior
- **D-09:** Profile save should be blocked until validation succeeds for both source and target connections.
- **D-10:** Validation must confirm source authentication, target authentication, source/target schema metadata access, and a lightweight non-mutating target capability check.
- **D-11:** Validation in Phase 1 should not depend on early table selection.

### the agent's Discretion
- Exact command names and flag aliases within the wizard-first model.
- Exact YAML schema layout, as long as it supports placeholders and later extension.
- Exact wording and visual presentation of prompts, review screens, and validation output.

</decisions>

<specifics>
## Specific Ideas

- Fast-path setup matters more than an overly ceremonial wizard.
- Saved profiles should feel reusable and editable, not disposable one-off runs.
- Secret handling should keep plain credentials out of profile files while remaining simple for developers.

</specifics>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project scope
- `.planning/PROJECT.md` — Product framing, constraints, and locked project-level decisions for the DB sync CLI.
- `.planning/REQUIREMENTS.md` — Phase 1 requirement IDs PROF-01 through PROF-03 and adjacent product constraints.
- `.planning/ROADMAP.md` — Phase 1 goal, success criteria, and plan placeholders that bound this discussion.

### External specs
- No external specs — requirements are fully captured in the decisions above and the project planning documents.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- None yet — the repository is greenfield.

### Established Patterns
- None yet — Phase 1 will define the foundational CLI and profile-management patterns.

### Integration Points
- None yet beyond the `.planning` artifacts; Phase 1 will establish the first application structure.

</code_context>

<deferred>
## Deferred Ideas

- Scan-only behavior and live run command semantics belong to later phases.
- Non-interactive automation-first command design is deferred until later automation scope is planned.
- Alternative secret providers such as OS keychain support may be revisited in a future phase if environment-placeholder references are insufficient.

</deferred>

---

*Phase: 01-foundation-cli*
*Context gathered: 2026-03-23*