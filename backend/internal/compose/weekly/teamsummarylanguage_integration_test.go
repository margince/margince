// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"context"
	"maps"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTeamSummaryKeepsItsWriteLanguageOnReplayAndRead(t *testing.T) {
	for _, tc := range []struct{ language, prefix, pattern string }{{"en", "1 of 1 members counted; 0 deals won and 0 lost.", "No priority indicated by the recorded metrics"}, {"de", "Erfasste Teammitglieder: 1 von 1; 0 Deals gewonnen und 0 verloren.", "Die erfassten Kennzahlen zeigen keine Priorität an"}, {"vi", "Đã tính 1 trên 1 thành viên; thắng 0 giao dịch và thua 0.", "Các chỉ số đã ghi nhận không cho thấy ưu tiên nào"}} {
		t.Run(tc.language, func(t *testing.T) {
			e := setupWeekly(t)
			e.engine.WithForecast(&stubForecast{coverage: []Coverage{{PeriodKind: "month", Basis: "manager_call", BaseCurrency: "EUR", CoverageBp: 6400, ReplanRecommended: true}}})
			store := settings.New(e.Pool, settings.NewRegistry(identity.BaseLanguage))
			if err := settings.Set(e.Admin(), store, identity.BaseLanguage, tc.language); err != nil {
				t.Fatal(err)
			}
			members := []TeamMember{{UserID: e.Rep1, DisplayName: "Original Name"}}
			empty, _, err := e.engine.AssembleTeamFor(e.repCtx, e.Team1, "Original Team", members, weekClock)
			if err != nil {
				t.Fatal(err)
			}
			if empty.Counts.RepsCounted != 0 || e.WsCount(t, `SELECT count(*) FROM team_weekly_review WHERE summary_language IS NOT NULL`) != 0 {
				t.Fatal("unmeasured snapshot stamped a language an older writer would inherit")
			}
			assertSummaryLanguageAudit(t, e, empty.ID, "create", "")
			if _, _, err := e.engine.AssembleFor(e.repCtx, weekClock); err != nil {
				t.Fatal(err)
			}
			first, wrote, err := e.engine.AssembleTeamFor(e.repCtx, e.Team1, "Original Team", members, weekClock)
			if err != nil {
				t.Fatal(err)
			}
			if !wrote || first.ID != empty.ID || first.Language != tc.language || !strings.HasPrefix(first.Summary, tc.prefix) {
				t.Fatalf("first: %+v, wrote %t", first, wrote)
			}
			assertSummaryLanguageAudit(t, e, first.ID, "update", tc.language)
			if len(first.Reps) != 1 || first.Reps[0].FocusLabel != tc.pattern {
				t.Fatalf("stored pattern: %+v", first.Reps)
			}
			nextLanguage := "vi"
			if tc.language == "vi" {
				nextLanguage = "de"
			}
			if err := settings.Set(e.Admin(), store, identity.BaseLanguage, nextLanguage); err != nil {
				t.Fatal(err)
			}
			replay, wrote, err := e.engine.AssembleTeamFor(e.repCtx, e.Team1, "Renamed", members, weekClock)
			if err != nil {
				t.Fatal(err)
			}
			if wrote || replay.ID != first.ID || replay.Summary != first.Summary || replay.Language != first.Language || replay.Counts != first.Counts || replay.Reps[0].FocusLabel != first.Reps[0].FocusLabel {
				t.Fatalf("replay rewrote shared prose: %+v", replay)
			}
			latest, err := e.engine.LatestTeamReview(e.repCtx, e.Team1, nil)
			if err != nil {
				t.Fatal(err)
			}
			if latest.Summary != first.Summary || latest.Language != first.Language {
				t.Fatalf("read changed shared prose: %+v", latest)
			}
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			delete(perms.Objects, "forecast")
			limited, err := e.engine.LatestTeamReview(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), e.Team1, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(first.Summary, "64%") || strings.Contains(limited.Summary, "64%") || limited.Summary != tc.prefix || limited.Language != tc.language || len(limited.Coverage) != 1 || limited.Coverage[0].Absent != CoverageNotAuthorized {
				t.Fatalf("permission-filtered read lost frozen language or disclosed coverage: %+v", limited)
			}
		})
	}
}

func assertSummaryLanguageAudit(t *testing.T, e *weekEnv, id ids.UUID, action, language string) {
	t.Helper()
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'team_weekly_review' AND entity_id = $1 AND action = $2
		AND after ? 'summary_language' AND coalesce(after->>'summary_language', '') = $3`, id, action, language); got != 1 {
		t.Fatalf("%s audit did not preserve language %q: %d rows", action, language, got)
	}
}

func TestLegacyTeamSummaryWithoutLanguageReadsInEnglish(t *testing.T) {
	e := setupWeekly(t)
	e.engine.WithForecast(&stubForecast{coverage: []Coverage{{PeriodKind: "month", Basis: "manager_call", BaseCurrency: "EUR", CoverageBp: 6400, ReplanRecommended: true}}})
	if _, _, err := e.engine.AssembleFor(e.repCtx, weekClock); err != nil {
		t.Fatal(err)
	}
	first, wrote, err := e.engine.AssembleTeamFor(e.repCtx, e.Team1, "Original Team", []TeamMember{{UserID: e.Rep1, DisplayName: "Original Name"}}, weekClock)
	if err != nil || !wrote {
		t.Fatalf("write: %t, %v", wrote, err)
	}
	assertSummaryLanguageAudit(t, e, first.ID, "create", "en")
	// Model a row written before language metadata existed, through the real writer first.
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `UPDATE team_weekly_review SET summary_language = NULL WHERE id = $1`, first.ID); err != nil {
		t.Fatal(err)
	}
	store := settings.New(e.Pool, settings.NewRegistry(identity.BaseLanguage))
	if err := settings.Set(e.Admin(), store, identity.BaseLanguage, "de"); err != nil {
		t.Fatal(err)
	}
	latest, err := e.engine.LatestTeamReview(e.repCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Language != "en" || latest.Summary != first.Summary {
		t.Fatalf("legacy read: %+v", latest)
	}
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	delete(perms.Objects, "forecast")
	limited, err := e.engine.LatestTeamReview(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if limited.Language != "en" || limited.Summary != "1 of 1 members counted; 0 deals won and 0 lost." || strings.Contains(limited.Summary, "64%") || len(limited.Coverage) != 1 || limited.Coverage[0].Absent != CoverageNotAuthorized {
		t.Fatalf("legacy filtered read: %+v", limited)
	}
}
