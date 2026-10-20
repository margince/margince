// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A create whose body names a record the caller cannot read answers 422 naming
// the field. A deal carries several ids, and a bare not-found does not say
// which one was wrong.
//
// A missing record and a hidden one answer alike, so the status tells nobody
// which rows exist.

import (
	"maps"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

const absentRecordID = "00000000-0000-7000-8000-0000000000ee"

func TestACreateNamingAMissingRecordIsA422NamingTheField(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Missing Refs", "admin@missingrefs.test", "Admin")

	contact := createAndID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Real Contact"})
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
	pipeline, stage := pipelines.Data[0].ID, ""
	for _, s := range pipelines.Data[0].Stages {
		if s.Semantic == "open" {
			stage = s.ID
			break
		}
	}
	deal := func(extra AnyMap) AnyMap {
		body := AnyMap{"name": "Probe", "pipeline_id": pipeline, "stage_id": stage, "source": "manual"}
		maps.Copy(body, extra)
		return body
	}

	cases := map[string]struct {
		path  string
		body  AnyMap
		field string
	}{
		"company parent": {
			"/v1/companies",
			AnyMap{"source": "manual", "display_name": "Child", "parent_company_id": absentRecordID},
			"parent_company_id",
		},
		"deal company":  {"/v1/deals", deal(AnyMap{"company_id": absentRecordID}), "company_id"},
		"deal partner":  {"/v1/deals", deal(AnyMap{"partner_company_id": absentRecordID}), "partner_company_id"},
		"deal project":  {"/v1/deals", deal(AnyMap{"project_id": absentRecordID}), "project_id"},
		"deal stage":    {"/v1/deals", deal(AnyMap{"stage_id": absentRecordID}), "stage_id"},
		"deal pipeline": {"/v1/deals", deal(AnyMap{"pipeline_id": absentRecordID}), "stage_id"},
		"lead project": {
			"/v1/leads",
			AnyMap{"source": "manual", "full_name": "Lead", "email": "lead@missingrefs.test", "project_id": absentRecordID},
			"project_id",
		},
		"activity link": {
			"/v1/activities",
			AnyMap{
				"kind": "note", "body": "x",
				"links": []AnyMap{
					{"entity_type": "contact", "entity_id": contact},
					{"entity_type": "contact", "entity_id": absentRecordID},
				},
			},
			"links",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var problem problemBody
			status := e.Call(t, "POST", c.path, c.body, nil, &problem)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("POST %s → %d %+v, want 422", c.path, status, problem)
			}
			if !namesField(problem, c.field) {
				t.Fatalf("the 422 names %+v, want %s", problem.Details.Errors, c.field)
			}
		})
	}
}
