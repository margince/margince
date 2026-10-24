// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

func visitsUnit() extension.Extension {
	return extension.Extension{Name: "visits", Settings: []extension.Setting{
		{Key: "live", Label: "Live", Kind: extension.SettingSwitch},
		{Key: "region", Label: "Region", Hint: "Where visits happen", Kind: extension.SettingText},
	}}
}

func TestDeclaredChangesRefuseAKeyTheUnitNeverDeclaredAndAValueItsKindCannotHold(t *testing.T) {
	changes, err := declaredChanges(visitsUnit(), []crmcontracts.DeclaredExtensionSetting{
		{Key: "live", Value: "true"}, {Key: "region", Value: "eu-west"},
	})
	if err != nil || changes["live"] != "true" || changes["region"] != "eu-west" {
		t.Fatalf("changes = %v, err = %v, want both saved", changes, err)
	}
	for name, sent := range map[string]crmcontracts.DeclaredExtensionSetting{
		"an undeclared key":    {Key: "other", Value: "x"},
		"a switch set to text": {Key: "live", Value: "maybe"},
		"a multi-line text":    {Key: "region", Value: "a\nb"},
	} {
		if _, err := declaredChanges(visitsUnit(), []crmcontracts.DeclaredExtensionSetting{{Key: "live", Value: "true"}, sent}); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("%s: err = %v, want ErrInvalidArgument for the whole write", name, err)
		}
	}
}

func TestDeclaredViewShowsTheSavedValueOrTheDefault(t *testing.T) {
	view := declaredView(visitsUnit(), map[string]string{"visits/region": "eu-west", "other/region": "elsewhere"})
	if len(view) != 2 || view[0].Key != "live" || view[0].Value != "false" || view[1].Value != "eu-west" {
		t.Fatalf("view = %+v, want live at its default and region as saved", view)
	}
	if *view[1].Hint != "Where visits happen" || *view[0].Kind != crmcontracts.DeclaredExtensionSettingKind("switch") {
		t.Errorf("view does not carry the declared hint and kind: %+v", view)
	}
}

func TestExtensionSettingsRefuseWhatTheCallerMayNotDoAndAnUnknownUnit(t *testing.T) {
	t.Cleanup(func() { setComposedExtensions(nil) })
	human := func(role string, grants map[string]principal.ObjectGrant) context.Context {
		return principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalHuman, Permissions: principal.Permissions{RoleKeys: []string{role}, Objects: grants},
		})
	}
	put := func(ctx context.Context, unit string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/v1/extensions/"+unit+"/settings",
			strings.NewReader(`{"overwrites_edited_fields":true}`)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		extensionSettingsHandlers{}.UpdateExtensionSettings(rec, req, unit)
		return rec.Code
	}
	get := func(ctx context.Context, unit string) int {
		rec := httptest.NewRecorder()
		extensionSettingsHandlers{}.GetExtensionSettings(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx), unit)
		return rec.Code
	}

	if got := get(human("rep", map[string]principal.ObjectGrant{"job_health": {Read: true}}), "alpha"); got != http.StatusForbidden {
		t.Errorf("read without extension_access → %d, want 403", got)
	}
	reader := human("ops", map[string]principal.ObjectGrant{"extension_access": {Read: true}})
	if got := get(reader, "no-such-unit"); got != http.StatusNotFound {
		t.Errorf("read of a unit nobody composed → %d, want 404", got)
	}
	if got := put(human("admin", nil), "no-such-unit"); got != http.StatusNotFound {
		t.Errorf("write for a unit nobody composed → %d, want 404", got)
	}
	agent := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, ID: "agent:feed"})
	if got := put(agent, "alpha"); got != http.StatusForbidden {
		t.Errorf("write by an agent → %d, want 403", got)
	}
}
