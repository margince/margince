// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

// Who decided that we may or may not write to somebody, and who may overrule it.
//
// One rule, and it has no special cases: YOU MAY OVERRULE A DECISION MADE BELOW
// YOUR LEVEL, NEVER AT OR ABOVE IT. The tiers are ordered by how much authority
// the decision carries, not by how senior the person is — which is why the
// subject outranks the admin who administers the installation they are recorded
// in.
//
// The engine deciding from evidence is the weakest, because it is a reading of
// what the record happens to show and the record is often incomplete. A rep who
// knows the customer phoned them can say so. An admin can overrule the rep. And
// nobody overrules the person themselves: an Art. 21 objection to direct
// marketing is absolute in law, so a product that offered an admin a button to
// lift one would be offering a button that cannot lawfully be pressed.

// AuthorityLevel is the tier of the party a decision came from.
type AuthorityLevel string

const (
	// LevelMachine is the engine's own reading of the record's evidence.
	LevelMachine AuthorityLevel = "machine"
	// LevelUser is a rep or SDR exercising judgement about a contact they know.
	LevelUser AuthorityLevel = "user"
	// LevelAdmin is ops or a workspace administrator.
	LevelAdmin AuthorityLevel = "admin"
	// LevelSubject is the person the data is about, acting for themselves.
	LevelSubject AuthorityLevel = "subject"
)

// rank orders the tiers. Unexported and unexported-only: a caller comparing
// ranks is a caller reimplementing CanOverrule, and the second implementation
// is the one that stops matching.
//
// An unknown level ranks ABOVE every real one rather than below. A level this
// build does not recognise is a level written by a newer build, and treating it
// as weak would let this one overrule a decision it cannot even name.
func (l AuthorityLevel) rank() int {
	switch l {
	case LevelMachine:
		return 0
	case LevelUser:
		return 1
	case LevelAdmin:
		return 2
	case LevelSubject:
		return 3
	default:
		return 4
	}
}

// Valid reports whether the level is one this build knows.
//
// Storage is constrained by a CHECK, so this guards the other direction: a
// level arriving from a request body or a newer row reaches a decision without
// being silently treated as machine.
func (l AuthorityLevel) Valid() bool {
	return l == LevelMachine || l == LevelUser || l == LevelAdmin || l == LevelSubject
}

// CanOverrule reports whether a party at this level may overrule a decision
// recorded at `decided`.
//
// Strictly greater, never equal. Equal ranks mean two parties of the same
// standing disagree, and the product does not let the second one silently win —
// a rep does not overrule another rep's judgement about a contact, and an admin
// does not quietly reverse another admin. Reversing a peer's decision is a
// deliberate act with its own audit trail, not a side effect of sending mail.
//
// The one tier nothing satisfies is LevelSubject: no level outranks it, so this
// returns false for every caller including LevelAdmin. That is the intended
// answer and not an oversight.
func (l AuthorityLevel) CanOverrule(decided AuthorityLevel) bool {
	return l.rank() > decided.rank()
}

// LevelForReason maps a refusal the engine produced to the authority it speaks
// with, so one vocabulary answers "who decided this" whether the refusal came
// from a stored suppression row or from the engine's own reading.
//
// The question each arm answers is narrow: WHOSE DECISION WAS THIS. It is not
// "how serious is the refusal" and not "how likely is a seat to be right". A
// refusal can bind absolutely and still be nobody's decision — a dead mailbox,
// a rolling volume window, an address the engine cannot resolve to one person.
// Those are facts about the world, and a fact is corrected rather than
// overruled, which is why they are LevelMachine and a seat may clear them.
//
// Held by: TestEveryAbsoluteReasonIsClassifiedDeliberately (authority_test.go),
// which fails when a new absolute reason is added without an arm here.
func LevelForReason(reasonCode string) AuthorityLevel {
	switch reasonCode {
	// THE SUBJECT'S OWN ACT. An objection, a withdrawal and a restriction are
	// things the person did, and Art. 21 makes the first absolute. Nobody in
	// the installation lifts these, admin included.
	case ReasonObjection, ReasonRestricted, ReasonConsentWithdrawn:
		return LevelSubject

	// EVERYTHING ELSE IS THE ENGINE READING AN INCOMPLETE RECORD, and a seat
	// may know better. That includes four refusals that BIND ABSOLUTELY but are
	// nobody's decision, so being overrulable is the right answer for each:
	//
	//   - a hard bounce is a mailbox fact, cleared by correcting the address;
	//   - a frequency cap is a rolling window that clears itself, so the honest
	//     answer is "not until Thursday" and never "never";
	//   - an unresolvable recipient means two records share an address, and the
	//     remedy is a merge — a human act on the CRM, not a lift of anybody's
	//     wishes;
	//   - an unconfirmed double opt-in is the absence of the subject's act
	//     rather than an act, and an installation holding a paper opt-in needs
	//     a way to say so.
	//
	// Absolute and overrulable are different axes, and Absolute() already
	// carries the first. Collapsing them here would make an admin stare at a
	// dead button for a duplicate contact they could fix in one merge.
	default:
		return LevelMachine
	}
}
