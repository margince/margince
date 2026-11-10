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
	}
	recorded := []RefusedRecipient{refusedRecipientOf(original)}
	for _, tc := range []struct {
		name   string
		change func(*commsauthz.Decision)
		stale  bool
	}{
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
			var stale *InstructionStaleError
			if tc.stale {
				if !errors.As(err, &stale) {
					t.Fatalf("changed scope error=%v, want a fresh acknowledgement", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
