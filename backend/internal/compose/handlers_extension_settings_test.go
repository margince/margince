// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

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
