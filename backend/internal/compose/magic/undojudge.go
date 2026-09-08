// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// Whether a line the receipt draws can actually be taken back.
//
// The done lane used to answer `Undoable: false` for every row, which was true
// by accident: the sweep's corrections could not be reversed by any path, so a
// hardcoded no was not wrong. It is wrong now, and a receipt that says "the
// machine changed your deal" while greying out the only control that answers
// that is worse than one that does not mention the change.
//
// A SEAM rather than a call, for the reason every other lane here is one: the
// evaluator lives in compose, compose imports this package, and a direct
// dependency would be a cycle. Unbound, every line reads not-undoable with a
// stated reason — the same answer this file replaces, but declared rather than
// assumed.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// UndoJudge answers whether one audit entry can be reversed, for a reader.
//
// Advisory, not binding: a line saying "you can take this back" is an offer to
// open the control, and the write re-asks under its own lock. A judge that
// answered yes to a reversal the writer would refuse costs a rep one clear
// refusal; one that answered no to a reversal the writer would allow hides the
// feature entirely, which is why this is asked at all rather than defaulted.
type UndoJudge interface {
	// JudgeUndoPage answers for a whole page at once, keyed by audit id.
	//
	// The page rather than the line, because the judgment reads more than the
	// entry: an omitted answer is read as not-evaluated, so a judge may decline
	// a subject it does not serve without inventing a verdict for it. The
	// reasons are compose/undoability's own vocabulary rather than a second set
	// written here.
	JudgeUndoPage(ctx context.Context, tx pgx.Tx, subjects []UndoSubject) (map[ids.UUID]*crmcontracts.MagicUndo, error)
}

// UndoSubject is one entry a page asks about.
type UndoSubject struct {
	AuditID    ids.UUID
	EntityType string
}

// WithUndoJudge binds the undoability read. An option for the reason
// WithTroubledRuns is one: an installation that has not wired it gets a receipt
// that names its own limitation instead of a page that refuses.
func (s *Service) WithUndoJudge(j UndoJudge) *Service {
	s.undo = j
	return s
}

// unwiredUndoReason is what a line says when no judge is bound.
//
// Named rather than blank, because the client draws the reason beside a greyed
// control: "not evaluated" tells a rep the product did not look, which is a
// different thing from "this cannot be undone" and should not read as it.
const unwiredUndoReason = "undo_not_evaluated"

// judgeUndoOn fills the undo answer on every line of a page.
//
// Bounded by the page, which is bounded by maxLimit: at most 100 evaluations,
// each short-circuiting on the cheap refusals before it reads the record.
//
// A judge that errors takes NO line down with it. The page still says what
// happened and who did it, which is most of its value, and one unreadable entry
// must not cost a rep the whole morning's receipt — the same reasoning fieldsOf
// applies to an unreadable audit blob.
func (s *Service) judgeUndoOn(ctx context.Context, tx pgx.Tx, lines []crmcontracts.MagicLine) error {
	if s.undo == nil || len(lines) == 0 {
		for i := range lines {
			lines[i].Undo = refusedUndo(unwiredUndoReason)
		}
		return nil
	}
	// ONE call for the page, not one per line. The judgment behind a single
	// answer reads the record, its history and the installation's own posture,
	// and asking it a hundred times over is a hundred round trips for a page
	// that already knows every id it needs. The per-page shape also lets the
	// judge resolve what is constant — the overlay posture is a property of the
	// workspace, not of the line — exactly once.
	answers, err := s.undo.JudgeUndoPage(ctx, tx, entriesOf(lines))
	if err != nil {
		return err
	}
	for i := range lines {
		answer, ok := answers[ids.UUID(lines[i].Id)]
		if !ok {
			lines[i].Undo = refusedUndo(unwiredUndoReason)
			continue
		}
		lines[i].Undo = answer
	}
	return nil
}

// entriesOf names what the page needs judged: the audit entry each line is
// about, and the record type that entry sits on.
func entriesOf(lines []crmcontracts.MagicLine) []UndoSubject {
	out := make([]UndoSubject, 0, len(lines))
	for _, line := range lines {
		if line.Entity == nil {
			continue
		}
		out = append(out, UndoSubject{
			AuditID:    ids.UUID(line.Id),
			EntityType: line.Entity.Type,
		})
	}
	return out
}

// refusedUndo is the shape a client draws as a greyed control with a reason
// beside it. The reason is always present, because a refusal without one is the
// blank the contract's own comment says this field exists to replace.
func refusedUndo(reason string) *crmcontracts.MagicUndo {
	if reason == "" {
		reason = unwiredUndoReason
	}
	return &crmcontracts.MagicUndo{Undoable: false, Reason: &reason}
}
