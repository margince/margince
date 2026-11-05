// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An Undo of Margince's change puts back a probability a colleague set. The
// deal still names that colleague, never the owner who pressed Undo.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// setByHand sets the deal's probability as the harness admin, who is not the
// deal's owner.
func setByHand(t *testing.T, ctx context.Context, store *deals.Store, deal ids.UUID) {
	t.Helper()
	set := 95
	if _, err := store.UpdateDeal(ctx, ids.From[ids.DealKind](deal), deals.UpdateDealInput{WinProbability: &set}); err != nil {
		t.Fatalf("the colleague setting the probability: %v", err)
	}
}

// probabilitySetter reads the deal as reader and answers who it says set the
// probability, empty when it names nobody.
func probabilitySetter(t *testing.T, reader context.Context, store *deals.Store, deal ids.UUID) string {
	t.Helper()
	read, err := store.GetDeal(reader, ids.From[ids.DealKind](deal), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if read.WinProbabilitySetByUserId == nil || read.WinProbabilitySource == nil ||
		string(*read.WinProbabilitySource) != string(deals.SourceHuman) {
		return ""
	}
	return read.WinProbabilitySetByUserId.String()
}

func TestAnUndoneStageMoveStillNamesTheColleagueWhoSetTheProbability(t *testing.T) {
	v := setupStageMove(t)
	setByHand(t, v.Admin(), v.Deals, v.deal)
	v.run(t)
	if got := v.field(t, "win_probability_override"); got != "" {
		t.Fatalf("after Margince's move the deal's own probability = %q, want it cleared", got)
	}

	v.undo(t, deals.VerdictStageMoveCorrection)
	if got := v.field(t, "win_probability_override"); got != "95" {
		t.Fatalf("after the Undo the deal's own probability = %q, want the colleague's 95 back", got)
	}
	if got := probabilitySetter(t, v.owner(t), v.Deals, v.deal); got != v.AdminUser.String() {
		t.Fatalf("after the owner's Undo the setter = %q, want the colleague who set it %s, not the owner %s",
			got, v.AdminUser, v.Rep1)
	}
}

func TestAnUndoneCloseStillNamesTheColleagueWhoSetTheProbability(t *testing.T) {
	c := setupCloseLost(t)
	setByHand(t, c.Admin(), c.Deals, c.silent)
	if got := c.run(t); got != 1 {
		t.Fatalf("closed %d deals, want the silent one", got)
	}

	c.undo(t, c.silent)
	if got := c.WsScalar(t, `SELECT coalesce(win_probability_override::text, '') FROM deal WHERE id = $1`, c.silent); got != "95" {
		t.Fatalf("after the Undo the deal's own probability = %q, want the colleague's 95 back", got)
	}
	if got := probabilitySetter(t, c.owner(t), c.Deals, c.silent); got != c.AdminUser.String() {
		t.Fatalf("after the owner's Undo the setter = %q, want the colleague who set it %s, not the owner %s",
			got, c.AdminUser, c.Rep1)
	}
}
