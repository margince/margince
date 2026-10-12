// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The stored reasons are English sentences, so the machine-written ones are
// spelled here beside the read that turns them back into codes.

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// claimedDomainReason is the reason a contact's company claim records, read
// back as reason_code company_created.
const claimedDomainReason = "a contact put this domain on a company they created"

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
		if code := crmcontracts.BlockedDomainReasonCode(e.Source); code.Valid() {
			return code, nil, nil
		}
	}
	switch {
	case e.Source == AdmissionSourceVerdict:
		if kind, ok := bulkSenderKind(e.Reason); ok {
			return crmcontracts.BlockedDomainReasonCodeBulkSender, nil, &kind
		}
		return crmcontracts.BlockedDomainReasonCodeBulkSender, nil, nil
	case e.Source == AdmissionSourceHuman && e.Reason == claimedDomainReason:
		return crmcontracts.BlockedDomainReasonCodeCompanyCreated, nil, nil
	}
	note := e.Reason
	return crmcontracts.BlockedDomainReasonCodeStated, &note, nil
}
