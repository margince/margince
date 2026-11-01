// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A restore brings a contact's employment edges back with it, over a real
// Postgres and through the restore the bulk and inverse writers call.

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRestoringAContactFilesItsSameEmployerTwin(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	acme := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme Robotics GmbH", nil))
	archived := ids.From[ids.ContactKind](e.SeedContact(t, "Marlene Wiesinger", nil))
	employThroughTheWriter(t, e, archived, acme)
	if _, err := e.Contacts.ArchiveContact(ctx, archived, nil); err != nil {
		t.Fatalf("archiving the first contact: %v", err)
	}

	// The twin arrives while the first is archived, so its own duplicate check
	// cannot have seen the pair the restore goes on to create.
	twin := ids.From[ids.ContactKind](e.SeedContact(t, "Marlene Wiesingr", nil))
	employThroughTheWriter(t, e, twin, acme)
	if pairs := pairsNaming(t, e, archived); len(pairs) != 0 {
		t.Fatalf("got %d pairs while the contact was archived, want 0", len(pairs))
	}

	if err := restoreIn(t, e, func(tx pgx.Tx) (storekit.RestoreReport, error) {
		return e.Contacts.RestoreContactTx(ctx, tx, archived, nil, unarchiveWith(nil))
	}); err != nil {
		t.Fatalf("restoring the contact: %v", err)
	}

	if pairs := pairsNaming(t, e, archived); len(pairs) != 1 {
		t.Fatalf("got pairs %+v, want the one pair with the live twin at the employer", pairs)
	}
}

// employThroughTheWriter records employment the way the app does, so the edge
// carries the duplicate check a raw insert would skip.
func employThroughTheWriter(t *testing.T, e *integration.Env, contact ids.ContactID, employer ids.CompanyID) {
	t.Helper()
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contact, CompanyID: &employer, Source: "manual",
	}); err != nil {
		t.Fatalf("employing %s: %v", contact, err)
	}
}

func pairsNaming(t *testing.T, e *integration.Env, contact ids.ContactID) []contacts.DedupeCandidateRow {
	t.Helper()
	pairs, err := e.Contacts.OpenCandidatesNaming(e.Admin(), "contact", contact.UUID, nil)
	if err != nil {
		t.Fatalf("reading the duplicate queue: %v", err)
	}
	return pairs
}
