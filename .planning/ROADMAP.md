# Roadmap: DB Sync CLI

## Overview

The MVP starts by establishing a reliable CLI foundation and saved profile model, then adds schema and dependency discovery, preflight safety checks, high-throughput sync execution, rollback support, and final UX/performance hardening. The sequence is designed so that every later phase builds on validated operator safety and data integrity assumptions from the earlier ones.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

- [ ] **Phase 1: Foundation CLI** - Establish the project structure, saved profile workflow, and connection validation basics.
- [ ] **Phase 2: Schema And Dependency Discovery** - Introspect databases, analyze drift, and make table selection dependency-aware.
- [ ] **Phase 3: Preflight Scan Engine** - Surface blockers and remediation guidance before any live writes occur.
- [ ] **Phase 4: High-Throughput Sync Execution** - Implement dependency-safe large-scale sync behavior for inserts and mirror deletes.
- [ ] **Phase 5: Rollback And Run Audit** - Prepare recovery artifacts and provide failure-time restoration support.
- [ ] **Phase 6: Operator UX And Hardening** - Polish terminal UX, reporting, packaging, and performance confidence.

## Phase Details

### Phase 1: Foundation CLI
**Goal**: Create the core CLI application, interactive profile setup flow, and source/target connection validation.
**Depends on**: Nothing (first phase)
**Requirements**: PROF-01, PROF-02, PROF-03
**Success Criteria** (what must be TRUE):
  1. Operator can start the CLI and create a reusable sync profile interactively.
  2. Operator can validate source and target connectivity before saving the profile.
  3. Profile data model is ready for later phases to extend with scan and sync options.
**Plans**: 3 plans

Plans:
- [ ] 01-01: Define CLI architecture, command layout, and profile persistence model
- [ ] 01-02: Implement interactive profile setup and editing flow
- [ ] 01-03: Implement connection validation and profile save/load behavior

### Phase 2: Schema And Dependency Discovery
**Goal**: Understand source and target schemas well enough to explain drift and table dependencies before sync planning.
**Depends on**: Phase 1
**Requirements**: SCMA-01, SCMA-02, SCMA-03, SCMA-04, SCMA-05, TABL-01, TABL-02, TABL-03
**Success Criteria** (what must be TRUE):
  1. Tool can inspect supported databases and compare source and target table structures.
  2. Operator can see dependency-aware table choices instead of guessing required foreign tables.
  3. Schema drift results are classified clearly enough for scan and live run workflows to consume.
**Plans**: 3 plans

Plans:
- [ ] 02-01: Build schema introspection adapters for supported databases
- [ ] 02-02: Build drift analysis and compatibility classification
- [ ] 02-03: Build dependency graphing and interactive table selection model

### Phase 3: Preflight Scan Engine
**Goal**: Let operators detect blockers, warnings, and remediation steps without changing live target data.
**Depends on**: Phase 2
**Requirements**: SCAN-01, SCAN-02, SCAN-03, SCAN-04, UX-02
**Success Criteria** (what must be TRUE):
  1. Operator can run scan mode for a selected profile and receive a no-write safety report.
  2. Scan output identifies blocked writes caused by constraints, missing required values, or dependency issues.
  3. Scan summaries provide clear next steps before a live run is attempted.
**Plans**: 3 plans

Plans:
- [ ] 03-01: Design scan result model and table-level reporting pipeline
- [ ] 03-02: Implement constraint and writeability checks using schema and data sampling rules
- [ ] 03-03: Build remediation messaging and scan summaries for operators

### Phase 4: High-Throughput Sync Execution
**Goal**: Execute fast, dependency-safe sync runs for large tables without exhausting memory.
**Depends on**: Phase 3
**Requirements**: SYNC-01, SYNC-02, SYNC-03, SYNC-05, PERF-02, PERF-03
**Success Criteria** (what must be TRUE):
  1. Tool can sync selected tables in dependency-safe order from source to target.
  2. Large tables are processed in chunks or batches rather than full in-memory loads.
  3. Insert-missing and optional mirror-delete behaviors work according to the selected profile.
**Plans**: 4 plans

Plans:
- [ ] 04-01: Design execution planner for table ordering, batching, and checkpoints
- [ ] 04-02: Implement high-throughput read and insert pipelines
- [ ] 04-03: Implement mirror-delete behavior with safety guards
- [ ] 04-04: Add progress-state tracking to support resumable execution design

### Phase 5: Rollback And Run Audit
**Goal**: Ensure live runs create recovery information and clear audit records before applying writes.
**Depends on**: Phase 4
**Requirements**: ROLL-01, ROLL-02, ROLL-03, UX-03
**Success Criteria** (what must be TRUE):
  1. Live sync preparation creates rollback artifacts before the first target write occurs.
  2. Failed runs leave operators with concrete restore guidance and a record of what changed.
  3. Run metadata is durable enough to support auditing and recovery workflows.
**Plans**: 3 plans

Plans:
- [ ] 05-01: Define rollback artifact format and recovery workflow
- [ ] 05-02: Implement failure interception and restore guidance flow
- [ ] 05-03: Persist run manifests, audit summaries, and recovery metadata

### Phase 6: Operator UX And Hardening
**Goal**: Make the tool operationally clear, polished, and credible for large production-style syncs.
**Depends on**: Phase 5
**Requirements**: PERF-01, UX-01, SYNC-04
**Success Criteria** (what must be TRUE):
  1. Interactive CLI presents progress bars, colored summaries, and readable status output throughout scan and live runs.
  2. Performance validation demonstrates the architecture is suitable for large-table sync targets.
  3. The packaged tool feels ready for repeated operator use rather than an internal prototype.
**Plans**: 3 plans

Plans:
- [ ] 06-01: Polish terminal UX and progress presentation across workflows
- [ ] 06-02: Add benchmarks and performance validation scenarios
- [ ] 06-03: Package the CLI and finalize operator-facing run/report output

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation CLI | 0/3 | Not started | - |
| 2. Schema And Dependency Discovery | 0/3 | Not started | - |
| 3. Preflight Scan Engine | 0/3 | Not started | - |
| 4. High-Throughput Sync Execution | 0/4 | Not started | - |
| 5. Rollback And Run Audit | 0/3 | Not started | - |
| 6. Operator UX And Hardening | 0/3 | Not started | - |