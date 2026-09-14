// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

// What a check is, and how bad breaking it is.
//
// A rule used to be a bare string at the site that raised it, and its severity
// was nowhere — so the loop that picks which of two drafts to serve could only
// count findings. Counting is not a judgement: the band-gated loops append one
// finding per matching phrase while the world-claim rules report one per rule,
// so a draft saying the same wrong thing three ways outscored a draft making
// two false statements, and the more serious one was discarded.
//
// The severity travels WITH the name, in one value, so a rule cannot exist
// without one. That is the compile-time half of the fix: a new check has to
// name a Rule, and there is no way to write a Rule that has not said which kind
// of wrong it is.

// Severity says what KIND of wrong a finding is, which is the only comparison a
// reader would agree with.
//
// TWO CLASSES, deliberately, and not a numeric scale. "Is this a false
// statement, or is it a phrasing tic" is decidable per rule by anyone who reads
// the rule; "is this a 7 or an 8" is not, and a scale invites a rule to be
// tuned until the arithmetic comes out the way somebody wanted.
type Severity int

const (
	// Style is a phrasing or shape defect: filler, a wall of text, a subject
	// line too long to read. The message is true; it reads badly.
	Style Severity = iota
	// Claim is the draft stating something about the world or the relationship
	// that the input does not support — a call that never happened, a meeting
	// nobody booked, a reply prefix on a thread that was never replied to.
	//
	// Strictly worse than Style, and the ordering is the whole point: a rep
	// sends what the product wrote, and a false claim reaches the recipient as
	// the company's own word. A stale phrase is a habit; an invented call is a
	// thing that did not happen.
	Claim
)

// Rule is one check's identity and its severity, as one value.
type Rule struct {
	name     string
	severity Severity
}

// Name is what a log and a regeneration prompt call this rule.
func (r Rule) Name() string { return r.name }

// Severity is how bad breaking it is.
func (r Rule) Severity() Severity { return r.severity }

// String makes a Rule print as its name wherever a string was printed before.
func (r Rule) String() string { return r.name }

// The rules, each declaring what kind of wrong it is.
//
// Grouped by severity rather than by the file that raises them, because the
// grouping IS the classification and a reader checking one rule's class should
// see it beside its peers rather than beside the code that happens to match it.
var (
	// A false statement about the world or the relationship.
	RuleInventedRelationship   = Rule{"invented-relationship", Claim}
	RuleInventedFirstTouch     = Rule{"invented-first-touch", Claim}
	RuleAssumedResolution      = Rule{"assumed-resolution", Claim}
	RuleAssumedMemory          = Rule{"assumed-memory", Claim}
	RuleInventedConversation   = Rule{"invented-conversation", Claim}
	RuleAttributedClaim        = Rule{"attributed-claim", Claim}
	RuleUnscheduledArrangement = Rule{"unscheduled-arrangement", Claim}
	RuleUnearnedReplyPrefix    = Rule{"unearned-reply-prefix", Claim}
	RuleInventedHistorySubject = Rule{"invented-history-subject", Claim}

	// A phrasing or shape defect. The message is true; it reads badly.
	RuleWellbeingOpener = Rule{"wellbeing-opener", Style}
	RuleMixedRegister   = Rule{"mixed-register", Style}
	RuleUnbrokenBlock   = Rule{"unbroken-block", Style}
	RuleEmptySubject    = Rule{"empty-subject", Style}
	RuleLongSubject     = Rule{"long-subject", Style}
)

// Worst is the severity of the most serious finding present, and false when
// there are none.
func Worst(findings []Finding) (Severity, bool) {
	if len(findings) == 0 {
		return Style, false
	}
	worst := findings[0].Rule.Severity()
	for _, f := range findings[1:] {
		if f.Rule.Severity() > worst {
			worst = f.Rule.Severity()
		}
	}
	return worst, true
}

// Rules counts the DISTINCT rules a set of findings breaks.
//
// The comparable number, and the reason the raw length is not: the band-gated
// loops append one finding per matching phrase, so three phrasings of one
// assumption counted three against another draft's two separate rules. Nobody
// asked which draft has more matches; they asked which draft is worse.
func Rules(findings []Finding) int {
	seen := make(map[string]struct{}, len(findings))
	for _, f := range findings {
		seen[f.Rule.Name()] = struct{}{}
	}
	return len(seen)
}
