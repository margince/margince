// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
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
