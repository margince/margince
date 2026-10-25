// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Old rows encode reasons in English. New writes carry codes beside the
// compatibility sentence, so a changed sentence cannot change its meaning.

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

type domainAdmissionReason struct {
	Text       string
	Code       crmcontracts.BlockedDomainReasonCode
	SenderKind crmcontracts.BlockedDomainSenderKind
}

func statedAdmissionReason(text string) domainAdmissionReason {
	return domainAdmissionReason{Text: text, Code: crmcontracts.BlockedDomainReasonCodeStated}
}

// claimedDomainReason is the reason a contact's company claim records, read
// back as reason_code company_created.
const claimedDomainReason = "a contact put this domain on a company they created"

// keptDomainReason remains readable to clients that predate reason codes.
const keptDomainReason = "Kept from the open domain question: a colleague judged this domain a company."

// The stored sentence a bulk-sender refusal carries. One pair writes it and
// reads the sender kind back, so the list names the kind in any language.
const (
	bulkSenderReasonPrefix = "mail from this domain was judged "
	bulkSenderReasonSuffix = ", so it is not a company this business works with"
)

func bulkSenderReason(senderKind string) string {
	return bulkSenderReasonPrefix + senderKind + bulkSenderReasonSuffix
}

func bulkSenderKind(reason string) (crmcontracts.BlockedDomainSenderKind, bool) {
	rest, ok := strings.CutPrefix(reason, bulkSenderReasonPrefix)
	if !ok {
		return "", false
	}
	kind := crmcontracts.BlockedDomainSenderKind(strings.TrimSuffix(rest, bulkSenderReasonSuffix))
	return kind, strings.HasSuffix(rest, bulkSenderReasonSuffix) && kind.Valid()
}

// reasonCodeOf names why a domain stands where it does, so a client words it in
// the reader's language. Only a reason somebody wrote travels as their words.
func reasonCodeOf(e BlockedDomain) (crmcontracts.BlockedDomainReasonCode, *string, *crmcontracts.BlockedDomainSenderKind) {
	if e.Admission == DomainUndecided {
		switch e.Source {
		case PendingUnevidenced, PendingStaleEvidence, PendingNearDuplicate, PendingForwardedIdentity, PendingVendorVerdict, PendingReservedDomain:
			return crmcontracts.BlockedDomainReasonCode(e.Source), nil, nil
		}
		return crmcontracts.BlockedDomainReasonCodeUnknown, nil, nil
	}
	if e.ReasonCode != "" {
		code := crmcontracts.BlockedDomainReasonCode(e.ReasonCode)
		if !code.Valid() {
			return crmcontracts.BlockedDomainReasonCodeUnknown, nil, nil
		}
		if code == crmcontracts.BlockedDomainReasonCodeStated {
			note := e.Reason
			return code, &note, nil
		}
		kind := crmcontracts.BlockedDomainSenderKind(e.SenderKind)
		if code == crmcontracts.BlockedDomainReasonCodeBulkSender && kind.Valid() {
			return code, nil, &kind
		}
		return code, nil, nil
	}
	switch e.Source {
	case AdmissionSourceVerdict:
		if kind, ok := bulkSenderKind(e.Reason); ok {
			return crmcontracts.BlockedDomainReasonCodeBulkSender, nil, &kind
		}
		return crmcontracts.BlockedDomainReasonCodeBulkSender, nil, nil
	case AdmissionSourceHuman:
		switch e.Reason {
		case claimedDomainReason:
			return crmcontracts.BlockedDomainReasonCodeCompanyCreated, nil, nil
		case keptDomainReason:
			return crmcontracts.BlockedDomainReasonCodeKeptAsCompany, nil, nil
		case "":
			return crmcontracts.BlockedDomainReasonCodeUnknown, nil, nil
		}
		note := e.Reason
		return crmcontracts.BlockedDomainReasonCodeStated, &note, nil
	}
	return crmcontracts.BlockedDomainReasonCodeUnknown, nil, nil
}

// An older writer advances admission_at without knowing the code columns.
// Its newer decision must not inherit the previous writer's explanation.
const domainReasonCodeColumns = `
 COALESCE(CASE WHEN d.admission_reason_coded_at = d.admission_at THEN d.admission_reason_code END, ''),
 COALESCE(CASE WHEN d.admission_reason_coded_at = d.admission_at THEN d.admission_sender_kind END, '')`
