// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// ignoreInstallationName stands in for identity's setting, which this module
// cannot import; compose's suites prove the real rename.
func ignoreInstallationName(context.Context, pgx.Tx, string) error { return nil }

// A store composed without the renamer refuses a human renaming the anchor,
// and the rename rolls back rather than leaving the installation name behind.
func TestAnUnwiredStoreRefusesToRenameTheAnchor(t *testing.T) {
	env := newAnchorEnv(t)
	unwired := NewStore(env.store.db)

	_, err := unwired.SaveCompany(env.ctx, SaveCompanyInput{DisplayName: "Renamed Without The Installation"})
	if !errors.Is(err, errInstallationRenameUnwired) {
		t.Fatalf("SaveCompany through an unwired store = %v, want errInstallationRenameUnwired", err)
	}
	company, err := env.store.GetAnchorCompany(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if company.DisplayName != "Our Company" {
		t.Errorf("the refused rename left the anchor named %q, want %q", company.DisplayName, "Our Company")
	}

	// A customer company is not the installation's, so the same store renames it.
	customer := env.newCompany(t)
	name := "Brandt Renamed GmbH"
	if _, err := unwired.UpdateCompany(env.ctx, customer, UpdateCompanyInput{DisplayName: &name}); err != nil {
		t.Errorf("renaming a customer through an unwired store = %v, want it to pass", err)
	}
}
