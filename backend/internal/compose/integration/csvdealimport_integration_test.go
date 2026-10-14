// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// dealImportParts maps a deals file's header onto the targets of the same
// names, which is how every column of these files is spelled.
func dealImportParts(header string) []AnyMap {
	mapping := AnyMap{}
	for _, column := range strings.Split(header, ",") {
		mapping[column] = column
	}
	return []AnyMap{{"file": "deals.csv", "object": "deal", "mapping": mapping}}
}

// seededStages is the seeded pipeline's name and one stage of each semantic.
func seededStages(t *testing.T, e *apptest.AppEnv) (pipeline, open, won, lost string) {
	t.Helper()
	pipeline, open, won = seededStageNames(t, e)
	var pipelines struct {
		Data []struct {
			Stages []struct {
				Name     string `json:"name"`
				Semantic string `json:"semantic"`
			} `json:"stages"`
		} `json:"data"`
	}
	e.Call(t, http.MethodGet, "/v1/pipelines", nil, nil, &pipelines)
	for _, s := range pipelines.Data[0].Stages {
		if s.Semantic == "lost" && lost == "" {
			lost = s.Name
		}
	}
	return pipeline, open, won, lost
}

// importedDeal is one deal as the importer left it, read from its tables.
type importedDeal struct {
	ID, Status, Stage                string
	ClosedAt, CloseDate              *time.Time
	LostReason, WonReason, CompanyID *string
	History                          int
	SourceSystem                     *string
}

func dealNamed(t *testing.T, e *apptest.AppEnv, name string) importedDeal {
	t.Helper()
	var d importedDeal
	err := e.Owner.QueryRow(context.Background(), `
		SELECT d.id::text, d.status, s.name, d.closed_at, d.expected_close_date, d.lost_reason,
		       d.won_without_contract_reason, d.company_id::text, d.source_system,
		       (SELECT count(*) FROM deal_stage_history h WHERE h.deal_id = d.id)
		  FROM deal d JOIN stage s ON s.id = d.stage_id
		 WHERE d.name = $1 AND d.archived_at IS NULL`, name).
		Scan(&d.ID, &d.Status, &d.Stage, &d.ClosedAt, &d.CloseDate, &d.LostReason, &d.WonReason, &d.CompanyID, &d.SourceSystem, &d.History)
	if err != nil {
		t.Fatalf("reading deal %q: %v", name, err)
	}
	return d
}

// importDeals uploads a deals file (with any other files), stages it, commits
// it and answers the finished report.
func importDeals(t *testing.T, e *apptest.AppEnv, name string, files map[string]string, parts []AnyMap) (importRunDTO, importReportDTO) {
	t.Helper()
	run := stageBundle(t, e, uploadZip(t, e, "crm.zip", zipBytes(t, files)).SourceRef, name, parts)
	approveAndRead(t, e, run.ID)
	return run, readImportReport(t, e, run.ID)
}

func TestAnImportedWonDealClosesInThePastThroughTheAdvance(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, won, lost := seededStages(t, e)
	_, report := importDeals(t, e, "crm", map[string]string{"deals.csv": "ref,name,pipeline,stage,amount,currency,close_date,lost_reason\n" +
		fmt.Sprintf("D-1,Open one,%s,%s,,,2099-01-31,\n", pipeline, open) +
		fmt.Sprintf("D-2,Won one,%s,%s,,,2025-06-30,\n", pipeline, won) +
		fmt.Sprintf("D-3,Lost one,%s,%s,,,2024-03-01,Went with a rival\n", pipeline, lost)}, dealImportParts("ref,name,pipeline,stage,amount,currency,close_date,lost_reason"))
	if report.Disposition.Created != 3 || report.Disposition.Skipped != 0 {
		t.Fatalf("disposition = %+v, issues %+v, want three deals created", report.Disposition, report.Issues)
	}
	won1 := dealNamed(t, e, "Won one")
	if won1.Status != "won" || won1.ClosedAt == nil || won1.ClosedAt.Format(time.DateOnly) != "2025-06-30" ||
		won1.WonReason == nil || *won1.WonReason != "imported" || won1.History != 2 || won1.SourceSystem == nil {
		t.Fatalf("won deal = %+v, want won on 2025-06-30, win reason imported, birth and move in its history", won1)
	}
	lost1 := dealNamed(t, e, "Lost one")
	if lost1.Status != "lost" || lost1.LostReason == nil || *lost1.LostReason != "Went with a rival" ||
		lost1.CloseDate == nil || lost1.CloseDate.Format(time.DateOnly) != "2024-03-01" {
		t.Fatalf("lost deal = %+v, want lost with its reason and its past close date", lost1)
	}
	if open1 := dealNamed(t, e, "Open one"); open1.Status != "open" || open1.Stage != open || open1.History != 1 {
		t.Fatalf("open deal = %+v, want open on %q with one history row", open1, open)
	}
}

func TestTheLiveCreateStillRefusesAPastCloseDate(t *testing.T) {
	e := setupImportApp(t)
	var pipelines struct {
		Data []struct {
			ID     string `json:"id"`
			Stages []struct {
				ID       string `json:"id"`
				Semantic string `json:"semantic"`
			} `json:"stages"`
		} `json:"data"`
	}
	e.Call(t, http.MethodGet, "/v1/pipelines", nil, nil, &pipelines)
	body := AnyMap{"name": "Late", "pipeline_id": pipelines.Data[0].ID, "stage_id": pipelines.Data[0].Stages[0].ID, "expected_close_date": "2020-01-01"}
	if status := e.Call(t, http.MethodPost, "/v1/deals", body, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("create with a past close date → %d, want 422", status)
	}
}

func TestADealLinksToACompanyAndAContactInTheSameZip(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _, _ := seededStages(t, e)
	existing := createdID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Eve Held", "emails": []AnyMap{{"email": "eve@deal-links.example", "email_type": "work", "is_primary": true}}})
	_, report := importDeals(t, e, "crm", map[string]string{
		"companies.csv": "ref,name\nCO-1,Deal Links GmbH\n",
		"contacts.csv":  "ref,email,name\nP-1,jo@deal-links.example,Jo Tran\n",
		"deals.csv": "ref,name,pipeline,stage,company_ref,contact_ref\n" +
			fmt.Sprintf("D-1,By ref,%s,%s,CO-1,P-1\n", pipeline, open) +
			fmt.Sprintf("D-2,By email,%s,%s,CO-1,eve@deal-links.example\n", pipeline, open) +
			fmt.Sprintf("D-3,By id,%s,%s,,%s\n", pipeline, open, existing),
	}, append(bundleParts(), dealImportParts("ref,name,pipeline,stage,company_ref,contact_ref")...))
	if report.Disposition.Skipped != 0 {
		t.Fatalf("report = %+v, want nothing skipped", report)
	}
	byRef := dealNamed(t, e, "By ref")
	if byRef.CompanyID == nil {
		t.Fatalf("deal by ref = %+v, want its company linked", byRef)
	}
	rows, err := e.Owner.Query(context.Background(), `
		SELECT d.name, c.id::text, r.role FROM relationship r
		  JOIN deal d ON d.id = r.deal_id JOIN contact c ON c.id = r.contact_id
		 WHERE r.kind = 'deal_stakeholder' AND r.archived_at IS NULL ORDER BY d.name`)
	if err != nil {
		t.Fatalf("reading the stakeholders: %v", err)
	}
	defer rows.Close()
	var seats []string
	for rows.Next() {
		var deal, contact string
		var role *string
		if err := rows.Scan(&deal, &contact, &role); err != nil {
			t.Fatalf("reading a stakeholder: %v", err)
		}
		if role != nil {
			t.Errorf("stakeholder of %q has role %q, want none", deal, *role)
		}
		seats = append(seats, deal)
	}
	if strings.Join(seats, ",") != "By email,By id,By ref" {
		t.Fatalf("stakeholder edges on %v, want one each on the three deals", seats)
	}
}

func TestADealLinkToAContactTheCallerCannotSeeIsNotFound(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _, _ := seededStages(t, e)
	e.DescribeCompany(t)
	colleague := createdID(t, e, "/v1/users", AnyMap{"email": "colleague@deal-hidden.test", "display_name": "Colleague", "role": "rep"})
	hidden := createdID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Hidden Hana", "emails": []AnyMap{{"email": "hana@deal-hidden.test", "email_type": "work", "is_primary": true}}})
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, hidden, colleague); err != nil {
		t.Fatalf("making the contact private to the colleague: %v", err)
	}
	run := stageBundle(t, e, uploadZip(t, e, "d.zip", zipBytes(t, map[string]string{"deals.csv": "ref,name,pipeline,stage,contact_ref\n" +
		fmt.Sprintf("D-1,Hidden by email,%s,%s,hana@deal-hidden.test\n", pipeline, open) +
		fmt.Sprintf("D-2,Hidden by id,%s,%s,%s\n", pipeline, open, hidden) +
		fmt.Sprintf("D-3,Never existed,%s,%s,nobody@deal-hidden.test\n", pipeline, open)})).SourceRef, "d", dealImportParts("ref,name,pipeline,stage,contact_ref"))
	report := readImportReport(t, e, run.ID)
	missing := issueOf(report, "broken_link", "nobody@deal-hidden.test")
	if missing == nil || report.Disposition.Created != 0 {
		t.Fatalf("report = %+v, want every row skipped on a broken contact link", report)
	}
	for _, value := range []string{"hana@deal-hidden.test", hidden} {
		issue := issueOf(report, "broken_link", value)
		if issue == nil || issue.Reason != strings.ReplaceAll(missing.Reason, "nobody@deal-hidden.test", value) {
			t.Errorf("issue for %q = %+v, want the words for a contact that does not exist", value, issue)
		}
	}
}

func TestADealRowWithoutAStageIsSkippedWithItsReason(t *testing.T) {
	e := setupImportApp(t)
	parts := []AnyMap{{"file": "deals.csv", "object": "deal", "mapping": AnyMap{"ref": "ref", "name": "name"}}}
	_, report := importDeals(t, e, "crm", map[string]string{"deals.csv": "ref,name\nD-1,No stage\n"}, parts)
	if report.Disposition.Created != 0 || report.Disposition.Skipped != 1 {
		t.Fatalf("report = %+v, want the deal skipped, not the run failed", report)
	}
}

func TestACorrectedFileUpdatesAMovesAndRefusesWhatTheStoreWould(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, won, _ := seededStages(t, e)
	first := "ref,name,pipeline,stage,close_date\n" +
		fmt.Sprintf("D-1,Moves up,%s,%s,2099-01-01\n", pipeline, open) +
		fmt.Sprintf("D-2,Dates back,%s,%s,2099-01-01\n", pipeline, open) +
		fmt.Sprintf("D-3,Closed by a colleague,%s,%s,2099-01-01\n", pipeline, open)
	importDeals(t, e, "crm", map[string]string{"deals.csv": first}, dealImportParts("ref,name,pipeline,stage,close_date"))
	closedByAColleague := dealNamed(t, e, "Closed by a colleague")
	if _, err := e.Owner.Exec(context.Background(), `
		UPDATE deal SET status = 'won', closed_at = now(), won_without_contract_reason = 'imported',
		       stage_id = (SELECT id FROM stage WHERE name = $2 AND semantic = 'won' LIMIT 1)
		 WHERE id = $1`, closedByAColleague.ID, won); err != nil {
		t.Fatalf("closing a deal as a colleague would have: %v", err)
	}
	second := "ref,name,pipeline,stage,close_date\n" +
		fmt.Sprintf("D-1,Moves up,%s,%s,2025-01-01\n", pipeline, won) +
		fmt.Sprintf("D-2,Dates back,%s,%s,2020-01-01\n", pipeline, open) +
		fmt.Sprintf("D-3,Closed by a colleague,%s,%s,2099-01-01\n", pipeline, open)
	_, report := importDeals(t, e, "crm", map[string]string{"deals.csv": second}, dealImportParts("ref,name,pipeline,stage,close_date"))
	if report.Disposition.Updated != 1 || report.Disposition.Skipped != 2 {
		t.Fatalf("report = %+v, issues %+v, want one move and two refusals", report.Disposition, report.Issues)
	}
	if moved := dealNamed(t, e, "Moves up"); moved.Status != "won" || moved.ClosedAt == nil || moved.ClosedAt.Format(time.DateOnly) != "2025-01-01" {
		t.Fatalf("moved deal = %+v, want won on the day the file gives", moved)
	}
	if untouched := dealNamed(t, e, "Dates back"); untouched.CloseDate == nil || untouched.CloseDate.Format(time.DateOnly) != "2099-01-01" {
		t.Fatalf("refused deal = %+v, want its future close date kept", untouched)
	}
	if kept := dealNamed(t, e, "Closed by a colleague"); kept.Status != "won" || kept.Stage != won {
		t.Fatalf("deal closed by a colleague = %+v, want it left won", kept)
	}
}

func TestAnOpenDealIsNotGivenAPastCloseDateByAFileWithNoStageColumn(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _, _ := seededStages(t, e)
	importDeals(t, e, "crm", map[string]string{"deals.csv": "ref,name,pipeline,stage,close_date\n" +
		fmt.Sprintf("D-1,Stays open,%s,%s,2099-01-01\n", pipeline, open)}, dealImportParts("ref,name,pipeline,stage,close_date"))
	_, report := importDeals(t, e, "crm", map[string]string{"deals.csv": "ref,name,close_date\nD-1,Stays open,2020-01-01\n"},
		dealImportParts("ref,name,close_date"))
	if report.Disposition.Skipped != 1 || len(report.Issues) != 1 || report.Issues[0].Reason == "" {
		t.Fatalf("report = %+v, want the row skipped with the past-date reason rather than the run failed", report)
	}
	if kept := dealNamed(t, e, "Stays open"); kept.CloseDate == nil || kept.CloseDate.Format(time.DateOnly) != "2099-01-01" {
		t.Fatalf("deal = %+v, want its close date unchanged", kept)
	}
}

func TestUndoArchivesImportedDealsBeforeContacts(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _, _ := seededStages(t, e)
	run, _ := importDeals(t, e, "crm", map[string]string{
		"companies.csv": "ref,name\nCO-1,Undo Deals GmbH\n",
		"contacts.csv":  "ref,email,name\nP-1,jo@undo-deals.example,Jo\n",
		"deals.csv":     fmt.Sprintf("ref,name,pipeline,stage,contact_ref\nD-1,Undone,%s,%s,P-1\n", pipeline, open),
	}, append(bundleParts(), dealImportParts("ref,name,pipeline,stage,contact_ref")...))
	if status := importCall(t, e, run.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d", status)
	}
	order := archiveOrder(t, e, run.ID, "deal", "contact", "company")
	if len(order) != 3 || order[0] != "deal" || order[1] != "contact" {
		t.Fatalf("archive order %v, want the deal, then the contact, then the company", order)
	}
}

func TestUndoKeepsADealSomeoneEditedSinceTheImport(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _, _ := seededStages(t, e)
	run, _ := importDeals(t, e, "crm", map[string]string{"deals.csv": "ref,name,pipeline,stage\n" +
		fmt.Sprintf("D-1,Edited by a rep,%s,%s\nD-2,Left alone,%s,%s\n", pipeline, open, pipeline, open)}, dealImportParts("ref,name,pipeline,stage"))
	edited := dealNamed(t, e, "Edited by a rep")
	if status := e.Call(t, http.MethodPatch, "/v1/deals/"+edited.ID, AnyMap{"description": "Called them"}, nil, nil); status != http.StatusOK {
		t.Fatalf("editing the deal → %d", status)
	}
	var report importReportWithUndoDTO
	if status := importCall(t, e, run.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d", status)
	}
	e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report)
	if report.Undo == nil || report.Undo.ReversedCount != 1 || len(report.Undo.Kept) != 1 || report.Undo.Kept[0].ID != edited.ID {
		t.Fatalf("undo = %+v, want one deal reversed and the edited one kept", report.Undo)
	}
}

// edgeShape is a stakeholder edge without its ids and provenance text, so two
// edges written by two paths compare as the same kind of row.
type edgeShape struct {
	Kind                                                   string
	Role                                                   *string
	OnlyContactAndDeal, CapturedByHuman, AuditedAsCreation bool
}

func stakeholderShape(t *testing.T, e *apptest.AppEnv, dealName string) edgeShape {
	t.Helper()
	var s edgeShape
	err := e.Owner.QueryRow(context.Background(), `
		SELECT r.kind, r.role,
		       r.contact_id IS NOT NULL AND r.deal_id IS NOT NULL AND r.company_id IS NULL
		         AND r.counterparty_contact_id IS NULL AND r.counterparty_company_id IS NULL AND r.project_id IS NULL,
		       r.captured_by LIKE 'human:%',
		       EXISTS (SELECT 1 FROM audit_log a WHERE a.entity_type = 'relationship' AND a.entity_id = r.id AND a.action = 'create')
		  FROM relationship r JOIN deal d ON d.id = r.deal_id
		 WHERE d.name = $1 AND r.kind = 'deal_stakeholder'`, dealName).
		Scan(&s.Kind, &s.Role, &s.OnlyContactAndDeal, &s.CapturedByHuman, &s.AuditedAsCreation)
	if err != nil {
		t.Fatalf("reading the stakeholder edge of %q: %v", dealName, err)
	}
	return s
}

func TestPromotionAndImportWriteTheStakeholderEdgeThroughOneFunction(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _, _ := seededStages(t, e)
	leadID := createdID(t, e, "/v1/leads", AnyMap{"source": "manual", "full_name": "Promoted Pia", "email": "pia@one-writer.example"})
	if status := e.Call(t, http.MethodPost, "/v1/leads/"+leadID+"/promote",
		AnyMap{"trigger": "human_qualify", "deal": AnyMap{"name": "Promoted deal"}}, nil, nil); status != http.StatusOK {
		t.Fatalf("promote with a deal → %d", status)
	}
	importDeals(t, e, "crm", map[string]string{
		"contacts.csv": "ref,email,name\nP-1,jo@one-writer.example,Jo\n",
		"deals.csv":    fmt.Sprintf("ref,name,pipeline,stage,contact_ref\nD-1,Imported deal,%s,%s,P-1\n", pipeline, open),
	}, append(bundleParts()[:1], dealImportParts("ref,name,pipeline,stage,contact_ref")...))
	promoted, imported := stakeholderShape(t, e, "Promoted deal"), stakeholderShape(t, e, "Imported deal")
	if promoted != imported || !promoted.OnlyContactAndDeal || promoted.Role != nil || !promoted.AuditedAsCreation {
		t.Fatalf("promotion wrote %+v, import wrote %+v, want the same row: kind, no role, contact and deal only, audited as a creation", promoted, imported)
	}
}

func TestTheDryRunSkipsAClosedDealInACurrencyWithNoRateForTheReasonTheCommitGives(t *testing.T) {
	e := setupImportApp(t)
	pipeline, _, won, _ := seededStages(t, e)
	header := "ref,name,pipeline,stage,close_date,amount,currency"
	files := map[string]string{"deals.csv": header + "\n" +
		fmt.Sprintf("D-1,No rate,%s,%s,2025-06-30,1000,ISK\n", pipeline, won)}
	run := stageBundle(t, e, uploadZip(t, e, "crm.zip", zipBytes(t, files)).SourceRef, "crm", dealImportParts(header))
	preview := readImportReport(t, e, run.ID)
	approveAndRead(t, e, run.ID)
	committed := readImportReport(t, e, run.ID)
	if committed.Disposition.Skipped != 1 || len(committed.Issues) != 1 {
		t.Fatalf("commit = %+v, want the closed deal skipped", committed)
	}
	if preview.Disposition.Created != 0 || preview.Disposition.Skipped != 1 || len(preview.Issues) != 1 {
		t.Fatalf("dry run = %+v, want the skip the commit gives, not a created deal", preview.Disposition)
	}
	if got, want := preview.Issues[0].Reason, committed.Issues[0].Reason; got != want {
		t.Errorf("dry run reason = %q, commit reason = %q, want them equal", got, want)
	}
}
