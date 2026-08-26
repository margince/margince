// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/auditverb"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// AuditVerb is shared/kernel/auditverb.Verb under this package's name. The type
// is declared in the kernel because the provider seam carries it and the layer
// DAG forbids that seam importing this package; aliasing rather than redeclaring
// keeps a verb set on an UpdateInput and a verb written to audit_log the same
// value, not two that a conversion could silently disagree about.
type AuditVerb = auditverb.Verb

const (
	VerbUpdate  = auditverb.Update
	VerbRestore = auditverb.Restore
)

// AuditTrail is shared/kernel/auditverb.Trail under this package's name: the
// verb and the evidence an update-shaped write carries, which every record
// type's update input embeds and hands to the door here.
type AuditTrail = auditverb.Trail

// AuditWithTrail is Audit for a write whose verb the caller carries rather than
// spells. It exists so the six record types thread the reversal path in one
// line each: an update and a restore differ only in what the trail calls them,
// and a per-store branch on that would be six copies of one decision.
//
// Every call here is update-shaped by construction — Trail.Resolve admits two
// verbs and nothing else — so the before-image rule binds all of them, and
// auditbeforeimage_test.go judges these sites instead of ratifying them the way
// it must ratify a verb it cannot reduce.
//
//craft:ignore naked-any the audit seam: before/after images are each entity's own snapshot shape, serialized to jsonb
func AuditWithTrail(ctx context.Context, tx pgx.Tx, trail AuditTrail, entityType string, entityID ids.UUID, before, after any) (ids.UUID, error) {
	action, evidence, err := trail.Resolve()
	if err != nil {
		return ids.Nil, err
	}
	return AuditWithEvidence(ctx, tx, action, entityType, entityID, before, after, evidence)
}
