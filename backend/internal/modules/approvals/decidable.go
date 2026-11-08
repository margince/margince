// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
// one statement, and each target type one more; offers and lead reversals take
// two. Staged archives take one more per target type.
func decidableRows(ctx context.Context, tx pgx.Tx, p principal.Principal, rows []row) ([]bool, error) {
	readable, err := proposalsReadable(ctx, tx, p, rows)
	if err != nil {
		return nil, err
	}
	var visible []row
	var positions []int
	for i, allowed := range readable {
		if allowed {
			visible, positions = append(visible, rows[i]), append(positions, i)
		}
	}
	permitted, err := authorityRows(ctx, tx, p, visible)
	if err != nil {
		return nil, err
	}
	out := make([]bool, len(rows))
	applyTargetPermissions(out, positions, permitted)
	return out, nil
}

func authorityRows(ctx context.Context, tx pgx.Tx, p principal.Principal, rows []row) ([]bool, error) {
	readable := make([]bool, len(rows))
	for i, proposal := range rows {
		readable[i] = requireDecisionGrants(p, proposal) == nil
	}

	var targets, demotions, offers, linked []targetRef
	var targetAt, demotionAt, offerAt, linkedAt []int
	for i, a := range rows {
		switch {
		case !readable[i]:
		case a.Kind == "invite_meeting" || a.Kind == "book_meeting":
			linked, linkedAt = append(linked, targetRef{a.TargetType, a.TargetID}), append(linkedAt, i)
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
	visible, err := targetsPermitted(ctx, tx, linked, forReading)
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
	applyTargetPermissions(out, linkedAt, visible)
	applyTargetPermissions(out, targetAt, permitted)
	applyTargetPermissions(out, demotionAt, demoted)
	applyTargetPermissions(out, offerAt, offered)
	if err := narrowToOwnArchives(ctx, tx, p, rows, out); err != nil {
		return nil, err
	}
	if err := narrowToWrittenCoTargets(ctx, tx, rows, out); err != nil {
		return nil, err
	}
	return out, nil
}

func applyTargetPermissions(out []bool, positions []int, permitted []bool) {
	for j, i := range positions {
		out[i] = permitted[j]
	}
}

func narrowToWrittenCoTargets(ctx context.Context, tx pgx.Tx, rows []row, out []bool) error {
	var targets []targetRef
	var positions []int
	for i, proposal := range rows {
		if out[i] && (proposal.Kind == previewMergeTags || proposal.Kind == previewMergeRecords) && proposal.CoTargetType != nil && proposal.CoTargetID != nil {
			targets = append(targets, targetRef{proposal.CoTargetType, proposal.CoTargetID})
			positions = append(positions, i)
		}
	}
	permitted, err := targetsPermitted(ctx, tx, targets, forDeciding)
	if err != nil {
		return err
	}
	for i, position := range positions {
		out[position] = permitted[i]
	}
	return nil
}

// narrowToOwnArchives keeps a staged archive decidable, for a decider without
// the target's delete grant, only while the record is theirs to archive. It
// asks the archive's own rule, once per record type. A decider holding delete
// is not narrowed.
func narrowToOwnArchives(ctx context.Context, tx pgx.Tx, p principal.Principal, rows []row, out []bool) error {
	targets := map[string][]ids.UUID{}
	for i, a := range rows {
		if narrowedAsArchive(p, a, out[i]) {
			targets[*a.TargetType] = append(targets[*a.TargetType], *a.TargetID)
		}
	}
	archivable := make(map[string]map[ids.UUID]bool, len(targets))
	for table, rowIDs := range targets {
		rule, ruled := auth.ArchiveRuleFor(table)
		if !ruled {
			continue
		}
		subset, err := auth.OwnedArchivableSubset(ctx, tx, rule, rowIDs)
		if err != nil {
			return err
		}
		archivable[table] = subset
	}
	for i, a := range rows {
		if narrowedAsArchive(p, a, out[i]) {
			out[i] = archivable[*a.TargetType][*a.TargetID]
		}
	}
	return nil
}

// narrowedAsArchive reports whether a so far decidable row is a staged archive
// whose decider lacks the delete grant on its target.
func narrowedAsArchive(p principal.Principal, a row, decidable bool) bool {
	return decidable && a.Kind == kindArchiveRecord && a.TargetType != nil && a.TargetID != nil &&
		!p.Permissions.Allows(*a.TargetType, principal.ActionDelete)
}
