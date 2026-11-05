// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Onboarding from a website read ends in this confirmation rather than in
// PUT /company, and the name the human confirms there is the installation's.
func TestConfirmingTheOnboardingReadRenamesTheInstallation(t *testing.T) {
	e := integration.Setup(t)
	ready := onboardingDraft(t, e)
	store := contacts.NewStore(e.DB()).WithInstallationRenamer(installationRenamer(e.Pool))
	engine := &deepReadEngine{contacts: store}
	offer, icp, website := "Employee onboarding software", "Growing RevOps teams", seedURL

	if _, _, err := store.ConfirmCompanySiteRead(e.As(e.Rep1, nil, integration.AdminPerms), contacts.ConfirmCompanySiteReadInput{
		ReadID: ready.ID, DraftVersion: ready.DraftVersion, ProposalHash: ready.ProposalHash,
		DisplayName: "Acme Onboarding GmbH", Website: &website,
		Fields: map[string]*string{"offer_summary": &offer, "icp": &icp},
	}, engine.stageOnboardingContacts); err != nil {
		t.Fatalf("confirm onboarding read: %v", err)
	}

	var name string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		name, err = identity.InstallationNameOf(e.Admin(), tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if name != "Acme Onboarding GmbH" {
		t.Errorf("installation name = %q after the onboarding confirmation, want %q", name, "Acme Onboarding GmbH")
	}
}

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
