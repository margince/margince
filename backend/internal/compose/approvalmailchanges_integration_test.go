// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The single email for a staged field change names the deal and the proposal.
// It shows each field's value before and after, as the decider's card does.
func TestAFieldChangeEmailSaysWhatWouldChange(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Harbor renewal", pipeline, open, &e.AdminUser)
	amount, currency := int64(250000), "EUR"
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{AmountMinor: &amount, Currency: &currency}); err != nil {
		t.Fatal(err)
	}
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":"deal","id":%q,"fields":{"amount_minor":300000}}`, deal)))
	if err != nil {
		t.Fatal(err)
	}
	id, err := NewApprovalsService(e.DB()).Stage(e.Admin(), approvals.StageInput{
		Kind: "update_record", TargetType: "deal", TargetID: deal, ProposedChange: raw, DiffHash: hash,
		Summary: `Update deal "Harbor renewal": overwrite Value, edited by hand`,
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt := notices.EmailAttempt{
		ApprovalID: &id, Notice: notices.Notice{Kind: notices.KindApprovalPending}, Recipient: ids.From[ids.UserKind](e.AdminUser),
	}
	for use, rich := range map[approvalNoticeUse]bool{forDigestLine: false, forSingleEmail: true} {
		_, facts, send, err := refreshApprovalNotice(e.Admin(), e.Pool, attempt, clocktest.Now(t), use)
		if err != nil || !send {
			t.Fatalf("refreshing the notice for use %v: send=%v, %v", use, send, err)
		}
		if facts.Rich != rich {
			t.Fatalf("use %v read the change facts: %v, want %v", use, facts.Rich, rich)
		}
		if !rich {
			continue
		}
		text := factsText(facts)
		for _, want := range []string{"Harbor renewal\n", "Proposal: Update deal", "Value: 2,500.00 EUR → 3,000.00 EUR"} {
			if !strings.Contains(text, want) {
				t.Errorf("the email facts do not say %q:\n%s", want, text)
			}
		}
	}
}
