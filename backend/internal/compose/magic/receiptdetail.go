// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// Opening one receipt: who did it, and what it moved.
//
// The worklist's Done for you list reports the same acts this page does, and
// names their actor and values through the owners here. A second actor map or
// a second reading of the audit images would drift from this one.

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	byMember    = "magic.by.member"
	byColleague = "magic.by.colleague"
)

// ActorLabel names who acted. This page reports only the machinery, so only a
// surface that also reports members' acts names a member. A seat the reader
// cannot name is a colleague, never an id.
func ActorLabel(actorType, actorID, memberName string) crmcontracts.MagicSentence {
	if actorType != string(principal.PrincipalHuman) {
		return actorLabel(entry{ActorType: actorType, ActorID: actorID})
	}
	if memberName == "" {
		return crmcontracts.MagicSentence{Key: byColleague}
	}
	return crmcontracts.MagicSentence{Key: byMember, Values: &map[string]string{"name": memberName}}
}

// ReceiptRecord is one receipt and the record its act wrote to.
type ReceiptRecord struct {
	ReceiptID  ids.UUID
	EntityType string
	EntityID   ids.UUID
}

// ReceiptChanges answers every field each receipt's act moved on its record,
// masked as the record's own history masks it.
//
// A receipt is absent when none of its audit rows can be read. The record may
// be out of the reader's scope or of a type this page cannot place. Or the act
// predates the link from an effect to its approval.
func (s *Service) ReceiptChanges(ctx context.Context, records []ReceiptRecord) (map[ids.UUID][]crmcontracts.MagicFieldChange, error) {
	byType := map[string][]ReceiptRecord{}
	for _, record := range records {
		if _, placed := scopedTypes[record.EntityType]; placed {
			byType[record.EntityType] = append(byType[record.EntityType], record)
		}
	}
	out := map[ids.UUID][]crmcontracts.MagicFieldChange{}
	if len(byType) == 0 {
		return out, nil
	}
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		for entityType, wanted := range byType {
			entries, err := receiptEntries(ctx, tx, entityType, wanted)
			if err != nil {
				return err
			}
			changes, err := changesByReceipt(ctx, entityType, wanted, entries)
			if err != nil {
				return err
			}
			for id, moved := range changes {
				out[id] = moved
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read what a receipt changed: %w", err)
	}
	return out, nil
}

// receiptEntries reads the audit rows the receipts name on one record type,
// under that type's grant, row scope and erasure boundary, oldest first.
func receiptEntries(ctx context.Context, tx pgx.Tx, entityType string, wanted []ReceiptRecord) ([]entry, error) {
	if err := auth.Require(ctx, entityType, principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, nil
		}
		return nil, err
	}
	receipts := make([]ids.UUID, 0, len(wanted))
	receiptTexts := make([]string, 0, len(wanted))
	records := make([]ids.UUID, 0, len(wanted))
	for _, record := range wanted {
		receipts = append(receipts, record.ReceiptID)
		receiptTexts = append(receiptTexts, record.ReceiptID.String())
		records = append(records, record.EntityID)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	typeAt, recordsAt := arg(entityType), arg(records)
	receiptsAt, textsAt := arg(receipts), arg(receiptTexts)
	releasedAt, stageAt := arg(storekit.EvidenceKeyReleasedBy), arg(deals.StageApprovalEvidenceKey)
	scope, err := auth.ScopeClauseFor(ctx, scopedTypes[entityType], "e", arg)
	if err != nil {
		return nil, err
	}
	unscrubbed := privacy.UnscrubbedImageSQL("a", fmt.Sprintf("$%d", arg(privacy.ScrubVerbs())))
	where := fmt.Sprintf(`a.entity_type = $%d AND a.entity_id = ANY($%d)
		   AND (a.id = ANY($%d) OR a.evidence->>$%d = ANY($%d) OR a.evidence->>$%d = ANY($%d))
		   AND %s`, typeAt, recordsAt, receiptsAt, releasedAt, textsAt, stageAt, textsAt, unscrubbed)
	if scope != "" {
		where += " AND " + scope
	}
	// The table name is a compile-time value out of scopedTypes, never a string
	// off a request.
	query := fmt.Sprintf(`
		SELECT a.id, a.occurred_at, a.action, a.entity_type, a.entity_id,
		       a.actor_type, a.actor_id, a.on_behalf_of, a.before, a.after,
		       NULL::text, a.evidence, a.batch_id
		  FROM audit_log a
		  JOIN %s e ON e.id = a.entity_id
		 WHERE %s
		 ORDER BY a.occurred_at, a.id
		 LIMIT $%d`, scopedTypes[entityType], where, arg(readCap))
	return scanEntries(ctx, tx, query, args)
}

// changesByReceipt folds each receipt's rows into one change per field: the
// value before the act's first write and after its last.
func changesByReceipt(
	ctx context.Context, entityType string, wanted []ReceiptRecord, entries []entry,
) (map[ids.UUID][]crmcontracts.MagicFieldChange, error) {
	recordOf := make(map[ids.UUID]ids.UUID, len(wanted))
	for _, record := range wanted {
		recordOf[record.ReceiptID] = record.EntityID
	}
	fields := map[ids.UUID]map[string]crmcontracts.MagicFieldChange{}
	for _, e := range entries {
		receipt := ids.UUID(receiptIdentity(e))
		if record, asked := recordOf[receipt]; !asked || record != e.EntityID {
			continue
		}
		if fields[receipt] == nil {
			fields[receipt] = map[string]crmcontracts.MagicFieldChange{}
		}
		for _, change := range changesOf(e) {
			if earlier, seen := fields[receipt][change.Field]; seen {
				change.Before = earlier.Before
			}
			fields[receipt][change.Field] = change
		}
	}
	out := make(map[ids.UUID][]crmcontracts.MagicFieldChange, len(fields))
	for receipt, moved := range fields {
		list := make([]crmcontracts.MagicFieldChange, 0, len(moved))
		for _, change := range moved {
			list = append(list, change)
		}
		sort.Slice(list, func(a, b int) bool { return list[a].Field < list[b].Field })
		visible, err := visibleChanges(ctx, entityType, list)
		if err != nil {
			return nil, err
		}
		out[receipt] = visible
	}
	return out, nil
}
