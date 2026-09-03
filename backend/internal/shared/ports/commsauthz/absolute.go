// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

// Reason codes a decision carries. Stable, bounded and safe to put in a metric
// label or show an operator: none of them names a person, an address or a
// message.
const (
	// ReasonObjection is Art. 21 — the subject objected to direct marketing.
	ReasonObjection = "marketing_objection"
	// ReasonRestricted is a statutory or subject-requested processing restriction.
	ReasonRestricted = "processing_restricted"
	// ReasonHardBounce is an address that does not accept mail.
	ReasonHardBounce = "hard_bounce"
	// ReasonUnconfirmedDOI is a marketing grant whose round trip never happened.
	ReasonUnconfirmedDOI = "unconfirmed_double_opt_in"
	// ReasonNoEvidence is a category whose evidence is absent or does not match.
	ReasonNoEvidence = "no_compatible_evidence"
	// ReasonLegacyTransactionalUnevidenced is a caller naming the old
	// transactional purpose with nothing to support it.
	ReasonLegacyTransactionalUnevidenced = "legacy_transactional_unevidenced"
	// ReasonUnknownPurpose is a purpose key nothing defines.
	ReasonUnknownPurpose = "unknown_purpose"
	// ReasonNoSubject is a recipient that resolves to nobody, or to two people.
	ReasonNoSubject = "recipient_resolves_to_no_single_subject"
	// ReasonNoMarketingConsent is marketing without a grant or an exception.
	ReasonNoMarketingConsent = "no_marketing_consent"
	// ReasonConsentWithdrawn is Art. 7(3): the subject took a consent back.
	// Distinct from an objection because they are different legal facts, and a
	// proof row that called one the other would misstate what somebody did.
	ReasonConsentWithdrawn = "consent_withdrawn"
	// ReasonAllowed is the allow path's own code, so every row has one.
	ReasonAllowed = "allowed"
)

// absoluteDenials are the refusals no rollout mode may soften.
//
// Held by: TestAbsoluteDenialsSurviveEveryMode (commsauthz_test.go), which
// fails if any member stops denying under observe or warn.
//
// Observe mode exists so a new engine can be measured against the old gate
// without blocking legitimate mail while the two are compared. It does NOT
// exist to let a message reach somebody who objected, whose processing is
// restricted, whose address is dead, or whose marketing consent was never
// confirmed. Those four are decided by law or by the subject, not by how far
// along a rollout is, so they deny from the first day the engine runs.
var absoluteDenials = map[string]bool{
	ReasonObjection:      true,
	ReasonRestricted:     true,
	ReasonHardBounce:     true,
	ReasonUnconfirmedDOI: true,
	// A recipient the engine cannot resolve to exactly one subject is the
	// fifth, and it belongs here for a different reason than the other four:
	// they are refusals ABOUT somebody, this one is the admission that nobody
	// knows who the message is going to. No suppression, objection or consent
	// state can be read for a subject that was never identified, so letting a
	// rollout mode soften it would send precisely the messages nothing was
	// able to check.
	ReasonNoSubject: true,
	// A withdrawal is the subject saying stop. It is a different act from an
	// objection and gets its own code, but it binds exactly as hard: no
	// rollout mode may send to somebody who took their consent back.
	ReasonConsentWithdrawn: true,
}

// Absolute reports whether this reason denies regardless of Mode.
func Absolute(reasonCode string) bool { return absoluteDenials[reasonCode] }

// HasAbsoluteDenial reports whether any recipient was refused for a reason the
// rollout mode may not soften. A caller in observe mode still refuses the send
// when this is true.
func (s DecisionSet) HasAbsoluteDenial() bool {
	for _, d := range s.Decisions {
		if d.Verdict != VerdictAllow && Absolute(d.ReasonCode) {
			return true
		}
	}
	return false
}

// Effective folds the mode in: what this set actually permits right now.
//
// In enforce the engine rules. In observe and warn the old gate rules, EXCEPT
// for an absolute denial, which rules in every mode.
func (s DecisionSet) Effective(mode Mode, legacyAllowed bool) bool {
	if s.HasAbsoluteDenial() {
		return false
	}
	if mode == ModeEnforce {
		return legacyAllowed && s.Allowed()
	}
	return legacyAllowed
}
