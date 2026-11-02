// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	threadBasisArg        = "basis"
	basisAuditField       = "communication_basis"
	basisAuditIDField     = "basis_id"
	basisSourceAuditField = "source_activity_id"
	basisStatusAuditField = "communication_basis_status"
)

// ThreadBasisCopyTx retains one currently active ground on both partitions.
// Its source identity makes replay idempotent even after a partial repair.
func ThreadBasisCopyTx(ctx context.Context, tx pgx.Tx, basisID ids.UUID, from, to string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if from == "" || to == "" || from == to {
		return nil
	}
	live, err := lockThreadBasisSourceTx(ctx, tx, basisID)
	if err != nil || !live {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	var copied, origin ids.UUID
	var proofSource *ids.UUID
	var contact, lead *ids.UUID
	var kind string
	err = tx.QueryRow(ctx, `INSERT INTO communication_basis
 (contact_id,lead_id,kind,thread_key,source_activity_id,valid_from,valid_until,captured_by,thread_derived_from,thread_source_activity_id)
 SELECT b.contact_id,b.lead_id,b.kind,@to,b.source_activity_id,b.valid_from,b.valid_until,@by,coalesce(b.thread_derived_from,b.id),coalesce(b.thread_source_activity_id,b.source_activity_id)
 FROM communication_basis b WHERE b.id=@basis AND b.thread_key=@from
 AND (coalesce(b.thread_source_activity_id,b.source_activity_id) IS NULL OR EXISTS(SELECT 1 FROM activity source
 WHERE source.id=coalesce(b.thread_source_activity_id,b.source_activity_id)
 AND source.archived_at IS NULL AND source.restricted_at IS NULL AND NOT (source.redacted_fields && ARRAY['subject','body']::text[])
 AND (source.subject IS NOT NULL OR source.body IS NOT NULL)))
 AND `+threadBasisOriginLiveSQL("b")+`
 AND b.revoked_at IS NULL AND b.valid_from<=now() AND (b.valid_until IS NULL OR b.valid_until>now())
 AND NOT EXISTS (SELECT 1 FROM communication_basis d WHERE d.thread_key=@to
 AND d.contact_id IS NOT DISTINCT FROM b.contact_id AND d.lead_id IS NOT DISTINCT FROM b.lead_id
 AND d.kind=b.kind AND d.source_activity_id IS NOT DISTINCT FROM b.source_activity_id
 AND d.valid_from=b.valid_from AND d.revoked_at IS NULL)
 RETURNING id,thread_derived_from,thread_source_activity_id,contact_id,lead_id,kind`, pgx.NamedArgs{threadBasisArg: basisID, "from": from, "to": to, "by": by}).Scan(&copied, &origin, &proofSource, &contact, &lead, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("consent: inheriting a conversation's active ground: %w", err)
	}
	entity, subject := entityContact, contact
	if subject == nil {
		entity, subject = entityLead, lead
	}
	if subject == nil {
		return errors.New("consent: inherited ground has no subject")
	}
	_, err = storekit.AuditEvent(ctx, tx, "update", entity, *subject, map[string]any{basisAuditField: kind, "thread_key": to, basisAuditIDField: copied, "derived_from_basis_id": origin, basisSourceAuditField: proofSource})
	return err
}

// InvalidateThreadBasisTx retracts one ground whose source evidence was erased.
// Source-erasure audits cover hard-delete FK revocation; basis audits retain the source and ground identities.
func InvalidateThreadBasisTx(ctx context.Context, tx pgx.Tx, source ids.ActivityID) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	var revoked ids.UUID
	var contact, lead *ids.UUID
	var kind string
	err := tx.QueryRow(ctx, `UPDATE communication_basis SET revoked_at=coalesce(revoked_at,now()),thread_source_activity_id=NULL,source_activity_id=NULL,note=NULL WHERE id=(
 SELECT id FROM communication_basis WHERE coalesce(thread_source_activity_id,source_activity_id)=@source ORDER BY id LIMIT 1)
 RETURNING id,contact_id,lead_id,kind`, pgx.NamedArgs{carrySourceArg: source}).Scan(&revoked, &contact, &lead, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("consent: invalidating erased correspondence proof: %w", err)
	}
	entity, subject := entityContact, contact
	if subject == nil {
		entity, subject = entityLead, lead
	}
	if subject == nil {
		return false, errors.New("consent: invalidated ground has no subject")
	}
	_, err = storekit.AuditEvent(ctx, tx, "update", entity, *subject, map[string]any{basisAuditField: kind, basisStatusAuditField: "source_erased", basisAuditIDField: revoked, basisSourceAuditField: source.UUID})
	return true, err
}

func threadBasisOriginLiveSQL(alias string) string {
	return "(" + alias + ".thread_derived_from IS NULL OR EXISTS(SELECT 1 FROM communication_basis original WHERE original.id=" + alias + ".thread_derived_from AND original.revoked_at IS NULL AND original.valid_from<=now() AND (original.valid_until IS NULL OR original.valid_until>now())))"
}

func lockThreadBasisSourceTx(ctx context.Context, tx pgx.Tx, basis ids.UUID) (bool, error) {
	var source, contact, lead *ids.UUID
	err := tx.QueryRow(ctx, `SELECT coalesce(thread_source_activity_id,source_activity_id),contact_id,lead_id FROM communication_basis WHERE id=@basis`, pgx.NamedArgs{threadBasisArg: basis}).Scan(&source, &contact, &lead)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if contact != nil {
		if err := auth.EnsureVisibleForSubjectRights(ctx, tx, "contact", *contact); err != nil {
			return false, err
		}
	}
	if lead != nil {
		if err := auth.EnsureVisibleForSubjectRights(ctx, tx, "lead", *lead); err != nil {
			return false, err
		}
	}
	var locked ids.UUID
	if source != nil {
		err = tx.QueryRow(ctx, `SELECT id FROM activity WHERE id=@source AND archived_at IS NULL AND restricted_at IS NULL AND NOT (redacted_fields && ARRAY['subject','body']::text[]) AND (subject IS NOT NULL OR body IS NOT NULL) FOR UPDATE`, pgx.NamedArgs{carrySourceArg: *source}).Scan(&locked)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
	err = tx.QueryRow(ctx, `SELECT original.id FROM communication_basis b JOIN communication_basis original ON original.id=coalesce(b.thread_derived_from,b.id) WHERE b.id=@basis AND original.revoked_at IS NULL AND original.valid_from<=now() AND (original.valid_until IS NULL OR original.valid_until>now()) FOR SHARE OF b,original`, pgx.NamedArgs{threadBasisArg: basis}).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
