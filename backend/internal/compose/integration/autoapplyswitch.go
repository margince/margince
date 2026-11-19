// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SetAutoApplyEnabled flips the installation's automatic-application switch as
// an admin does, through its real writer, and puts it back on when the test ends.
func SetAutoApplyEnabled(t *testing.T, e *Env, on bool) {
	t.Helper()
	cfg := configtest.StoredOver(t, e.Pool, approvals.ConfigSettings())
	t.Cleanup(func() {
		if err := approvals.SetAutoApplyEnabled(e.AdminWithRole(), cfg, true); err != nil {
			t.Errorf("putting the switch back on: %v", err)
		}
	})
	if err := approvals.SetAutoApplyEnabled(e.AdminWithRole(), cfg, on); err != nil {
		t.Fatalf("switching the installation to %v: %v", on, err)
	}
}

// AdminWithRole is the admin seat with the admin role key, which the
// installation switches ask for.
func (e *Env) AdminWithRole() context.Context {
	return e.As(e.AdminUser, nil, principal.Permissions{
		RoleKeys: []string{roleAdmin}, RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
	})
}
