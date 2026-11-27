// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The cross-record questions X01 to X03 of the canonical corpus. Each is a
// filter on a deal report, not a second record kind in the query. The corpus
// file is the one spelling of each question. The root compose census leaves the
// X ids to the census here. So a question cannot be added without a proof, or
// proved under a wording the corpus no longer carries.

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// quietEnv is a workspace with one open deal per case, each worth a distinct
// power of two. A sum of amounts then names which deals an answer counted.
type quietEnv struct {
	*Env
	pipeline ids.PipelineID
	open     ids.StageID
	next     int64
}

func setupQuiet(t *testing.T) *quietEnv {
	t.Helper()
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	return &quietEnv{Env: e, pipeline: pipeline, open: open, next: 1}
}

// deal seeds an open deal owned by the owner, at a company or none. It returns
// the deal's id and the amount that identifies it in an answer.
func (q *quietEnv) deal(t *testing.T, name string, owner ids.UUID, company *ids.UUID) (ids.UUID, int64) {
	t.Helper()
	id := q.SeedDeal(t, name, q.pipeline, q.open, &owner)
	amount := q.next
	q.next *= 2
	q.WsExec(t, `UPDATE deal SET amount_minor = $2, currency = 'EUR', company_id = $3 WHERE id = $1`, id, amount, company)
	return id, amount
}

// touch writes one note some days ago on a deal or a company. The activity
// triggers move the stored clock, as they do for a captured activity.
func (q *quietEnv) touch(t *testing.T, column string, record ids.UUID, daysAgo int) {
	t.Helper()
	activity := ids.NewV7()
	q.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'note', 'Touch', now() - make_interval(days => $2), 'manual', 'human:x')`, activity, daysAgo)
	entity := strings.TrimSuffix(column, "_id")
	q.WsExec(t, `INSERT INTO activity_link (id, activity_id, entity_type, `+column+`) VALUES ($1, $2, $3, $4)`,
		ids.NewV7(), activity, entity, record)
}

// meetingAhead books a meeting that has not happened yet on a deal.
func (q *quietEnv) meetingAhead(t *testing.T, deal ids.UUID) {
	t.Helper()
	activity := ids.NewV7()
	q.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, source, captured_by)
		VALUES ($1, 'meeting', 'Review', now() + interval '2 days', 'booked', 'manual', 'human:x')`, activity)
	q.WsExec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id) VALUES ($1, $2, 'deal', $3)`,
		ids.NewV7(), activity, deal)
}

// openAmount is the sum of the deals' amounts that a question counts, which
// names the deals because every amount is a distinct power of two. Without
// filters naming another population it reads deals-by-stage, in each deal's own
// currency; pipeline-current reads the open pipeline in the base currency.
func (q *quietEnv) openAmount(ctx context.Context, t *testing.T, filters ...analyticsquery.Filter) (int64, error) {
	t.Helper()
	return q.sum(ctx, t, analyticsquery.Query{
		Entity:   "deals-by-stage",
		GroupBy:  []string{"currency"},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.Sum, Field: "amount_minor", As: "amount"}},
		Filters:  append([]analyticsquery.Filter{{Field: "status", Op: analyticsquery.OpEq, Value: "open"}}, filters...),
	})
}

// pipelineAmount is the open pipeline in the base currency under the filters.
func (q *quietEnv) pipelineAmount(ctx context.Context, t *testing.T, filters ...analyticsquery.Filter) (int64, error) {
	t.Helper()
	return q.sum(ctx, t, analyticsquery.Query{
		Entity:   "pipeline-current",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.Sum, Field: "amount_base_minor", As: "amount"}},
		Filters:  filters,
	})
}

func (q *quietEnv) sum(ctx context.Context, t *testing.T, query analyticsquery.Query) (int64, error) {
	t.Helper()
	var answer compose.AnalyticsAnswer
	err := database.WithWorkspaceTx(ctx, q.Pool, func(tx pgx.Tx) error {
		var err error
		answer, err = compose.RunAnalyticsQuery(ctx, tx, query, analyticsquery.Floor(1))
		return err
	})
	if err != nil {
		return 0, err
	}
	if len(answer.Rows) == 0 {
		return 0, nil
	}
	if len(answer.Rows) != 1 {
		t.Fatalf("one currency answered %d rows: %v", len(answer.Rows), answer.Rows)
	}
	number, ok := answer.Rows[0]["amount"].(pgtype.Numeric)
	if !ok {
		t.Fatalf("the amount is %T, want a number: %v", answer.Rows[0]["amount"], answer.Rows[0])
	}
	amount, err := number.Int64Value()
	if err != nil {
		t.Fatalf("the amount %v is not a whole number: %v", number, err)
	}
	return amount.Int64, nil
}

func (q *quietEnv) mustOpenAmount(ctx context.Context, t *testing.T, filters ...analyticsquery.Filter) int64 {
	t.Helper()
	got, err := q.openAmount(ctx, t, filters...)
	if err != nil {
		t.Fatalf("the question was refused: %v", err)
	}
	return got
}

func quietSeat(e *Env, objects ...string) context.Context {
	perms := AccountRepPerms
	perms.RowScope = principal.RowScopeOwn
	perms.Objects = maps.Clone(AccountRepPerms.Objects)
	for _, object := range objects {
		delete(perms.Objects, object)
	}
	return e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
}

//craft:ignore naked-any a filter value is whatever JSON type the question sends
func filterOn(field string, op analyticsquery.FilterOp, value any) analyticsquery.Filter {
	return analyticsquery.Filter{Field: field, Op: op, Value: value}
}

// X01: Which open deals have had no activity in the last 30 days? A deal with
// no activity at all is quiet. So is a deal whose only entry is a meeting still
// to come, because the clock counts what happened.
func proveX01OpenDealsWithNoActivityInThirtyDays(t *testing.T) {
	q := setupQuiet(t)
	quiet, quietAmount := q.deal(t, "Quiet", q.Rep1, nil)
	q.touch(t, "deal_id", quiet, 45)
	active, _ := q.deal(t, "Active", q.Rep1, nil)
	q.touch(t, "deal_id", active, 5)
	_, neverAmount := q.deal(t, "Never touched", q.Rep1, nil)
	ahead, aheadAmount := q.deal(t, "Meeting only ahead", q.Rep1, nil)
	q.meetingAhead(t, ahead)
	if got := q.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND last_activity_at IS NULL`, ahead); got != 1 {
		t.Fatalf("a future meeting moved the deal's clock, so this case proves nothing about it")
	}

	got := q.mustOpenAmount(q.Admin(), t, filterOn("days_since_activity", analyticsquery.OpGte, 30))
	if want := quietAmount + neverAmount + aheadAmount; got != want {
		t.Errorf("deals quiet 30 days sum to %d, want %d: the deal with activity 5 days ago must stay out, "+
			"and the one never touched and the one with only a booked meeting must be in", got, want)
	}
	if got := q.mustOpenAmount(q.Admin(), t, filterOn("days_since_activity", analyticsquery.OpLt, 30)); got != 2 {
		t.Errorf("deals touched within 30 days sum to %d, want only the active deal's 2: a deal never touched "+
			"is not within any number of days", got)
	}
}

// X02: Which open deals have no meeting booked? A booked meeting counts and a
// past meeting does not.
func proveX02OpenDealsWithNoMeetingBooked(t *testing.T) {
	q := setupQuiet(t)
	booked, _ := q.deal(t, "Booked", q.Rep1, nil)
	q.meetingAhead(t, booked)
	past, pastAmount := q.deal(t, "Only a past call", q.Rep1, nil)
	q.touch(t, "deal_id", past, 3)
	_, plainAmount := q.deal(t, "Nothing", q.Rep1, nil)

	got := q.mustOpenAmount(q.Admin(), t, filterOn("meeting_ahead", analyticsquery.OpEq, false))
	if want := pastAmount + plainAmount; got != want {
		t.Errorf("deals with no meeting ahead sum to %d, want %d: the booked deal must be out", got, want)
	}
	if got := q.mustOpenAmount(q.Admin(), t, filterOn("meeting_ahead", analyticsquery.OpEq, true)); got != 1 {
		t.Errorf("deals with a meeting ahead sum to %d, want the booked deal's 1", got)
	}
}

// X03: How much open pipeline sits with companies nobody has been in touch with
// for 60 days? A company never touched counts, a deal with no company does not,
// and a company the asker may not open leaves the answer.
func proveX03PipelineAtCompaniesQuietSixtyDays(t *testing.T) {
	q := setupQuiet(t)
	quietCo := q.SeedCompany(t, "Quiet Co", &q.Rep1)
	q.touch(t, "company_id", quietCo, 90)
	activeCo := q.SeedCompany(t, "Active Co", &q.Rep1)
	q.touch(t, "company_id", activeCo, 4)
	neverCo := q.SeedCompany(t, "Never Co", &q.Rep1)
	privateCo := q.SeedCompany(t, "Private Co", &q.Rep3)
	q.touch(t, "company_id", privateCo, 120)
	q.MakeCapturePrivate(t, "company", privateCo, q.Rep3)

	_, quietAmount := q.deal(t, "At quiet", q.Rep1, &quietCo)
	_, activeAmount := q.deal(t, "At active", q.Rep1, &activeCo)
	_, neverAmount := q.deal(t, "At never touched", q.Rep1, &neverCo)
	_, noCompanyAmount := q.deal(t, "No company", q.Rep1, nil)
	_, privateAmount := q.deal(t, "At private", q.Rep1, &privateCo)
	ask := filterOn("company_days_since_activity", analyticsquery.OpGte, 60)

	own, err := q.pipelineAmount(quietSeat(q.Env), t, ask)
	if err != nil {
		t.Fatalf("a rep's own question was refused: %v", err)
	}
	if want := quietAmount + neverAmount; own != want {
		t.Errorf("pipeline at quiet companies sums to %d, want %d: the deal at a company the rep may not "+
			"open must not be counted", own, want)
	}
	all, err := q.pipelineAmount(quietSeat(q.Env), t)
	if err != nil {
		t.Fatalf("the unfiltered pipeline was refused: %v", err)
	}
	if want := own + activeAmount + noCompanyAmount + privateAmount; all != want {
		t.Errorf("the rep's whole pipeline sums to %d, want %d: the private company's deal is the rep's own "+
			"and only the company filter may drop it", all, want)
	}
	_, err = q.pipelineAmount(quietSeat(q.Env, "company"), t, ask)
	var refusal *analyticsquery.RefusalError
	if !errors.As(err, &refusal) || refusal.Kind != analyticsquery.RefusalUnsupported {
		t.Errorf("a seat with no company grant asked the company filter and got %v, want it refused as no such field", err)
	}
}

// The meeting filter reads activities, so it is not offered to a seat that may
// not read them.
func TestTheMeetingFilterIsNotOfferedWithoutTheActivityGrant(t *testing.T) {
	q := setupQuiet(t)
	_, err := q.openAmount(quietSeat(q.Env, "activity"), t, filterOn("meeting_ahead", analyticsquery.OpEq, true))
	var refusal *analyticsquery.RefusalError
	if !errors.As(err, &refusal) || refusal.Kind != analyticsquery.RefusalUnsupported {
		t.Errorf("a seat with no activity grant asked meeting_ahead and got %v, want it refused as no such field", err)
	}
	if _, err := q.openAmount(quietSeat(q.Env, "activity"), t, filterOn("days_since_activity", analyticsquery.OpGte, 30)); err != nil {
		t.Errorf("days_since_activity reads the deal's own column and was refused: %v", err)
	}
}

// quietProofs maps each X question to its proof.
var quietProofs = map[string]func(*testing.T){
	"X01": proveX01OpenDealsWithNoActivityInThirtyDays,
	"X02": proveX02OpenDealsWithNoMeetingBooked,
	"X03": proveX03PipelineAtCompaniesQuietSixtyDays,
}

func TestQuietDealQuestionsAnswerThroughDerivedFilters(t *testing.T) {
	var doc struct {
		Questions []struct {
			ID   string `json:"id"`
			Tool string `json:"tool"`
		} `json:"questions"`
	}
	raw, err := os.ReadFile("../testdata/analytics_mcp_questions.json")
	if err != nil {
		t.Fatalf("reading the canonical corpus: %v", err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the corpus is not the shape this suite reads: %v", err)
	}
	var inCorpus []string
	for _, question := range doc.Questions {
		if !strings.HasPrefix(question.ID, "X") {
			continue
		}
		inCorpus = append(inCorpus, question.ID)
		if question.Tool != "run_analytics_query" {
			t.Errorf("%s names tool %q, but its proof drives the typed query", question.ID, question.Tool)
		}
	}
	slices.Sort(inCorpus)
	if want := slices.Sorted(maps.Keys(quietProofs)); !slices.Equal(inCorpus, want) {
		t.Fatalf("the corpus carries %v and the proofs cover %v: a question without proof is prose", inCorpus, want)
	}
	for _, id := range inCorpus {
		t.Run(id, quietProofs[id])
	}
}
