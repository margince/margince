// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What narrowing to a scope could not answer for, and how the page says so.
//
// Its own file because it is one concept rather than a step of either caller:
// the filters in scope.go produce it and the assembly in worklist.go renders
// it, and putting it in either would make the other reach across.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// teamRosterSource names the roster in a refusal. It is not one of the day's
// lanes: it is the membership the `team` scope is resolved against, and a
// reader told only that their queue is empty cannot tell that apart from a team
// with nothing to do.
const teamRosterSource = "team_roster"

// scopeNote is what narrowing to a scope could not answer for.
//
// The roster behind `team` is BOUNDED and can fail, and both outcomes look
// identical in the rows alone: a page short by every teammate past the cap, and
// a page short by the whole team, are each a shorter list that reads as a
// complete one. The page carries them out rather than deciding here, because
// what to say about a partial answer belongs with the other admissions the
// response already makes.
type scopeNote struct {
	// truncated: the roster came back at its cap, so rows owned by teammates
	// past it were dropped without this read ever seeing them.
	truncated bool
	// failed: the roster did not answer, and the empty page below is fail-closed
	// rather than an empty team.
	failed bool
}

// merge folds one narrowing's note into another's. A page narrows twice — the
// day's rows and the waits beside them — over the same roster, so either half
// reporting a short answer is the page's answer.
func (n scopeNote) merge(other scopeNote) scopeNote {
	return scopeNote{truncated: n.truncated || other.truncated, failed: n.failed || other.failed}
}

// truncationOf carries a short answer onto the page, and nothing otherwise: the
// field is absent unless it is true, so a page that saw its whole scope makes no
// claim about the cap at all.
func truncationOf(scoped scopeNote) *bool {
	if !scoped.truncated {
		return nil
	}
	cut := true
	return &cut
}

// withRosterRefusal names a roster that did not answer, rather than folding it
// into the empty page it produced. The narrowing fails CLOSED, so the queue is
// empty whatever the team was carrying, and an unnamed refusal is exactly the
// "there is nothing" that means "I could not look".
//
// It carries NO category on purpose, and must be appended after the categoriser
// for that to survive. The roster is not one of the day's lanes — it is the
// membership every lane was then filtered against — so naming a category would
// mark one strip while leaving the others reading as exact over rows nobody
// could see.
func withRosterRefusal(
	missing []crmcontracts.WorklistSourceUnavailable, scoped scopeNote,
) []crmcontracts.WorklistSourceUnavailable {
	if !scoped.failed {
		return missing
	}
	return append(missing, crmcontracts.WorklistSourceUnavailable{
		Source: teamRosterSource, Reason: crmcontracts.WorklistSourceUnavailableReasonFailed,
	})
}
