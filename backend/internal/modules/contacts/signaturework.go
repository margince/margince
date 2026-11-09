// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const signatureReopenSetSQL = `activity_id=NULL, last_activity_at=NULL, history_depth=NULL,
 live_generation=st.requested_generation+1, requested_generation=st.requested_generation+1, due_at=now(), deferred_run_id=NULL`

// SignatureWork is one durable request observed before a signature read.
type SignatureWork struct {
	ContactID  ids.ContactID
	Generation int64
	Snapshot   string
}

// QueueSignatureWorkTx preserves a live request when historical mail arrives behind it.
func QueueSignatureWorkTx(ctx context.Context, tx pgx.Tx, contacts []ids.ContactID, deferredRun *ids.UUID) error {
	_, err := QueueSignatureEventTx(ctx, tx, contacts, SignatureRequest{DeferredRun: deferredRun, Live: deferredRun == nil})
	return err
}

// SignatureRequest distinguishes live work from a released historical request.
type SignatureRequest struct {
	DeferredRun *ids.UUID
	SourceXID   string
	Reopen      bool
	Live        bool
}

// QueueSignatureEventTx skips changes included in the last read and reopens newly eligible sources.
func QueueSignatureEventTx(ctx context.Context, tx pgx.Tx, contacts []ids.ContactID, request SignatureRequest) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	args := []any{contacts, request.DeferredRun, request.SourceXID, request.Reopen, request.Live}
	tag, err := tx.Exec(ctx, fmt.Sprintf(`
 WITH subjects AS (SELECT id FROM contact
 WHERE id=ANY($%[1]d::uuid[]) AND archived_at IS NULL AND merged_into_id IS NULL ORDER BY id FOR SHARE)
 INSERT INTO contact_signature_enrich_state AS st (contact_id, requested_generation, deferred_run_id,live_generation,attempted_at)
 SELECT id, 1, $%[2]d,CASE WHEN $%[5]d THEN 1 ELSE 0 END,NULL FROM subjects ORDER BY id
 ON CONFLICT (contact_id) DO UPDATE SET
 requested_generation=st.requested_generation+1,
 live_generation=CASE WHEN $%[5]d THEN st.requested_generation+1 ELSE st.live_generation END,
 activity_id=CASE WHEN $%[4]d THEN NULL ELSE st.activity_id END,
 last_activity_at=CASE WHEN $%[4]d THEN NULL ELSE st.last_activity_at END,
 history_depth=CASE WHEN $%[4]d THEN NULL ELSE st.history_depth END,
 due_at=CASE WHEN $%[5]d THEN LEAST(st.due_at, now()) ELSE st.due_at END,
 deferred_run_id=EXCLUDED.deferred_run_id
 WHERE CASE WHEN $%[3]d::text='' OR st.read_snapshot IS NULL THEN true
 ELSE NOT pg_visible_in_snapshot($%[3]d::text::xid8,st.read_snapshot) END`, len(args)-4, len(args)-3, len(args)-2, len(args)-1, len(args)), args...)
	return tag.RowsAffected() > 0, err
}

// ReleaseSignatureWorkTx transfers requests to the remaining import or makes them runnable.
func ReleaseSignatureWorkTx(ctx context.Context, tx pgx.Tx, runs []ids.UUID, replacement *ids.UUID, limit int) (int64, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	args := []any{runs, replacement, limit}
	tag, err := tx.Exec(ctx, fmt.Sprintf(`WITH locked AS (
 SELECT contact_id FROM contact_signature_enrich_state WHERE deferred_run_id=ANY($%[1]d::uuid[])
 ORDER BY contact_id LIMIT $%[3]d FOR UPDATE SKIP LOCKED)
 UPDATE contact_signature_enrich_state st SET deferred_run_id=$%[2]d,
 due_at=CASE WHEN $%[2]d::uuid IS NULL THEN now() ELSE st.due_at END
 FROM locked WHERE st.contact_id=locked.contact_id`, len(args)-2, len(args)-1, len(args)), args...)
	return tag.RowsAffected(), err
}

// PendingSignatureWork selects the indexed cohort before inspecting any mail.
func (s *Store) PendingSignatureWork(ctx context.Context, limit int) ([]SignatureWork, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	var work []SignatureWork
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		live, err := pendingSignatureCohort(ctx, tx, limit, true)
		if err != nil {
			return err
		}
		work = live
		if len(work) < limit {
			history, err := pendingSignatureCohort(ctx, tx, limit-len(work), false)
			if err != nil {
				return err
			}
			work = append(work, history...)
		}
		return nil
	})
	return work, err
}

func pendingSignatureCohort(ctx context.Context, tx pgx.Tx, limit int, live bool) ([]SignatureWork, error) {
	policy := `live_generation<=processed_generation AND deferred_run_id IS NULL`
	if live {
		policy = `live_generation>processed_generation`
	}
	args := []any{limit}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT contact_id,requested_generation,pg_current_snapshot()::text FROM contact_signature_enrich_state
 WHERE requested_generation>processed_generation AND `+policy+` AND due_at<=now()
 ORDER BY due_at,contact_id LIMIT $%d`, len(args)), args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[SignatureWork])
}

// SettleSignatureWork acknowledges only the observed generation; a failed read backs off that request.
func (s *Store) SettleSignatureWork(ctx context.Context, work SignatureWork, retryAfter time.Duration) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{work.ContactID, work.Generation, retryAfter.Milliseconds(), work.Snapshot}
		_, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE contact_signature_enrich_state SET
 read_snapshot=CASE WHEN $%[3]d::bigint=0 AND requested_generation=$%[2]d AND $%[4]d::text<>'' THEN $%[4]d::text::pg_snapshot ELSE read_snapshot END,
 processed_generation=CASE WHEN $%[3]d::bigint=0 THEN GREATEST(processed_generation,$%[2]d) ELSE processed_generation END,
 due_at=CASE WHEN (requested_generation=$%[2]d OR live_generation<=$%[2]d) AND $%[3]d::bigint>0 THEN now()+$%[3]d*interval '1 millisecond' ELSE due_at END
 WHERE contact_id=$%[1]d AND requested_generation >= $%[2]d`, len(args)-3, len(args)-2, len(args)-1, len(args)), args...)
		return err
	})
}
