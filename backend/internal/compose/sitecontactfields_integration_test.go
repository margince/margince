// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Auto-filling a site contact onto an employee the workspace already has
// (ADR-0072 phase 4B): who counts as unmistakably the same contact, what is
// written when they are, and every case that still stages a lead instead.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatEmployee plants one contact employed by company, optionally with an email.
func seatEmployee(t *testing.T, e *integration.Env, company ids.UUID, fullName, email string) ids.UUID {
	t.Helper()
	contact := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `
			INSERT INTO contact (id, full_name, source, captured_by)
			VALUES ($1, $2, 'gmail:seed', 'connector:gmail')`, contact, fullName); err != nil {
			return err
		}
		if email != "" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO contact_email (contact_id, email, email_type, is_primary, source, captured_by)
				VALUES ($1, $2, 'work', true, 'gmail:seed', 'connector:gmail')`, contact, email); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO relationship (kind, contact_id, company_id, is_current_primary, source, captured_by)
			VALUES ('employment', $1, $2, true, 'gmail:seed', 'connector:gmail')`, contact, company)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return contact
}

func seatedTitle(t *testing.T, e *integration.Env, contact ids.UUID) *string {
	t.Helper()
	var title *string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT title FROM contact WHERE id = $1`, contact).Scan(&title)
	})
	if err != nil {
		t.Fatal(err)
	}
	return title
}

func TestApplySiteContactFieldsMatchesAnEmployeeByName(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	company := e.SeedCompany(t, "Acme", nil)
	contact := seatEmployee(t, e, company, "Bob Builder", "")

	matched, err := store.ApplySiteContactFields(e.Admin(), ids.From[ids.CompanyKind](company),
		contacts.SiteContactFields{
			Name: "Bob Builder", Role: "Head of Delivery",
			EvidenceSnippet: "Bob Builder — Head of Delivery",
			SourceURL:       "https://acme.example/team",
		})
	if err != nil {
		t.Fatalf("ApplySiteContactFields: %v", err)
	}
	if !matched {
		t.Fatal("the site contact names an employee of this company and was not matched")
	}
	if got := seatedTitle(t, e, contact); got == nil || *got != "Head of Delivery" {
		t.Fatalf("title = %v, want the role the site published", got)
	}
	// The evidence is what makes the value auditable back to the page.
	if n := e.WsCount(t, `
		SELECT count(*) FROM contact_profile_field
		 WHERE contact_id = $1 AND field = 'role' AND source = 'site_read'`, contact); n != 1 {
		t.Fatalf("%d role evidence rows, want 1", n)
	}

	t.Run("a re-read applies nothing twice", func(t *testing.T) {
		matched, err := store.ApplySiteContactFields(e.Admin(), ids.From[ids.CompanyKind](company),
			contacts.SiteContactFields{
				Name: "Bob Builder", Role: "Chief of Everything",
				EvidenceSnippet: "Bob Builder — Chief of Everything",
				SourceURL:       "https://acme.example/team",
			})
		if err != nil || !matched {
			t.Fatalf("second read: matched=%v err=%v", matched, err)
		}
		if got := seatedTitle(t, e, contact); got == nil || *got != "Head of Delivery" {
			t.Fatalf("title = %v — the first answer must stand", got)
		}
	})
}

func TestApplySiteContactFieldsRefusesToGuess(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	company := e.SeedCompany(t, "Acme", nil)
	seatEmployee(t, e, company, "Chris Taylor", "")
	seatEmployee(t, e, company, "Chris Taylor", "")

	t.Run("two employees of the same name are not identifiable", func(t *testing.T) {
		matched, err := store.ApplySiteContactFields(e.Admin(), ids.From[ids.CompanyKind](company),
			contacts.SiteContactFields{
				Name: "Chris Taylor", Role: "Engineer",
				EvidenceSnippet: "Chris Taylor — Engineer", SourceURL: "https://acme.example/team",
			})
		if err != nil {
			t.Fatalf("ApplySiteContactFields: %v", err)
		}
		if matched {
			t.Fatal("an ambiguous name was matched — the lead must stage instead")
		}
	})

	t.Run("a stranger on the team page is not matched", func(t *testing.T) {
		matched, err := store.ApplySiteContactFields(e.Admin(), ids.From[ids.CompanyKind](company),
			contacts.SiteContactFields{
				Name: "Someone Entirely Else", Role: "Engineer",
				EvidenceSnippet: "Someone Entirely Else — Engineer", SourceURL: "https://acme.example/team",
			})
		if err != nil {
			t.Fatalf("ApplySiteContactFields: %v", err)
		}
		if matched {
			t.Fatal("a stranger was matched — strangers stay staged (NEVER-8)")
		}
	})
}

func TestApplySiteContactFieldsStaysInsideTheCompany(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	acme := e.SeedCompany(t, "Acme", nil)
	other := e.SeedCompany(t, "Other", nil)
	contact := seatEmployee(t, e, other, "Dana Reed", "dana@other.example")

	// Acme's site publishes Dana. The CRM records Dana at OTHER, so the two
	// claims disagree about where she works — a human's call, not a sweep's.
	matched, err := store.ApplySiteContactFields(e.Admin(), ids.From[ids.CompanyKind](acme),
		contacts.SiteContactFields{
			Name: "Dana Reed", Role: "CTO", PublishedEmail: "dana@other.example",
			EvidenceSnippet: "Dana Reed — CTO", SourceURL: "https://acme.example/team",
		})
	if err != nil {
		t.Fatalf("ApplySiteContactFields: %v", err)
	}
	if matched {
		t.Fatal("a contact employed elsewhere was matched from this company's site")
	}
	if got := seatedTitle(t, e, contact); got != nil {
		t.Fatalf("title = %q — another company's site must not fill it", *got)
	}
}

func TestApplySiteContactFieldsNeverTouchesAHumansAnswer(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	company := e.SeedCompany(t, "Acme", nil)
	contact := seatEmployee(t, e, company, "Erin Vance", "erin@acme.example")
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE contact SET title = 'Handwritten Title' WHERE id = $1`, contact)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	matched, err := store.ApplySiteContactFields(e.Admin(), ids.From[ids.CompanyKind](company),
		contacts.SiteContactFields{
			Name: "Erin Vance", Role: "VP Sales", PublishedEmail: "erin@acme.example",
			EvidenceSnippet: "Erin Vance — VP Sales", SourceURL: "https://acme.example/team",
		})
	if err != nil {
		t.Fatalf("ApplySiteContactFields: %v", err)
	}
	if !matched {
		t.Fatal("an exact email match among the company's employees must match")
	}
	if got := seatedTitle(t, e, contact); got == nil || *got != "Handwritten Title" {
		t.Fatalf("title = %v — the human's answer was touched", got)
	}
}

// seatEmployeeOwnedBy plants an employee whose row-scope OWNER is chosen, which
// is what the two tests below turn on. seatEmployee leaves owner_id NULL, and an
// unowned row is shared with everyone — so a probe against it passes for every
// seat and would prove nothing about scope.
func seatEmployeeOwnedBy(t *testing.T, e *integration.Env, company, owner ids.UUID, fullName string) ids.UUID {
	t.Helper()
	contact := seatEmployee(t, e, company, fullName, "")
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE contact SET owner_id = $2 WHERE id = $1`, contact, owner)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return contact
}

// siteContactRepPerms is a team-scoped rep who may read the company and update a
// contact. RepPerms itself is NOT usable here: it carries no `company`
// grant, so the company gate would refuse before the contact probe is ever
// reached and the test would pass for the wrong reason.
var siteContactRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":               {Create: true, Read: true, Update: true},
		"company":               {Create: true, Read: true, Update: true},
		"relationship":          {Create: true, Read: true, Update: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

// The fill writes a contact resolved from the COMPANY's employment edges,
// so the company gate says nothing about it. A caller who may read the
// company but may not write that employee must not change them.
//
// It has to be driven directly rather than through either production caller:
// both are PrincipalSystem, for whom every row-scope probe is inert by
// construction, so a test routed through them would be green whether or not the
// probe existed. The four tests above all use e.Admin() for the same reason and
// are equally unable to see this.
func TestApplySiteContactFieldsWillNotWriteAnEmployeeTheCallerCannotChange(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	// The company is the other team's, shared with nobody — but every seat
	// reads a company, so the company gate passes and the CONTACT probe is
	// what the assertion turns on.
	company := e.SeedCompany(t, "Acme", &e.Rep3)
	theirs := seatEmployeeOwnedBy(t, e, company, e.Rep3, "Bob Builder")
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, siteContactRepPerms)

	matched, err := store.ApplySiteContactFields(rep, ids.From[ids.CompanyKind](company),
		contacts.SiteContactFields{
			Name: "Bob Builder", Role: "Head of Delivery",
			EvidenceSnippet: "Bob Builder — Head of Delivery",
			SourceURL:       "https://acme.example/team",
		})
	if err != nil {
		t.Fatalf("ApplySiteContactFields: %v", err)
	}
	// Skip, not refuse: an employee this caller cannot write is the same case
	// as one the page did not identify, and the lead stages exactly as before.
	// Refusing would abort a whole company's confirmation over one row.
	if matched {
		t.Error("reported a match on an employee outside the caller's write authority — " +
			"the caller now learns the contact exists by watching the write succeed")
	}
	if got := seatedTitle(t, e, theirs); got != nil {
		t.Errorf("title = %q on a contact the caller may not change; want it untouched", *got)
	}
	if n := e.WsCount(t, `
		SELECT count(*) FROM contact_profile_field
		 WHERE contact_id = $1 AND source = 'site_read'`, theirs); n != 0 {
		t.Errorf("%d site_read evidence rows written for an unwritable contact, want 0", n)
	}
	// The audit and the bus event hang off the same path, so a leak there is
	// the same disclosure by another door.
	if n := e.WsCount(t, `
		SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1`,
		theirs); n != 0 {
		t.Errorf("%d audit rows for a write that must not have happened, want 0", n)
	}
}

// The mirror, and the reason it is here: a probe that refused EVERYTHING would
// pass the test above while breaking the feature, and nothing else in this file
// runs under a bounded seat to catch it.
func TestApplySiteContactFieldsStillFillsAnEmployeeTheCallerMayChange(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	mine := seatEmployeeOwnedBy(t, e, company, e.Rep1, "Bob Builder")
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, siteContactRepPerms)

	matched, err := store.ApplySiteContactFields(rep, ids.From[ids.CompanyKind](company),
		contacts.SiteContactFields{
			Name: "Bob Builder", Role: "Head of Delivery",
			EvidenceSnippet: "Bob Builder — Head of Delivery",
			SourceURL:       "https://acme.example/team",
		})
	if err != nil {
		t.Fatalf("ApplySiteContactFields: %v", err)
	}
	if !matched {
		t.Fatal("the caller owns this employee and may change them; the fill was refused anyway")
	}
	if got := seatedTitle(t, e, mine); got == nil || *got != "Head of Delivery" {
		t.Fatalf("title = %v, want the role the site published", got)
	}
}
