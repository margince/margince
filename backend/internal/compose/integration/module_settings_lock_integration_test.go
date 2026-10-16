// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// lockedWrite is one request to a settings surface whose field a boot source
// sets, and the status it must answer.
type lockedWrite struct {
	name   string
	method string
	path   string
	body   any
	want   int
}

// expectLockedWrites sends each write in order and fails on any other status.
func expectLockedWrites(t *testing.T, e *apptest.AppEnv, writes []lockedWrite) {
	t.Helper()
	for _, w := range writes {
		if status := e.Call(t, w.method, w.path, w.body, nil, nil); status != w.want {
			t.Errorf("%s: %s %s → %d, want %d", w.name, w.method, w.path, status, w.want)
		}
	}
}

// The follow-up window the api's environment sets reads as that value and
// refuses a change, even an invalid one; resending it is accepted.
func TestFollowUpWindowSetByEnvIsLockedAgainstAChange(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{"MARGINCE_ACTIVITIES_FOLLOW_UP_AFTER_DAYS=6"})
	e.BootstrapWorkspace(t)

	const path = "/v1/activities/follow-up-settings"
	var read struct {
		Days int `json:"follow_up_after_days"`
	}
	if status := e.Call(t, http.MethodGet, path, nil, nil, &read); status != http.StatusOK || read.Days != 6 {
		t.Fatalf("GET %s → %d, %d days, want the 6 the environment sets", path, status, read.Days)
	}
	expectLockedWrites(t, e, []lockedWrite{
		{"a changed window", http.MethodPatch, path, map[string]any{"follow_up_after_days": 3}, http.StatusConflict},
		{"a window past its bounds", http.MethodPatch, path, map[string]any{"follow_up_after_days": 45}, http.StatusConflict},
		{"the locked window resent", http.MethodPatch, path, map[string]any{"follow_up_after_days": 6}, http.StatusOK},
	})
	if rows := settingRows(t, e, activities.FollowUpAfterDays.Key()); rows != 0 {
		t.Fatalf("the locked window wrote %d rows, want none", rows)
	}
}

// The particulars the api's environment sets read as that value and refuse a
// change, even an invalid one; resending them is accepted.
func TestControllerParticularsSetByEnvAreLockedAgainstAChange(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{
		`MARGINCE_CONSENT_CONTROLLER_PARTICULARS={"LegalName":"Env GmbH","PostalAddress":"Envweg 1"}`,
	})
	e.BootstrapWorkspace(t)

	const path = "/v1/privacy/controller-particulars"
	var read struct {
		LegalName     string `json:"legal_name"`
		PostalAddress string `json:"postal_address"`
	}
	if status := e.Call(t, http.MethodGet, path, nil, nil, &read); status != http.StatusOK || read.LegalName != "Env GmbH" {
		t.Fatalf("GET %s → %d, %+v, want the Env GmbH the environment sets", path, status, read)
	}
	same := map[string]any{"legal_name": "Env GmbH", "postal_address": "Envweg 1"}
	expectLockedWrites(t, e, []lockedWrite{
		{"a changed name", http.MethodPut, path, map[string]any{"legal_name": "Other GmbH", "postal_address": "Envweg 1"}, http.StatusConflict},
		{"a name of only whitespace", http.MethodPut, path, map[string]any{"legal_name": "   "}, http.StatusConflict},
		{"the locked particulars resent", http.MethodPut, path, same, http.StatusOK},
	})
	if rows := settingRows(t, e, consent.ControllerIdentity.Key()); rows != 0 {
		t.Fatalf("the locked particulars wrote %d rows, want none", rows)
	}
}

// The lead settings the api's environment sets read as those values and refuse
// a change, even an invalid one; resending them is accepted.
func TestLeadSettingsSetByEnvAreLockedAgainstAChange(t *testing.T) {
	seat := ids.NewV7().String()
	e := apptest.SetupAppWithEnv(t, []string{
		"MARGINCE_CONTACTS_FIRST_RESPONSE_TARGET_MINUTES=60",
		"MARGINCE_CONTACTS_UNASSIGNED_ESCALATION_USER_ID=" + seat,
	})
	e.BootstrapWorkspace(t)

	const path = "/v1/leads/settings"
	var read struct {
		Target int `json:"first_response_target_minutes"`
	}
	if status := e.Call(t, http.MethodGet, path, nil, nil, &read); status != http.StatusOK || read.Target != 60 {
		t.Fatalf("GET %s → %d, target %d, want the 60 the environment sets", path, status, read.Target)
	}
	expectLockedWrites(t, e, []lockedWrite{
		{"a changed target", http.MethodPatch, path, map[string]any{"first_response_target_minutes": 90}, http.StatusConflict},
		{"a target below its bounds", http.MethodPatch, path, map[string]any{"first_response_target_minutes": 5}, http.StatusConflict},
		{"another seat, which is no colleague", http.MethodPatch, path, map[string]any{"unassigned_escalation_user_id": ids.NewV7().String()}, http.StatusConflict},
		{"the locked values resent beside the switch", http.MethodPatch, path,
			map[string]any{"first_response_target_minutes": 60, "unassigned_escalation_user_id": seat, "first_response_enabled": true}, http.StatusOK},
	})
	for _, key := range []string{contacts.FirstResponseTargetMinutes.Key(), contacts.UnassignedEscalationUserID.Key()} {
		if rows := settingRows(t, e, key); rows != 0 {
			t.Errorf("the locked %s wrote %d rows, want none", key, rows)
		}
	}
	if rows := settingRows(t, e, contacts.FirstResponseEnabled.Key()); rows != 1 {
		t.Errorf("the switch saved beside the locked values wrote %d rows, want 1", rows)
	}
}

// The signature template the api's environment sets reads as that value and
// refuses a change, even an invalid one; resending it is accepted.
func TestSignatureTemplateSetByEnvIsLockedAgainstAChange(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{"MARGINCE_CONTACTS_SIGNATURE_TEMPLATE=<p>{name}</p>"})
	e.BootstrapWorkspace(t)

	const path = "/v1/email-signature-template"
	var read struct {
		Template string `json:"template"`
	}
	if status := e.Call(t, http.MethodGet, path, nil, nil, &read); status != http.StatusOK || read.Template != "<p>{name}</p>" {
		t.Fatalf("GET %s → %d, %q, want the template the environment sets", path, status, read.Template)
	}
	expectLockedWrites(t, e, []lockedWrite{
		{"a changed template", http.MethodPut, path, map[string]any{"template": "<p>{title}</p>"}, http.StatusConflict},
		{"a template past its bound", http.MethodPut, path, map[string]any{"template": strings.Repeat("x", contacts.SignatureTemplateMaxRunes+1)}, http.StatusConflict},
		{"the locked template resent", http.MethodPut, path, map[string]any{"template": "<p>{name}</p>"}, http.StatusOK},
	})
	if rows := settingRows(t, e, contacts.SignatureTemplate.Key()); rows != 0 {
		t.Fatalf("the locked template wrote %d rows, want none", rows)
	}
}
