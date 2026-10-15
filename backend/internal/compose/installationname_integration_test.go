// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The company's name is the installation's. When a boot source sets it, the
// company profile may save that name and no other.
func TestABootSetNameRefusesAnotherCompanyName(t *testing.T) {
	e := integration.Setup(t)
	locked := bootConfigForTest(t, config.RoleAPI, "MARGINCE_INSTALLATION_NAME=Locked Co").
		WithDatabase(NewSettingsStore(e.Pool))
	store := contacts.NewStore(e.DB()).WithInstallationRenamer(installationRenamerUnder(locked))
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	if _, err := store.SaveCompany(ctx, contacts.SaveCompanyInput{DisplayName: "Acme GmbH"}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("naming the company other than the boot-set name = %v, want a conflict", err)
	}
	if _, err := store.SaveCompany(ctx, contacts.SaveCompanyInput{DisplayName: "Locked Co"}); err != nil {
		t.Fatalf("naming the company the boot-set name = %v, want it saved", err)
	}
}
