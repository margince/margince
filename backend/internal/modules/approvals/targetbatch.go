// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The target rule of targetvisibility.go, asked of many staged targets at once.
// It takes one statement per target type rather than one per row.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// targetRef is one staged target pair, either half possibly absent.
type targetRef struct {
	Type *string
	ID   *ids.UUID
}

// targetsPermitted applies each target record's own read rule, answering by
// position. The object read grant on the type comes first, as the owning store
// applies it, then that store's row rule. Shapes settledByShape settles, and
// refused grants, cost no statement.
func targetsPermitted(ctx context.Context, tx pgx.Tx, refs []targetRef, want targetAuthority) ([]bool, error) {
	out := make([]bool, len(refs))
	floor := map[string]bool{}
	probed := map[string][]int{}
	for i, ref := range refs {
		settled, visible := settledByShape(targetShape{hasType: ref.Type != nil, hasID: ref.ID != nil})
		if ref.Type == nil {
			// No type is no object to require a read grant on.
			out[i] = visible
			continue
		}
		readable, asked := floor[*ref.Type]
		if !asked {
			var err error
			if readable, err = objectReadable(ctx, readObjectFor(*ref.Type)); err != nil {
				return nil, err
			}
			floor[*ref.Type] = readable
		}
		switch {
		case !readable:
		case settled:
			// A staged create: no row exists yet whose scope could bound it.
			out[i] = visible
		default:
			probed[*ref.Type] = append(probed[*ref.Type], i)
		}
	}
	for _, targetType := range slices.Sorted(maps.Keys(probed)) {
		at := probed[targetType]
		rowIDs := make([]ids.UUID, len(at))
		for j, i := range at {
			rowIDs[j] = *refs[i].ID
		}
		passing, err := rowsPermitted(ctx, tx, targetType, rowIDs, want)
		if err != nil {
			return nil, err
		}
		for _, i := range at {
			out[i] = passing[*refs[i].ID]
		}
	}
	return out, nil
}

// rowsPermitted is the row half for the ids of one target type.
func rowsPermitted(ctx context.Context, tx pgx.Tx, targetType string, rowIDs []ids.UUID, want targetAuthority) (map[ids.UUID]bool, error) {
	switch probeFor(targetType) {
	case probeOwnScope:
		// Live, because an unbounded actor's empty clause would admit an id no row
		// holds. Art. 17 erasure also archives a contact in place, owner_id kept.
		return auth.PassingSubset(ctx, tx, targetType, rowIDs, liveRule(want))
	case probeInheritedScope:
		return targetsThroughParent(ctx, tx, targetType, rowIDs, want)
	case probeExistence:
		return targetsExist(ctx, tx, targetType, rowIDs)
	case probeOwnerOnly:
		return targetsOwnedByActor(ctx, tx, targetType, rowIDs)
	case probeActingWorkspace:
		// A price sheet proposal targets a workspace, and its effect writes to the
		// acting one, so the target must be the acting workspace.
		out := map[ids.UUID]bool{}
		if wsID, ok := principal.WorkspaceID(ctx); ok && slices.Contains(rowIDs, wsID) {
			out[wsID] = true
		}
		return out, nil
	default:
		return map[ids.UUID]bool{}, nil // no rule for this target type: fail closed
	}
}

// liveRule is the own-scope probe each authority asks. Deciding spends write
// authority, so a read share shows a proposal and cannot release it.
func liveRule(want targetAuthority) auth.RowRule {
	if want == forDeciding {
		return auth.RuleWritableLive
	}
	return auth.RuleVisibleLive
}

// targetsThroughParent answers for target kinds with no owner_id of their own.
// Each is visible when the record it hangs off is, as its own store anchors it.
func targetsThroughParent(ctx context.Context, tx pgx.Tx, targetType string, rowIDs []ids.UUID, want targetAuthority) (map[ids.UUID]bool, error) {
	switch targetType {
	case targetOffer:
		return offersThroughDeal(ctx, tx, rowIDs, want)
	case targetSignal:
		return auth.SignalVisibleLiveSubset(ctx, tx, rowIDs)
	case objectActivity:
		return auth.ActivityContentSubset(ctx, tx, rowIDs, true)
	case targetRelationship:
		// No live variant: an edge archived after staging stays decidable, so a
		// human can still reject it, as the owning store's clause has it.
		return auth.RelationshipVisibleSubset(ctx, tx, rowIDs)
	default:
		// A type enrolled as inherited-scope with no parent probe here fails
		// loudly rather than being answered against another table.
		return nil, fmt.Errorf(
			"crmapprovals: %q is classified as inherited-scope with no parent probe", targetType)
	}
}

// offersThroughDeal requires the offer and its deal to be live. Archive is the
// delete on offers, so an effect staged against an archived one could not land.
func offersThroughDeal(ctx context.Context, tx pgx.Tx, offerIDs []ids.UUID, want targetAuthority) (map[ids.UUID]bool, error) {
	rows, err := tx.Query(ctx, `SELECT id, deal_id FROM offer WHERE id = ANY($1) AND archived_at IS NULL`, offerIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dealOf := map[ids.UUID]ids.UUID{}
	for rows.Next() {
		var offer, deal ids.UUID
		if err := rows.Scan(&offer, &deal); err != nil {
			return nil, err
		}
		dealOf[offer] = deal
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := map[ids.UUID]bool{}
	if len(dealOf) == 0 {
		return out, nil
	}
	passing, err := auth.PassingSubset(ctx, tx, tableDeal, slices.Collect(maps.Values(dealOf)), liveRule(want))
	if err != nil {
		return nil, err
	}
	for offer, deal := range dealOf {
		out[offer] = passing[deal]
	}
	return out, nil
}

// targetsExist is the floor of a workspace-shared target type: the ids that
// still exist under that type's own existence query.
func targetsExist(ctx context.Context, tx pgx.Tx, targetType string, rowIDs []ids.UUID) (map[ids.UUID]bool, error) {
	query, ok := existenceProbes[targetType]
	if !ok {
		if ext, registered := extensionTarget(targetType); registered {
			query, ok = extensionExistenceQuery(ext.TargetTable), true
		}
	}
	if !ok {
		// Unreachable while probeFor derives its existence arm from that map, and
		// total so a type is never answered against some other type's table.
		return nil, fmt.Errorf("crmapprovals: %q is classified as existence-probed with no existence query", targetType)
	}
	return idsAmong(ctx, tx, query, rowIDs)
}

// targetsOwnedByActor is the floor for personal state, which its store serves
// to its owner alone. Auth's own/team/all scope would show it to a manager.
func targetsOwnedByActor(ctx context.Context, tx pgx.Tx, targetType string, rowIDs []ids.UUID) (map[ids.UUID]bool, error) {
	if targetType != targetSavedView {
		return nil, fmt.Errorf("crmapprovals: %q is classified as owner-only with no ownership query", targetType)
	}
	p, ok := principal.Actor(ctx)
	if !ok {
		return nil, errors.New("crmapprovals: no actor bound to context")
	}
	if p.UserID == ids.Nil {
		// Comparing owner_id against the nil UUID would answer for a row nobody
		// owns rather than for nothing.
		return map[ids.UUID]bool{}, nil
	}
	return idsAmong(ctx, tx,
		`SELECT id FROM saved_view WHERE id = ANY($1) AND owner_id = $2 AND archived_at IS NULL`, rowIDs, p.UserID)
}

// idsAmong runs a statement selecting the ids among $1 that pass it.
func idsAmong(ctx context.Context, tx pgx.Tx, query string, rowIDs []ids.UUID, more ...any) (map[ids.UUID]bool, error) {
	rows, err := tx.Query(ctx, query, append([]any{rowIDs}, more...)...)
	if err != nil {
		return nil, err
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]bool, len(found))
	for _, id := range found {
		out[id] = true
	}
	return out, nil
}
