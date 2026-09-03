// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Basis is what makes a message lawful. It is EVIDENCE, not consent: eight of
// the nine are things that happened and were recorded, and only BasisConsent
// is somebody agreeing to something.
type Basis string

const (
	// BasisContract is a live contract or customer relationship.
	BasisContract Basis = "contract"
	// BasisPrecontractRequest is a request the subject made before contracting.
	BasisPrecontractRequest Basis = "precontract_request"
	// BasisSubjectInitiatedCorrespondence is the subject having written first.
	BasisSubjectInitiatedCorrespondence Basis = "subject_initiated_correspondence"
	// BasisLegitimateInterests carries an assessment, and never covers marketing.
	BasisLegitimateInterests Basis = "legitimate_interests"
	// BasisLegalObligation is a duty the controller must discharge.
	BasisLegalObligation Basis = "legal_obligation"
	// BasisVitalOrSecurityInterest covers a security or safety notice.
	BasisVitalOrSecurityInterest Basis = "vital_or_security_interest"
	// BasisConsent is the subject's own purpose-specific agreement.
	BasisConsent Basis = "consent"
	// BasisExistingCustomerException is the narrow own-similar-products lane.
	BasisExistingCustomerException Basis = "existing_customer_exception"
	// BasisVNSubjectAgreement is the Vietnamese advertising agreement.
	BasisVNSubjectAgreement Basis = "vn_subject_agreement"
)

// Verdict is what the engine answered.
type Verdict string

const (
	// VerdictAllow sends.
	VerdictAllow Verdict = "allow"
	// VerdictDeny refuses, and the reason code says why.
	VerdictDeny Verdict = "deny"
	// VerdictReview means nobody has established this either way. It is not a
	// soft allow: it does not send, and it names the work that would settle it.
	VerdictReview Verdict = "review"
)

// Phase is when the question was asked. Preview is deliberately absent — a
// preview authorizes nothing and is never persisted, so it is not a phase a
// decision row can carry.
type Phase string

const (
	// PhaseStaging is the decision taken as the message is written down.
	PhaseStaging Phase = "staging"
	// PhaseTransmit is the decision taken immediately before provider I/O.
	PhaseTransmit Phase = "transmit"
)

// Mode is how much authority the engine's answer carries for one category.
type Mode string

const (
	// ModeObserve records the decision and lets the old gate rule.
	ModeObserve Mode = "observe"
	// ModeWarn explains the disagreement without changing the outcome.
	ModeWarn Mode = "warn"
	// ModeEnforce lets the decision control staging and transmission.
	ModeEnforce Mode = "enforce"
)

// Evidence names the records a decision rests on, by id and never by content.
// Every field is optional: which ones a category needs is the validator's
// question, not this type's.
type Evidence struct {
	ActivityID ids.UUID
	DealID     ids.UUID
	InvoiceID  ids.UUID
	ContractID ids.UUID
	// ConsentEventID is the proof row a marketing allow stands on.
	ConsentEventID ids.UUID
	// BasisID is a durable communication_basis row.
	BasisID ids.UUID
}

// Request is one message put to the engine.
type Request struct {
	// Recipients carries To, Cc and Bcc merged. A blind copy is blind to the
	// other recipients and never to the engine.
	Recipients []connector.Recipient
	// Context is the category the caller CLAIMS. Empty is honest: a caller
	// that does not know says so, and resolution works it out from the origin.
	Context Category
	// LegacyPurposeKey is the deprecated consent_purpose. It never authorizes
	// by itself; resolution maps it conservatively and records that it did.
	LegacyPurposeKey string
	// MarketingPurpose names the topic a marketing send is for.
	MarketingPurpose string
	// OperatorReason is what a human typed when the first message was genuinely
	// ambiguous. It is recorded, and it grants nothing.
	OperatorReason string
	// Evidence is the typed record references the caller can name.
	Evidence Evidence
	// AnchorActivityID is the message being replied to, zero when there is none.
	AnchorActivityID ids.UUID
	// Links are the records this message is filed under.
	Links []ids.UUID
	// Subject and Body are fingerprinted, never stored on the decision.
	Subject string
	Body    string
}

// Decision is the engine's answer about ONE recipient at ONE phase.
type Decision struct {
	Recipient   connector.Recipient
	SubjectKind string
	SubjectID   ids.UUID
	Phase       Phase
	Requested   Category
	Resolved    Category
	Verdict     Verdict
	ReasonCode  string
	Basis       Basis
	Evidence    Evidence
	Suppression string
	Mode        Mode
	// LegacyVerdict is what the old purpose gate said, so a disagreement is
	// visible in the row rather than only in a metric.
	LegacyVerdict string
}

// DecisionSet holds the per-recipient answers for one delivery and phase.
type DecisionSet struct {
	Decisions []Decision
}

// Allowed reports whether the whole message may go.
//
// It is a conjunction, and that is a decision rather than an oversight: one
// denied recipient refuses the message rather than quietly sending a smaller
// version of it. A rep who wrote to four people and reached three, without
// being told which, has been lied to about what happened.
func (s DecisionSet) Allowed() bool {
	if len(s.Decisions) == 0 {
		return false
	}
	for _, d := range s.Decisions {
		if d.Verdict != VerdictAllow {
			return false
		}
	}
	return true
}

// Denied returns the decisions that refused, for a message that names them.
func (s DecisionSet) Denied() []Decision {
	var out []Decision
	for _, d := range s.Decisions {
		if d.Verdict != VerdictAllow {
			out = append(out, d)
		}
	}
	return out
}
