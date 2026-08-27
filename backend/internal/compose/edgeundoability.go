// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// JUDGING one audited change to a LINK, which is a different question from
// performing its inverse — edgereverse.go does that, and this decides whether it
// may run at all.
//
// Its own file rather than a branch set inside undoability.go, because only three
// of the record's refusals mean the same thing about an edge: a link has no
// update shape of its own, no archived_at a record path would read, and no
// incumbent behind it. The two paths share the ORDER — cheapest and most certain
// first — and the reason vocabulary, and nothing else.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/people"
)

// evaluateEdge answers whether one audited change to a LINK can be reversed.
//
// Its own branch set rather than the record's, because only three of the
// refusals mean the same thing about an edge — and because two of the record's
// would read the wrong table. The order is the record path's: cheapest and most
// certain first, so a row failing several reports the most useful one.
//
// ExternallyGoverned is NOT asked, and the omission is a decision rather than the
// record branch missed. That refusal exists because an overlay workspace's
// records live in the incumbent, so putting one back is a write-back the
// reversal path cannot link or attribute. A LINK has no incumbent counterpart to
// write back to: `relationship` is not a member of the overlay mirror's entity
// set and the overlay provider declares no write verb for it, so the local row is
// not a copy of anything and the local write is the whole write. Refusing here
// would leave a link nobody can reverse in a workspace where nothing else could
// have changed it either.
//
// TestALinkInAnOverlayGovernedWorkspaceIsStillReversible pins the answer and the
// premise together, so the day the mirror learns to carry links this branch is
// what fails rather than a customer's two systems quietly disagreeing.
func (e Evaluator) evaluateEdge(ctx context.Context, tx pgx.Tx, row AuditRow) (Undoability, error) {
	if e.EdgeFacts == nil {
		return Undoability{}, fmt.Errorf("compose: no edge reader is wired, so entry %s cannot be judged", row.ID)
	}
	facts, err := e.EdgeFacts(ctx, tx, row.EntityID)
	if err != nil {
		if refusal, refused := edgeScopeRefusal(err); refused {
			return refusal, nil
		}
		return Undoability{}, err
	}
	if answer, decided := edgeShapeRefusal(row.Action, facts); decided {
		return answer, nil
	}
	// Asked before the link's own state, for the reason the record path asks it
	// before supersession: an entry somebody has put back should SAY so. Putting
	// a link back removes it, so "the link is gone" would otherwise answer first
	// and tell the reader nothing they can act on.
	if e.AlreadyUndone != nil {
		undone, err := e.AlreadyUndone(ctx, tx, row)
		if err != nil {
			return Undoability{}, err
		}
		if undone {
			return refuse(ReasonAlreadyUndone, ""), nil
		}
	}
	if facts.Archived {
		// The link this entry made has since been removed, and putting it back is
		// an un-archive no write path here performs. Naming the state is what a
		// reader can act on.
		return refuse(ReasonRecordArchived, ""), nil
	}
	if e.EdgeWritable != nil {
		if err := e.EdgeWritable(ctx, tx, facts, row.Action); err != nil {
			if refusal, refused := edgeScopeRefusal(err); refused {
				return refusal, nil
			}
			return Undoability{}, err
		}
	}
	unclearable, err := edgeFieldsNoPatchCanClear(row.Before, row.After)
	if err != nil {
		return Undoability{}, err
	}
	if len(unclearable) > 0 {
		return refuse(ReasonNullUnwritableByModule, strings.Join(unclearable, ", ")), nil
	}
	return e.edgeTrailState(ctx, tx, row)
}

// edgeScopeRefusal renders a refusal from the edge's own gates as the answer the
// button shows. The row reached the caller through the history read's gates, so
// its existence is not what is hidden here — only whether they may act on it, and
// a fault is a different thing and stays one.
func edgeScopeRefusal(err error) (Undoability, bool) {
	if !isWriteScopeRefusal(err) {
		return Undoability{}, false
	}
	return refuse(ReasonNotWritableByCaller, ""), true
}

// edgeShapeRefusal answers the refusals decided by the KIND of link and the verb
// alone, before any authority is asked. Kind first, and whatever the verb:
// project_company needs write authority over the project ROW and a project must
// keep one company, so a generic reverse of one is a side door around both rules
// — including on an unlink, where the honest answer names the kind rather than
// the un-archive it would also have refused.
func edgeShapeRefusal(action string, facts people.EdgeFacts) (Undoability, bool) {
	if facts.Kind == people.ProjectCompanyKind {
		return refuse(ReasonNotRestorableByThisPath, facts.Kind), true
	}
	if action == edgeActionArchive {
		return refuse(ReasonEdgeRelinkUnsupported, ""), true
	}
	if !reversibleEdgeAction(action) {
		return refuse(ReasonNotAReplayableVerb, action), true
	}
	return Undoability{}, false
}

// edgeTrailState asks the two refusals that read past this entry. Supersession is
// computed against the EDGE, whose columns the image names — the record's own
// fields are not what an edge change moved.
func (e Evaluator) edgeTrailState(ctx context.Context, tx pgx.Tx, row AuditRow) (Undoability, error) {
	moved, err := fieldsThatMovedSince(ctx, tx, row)
	if err != nil {
		return Undoability{}, err
	}
	if len(moved) > 0 {
		return refuse(ReasonSuperseded, strings.Join(moved, ", ")), nil
	}
	if e.BehindErasure != nil {
		behind, err := e.BehindErasure(ctx, tx, row)
		if err != nil {
			return Undoability{}, err
		}
		if behind {
			return refuse(ReasonBehindErasureBoundary, ""), nil
		}
	}
	return undoable(), nil
}
