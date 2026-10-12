// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// Linking a module's own write to the history entry it reverses.
//
// Undoing a create archives the record, undoing an archive un-archives it, and
// undoing a promotion demotes the lead. Each of those is the owning module's
// own verb, and each writes its own audit row. The row has to name the entry it
// reverses, or the history cannot say "undone" and a second press would run the
// reversal again.
//
// The link rides the context for the reason WithBatch does: threading an
// evidence parameter through every archive, un-archive and demote writer would
// be a parameter each new writer has to remember.

import (
	"context"
	"maps"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EvidenceKeyUndidAuditLog names the audit row a reversal undoes. The history
// read follows it from the reversal back to the entry.
const EvidenceKeyUndidAuditLog = "undid_audit_log_id"

type reversalKey struct{}

type reversal struct {
	entityType string
	entityID   ids.UUID
	undid      ids.UUID
}

// WithReversal marks the audit row written under ctx for this record as the
// reversal of the entry `undid`. Rows about other records are left alone: a
// demotion that archives the contact it created reverses the LEAD's promotion,
// and only the lead's own row says so.
func WithReversal(ctx context.Context, entityType string, entityID, undid ids.UUID) context.Context {
	return context.WithValue(ctx, reversalKey{}, reversal{entityType: entityType, entityID: entityID, undid: undid})
}

// withReversalLink adds the link to a row about the marked record. A caller
// that already named the entry it reverses keeps its own value.
//
//craft:ignore naked-any the audit evidence seam is jsonb; its members are each writer's own shape
func withReversalLink(ctx context.Context, entityType string, entityID ids.UUID, evidence map[string]any) map[string]any {
	marked, ok := ctx.Value(reversalKey{}).(reversal)
	if !ok || marked.entityType != entityType || marked.entityID != entityID {
		return evidence
	}
	if _, named := evidence[EvidenceKeyUndidAuditLog]; named {
		return evidence
	}
	linked := make(map[string]any, len(evidence)+1)
	maps.Copy(linked, evidence)
	linked[EvidenceKeyUndidAuditLog] = marked.undid.String()
	return linked
}
