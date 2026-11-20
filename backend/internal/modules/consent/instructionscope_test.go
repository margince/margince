// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestAnInstructionKeepsItsRecipientCompanyAndPurpose(t *testing.T) {
	t.Parallel()
	purpose := ids.NewV7()
	original := commsauthz.Decision{
		Recipient:   connector.Recipient{Email: "recipient@example.test"},
		SubjectKind: entityContact, SubjectID: ids.NewV7(), SendingEntity: ids.NewV7(),
		Requested: commsauthz.CategoryRequestedFollowup, Resolved: commsauthz.CategoryRequestedFollowup,
		Verdict: commsauthz.VerdictReview, PurposeID: &purpose,
		Evidence: commsauthz.Evidence{PolicyRevision: "first-stop"},
	}
	recorded := []RefusedRecipient{refusedRecipientOf(original)}
	for _, tc := range []struct {
		name   string
		change func(*commsauthz.Decision)
		stale  bool
	}{
		{"new recipient preference", func(d *commsauthz.Decision) { d.Evidence.PolicyRevision = "second-stop" }, true},
		{"same scope", func(*commsauthz.Decision) {}, false},
		{"address", func(d *commsauthz.Decision) { d.Recipient.Email = "other@example.test" }, true},
		{"record", func(d *commsauthz.Decision) { d.SubjectID = ids.NewV7() }, true},
		{"record kind", func(d *commsauthz.Decision) { d.SubjectKind = entityLead }, true},
		{"company", func(d *commsauthz.Decision) { d.SendingEntity = ids.NewV7() }, true},
		{"purpose", func(d *commsauthz.Decision) { id := ids.NewV7(); d.PurposeID = &id }, true},
		{"reason for contact", func(d *commsauthz.Decision) { d.Requested = commsauthz.CategoryPrecontractQuote }, true},
		{"deal", func(d *commsauthz.Decision) { d.Evidence.DealID = ids.NewV7() }, true},
		{"now supported", func(d *commsauthz.Decision) { d.Verdict = commsauthz.VerdictAllow }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := original
			tc.change(&d)
			err := instructionScopeStillFits(t.Context(), nil, recorded, commsauthz.DecisionSet{Decisions: []commsauthz.Decision{d}})
			if tc.stale {
				if _, ok := errors.AsType[*InstructionStaleError](err); !ok {
					t.Fatalf("changed scope error=%v, want a fresh acknowledgement", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// A pack that changes only the law it cites changes what the human was shown.
// The review refreshes and an instruction acknowledging the old law is stale.
// Where nothing is cited on either side, the answer is as it always was.
func TestAChangedCitationNeedsAFreshAcknowledgement(t *testing.T) {
	t.Parallel()
	old := commsauthz.Provision{Instrument: "UWG", Provision: "§ 7(2)", SourceURL: "https://example.org/uwg"}
	amended := commsauthz.Provision{Instrument: "UWG", Provision: "§ 7(3)", SourceURL: "https://example.org/uwg"}
	refused := func(refusal, footer commsauthz.Provision) commsauthz.Decision {
		d := commsauthz.Decision{
			Recipient:   connector.Recipient{Email: "anna@example.test"},
			SubjectKind: entityContact, SubjectID: ids.UUID{1}, Verdict: commsauthz.VerdictDeny,
			ReasonCode: commsauthz.ReasonObjection, Mode: commsauthz.ModeEnforce,
			Citations: map[string][]commsauthz.Provision{
				commsauthz.ReasonObjection: {refusal}, "objection_route": {footer},
			},
		}
		return d.WithFooterFindings([]commsauthz.FooterFinding{{Kind: "objection_route"}})
	}
	for _, tc := range []struct {
		name       string
		shown, now commsauthz.Decision
		stale      bool
	}{
		{"the refusal's law changed", refused(old, old), refused(amended, old), true},
		{"the footer finding's law changed", refused(old, old), refused(old, amended), true},
		{"a law is cited where none was", refused(commsauthz.Provision{}, commsauthz.Provision{}), refused(old, old), true},
		{"the same law", refused(old, old), refused(old, old), false},
		{"nothing cited either time", refused(commsauthz.Provision{}, commsauthz.Provision{}), refused(commsauthz.Provision{}, commsauthz.Provision{}), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorded := []RefusedRecipient{refusedRecipientOf(tc.shown)}
			current := commsauthz.DecisionSet{Decisions: []commsauthz.Decision{tc.now}}
			if matches := (Review{Refusals: recorded}).MatchesAssessment(current); matches == tc.stale {
				t.Errorf("the review matches the current send = %v, want %v", matches, !tc.stale)
			}
			if shown := everyRefusalWasShown(recorded, current); shown == tc.stale {
				t.Errorf("every refusal was shown = %v, want %v", shown, !tc.stale)
			}
			_, stale := errors.AsType[*InstructionStaleError](instructionScopeStillFits(t.Context(), nil, recorded, current))
			if stale != tc.stale {
				t.Errorf("the instruction's scope is stale = %v, want %v", stale, tc.stale)
			}
		})
	}
}
