// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/extsecrets"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestUnitSettingValuesAreAdminOnlyAuditedAndKeepEachUnitsOwn(t *testing.T) {
	t.Parallel()
	e := setupCRMIngress(t)
	store := NewSettingsStore(e.Pool)

	member := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, Permissions: principal.Permissions{RoleKeys: []string{"rep"}},
	})
	err := identity.SetExtensionSettings(member, store, ingressUnit, false, map[string]string{"live": "true"})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a member's write: err = %v, want ErrPermissionDenied", err)
	}

	if err := identity.SetExtensionSettings(e.Admin(), store, ingressUnit, false, map[string]string{"live": "true"}); err != nil {
		t.Fatalf("an admin's write: %v", err)
	}
	if err := identity.SetExtensionSettings(e.Admin(), store, "other-unit", false, map[string]string{"live": "false"}); err != nil {
		t.Fatalf("a second unit's write: %v", err)
	}
	got, err := identity.ExtensionSettingValuesOf(e.Admin(), store)
	if err != nil || got[ingressUnit+"/live"] != "true" || got["other-unit/live"] != "false" {
		t.Errorf("stored = %v, err = %v, want each unit's own value", got, err)
	}
	if _, err := identity.ExtensionSettingValuesOf(member, store); err == nil {
		t.Error("a member read the unit setting values")
	}

	audited := e.countAsWorkspace(t,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'installation_settings'
		   AND after ? 'identity.extension_setting_values'`)
	if audited != 2 {
		t.Errorf("audit rows for the unit setting values = %d, want one per change (2)", audited)
	}
}

func TestAUnitNeedsSetupUntilItsWorkspaceSecretIsStored(t *testing.T) {
	e := setupExtRuntime(t)
	const unit = "setup-probe"
	t.Cleanup(func() { setComposedExtensions(nil) })
	setComposedExtensions([]extension.Extension{{Name: unit, Version: "1.0.0", Secrets: []extension.SecretsRequest{
		{Key: "api-key", Scope: extension.SecretScopeWorkspace},
		{Key: "own-token", Scope: extension.SecretScopeUser},
	}}})
	list := func(ctx context.Context) *[]crmcontracts.ComposedExtensionSetupItem {
		t.Helper()
		rec := httptest.NewRecorder()
		extensionsHandlers{pool: e.Pool}.ListExtensions(rec, httptest.NewRequest(http.MethodGet, "/v1/extensions", nil).WithContext(ctx))
		var got crmcontracts.ExtensionDirectory
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || len(got.Extensions) != 1 {
			t.Fatalf("list → %d %s (decode err %v)", rec.Code, rec.Body.String(), err)
		}
		return got.Extensions[0].SetupMissing
	}
	// An operator who reads the inventory but is no admin is not told which keys exist.
	operator := principal.WithActor(e.Admin(), principal.Principal{
		Type: principal.PrincipalHuman, Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"extension_access": {Read: true}}},
	})
	if got := list(operator); got != nil {
		t.Errorf("a non-admin reader was told %+v, want setup_missing left out", *got)
	}

	want := []crmcontracts.ComposedExtensionSetupItem{{Kind: crmcontracts.ComposedExtensionSetupItemKindWorkspaceSecret, Name: "api-key"}}
	if got := list(e.Admin()); got == nil || !slices.Equal(*got, want) {
		t.Fatalf("before the key is set, missing = %v, want %+v", got, want)
	}
	if err := extsecrets.For(unit, e.Pool, e.vault).Put(e.callCtx(e.WS), "api-key", []byte("sk_probe")); err != nil {
		t.Fatalf("storing the key: %v", err)
	}
	if got := list(e.Admin()); got == nil || len(*got) != 0 {
		t.Errorf("after the key is set, missing = %v, want an empty list: the unit is ready", got)
	}
}
