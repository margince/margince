// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A write whose body names a record the caller cannot read answers 422
// reference_not_found naming the field. A record can carry several ids, and a
// bare not-found does not say which one was wrong.
//
// A missing record and a hidden one answer alike, so the status tells nobody
// which rows exist.

import (
	"maps"
	"net/http"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const absentRecordID = "00000000-0000-7000-8000-0000000000ee"

func namesFieldWithCode(problem problemBody, field, code string) bool {
	for _, refusal := range problem.Details.Errors {
		if refusal.Field == field && refusal.Code == code {
			return true
		}
	}
	return false
}

func TestAWriteNamingAMissingRecordIsA422NamingTheField(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Missing Refs", "admin@missingrefs.test", "Admin")

	contact := createAndID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Real Contact"})
	company := createAndID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Real Co"})
	pipelineID, stageID, dealID := "", "", ""
	var pipelines struct {
		Data []struct {
			ID     string `json:"id"`
			Stages []struct {
				ID       string `json:"id"`
				Semantic string `json:"semantic"`
			} `json:"stages"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/pipelines", nil, nil, &pipelines); status != http.StatusOK || len(pipelines.Data) == 0 {
		t.Fatalf("list pipelines → %d", status)
	}
	pipelineID = pipelines.Data[0].ID
	for _, s := range pipelines.Data[0].Stages {
		if s.Semantic == "open" {
			stageID = s.ID
			break
		}
	}
	deal := func(extra AnyMap) AnyMap {
		body := AnyMap{"name": "Probe", "pipeline_id": pipelineID, "stage_id": stageID, "source": "manual"}
		maps.Copy(body, extra)
		return body
	}
	dealID = createAndID(t, e, "/v1/deals", deal(nil))
	contractID := createAndID(t, e, "/v1/contracts", AnyMap{"company_id": company, "title": "MSA", "value_basis": "total"})

	cases := map[string]struct {
		method, path string
		body         AnyMap
		field, code  string
	}{
		"company create, parent": {
			"POST", "/v1/companies",
			AnyMap{"source": "manual", "display_name": "Child", "parent_company_id": absentRecordID},
			"parent_company_id", "reference_not_found",
		},
		"company edit, parent": {
			"PATCH", "/v1/companies/" + company,
			AnyMap{"parent_company_id": absentRecordID},
			"parent_company_id", "reference_not_found",
		},
		"deal create, company":               {"POST", "/v1/deals", deal(AnyMap{"company_id": absentRecordID}), "company_id", "reference_not_found"},
		"deal create, partner":               {"POST", "/v1/deals", deal(AnyMap{"partner_company_id": absentRecordID}), "partner_company_id", "reference_not_found"},
		"deal create, project":               {"POST", "/v1/deals", deal(AnyMap{"project_id": absentRecordID}), "project_id", "reference_not_found"},
		"deal create, stage":                 {"POST", "/v1/deals", deal(AnyMap{"stage_id": absentRecordID}), "stage_id", "reference_not_found"},
		"deal create, a pipeline nobody has": {"POST", "/v1/deals", deal(AnyMap{"pipeline_id": absentRecordID}), "stage_id", "stage_not_in_pipeline"},
		"deal edit, company": {
			"PATCH", "/v1/deals/" + dealID, AnyMap{"company_id": absentRecordID}, "company_id", "reference_not_found",
		},
		"deal edit, partner": {
			"PATCH", "/v1/deals/" + dealID, AnyMap{"partner_company_id": absentRecordID}, "partner_company_id", "reference_not_found",
		},
		"deal edit, project": {
			"PATCH", "/v1/deals/" + dealID, AnyMap{"project_id": absentRecordID}, "project_id", "reference_not_found",
		},
		"deal advance, stage": {
			"POST", "/v1/deals/" + dealID + "/advance", AnyMap{"to_stage_id": absentRecordID}, "to_stage_id", "reference_not_found",
		},
		"lead create, project": {
			"POST", "/v1/leads",
			AnyMap{"source": "manual", "full_name": "Lead", "email": "lead@missingrefs.test", "project_id": absentRecordID},
			"project_id", "reference_not_found",
		},
		"activity create, link": {
			"POST", "/v1/activities",
			AnyMap{"kind": "note", "body": "x", "links": []AnyMap{
				{"entity_type": "contact", "entity_id": contact},
				{"entity_type": "contact", "entity_id": absentRecordID},
			}},
			"links", "reference_not_found",
		},
		"project create, company": {
			"POST", "/v1/projects",
			AnyMap{"name": "Orphan", "company_id": absentRecordID, "source": "manual"},
			"company_id", "reference_not_found",
		},
		"contract create, company": {
			"POST", "/v1/contracts",
			AnyMap{"company_id": absentRecordID, "title": "MSA", "value_basis": "total"},
			"company_id", "reference_not_found",
		},
		"contract create, deal": {
			"POST", "/v1/contracts",
			AnyMap{"company_id": company, "deal_id": absentRecordID, "title": "MSA", "value_basis": "total"},
			"deal_id", "reference_not_found",
		},
		"contract edit, project": {
			"PATCH", "/v1/contracts/" + contractID,
			AnyMap{"project_id": absentRecordID},
			"project_id", "reference_not_found",
		},
		"relationship create, contact": {
			"POST", "/v1/relationships",
			AnyMap{"kind": "deal_stakeholder", "deal_id": dealID, "contact_id": absentRecordID, "source": "manual"},
			"contact_id", "reference_not_found",
		},
		"signal create, subject": {
			"POST", "/v1/signals",
			AnyMap{"kind": "risk", "summary": "x", "source": "manual", "entity_type": "company", "entity_id": absentRecordID},
			"entity_id", "reference_not_found",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var problem problemBody
			status := e.Call(t, c.method, c.path, c.body, map[string]string{"If-Match": "1"}, &problem)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("%s %s → %d %+v, want 422", c.method, c.path, status, problem)
			}
			if !namesFieldWithCode(problem, c.field, c.code) {
				t.Fatalf("the 422 names %+v, want %s (%s)", problem.Details.Errors, c.field, c.code)
			}
		})
	}
}

// A record that does not exist and one the caller may not read answer alike.
// Otherwise the status would enumerate rows.
func TestAMissingRecordAndAHiddenOneAnswerAlike(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	hidden := e.SeedCompany(t, "Their Company", &e.Rep3)
	e.MakeCapturePrivate(t, "company", hidden, e.Rep3)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, repPermsWithCompany())

	answer := func(company ids.UUID) httperr.Fault {
		id := ids.From[ids.CompanyKind](company)
		_, err := e.Deals.CreateDeal(rep, deals.CreateDealInput{
			Name: "Probe", PipelineID: pipeline, StageID: open, CompanyID: &id, ReferencesFromRequest: true,
		})
		fault, ok := httperr.Classify(err)
		if !ok {
			t.Fatalf("CreateDeal naming %s → %v, want a classified refusal", company, err)
		}
		return fault
	}
	missing, refused := answer(ids.NewV7()), answer(hidden)
	if missing.Status != http.StatusUnprocessableEntity {
		t.Fatalf("a missing company → %d, want 422", missing.Status)
	}
	if !sameAnswer(missing, refused) {
		t.Fatalf("a missing company answers %+v and a hidden one %+v, want the same answer", missing, refused)
	}

	parentAnswer := func(parent ids.UUID) httperr.Fault {
		_, err := e.Contacts.CreateCompany(rep, contacts.CreateCompanyInput{
			DisplayName: "Child", ParentCompanyID: ptrCompany(parent),
		})
		fault, ok := httperr.Classify(err)
		if !ok {
			t.Fatalf("CreateCompany under %s → %v, want a classified refusal", parent, err)
		}
		return fault
	}
	if missing, refused := parentAnswer(ids.NewV7()), parentAnswer(hidden); !sameAnswer(missing, refused) {
		t.Fatalf("a missing parent answers %+v and a hidden one %+v, want the same answer", missing, refused)
	}
}

// A deal whose ids a system derived (a stored suggestion, a lead) must not be
// refused naming a field the caller never sent.
func TestADerivedIDThatIsGoneIsNotNamedAsTheCallersField(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	gone := ids.From[ids.CompanyKind](ids.NewV7())

	_, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Derived", PipelineID: pipeline, StageID: open, CompanyID: &gone,
	})
	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != http.StatusNotFound {
		t.Fatalf("a derived company that is gone → %+v (%v), want the bare 404", fault, err)
	}
}

// sameAnswer compares what a caller can read of a refusal: its status, code,
// sentence and per-field breakdown.
func sameAnswer(a, b httperr.Fault) bool {
	return a.Status == b.Status && a.Code == b.Code && a.Detail == b.Detail &&
		reflect.DeepEqual(a.Fields, b.Fields)
}

func ptrCompany(id ids.UUID) *ids.CompanyID {
	company := ids.From[ids.CompanyKind](id)
	return &company
}
