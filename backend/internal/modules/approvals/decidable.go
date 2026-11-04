// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// decidable is the one visibility-and-authority predicate behind List, Get and
// Decide, so what a human sees is what they could act on. It needs every grant
// approving a takes and a seat a may be shown to. It also needs read access to
// what a cites and the right to change its target. An ownership offer is the
// exception: its receiver decides it before they may change the record.
// Unknown kinds fail closed.
func decidable(ctx context.Context, tx pgx.Tx, p principal.Principal, a row) (bool, error) {
	decides, err := decidableRows(ctx, tx, p, []row{a})
	if err != nil {
		return false, err
	}
	return decides[0], nil
}

// decidableRows is decidable for a batch, answering by position. Each check
// that needs the database is asked once per batch. The cited activities take
// one statement, and each target type one more; offers and lead reversals take two.
func decidableRows(ctx context.Context, tx pgx.Tx, p principal.Principal, rows []row) ([]bool, error) {
	readable, err := proposalsReadable(ctx, tx, p, rows)
	if err != nil {
		return nil, err
	}
	var targets, demotions, offers []targetRef
	var targetAt, demotionAt, offerAt []int
	for i, a := range rows {
		switch {
		case !readable[i]:
		case a.Kind == "demote_lead":
			demotions, demotionAt = append(demotions, targetRef{a.TargetType, a.TargetID}), append(demotionAt, i)
		case offeredToReader(p, a):
			offers, offerAt = append(offers, targetRef{a.TargetType, a.TargetID}), append(offerAt, i)
		default:
			targets, targetAt = append(targets, targetRef{a.TargetType, a.TargetID}), append(targetAt, i)
		}
	}
	// Deciding spends write authority over the target on the decider's behalf.
	permitted, err := targetsPermitted(ctx, tx, targets, forDeciding)
	if err != nil {
		return nil, err
	}
	demoted, err := demotionTargetsDecidable(ctx, tx, demotions)
	if err != nil {
		return nil, err
	}
	offered, err := offeredRecordsLive(ctx, tx, p, offers)
	if err != nil {
		return nil, err
	}
	out := make([]bool, len(rows))
	for j, i := range targetAt {
		out[i] = permitted[j]
	}
	for j, i := range demotionAt {
		out[i] = demoted[j]
	}
	for j, i := range offerAt {
		out[i] = offered[j]
	}
	if err := narrowToOwnArchives(ctx, tx, p, rows, out); err != nil {
		return nil, err
	}
	return out, nil
}

// narrowToOwnArchives keeps a staged archive decidable by a decider without the
// target's delete grant only while the record is theirs to archive, by the rule
// the archive itself asks. A decider holding delete is not narrowed.
func narrowToOwnArchives(ctx context.Context, tx pgx.Tx, p principal.Principal, rows []row, out []bool) error {
	for i, a := range rows {
		if !out[i] || a.Kind != kindArchiveRecord || a.TargetType == nil || a.TargetID == nil ||
			p.Permissions.Allows(*a.TargetType, principal.ActionDelete) {
			continue
		}
		rule, ruled := auth.ArchiveRuleFor(*a.TargetType)
		if !ruled {
			out[i] = false
			continue
		}
		err := auth.EnsureArchivable(ctx, tx, rule, *a.TargetID)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			out[i] = false
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}
