// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// dealFilterSamples is one operand per deal list filter a grant or a mask can
// refuse, each of a shape the list accepts.
var dealFilterSamples = []struct{ parameter, sample string }{
	{"company_id", "00000000-0000-7000-8000-000000000001"},
	{"project_id", "00000000-0000-7000-8000-000000000002"},
	{"partner_company_id", "00000000-0000-7000-8000-000000000003"},
	{"partner_sourced", "true"},
	{"partner_attribution", "sourced"},
	{"value_min", "0"},
	{"value_max", "0"},
	{"currency", "EUR"},
	{"country", "DE"},
}

// narrowSignedInRole moves the signed-in admin onto a role of its own: the
// admin's document on team scope, with `objects` patched. Roles and masks
// outlive the per-test reset, so each test names its own and removes it.
func narrowSignedInRole(t *testing.T, e *apptest.AppEnv, role, objects string) {
	t.Helper()
	if _, err := e.Owner.Exec(t.Context(), `
		INSERT INTO role (key, name, is_system, permissions)
		SELECT $1, $1, false,
		       jsonb_set(r.permissions || '{"row_scope": "team"}', '{objects}', r.permissions->'objects' || $2::jsonb)
		FROM role r JOIN role_assignment ra ON ra.role_id = r.id
		JOIN app_user u ON u.id = ra.user_id WHERE u.email = 'ada@example.com'`, role, objects); err != nil {
		t.Fatalf("writing the narrowed role: %v", err)
	}
	t.Cleanup(func() {
		for _, statement := range []string{
			`DELETE FROM field_mask WHERE role_key = $1`,
			`DELETE FROM role_assignment WHERE role_id = (SELECT id FROM role WHERE key = $1)`,
			`DELETE FROM role WHERE key = $1`,
		} {
			if _, err := e.Owner.Exec(context.Background(), statement, role); err != nil {
				t.Errorf("removing the narrowed role: %v", err)
			}
		}
	})
	if _, err := e.Owner.Exec(t.Context(), `
		UPDATE role_assignment SET role_id = (SELECT id FROM role WHERE key = $1)
		WHERE user_id = (SELECT id FROM app_user WHERE email = 'ada@example.com')`, role); err != nil {
		t.Fatalf("assigning the narrowed role: %v", err)
	}
}

func maskDealField(t *testing.T, e *apptest.AppEnv, role, field, condition string) {
	t.Helper()
	if _, err := e.Owner.Exec(t.Context(),
		`INSERT INTO field_mask (role_key, object, field, condition) VALUES ($1, 'deal', $2, $3)`,
		role, field, condition); err != nil {
		t.Fatalf("masking deal.%s: %v", field, err)
	}
}

// refusedDealFilters is what /me tells the signed-in seat it may not narrow by.
func refusedDealFilters(t *testing.T, e *apptest.AppEnv) []string {
	t.Helper()
	var me struct {
		Authorization struct {
			RefusedDealFilters *[]string `json:"refused_deal_filters"`
		} `json:"authorization"`
	}
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("GET /v1/me = %d", status)
	}
	if me.Authorization.RefusedDealFilters == nil {
		t.Fatal("/me carried no refused_deal_filters, so a client cannot tell which dials the list refuses")
	}
	return *me.Authorization.RefusedDealFilters
}

// assertSnapshotMatchesTheList asks the list itself for every sampled filter,
// so the snapshot is held to what the list answers, refused or not.
func assertSnapshotMatchesTheList(t *testing.T, e *apptest.AppEnv, refused []string) {
	t.Helper()
	for _, filter := range dealFilterSamples {
		parameter, sample := filter.parameter, filter.sample
		var problem codeRefusal
		status := e.Call(t, "GET", "/v1/deals?"+parameter+"="+sample, nil, nil, &problem)
		masked := status == http.StatusUnprocessableEntity && len(problem.Details.Errors) == 1 &&
			problem.Details.Errors[0].Code == "field_masked"
		listRefuses := masked || status == http.StatusForbidden
		if !listRefuses && status != http.StatusOK {
			t.Errorf("GET /v1/deals?%s=%s = %d, want 200 or a refusal of the caller's reach", parameter, sample, status)
		}
		if listRefuses != slices.Contains(refused, parameter) {
			t.Errorf("the list refuses %s: %v (status %d), and /me lists it as refused: %v",
				parameter, listRefuses, status, slices.Contains(refused, parameter))
		}
	}
}

func assertRefuses(t *testing.T, refused []string, want ...string) {
	t.Helper()
	for _, parameter := range want {
		if !slices.Contains(refused, parameter) {
			t.Errorf("/me refused %v, want %s among them", refused, parameter)
		}
	}
}

// A role that masks the amount on every deal is refused the value dials, and
// /me says so before the reader picks one.
func TestMeNamesTheValueFiltersARoleMaskRefuses(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	narrowSignedInRole(t, e, "reach_amount_mask", `{}`)
	maskDealField(t, e, "reach_amount_mask", "amount_minor", "always")

	refused := refusedDealFilters(t, e)
	assertRefuses(t, refused, "value_min", "value_max")
	assertSnapshotMatchesTheList(t, e, refused)
}

// A mask that lifts on the deals the reader may change still withholds the
// rest. So the value dials are refused, even on a page of the reader's own deals.
func TestMeNamesTheValueFiltersAWriteConditionedMaskRefuses(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	pipelineID, stageID, _ := companyRollupOpenStage(t, e)
	createSearchableDeal(t, e, pipelineID, stageID, "The reader's own deal", "")
	narrowSignedInRole(t, e, "reach_write_mask", `{}`)
	maskDealField(t, e, "reach_write_mask", "amount_minor", "outside_write_authority")

	refused := refusedDealFilters(t, e)
	assertRefuses(t, refused, "value_min", "value_max")
	assertSnapshotMatchesTheList(t, e, refused)
}

// A deal whose company is private to its captor masks company_id on that row
// alone. The role withholds nothing, so the country dial stays offered.
func TestAPrivateCompanyOnOneDealRefusesNoFilter(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	pipelineID, stageID, _ := companyRollupOpenStage(t, e)
	var company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", AnyMap{
		"display_name": "Captured GmbH", "address": AnyMap{"country": "DE"}, "source": "manual",
	}, nil, &company); status != http.StatusCreated {
		t.Fatalf("creating the company = %d", status)
	}
	var deal struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/deals", AnyMap{
		"name": "On a private company", "pipeline_id": pipelineID, "stage_id": stageID,
		"company_id": company.ID, "source": "manual",
	}, nil, &deal); status != http.StatusCreated {
		t.Fatalf("creating the deal = %d", status)
	}
	if _, err := e.Owner.Exec(t.Context(), `
		WITH captor AS (
			INSERT INTO app_user (id, email, display_name) VALUES (gen_random_uuid(), 'captor@reach.test', 'Captor')
			RETURNING id)
		UPDATE company SET visibility = 'owner', owner_id = (SELECT id FROM captor) WHERE id = $1`, company.ID); err != nil {
		t.Fatalf("making the company private to its captor: %v", err)
	}
	narrowSignedInRole(t, e, "reach_private_company", `{}`)

	var page struct {
		Data []struct {
			ID           string   `json:"id"`
			MaskedFields []string `json:"masked_fields"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deals", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("GET /v1/deals = %d", status)
	}
	masked := false
	for _, row := range page.Data {
		masked = masked || (row.ID == deal.ID && slices.Contains(row.MaskedFields, "company_id"))
	}
	if !masked {
		t.Fatalf("the deal on the private company was not listed with company_id masked: %+v", page.Data)
	}

	refused := refusedDealFilters(t, e)
	if slices.Contains(refused, "country") || slices.Contains(refused, "company_id") {
		t.Errorf("/me refused %v: a mask on one row must not withhold the country or company dial", refused)
	}
	assertSnapshotMatchesTheList(t, e, refused)
}

// The country is the company's, so a seat that may not read companies is
// refused it. The company_id filter stays the list's to answer.
func TestMeNamesTheCountryFilterForASeatWithoutCompanyRead(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	narrowSignedInRole(t, e, "reach_no_company_read", `{"company": {"create": false, "read": false, "update": false, "delete": false}}`)

	refused := refusedDealFilters(t, e)
	assertRefuses(t, refused, "country")
	assertSnapshotMatchesTheList(t, e, refused)
}
