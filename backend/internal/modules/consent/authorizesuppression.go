// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a suppression binds, and what it leaves alone.
//
// A suppression is not one rule. Three kinds of "do not write to this contact"
// exist and they bind different things, so applying the strongest one to every
// message refuses mail nobody objected to — an Art. 21 marketing objection
// stopping that contact's INVOICE is the case this file exists to prevent.

import (
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// applySuppression narrows a decision by the live suppressions on a
// recipient, against the category the record actually resolved to and the
// purpose (if any) the send itself resolved to.
//
// Called AFTER resolution, which is the whole point: liveSuppression can say a
// row exists, but only the resolved category — and, for a narrow row, the
// resolved purpose — says whether it binds THIS message.
func applySuppression(d commsauthz.Decision, stops []liveStop, sendPurpose *ids.UUID) commsauthz.Decision {
	if len(stops) == 0 {
		return d
	}
	// EVERY stop is asked, and the first that binds refuses. A contact may
	// carry several at once, and since reach became category- and
	// purpose-dependent they no longer agree: an objection binds only
	// marketing while a hard bounce binds everything, so picking one and
	// asking it alone lets the others through.
	//
	// The row records the binding kind when one binds, and otherwise the first
	// that stood — a message that went out while a suppression stood is exactly
	// what a later reader needs to see, with the verdict beside it saying the
	// engine knew and let it through.
	for _, s := range stops {
		if suppressionBinds(s.Kind, d.Resolved, s.PurposeID, sendPurpose) {
			d.Verdict = commsauthz.VerdictDeny
			d.ReasonCode = s.Kind
			d.Suppression = s.Kind
			return d
		}
	}
	// None of them binds this send, so it stands and the row records that a
	// stop stood anyway — but only a BROAD one, because only a broad one's
	// non-binding is explicable from what the row stores.
	//
	// communication_decision keeps kind and resolved_category and NO purpose.
	// A broad objection recorded beside an allowed invoice explains itself:
	// the two columns say marketing was refused and this was not marketing.
	// A NARROW objection recorded beside an allowed marketing send does not —
	// the columns read "we knew they objected to marketing and sent marketing
	// anyway", and the purpose that makes it correct is nowhere in the row.
	// That combination was impossible before purpose_id existed, because an
	// objection always bound a marketing send; it became the ordinary outcome
	// the moment a stop could name one list. privacy/sarcommunication.go hands
	// these columns to the subject in their Art. 15 export, so the unexplained
	// row is not an internal curiosity — it is a self-contradiction shown to
	// the person it is about.
	//
	// So a narrow row is left off this field rather than misdescribed. It is
	// not lost: the suppression table still holds it, and a review opened from
	// this decision reads it through Decision.PurposeID.
	//
	// Sorted for the record's sake only: an unordered read would make this
	// field depend on the planner, and the same message would describe itself
	// differently on two runs.
	broad := make([]liveStop, 0, len(stops))
	for _, s := range stops {
		if s.PurposeID == nil {
			broad = append(broad, s)
		}
	}
	if len(broad) == 0 {
		return d
	}
	slices.SortFunc(broad, func(a, b liveStop) int {
		return strings.Compare(a.Kind, b.Kind)
	})
	d.Suppression = broad[0].Kind
	return d
}

// suppressionBindsAny reports whether ANY live stop binds this category and
// this send's purpose.
//
// The same question applySuppression asks, asked earlier: the basis writers run
// before the decision is assembled, and what they need to know is not whether
// the subject carries a stop but whether one reaches THIS message. A contact
// who objected to marketing and is being sent an invoice carries a live stop
// that binds nothing here, and the send is lawful — so the ground it relied on
// belongs on the record like any other.
//
// Built from suppressionBinds rather than repeating its cases, so the write and
// the refusal cannot come to disagree about what a stop covers — including
// about the send's purpose, which a narrow row is compared against here exactly
// as applySuppression compares it later.
func suppressionBindsAny(stops []liveStop, category commsauthz.Category, sendPurpose *ids.UUID) bool {
	for _, s := range stops {
		if suppressionBinds(s.Kind, category, s.PurposeID, sendPurpose) {
			return true
		}
	}
	return false
}

// suppressionBinds reports whether one kind of suppression stops one category.
//
// FOUR RULES, and the default refuses. Each widening below is a deliberate line
// naming the reason code it applies to, because the one time this was written
// as a permissive default it swallowed subject_request — the only kind a seat
// can write, and so the only kind a real installation holds — and turned
// "please stop emailing me" into permission to send five categories of mail.
//
//   - A MARKETING OBJECTION binds marketing and nothing else. Art. 21 is an
//     objection to direct marketing, so it beats consent and every exception
//     for that category, and says nothing about the invoice the same contact is
//     owed. Today's objectionStands is already purpose-scoped for the legacy
//     gate; this is the same scoping, one layer up.
//   - A STATUTORY RESTRICTION (Art. 18) binds everything except the three
//     categories Art. 18(2) does not reach: a security warning is an Art. 34
//     obligation, a privacy notice discharges Art. 13/14, and an opt-out
//     acknowledgement is Art. 12(3) confirmation of the subject's own act.
//     NOT the other two subject-serving categories: a record confirmation is
//     an unsolicited invitation to review a record and a consent confirmation
//     solicits a NEW grant, and Art. 18(2) offers no gateway for either.
//   - A SUBJECT'S REQUEST binds everything EXCEPT those same three. It used to
//     bind all fourteen, on the reasoning that somebody said stop and answering
//     with unasked-for mail is the thing they asked us not to do. That reasoning
//     is right about the twelve and wrong about these three, and the way it was
//     wrong is visible: a contact who said "stop emailing me" never received the
//     confirmation that we had stopped, because opt-out confirmation was bound
//     by the very request it was confirming. The privacy notice that answers
//     their own rights request and the security warning about their own account
//     fail the same way. Art. 12(3), 13/14 and 34 are obligations the controller
//     owes REGARDLESS of what the subject wants sent, which is exactly the
//     shape of Art. 18(2)'s carve-out — so the two share survivesARestriction
//     rather than keeping a second list to drift apart from it.
//   - Everything else binds every category, which is where a hard bounce lands
//     and where an unrecognised reason code lands. No template makes a dead
//     address accept mail, and a code this function does not know must refuse
//     rather than pick a narrower rule — the direction liveSuppression,
//     blockedReasonCode and the validators all already fail in.
//
// THE PURPOSE BOUNDARY, added for communication_suppression.purpose_id: a
// broad row (rowPurpose nil) binds every marketing send exactly as before that
// column existed. A NARROW row binds ONLY a send whose own resolved purpose
// (sendPurpose) equals it — two marketing purposes are two different pieces of
// direct marketing, and an objection to one says nothing about the other. A
// send with NO resolved purpose (sendPurpose nil — the evidence arms, which
// never consult a purpose key at all) is deliberately NOT bound by a narrow
// row: nothing established that this particular message belongs to the
// purpose the row names, and guessing that it does would refuse mail on a
// coincidence rather than a match. Only ReasonObjection ever carries a
// non-nil rowPurpose today; every other kind's row is always broad, so the
// comparison never narrows them.
func suppressionBinds(kind string, category commsauthz.Category, rowPurpose, sendPurpose *ids.UUID) bool {
	switch kind {
	case commsauthz.ReasonObjection:
		return category == commsauthz.CategoryMarketing &&
			(rowPurpose == nil || (sendPurpose != nil && *rowPurpose == *sendPurpose))
	case commsauthz.ReasonRestricted, commsauthz.ReasonSubjectRequest:
		return !survivesARestriction(category)
	default:
		return true
	}
}

// survivesARestriction reports whether Art. 18(2) leaves room for a category.
//
// Three, not the five ServesTheSubject names: that predicate answers "does this
// exist to serve the recipient", which is the right question for a HARD
// suppression and too wide for a statutory restriction.
func survivesARestriction(c commsauthz.Category) bool {
	switch c {
	case commsauthz.CategorySecurityNotice, commsauthz.CategoryPrivacyNotice,
		commsauthz.CategoryOptoutConfirmation:
		return true
	default:
		return false
	}
}

// bindsEveryCategory names a kind among these that stops every category the
// engine can resolve to, so the caller may answer without resolving at all.
//
// MUST AGREE WITH suppressionBinds's unconditional arm, and a restatement is
// how the two drift: an earlier version had this naming hard bounce while
// suppressionBinds spared the five for a restriction, and the comment on each
// described the other's rule. Held by TestTheEarlyExitAgreesWithTheRule, which
// asserts the implication over every category and every reason code.
func bindsEveryCategory(kinds []string) (string, bool) {
	for _, kind := range kinds {
		switch kind {
		case commsauthz.ReasonObjection, commsauthz.ReasonRestricted,
			// subject_request joined the scoped kinds when it stopped binding
			// the three categories Art. 12(3)/13/14/34 oblige us to send. Left
			// here it would take the early exit and refuse an opt-out
			// confirmation without ever asking suppressionBinds — the rule and
			// its shortcut disagreeing, which is what this pair exists to
			// prevent.
			commsauthz.ReasonSubjectRequest:
		default:
			return kind, true
		}
	}
	return "", false
}
