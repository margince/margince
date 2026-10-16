// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The installation has one name: the company profile's display name. A human
// saving it renames the installation in the same transaction, and every
// surface that names the installation reads the new name.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
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
		AnyMap{"value": "  Corrected Field SE  "}, nil, nil); status != http.StatusOK {
		t.Fatalf("PATCH the anchor's display_name field = %d, want 200", status)
	}
	if got := anchorName(t, e, anchor); got != "Corrected Field SE" {
		t.Errorf("after a field correction the anchor is %q, want the trimmed %q", got, "Corrected Field SE")
	}
	if got := settingsName(t, e); got != "Corrected Field SE" {
		t.Errorf("after a field correction the installation is %q, want %q", got, "Corrected Field SE")
	}
}

// renameDoor sends an agent's rename of the anchor through one door and answers
// what came back and whether the door refused it.
type renameDoor func(t *testing.T, name string) (answer string, refused bool)

func anchorRenameDoors(d *twoDoors, anchor string) map[string]renameDoor {
	rest := func(path, key string) renameDoor {
		return func(t *testing.T, name string) (string, bool) {
			t.Helper()
			var answer json.RawMessage
			status := d.e.Call(t, http.MethodPatch, path, AnyMap{key: name}, d.bearer, &answer)
			if status != http.StatusOK && status != http.StatusUnprocessableEntity {
				t.Fatalf("PATCH %s = %d %s, want 200 or 422", path, status, answer)
			}
			return string(answer), status == http.StatusUnprocessableEntity
		}
	}
	tool := func(name string, args func(string) map[string]any) renameDoor {
		return func(t *testing.T, value string) (string, bool) {
			t.Helper()
			got := d.mcp.Call(t, name, args(value))
			return got.Text, got.IsError
		}
	}
	return map[string]renameDoor{
		"PATCH the record": rest("/v1/companies/"+anchor, "display_name"),
		"PATCH the field":  rest("/v1/companies/"+anchor+"/profile-fields/display_name", "value"),
		"update_record": tool("update_record", func(name string) map[string]any {
			return map[string]any{"record_type": "company", "id": anchor, "fields": map[string]any{"display_name": name}}
		}),
		"change_record_parts": tool("change_record_parts", func(name string) map[string]any {
			return map[string]any{"action": "update_company_profile_field", "company_id": anchor, "field": "display_name", "value": name}
		}),
	}
}

// No agent rename of the anchor reaches an approval, on any door that carries
// one; resending its current name is no rename.
func TestAnAgentRenamingTheAnchorIsRefusedBeforeAnyoneIsAsked(t *testing.T) {
	d := newTwoDoors(t, "anchor-renamer", "read", "write")
	anchor := saveAnchor(t, d.e, "Onboarded Name AG")
	// A name a human last wrote is what an agent's rename stages over.
	if status := d.e.Call(t, http.MethodPatch, "/v1/companies/"+anchor,
		AnyMap{"display_name": "Profile Name AG"}, nil, nil); status != http.StatusOK {
		t.Fatalf("a human renaming the anchor = %d, want 200", status)
	}

	doors := anchorRenameDoors(d, anchor)
	// Renames run before resends. A resend makes the agent the name's last
	// writer, and a rename over an agent's name never stages.
	for door, rename := range doors {
		answer, refused := rename(t, "Agent Picked Ltd")
		if !refused || !strings.Contains(answer, values.AnchorRenameRefusal) {
			t.Errorf("%s: an agent's rename answered %s, want it refused as the installation's name", door, answer)
		}
		if !strings.Contains(answer, "display_name") || !strings.Contains(answer, "anchor_protected") ||
			strings.Contains(answer, "inputSchema") {
			t.Errorf("%s: the refusal %s must name display_name and anchor_protected, and not send the agent to the schema",
				door, answer)
		}
	}
	for door, rename := range doors {
		if answer, refused := rename(t, "Profile Name AG"); refused {
			t.Errorf("%s: an agent resending the anchor's name was refused: %s", door, answer)
		}
	}
	var staged int
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM approval`).Scan(&staged); err != nil {
		t.Fatalf("counting approvals: %v", err)
	}
	if staged != 0 {
		t.Errorf("the agent's renames and resends staged %d approvals, want none", staged)
	}
	if got := anchorName(t, d.e, anchor); got != "Profile Name AG" {
		t.Errorf("the refused renames left the anchor named %q, want %q", got, "Profile Name AG")
	}
}

// The anchor's name carries the installation name's bound on every path that
// renames it; a customer company's name does not.
func TestAnAnchorNameTheInstallationCannotCarryIsRefused(t *testing.T) {
	e := oneNameApp(t)
	anchor := saveAnchor(t, e, "Profile Name AG")
	tooLong := strings.Repeat("a", 201)

	for name, code := range map[string]string{tooLong: "too_long", "   ": "required"} {
		body := wellFormedCompany()
		body["display_name"] = name
		var problem companyProblem
		if status := e.Call(t, http.MethodPut, "/v1/company", body, nil, &problem); status != http.StatusUnprocessableEntity {
			t.Fatalf("PUT /v1/company with a %d-rune name = %d, want 422", len(name), status)
		}
		wantNameFault(t, problem, code)
		var patched companyProblem
		if status := e.Call(t, http.MethodPatch, "/v1/companies/"+anchor,
			AnyMap{"display_name": name}, nil, &patched); status != http.StatusUnprocessableEntity {
			t.Fatalf("PATCH the anchor's display_name to %d runes = %d, want 422", len(name), status)
		}
		wantNameFault(t, patched, code)
		var corrected companyProblem
		if status := e.Call(t, http.MethodPatch, "/v1/companies/"+anchor+"/profile-fields/display_name",
			AnyMap{"value": name}, nil, &corrected); status != http.StatusUnprocessableEntity {
			t.Fatalf("correct the anchor's display_name to %d runes = %d, want 422", len(name), status)
		}
		wantNameFault(t, corrected, code)
	}
	if got := anchorName(t, e, anchor); got != "Profile Name AG" {
		t.Errorf("a refused rename left the anchor named %q", got)
	}
	if got := settingsName(t, e); got != "Profile Name AG" {
		t.Errorf("a refused rename moved the installation to %q", got)
	}

	customer := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Brandt Logistik GmbH", "source": "manual"})
	if status := e.Call(t, http.MethodPatch, "/v1/companies/"+customer,
		AnyMap{"display_name": tooLong}, nil, nil); status != http.StatusOK {
		t.Errorf("a customer's 201-character name = %d, want 200: only the anchor carries the bound", status)
	}
}

// Renaming the anchor renames the installation. So a seat that may edit the
// record but not the installation is refused, and resending the name is no rename.
func TestOnlyAnAdministratorRenamesTheAnchor(t *testing.T) {
	e := oneNameApp(t)
	anchor := saveAnchor(t, e, "Profile Name AG")
	if status := e.Call(t, http.MethodPatch, "/v1/companies/"+anchor,
		AnyMap{"owner_id": sessionUserID(t, e)}, nil, nil); status != http.StatusOK {
		t.Fatalf("hand the anchor to the seat about to be a rep = %d, want 200", status)
	}
	demoteToRep(t, e)

	var refused companyProblem
	if status := e.Call(t, http.MethodPatch, "/v1/companies/"+anchor,
		AnyMap{"display_name": "Rep Picked GmbH"}, nil, &refused); status != http.StatusForbidden {
		t.Fatalf("a rep renaming the anchor = %d, want 403", status)
	}
	if refused.Code != "permission_denied" || !strings.Contains(refused.Detail, "only an administrator renames the company") {
		t.Errorf("refusal = %s %q, want permission_denied saying only an administrator renames the company",
			refused.Code, refused.Detail)
	}
	if status := e.Call(t, http.MethodPatch, "/v1/companies/"+anchor+"/profile-fields/display_name",
		AnyMap{"value": "Rep Picked GmbH"}, nil, nil); status != http.StatusForbidden {
		t.Errorf("a rep correcting the anchor's name = %d, want 403", status)
	}
	if status := e.Call(t, http.MethodPatch, "/v1/companies/"+anchor+"/profile-fields/display_name",
		AnyMap{"value": "Profile Name AG"}, nil, nil); status != http.StatusOK {
		t.Errorf("a rep resending the anchor's own name = %d, want 200", status)
	}
	if got := anchorName(t, e, anchor); got != "Profile Name AG" {
		t.Errorf("the refused rename left the anchor named %q", got)
	}
}

func anchorName(t *testing.T, e *apptest.AppEnv, anchor string) string {
	t.Helper()
	var record struct {
		DisplayName string `json:"display_name"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/companies/"+anchor, nil, nil, &record); status != http.StatusOK {
		t.Fatalf("GET /v1/companies/{anchor} = %d, want 200", status)
	}
	return record.DisplayName
}

func wantNameFault(t *testing.T, problem companyProblem, code string) {
	t.Helper()
	if len(problem.Details.Errors) != 1 || problem.Details.Errors[0].Field != "display_name" ||
		problem.Details.Errors[0].Code != code {
		t.Errorf("refusal = %+v, want one %s error on display_name", problem.Details.Errors, code)
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

	before := settingsTimezone(t, e)
	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings",
		AnyMap{"name": "Typed Elsewhere Inc", "timezone": "Asia/Tokyo"}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("a refused rename beside a valid zone = %d, want 422", status)
	}
	if got := settingsTimezone(t, e); got != before {
		t.Errorf("the refused patch moved the zone to %q; one refused field writes nothing", got)
	}
}

func settingsTimezone(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var settings struct {
		Timezone string `json:"timezone"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/installation/settings", nil, nil, &settings); status != http.StatusOK {
		t.Fatalf("GET /v1/installation/settings = %d, want 200", status)
	}
	return settings.Timezone
}
