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
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// writeAuditRow writes one row through insertAuditRows, which every door here
// reaches: Audit and AuditWithEvidence after they have judged the before-image,
// AuditEvent without one to judge, and AuditEach for many rows.
//
// The table takes writes from elsewhere too — approvals sends its own statement
// for the approval row's lifecycle, a different subject carrying no field images
// at all — so a rule enforced here binds callers of these doors and nothing
// further. auditbeforeimage_test.go censuses the direct writers, which is what
// keeps that a fact somebody checked rather than a comment nobody can.
//
//craft:ignore naked-any the audit seam: before/after images are each entity's own snapshot shape, serialized to jsonb
func writeAuditRow(ctx context.Context, tx pgx.Tx, action, entityType string, entityID ids.UUID, before, after any, evidence map[string]any) (ids.UUID, error) {
	row, err := prepareAuditRow(ctx, entityType, entityID, before, after, evidence)
	if err != nil {
		return ids.Nil, err
	}
	return row.id, insertAuditRows(ctx, tx, action, entityType, []auditRow{row})
}

// auditRow is one audit_log row ready to insert: its images and evidence
// marshalled, linked and masked.
type auditRow struct {
	id, entityID            ids.UUID
	before, after, evidence []byte
}

//craft:ignore naked-any the audit seam: before/after images are each entity's own snapshot shape, serialized to jsonb
func prepareAuditRow(ctx context.Context, entityType string, entityID ids.UUID, before, after any, evidence map[string]any) (auditRow, error) {
	row := auditRow{id: ids.NewV7(), entityID: entityID}
	var err error
	if row.before, err = marshalOrNil(before); err != nil {
		return auditRow{}, err
	}
	if row.after, err = marshalOrNil(after); err != nil {
		return auditRow{}, err
	}
	evidence, err = withExtensionAttribution(ctx, withReleasedByLink(ctx, entityID, withReversalLink(ctx, entityType, entityID, evidence)))
	if err != nil {
		return auditRow{}, err
	}
	if evidence != nil {
		if row.evidence, err = json.Marshal(evidence); err != nil {
			return auditRow{}, err
		}
	}
	if err := maskAuditColumns(entityType, &row.before, &row.after, &row.evidence); err != nil {
		return auditRow{}, err
	}
	return row, nil
}

// insertAuditRows is this package's INSERT into audit_log: rows of one action
// and entity type, by the bound actor, in one statement.
func insertAuditRows(ctx context.Context, tx pgx.Tx, action, entityType string, rows []auditRow) error {
	p, err := Actor(ctx)
	if err != nil {
		return err
	}
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	shared := strings.Join([]string{
		arg(string(p.Type)), arg(p.ID), arg(UUIDOrNil(p.PassportID)), arg(UUIDOrNil(p.OnBehalfOf)),
		arg(action), arg(entityType),
	}, ", ")
	tail := arg(auth.AuthzRule(p, entityType, action)) + ", " + arg(batchOf(ctx))
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		values = append(values, "("+arg(row.id)+", "+shared+", "+arg(row.entityID)+", "+
			arg(row.before)+", "+arg(row.after)+", "+arg(row.evidence)+", "+tail+")")
	}
	// No tenant column: the installation is the only answer to where. The table
	// name is the constant the fault classifier reads, so a rename moves both.
	_, err = tx.Exec(ctx, `INSERT INTO `+TableAudit+` (id, actor_type, actor_id, passport_id, on_behalf_of, action, entity_type, entity_id, before, after, evidence, authorization_rule, batch_id)
		 VALUES `+strings.Join(values, ", "), args...)
	return err
}
