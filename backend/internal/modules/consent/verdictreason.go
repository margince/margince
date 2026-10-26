// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Why a verdict answers as it does, as a code. The contract's
// `ContactConsentGuardEntry.reason_code` enum lists the same values, and a
// screen says each in its reader's language. Reason stays the English
// sentence, for an agent and for the log.
const (
	ReasonObjection                    = "objection"
	ReasonSuppressedObjection          = "suppressed_objection"
	ReasonSuppressedRestricted         = "suppressed_restricted"
	ReasonSuppressedSubjectRequest     = "suppressed_subject_request"
	ReasonSuppressedHardBounce         = "suppressed_hard_bounce"
	ReasonSuppressed                   = "suppressed"
	ReasonContractNotice               = "contract_notice"
	ReasonNoCallPath                   = "no_call_path"
	ReasonSaidWeMayWrite               = "said_we_may_write"
	ReasonUnconfirmedDOI               = "unconfirmed_double_opt_in"
	ReasonNoConnection                 = "no_connection"
	ReasonQualifyingEvent              = "qualifying_event"
	ReasonConsentConfirmed             = "consent_confirmed"
	ReasonConsentWithdrawn             = "consent_withdrawn"
	ReasonNoConsentExceptionNotOffered = "no_consent_exception_not_offered"
	ReasonNoConsent                    = "no_consent"
)

// said fills a verdict's sentence from its code.
func said(v Verdict) Verdict {
	v.Reason = reasonSentence(v)
	return v
}

// knownDate is nil for the zero time. An objection with a state row and no
// proof row has no date to quote.
func knownDate(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	return &at
}

func suppressionReasonCode(kind string) string {
	switch kind {
	case commsauthz.ReasonObjection:
		return ReasonSuppressedObjection
	case commsauthz.ReasonRestricted:
		return ReasonSuppressedRestricted
	case commsauthz.ReasonSubjectRequest:
		return ReasonSuppressedSubjectRequest
	case commsauthz.ReasonHardBounce:
		return ReasonSuppressedHardBounce
	default:
		return ReasonSuppressed
	}
}

var reasonSentences = map[string]string{
	ReasonObjection:                    "they objected, and an objection overrides every other basis",
	ReasonSuppressedObjection:          "they objected to marketing, and an objection overrides every other basis",
	ReasonSuppressedRestricted:         "a statutory restriction is recorded against writing to them",
	ReasonSuppressedSubjectRequest:     "they asked not to be contacted, and a rep recorded it",
	ReasonSuppressedHardBounce:         "their address does not accept mail",
	ReasonSuppressed:                   "a suppression is recorded against writing to them",
	ReasonContractNotice:               "account and contract notices need no consent",
	ReasonNoCallPath:                   "no call path is configured",
	ReasonSaidWeMayWrite:               "they agreed to be written to",
	ReasonUnconfirmedDOI:               "consent was recorded but never confirmed by the double opt-in",
	ReasonNoConnection:                 "they have never written to you and no deal or inquiry connects you",
	ReasonConsentConfirmed:             "they gave consent, with the confirmation on file",
	ReasonConsentWithdrawn:             "they withdrew consent for this purpose",
	ReasonNoConsentExceptionNotOffered: "no consent recorded, and the existing-customer exception is not offered",
	ReasonNoConsent:                    "no consent recorded",
}

func reasonSentence(v Verdict) string {
	switch {
	case v.ReasonCode == ReasonObjection && v.ReasonAt != nil:
		return fmt.Sprintf("they objected on %s, and an objection overrides every other basis",
			v.ReasonAt.Format("2 Jan 2006"))
	case v.ReasonCode == ReasonQualifyingEvent && v.Qualifying != nil:
		return qualifyingReason(*v.Qualifying)
	}
	return reasonSentences[v.ReasonCode]
}

func qualifyingReason(event QualifyingEvent) string {
	when := event.OccurredAt.Format("2 Jan")
	switch event.Kind {
	case "inbound_message":
		return fmt.Sprintf("they wrote to you on %s", when)
	case "inquiry":
		return fmt.Sprintf("they made an inquiry on %s", when)
	case "active_deal":
		return "an open deal connects you"
	case KindMeeting:
		// Tenseless: the arm admits a meeting still in the diary, and a reason
		// on the wrong side of today reads as a bug.
		return fmt.Sprintf("a meeting on %s connects you", when)
	case KindRequestedBySubject:
		// Named rather than left to the default arm below, because what this
		// says is stronger than "an exchange happened": the contact ASKED. A rep
		// reading why a send is lawful should see whose move it was.
		return fmt.Sprintf("they asked you to write to them on %s: %s", when, event.Note)
	default:
		return fmt.Sprintf("a recorded exchange on %s: %s", when, event.Note)
	}
}
