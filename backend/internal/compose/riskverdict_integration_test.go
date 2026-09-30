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
	openapi_types "github.com/oapi-codegen/runtime/types"

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
	pipeline         ids.PipelineID
	open             ids.StageID
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
	env := &verdictEnv{Env: e, pipeline: pipeline, open: open}
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

// teamLead is Rep2 reading at team scope.
func (e *verdictEnv) teamLead() context.Context {
	return e.As(e.Rep2, []ids.UUID{e.Team1}, e.teamLeadPerms())
}

func (e *verdictEnv) teamLeadPerms() principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{"manager"},
		Objects: map[string]principal.ObjectGrant{
			"deal": {Read: true}, "activity": {Read: true}, "contact": {Read: true},
			"company": {Read: true}, "lead": {Read: true},
			// Every seeded role holds it; the figure reads the installation zone.
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeTeam,
	}
}

// figureAt reads GET /worklist/response's projection over its default
// fortnight with the clock at `at`.
func (e *verdictEnv) figureAt(ctx context.Context, t *testing.T, at time.Time) atRiskFigure {
	t.Helper()
	return e.figureOver(ctx, t, at, 14)
}

func (e *verdictEnv) figureOver(ctx context.Context, t *testing.T, at time.Time, days int) atRiskFigure {
	t.Helper()
	got, err := newAttentionService(e.Pool, nil, func() time.Time { return at }).ResponseMetrics(ctx, days)
	if err != nil {
		t.Fatalf("reading the response figures: %v", err)
	}
	return figureOf(got)
}

// atRiskFigure is the at-risk group of one response, with Stated false when
// the group was absent.
type atRiskFigure struct {
	Stated         bool
	Judged, Booked int
	RecordedSince  *openapi_types.Date
}

func figureOf(m crmcontracts.ResponseMetrics) atRiskFigure {
	if m.AtRiskJudged == nil || m.AtRiskBookedSameDay == nil {
		return atRiskFigure{}
	}
	return atRiskFigure{Stated: true, Judged: *m.AtRiskJudged, Booked: *m.AtRiskBookedSameDay, RecordedSince: m.AtRiskRecordedSince}
}

// The pass on three days, the task booked on the middle one: that day is a
// hit, the days either side are misses, and only material at-risk deals are
// judged at all.
func TestTheSameDayNextStepFigureCountsWhatThePassRecorded(t *testing.T) {
	e := setupVerdicts(t)
	e.passAt(t, e.taskAt.Add(-24*time.Hour))
	e.passAt(t, e.taskAt)
	e.passAt(t, e.taskAt.Add(24*time.Hour))

	for name, deal := range map[string]ids.UUID{"booked": e.booked, "other team's": e.other} {
		if got := e.verdicts(t, deal); got != 3 {
			t.Errorf("the %s material at-risk deal has %d verdicts over three days, want 3", name, got)
		}
	}
	for name, deal := range map[string]ids.UUID{"€100 at-risk": e.small, "€200 at-risk": e.mid, "not at risk": e.calm} {
		if got := e.verdicts(t, deal); got != 0 {
			t.Errorf("the %s deal was recorded %d times; only material at-risk deals are judged", name, got)
		}
	}

	later := e.taskAt.Add(48 * time.Hour)
	whole := e.figureAt(e.Admin(), t, later)
	if whole.Judged != 6 || whole.Booked != 1 {
		t.Errorf("an unbounded reader sees %d judged and %d booked, want 6 and 1 — the task counts on the day it was "+
			"created, not on the day before it and not on the day after", whole.Judged, whole.Booked)
	}
	// Two days back from `later` hold one whole day, the one after the task.
	if short := e.figureOver(e.Admin(), t, later, 2); short.Judged != 2 || short.Booked != 0 {
		t.Errorf("a two-day window counts %d judged and %d booked, want 2 and 0 — only the day after the task "+
			"lies wholly inside it", short.Judged, short.Booked)
	}
	team := e.figureAt(e.teamLead(), t, later)
	if team.Judged != whole.Judged || team.Booked != whole.Booked {
		t.Errorf("Team1's lead sees %d judged and %d booked where an unbounded reader sees %d and %d — "+
			"every seat reads every deal, so the figure is the same for both",
			team.Judged, team.Booked, whole.Judged, whole.Booked)
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

// A meeting and an archived task are not next steps: the meeting's row is
// dated by the calendar sync, and an archived task is retired.
//
// Both land on the other team's deal and the pass runs on the day they were
// created. The booked deal's own task counts only if it was created that same
// day, which the suite reads off the rows rather than assuming.
func TestOnlyALiveTaskIsANextStep(t *testing.T) {
	e := setupVerdicts(t)
	subject, ahead := "Quarterly review", time.Now().AddDate(0, 0, 3)
	link := []activities.ActivityLinkInput{{EntityType: "deal", EntityID: e.other}}
	meeting, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &ahead, Source: "manual", Links: link,
	})
	if err != nil {
		t.Fatalf("booking the meeting: %v", err)
	}
	retired, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &ahead, Source: "manual", Links: link,
	})
	if err != nil {
		t.Fatalf("booking the task to retire: %v", err)
	}
	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(retired.Id)), nil); err != nil {
		t.Fatalf("retiring the task: %v", err)
	}
	e.passAt(t, meeting.CreatedAt)

	zone, err := installationZone(e.Admin(), e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	onDay := func(at time.Time) bool {
		return at.In(zone).Format(time.DateOnly) == meeting.CreatedAt.In(zone).Format(time.DateOnly)
	}
	want := 0
	if onDay(e.taskAt) {
		want = 1
	}
	got := e.figureAt(e.Admin(), t, meeting.CreatedAt.Add(48*time.Hour))
	if got.Judged != 2 || got.Booked != want {
		t.Errorf("%d judged and %d booked, want 2 and %d — a meeting or an archived task on the other deal "+
			"must not count as its next step", got.Judged, got.Booked, want)
	}
}

// The day still running is not counted: its step can still be booked.
func TestTodayIsNotCountedUntilItEnds(t *testing.T) {
	e := setupVerdicts(t)
	e.passAt(t, e.taskAt)

	sameDay := e.figureAt(e.Admin(), t, e.taskAt.Add(time.Minute))
	if sameDay.Judged != 0 {
		t.Errorf("read on the verdict's own day, the figure counts %d judged, want 0 until the day ends",
			sameDay.Judged)
	}
}

// Nothing recorded is "no data", not 0%: the boundary is absent until the
// first pass, then names the first recorded day.
func TestTheFigureSaysWhenRecordingBegan(t *testing.T) {
	e := setupVerdicts(t)
	later := e.taskAt.Add(48 * time.Hour)

	before := e.figureAt(e.Admin(), t, later)
	if before.RecordedSince != nil {
		t.Fatalf("no verdict is on record and the figure names %s as its first day", before.RecordedSince)
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
	if after.RecordedSince == nil || after.RecordedSince.Format(time.DateOnly) != want {
		t.Fatalf("recording began on %s and the figure says %v", want, after.RecordedSince)
	}
}

// The day's first pass records the morning set; a later pass that day adds
// nothing, even for a deal that has turned material and at risk since.
func TestTheDayCountsTheSetItBeganWith(t *testing.T) {
	e := setupVerdicts(t)
	e.passAt(t, e.taskAt)
	// The €50,000 deal slips past its close date after the morning pass: it is
	// now at risk and clears the bar, and it is not the day's to count.
	e.WsExec(t, `UPDATE deal SET expected_close_date = current_date - 7 WHERE id = $1`, e.calm)
	e.passAt(t, e.taskAt.Add(time.Minute))

	if got := e.verdicts(t, e.booked); got != 1 {
		t.Errorf("two passes on one day left %d verdicts on the deal, want 1", got)
	}
	if got := e.verdicts(t, e.calm); got != 0 {
		t.Errorf("a deal that turned at risk after the day's first pass was added %d times — the day counts the set it began with", got)
	}
	if days := e.WsCount(t, `SELECT count(*) FROM deal_risk_day`); days != 1 {
		t.Errorf("%d recorded days after two passes on one, want 1", days)
	}
	audits := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_risk_day' AND action = 'create'`)
	if audits != 1 {
		t.Errorf("%d audit rows for one recorded day — the first pass writes one and a repeat none", audits)
	}

	// The next day starts with it.
	e.passAt(t, e.taskAt.Add(24*time.Hour))
	if got := e.verdicts(t, e.calm); got != 1 {
		t.Errorf("the next day's first pass judged the slipped deal %d times, want 1", got)
	}
}

// The pass takes the at-risk set, the prices and the day from ONE instant.
// A deal closing in three days is not at risk now and is at risk five days
// on; judged at an instant five days on, it is in that day's set.
func TestThePassJudgesAtTheInstantItFilesUnder(t *testing.T) {
	e := setupVerdicts(t)
	minor, currency, who, closes := int64(60_000_00), "EUR", ids.From[ids.UserKind](e.Rep1), time.Now().AddDate(0, 0, 3)
	soon, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Closing this week", PipelineID: e.pipeline, StageID: e.open, Source: "manual",
		AmountMinor: &minor, Currency: &currency, OwnerID: &who, ExpectedClose: &closes,
	})
	if err != nil {
		t.Fatal(err)
	}
	ahead := time.Now().AddDate(0, 0, 5).Truncate(time.Microsecond)
	e.passAt(t, ahead)

	zone, err := installationZone(e.Admin(), e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	day := ahead.In(zone).Format(time.DateOnly)
	got := e.WsCount(t, `SELECT count(*) FROM deal_risk_verdict v JOIN deal_risk_day rd ON rd.id = v.day_id
		WHERE v.deal_id = $1 AND rd.local_day = $2::date AND rd.judged_at = $3`, ids.UUID(soon.Id), day, ahead)
	if got != 1 {
		t.Fatalf("a pass judging at %s did not file the deal overdue by then under %s — the selection and the day "+
			"read different clocks", ahead, day)
	}
}

// Moving the installation to another zone does not move which tasks fell on
// a day already recorded: the day's bounds were fixed when it was judged.
func TestAZoneChangeDoesNotRewriteARecordedDay(t *testing.T) {
	e := setupVerdicts(t)
	e.WsExec(t, `UPDATE setting SET value = '"Pacific/Kiritimati"'::jsonb WHERE key = 'installation.timezone'`)
	e.passAt(t, e.taskAt)
	later := e.taskAt.Add(72 * time.Hour)
	before := e.figureAt(e.Admin(), t, later)

	e.WsExec(t, `UPDATE setting SET value = '"Etc/GMT+12"'::jsonb WHERE key = 'installation.timezone'`)
	after := e.figureAt(e.Admin(), t, later)
	if before.Booked != 1 || after.Booked != before.Booked || after.Judged != before.Judged {
		t.Fatalf("recorded at UTC+14: %d judged, %d booked; read after moving to UTC-12: %d judged, %d booked — "+
			"want 2 and 1 both times", before.Judged, before.Booked, after.Judged, after.Booked)
	}
}

// The verdict is the workspace's, taken over every amount. A reader whose
// masks withhold deal money is not shown a rate over deals chosen by amounts
// they may not read — the whole group is absent, not zero.
func TestAReaderWithDealMoneyMaskedGetsNoAtRiskFigure(t *testing.T) {
	e := setupVerdicts(t)
	e.passAt(t, e.taskAt)
	later := e.taskAt.Add(48 * time.Hour)

	perms := e.teamLeadPerms()
	perms.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority}}
	masked := e.figureAt(e.As(e.Rep2, []ids.UUID{e.Team1}, perms), t, later)
	if masked.Stated {
		t.Errorf("a reader with deal money masked was shown %d judged and %d booked, want the group absent",
			masked.Judged, masked.Booked)
	}
	if open := e.figureAt(e.teamLead(), t, later); !open.Stated || open.Judged != 2 {
		t.Errorf("an unmasked lead was shown %+v, want the figure over 2 judged deal-days", open)
	}
}

// Only the pass may write a verdict: a seat planting one would move a figure
// that reports on what the product judged.
func TestASeatCannotRecordAVerdict(t *testing.T) {
	e := setupVerdicts(t)
	now := time.Now()
	_, err := e.Deals.RecordRiskDay(e.Admin(), deals.RiskDay{
		LocalDay: now, Start: now.Add(-time.Hour), End: now.Add(time.Hour), JudgedAt: now,
	}, []ids.UUID{e.booked})
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
	// The earlier day moved ninety-one days back; the later one stays fresh.
	e.WsExec(t, `UPDATE deal_risk_day SET day_start = day_start - interval '91 days',
		day_end = day_end - interval '91 days', judged_at = judged_at - interval '91 days'
		WHERE local_day = (SELECT min(local_day) FROM deal_risk_day)`)

	svc := NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(integration.RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("running the retention sweep: %v", err)
	}
	// Asked of the row that should SURVIVE, because a count alone reads the
	// same when the sweep takes the fresh verdict and leaves the aged one.
	fresh := e.WsCount(t, `SELECT count(*) FROM deal_risk_verdict v JOIN deal_risk_day rd ON rd.id = v.day_id
		WHERE v.deal_id = $1 AND rd.day_start > now() - interval '3 days'`, e.booked)
	if got := e.verdicts(t, e.booked); got != 1 || fresh != 1 {
		t.Errorf("the booked deal holds %d verdicts after the sweep (%d fresh), want the fresh one alone", got, fresh)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1`, e.booked); got != 1 {
		t.Error("the sweep took the deal with its verdict — only the record of the judgement ages out")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_risk_day' AND action = 'erase'`); got != 1 {
		t.Errorf("%d erase audit rows, want one for the aged day", got)
	}

	e.WsExec(t, `DELETE FROM deal WHERE id = $1`, e.other)
	if got := e.verdicts(t, e.other); got != 0 {
		t.Errorf("%d verdicts outlived their deal", got)
	}
}

// A pass starting a moment before local midnight files its judgement under
// the day it started in, even when the clock has moved on by the time it
// writes: the pass reads the clock once.
func TestAPassCrossingMidnightKeepsItsDay(t *testing.T) {
	e := setupVerdicts(t)
	zone, err := installationZone(e.Admin(), e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	ahead := time.Now().In(zone).AddDate(0, 0, 2)
	start := time.Date(ahead.Year(), ahead.Month(), ahead.Day(), 23, 59, 59, 0, zone)
	reads := 0
	clock := func() time.Time {
		reads++
		// Every read after the first is past midnight.
		return start.Add(time.Duration(reads-1) * 2 * time.Second)
	}
	w := newRiskVerdictSweepWorker(e.Pool, clock, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := w.recordWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("the pass starting at %s: %v", start, err)
	}
	day := e.WsScalar(t, `SELECT local_day::text FROM deal_risk_day WHERE judged_at = $1`, start)
	if want := start.Format(time.DateOnly); day != want {
		t.Fatalf("a pass starting on %s filed its judgement under %s", want, day)
	}
}
