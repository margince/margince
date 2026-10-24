// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sectionsReader reads the lead and partner sections under one row scope. It
// holds the read grants those metrics need, less any named in without.
func sectionsReader(e *forecastEnv, user ids.UUID, teams []ids.UUID, scope principal.RowScope, without ...string) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{"report_definition", "installation_settings", "deal", "pipeline", "lead", "company", "partner", "commission", "activity"} {
		if !slices.Contains(without, object) {
			grants[object] = principal.ObjectGrant{Read: true}
		}
	}
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, SeatType: principal.SeatFull, TeamIDs: teams, Permissions: principal.Permissions{RowScope: scope, Objects: grants}})
}

func sectionsWriter(e *forecastEnv) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	return principal.SystemActing(ctx, "system:sections-fixture")
}

// sectionsEvaluation reads the metrics over a window that holds every write of
// the test, evaluated at the instant the service clock answers.
func sectionsEvaluation(ctx context.Context, t *testing.T, service *reporting.Service, start time.Time, scope crmcontracts.ReportingScope, metrics ...crmcontracts.ReportingMetricID) crmcontracts.ReportingEvaluation {
	t.Helper()
	interval := crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 0, 3)}
	selection := crmcontracts.ReportingSelection{Scope: scope, Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open", Metrics: metrics, Blocks: []crmcontracts.ReportingBlockKind{"breakdown", "metric_reading"}}
	result, err := service.Evaluate(ctx, selection)
	if err != nil {
		t.Fatalf("evaluating %v: %v", metrics, err)
	}
	return result
}

func sectionsReading(t *testing.T, result crmcontracts.ReportingEvaluation, id crmcontracts.ReportingMetricID) crmcontracts.ReportingMetric {
	t.Helper()
	for _, metric := range result.Metrics {
		if metric.Id == id {
			return metric
		}
	}
	t.Fatalf("%s was not read", id)
	return crmcontracts.ReportingMetric{}
}

func sectionsValue(t *testing.T, result crmcontracts.ReportingEvaluation, id crmcontracts.ReportingMetricID) float64 {
	t.Helper()
	metric := sectionsReading(t, result, id)
	if metric.Value == nil {
		t.Fatalf("%s has no value: %+v", id, metric.Coverage)
	}
	return *metric.Value
}

// sectionsBreakdown answers a metric's breakdown chart as group key to value
// and group key to label.
func sectionsBreakdown(t *testing.T, result crmcontracts.ReportingEvaluation, id crmcontracts.ReportingMetricID) (map[string]float64, map[string]string) {
	t.Helper()
	values, labels := map[string]float64{}, map[string]string{}
	for _, chart := range result.Charts {
		if chart.Kind != "breakdown" || chart.Metric != id {
			continue
		}
		for _, point := range chart.Points {
			if point.Value != nil {
				values[point.Key] = *point.Value
			}
			labels[point.Key] = point.Label
		}
		return values, labels
	}
	t.Fatalf("%s has no breakdown chart", id)
	return nil, nil
}

func sectionsEvidence(ctx context.Context, t *testing.T, service *reporting.Service, result crmcontracts.ReportingEvaluation, id crmcontracts.ReportingMetricID, contextID, group string) []crmcontracts.ReportingEvidenceRow {
	t.Helper()
	evidence, err := service.Evidence(ctx, result.Selection, id, contextID, group, nil, nil, 100, reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
	if err != nil {
		t.Fatalf("evidence of %s %s: %v", id, group, err)
	}
	return evidence.Rows
}

func seedSectionLead(writer context.Context, t *testing.T, store *contacts.Store, name string, owner ids.UUID, origin *string) ids.LeadID {
	t.Helper()
	ownerID := ids.From[ids.UserKind](owner)
	lead, _, err := store.CreateLead(writer, contacts.CreateLeadInput{FullName: &name, Status: "new", OwnerID: &ownerID, Source: "manual", Origin: origin})
	if err != nil {
		t.Fatalf("seeding lead %s: %v", name, err)
	}
	return ids.From[ids.LeadKind](ids.UUID(lead.Id))
}

func moveSectionLead(writer context.Context, t *testing.T, store *contacts.Store, lead ids.LeadID, statuses ...string) {
	t.Helper()
	for _, status := range statuses {
		if _, err := store.UpdateLead(writer, lead, contacts.UpdateLeadInput{Status: &status}); err != nil {
			t.Fatalf("moving lead to %s: %v", status, err)
		}
	}
}

// Leads are counted from the lead writers' own rows and audit images. Rep1 has
// five leads and Rep3 one. Two of Rep1's reach an outcome, and the five are
// answered after 1, 2, 4, 8 and 30 hours.
func TestReportingLeadSectionCountsThroughTheLeadWriters(t *testing.T) {
	e := setupForecast(t)
	writer := sectionsWriter(e)
	dealsStore := deals.NewStore(InstallationDB(e.Pool), DealsInstallation())
	store := contacts.NewStore(InstallationDB(e.Pool)).WithDealOpener(leadDealOpener{deals: dealsStore})
	start := time.Now().UTC().Add(-time.Hour)
	website, event := "website", "event"
	var rep1Leads []ids.LeadID
	for i, origin := range []*string{&website, &website, nil, nil, nil} {
		rep1Leads = append(rep1Leads, seedSectionLead(writer, t, store, "Rep1 lead "+string(rune('A'+i)), e.Rep1, origin))
	}
	seedSectionLead(writer, t, store, "Rep3 lead", e.Rep3, &event)
	for i, hours := range []int{1, 2, 4, 8, 30} {
		var created time.Time
		if err := e.owner.QueryRow(context.Background(), `SELECT created_at FROM lead WHERE id=$1`, rep1Leads[i]).Scan(&created); err != nil {
			t.Fatal(err)
		}
		if _, err := store.RecordLeadFirstResponse(writer, rep1Leads[i], created.Add(time.Duration(hours)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	moveSectionLead(writer, t, store, rep1Leads[0], "contacted", "engaged")
	if _, err := store.QualifyLead(writer, rep1Leads[0], contacts.PromoteLeadInput{Trigger: "human_qualify", Deal: &contacts.QualifyDealInput{}}); err != nil {
		t.Fatalf("qualifying: %v", err)
	}
	moveSectionLead(writer, t, store, rep1Leads[1], "contacted")
	if _, err := store.DisqualifyLead(writer, rep1Leads[1], contacts.DisqualifyLeadInput{}); err != nil {
		t.Fatalf("disqualifying: %v", err)
	}

	evaluated := time.Now().UTC().Add(48 * time.Hour)
	service := newReportingService(e.Pool, func() time.Time { return evaluated })
	admin := sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeAll)
	workspace := crmcontracts.ReportingScope{Kind: "workspace"}
	result := sectionsEvaluation(admin, t, service, start, workspace, "leads_created", "lead_status_moves", "leads_promoted", "lead_first_response")
	if got := sectionsValue(t, result, "leads_created"); got != 6 {
		t.Fatalf("six leads came in, read %v", got)
	}
	origins, labels := sectionsBreakdown(t, result, "leads_created")
	if origins["origin:website"] != 2 || origins["origin:"] != 3 || origins["origin:event"] != 1 || labels["origin:"] != "No origin" || labels["origin:website"] != "Website" {
		t.Fatalf("leads split by origin wrongly: %v %v", origins, labels)
	}
	moves, _ := sectionsBreakdown(t, result, "lead_status_moves")
	if sectionsValue(t, result, "lead_status_moves") != 5 || moves["status:contacted"] != 2 || moves["status:engaged"] != 1 || moves["status:promoted"] != 1 || moves["status:disqualified"] != 1 {
		t.Fatalf("status moves miscounted: %v", moves)
	}
	promoted := sectionsReading(t, result, "leads_promoted")
	if promoted.Value == nil || *promoted.Value != 1 || promoted.Denominator == nil || *promoted.Denominator != 6 {
		t.Fatalf("one of six leads was qualified into a deal: %+v", promoted)
	}
	if got := sectionsValue(t, result, "lead_first_response"); got != 4 {
		t.Fatalf("the middle of 1, 2, 4, 8 and 30 hours is 4, read %v", got)
	}
	if rows := sectionsEvidence(admin, t, service, result, "leads_created", "interval", "origin:website"); len(rows) != 2 {
		t.Fatalf("the website group's evidence is its two leads, got %d", len(rows))
	}

	// Rep3 reads under their own row scope. Owner scope narrows to their one
	// lead. Leads are workspace-readable, so own row scope narrows nothing and a
	// workspace request reads every lead, as the lead list shows them.
	rep3 := sectionsReader(e, e.Rep3, []ids.UUID{e.Team2}, principal.RowScopeOwn)
	own := sectionsEvaluation(rep3, t, service, start, crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep3)}, "leads_created", "lead_status_moves")
	if sectionsValue(t, own, "leads_created") != 1 || sectionsValue(t, own, "lead_status_moves") != 0 {
		t.Fatalf("Rep3 reads leads beyond their own: %+v", own.Metrics)
	}
	wide := sectionsEvaluation(rep3, t, service, start, workspace, "leads_created", "lead_status_moves")
	if sectionsValue(t, wide, "leads_created") != 6 || sectionsValue(t, wide, "lead_status_moves") != 5 {
		t.Fatalf("a workspace request reads other than the leads the lead list shows Rep3: %+v", wide.Metrics)
	}
	for _, row := range sectionsEvidence(rep3, t, service, own, "leads_created", "interval", "") {
		if row.OwnerId == nil || ids.UUID(*row.OwnerId) != e.Rep3 {
			t.Fatalf("Rep3's evidence names a lead they may not see: %+v", row)
		}
	}
}

// Partner deals, the commission ledger and the partner rows are each written by
// their own module. Each reader keeps to what its reader may see.
func TestReportingPartnerSectionReadsThroughTheModuleWriters(t *testing.T) {
	e := setupForecast(t)
	writer := sectionsWriter(e)
	contactsStore := contacts.NewStore(InstallationDB(e.Pool))
	dealsStore := deals.NewStore(InstallationDB(e.Pool), DealsInstallation())
	ledger := commissions.NewStore(InstallationDB(e.Pool))
	start := time.Now().UTC().Add(-time.Hour)
	partner := func(name string, owner ids.UUID, status string) ids.UUID {
		ownerID := ids.From[ids.UserKind](owner)
		company, err := contactsStore.CreateCompany(writer, contacts.CreateCompanyInput{DisplayName: name, OwnerID: &ownerID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := contactsStore.UpsertPartner(writer, contacts.UpsertPartnerInput{CompanyID: ids.From[ids.CompanyKind](ids.UUID(company.Id)), PartnerRole: "consulting", CertStatus: &status}); err != nil {
			t.Fatalf("partner %s: %v", name, err)
		}
		return ids.UUID(company.Id)
	}
	alpha, beta, gamma := partner("Alpha", e.Rep1, "certified"), partner("Beta", e.Rep1, "applied"), partner("Gamma", e.Rep3, "certified")
	subject := "Quarterly check-in"
	if _, _, err := activities.NewStore(InstallationDB(e.Pool)).LogActivity(writer, activities.LogActivityInput{Kind: "note", Subject: &subject, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: alpha}}}); err != nil {
		t.Fatalf("logging an activity with Alpha: %v", err)
	}
	deal := func(name string, owner, company ids.UUID, attribution string, amount int64) crmcontracts.Deal {
		ownerID, partnerID, currency := ids.From[ids.UserKind](owner), ids.From[ids.CompanyKind](company), "EUR"
		created, err := dealsStore.CreateDeal(writer, deals.CreateDealInput{Name: name, AmountMinor: &amount, Currency: &currency, PipelineID: ids.From[ids.PipelineKind](e.pipeline), StageID: ids.From[ids.StageKind](e.stages[20]), OwnerID: &ownerID, Source: "manual", PartnerCompanyID: &partnerID, PartnerAttribution: &attribution})
		if err != nil {
			t.Fatalf("deal %s: %v", name, err)
		}
		return created
	}
	sourced := deal("Alpha sourced", e.Rep1, alpha, "sourced", 20000)
	influenced := deal("Alpha influenced", e.Rep1, alpha, "influenced", 5000)
	gammaDeal := deal("Gamma sourced", e.Rep3, gamma, "sourced", 7000)
	deal("Alpha seen from Rep3", e.Rep3, alpha, "influenced", 1000)
	won := e.seedID(t, `INSERT INTO stage(id,pipeline_id,name,position,semantic,win_probability) VALUES($1,$2,'Won',10,'won',100)`, e.pipeline)
	reason := "purchase_order"
	if _, err := dealsStore.AdvanceDeal(writer, ids.From[ids.DealKind](ids.UUID(influenced.Id)), deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](won), WonWithoutContractReason: &reason}); err != nil {
		t.Fatalf("winning: %v", err)
	}
	for _, accrual := range []struct {
		deal    crmcontracts.Deal
		partner ids.UUID
		basis   int64
	}{{sourced, alpha, 20000}, {gammaDeal, gamma, 7000}} {
		if _, err := ledger.Accrue(writer, commissions.AccrueInput{DealID: ids.From[ids.DealKind](ids.UUID(accrual.deal.Id)), PartnerCompanyID: ids.From[ids.CompanyKind](accrual.partner), Attribution: commissions.AttributionSourced, RateBps: 1000, BasisMinor: accrual.basis, Currency: "EUR"}); err != nil {
			t.Fatalf("accruing: %v", err)
		}
	}

	evaluated := time.Now().UTC().Add(time.Hour)
	service := newReportingService(e.Pool, func() time.Time { return evaluated })
	admin := sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeAll)
	result := sectionsEvaluation(admin, t, service, start, crmcontracts.ReportingScope{Kind: "workspace"}, "partner_pipeline", "partner_won", "partner_commission", "partner_activity")
	pipeline, labels := sectionsBreakdown(t, result, "partner_pipeline")
	alphaKey, gammaKey := "partner:"+alpha.String(), "partner:"+gamma.String()
	if sectionsValue(t, result, "partner_pipeline") != 33000 || pipeline[alphaKey+":sourced"] != 20000 || pipeline[alphaKey+":influenced"] != 6000 || pipeline[gammaKey+":sourced"] != 7000 || labels[alphaKey+":sourced"] != "Alpha · Sourced" {
		t.Fatalf("partner pipeline split wrongly: %v %v", pipeline, labels)
	}
	wonSplit, _ := sectionsBreakdown(t, result, "partner_won")
	if sectionsValue(t, result, "partner_won") != 5000 || wonSplit[alphaKey+":influenced"] != 5000 {
		t.Fatalf("partner won split wrongly: %v", wonSplit)
	}
	commission, _ := sectionsBreakdown(t, result, "partner_commission")
	if sectionsValue(t, result, "partner_commission") != 2700 || commission[alphaKey] != 2000 || commission[gammaKey] != 700 {
		t.Fatalf("commission split wrongly: %v", commission)
	}
	activity := sectionsReading(t, result, "partner_activity")
	if activity.Value == nil || *activity.Value != 1 || activity.Denominator == nil || *activity.Denominator != 3 {
		t.Fatalf("one of three partners is certified with a recent activity: %+v", activity)
	}
	if rows := sectionsEvidence(admin, t, service, result, "partner_pipeline", "interval", alphaKey+":sourced"); len(rows) != 1 || rows[0].SourceId == nil || ids.UUID(*rows[0].SourceId) != ids.UUID(sourced.Id) {
		t.Fatalf("the sourced group's evidence is its one deal: %+v", rows)
	}
	inactive := sectionsEvidence(admin, t, service, result, "partner_activity", "state", partnerInactiveGroup)
	if len(inactive) != 2 || (ids.UUID(*inactive[0].SourceId) != beta && ids.UUID(*inactive[1].SourceId) != beta) {
		t.Fatalf("Beta, applied and not certified, is inactive: %+v", inactive)
	}

	// Owner scope narrows Rep3 to their own deals, entries and partners. Companies
	// are workspace-readable, so Alpha's name shows on Rep3's own deal with it.
	rep3 := sectionsReader(e, e.Rep3, []ids.UUID{e.Team2}, principal.RowScopeOwn)
	rep3Scope := crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep3)}
	own := sectionsEvaluation(rep3, t, service, start, rep3Scope, "partner_pipeline", "partner_commission", "partner_activity")
	ownPipeline, ownLabels := sectionsBreakdown(t, own, "partner_pipeline")
	if sectionsValue(t, own, "partner_pipeline") != 8000 || ownPipeline[gammaKey+":sourced"] != 7000 || ownPipeline[alphaKey+":influenced"] != 1000 || ownLabels[alphaKey+":influenced"] != "Alpha · Influenced" {
		t.Fatalf("Rep3's partner pipeline reads beyond their own: %v %v", ownPipeline, ownLabels)
	}
	if sectionsValue(t, own, "partner_commission") != 700 {
		t.Fatalf("Rep3 reads commission on deals they may not see")
	}
	ownActivity := sectionsReading(t, own, "partner_activity")
	if ownActivity.Denominator == nil || *ownActivity.Denominator != 1 {
		t.Fatalf("Rep3 counts partners they may not see: %+v", ownActivity)
	}

	// A reader who may not read companies still counts the deals, under one
	// group that names no partner.
	companyless := sectionsReader(e, e.Rep3, []ids.UUID{e.Team2}, principal.RowScopeOwn, "company")
	nameless := sectionsEvaluation(companyless, t, service, start, rep3Scope, "partner_pipeline", "partner_commission", "partner_activity")
	hidden, hiddenLabels := sectionsBreakdown(t, nameless, "partner_pipeline")
	if hidden["partner:hidden:sourced"] != 7000 || hidden["partner:hidden:influenced"] != 1000 || hiddenLabels["partner:hidden:sourced"] != "Partner you cannot see · Sourced" {
		t.Fatalf("a reader without company access sees partner names: %v %v", hidden, hiddenLabels)
	}
	hiddenCommission, _ := sectionsBreakdown(t, nameless, "partner_commission")
	if hiddenCommission["partner:hidden"] != 700 || len(hiddenCommission) != 1 {
		t.Fatalf("a reader without company access sees which partner earned commission: %v", hiddenCommission)
	}
	withheld := sectionsReading(t, nameless, "partner_activity")
	if withheld.Value != nil || !withheld.Coverage.Withheld || withheld.Coverage.Status != "unavailable" {
		t.Fatalf("partner activity names partner companies to a reader who may not see them: %+v", withheld)
	}
	if rows := sectionsEvidence(companyless, t, service, nameless, "partner_activity", "state", ""); len(rows) != 0 {
		t.Fatalf("partner activity evidence names partner companies: %+v", rows)
	}
}
