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
	openapi_types "github.com/oapi-codegen/runtime/types"

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
	// JudgeUndo reports whether the entry may be reversed, and why not when it
	// may not. The reason is compose/undoability's own vocabulary — the same
	// words the refusal would carry — rather than a second set written here.
	JudgeUndo(ctx context.Context, tx pgx.Tx, auditID ids.UUID, entityType string) (undoable bool, reason string, err error)
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
func (s *Service) judgeUndoOn(ctx context.Context, tx pgx.Tx, lines []crmcontracts.MagicLine) {
	for i := range lines {
		lines[i].Undo = s.judgeOne(ctx, tx, lines[i])
	}
}

// judgeOne answers for a single line.
func (s *Service) judgeOne(ctx context.Context, tx pgx.Tx, line crmcontracts.MagicLine) *crmcontracts.MagicUndo {
	if s.undo == nil || line.Entity == nil {
		return refusedUndo(unwiredUndoReason)
	}
	auditID := ids.UUID(line.Id)
	undoable, reason, err := s.undo.JudgeUndo(ctx, tx, auditID, line.Entity.Type)
	if err != nil {
		return refusedUndo(unwiredUndoReason)
	}
	if !undoable {
		return refusedUndo(reason)
	}
	id := openapi_types.UUID(auditID)
	return &crmcontracts.MagicUndo{Undoable: true, AuditId: &id}
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
