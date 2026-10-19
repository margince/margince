// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/settings"
)

func TestWeeklyMetricCoverageUsesAndKeepsTheWriteLanguage(t *testing.T) {
	for _, tc := range []struct{ language, prefix string }{{"en", "No "}, {"de", "Im betrachteten Bereich"}, {"vi", "Chưa ghi nhận"}} {
		t.Run(tc.language, func(t *testing.T) {
			e := integration.Setup(t)
			store := settings.New(e.Pool, settings.NewRegistry(identity.BaseLanguage))
			if err := settings.Set(e.Admin(), store, identity.BaseLanguage, tc.language); err != nil {
				t.Fatal(err)
			}
			engine := newWeeklyEngine(e.Pool)
			own := assembleMemberWeek(t, e, engine, e.Rep1)
			assembleMemberWeek(t, e, engine, e.Rep2)
			team := assembleTeamCoverage(t, e, engine)
			for _, reason := range []*string{own.Deals.Reason, team.Deals.Reason, own.Commitments.Reason, team.Commitments.Reason} {
				if reason == nil || !strings.HasPrefix(*reason, tc.prefix) {
					t.Fatalf("%s coverage: %v", tc.language, reason)
				}
			}
			next := "de"
			if tc.language == next {
				next = "vi"
			}
			if err := settings.Set(e.Admin(), store, identity.BaseLanguage, next); err != nil {
				t.Fatal(err)
			}
			replay := assembleMemberWeek(t, e, engine, e.Rep1)
			replayTeam := assembleTeamCoverage(t, e, engine)
			if replay.Commitments.Reason == nil || *replay.Commitments.Reason != *own.Commitments.Reason || replayTeam.Commitments.Reason == nil || *replayTeam.Commitments.Reason != *team.Commitments.Reason {
				t.Fatal("replay rewrote frozen commitment coverage")
			}
			if replay.Deals.Reason == nil || *replay.Deals.Reason != *own.Deals.Reason || replayTeam.Deals.Reason == nil || *replayTeam.Deals.Reason != *team.Deals.Reason {
				t.Fatal("replay rewrote frozen metric explanations")
			}
		})
	}
}
