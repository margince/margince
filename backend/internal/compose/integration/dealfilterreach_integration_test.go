// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// reachApp boots the app with lists on, so list_id is resolved, and refused,
// where the deal list resolves it.
func reachApp(t *testing.T) *apptest.AppEnv {
	t.Helper()
	e := apptest.SetupAppWithOptions(t, compose.WithListsEnabled(true))
	e.BootstrapWorkspace(t)
	return e
}

// probedDealFilters is each filter the deal list binds, with the operand the
// probe sends it. A role that withholds every deal field shows the refusable ones.
func probedDealFilters(t *testing.T) []deals.FilterReach {
	t.Helper()
	var masks []principal.FieldMask
	for _, field := range fieldmask.Maskable(fieldmask.Deal) {
		masks = append(masks, principal.FieldMask{Object: fieldmask.Deal, Field: field, Condition: principal.MaskAlways})
	}
	reach, err := deals.ListFilterReach(principal.WithActor(t.Context(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			RowScope:   principal.RowScopeTeam,
			Objects:    map[string]principal.ObjectGrant{fieldmask.Deal: {Read: true}},
			FieldMasks: masks,
		},
	}), nil)
	if err != nil {
		t.Fatalf("probing the deal list filters: %v", err)
	}
	if !slices.ContainsFunc(reach, func(probe deals.FilterReach) bool { return probe.Refused }) {
		t.Fatal("no deal list filter is refusable, so the snapshot would be checked against nothing")
	}
	return reach
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

// assertSnapshotMatchesTheList sends each deal list filter over HTTP, so the
// snapshot is held to what the list answers, wherever the list refuses it.
func assertSnapshotMatchesTheList(t *testing.T, e *apptest.AppEnv, refused []string) {
	t.Helper()
	probed := probedDealFilters(t)
	for _, filter := range probed {
		listRefuses := theListRefuses(t, e, filter.Parameter, filter.Operand)
		if listRefuses != slices.Contains(refused, filter.Parameter) {
			t.Errorf("the list refuses %s=%s: %v, and /me lists it as refused: %v",
				filter.Parameter, filter.Operand, listRefuses, slices.Contains(refused, filter.Parameter))
		}
	}
	for _, parameter := range refused {
		if !slices.ContainsFunc(probed, func(probe deals.FilterReach) bool { return probe.Parameter == parameter }) {
			t.Errorf("/me refuses %s, which the deal list does not bind", parameter)
		}
	}
}

// theListRefuses reads one filtered deal list. A 403 or a masked field refuses
// the caller; a page, an id that names nothing or a malformed operand does not.
func theListRefuses(t *testing.T, e *apptest.AppEnv, parameter, operand string) bool {
	t.Helper()
	var problem codeRefusal
	status := e.Call(t, "GET", "/v1/deals?"+url.Values{parameter: {operand}}.Encode(), nil, nil, &problem)
	fault := problem.Details.Errors
	switch {
	case status == http.StatusForbidden:
		return true
	case status == http.StatusUnprocessableEntity && len(fault) == 1 && fault[0].Code == "field_masked":
		return true
	case status == http.StatusOK || status == http.StatusNotFound:
		return false
	case status == http.StatusUnprocessableEntity && len(fault) == 1 && fault[0].Field == parameter:
		return false
	}
	t.Errorf("GET /v1/deals?%s=%s = %d %+v, want a page, a refusal or a fault in the operand", parameter, operand, status, fault)
	return false
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
	e := reachApp(t)
	narrowSignedInRole(t, e, "reach_amount_mask", `{}`)
	maskDealField(t, e, "reach_amount_mask", "amount_minor", "always")

	refused := refusedDealFilters(t, e)
	assertRefuses(t, refused, "value_min", "value_max")
	assertSnapshotMatchesTheList(t, e, refused)
}

// A mask that lifts on the deals the reader may change still withholds the
// rest. So the value dials are refused, even on a page of the reader's own deals.
func TestMeNamesTheValueFiltersAWriteConditionedMaskRefuses(t *testing.T) {
	e := reachApp(t)
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
	e := reachApp(t)
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
	e := reachApp(t)
	narrowSignedInRole(t, e, "reach_no_company_read", `{"company": {"create": false, "read": false, "update": false, "delete": false}}`)

	refused := refusedDealFilters(t, e)
	assertRefuses(t, refused, "country")
	assertSnapshotMatchesTheList(t, e, refused)
}

// A seat that may not read lists is refused list_id where the list is resolved,
// before any deal clause runs. /me names it all the same.
func TestMeNamesTheListFilterForASeatWithoutListRead(t *testing.T) {
	e := reachApp(t)
	narrowSignedInRole(t, e, "reach_no_list_read", `{"list": {"create": false, "read": false, "update": false, "delete": false}}`)

	refused := refusedDealFilters(t, e)
	assertRefuses(t, refused, "list_id")
	assertSnapshotMatchesTheList(t, e, refused)
}

// The signed-in admin, unnarrowed, is refused nothing, and the list agrees.
func TestMeRefusesNoDealFilterToTheAdmin(t *testing.T) {
	e := reachApp(t)

	refused := refusedDealFilters(t, e)
	if len(refused) != 0 {
		t.Errorf("/me refused the admin %v, want none", refused)
	}
	assertSnapshotMatchesTheList(t, e, refused)
}
