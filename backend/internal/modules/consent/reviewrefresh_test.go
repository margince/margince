// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// MatchesAssessment treats a new sending entity, purpose or deal as a changed
// message. The acknowledgement of the old one must not answer the new one.
func TestANewSendingScopeChangesTheAcknowledgement(t *testing.T) {
	first, second := ids.NewV7(), ids.NewV7()
	stop := func(entity, purpose, deal *ids.UUID) Review {
		return Review{Refusals: []RefusedRecipient{{Address: "anna@example.test", ReasonCode: commsauthz.ReasonSubjectRequest, SendingEntity: entity, PurposeID: purpose, DealID: deal}}}
	}
	original := stop(&first, &first, &first)
	same := first
	if original.AssessmentRevision() != stop(&same, &same, &same).AssessmentRevision() {
		t.Fatal("the same scope read through other pointers changed the acknowledgement")
	}
	for name, changed := range map[string]Review{
		"sending entity": stop(&second, &first, &first),
		"purpose":        stop(&first, &second, &first),
		"deal":           stop(&first, &first, &second),
		"no purpose":     stop(&first, nil, &first),
	} {
		if err := changed.checkAcknowledgement(original.AssessmentRevision()); err == nil {
			t.Errorf("a changed %s accepted the old acknowledgement", name)
		}
	}
}
