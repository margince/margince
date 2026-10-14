// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The installation has one name: the company profile's display name. A human
// saving it renames the installation in the same transaction, and every
// surface that names the installation reads the new name.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

const configuredName = "Configured Name GmbH"

// oneNameApp is an installation bootstrapped from configuration, with the data
// reset armed so its confirmation can be asked which name it expects.
func oneNameApp(t *testing.T) *apptest.AppEnv {
	t.Helper()
	e := apptest.SetupAppWithOptions(t,
		compose.WithDataReset(nil, deployconfig.Seeds{}, true),
		compose.WithDataResetAvailable(true),
	)
	apptest.BootstrapWorkspaceSession(t, e, configuredName, "ada@example.com", "Ada Admin")
	return e
}

func meWorkspaceName(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var me struct {
		WorkspaceName string `json:"workspace_name"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("GET /v1/me = %d, want 200", status)
	}
	return me.WorkspaceName
}

func settingsName(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var settings struct {
		Name string `json:"name"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/installation/settings", nil, nil, &settings); status != http.StatusOK {
		t.Fatalf("GET /v1/installation/settings = %d, want 200", status)
	}
	return settings.Name
}

func saveAnchor(t *testing.T, e *apptest.AppEnv, name string) string {
	t.Helper()
	body := wellFormedCompany()
	body["display_name"] = name
	var saved companyProfileDTO
	if status := e.Call(t, http.MethodPut, "/v1/company", body, nil, &saved); status != http.StatusOK {
		t.Fatalf("PUT /v1/company = %d, want 200", status)
	}
	return saved.CompanyID
}

func TestSavingTheCompanyProfileRenamesTheInstallation(t *testing.T) {
	e := oneNameApp(t)
	if got := meWorkspaceName(t, e); got != configuredName {
		t.Fatalf("before onboarding /me workspace_name = %q, want the configured %q", got, configuredName)
	}

	saveAnchor(t, e, "Profile Name AG")

	if got := meWorkspaceName(t, e); got != "Profile Name AG" {
		t.Errorf("/me workspace_name = %q after the profile save, want %q", got, "Profile Name AG")
	}
	if got := settingsName(t, e); got != "Profile Name AG" {
		t.Errorf("installation settings name = %q after the profile save, want %q", got, "Profile Name AG")
	}
	var login struct {
		WorkspaceName string `json:"workspace_name"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/auth/login", AnyMap{
		"email": "ada@example.com", "password": "correct-horse-battery",
	}, nil, &login); status != http.StatusOK {
		t.Fatalf("sign in again = %d, want 200", status)
	}
	if login.WorkspaceName != "Profile Name AG" {
		t.Errorf("sign-in names the installation %q, want %q", login.WorkspaceName, "Profile Name AG")
	}
	if status := e.Call(t, http.MethodPost, "/v1/admin/reset-data",
		AnyMap{"confirmation": configuredName}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("reset confirmed with the configured name = %d, want 422", status)
	}
	if status := e.Call(t, http.MethodPost, "/v1/admin/reset-data",
		AnyMap{"confirmation": "Profile Name AG"}, nil, nil); status != http.StatusOK {
		t.Fatalf("reset confirmed with the profile's name = %d, want 200", status)
	}
	// The reset sweeps the anchor and keeps the name; the next save sets it again.
	if got := meWorkspaceName(t, e); got != "Profile Name AG" {
		t.Errorf("after the reset /me workspace_name = %q, want the kept %q", got, "Profile Name AG")
	}
	saveAnchor(t, e, "Second Start KG")
	if got := meWorkspaceName(t, e); got != "Second Start KG" {
		t.Errorf("after onboarding again /me workspace_name = %q, want %q", got, "Second Start KG")
	}
}

func TestAHumanRenamingTheAnchorRecordRenamesTheInstallation(t *testing.T) {
	e := oneNameApp(t)
	anchor := saveAnchor(t, e, "Profile Name AG")

	if status := e.Call(t, http.MethodPatch, "/v1/companies/"+anchor,
		AnyMap{"display_name": "Edited Record GmbH"}, nil, nil); status != http.StatusOK {
		t.Fatalf("PATCH /v1/companies/{anchor} = %d, want 200", status)
	}
	if got := settingsName(t, e); got != "Edited Record GmbH" {
		t.Errorf("after a record edit the installation is %q, want %q", got, "Edited Record GmbH")
	}

	if status := e.Call(t, http.MethodPatch, "/v1/companies/"+anchor+"/profile-fields/display_name",
		AnyMap{"value": "Corrected Field SE"}, nil, nil); status != http.StatusOK {
		t.Fatalf("PATCH the anchor's display_name field = %d, want 200", status)
	}
	if got := settingsName(t, e); got != "Corrected Field SE" {
		t.Errorf("after a field correction the installation is %q, want %q", got, "Corrected Field SE")
	}
}

func TestAnAgentRenamingTheAnchorLeavesTheInstallationName(t *testing.T) {
	e := oneNameApp(t)
	anchor := saveAnchor(t, e, "Profile Name AG")
	agent := apptest.PassportBearer(t, e, "renaming agent", "read", "write")
	rename := AnyMap{"display_name": "Agent Picked Ltd"}

	var staged companyProblem
	if status := e.Call(t, http.MethodPatch, "/v1/companies/"+anchor, rename, agent, &staged); status != http.StatusForbidden {
		t.Fatalf("an agent's first PATCH /v1/companies/{anchor} = %d, want 403 approval_required", status)
	}
	approval := ExtractStagedApprovalID(t, staged.Detail)
	if status := e.Call(t, http.MethodPost, "/v1/approvals/"+approval+"/approve", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("human approve = %d, want 200", status)
	}
	agent["X-Approval-Token"] = approval
	if status := e.Call(t, http.MethodPatch, "/v1/companies/"+anchor, rename, agent, nil); status != http.StatusOK {
		t.Fatalf("the approved agent PATCH = %d, want 200", status)
	}
	var record struct {
		DisplayName string `json:"display_name"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/companies/"+anchor, nil, nil, &record); status != http.StatusOK ||
		record.DisplayName != "Agent Picked Ltd" {
		t.Fatalf("the anchor reads %d %q after the approved rename, want 200 %q", status, record.DisplayName, "Agent Picked Ltd")
	}
	if got := settingsName(t, e); got != "Profile Name AG" {
		t.Errorf("an agent's rename moved the installation to %q, want %q", got, "Profile Name AG")
	}
}

func TestTheDeprecatedSettingsNameCannotDriftFromTheProfile(t *testing.T) {
	e := oneNameApp(t)
	saveAnchor(t, e, "Profile Name AG")

	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings",
		AnyMap{"name": "Profile Name AG", "timezone": "Europe/Berlin"}, nil, nil); status != http.StatusOK {
		t.Fatalf("an older client resending the stored name = %d, want 200", status)
	}
	var problem companyProblem
	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings",
		AnyMap{"name": "Typed Elsewhere Inc"}, nil, &problem); status != http.StatusUnprocessableEntity {
		t.Fatalf("renaming through the settings = %d, want 422", status)
	}
	if len(problem.Details.Errors) != 1 || problem.Details.Errors[0].Code != "set_on_company_profile" {
		t.Errorf("refusal = %+v, want one set_on_company_profile error", problem.Details.Errors)
	}
	if got := settingsName(t, e); got != "Profile Name AG" {
		t.Errorf("the refused rename left the installation as %q, want %q", got, "Profile Name AG")
	}
}
