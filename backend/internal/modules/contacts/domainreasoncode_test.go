// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestDomainReasonCodesDoNotInferMeaningFromCurrentWording(t *testing.T) {
	for _, test := range []struct {
		name string
		row  BlockedDomain
		code crmcontracts.BlockedDomainReasonCode
		note bool
	}{
		{"human quoted old machine wording", BlockedDomain{Source: AdmissionSourceHuman, Reason: keptDomainReason, ReasonCode: "stated"}, crmcontracts.BlockedDomainReasonCodeStated, true},
		{"translated keep", BlockedDomain{Source: AdmissionSourceHuman, Reason: "Als Unternehmen behalten.", ReasonCode: "kept_as_company"}, crmcontracts.BlockedDomainReasonCodeKeptAsCompany, false},
		{"legacy keep", BlockedDomain{Source: AdmissionSourceHuman, Reason: keptDomainReason}, crmcontracts.BlockedDomainReasonCodeKeptAsCompany, false},
		{"newer code", BlockedDomain{Source: AdmissionSourceHuman, Reason: keptDomainReason, ReasonCode: "future_reason"}, crmcontracts.BlockedDomainReasonCodeUnknown, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			code, note, _ := reasonCodeOf(test.row)
			if code != test.code || (note != nil) != test.note {
				t.Fatalf("code=%s note=%v", code, note)
			}
			if note != nil && *note != test.row.Reason {
				t.Fatal("human note was rewritten")
			}
		})
	}
}

func TestBulkSenderReasonUsesTheStoredKind(t *testing.T) {
	row := BlockedDomain{Source: AdmissionSourceVerdict, Reason: "Unchanged evidence words", ReasonCode: "bulk_sender", SenderKind: "newsletter"}
	code, _, kind := reasonCodeOf(row)
	if code != crmcontracts.BlockedDomainReasonCodeBulkSender || kind == nil || string(*kind) != row.SenderKind {
		t.Fatalf("structured sender kind was lost: %s %v", code, kind)
	}
	row.SenderKind = "future_sender_kind"
	_, _, kind = reasonCodeOf(row)
	if kind != nil {
		t.Fatal("unknown sender kind was invented")
	}
}
