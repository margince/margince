// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package credentialrepair is the data task that masks credentials stored
// before the masking rules. It masks them in the mail row and in the audit
// images of edits to it, and erases the provider payload the capture kept.
package credentialrepair

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// auditRewriteMigration is the migration that admits the audit-image rewrite.
// The audit step stands down where the trigger lacks it, so running before it
// would finish that step with every image untouched.
const auditRewriteMigration = "core/1794348833"

const taskID = "core/1794348833_mask_stored_credentials"

// PurgeRawCapture erases the stored originals of the named activities. Compose
// passes capture's own eraser, so the repair writes no second one.
type PurgeRawCapture func(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) error

// DataTask repairs what the masking rules now keep out. The original-erase
// step finds its mail by the marker the row step writes, so the row step runs
// first.
func DataTask(purge PurgeRawCapture) datatask.Task {
	return datatask.Task{
		ID: taskID, Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1794348833_mask_stored_credentials"),
		Requires: []string{auditRewriteMigration},
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
				Batch:    purgeMaskedMailOriginalsBatch(purge),
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
	argMaskedFlag    = "masked_flag"
	argEstimateCap   = "estimate_cap"
	argAfterActivity = "after_activity"
	argAfterRow      = "after_row"
	argMaskedTypes   = "masked_types"
	argPageSize      = "page_size"
)

// maskableMailText is the mail whose text the rules may still reach. It holds
// only the kinds that mask on a live write, so no marker lands where a member
// typed.
// A row under a statutory hold is left alone, because the hold says the record
// is not to be altered, and that outranks the repair.
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

// auditImagesWithStoredText is the audit trail of the record types the masking
// rules reach; storekit owns that list, so a type added there is reached here.
// The hold reaches the trail too: an image of a held mail is the same text.
//
// A LEFT JOIN, because an audit row outlives its record: the untold-lead
// erasure deletes notice mail and leaves its trail standing.
const auditImagesWithStoredText = `FROM audit_log l
  LEFT JOIN activity a ON l.entity_type = 'activity' AND a.id = l.entity_id
 WHERE l.entity_type = ANY(@masked_types)
   AND (a.id IS NULL OR a.restricted_at IS NULL)
   AND (l.before IS NOT NULL OR l.after IS NOT NULL)`

func estimateMaskableAuditImages(
	ctx context.Context, q datatask.Querier, maximum int64,
) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT l.id `+auditImagesWithStoredText+`
		 LIMIT @estimate_cap) rows_with_images`,
		pgx.NamedArgs{
			argMaskedTypes: storekit.MaskedAuditEntityTypes(), argEstimateCap: maximum + 1,
		}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

// maskAuditImagesBatch rewrites the images of one page of audit rows, one
// record at a time through storekit.MaskRecordAuditImagesTx. A mail placed
// under a hold since the page was read is left alone.
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
	admits, err := storekit.AuditTrailAdmitsMasking(ctx, tx)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	if !admits {
		return standDown(ctx, tx, in)
	}
	rows, err := tx.Query(ctx, `SELECT l.id, l.entity_type, l.entity_id, a.id IS NOT NULL, l.before, l.after `+
		auditImagesWithStoredText+`
		   AND l.id > @after_row
		 ORDER BY l.id LIMIT @page_size`,
		pgx.NamedArgs{
			argMaskedTypes: storekit.MaskedAuditEntityTypes(),
			argAfterRow:    after, argPageSize: in.Limit,
		})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	images, err := pgx.CollectRows(rows, pgx.RowToStructByPos[auditImageRow])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	trails, scanned, liveMail := groupByRecord(images)
	unheld, err := activities.LockUnheldActivitiesTx(ctx, tx, liveMail)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	masked := 0
	for _, trail := range trails {
		if trail.liveMail && !unheld[trail.record.id] {
			continue
		}
		wrote, err := storekit.MaskRecordAuditImagesTx(ctx, tx,
			trail.record.entityType, trail.record.id, trail.images)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if wrote > 0 {
			masked++
		}
	}
	return datatask.AdvanceUUID(in, scanned, masked)
}

// standDown ends the audit step where the trigger admits no rewrite. A later
// release restores the strict trigger. An installation that upgrades across
// both meets it before this task runs. The row and raw-capture steps still owe
// their work, so this step finishes instead of failing the task. The log names
// how many audit rows with images it left as stored, for the operator.
func standDown(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	var left int64
	if err := tx.QueryRow(ctx, `SELECT count(*) `+auditImagesWithStoredText,
		pgx.NamedArgs{argMaskedTypes: storekit.MaskedAuditEntityTypes()}).Scan(&left); err != nil {
		return datatask.BatchOut{}, err
	}
	slog.WarnContext(ctx, "data task step left audit images unmasked: the audit trail admits no image rewrite",
		"task", taskID, "step", "mask_audit_images", "audit_rows_left_as_stored", left)
	return datatask.BatchOut{Cursor: in.Cursor, Done: true}, nil
}

// auditedRecord is one record the audit step reached, keyed by type as well as
// id: two record types number their rows independently.
type auditedRecord struct {
	entityType string
	id         ids.UUID
}

type auditImageRow struct {
	ID            ids.UUID
	EntityType    string
	EntityID      ids.UUID
	LiveMail      bool
	Before, After []byte
}

// recordTrail is the audit rows of one record on this page.
type recordTrail struct {
	record   auditedRecord
	liveMail bool
	images   []storekit.AuditImageRow
}

// groupByRecord gathers a page's audit rows by record, in the page's order. It
// also names the mail rows that still exist, which the step locks before it
// writes.
func groupByRecord(images []auditImageRow) ([]*recordTrail, []ids.UUID, []ids.UUID) {
	var trails []*recordTrail
	byRecord := map[auditedRecord]*recordTrail{}
	scanned := make([]ids.UUID, 0, len(images))
	var liveMail []ids.UUID
	for _, image := range images {
		scanned = append(scanned, image.ID)
		record := auditedRecord{entityType: image.EntityType, id: image.EntityID}
		trail, seen := byRecord[record]
		if !seen {
			trail = &recordTrail{record: record, liveMail: image.LiveMail}
			byRecord[record] = trail
			trails = append(trails, trail)
			if image.LiveMail {
				liveMail = append(liveMail, image.EntityID)
			}
		}
		trail.images = append(trail.images,
			storekit.AuditImageRow{ID: image.ID, Before: image.Before, After: image.After})
	}
	return trails, scanned, liveMail
}

// maskedMailWithAStoredOriginal is the mail the row step masked, found by the
// evidence flag only that step writes. The text itself is no proof: a sender
// can type the marker.
//
// Derived rather than carried between steps, so a run that stopped after the
// text leaves this step the same work on its next pass.
//
// Selected through the purge's own condition rather than through
// raw_capture_id, which only rows written since that column landed carry.
var maskedMailWithAStoredOriginal = `FROM activity a
 WHERE a.restricted_at IS NULL
   AND a.kind IN ('email', 'message')
   AND EXISTS (SELECT 1 FROM audit_log l
                WHERE l.entity_type = 'activity' AND l.entity_id = a.id
                  AND l.action = 'update' AND l.actor_type = 'system'
                  AND l.evidence -> @masked_flag = 'true'::jsonb)
   AND EXISTS (SELECT 1 FROM raw_capture r WHERE ` + storekit.StoredOriginalSQL("a", "r") + `)`

func estimateMaskedMailOriginals(
	ctx context.Context, q datatask.Querier, maximum int64,
) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT a.id `+maskedMailWithAStoredOriginal+`
		 LIMIT @estimate_cap) masked`,
		pgx.NamedArgs{
			argMaskedFlag: activities.EvidenceKeyMaskedStoredText, argEstimateCap: maximum + 1,
		}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

// purgeMaskedMailOriginalsBatch erases the provider payloads of the mail this
// repair masked. The payload is the original text, so masking the row without
// it leaves the credential readable one join away. A mail placed under a hold
// since the page was read keeps its original.
func purgeMaskedMailOriginalsBatch(
	purge PurgeRawCapture,
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
				argMaskedFlag:    activities.EvidenceKeyMaskedStoredText,
				argAfterActivity: after, argPageSize: in.Limit,
			})
		if err != nil {
			return datatask.BatchOut{}, err
		}
		scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return datatask.BatchOut{}, err
		}
		unheld, err := activities.LockUnheldActivitiesTx(ctx, tx, scanned)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		erase := make([]ids.UUID, 0, len(scanned))
		for _, id := range scanned {
			if unheld[id] {
				erase = append(erase, id)
			}
		}
		if err := purge(ctx, tx, erase); err != nil {
			return datatask.BatchOut{}, err
		}
		return datatask.AdvanceUUID(in, scanned, len(erase))
	}
}
