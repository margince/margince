// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *Eraser) purgePartyProjections(ctx context.Context, tx pgx.Tx, emails []string) error {
	if len(emails) == 0 {
		return nil
	}
	if e.purgeRawCaptures != nil {
		return e.purgeRawCaptures.PurgePartySourcesForEmailsTx(ctx, tx, emails)
	}
	var held bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capture_party_source WHERE recipients && ARRAY(SELECT lower(btrim(address)) FROM unnest(@emails::text[]) address) OR policy_addresses && ARRAY(SELECT lower(btrim(address)) FROM unnest(@emails::text[]) address))`, pgx.NamedArgs{"emails": emails}).Scan(&held); err != nil {
		return err
	}
	if held {
		return ErrRetentionSeamMissing
	}
	return nil
}

func (e *Eraser) purgeErasedActivityProjections(ctx context.Context, tx pgx.Tx, activities []ids.UUID) error {
	return purgeErasedActivityProjectionsTx(ctx, tx, activities, e.purgeRawCaptures)
}

func purgeErasedActivityProjectionsTx(ctx context.Context, tx pgx.Tx, activities []ids.UUID, purge RawCapturePurger) error {
	if len(activities) == 0 {
		return nil
	}
	if rawCapturePurgerMissing(purge) {
		purge = nil
	}
	if purge != nil {
		return purge.PurgeActivityProjectionsTx(ctx, tx, activities)
	}
	var held bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capture_thread_topic WHERE activity_id=ANY(@activities::uuid[]))
 OR EXISTS(SELECT 1 FROM capture_thread_repair_node WHERE activity_id=ANY(@activities::uuid[]))
 OR EXISTS(SELECT 1 FROM capture_thread_reply WHERE activity_id=ANY(@activities::uuid[]))
 OR EXISTS(SELECT 1 FROM capture_thread_invalidation WHERE activity_id=ANY(@activities::uuid[]))
 OR EXISTS(SELECT 1 FROM capture_import WHERE activity_id=ANY(@activities::uuid[]) AND thread_input_hash<>'')`, pgx.NamedArgs{"activities": activities}).Scan(&held)
	if err != nil {
		return err
	}
	if held {
		return ErrRetentionSeamMissing
	}
	return nil
}
