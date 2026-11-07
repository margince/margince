// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A lead who holds a grant and then stops mail is refused by the send check,
// as a contact is. Each stop is written by the code that writes it in
// production: the withdrawal link press and the hard-bounce report.
func TestTheSendCheckReadsStopsRecordedOnALead(t *testing.T) {
	cases := []struct {
		name    string
		stop    func(t *testing.T, e *leadConsentEnv)
		refused bool
	}{
		{name: "a lead with no stop is sent to", stop: func(*testing.T, *leadConsentEnv) {}},
		{name: "an all-marketing link press refuses", refused: true, stop: func(t *testing.T, e *leadConsentEnv) {
			pressLeadLink(t, e, WithdrawalRef{LeadID: e.lead, Address: e.leadEmail, Scope: WithdrawalScopeAllMarketing})
		}},
		{name: "a press for the sent purpose refuses", refused: true, stop: func(t *testing.T, e *leadConsentEnv) {
			pressLeadLink(t, e, WithdrawalRef{
				LeadID: e.lead, Address: e.leadEmail,
				Scope: WithdrawalScopeNamedPurpose, PurposeID: e.newsletter.UUID,
			})
		}},
		{name: "a press for another purpose leaves this one sending", stop: func(t *testing.T, e *leadConsentEnv) {
			pressLeadLink(t, e, WithdrawalRef{
				LeadID: e.lead, Address: e.leadEmail,
				Scope: WithdrawalScopeNamedPurpose, PurposeID: e.doiNews.UUID,
			})
		}},
		{name: "a stop recorded on the lead under its earlier address refuses", refused: true, stop: func(t *testing.T, e *leadConsentEnv) {
			pressLeadLink(t, e, WithdrawalRef{LeadID: e.lead, Address: "earlier-" + e.leadEmail, Scope: WithdrawalScopeAllMarketing})
		}},
		{name: "a hard bounce on the lead's address refuses", refused: true, stop: func(t *testing.T, e *leadConsentEnv) {
			if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
				return RecordHardBounceTx(e.ctx, tx, HardBounceFact{Address: e.leadEmail, DeliveryID: ids.NewV7()})
			}); err != nil {
				t.Fatalf("recording the bounce: %v", err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := setupLeadConsent(t)
			if _, err := e.store.Record(e.ctx, RecordInput{
				LeadID: e.lead, PurposeID: e.newsletter, NewState: "granted", PolicyText: &grantWording,
			}); err != nil {
				t.Fatal(err)
			}
			tc.stop(t, e)

			err := NewGate(e.store).RequireGrantedForEmails(e.ctx, []string{e.leadEmail}, "newsletter")
			switch {
			case tc.refused && !errors.Is(err, apperrors.ErrConsentNotGranted):
				t.Fatalf("send check = %v, want ErrConsentNotGranted", err)
			case !tc.refused && err != nil:
				t.Fatalf("send check = %v, want the granted lead sent to", err)
			}
		})
	}
}

func pressLeadLink(t *testing.T, e *leadConsentEnv, ref WithdrawalRef) {
	t.Helper()
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		stopped, err := e.store.StopForCredentialTx(e.ctx, tx, ref)
		if err == nil && !stopped {
			t.Error("the press recorded no stop")
		}
		return err
	}); err != nil {
		t.Fatalf("pressing the lead's link: %v", err)
	}
}
