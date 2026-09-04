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
	// ReasonFrequencyCapReached is a jurisdiction's ceiling on how many
	// advertising messages one address may receive in a window. A fact about
	// VOLUME rather than about the person: nothing they did refuses this
	// message, and the same message is lawful again once the window rolls.
	ReasonFrequencyCapReached = "frequency_cap_reached"
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
	// A jurisdiction's ceiling on advertising is decided by that jurisdiction,
	// not by how far along a rollout is. It is here for the same reason as the
	// rest and one of its own: an installation that declares a country is
	// asserting which law it sends under, so a mode setting that let it exceed
	// that country's statutory limit would make the declaration false. It is
	// also the one denial a sender can clear by waiting — the window rolls and
	// the same message becomes lawful — so refusing costs a delay rather than
	// the message.
	ReasonFrequencyCapReached: true,
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

// Effective folds the modes in: what this set actually permits right now.
//
// The mode is read per DECISION rather than once for the set, because the
// recipients of one message need not resolve to one category — a reply to a
// thread that copies somebody the engine calls marketing is two categories in
// one send, and a single mode would have to pick one of them. Each recipient
// is judged under the authority its own category carries.
//
// In enforce the engine rules that recipient. In observe and warn the old gate
// does. An absolute denial rules in every mode, whatever any category's mode
// says — that is what "absolute" means here.
//
// Whole-message refusal is preserved: one recipient the engine refuses under
// enforce refuses the send, exactly as one recipient the old gate refuses does.
func (s DecisionSet) Effective(modeFor func(Category) Mode, legacyAllowed bool) bool {
	if s.HasAbsoluteDenial() {
		return false
	}
	if len(s.Decisions) == 0 {
		// No decision is not an allow. An empty set reaching here means the
		// engine was asked about nobody, and a message with no authorized
		// recipient is not a message that may go out.
		return false
	}
	enforced := false
	for _, d := range s.Decisions {
		if modeFor(d.Resolved) != ModeEnforce {
			continue
		}
		enforced = true
		if d.Verdict != VerdictAllow {
			return false
		}
	}
	// THE ENGINE ALONE DECIDES A RECIPIENT IT ENFORCES.
	//
	// While every category observed, this returned legacyAllowed and the old
	// purpose gate ruled. Under enforce that conjunction is not caution, it is
	// the old gate's defects kept alive: it answers on a caller-supplied
	// purpose key, and its business-correspondence arm reads qualifying events
	// only — so an ordinary reply to a thread the subject started is refused
	// for want of a consent row nobody ever had reason to record. The engine
	// resolves that reply from the thread itself, which is the strongest ground
	// a message can have, and being overruled by a weaker authority is the
	// regression this rollout exists to end.
	//
	// A set with NO enforced recipient still defers. That is not a fallback: it
	// is a category still being observed, and the old gate is what decides
	// there until it is not.
	if enforced {
		return true
	}
	return legacyAllowed
}
