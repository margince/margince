// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a suppression binds, and what it leaves alone.
//
// A suppression is not one rule. Three kinds of "do not write to this person"
// exist and they bind different things, so applying the strongest one to every
// message refuses mail nobody objected to — an Art. 21 marketing objection
// stopping that person's INVOICE is the case this file exists to prevent.

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// applySuppression narrows a decision by a live suppression, against the
// category the record actually resolved to.
//
// Called AFTER resolution, which is the whole point: liveSuppression can say a
// row exists, but only the resolved category says whether it binds THIS
// message.
func applySuppression(d commsauthz.Decision, kinds []string) commsauthz.Decision {
	if len(kinds) == 0 {
		return d
	}
	// EVERY kind is asked, and the first that binds refuses. A person may
	// carry several at once, and since this change they no longer agree: an
	// objection binds only marketing while a hard bounce binds everything, so
	// picking one and asking it alone lets the others through.
	//
	// The row records the binding kind when one binds, and otherwise the first
	// that stood — a message that went out while a suppression stood is exactly
	// what a later reader needs to see, with the verdict beside it saying the
	// engine knew and let it through.
	for _, kind := range kinds {
		if suppressionBinds(kind, d.Resolved) {
			d.Verdict = commsauthz.VerdictDeny
			d.ReasonCode = kind
			d.Suppression = kind
			return d
		}
	}
	// None of them binds this category, so the send stands and the row records
	// that one stood anyway. Sorted for the record's sake only: an unordered
	// read would make this field depend on the planner, and the same message
	// would describe itself differently on two runs.
	sorted := slices.Clone(kinds)
	slices.Sort(sorted)
	d.Suppression = sorted[0]
	return d
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
//     for that category, and says nothing about the invoice the same person is
//     owed. Today's objectionStands is already purpose-scoped for the legacy
//     gate; this is the same scoping, one layer up.
//   - A STATUTORY RESTRICTION (Art. 18) binds everything except the three
//     categories Art. 18(2) does not reach: a security warning is an Art. 34
//     obligation, a privacy notice discharges Art. 13/14, and an opt-out
//     acknowledgement is Art. 12(3) confirmation of the subject's own act.
//     NOT the other two subject-serving categories: a record confirmation is
//     an unsolicited invitation to review a record and a consent confirmation
//     solicits a NEW grant, and Art. 18(2) offers no gateway for either.
//   - A SUBJECT'S REQUEST binds everything. Somebody said stop, in words a rep
//     wrote down; answering it with mail they did not ask for is the thing they
//     asked us not to do.
//   - Everything else binds every category too, which is where a hard bounce
//     lands and where an unrecognised reason code lands. No template makes a
//     dead address accept mail, and a code this function does not know must
//     refuse rather than pick a narrower rule — the direction liveSuppression,
//     blockedReasonCode and the validators all already fail in.
func suppressionBinds(kind string, category commsauthz.Category) bool {
	switch kind {
	case commsauthz.ReasonObjection:
		return category == commsauthz.CategoryMarketing
	case commsauthz.ReasonRestricted:
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
		if kind != commsauthz.ReasonObjection && kind != commsauthz.ReasonRestricted {
			return kind, true
		}
	}
	return "", false
}
