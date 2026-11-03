// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MergePrimaryRepairPage is one page of contacts a merge left without a
// current employer, in id order.
type MergePrimaryRepairPage struct {
	Scanned []ids.UUID
	Changed int
}

// mergeDroppedHeir is contact c's live current employment edge whose
// duplicate a merge archived while that duplicate held c's primary slot. The
// archived edge sits at a company merged into the heir's, or on a contact
// merged into c. A merge keeps redirect chains one hop deep.
//
// The archive row with origin merge proves the merge archived the edge. A flag
// change on c's employments after that row is a person's choice, kept as is.
var mergeDroppedHeir = `SELECT n.id FROM relationship n
 JOIN relationship p ON p.kind = 'employment' AND p.is_current_primary AND p.archived_at IS NOT NULL
 AND ((p.contact_id = c.id AND EXISTS (SELECT 1 FROM company x WHERE x.id = p.company_id AND x.merged_into_id = n.company_id))
   OR (p.company_id = n.company_id AND EXISTS (SELECT 1 FROM contact m WHERE m.id = p.contact_id AND m.merged_into_id = c.id)))
 JOIN audit_log pa ON pa.entity_type = 'relationship' AND pa.entity_id = p.id AND pa.action = 'archive'
 AND pa.evidence ->> '` + auditKeyOrigin + `' = '` + originMerge + `'
 WHERE n.contact_id = c.id AND n.kind = 'employment' AND n.archived_at IS NULL AND ` + employment.IsCurrentSQL("n.ended_at") + `
 AND NOT EXISTS (SELECT 1 FROM audit_log al JOIN relationship e ON e.id = al.entity_id
   WHERE al.entity_type = 'relationship' AND e.contact_id = c.id AND e.kind = 'employment' AND al.after ? 'is_current_primary'
   AND al.occurred_at > pa.occurred_at)`

// mergeDroppedPrimary picks a live contact with no primary employer and one
// live current employment, which is its merge heir. With two current
// employments the choice is a person's.
var mergeDroppedPrimary = `c.archived_at IS NULL AND c.merged_into_id IS NULL
 AND NOT EXISTS (SELECT 1 FROM relationship s WHERE s.contact_id = c.id AND ` + employment.CurrentPrimarySlotSQL("s") + `)
 AND (SELECT count(*) FROM relationship n WHERE n.contact_id = c.id AND n.kind = 'employment' AND n.archived_at IS NULL
   AND ` + employment.IsCurrentSQL("n.ended_at") + `) = 1
 AND EXISTS (` + mergeDroppedHeir + `)`

// ReconcileMergeDroppedPrimariesTx gives each contact on this page its merge
// heir back as primary employer. Each contact runs in its own savepoint under
// its employment lock. The question is asked again there, so an edit since the
// page was read wins.
func ReconcileMergeDroppedPrimariesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID string) (MergePrimaryRepairPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return MergePrimaryRepairPage{}, err
	}
	if err := auth.Require(ctx, contactEntity, principal.ActionUpdate); err != nil {
		return MergePrimaryRepairPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT c.id FROM contact c WHERE c.id > $1 AND `+mergeDroppedPrimary+` ORDER BY c.id LIMIT $2`, after, limit)
	if err != nil {
		return MergePrimaryRepairPage{}, fmt.Errorf("contacts: paging contacts a merge left without an employer: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return MergePrimaryRepairPage{}, fmt.Errorf("contacts: paging contacts a merge left without an employer: %w", err)
	}
	page := MergePrimaryRepairPage{Scanned: scanned}
	for _, id := range scanned {
		changed := false
		err := pgx.BeginFunc(ctx, tx, func(savepoint pgx.Tx) error {
			var err error
			changed, err = restoreMergeDroppedPrimary(ctx, savepoint, ids.From[ids.ContactKind](id), runID)
			return err
		})
		if err != nil {
			return MergePrimaryRepairPage{}, err
		}
		if changed {
			page.Changed++
		}
	}
	return page, nil
}

func restoreMergeDroppedPrimary(ctx context.Context, tx pgx.Tx, contact ids.ContactID, runID string) (bool, error) {
	if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, contact.String()); err != nil {
		return false, err
	}
	var heir ids.UUID
	err := tx.QueryRow(ctx, `SELECT (`+mergeDroppedHeir+` LIMIT 1) FROM contact c WHERE c.id = $1 AND `+mergeDroppedPrimary, contact).Scan(&heir)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: re-reading a contact a merge left without an employer: %w", err)
	}
	promoted, err := scanRelationship(tx.QueryRow(ctx, `UPDATE relationship a SET is_current_primary = true
 WHERE a.id = $1 AND a.archived_at IS NULL AND NOT a.is_current_primary
 AND NOT EXISTS (SELECT 1 FROM relationship b WHERE `+mergePrimarySlotWitness+`)
 RETURNING `+relationshipColumns, heir))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: restoring a primary employer a merge dropped: %w", err)
	}
	before := relationshipFieldImage(promoted)
	before["is_current_primary"] = false
	evidence := map[string]any{"data_task_run": runID, auditKeyReason: "merge_dropped_primary"}
	if err := emitRelationshipChangeWithEvidence(ctx, tx, "update", before, promoted, evidence); err != nil {
		return false, fmt.Errorf("contacts: recording a restored primary employer: %w", err)
	}
	return true, nil
}

// CountMergeDroppedPrimaries counts, up to maximum+1, the contacts the repair
// would change.
func CountMergeDroppedPrimaries(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, maximum int64,
) (int64, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM contact c WHERE `+mergeDroppedPrimary+` LIMIT $1) candidates`, maximum+1).Scan(&count)
	return count, err
}
