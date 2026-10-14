// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A stored logo and its provenance arrive together. Nothing stopped a row
// holding a key with no origin, and the one question asked about a logo
// somebody disputes is where it came from.

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func TestACompanyLogoKeyWithoutAnOriginIsRefused(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	companyID := seedLogoCompany(ctx, t, e, "Ostsee Werft AG", "ostsee.test")

	for _, tc := range []struct {
		name       string
		statement  string
		constraint string
	}{
		{
			name:       "the wide logo",
			statement:  `UPDATE company SET logo_object_key = 'logos/orphan.png' WHERE id = $1`,
			constraint: "company_logo_names_its_origin",
		},
		{
			name:       "the icon",
			statement:  `UPDATE company SET logo_icon_object_key = 'logos/orphan-icon.png' WHERE id = $1`,
			constraint: "company_logo_icon_names_its_origin",
		},
		// The other direction: an origin with no key is the same half-written
		// row read from the other side, and names the same pair.
		{
			name:       "an origin with no key",
			statement:  `UPDATE company SET logo_origin = 'upload' WHERE id = $1`,
			constraint: "company_logo_names_its_origin",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := e.store.tx(ctx, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, tc.statement, companyID)
				return err
			})
			if err == nil {
				t.Fatal("a half-written logo was accepted, so a stored image can have no stated origin")
			}
			name, ok := storekit.CheckViolation(err)
			if !ok {
				t.Fatalf("got %v, want a check violation the error classifier can answer 422 for", err)
			}
			if name != tc.constraint {
				t.Errorf("constraint = %q, want %q — the name is what tells a reader which pair is half-written",
					name, tc.constraint)
			}
		})
	}
}

// Both halves together are accepted, which is what the constraint is for: it
// refuses a missing origin, not a logo.
func TestACompanyLogoWithItsOriginIsAccepted(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	companyID := seedLogoCompany(ctx, t, e, "Nordsee Werft AG", "nordsee.test")

	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE company
			   SET logo_object_key = 'logos/whole.png', logo_origin = 'upload',
			       logo_icon_object_key = 'logos/whole-icon.png', logo_icon_origin = 'upload'
			 WHERE id = $1`, companyID)
		return err
	}); err != nil {
		t.Fatalf("a logo written with its origin was refused: %v", err)
	}
}
