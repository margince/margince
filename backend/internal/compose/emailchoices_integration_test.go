// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEmailScopePreservesAnExplicitOptOutAcrossPartialUpdates(t *testing.T) {
	e := integration.Setup(t)
	service := identity.NewService(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	none, scope := "none", "combined"
	if _, err := service.SaveMyDelivery(ctx, identity.Delivery{MorningBrief: &none, Scope: &scope, TeamID: &e.Team1}); err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err := service.SaveMyDelivery(ctx, identity.Delivery{CombineRoutine: &yes}); err != nil {
		t.Fatal(err)
	}
	got, err := service.MyDelivery(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.MorningBrief == nil || *got.MorningBrief != "none" || got.Scope == nil || *got.Scope != "combined" || got.TeamID == nil || *got.TeamID != e.Team1 {
		t.Fatalf("partial update changed existing choices: %+v", got)
	}
}

func TestARepCannotSubscribeToTeamMailWithoutTeamReviewAuthority(t *testing.T) {
	e := integration.Setup(t)
	service := identity.NewService(e.Pool)
	scope := "team"
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
	_, err := service.SaveMyDelivery(ctx, identity.Delivery{Scope: &scope, TeamID: &e.Team1})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("team selection returned %v", err)
	}
}

func TestRoutineWindowReusesPersonalWorkingDaysAndTimezone(t *testing.T) {
	e := integration.Setup(t)
	service := identity.NewService(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	_, err := service.SaveMyWorkingHours(ctx, identity.WorkingHours{StartMinute: 9 * 60, EndMinute: 17 * 60, Days: []int{7}, Timezone: "Asia/Ho_Chi_Minh"})
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		at   time.Time
		want bool
	}{
		{time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 10, 18, 5, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC), false},
	} {
		got, err := routineMailWindow(ctx, service, scenario.at, identity.Delivery{})
		if err != nil || got != scenario.want {
			t.Errorf("window at %s = %v, %v; want %v", scenario.at, got, err, scenario.want)
		}
	}
}

func TestPersonalTimeCanStartBeforeTheReportingMorning(t *testing.T) {
	relay := &countingMailer{}
	b := setupBriefJob(t).withMailer(relay)
	seedWorkFor(t, b.Env, b.Rep1)
	ctx := b.As(b.Rep1, []ids.UUID{b.Team1}, integration.AdminPerms)
	if _, err := b.worker.users.SaveMyWorkingHours(ctx, identity.WorkingHours{StartMinute: 540, EndMinute: 1020, Days: []int{1, 2, 3, 4, 5}, Timezone: "Asia/Tokyo"}); err != nil {
		t.Fatal(err)
	}
	b.now = time.Date(2026, 10, 19, 0, 30, 0, 0, time.UTC)
	if err := b.run(t); err != nil {
		t.Fatal(err)
	}
	if relay.count() != 1 {
		t.Fatalf("personal morning produced %d messages", relay.count())
	}
	if !strings.Contains(relay.bodies[0], "Globex") || !strings.Contains(relay.markup[0], "Globex") {
		t.Fatal("production alternatives lost the actual task")
	}
}

func TestPersonalWindowAcrossReportingMidnightKeepsOneAttempt(t *testing.T) {
	relay := &countingMailer{}
	b := setupBriefJob(t).withMailer(relay)
	seedWorkFor(t, b.Env, b.Rep1)
	ctx := b.As(b.Rep1, []ids.UUID{b.Team1}, integration.AdminPerms)
	if _, err := b.worker.users.SaveMyWorkingHours(ctx, identity.WorkingHours{StartMinute: 540, EndMinute: 1020, Days: []int{1, 2, 3, 4, 5}, Timezone: "America/New_York"}); err != nil {
		t.Fatal(err)
	}
	hour := "18:00"
	if _, err := b.worker.users.SaveMyDelivery(ctx, identity.Delivery{RoutineTime: &hour}); err != nil {
		t.Fatal(err)
	}
	for _, now := range []time.Time{time.Date(2026, 10, 19, 22, 30, 0, 0, time.UTC), time.Date(2026, 10, 20, 0, 30, 0, 0, time.UTC)} {
		b.now = now
		if err := b.run(t); err != nil {
			t.Fatal(err)
		}
	}
	if relay.count() != 1 {
		t.Fatalf("one personal window produced %d messages", relay.count())
	}
}

func TestCombinedWorkersClaimOneEditionAndKeepPersonalOptOut(t *testing.T) {
	e := setupWeeklyMail(t)
	yes, none := true, "none"
	if _, err := e.worker.users.SaveMyDelivery(e.repCtx, identity.Delivery{CombineRoutine: &yes, MorningBrief: &none}); err != nil {
		t.Fatal(err)
	}
	seedWorkFor(t, e.Env, e.Rep1)
	reviewID := e.seedReview(t, time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC))
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() { e.worker.mailWeekly(e.repCtx, reviewID, weeklyMailClock) })
	}
	group.Wait()
	if e.relay.count() != 1 {
		t.Fatalf("combined workers submitted %d editions", e.relay.count())
	}
	if strings.Contains(e.relay.bodies[0], "Send the revised Globex quote") {
		t.Fatal("weekly ignored the independent morning opt-out")
	}
	run, err := routineBriefEngine(e.Pool).SnapshotRun(e.repCtx, weeklyMailClock)
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := routineBriefEngine(e.Pool).ClaimMailAttempt(e.repCtx, run.ID, weeklyMailClock); err != nil || claimed {
		t.Fatalf("combined edition left daily claim reusable: %v %v", claimed, err)
	}
}

func TestLostTeamChoiceStillDeliversPersonalFacts(t *testing.T) {
	e := setupWeeklyMail(t)
	scope := "combined"
	if _, err := e.worker.users.SaveMyDelivery(e.repCtx, identity.Delivery{Scope: &scope, TeamID: &e.Team1}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Pool.Exec(e.repCtx, `UPDATE team SET archived_at=now() WHERE id=$1`, e.Team1); err != nil {
		t.Fatal(err)
	}
	reviewID := e.seedReview(t, time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC))
	e.worker.mailWeekly(e.repCtx, reviewID, weeklyMailClock)
	if e.relay.count() != 1 {
		t.Fatal("lost team access suppressed the personal edition")
	}
	if !strings.Contains(e.relay.bodies[0], "2026-06-29") {
		t.Fatal("personal reporting period disappeared")
	}
	personal := identity.RoutinePersonal
	updated, err := e.worker.users.SaveMyDelivery(e.repCtx, identity.Delivery{Scope: &personal})
	if err != nil || updated.ScopeChoice() != identity.RoutinePersonal {
		t.Fatalf("lost access prevented returning to personal delivery: %v", err)
	}
}

func TestPersonalTeamPatchDoesNotRevealAnUnknownTeam(t *testing.T) {
	e := integration.Setup(t)
	service := identity.NewService(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
	unknown := ids.NewV7()
	_, err := service.SaveMyDelivery(ctx, identity.Delivery{TeamID: &unknown})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("unknown team selection returned %v", err)
	}
}

func TestAWeeklyChoiceOnANonWorkingDayUsesTheNextWorkingDay(t *testing.T) {
	hours := identity.DefaultWorkingHours("UTC")
	if got := weeklyMailDay(hours, 6); got != 1 {
		t.Fatalf("Saturday resolves to %d", got)
	}
	e := setupWeeklyMail(t)
	saturday := 6
	if _, err := e.worker.users.SaveMyDelivery(e.repCtx, identity.Delivery{WeeklyDay: &saturday}); err != nil {
		t.Fatal(err)
	}
	reviewID := e.seedReview(t, time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC))
	e.worker.mailWeekly(e.repCtx, reviewID, weeklyMailClock)
	if e.relay.count() != 1 {
		t.Fatal("non-working day silenced the weekly review")
	}
}

func TestAWindowAtTwentyThreeSpansMidnightAndExpires(t *testing.T) {
	e := integration.Setup(t)
	users := identity.NewService(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := users.SaveMyWorkingHours(ctx, identity.WorkingHours{StartMinute: 540, EndMinute: 1020, Days: []int{1, 2, 3, 4, 5}, Timezone: "Europe/Berlin"}); err != nil {
		t.Fatal(err)
	}
	hour := "23:00"
	settings := identity.Delivery{RoutineTime: &hour}
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 19, 23, 0, 0, 0, zone)
	for _, test := range []struct {
		offset time.Duration
		want   bool
	}{{-time.Minute, false}, {0, true}, {2 * time.Hour, true}, {3 * time.Hour, false}} {
		_, allowed, err := routineWindowFor(ctx, users, start.Add(test.offset), settings)
		if err != nil || allowed != test.want {
			t.Errorf("window %+v: %v %v", test, allowed, err)
		}
	}
}

func TestCombinedRelayFailureIsRecordedOnEveryClaim(t *testing.T) {
	e := setupWeeklyMail(t)
	e.relay.fail = errors.New("relay refused")
	yes := true
	if _, err := e.worker.users.SaveMyDelivery(e.repCtx, identity.Delivery{CombineRoutine: &yes}); err != nil {
		t.Fatal(err)
	}
	reviewID := e.seedReview(t, time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC))
	e.worker.mailWeekly(e.repCtx, reviewID, weeklyMailClock)
	if e.relay.count() != 1 {
		t.Fatal("no combined relay attempt")
	}
	var briefCause, weeklyCause *string
	if err := integration.OwnerConn(t).QueryRow(e.repCtx, `SELECT mail_error FROM brief_run WHERE user_id=$1`, e.Rep1).Scan(&briefCause); err != nil {
		t.Fatal(err)
	}
	if err := integration.OwnerConn(t).QueryRow(e.repCtx, `SELECT mail_error FROM weekly_review WHERE id=$1`, reviewID).Scan(&weeklyCause); err != nil {
		t.Fatal(err)
	}
	if briefCause == nil || weeklyCause == nil {
		t.Fatalf("combined failure disappeared: %v %v", briefCause, weeklyCause)
	}
}
