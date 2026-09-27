// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The recorded material at-risk verdict and the same-day next-step figure,
// against a real database.
//
// The pipeline below has four overdue deals worth €100, €200, €5,000 and
// €9,000, and one €50,000 deal closing next month. The lower median of the
// at-risk four is €200, so the €5,000 and €9,000 deals are material and at
// risk; the two small ones are at risk and not material; the big one is not
// at risk at all. The €9,000 deal belongs to a rep in the other team: every
// seat reads every deal (deal is an identity table in platform/auth), so a
// team lead counts it like everybody else.
//
// Every row is written by the product's own writers: the deals store, the
// activities store, the hourly pass and the retention sweep.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type verdictEnv struct {
	*integration.Env
	// The two material at-risk deals: the one a next step is booked on, and
	// the other team's.
	booked, other ids.UUID
	// The three the verdict must leave out.
	small, mid, calm ids.UUID
	// taskAt is when the task on the booked deal was created, read back
	// from the row the activities store wrote.
	taskAt time.Time
}

func setupVerdicts(t *testing.T) *verdictEnv {
	t.Helper()
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	realNow := time.Now()
	nextWeek, nextMonth := realNow.AddDate(0, 0, 7), realNow.AddDate(0, 0, 30)
	deal := func(name string, euros int64, owner ids.UUID, closes time.Time) ids.UUID {
		t.Helper()
		minor, currency, who := euros*100, "EUR", ids.From[ids.UserKind](owner)
		created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
			Name: name, PipelineID: pipeline, StageID: open, Source: "manual",
			AmountMinor: &minor, Currency: &currency, OwnerID: &who, ExpectedClose: &closes,
		})
		if err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
		return ids.UUID(created.Id)
	}
	env := &verdictEnv{Env: e}
	env.small = deal("Small renewal", 100, e.Rep1, nextWeek)
	env.mid = deal("Mid renewal", 200, e.Rep1, nextWeek)
	env.booked = deal("Visible expansion", 5_000, e.Rep1, nextWeek)
	env.other = deal("Other team's expansion", 9_000, e.Rep3, nextWeek)
	env.calm = deal("Healthy platform deal", 50_000, e.Rep1, nextMonth)
	// A fortnight passes: the four close dates slip into the past. The store
	// refuses to CREATE a deal with a past close date, and the at-risk scan
	// reads the real clock, so moving the date is how the suite gets there.
	e.WsExec(t, `UPDATE deal SET expected_close_date = current_date - 7
		WHERE id = ANY($1)`, []ids.UUID{env.small, env.mid, env.booked, env.other})

	subject, due := "Send the revised proposal", realNow.AddDate(0, 0, 2)
	task, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: env.booked}},
	})
	if err != nil {
		t.Fatalf("booking the next step: %v", err)
	}
	env.taskAt = task.CreatedAt
	return env
}

// passAt runs the hourly pass for this workspace with its clock at `at`.
func (e *verdictEnv) passAt(t *testing.T, at time.Time) {
	t.Helper()
	w := newRiskVerdictSweepWorker(e.Pool, func() time.Time { return at }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := w.recordWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("the verdict pass at %s: %v", at, err)
	}
}

// verdicts counts the recorded rows for one deal.
func (e *verdictEnv) verdicts(t *testing.T, deal ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM deal_risk_verdict WHERE deal_id = $1`, deal)
}

// teamLead is Rep2 reading at team scope: Team1's deals, not Team2's.
func (e *verdictEnv) teamLead() context.Context {
	return e.As(e.Rep2, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"manager"},
		Objects: map[string]principal.ObjectGrant{
			"deal": {Read: true}, "activity": {Read: true}, "contact": {Read: true},
			"company": {Read: true}, "lead": {Read: true},
			// Every seeded role holds it; the figure reads the installation zone.
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeTeam,
	})
}

// figureAt reads GET /worklist/response's projection with the clock at `at`.
func (e *verdictEnv) figureAt(ctx context.Context, t *testing.T, at time.Time) crmcontracts.ResponseMetrics {
	t.Helper()
	got, err := newAttentionService(e.Pool, nil, func() time.Time { return at }).ResponseMetrics(ctx, 14)
	if err != nil {
		t.Fatalf("reading the response figures: %v", err)
	}
	return got
}

// The pass on two days, the task booked on the second: the second day is a
// hit, the first a miss, and only material at-risk deals are judged at all.
func TestTheSameDayNextStepFigureCountsWhatThePassRecorded(t *testing.T) {
	e := setupVerdicts(t)
	dayBefore := e.taskAt.Add(-24 * time.Hour)
	e.passAt(t, dayBefore)
	e.passAt(t, e.taskAt)

	for name, deal := range map[string]ids.UUID{"booked": e.booked, "other team's": e.other} {
		if got := e.verdicts(t, deal); got != 2 {
			t.Errorf("the %s material at-risk deal has %d verdicts over two days, want 2", name, got)
		}
	}
	for name, deal := range map[string]ids.UUID{"€100 at-risk": e.small, "€200 at-risk": e.mid, "not at risk": e.calm} {
		if got := e.verdicts(t, deal); got != 0 {
			t.Errorf("the %s deal was recorded %d times; only material at-risk deals are judged", name, got)
		}
	}

	later := e.taskAt.Add(48 * time.Hour)
	whole := e.figureAt(e.Admin(), t, later)
	if whole.AtRiskJudged != 4 || whole.AtRiskBookedSameDay != 1 {
		t.Errorf("an unbounded reader sees %d judged and %d booked, want 4 and 1 — the task counts on the day it was "+
			"created and not on the day before", whole.AtRiskJudged, whole.AtRiskBookedSameDay)
	}
	team := e.figureAt(e.teamLead(), t, later)
	if team.AtRiskJudged != whole.AtRiskJudged || team.AtRiskBookedSameDay != whole.AtRiskBookedSameDay {
		t.Errorf("Team1's lead sees %d judged and %d booked where an unbounded reader sees %d and %d — "+
			"every seat reads every deal, so the figure is the same for both",
			team.AtRiskJudged, team.AtRiskBookedSameDay, whole.AtRiskJudged, whole.AtRiskBookedSameDay)
	}
}

// A lead who may not read deals is refused the figure rather than shown one
// over deals they cannot open.
func TestALeadWithoutDealReadIsRefused(t *testing.T) {
	e := setupVerdicts(t)
	e.passAt(t, e.taskAt)
	noDeals := e.As(e.Rep2, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"manager"},
		Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true}, "contact": {Read: true}, "company": {Read: true},
			"lead": {Read: true}, "installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeTeam,
	})
	_, err := newAttentionService(e.Pool, nil, func() time.Time { return e.taskAt.Add(48 * time.Hour) }).
		ResponseMetrics(noDeals, 14)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a lead without deal.read read the figure (err %v), want a permission refusal", err)
	}
}

// The day still running is not counted: its step can still be booked.
func TestTodayIsNotCountedUntilItEnds(t *testing.T) {
	e := setupVerdicts(t)
	e.passAt(t, e.taskAt)

	sameDay := e.figureAt(e.Admin(), t, e.taskAt.Add(time.Minute))
	if sameDay.AtRiskJudged != 0 {
		t.Errorf("read on the verdict's own day, the figure counts %d judged, want 0 until the day ends",
			sameDay.AtRiskJudged)
	}
}

// Nothing recorded is "no data", not 0%: the boundary is absent until the
// first pass, then names the first recorded day.
func TestTheFigureSaysWhenRecordingBegan(t *testing.T) {
	e := setupVerdicts(t)
	later := e.taskAt.Add(48 * time.Hour)

	before := e.figureAt(e.Admin(), t, later)
	if before.AtRiskRecordedSince != nil {
		t.Fatalf("no verdict is on record and the figure names %s as its first day", before.AtRiskRecordedSince)
	}

	dayBefore := e.taskAt.Add(-24 * time.Hour)
	e.passAt(t, dayBefore)
	e.passAt(t, e.taskAt)
	after := e.figureAt(e.teamLead(), t, later)
	zone, err := installationZone(e.Admin(), e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	want := dayBefore.In(zone).Format(time.DateOnly)
	if after.AtRiskRecordedSince == nil || after.AtRiskRecordedSince.Format(time.DateOnly) != want {
		t.Fatalf("recording began on %s and the figure says %v", want, after.AtRiskRecordedSince)
	}
}

// The pass runs hourly; a second pass on one day adds nothing.
func TestASecondPassOnOneDayWritesNothing(t *testing.T) {
	e := setupVerdicts(t)
	e.passAt(t, e.taskAt)
	e.passAt(t, e.taskAt.Add(time.Minute))

	if got := e.verdicts(t, e.booked); got != 1 {
		t.Fatalf("two passes on one day left %d verdicts on the deal, want 1", got)
	}
	audits := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_risk_verdict' AND action = 'create'`)
	if rows := e.WsCount(t, `SELECT count(*) FROM deal_risk_verdict`); audits != rows {
		t.Fatalf("%d verdicts and %d audit rows — every verdict is written with its audit row and a repeat with none",
			rows, audits)
	}
}

// Only the pass may write a verdict: a seat planting one would move a figure
// that reports on what the product judged.
func TestASeatCannotRecordAVerdict(t *testing.T) {
	e := setupVerdicts(t)
	_, err := e.Deals.RecordRiskVerdicts(e.Admin(), time.Now(), []ids.UUID{e.booked})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an admin seat recorded a verdict (err %v), want a permission refusal", err)
	}
}

// The verdicts age out on the seeded window, and go with their deal.
func TestTheVerdictsAgeOutAndGoWithTheirDeal(t *testing.T) {
	e := setupVerdicts(t)
	e.passAt(t, e.taskAt.Add(-24*time.Hour))
	e.passAt(t, e.taskAt)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return consent.SeedDefaultRetentionTx(context.Background(), tx)
	}); err != nil {
		t.Fatalf("planting the default retention ladder: %v", err)
	}
	// One verdict per deal written ninety-one days ago; the other stays fresh.
	e.WsExec(t, `UPDATE deal_risk_verdict SET created_at = now() - interval '91 days'
		WHERE local_day = (SELECT min(local_day) FROM deal_risk_verdict)`)

	svc := NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(integration.RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("running the retention sweep: %v", err)
	}
	if got := e.verdicts(t, e.booked); got != 1 {
		t.Errorf("the booked deal holds %d verdicts after the sweep, want the fresh one alone", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1`, e.booked); got != 1 {
		t.Error("the sweep took the deal with its verdict — only the record of the judgement ages out")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_risk_verdict' AND action = 'erase'`); got != 2 {
		t.Errorf("%d erase audit rows, want one per aged verdict (2)", got)
	}

	e.WsExec(t, `DELETE FROM deal WHERE id = $1`, e.other)
	if got := e.verdicts(t, e.other); got != 0 {
		t.Errorf("%d verdicts outlived their deal", got)
	}
}
