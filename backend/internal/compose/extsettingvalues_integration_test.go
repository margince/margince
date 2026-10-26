// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestUnitSettingValuesAreAdminOnlyAuditedAndKeepEachUnitsOwn(t *testing.T) {
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
