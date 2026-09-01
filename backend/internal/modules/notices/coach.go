// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// A notice one PERSON raises for another, as against the ones a system flow
// raises under the system principal.
//
// The difference is the whole of this file. Create takes no gate because only
// compose-wired workflows reach it and their authority is the engine's; a
// human-raised notice is words placed in a colleague's queue, so it carries
// both halves of an authority question: may this seat coach at all, and may it
// coach THIS person.

import (
	"context"
	"fmt"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// noteBound caps the coach's own words. Far below bodyBound, which sizes a
// system flow's explanation: a nudge that needs five hundred characters is a
// conversation, and the product has surfaces for those.
const noteBound = 500

// fieldRecipient is the contract's spelling of who a coaching notice is for.
// Three refusals point at this field, and a caller matching on the name cannot
// tell a typo from a different field.
const fieldRecipient = "recipient_user_id"

// Teammates answers whether the recipient is on a team with the calling person.
//
// The caller is not a parameter: the module behind this reads it from the
// principal, so a coach cannot ask about an edge they are not an end of. Bound
// by compose to the same identity read the Worklist's owner parameter uses —
// one answer to "are these two teammates", not two.
type Teammates interface {
	SharesLiveTeamWithCaller(ctx context.Context, other ids.UUID) (bool, error)
}

// coachSubjects is what each kind says to the person who receives it.
//
// Derived from the contract's enum rather than kept as a parallel list: a kind
// the contract admits and this map does not would reach a recipient with a
// blank headline, and the write refuses a blank subject, so the failure would
// be a 500 on a request the contract said was valid.
var coachSubjects = map[crmcontracts.NoticeKind]string{
	crmcontracts.CoachReplyAging:        "A customer reply is getting old",
	crmcontracts.CoachDealNeedsNextStep: "A deal needs its next step",
	crmcontracts.CoachReviewBacklog:     "There is review work waiting",
	crmcontracts.CoachGeneral:           "Your lead left you a note",
}

// RaiseCoachNotice records one person's nudge to a teammate.
//
// AUTHORIZATION FIRST, then the request's own shape. A caller who may not coach
// learns nothing about what a well-formed coaching request looks like — every
// ask answers the same 403, whatever they put in it. The reverse order told a
// rep which kinds the vocabulary holds by answering 422 for a wrong one and 403
// for a right one, which is a shape the contract publishes anyway but is not
// this endpoint's to confirm.
func (s *Store) RaiseCoachNotice(
	ctx context.Context, mates Teammates, recipient ids.UserID, kind crmcontracts.NoticeKind, note string,
) (Notice, error) {
	// May this seat coach at all. Refuses an agent, a system pass and a buyer
	// as well as a rep: coaching is a thing a lead does, and a background flow
	// raising one would be writing in a person's voice.
	if err := auth.RequireCoach(ctx); err != nil {
		return Notice{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		// RequireCoach admits only a human principal, so this is the seated
		// identity behind it — needed for the self-coaching check below, and
		// unreachable through the gate above.
		return Notice{}, apperrors.ErrPermissionDenied
	}
	if !kind.Valid() {
		return Notice{}, &values.ParseError{
			Field: "kind", Code: "unknown_notice_kind",
			Message: "a coaching notice names one of the kinds the contract lists",
		}
	}
	subject, named := coachSubjects[kind]
	if !named {
		// The contract admitted a kind this map does not answer for. Refusing
		// is the honest answer: the alternative is a notice whose headline is
		// blank, which the write rejects one layer down as a server error.
		return Notice{}, fmt.Errorf("notices: no subject for kind %q", kind)
	}
	if recipient.IsZero() {
		// An absent recipient_user_id decodes to the zero UUID with no error,
		// so without this it reaches the membership question, comes back "not a
		// teammate", and answers 403 — a refusal about a person the caller
		// never named and cannot connect to anything they did.
		return Notice{}, &values.ParseError{
			Field: fieldRecipient, Code: "missing_recipient",
			Message: "a coaching notice names who it is for",
		}
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > noteBound {
		return Notice{}, &values.ParseError{
			Field: "note", Code: "note_too_long",
			Message: fmt.Sprintf("a coaching note is at most %d characters", noteBound),
		}
	}
	if recipient.UUID == actor.UserID {
		// Coaching yourself is not a refusal case worth a permission error, but
		// it is not a notice either: it would sit in the raiser's own queue
		// looking like somebody had asked them for something.
		return Notice{}, &values.ParseError{
			Field: fieldRecipient, Code: "self_coaching",
			Message: "a coaching notice goes to somebody else",
		}
	}
	shares, err := mates.SharesLiveTeamWithCaller(ctx, recipient.UUID)
	if err != nil {
		return Notice{}, err
	}
	if !shares {
		return Notice{}, apperrors.ErrPermissionDenied
	}

	// What ACTUALLY admitted this write, recorded beside it.
	//
	// The audit row's authorization_rule renders the caller's object policy —
	// `role[manager] notice.create …` — and no such grant was consulted: the
	// admitting facts are the coaching role and the live-team edge, and the
	// second is a fact about the world on the day it was true. Membership
	// changes, so a reader years later asking "why was this allowed" needs the
	// edge stated rather than re-derived from a team that has since moved.
	return s.insertNotice(ctx, recipient, string(kind), subject, note, map[string]any{
		"admitted_by":      "coaching_role_and_live_team",
		"shared_live_team": true,
	})
}
