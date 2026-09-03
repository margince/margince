// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

// The events that name no record, and why each one is allowed to.
//
// Split from catalog.go rather than living beside the type map: the catalog
// answers "what is this event and where does it ride", and this answers a
// different question — "why does this one hand a consumer nothing to read
// back". Keeping them together made one file two subjects, and the second is
// the one a reviewer has to think hardest about.

// pipelineEventTypes are the events that may ride the bus WITHOUT a subject
// entity ref, because the thing they report names no record.
//
// Two families qualify, for the same reason. A capture pipeline step can be
// subject-less by nature — capture.skipped names NOTHING (an excluded personal
// message creates no row), yet the spec still requires it on the bus as the
// machine-checkable "personal mail is never ingested" proof (capture.md AC1.3,
// EVT-SEM-10). An AI task's state change names no record either: the occurrence
// it reports is operational state, and the row that will hold it does not exist
// until the projection this event feeds writes it — so there is nothing for a
// consumer to read back under its own scope, which is what an entity ref is
// for. These events carry no entity handle, but they DO keep the ledger trace
// link (audit_log OR system_log) so the outcome stays attributable — Validate
// enforces the trace, only the entity is relaxed.
var pipelineEventTypes = map[string]struct{}{
	"capture.received":      {},
	"capture.normalized":    {},
	"capture.failed":        {},
	"capture.skipped":       {},
	"ai_task.state_changed": {},
	// A Brief open names no record it could hand a consumer: the subject is the
	// READING, not the run, and the run is the reader's own queue. Entity-less
	// is also what keeps it off the subscribable set — see its schema in
	// api/internal-events.yaml.
	"brief.opened": {},
}
