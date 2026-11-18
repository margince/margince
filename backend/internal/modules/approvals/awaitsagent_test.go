// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlyAnApprovedUnredeemedAgentStagingAwaitsTheAgent(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	decided := now.Add(-time.Minute)
	lapsed := now.Add(-redemptionTTL - time.Minute)
	passport := ids.New[ids.PassportKind]()
	agent := func(over func(*row)) row {
		a := row{Kind: "enrich", Status: approvalStatusApproved, PassportID: &passport, DecidedAt: &decided}
		if over != nil {
			over(&a)
		}
		return a
	}
	cases := []struct {
		name string
		a    row
		want bool
	}{
		{"agent staging, approved", agent(nil), true},
		{"already redeemed", agent(func(a *row) { a.ConsumedAt = &now }), false},
		{"the effect already ran", agent(func(a *row) { a.EffectRanAt = &now }), false},
		{"the effect failed", agent(func(a *row) { a.EffectFailedAt = &now }), false},
		{"redemption window lapsed", agent(func(a *row) { a.DecidedAt = &lapsed }), false},
		{"still pending", agent(func(a *row) { a.Status = statusPending }), false},
		{"expired", agent(func(a *row) { a.Status = "expired" }), false},
		{"a step-up runs on decision", agent(func(a *row) { a.Kind = KindVolumeRelease; a.EffectRanAt = &now }), false},
		{"a step-up whose effect failed", agent(func(a *row) { a.Kind = KindVolumeRelease; a.EffectFailedAt = &now }), false},
		{"a human agreement an agent filed", agent(func(a *row) { a.Kind = approvalsubject.KindOwnershipTransfer }), false},
		{"server proposed", agent(func(a *row) { a.PassportID = nil }), false},
	}
	for _, tc := range cases {
		if got := awaitsAgent(tc.a, now); got != tc.want {
			t.Errorf("%s: awaitsAgent = %v, want %v", tc.name, got, tc.want)
		}
	}
}
