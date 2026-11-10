// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestCoreFeatureSwitchesBlockBothDoorsAndKeepDataForReenable(t *testing.T) {
	var disabled []string
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp"), compose.WithListsEnabled(true), compose.WithDisabledCoreFeatures(disabled)}
	})
	apptest.BootstrapWorkspaceSession(t, e, "Feature switches", "switches@fable.test", "Admin")
	company := createdRecord(t, e, "/v1/companies", AnyMap{"display_name": "Kept company", "source": "manual"})
	contact := createdRecord(t, e, "/v1/contacts", AnyMap{"full_name": "Kept contact", "source": "manual"})
	var before crmcontracts.MeResponse
	mustCall(t, e, http.MethodGet, "/v1/me", nil, http.StatusOK, &before)
	for _, phase := range []string{"enabled", "only deals disabled", "only contacts disabled", "only analytics disabled", "disabled", "enabled again"} {
		t.Run(phase, func(t *testing.T) {
			if phase == "only analytics disabled" {
				disabled = []string{"analytics"}
			} else if phase == "disabled" {
				disabled = []string{"home", "contacts", "companies", "leads", "deals", "projects", "filters", "analytics"}
			} else if phase == "only contacts disabled" {
				disabled = []string{"contacts"}
			} else if phase == "only deals disabled" {
				disabled = []string{"deals"}
			} else {
				disabled = nil
			}
			off := slices.Contains(disabled, "contacts")
			if phase != "enabled" {
				e.Close()
				e = e.Reboot(t)
			}
			want := http.StatusOK
			if off {
				want = http.StatusNotFound
			}
			assertCoreFeatureDoors(t, e, disabled)
			if phase == "only contacts disabled" {
				var page crmcontracts.Company360
				mustCall(t, e, http.MethodGet, "/v1/companies/"+company+"/360", nil, http.StatusOK, &page)
				if page.Contacts != nil || !slices.Contains(page.SectionsOmitted, crmcontracts.Company360SectionsOmitted("contacts")) {
					t.Fatalf("company page exposed disabled contacts: %+v", page)
				}
			}
			if phase == "only deals disabled" {
				var account crmcontracts.Company
				mustCall(t, e, http.MethodGet, "/v1/companies/"+company, nil, http.StatusOK, &account)
				if account.ComputedFields != nil || account.OpenDealCount != nil {
					t.Fatal("company disclosed a disabled deal aggregate")
				}
				var page crmcontracts.Contact360
				mustCall(t, e, http.MethodGet, "/v1/contacts/"+contact+"/360", nil, http.StatusOK, &page)
				if page.Commercial != nil || !slices.Contains(page.SectionsOmitted, crmcontracts.Contact360SectionsOmitted("commercial")) {
					t.Fatalf("contact page exposed disabled deals: %+v", page)
				}
			}

			mustCall(t, e, http.MethodGet, "/v1/contacts/"+contact, nil, want, nil)
			var me crmcontracts.MeResponse
			mustCall(t, e, http.MethodGet, "/v1/me", nil, http.StatusOK, &me)
			if me.SettingsAvailability == nil || me.SettingsAvailability.DisabledFeatures == nil || len(*me.SettingsAvailability.DisabledFeatures) != len(disabled) {
				t.Fatalf("feature availability=%+v, want %v", me.SettingsAvailability, disabled)
			}
			if me.Authorization == nil || before.Authorization == nil ||
				!reflect.DeepEqual(before.Authorization.Objects, me.Authorization.Objects) ||
				before.Authorization.RowScope != me.Authorization.RowScope || before.Authorization.SeatType != me.Authorization.SeatType {
				t.Fatal("feature switches changed the caller's stored authority")
			}
			agent := apptest.NewMCPClient(e, apptest.MCPBearerToken(t, e, "feature reader", "read"))
			commission := AnyMap{"record_type": "commission", "limit": 10}
			if slices.Contains(disabled, "analytics") || slices.Contains(disabled, "deals") {
				agent.CallRefused(t, "list_records", commission)
				refusal := agent.CallRefused(t, "read_record", AnyMap{"record_type": "commission", "id": contact})
				if !strings.Contains(refusal, "unknown tool") {
					t.Fatalf("disabled commission reached its provider: %s", refusal)
				}
			} else {
				agent.CallOK(t, "list_records", commission)
			}
			args := AnyMap{"record_type": "contact", "id": contact}
			if off {
				agent.CallRefused(t, "read_record", args)
			} else {
				agent.CallOK(t, "read_record", args)
			}
			report := AnyMap{"report": "deals-by-stage"}
			if slices.Contains(disabled, "deals") || slices.Contains(disabled, "analytics") {
				agent.CallRefused(t, "run_report", report)
			} else {
				agent.CallOK(t, "run_report", report)
			}
			var name string
			if err := e.Owner.QueryRow(t.Context(), `SELECT full_name FROM contact WHERE id=@id`, pgx.StrictNamedArgs{"id": contact}).Scan(&name); err != nil || name != "Kept contact" {
				t.Fatalf("retained contact=%q, error=%v", name, err)
			}
		})
	}
}

func assertCoreFeatureDoors(t *testing.T, e *apptest.AppEnv, disabled []string) {
	t.Helper()
	for _, call := range []struct{ method, path, feature string }{
		{http.MethodGet, "/v1/worklist", "home"},
		{http.MethodGet, "/v1/contacts", "contacts"},
		{http.MethodGet, "/v1/companies", "companies"},
		{http.MethodGet, "/v1/leads", "leads"},
		{http.MethodGet, "/v1/deals", "deals"},
		{http.MethodGet, "/v1/projects", "projects"},
		{http.MethodGet, "/v1/views", "filters"},
		{http.MethodGet, "/v1/lists", "filters"},
		{http.MethodPost, "/v1/reports/deals-by-stage", "analytics"},
	} {
		want := http.StatusOK
		if slices.Contains(disabled, call.feature) || (call.path == "/v1/reports/deals-by-stage" && slices.Contains(disabled, "deals")) {
			want = http.StatusNotFound
		}
		if status := e.Call(t, call.method, call.path, AnyMap{}, nil, nil); status != want {
			t.Errorf("%s %s=%d, want %d", call.method, call.path, status, want)
		}
	}
}
