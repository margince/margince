// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The second audit door. `before = nil` on an update answers two different
// questions with one spelling — "the fields held nothing" and "nobody looked" —
// and no reader can tell them apart. AuditEvent is how a writer says the first
// one, so the absence stops being a default nobody chose.

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// AuditEvent records a mutation that has no prior field state: a secret
// rotated, a delivery replayed, a tag applied to a record. `before` is SQL NULL
// because there is nothing the fields held before — not because nobody looked.
//
// A write that changes a record's own field values uses Audit and records them.
// auditbeforeimage_test.go holds the difference, and censuses every caller of
// this function that audits an update, so choosing it is a claim someone can
// read back.
//
//craft:ignore naked-any the audit seam: an after image is the entity's own snapshot shape, serialized to jsonb
func AuditEvent(ctx context.Context, tx pgx.Tx, action, entityType string, entityID ids.UUID, after any) (ids.UUID, error) {
	return AuditEventWithEvidence(ctx, tx, action, entityType, entityID, after, nil)
}

// AuditEventWithEvidence is AuditEvent for a write that also carries
// operational evidence — context ABOUT the mutation, landing in
// audit_log.evidence rather than in the images.
//
//craft:ignore naked-any the audit seam: an after image is the entity's own snapshot shape, serialized to jsonb
func AuditEventWithEvidence(ctx context.Context, tx pgx.Tx, action, entityType string, entityID ids.UUID, after any, evidence map[string]any) (ids.UUID, error) {
	// Straight to the writer, never through AuditWithEvidence: that door
	// refuses an update carrying no before-image, which is exactly what every
	// caller here is. Routing through it would make the declared way of
	// recording an occurrence the one way that cannot be recorded.
	return writeAuditRow(ctx, tx, action, entityType, entityID, nil, after, evidence)
}

// writeAuditRow is this package's INSERT into audit_log, and the one every door
// here reaches: Audit and AuditWithEvidence after they have judged the
// before-image, AuditEvent without one to judge.
//
// Not the only writer of the table. The approvals module sends its own INSERT
// for the approval row's own lifecycle — a different subject, with no field
// images at all — so a rule enforced here binds every caller of these doors and
// nothing else. auditbeforeimage_test.go sweeps for the direct writers so that
// stays a fact somebody checked rather than a claim standing in a comment.
//
//craft:ignore naked-any the audit seam: before/after images are each entity's own snapshot shape, serialized to jsonb
func writeAuditRow(ctx context.Context, tx pgx.Tx, action, entityType string, entityID ids.UUID, before, after any, evidence map[string]any) (ids.UUID, error) {
	p, err := Actor(ctx)
	if err != nil {
		return ids.Nil, err
	}

	beforeJSON, err := marshalOrNil(before)
	if err != nil {
		return ids.Nil, err
	}
	afterJSON, err := marshalOrNil(after)
	if err != nil {
		return ids.Nil, err
	}
	evidence, err = withExtensionAttribution(ctx, evidence)
	if err != nil {
		return ids.Nil, err
	}
	var evidenceJSON []byte
	if evidence != nil {
		evidenceJSON, err = json.Marshal(evidence)
		if err != nil {
			return ids.Nil, err
		}
	}

	id := ids.NewV7()
	_, err = tx.Exec(ctx,
		// No tenant column. It came from the TRANSACTION's binding until
		// ADR-0091 §8 phase D reached the ledgers — the last two tables that
		// carried one — so an audit row now names WHAT happened and WHO did it,
		// and the installation is the only answer to where.
		`INSERT INTO audit_log (id, actor_type, actor_id, passport_id, on_behalf_of, action, entity_type, entity_id, before, after, evidence, authorization_rule)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		id, string(p.Type), p.ID, UUIDOrNil(p.PassportID), UUIDOrNil(p.OnBehalfOf),
		action, entityType, entityID, beforeJSON, afterJSON, evidenceJSON,
		auth.AuthzRule(p, entityType, action))
	return id, err
}
