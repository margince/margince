// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// boundBook is six open deals at a company every seat reads, three at one
// that is capture-private to Rep3, and two at none.
type boundBook struct {
	e       *integration.Env
	service *reporting.Service
	open    ids.UUID
	hidden  ids.UUID
}

// boundReportPerms is a seat that may read and write reports and read what
// they measure. The company is what tells two such seats apart: only Rep3 may
// open the capture-private one.
var boundReportPerms = func() principal.Permissions {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{"report_definition", "report_edition", "report_schedule", "reporting_framework", "sales_target", "deal", "pipeline", "installation_settings", "forecast", "reporting_credit", "team_oversight", "company", "contact", "activity"} {
		grants[object] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	}
	return principal.Permissions{RowScope: principal.RowScopeAll, Objects: grants}
}()

func seedBoundBook(t *testing.T) *boundBook {
	t.Helper()
	e := integration.Setup(t)
	company := func(name string) ids.UUID {
		created, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: name})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return ids.UUID(created.Id)
	}
	book := &boundBook{e: e, open: company("Open Co"), hidden: company("Hidden Co")}
	pipeline, open, _ := integration.DealFixture(t, e)
	store := deals.NewStore(e.DB(), installseam.Deals())
	seed := func(at *ids.UUID, n int) {
		for i := range n {
			input := deals.CreateDealInput{Name: fmt.Sprintf("Bound deal %d", i), PipelineID: pipeline, StageID: open}
			if at != nil {
				id := ids.From[ids.CompanyKind](*at)
				input.CompanyID = &id
			}
			if _, err := store.CreateDeal(e.Admin(), input); err != nil {
				t.Fatalf("seed deal: %v", err)
			}
		}
	}
	seed(&book.open, 6)
	seed(&book.hidden, 3)
	seed(nil, 2)
	e.MakeCapturePrivate(t, "company", book.hidden, e.Rep3)
	now := clocktest.Now(t)
	book.service = newReportingService(e.Pool, func() time.Time { return now })
	return book
}

// seat is a full seat that may write reports.
func (b *boundBook) seat(user ids.UUID, teams []ids.UUID) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), b.e.WS), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + user.String(),
		UserID: user, SeatType: principal.SeatFull, TeamIDs: teams, Permissions: boundReportPerms})
}

func (b *boundBook) rep3() context.Context { return b.seat(b.e.Rep3, []ids.UUID{b.e.Team2}) }

func (b *boundBook) admin() context.Context { return b.seat(b.e.AdminUser, nil) }

// save keeps a question as its asker saves one, through the real writer.
func (b *boundBook) save(ctx context.Context, t *testing.T, q analyticsquery.Query) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(ctx, b.e.Pool, func(tx pgx.Tx) error {
		answer, err := RunAnalyticsQuery(ctx, tx, q, analyticsquery.Floor(1))
		if err != nil {
			return err
		}
		id, err = SaveReportRun(ctx, tx, q, answer, analyticsquery.Floor(1))
		return err
	}); err != nil {
		t.Fatalf("saving %+v: %v", q, err)
	}
	return id
}

func dealsAt(company ids.UUID) analyticsquery.Query {
	return analyticsquery.Query{
		Entity:   "open-deals-per-company",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
		Filters:  []analyticsquery.Filter{{Field: "company_id", Op: analyticsquery.OpEq, Value: company.String()}},
	}
}

// report places one card bound to a saved question, over a scope.
func (b *boundBook) report(ctx context.Context, t *testing.T, run ids.UUID, scope crmcontracts.ReportingScope) crmcontracts.ReportingReport {
	t.Helper()
	report, err := b.service.CreateReport(ctx, reportInput(boundCard(run), scope))
	if err != nil {
		t.Fatalf("binding the saved question: %v", err)
	}
	return report
}

func boundCard(run ids.UUID) crmcontracts.ReportingCard {
	saved := openapi_types.UUID(run)
	return crmcontracts.ReportingCard{Id: "bound", Kind: "question", Width: crmcontracts.ReportingCardWidthFull,
		Question: &crmcontracts.ReportingCardQuestion{
			Entity: "open-deals-per-company", Measures: []crmcontracts.AnalyticsMeasure{{Fn: "count"}}, SavedQuestionId: &saved,
		}}
}

func reportInput(card crmcontracts.ReportingCard, scope crmcontracts.ReportingScope) crmcontracts.ReportingReportInput {
	layout := []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{card}}}
	return crmcontracts.ReportingReportInput{Name: "Bound", Audience: "workspace", Selection: crmcontracts.ReportingSelection{
		Scope: scope, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"bookings_won"}, Layout: &layout,
	}}
}

var workspaceScope = crmcontracts.ReportingScope{Kind: "workspace"}

// ask calls the card's answer route as the reader, under a floor.
func (b *boundBook) ask(ctx context.Context, t *testing.T, report crmcontracts.ReportingReport, floor analyticsquery.Floor) (int, string) {
	t.Helper()
	h := newAnalyticsQueryHandlers(InstallationDB(b.e.Pool), floor, newAttentionNames(InstallationDB(b.e.Pool)))
	h.reportMetrics = b.service
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	h.AnswerReportingQuestionCard(recorder, request, report.Id, "bound", crmcontracts.AnswerReportingQuestionCardParams{})
	return recorder.Code, recorder.Body.String()
}

func decodeAnswer(t *testing.T, body string) crmcontracts.ReportingQuestionAnswer {
	t.Helper()
	var out crmcontracts.ReportingQuestionAnswer
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decoding %s: %v", body, err)
	}
	return out
}

// A saved question's filter survives into the report: the card counts the
// deals at the one company it names, not every deal the reader may see.
func TestABoundQuestionAsksItsFilteredPopulationForAnAuthorizedReader(t *testing.T) {
	b := seedBoundBook(t)
	report := b.report(b.rep3(), t, b.save(b.rep3(), t, dealsAt(b.open)), workspaceScope)

	code, body := b.ask(b.rep3(), t, report, analyticsquery.Floor(1))
	answer := decodeAnswer(t, body)
	if code != http.StatusOK || answer.Status != crmcontracts.ReportingQuestionAnswerStatusAnswered || answer.Answer == nil {
		t.Fatalf("the bound card answered %d %s, want an answer", code, body)
	}
	if got := fmt.Sprint(answer.Answer.Rows[0]["count"]); got != "6" {
		t.Fatalf("the bound card counts %s deals, want the 6 at the company its question filters on", got)
	}
	if answer.Binding == nil || !answer.Binding.Filtered {
		t.Errorf("the answer does not say its question is filtered: %+v", answer.Binding)
	}
	card := (*report.Selection.Layout)[0].Cards[0].Question
	if card.Binding == nil || card.SavedQuestionId != nil {
		t.Errorf("the stored card reads %+v, want a reference and no saved question id", card)
	}
}

// Every way a reader can reach a bound card's definition carries no filter
// value, and a reader who may not open the filtered company is answered
// restricted: no rows, no labels, no total, no id in an error.
func TestAReaderDeniedAFilteredRecordLearnsNothingOfIt(t *testing.T) {
	b := seedBoundBook(t)
	run := b.save(b.rep3(), t, dealsAt(b.hidden))
	report := b.report(b.rep3(), t, run, workspaceScope)
	hidden := b.hidden.String()

	code, body := b.ask(b.admin(), t, report, analyticsquery.Floor(1))
	answer := decodeAnswer(t, body)
	if code != http.StatusOK || answer.Status != crmcontracts.ReportingQuestionAnswerStatusRestricted || answer.Answer != nil {
		t.Fatalf("a reader who may not open the company was answered %d %s, want restricted with no answer", code, body)
	}
	for _, reader := range []context.Context{b.admin(), b.rep3()} {
		got, err := b.service.GetReport(reader, ids.UUID(report.Id))
		listed, listErr := b.service.ListReports(reader, nil, 50, false)
		revisions, revisionErr := b.service.ListReportRevisions(reader, ids.UUID(report.Id))
		if err != nil || listErr != nil || revisionErr != nil {
			t.Fatalf("reading the definition: %v %v %v", err, listErr, revisionErr)
		}
		for name, read := range map[string]any{"report": got, "list": listed, "revisions": revisions} {
			raw, err := json.Marshal(read)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), hidden) {
				t.Errorf("the %s read names the filtered company: %s", name, raw)
			}
		}
	}

	h := newAnalyticsQueryHandlers(InstallationDB(b.e.Pool), analyticsquery.Floor(1), newAttentionNames(InstallationDB(b.e.Pool)))
	h.reportMetrics = b.service
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`)).WithContext(b.admin())
	h.ExplainReportingQuestionCard(recorder, request, report.Id, "bound", crmcontracts.ExplainReportingQuestionCardParams{})
	if recorder.Code != http.StatusNotFound || strings.Contains(recorder.Body.String(), hidden) {
		t.Errorf("the restricted card's evidence answered %d %s, want 404 naming nothing", recorder.Code, recorder.Body.String())
	}

	_, err := b.service.CreateReport(b.admin(), reportInput(boundCard(run), workspaceScope))
	if err == nil || strings.Contains(err.Error(), hidden) {
		t.Errorf("an author who may not open the company bound its question: %v", err)
	}
}

// A question for one person inside a report for a team cannot hold, and the
// floor and its complementary withholding apply to a bound card as they do to
// any question.
func TestABoundQuestionThatCannotHoldIsRefusedWithItsReason(t *testing.T) {
	b := seedBoundBook(t)
	mine := dealsAt(b.open)
	mine.ScopeKind, mine.ScopeID = ScopeKindOwner, b.e.AdminUser.String()
	team := crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(b.e.Team2)}
	report := b.report(b.rep3(), t, b.save(b.rep3(), t, mine), team)
	code, body := b.ask(b.rep3(), t, report, analyticsquery.Floor(1))
	if code != http.StatusBadRequest || !strings.Contains(body, "different team or person") {
		t.Fatalf("a question for one person inside a team report answered %d %s, want the reason it cannot hold", code, body)
	}

	floored := b.report(b.rep3(), t, b.save(b.rep3(), t, dealsAt(b.open)), workspaceScope)
	code, body = b.ask(b.rep3(), t, floored, analyticsquery.Floor(10))
	if code != http.StatusBadRequest || !strings.Contains(body, "too few records") {
		t.Fatalf("a filter leaving out five deals under a floor of ten answered %d %s, want the privacy refusal", code, body)
	}
}

// A card carries its binding into the next draft by its reference, and a
// reference the report never held is refused rather than asked.
func TestACardKeepsItsBindingByReferenceAndRefusesAForeignOne(t *testing.T) {
	b := seedBoundBook(t)
	report := b.report(b.rep3(), t, b.save(b.rep3(), t, dealsAt(b.open)), workspaceScope)
	selection := report.Selection
	if _, err := b.service.SaveReportDraft(b.rep3(), ids.UUID(report.Id), report.Version, crmcontracts.ReportingReportDraftInput{Selection: selection}); err != nil {
		t.Fatalf("a draft keeping the card's reference was refused: %v", err)
	}
	draft, err := b.service.GetReport(b.rep3(), ids.UUID(report.Id))
	if err != nil || draft.DraftRevision == nil {
		t.Fatalf("reading the draft: %+v %v", draft, err)
	}
	var card reporting.QuestionCard
	if err := database.WithWorkspaceTx(b.rep3(), b.e.Pool, func(tx pgx.Tx) error {
		card, _, err = b.service.QuestionCardIn(b.rep3(), tx, ids.UUID(report.Id), draft.DraftRevision, "bound")
		return err
	}); err != nil || card.Bound == nil {
		t.Fatalf("the draft's card asks %+v (%v), want the kept question", card, err)
	}

	foreign := ids.NewV7().String()
	layout := []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{{Id: "bound", Kind: "question", Width: crmcontracts.ReportingCardWidthFull,
		Question: &crmcontracts.ReportingCardQuestion{Entity: "open-deals-per-company", Measures: []crmcontracts.AnalyticsMeasure{{Fn: "count"}}, Binding: &foreign}}}}}
	selection.Layout = &layout
	if _, err := b.service.SaveReportDraft(b.rep3(), ids.UUID(report.Id), draft.Version, crmcontracts.ReportingReportDraftInput{Selection: selection}); err == nil {
		t.Fatal("a reference this report never held was kept")
	}
}

// A card placed before bindings asks its shape alone, over the report's scope,
// as it did when it was published.
func TestAnUnboundCardKeepsAskingItsShape(t *testing.T) {
	b := seedBoundBook(t)
	card := crmcontracts.ReportingCard{Id: "bound", Kind: "question", Width: crmcontracts.ReportingCardWidthFull,
		Question: &crmcontracts.ReportingCardQuestion{Entity: "open-deals-per-company", Measures: []crmcontracts.AnalyticsMeasure{{Fn: "count"}}}}
	report, err := b.service.CreateReport(b.rep3(), reportInput(card, workspaceScope))
	if err != nil {
		t.Fatal(err)
	}
	_, body := b.ask(b.rep3(), t, report, analyticsquery.Floor(1))
	answer := decodeAnswer(t, body)
	if answer.Answer == nil || fmt.Sprint(answer.Answer.Rows[0]["count"]) != "11" || answer.Binding != nil {
		t.Fatalf("the unbound card answered %s, want all 11 deals Rep3 reads and no binding", body)
	}
}
