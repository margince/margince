// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestTheDecisionCounterRendersOneLinePerLabelCombination(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	writeAuthzDecisionMetrics(&out, map[consent.DecisionCount]uint64{
		{
			Verdict:  commsauthz.VerdictDeny,
			Category: commsauthz.CategoryMarketing, Mode: commsauthz.ModeEnforce,
		}: 7,
		{
			Verdict:  commsauthz.VerdictAllow,
			Category: commsauthz.CategoryReplyToInbound, Mode: commsauthz.ModeEnforce,
		}: 3,
	})
	rendered := out.String()

	for _, want := range []string{
		`margince_communication_authz_decisions_total{verdict="deny",category="marketing",mode="enforce"} 7`,
		`margince_communication_authz_decisions_total{verdict="allow",category="reply_to_inbound",mode="enforce"} 3`,
		"# TYPE margince_communication_authz_decisions_total counter",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the scrape does not carry %q:\n%s", want, rendered)
		}
	}
}

// A process that has authorized nothing must say nothing, rather than print a
// zero for every combination. "The engine allowed nothing" and "the engine has
// not run" call for opposite actions, and a rendered zero cannot tell them
// apart.
func TestTheDecisionCounterIsSilentBeforeItHasDecidedAnything(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	writeAuthzDecisionMetrics(&out, nil)
	if out.Len() != 0 {
		t.Errorf("a process that decided nothing rendered:\n%s", out.String())
	}
}

// The scrape is ordered, so a human reading it by hand sees a stable block
// rather than one the map reshuffled between requests.
func TestTheDecisionCounterRendersInAStableOrder(t *testing.T) {
	t.Parallel()

	totals := map[consent.DecisionCount]uint64{
		{
			Verdict:  commsauthz.VerdictDeny,
			Category: commsauthz.CategoryMarketing, Mode: commsauthz.ModeEnforce,
		}: 1,
		{
			Verdict:  commsauthz.VerdictAllow,
			Category: commsauthz.CategoryAccountNotice, Mode: commsauthz.ModeEnforce,
		}: 1,
		{
			Verdict:  commsauthz.VerdictReview,
			Category: commsauthz.CategoryInvoiceOrPayment, Mode: commsauthz.ModeWarn,
		}: 1,
	}
	var first strings.Builder
	writeAuthzDecisionMetrics(&first, totals)
	for range 8 {
		var again strings.Builder
		writeAuthzDecisionMetrics(&again, totals)
		if again.String() != first.String() {
			t.Fatalf("two renders of one reading disagree:\n%s\n---\n%s", first.String(), again.String())
		}
	}
}
