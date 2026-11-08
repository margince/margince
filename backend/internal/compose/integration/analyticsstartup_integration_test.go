// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAnalyticsStartupMatchesTheAuthorizedIndividualReads(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var plain, prepared crmcontracts.MeResponse
	for path, out := range map[string]*crmcontracts.MeResponse{"/v1/me": &plain, "/v1/me?prepare=analytics": &prepared} {
		if status := e.Call(t, http.MethodGet, path, nil, nil, out); status != http.StatusOK {
			t.Fatalf("%s = %d", path, status)
		}
	}
	if plain.AnalyticsStartup != nil {
		t.Fatal("ordinary session prepared unrequested Analytics data")
	}
	startup := prepared.AnalyticsStartup
	if startup == nil {
		t.Fatal("requested Analytics startup data is absent")
	}
	var context crmcontracts.AnalyticsContext
	var catalog crmcontracts.ReportingCatalog
	var framework crmcontracts.ReportingFramework
	var settings crmcontracts.InstallationSettings
	var company crmcontracts.CompanyContextCapabilities
	var pipelines crmcontracts.PipelineListResponse
	mustCall(t, e, http.MethodGet, "/v1/analytics/context", nil, http.StatusOK, &context)
	mustCall(t, e, http.MethodGet, "/v1/analytics/metrics", nil, http.StatusOK, &catalog)
	mustCall(t, e, http.MethodGet, "/v1/analytics/framework", nil, http.StatusOK, &framework)
	mustCall(t, e, http.MethodGet, "/v1/installation/settings", nil, http.StatusOK, &settings)
	mustCall(t, e, http.MethodGet, "/v1/company/context/capabilities", nil, http.StatusOK, &company)
	mustCall(t, e, http.MethodGet, "/v1/pipelines", nil, http.StatusOK, &pipelines)
	// Each response resolves its own clock; compare the authorization and data.
	context.AsOf, startup.Context.AsOf = time.Time{}, time.Time{}
	settings.AgentAllowances.AgentWindowResetsAt, startup.InstallationSettings.AgentAllowances.AgentWindowResetsAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(startup.Context, context) {
		t.Error("context differs from its individual read")
	}
	if !reflect.DeepEqual(startup.Catalog, catalog) {
		t.Error("catalog differs from its individual read")
	}
	if !reflect.DeepEqual(startup.Framework, framework) {
		t.Error("framework differs from its individual read")
	}
	if !reflect.DeepEqual(startup.InstallationSettings, settings) {
		t.Error("settings differ from their individual read")
	}
	if !reflect.DeepEqual(startup.CompanyContext, company) {
		t.Error("company capabilities differ from their individual read")
	}
	if !reflect.DeepEqual(startup.Pipelines, &pipelines.Data) {
		t.Error("pipelines differ from their individual read")
	}
}

func TestAnalyticsStartupUsesEachSeatsLiveReportGrants(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	roles := apptest.SeededRoles(t, e)
	allowed, denied := 0, 0
	for _, role := range roles {
		t.Run(role.Key, func(t *testing.T) {
			email := "startup-" + role.Key + "@example.com"
			var user struct {
				ID string `json:"id"`
			}
			body := AnyMap{"email": email, "display_name": "Analytics seat", "role": role.Key}
			if status := e.Call(t, http.MethodPost, "/v1/users", body, nil, &user); status != http.StatusCreated {
				t.Fatalf("create seat = %d", status)
			}
			const password = "analytics-startup-test-password"
			apptest.SetPasswordByLink(t, e, user.ID, password)
			seat := e.AsSecondSeat(t, email, password)
			var me crmcontracts.MeResponse
			if status := seat.Call(t, http.MethodGet, "/v1/me?prepare=analytics", nil, nil, &me); status != http.StatusOK {
				t.Fatalf("session = %d", status)
			}
			canRead := role.Grants("report_definition", "read") && role.Grants("reporting_framework", "read") && (role.Grants("deal", "read") || role.Grants("forecast", "read"))
			if !canRead {
				denied++
				if me.AnalyticsStartup != nil {
					t.Fatal("seat without report grants received Analytics data")
				}
				return
			}
			allowed++
			if me.AnalyticsStartup == nil {
				t.Fatal("seat with report grants received no Analytics data")
			}
			var frame crmcontracts.AnalyticsContext
			if status := seat.Call(t, http.MethodGet, "/v1/analytics/context", nil, nil, &frame); status != http.StatusOK {
				t.Fatalf("individual context = %d", status)
			}
			frame.AsOf, me.AnalyticsStartup.Context.AsOf = time.Time{}, time.Time{}
			if !reflect.DeepEqual(frame, me.AnalyticsStartup.Context) {
				t.Fatal("prepared context differs from the seat's own context")
			}
			if (me.AnalyticsStartup.Pipelines != nil) != role.Grants("pipeline", "read") {
				t.Fatal("prepared pipelines do not match the seat's pipeline grant")
			}
		})
	}
	if allowed == 0 || allowed+denied != len(roles) {
		t.Fatalf("role coverage = %d allowed, %d denied out of %d", allowed, denied, len(roles))
	}
}

func TestPreparedSessionNegotiatesCompressionWithoutLosingIdentityOrMetadata(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var plain crmcontracts.MeResponse
	mustCall(t, e, http.MethodGet, "/v1/me", nil, http.StatusOK, &plain)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, e.TS.URL+"/v1/me?prepare=analytics", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Encoding", "gzip")
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := res.Body.Close(); err != nil {
			t.Error(err)
		}
	})
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("prepared session = %d, encoding %q", res.StatusCode, res.Header.Get("Content-Encoding"))
	}
	if res.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatal("compressed session permits caching")
	}
	encoded, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if len(encoded) >= len(decoded) {
		t.Fatal("prepared metadata did not shrink on the wire")
	}
	var prepared crmcontracts.MeResponse
	if err := json.Unmarshal(decoded, &prepared); err != nil {
		t.Fatal(err)
	}
	if prepared.User.Id != plain.User.Id || prepared.User.Email != plain.User.Email {
		t.Fatal("compression changed the signed-in identity")
	}
	if prepared.AnalyticsStartup == nil {
		t.Fatal("compressed session lost its prepared metadata")
	}
	var catalog crmcontracts.ReportingCatalog
	mustCall(t, e, http.MethodGet, "/v1/analytics/metrics", nil, http.StatusOK, &catalog)
	if !reflect.DeepEqual(prepared.AnalyticsStartup.Catalog, catalog) {
		t.Fatal("compressed metadata differs from its authorized individual read")
	}
}
