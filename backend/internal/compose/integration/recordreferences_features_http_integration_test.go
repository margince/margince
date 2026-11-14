// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestRecordReferencesKeepFeatureIsolationThroughHTTPAndReadView(t *testing.T) {
	var disabled []string
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp"), compose.WithDisabledCoreFeatures(disabled)}
	})
	e.BootstrapWorkspace(t)
	records := createFeatureReferences(t, e)
	for _, feature := range []string{"", "contacts", "companies", "leads", "deals", "projects", ""} {
		phase := feature
		if phase == "" {
			phase = "enabled"
		}
		t.Run(phase, func(t *testing.T) {
			disabled = nil
			if feature != "" {
				disabled = []string{feature}
			}
			e.Close()
			e = e.Reboot(t)
			agent := apptest.NewMCPClient(e, apptest.MCPBearerToken(t, e, "reference feature reader", "read"))
			for _, record := range records {
				t.Run(record.kind, func(t *testing.T) {
					assertFeatureReferenceDoors(t, e, agent, record, record.feature == feature)
				})
			}
		})
	}
}

func TestRecordReferencesRequireEachKindsReadGrantThroughHTTPAndReadView(t *testing.T) {
	for _, denied := range []string{"contact", "company", "lead", "deal", "project"} {
		t.Run(denied, func(t *testing.T) {
			e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
				return []compose.Option{compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp")}
			})
			e.BootstrapWorkspace(t)
			records := createFeatureReferences(t, e)
			var role roleWire
			mustCall(t, e, http.MethodPost, "/v1/roles", AnyMap{"copy_from": "admin", "name": "Reference reader"}, http.StatusCreated, &role)
			t.Cleanup(func() {
				if _, err := e.Owner.Exec(context.Background(), `DELETE FROM field_mask WHERE role_key=@key`, pgx.StrictNamedArgs{"key": role.Key}); err != nil {
					t.Error(err)
				}
			})
			assignSessionRole(t, e, role.Key)
			agent := apptest.NewMCPClient(e, apptest.MCPBearerToken(t, e, "reference grant reader", "read"))
			for _, record := range records {
				assertFeatureReferenceDoors(t, e, agent, record, false)
			}
			path := "/v1/roles/" + role.Key + "/objects/" + denied
			mustCall(t, e, http.MethodPatch, path, AnyMap{"create": false, "read": false, "update": false, "delete": false}, http.StatusOK, nil)
			for _, record := range records {
				assertFeatureReferenceDoors(t, e, agent, record, record.kind == denied)
			}
		})
	}
}

type featureReference struct {
	kind, feature, id, name string
}

func createFeatureReferences(t *testing.T, e *apptest.AppEnv) []featureReference {
	t.Helper()
	company := createdRecord(t, e, "/v1/companies", AnyMap{"display_name": "Reference company", "source": "manual"})
	contact := createdRecord(t, e, "/v1/contacts", AnyMap{"full_name": "Reference contact", "source": "manual"})
	lead := createdRecord(t, e, "/v1/leads", AnyMap{"full_name": "Reference lead", "source": "manual"})
	deal := openDeal(t, e, apptest.DiscoverSeededPipeline(t, e), "Reference deal")
	project := createdRecord(t, e, "/v1/projects", AnyMap{"name": "Reference project", "company_id": company, "source": "manual"})
	return []featureReference{
		{"contact", "contacts", contact, "Reference contact"},
		{"company", "companies", company, "Reference company"},
		{"lead", "leads", lead, "Reference lead"},
		{"deal", "deals", deal, "Reference deal"},
		{"project", "projects", project, "Reference project"},
	}
}

func assertFeatureReferenceDoors(t *testing.T, e *apptest.AppEnv, agent *apptest.MCPClient, record featureReference, denied bool) {
	t.Helper()
	query := url.Values{"kind": {record.kind}, "id": {record.id}}
	path := "/v1/records/references?" + query.Encode()
	args := AnyMap{"view": "read_record_references", "kind": record.kind, "id": record.id}
	if denied {
		var problem struct {
			Code string `json:"code"`
		}
		mustCall(t, e, http.MethodGet, path, nil, http.StatusForbidden, &problem)
		if problem.Code != "permission_denied" {
			t.Fatalf("%s refusal code = %q", record.kind, problem.Code)
		}
		refusal := agent.CallRefused(t, "read_view", args)
		if !strings.Contains(refusal, "permission_denied") {
			t.Fatalf("%s read_view must reach the selected-kind grant: %s", record.kind, refusal)
		}
		return
	}
	var rest, tool crmcontracts.RecordReferenceListResponse
	mustCall(t, e, http.MethodGet, path, nil, http.StatusOK, &rest)
	agent.CallOK(t, "read_view", args).JSON(t, &tool)
	for door, response := range map[string]crmcontracts.RecordReferenceListResponse{"HTTP": rest, "read_view": tool} {
		if len(response.Data) != 1 {
			t.Fatalf("%s %s returned %d references, want one", door, record.kind, len(response.Data))
		}
		got := response.Data[0]
		if got.Id.String() != record.id || got.Name == nil || *got.Name != record.name || got.Archived {
			t.Fatalf("%s %s returned the wrong reference: %+v", door, record.kind, got)
		}
	}
}
