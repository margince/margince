// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The repair of credentials stored before the masking rules existed.
//
// Three places hold them, and each needs its own step. The live row, the audit
// images of an edit to it, and the provider payload the capture kept.
//
// The row comes first. The later steps read what this one left.

import (
	"bytes"
	"context"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maskStoredCredentialsDataTask repairs what the masking rules now keep out.
//
// Its steps are ordered rather than independent. The audit images and the
// stored originals are worth rewriting only once the row itself is masked.
//
// A run that stopped between them leaves each place consistent with the rules
// on its own.
func maskStoredCredentialsDataTask(pool *pgxpool.Pool) datatask.Task {
	return datatask.Task{
		ID: "core/1792971164_mask_stored_credentials", Version: 1,
		Title: "Credentials stored before the masking rules",
		Steps: []datatask.Step{
			{
				Name: "mask_mail_text", Limit: 200, MinLimit: 10,
				Audit:    datatask.AuditPolicy{PerRecord: true},
				Estimate: estimateMaskableMailText, Batch: maskMailTextBatch,
			},
			{
				Name: "mask_audit_images", Limit: 200, MinLimit: 10,
				Audit:    datatask.AuditPolicy{PerRecord: true},
				Estimate: estimateMaskableAuditImages, Batch: maskAuditImagesBatch,
			},
			{
				Name: "purge_masked_mail_originals", Limit: 200, MinLimit: 10,
				Audit:    datatask.AuditPolicy{PerRecord: true},
				Estimate: estimateMaskedMailOriginals,
				Batch:    purgeMaskedMailOriginalsBatch(pool),
			},
		},
	}
}

// The named arguments these steps bind. A step's estimate and its batch bind
// the same names.
//
// A typo in either reads as a missing value and not an error, so the name comes
// from here rather than the call site.
const (
	argMarker        = "marker"
	argEstimateCap   = "estimate_cap"
	argAfterActivity = "after_activity"
	argAfterRow      = "after_row"
	argPageSize      = "page_size"
)

// activityEntityType names the audited record, which is the table's name and
// not the agent record type that recordTypeActivity spells the same way.
const activityEntityType = "activity"

// maskableMailText is the mail whose text the rules may still reach. Only the
// kinds that mask on a live write are candidates, so the repair cannot put a
// marker where a member typed a word.
// A row under a statutory hold is left alone. The hold says this record is not
// to be altered, and that outranks the repair.
//
// The credential stays, named in the hold's own review.
const maskableMailText = `FROM activity a
 WHERE a.restricted_at IS NULL
   AND a.kind IN ('email', 'message') AND (a.subject IS NOT NULL OR a.body IS NOT NULL)`

func estimateMaskableMailText(
	ctx context.Context, q datatask.Querier, maximum int64,
) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	// Counted by kind and not by content. Whether a row holds a credential is a
	// question for the Go rules.
	//
	// A count that read every body to answer it would cost what the repair does.
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT a.id `+maskableMailText+`
		 LIMIT @estimate_cap) candidates`,
		pgx.NamedArgs{argEstimateCap: maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

func maskMailTextBatch(
	ctx context.Context, tx pgx.Tx, in datatask.BatchIn,
) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id `+maskableMailText+`
		   AND a.id > @after_activity
		 ORDER BY a.id LIMIT @page_size`,
		pgx.NamedArgs{argAfterActivity: after, argPageSize: in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	want := make([]ids.ActivityID, 0, len(scanned))
	for _, id := range scanned {
		want = append(want, ids.From[ids.ActivityKind](id))
	}
	changed, err := activities.MaskStoredMailTextTx(ctx, tx, want)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, scanned, len(changed))
}

// auditImagesWithStoredText is the audit trail of a mail edit. Only the record
// types whose images carry mail text are candidates, and storekit owns that
// list: a type added there must not need remembering here.
const auditImagesWithStoredText = `FROM audit_log l
 WHERE l.entity_type = 'activity' AND (l.before IS NOT NULL OR l.after IS NOT NULL)`

func estimateMaskableAuditImages(
	ctx context.Context, q datatask.Querier, maximum int64,
) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT l.id `+auditImagesWithStoredText+`
		 LIMIT @estimate_cap) rows_with_images`,
		pgx.NamedArgs{argEstimateCap: maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

// maskAuditImagesBatch rewrites the images of one page of audit rows.
//
// This is the one permitted rewrite of an audit image. The trigger from this
// task's migration admits it only while the setting below is on, and only when
// nothing but the two image columns moves.
//
// The setting is local, so it lapses with the transaction.
func maskAuditImagesBatch(
	ctx context.Context, tx pgx.Tx, in datatask.BatchIn,
) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT l.id, l.entity_id, l.before, l.after `+auditImagesWithStoredText+`
		   AND l.id > @after_row
		 ORDER BY l.id LIMIT @page_size`,
		pgx.NamedArgs{argAfterRow: after, argPageSize: in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	images, err := pgx.CollectRows(rows, pgx.RowToStructByPos[auditImageRow])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	admits, err := storekit.AuditTrailAdmitsMasking(ctx, tx)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(images))
	masked := map[ids.UUID]bool{}
	for _, image := range images {
		scanned = append(scanned, image.ID)
		if !admits {
			// This installation's trail stays as it is. The rows themselves are
			// masked by the step before this one, which needs no exception.
			continue
		}
		wrote, err := storekit.MaskAuditImagesTx(ctx, tx, storekit.AuditImageRow{
			ID: image.ID, EntityType: activityEntityType, Before: image.Before, After: image.After,
		})
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if wrote {
			masked[image.EntityID] = true
		}
	}
	// One row per entity rather than per image. The trail records that the
	// repair reached this mail, and twenty images of edits are one repair.
	//
	// Shaped like the erasure tombstone it is. Empty images, because a row
	// certifying a credential gone must not re-store it.
	//
	// The context rides evidence, which the History tab does not project as a
	// field change.
	reached := slices.SortedFunc(maps.Keys(masked), func(a, b ids.UUID) int {
		return bytes.Compare(a[:], b[:])
	})
	for _, entity := range reached {
		if _, err := storekit.AuditEventWithEvidence(ctx, tx, "erase", activityEntityType, entity,
			nil, map[string]any{storekit.EvidenceKeyMaskedAuditImages: true}); err != nil {
			return datatask.BatchOut{}, err
		}
	}
	return datatask.AdvanceUUID(in, scanned, len(masked))
}

type auditImageRow struct {
	ID            ids.UUID
	EntityID      ids.UUID
	Before, After []byte
}

// maskedMailWithAStoredOriginal is the mail this repair has masked. The marker
// in the stored text is the record of that.
//
// A mail whose text carried a credential carries the marker now, and its
// provider payload still holds the value verbatim.
//
// Derived rather than carried between steps, so a run that stopped after the
// text leaves this step the same work on its next pass.
const maskedMailWithAStoredOriginal = `FROM activity a
 WHERE a.restricted_at IS NULL
   AND a.kind IN ('email', 'message')
   AND (a.subject LIKE '%' || @marker || '%' OR a.body LIKE '%' || @marker || '%')
   AND a.raw_capture_id IS NOT NULL`

func estimateMaskedMailOriginals(
	ctx context.Context, q datatask.Querier, maximum int64,
) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT a.id `+maskedMailWithAStoredOriginal+`
		 LIMIT @estimate_cap) masked`,
		pgx.NamedArgs{argMarker: credentialmask.Marker, argEstimateCap: maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

// purgeMaskedMailOriginalsBatch erases the provider payloads of the mail this
// repair masked. The payload is the original text, so masking the row without
// it leaves the credential readable one join away.
func purgeMaskedMailOriginalsBatch(
	pool *pgxpool.Pool,
) func(context.Context, pgx.Tx, datatask.BatchIn) (datatask.BatchOut, error) {
	return func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		if err := auth.RequireSystem(ctx); err != nil {
			return datatask.BatchOut{}, err
		}
		after, err := datatask.UUIDCursor(in.Cursor)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		rows, err := tx.Query(ctx, `SELECT a.id `+maskedMailWithAStoredOriginal+`
			   AND a.id > @after_activity
			 ORDER BY a.id LIMIT @page_size`,
			pgx.NamedArgs{
				argMarker: credentialmask.Marker, argAfterActivity: after, argPageSize: in.Limit,
			})
		if err != nil {
			return datatask.BatchOut{}, err
		}
		scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return datatask.BatchOut{}, err
		}
		// The sweep's own erase path, which knows raw_capture has two writers
		// keyed differently and that older rows are only reachable by the
		// natural key.
		if err := capture.NewPendingStore(InstallationDB(pool)).
			PurgeRawCaptureTx(ctx, tx, scanned); err != nil {
			return datatask.BatchOut{}, err
		}
		return datatask.AdvanceUUID(in, scanned, len(scanned))
	}
}
