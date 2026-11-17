// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestFooterAcknowledgementRevisionChangesWithEveryDisplayedDetail(t *testing.T) {
	original := Review{Refusals: []RefusedRecipient{{Address: "anna@example.test", ReasonCode: "no_compatible_evidence", FooterFindings: []commsauthz.FooterFinding{{Kind: "objection_route"}}, FooterPreview: []string{"Example Ltd"}}}}
	baseline := original.AssessmentRevision()
	for _, current := range []RefusedRecipient{
		{Address: "other@example.test", ReasonCode: "no_compatible_evidence", FooterFindings: []commsauthz.FooterFinding{{Kind: "objection_route"}}, FooterPreview: []string{"Example Ltd"}},
		{Address: "anna@example.test", ReasonCode: "no_compatible_evidence", FooterFindings: []commsauthz.FooterFinding{{Kind: "objection_route", Language: "vi"}}, FooterPreview: []string{"Example Ltd"}},
		{Address: "anna@example.test", ReasonCode: "no_compatible_evidence", FooterFindings: []commsauthz.FooterFinding{{Kind: "objection_route"}}, FooterPreview: []string{"Other Ltd"}},
	} {
		if (Review{Refusals: []RefusedRecipient{current}}).AssessmentRevision() == baseline {
			t.Fatalf("changed disclosure reused the acknowledgement: %+v", current)
		}
	}
}

func TestANewRecipientPreferenceChangesTheAcknowledgement(t *testing.T) {
	original := Review{Refusals: []RefusedRecipient{{Address: "anna@example.test", ReasonCode: commsauthz.ReasonSubjectRequest, PolicyRevision: "first-stop"}}}
	changed := Review{Refusals: []RefusedRecipient{{Address: "anna@example.test", ReasonCode: commsauthz.ReasonSubjectRequest, PolicyRevision: "second-stop"}}}
	if original.AssessmentRevision() == changed.AssessmentRevision() {
		t.Fatal("new preference reused the acknowledgement")
	}
	if err := changed.checkAcknowledgement(original.AssessmentRevision()); err == nil {
		t.Fatal("old acknowledgement accepted")
	}
	if err := changed.checkAcknowledgement(changed.AssessmentRevision()); err != nil {
		t.Fatal(err)
	}
}
