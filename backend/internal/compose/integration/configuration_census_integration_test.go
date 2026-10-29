// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// lockedSurface is a settings response and the writes that would change a
// setting it carries. A write that must first read a revision has none here;
// its own lock test sends it.
type lockedSurface struct {
	contractPath string
	path         string
	writes       []lockedWrite
}

const (
	gmailEnvClientID = "env-client.apps.googleusercontent.com"
	gmailEnvSecret   = "GOCSPX-from-the-environment"
)

func lockedSurfaces() []lockedSurface {
	binding := strings.TrimPrefix(envBinding("changed-model"), "MARGINCE_AI_ROUTING_BINDING=")
	secret := "a-changed-secret"
	renamed := wellFormedCompany()
	renamed["display_name"] = "Renamed GmbH"
	return []lockedSurface{
		{"/capture/settings", "/v1/capture/settings", []lockedWrite{
			{method: http.MethodPatch, path: "/v1/capture/settings", body: map[string]any{"auto_enrich_daily_cap": 5}},
		}},
		{"/ai/routing", "/v1/ai/routing", []lockedWrite{
			{method: http.MethodPut, path: "/v1/ai/routing", body: rawJSON(binding)},
			{method: http.MethodPut, path: "/v1/ai/provider-settings/openai_compatible", body: map[string]any{"base_url": "https://openrouter.ai/api"}},
			{method: http.MethodPut, path: "/v1/ai/task-overrides", body: map[string]any{"capture_classify": map[string]any{"thinking": "high"}}},
		}},
		{"/ai/provider-keys", "/v1/ai/provider-keys", []lockedWrite{
			{method: http.MethodPut, path: "/v1/ai/provider-keys/anthropic", body: map[string]any{"api_key": "sk-changed"}},
			{method: http.MethodDelete, path: "/v1/ai/provider-keys/anthropic"},
		}},
		{"/ai/budget", "/v1/ai/budget", nil},
		{"/ai/price-sync", "/v1/ai/price-sync", []lockedWrite{
			{method: http.MethodPut, path: "/v1/ai/price-sync", body: map[string]any{"auto_sync": false}},
		}},
		{"/integrations/settings", "/v1/integrations/settings", []lockedWrite{
			{method: http.MethodPatch, path: "/v1/integrations/settings", body: map[string]any{"automatic_lookup": false}},
		}},
		{"/leads/settings", "/v1/leads/settings", []lockedWrite{
			{method: http.MethodPatch, path: "/v1/leads/settings", body: map[string]any{"first_response_target_minutes": 90}},
		}},
		{"/email-signature-template", "/v1/email-signature-template", []lockedWrite{
			{method: http.MethodPut, path: "/v1/email-signature-template", body: map[string]any{"template": "<p>{title}</p>"}},
		}},
		{"/retention/settings", "/v1/retention/settings", []lockedWrite{
			{method: http.MethodPatch, path: "/v1/retention/settings", body: map[string]any{"retain_only": true}},
		}},
		{"/activities/follow-up-settings", "/v1/activities/follow-up-settings", []lockedWrite{
			{method: http.MethodPatch, path: "/v1/activities/follow-up-settings", body: map[string]any{"follow_up_after_days": 3}},
		}},
		{"/legal-reviews/settings", "/v1/legal-reviews/settings", []lockedWrite{
			{method: http.MethodPut, path: "/v1/legal-reviews/settings", body: map[string]any{"enabled": false}},
		}},
		{"/forecast/assurance/enforcement", "/v1/forecast/assurance/enforcement", []lockedWrite{
			{method: http.MethodPut, path: "/v1/forecast/assurance/enforcement", body: map[string]any{"enabled": true}},
		}},
		{"/dedupe/settings", "/v1/dedupe/settings", []lockedWrite{
			{method: http.MethodPut, path: "/v1/dedupe/settings", body: map[string]any{"enabled": true}},
		}},
		{"/privacy/controller-particulars", "/v1/privacy/controller-particulars", []lockedWrite{
			{method: http.MethodPut, path: "/v1/privacy/controller-particulars", body: map[string]any{"legal_name": "Other GmbH"}},
		}},
		{"/installation/settings", "/v1/installation/settings", []lockedWrite{
			{method: http.MethodPatch, path: "/v1/installation/settings", body: map[string]any{"send_rate_limit": 8}},
			{method: http.MethodPut, path: "/v1/company", body: renamed},
		}},
		{"/installation/oauth-apps/{provider}", "/v1/installation/oauth-apps/google", []lockedWrite{
			{method: http.MethodPut, path: "/v1/installation/oauth-apps/google", body: crmcontracts.ConnectorAppInput{ClientId: "another.apps.googleusercontent.com", ClientSecret: &secret}},
			{method: http.MethodDelete, path: "/v1/installation/oauth-apps/google"},
		}},
	}
}

// rawJSON is a body sent as written.
type rawJSON string

func (r rawJSON) MarshalJSON() ([]byte, error) { return []byte(r), nil }

// unsurfaced are the stored settings no HTTP surface writes, so no response
// reports their lock. A scan of the save calls cannot name them: identity and
// capture save through a loop variable. The census fails when one gains a surface.
var unsurfaced = []string{"consent.authorization_modes", "deals.stage_autopilot_enabled", "deals.maintenance_writes_enabled"}

// censusValues are boot values for the stored settings that declare no
// usable default. Every other stored setting is set to its own default.
//
// gatekit:fixture boot values a test environment sets, not reasons
var censusValues = map[string]string{
	"ai.routing":                             "{}",
	"installation.name":                      "Census GmbH",
	"installation.country":                   "zk",
	"identity.enabled_oidc_providers":        "google",
	"identity.oidc_group_role_map":           "sales:rep",
	"consent.controller_particulars":         `{"LegalName":"Census GmbH"}`,
	"contacts.unassigned_escalation_user_id": ids.NewV7().String(),
	"contacts.signature_template":            "<p>{name}</p>",
}

// everyStoredSettingSet is an environment in which a boot source sets every
// stored setting, so every settings response must report every lock it can.
func everyStoredSettingSet(t *testing.T) (environ []string, envOf map[string]string) {
	t.Helper()
	envOf = map[string]string{}
	for _, s := range compose.StoredSettings() {
		value, ok := censusValues[s.Key()]
		if !ok {
			value, ok = s.DefaultText()
		}
		if s.Secret() {
			value, ok = "census-"+strings.ReplaceAll(s.Key(), ".", "-"), true
		}
		if !ok {
			t.Fatalf("%s declares no default; give it a boot value in censusValues", s.Key())
		}
		environ = append(environ, s.Env()+"="+value)
		envOf[s.Key()] = s.Env()
	}
	return environ, envOf
}

func loadContract(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatalf("loading the contract: %v", err)
	}
	return doc
}

// surfacesWithLocks are the GET paths whose answer carries `locks`, read from
// the contract, so a surface that gains the field joins the test below.
func surfacesWithLocks(doc *openapi3.T) []string {
	var paths []string
	for path, item := range doc.Paths.Map() {
		if item.Get == nil {
			continue
		}
		ok := item.Get.Responses.Status(http.StatusOK)
		if ok == nil || ok.Value == nil || ok.Value.Content.Get("application/json") == nil {
			continue
		}
		schema := ok.Value.Content.Get("application/json").Schema
		if schema != nil && schema.Value != nil && schema.Value.Properties["locks"] != nil {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths
}

// declaresTheLock reports whether the contract's operation for method and a
// request path declares the 409 a lock answers.
func declaresTheLock(doc *openapi3.T, method, requestPath string) bool {
	item := routeOf(doc, strings.TrimPrefix(requestPath, "/v1"))
	if item == nil || item.GetOperation(method) == nil {
		return false
	}
	conflict := item.GetOperation(method).Responses.Status(http.StatusConflict)
	return conflict != nil && (conflict.Ref == "#/components/responses/SettingLocked" ||
		(conflict.Value != nil && conflict.Value.Description != nil && strings.Contains(*conflict.Value.Description, lockedCode)))
}

// routeOf is the contract path a request path takes, the one with the fewest
// parameters where a literal and a template both match.
func routeOf(doc *openapi3.T, requested string) *openapi3.PathItem {
	var best *openapi3.PathItem
	fewest := -1
	for template, item := range doc.Paths.Map() {
		params, ok := templateMatch(template, requested)
		if ok && (fewest < 0 || params < fewest) {
			best, fewest = item, params
		}
	}
	return best
}

func templateMatch(template, requested string) (params int, ok bool) {
	want, got := strings.Split(template, "/"), strings.Split(requested, "/")
	if len(want) != len(got) {
		return 0, false
	}
	for i, segment := range want {
		switch {
		case strings.HasPrefix(segment, "{"):
			params++
		case segment != got[i]:
			return 0, false
		}
	}
	return params, true
}

// Every settings response names each boot-set setting on it, with its variable,
// and every stored setting is reported somewhere. A write to one is refused with
// the lock's code, and its contract declares that 409.
//
// The contract is held to writes on a response's own path and to each refusal
// seen here. Which other handler reaches a lock is a call graph nothing lists.
func TestEverySettingsResponseReportsItsLocks(t *testing.T) {
	doc := loadContract(t)
	surfaces := lockedSurfaces()
	var covered []string
	for _, s := range surfaces {
		covered = append(covered, s.contractPath)
		for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
			if doc.Paths.Value(s.contractPath).GetOperation(method) != nil && !declaresTheLock(doc, method, s.contractPath) {
				t.Errorf("%s %s writes a locked setting and declares no %s 409", method, s.contractPath, lockedCode)
			}
		}
	}
	slices.Sort(covered)
	if declared := surfacesWithLocks(doc); !slices.Equal(declared, covered) {
		t.Fatalf("the contract gives locks to %v; this test covers %v", declared, covered)
	}
	environ, envOf := everyStoredSettingSet(t)
	envOf["capture.google_app"] = "MARGINCE_GMAIL_CLIENT_ID"
	apptest.BindEnv(t, append(environ, "MARGINCE_GMAIL_CLIENT_ID="+gmailEnvClientID, "MARGINCE_GMAIL_CLIENT_SECRET="+gmailEnvSecret))
	e, _ := apptest.SetupGoogleConnect(t, gmailEnvClientID, gmailEnvSecret)

	reported := map[string]bool{}
	for _, s := range surfaces {
		for _, lock := range readLocks(t, e, s.path) {
			reported[lock.Key] = true
			if lock.Origin.Source != crmcontracts.SettingOriginSourceEnv || lock.Origin.Name != envOf[lock.Key] {
				t.Errorf("GET %s locks %s by %+v, want env %s", s.path, lock.Key, lock.Origin, envOf[lock.Key])
			}
		}
		for _, w := range s.writes {
			expectLockRefusal(t, e, doc, w)
		}
	}
	for key := range envOf {
		if want := !slices.Contains(unsurfaced, key); reported[key] != want {
			t.Errorf("%s is reported locked: %v, want %v", key, reported[key], want)
		}
	}
	anchor := saveAnchor(t, e, "Census GmbH")
	expectLockRefusal(t, e, doc, lockedWrite{method: http.MethodPatch, path: "/v1/companies/" + anchor, body: map[string]any{"display_name": "Renamed GmbH"}})
}

func readLocks(t *testing.T, e *apptest.AppEnv, path string) []crmcontracts.SettingLock {
	t.Helper()
	var read struct {
		Locks []crmcontracts.SettingLock `json:"locks"`
	}
	if status := e.Call(t, http.MethodGet, path, nil, nil, &read); status != http.StatusOK {
		t.Errorf("GET %s → %d", path, status)
	}
	return read.Locks
}

// expectLockRefusal sends w and fails unless it is refused with the lock's code
// on a route whose contract declares that refusal.
func expectLockRefusal(t *testing.T, e *apptest.AppEnv, doc *openapi3.T, w lockedWrite) {
	t.Helper()
	var refused problemBody
	if status := e.Call(t, w.method, w.path, w.body, nil, &refused); status != http.StatusConflict || refused.Code != lockedCode {
		t.Errorf("%s %s → %d %q, want 409 %s", w.method, w.path, status, refused.Code, lockedCode)
		return
	}
	if !declaresTheLock(doc, w.method, w.path) {
		t.Errorf("%s %s answers %s and its contract declares no such 409", w.method, w.path, lockedCode)
	}
}
