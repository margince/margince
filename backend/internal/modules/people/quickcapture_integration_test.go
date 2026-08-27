// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Quick capture writes three rows or none: the person, the employer they were
// captured for, and the employment edge between them.
//
// The all-or-nothing half is the one worth a real database. A unit test can see
// that the code calls three writers; only Postgres can show that a refusal in
// the third leaves no person behind, because what rolls it back is the
// transaction rather than anything in the Go.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestQuickCaptureWritesThePersonAndTheirEmployer(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	title := "VP Finance"
	orgName := "Acme Quick GmbH"
	profile := "https://linkedin.com/in/dana-quick"
	email := "dana@acme-quick.test"
	out, err := e.store.QuickCapture(ctx, QuickCaptureInput{
		FullName:         "Dana Quick",
		Title:            &title,
		OrganizationName: &orgName,
		ProfileURL:       &profile,
		Email:            &email,
	})
	if err != nil {
		t.Fatalf("quick capture: %v", err)
	}

	if out.Person.FullName != "Dana Quick" {
		t.Errorf("full name = %q, want %q", out.Person.FullName, "Dana Quick")
	}
	if out.OrganizationID == nil {
		t.Fatal("no employer attached, but a company name was given")
	}
	if !out.OrganizationCreated {
		t.Error("organization_created is false for a company this call created")
	}

	// The employment edge is the part two separate calls would lose.
	assertEmployedAt(
		ctx, t, e,
		ids.From[ids.PersonKind](ids.UUID(out.Person.Id)),
		*out.OrganizationID,
	)

	// The profile address is stored as stated, under the key every reader of a
	// person's profile link already looks at.
	if got := statedProfileURL(out.Person.Social); got != profile {
		t.Errorf("stored profile url = %q, want %q", got, profile)
	}
}

func TestQuickCaptureAttachesAnExistingEmployer(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	incumbent, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Existing Employer AG", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(incumbent.Id))

	// The name is deliberately WRONG and present: an id the caller picked from
	// the list answers the question the name was only guessing at, so a typo
	// beside it must not create a second company.
	wrongName := "Existng Employer"
	out, err := e.store.QuickCapture(ctx, QuickCaptureInput{
		FullName:         "Sam Second",
		OrganizationID:   &orgID,
		OrganizationName: &wrongName,
	})
	if err != nil {
		t.Fatalf("quick capture: %v", err)
	}
	if out.OrganizationID == nil || out.OrganizationID.UUID != ids.UUID(incumbent.Id) {
		t.Fatalf("attached org = %v, want the incumbent %v", out.OrganizationID, incumbent.Id)
	}
	if out.OrganizationCreated {
		t.Error("organization_created is true for a company that already existed")
	}
	if got := countOrganizationsNamed(ctx, t, e, wrongName); got != 0 {
		t.Errorf("the misspelled name created %d organization(s), want 0", got)
	}
}

func TestQuickCaptureWithoutAnEmployerIsStillAPerson(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	blank := "   "
	out, err := e.store.QuickCapture(ctx, QuickCaptureInput{
		FullName: "Lone Contact",
		// Whitespace is a box the reader left alone, not a company called "".
		OrganizationName: &blank,
	})
	if err != nil {
		t.Fatalf("quick capture: %v", err)
	}
	if out.OrganizationID != nil {
		t.Errorf("attached an employer %v for a blank company name", out.OrganizationID)
	}
	if out.Person.FullName != "Lone Contact" {
		t.Errorf("full name = %q, want %q", out.Person.FullName, "Lone Contact")
	}
}

// A seat that may create a person but not an organization gets NEITHER. The
// refusal has to roll the person back, or the list keeps a contact the reader
// was told did not save.
func TestQuickCaptureLeavesNoPersonWhenTheEmployerIsRefused(t *testing.T) {
	e := setupDedupe(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"person":       {Create: true, Read: true, Update: true},
				"relationship": {Create: true, Read: true},
				// organization: absent. Creating the employer is refused.
			},
			RowScope: principal.RowScopeAll,
		},
	})

	orgName := "Refused Employer GmbH"
	if _, err := e.store.QuickCapture(ctx, QuickCaptureInput{
		FullName:         "Rolled Back",
		OrganizationName: &orgName,
	}); err == nil {
		t.Fatal("quick capture succeeded without the organization grant")
	}

	if got := countPeopleNamed(ctx, t, e, "Rolled Back"); got != 0 {
		t.Errorf("%d person row(s) survived a refused capture, want 0", got)
	}
}

func assertEmployedAt(
	ctx context.Context,
	t *testing.T,
	e *dedupeEnv,
	personID ids.PersonID,
	orgID ids.OrganizationID,
) {
	t.Helper()
	var found int
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM relationship
			 WHERE kind = 'employment' AND person_id = $1 AND organization_id = $2
			   AND archived_at IS NULL`, personID, orgID).Scan(&found)
	})
	if err != nil {
		t.Fatalf("reading the employment edge: %v", err)
	}
	if found != 1 {
		t.Errorf("employment edges = %d, want 1", found)
	}
}

func countOrganizationsNamed(ctx context.Context, t *testing.T, e *dedupeEnv, name string) int {
	t.Helper()
	var found int
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM organization WHERE display_name = $1`, name).Scan(&found)
	})
	if err != nil {
		t.Fatalf("counting organizations: %v", err)
	}
	return found
}

func countPeopleNamed(ctx context.Context, t *testing.T, e *dedupeEnv, name string) int {
	t.Helper()
	var found int
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM person WHERE full_name = $1`, name).Scan(&found)
	})
	if err != nil {
		t.Fatalf("counting people: %v", err)
	}
	return found
}

func statedProfileURL(social *map[string]any) string {
	if social == nil {
		return ""
	}
	value, ok := (*social)[profileURLField].(string)
	if !ok {
		return ""
	}
	return value
}
