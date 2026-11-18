// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// What a domain is to the workspace, asked by the consumer-mail list and the
// seat's other-address list, over a real Postgres.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// domainSeat is a signed-in admin who may change capture settings and read
// companies and contacts, so a company's name may be said to them.
func domainSeat(ctx context.Context, t *testing.T, db *database.DB) context.Context {
	t.Helper()
	return domainSeatWith(ctx, t, db, principal.RowScopeAll, true)
}

// domainSeatWith is domainSeat with the row scope and the company grant named.
func domainSeatWith(ctx context.Context, t *testing.T, db *database.DB, scope principal.RowScope, readsCompanies bool) context.Context {
	t.Helper()
	seat := ids.NewV7()
	if err := database.WithWorkspaceTx(ctx, db.Pool(), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO app_user (id, email, display_name, status)
			VALUES ($1, $2, 'Domain Admin', 'active')`, seat, "domains-"+seat.String()+"@example.test")
		return err
	}); err != nil {
		t.Fatalf("seeding the seat: %v", err)
	}
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"capture_settings": {Create: true, Read: true, Update: true},
				"company":          {Read: readsCompanies},
				"contact":          {Read: true},
			},
			RowScope: scope,
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// seedDemoCustomer files the customer Demo GmbH with the domain demo.test and
// the contact alice@demo.test.
func seedDemoCustomer(ctx context.Context, t *testing.T, db *database.DB) {
	t.Helper()
	if err := database.WithWorkspaceTx(ctx, db.Pool(), func(tx pgx.Tx) error {
		var company, contact ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO company (display_name, source, captured_by)
			VALUES ('Demo GmbH', 'manual', 'human:test') RETURNING id`).Scan(&company); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by)
			VALUES ($1, 'demo.test', true, 'manual', 'human:test')`, company); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO contact (full_name, visibility, source, captured_by)
			VALUES ('Alice Demo', 'workspace', 'manual', 'human:test') RETURNING id`).Scan(&contact); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
			VALUES ($1, 'alice@demo.test', true, 'manual', 'human:test')`, contact)
		return err
	}); err != nil {
		t.Fatalf("seeding the customer: %v", err)
	}
}

func wantRefusal(t *testing.T, err error, status int, code string) *httperr.DetailedError {
	t.Helper()
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || refusal.Status != status {
		t.Fatalf("want %d %s, got %v", status, code, err)
	}
	got := refusal.Code
	if len(refusal.Fields) == 1 {
		got = refusal.Fields[0].Code
	}
	if got != code {
		t.Fatalf("want %d %s, got code %s", status, code, got)
	}
	return refusal
}

func domainFixture(t *testing.T) (context.Context, *database.DB) {
	t.Helper()
	ctx, db := bootstrapInternalMailWorkspace(t, "ours.test")
	seedDemoCustomer(ctx, t, db)
	return domainSeat(ctx, t, db), db
}

func TestConsumerMailListRefusesOwnDomainAndShippedDomainAndAsksForACompany(t *testing.T) {
	ctx, db := domainFixture(t)
	store := capture.NewFreemailDomains(db)

	_, err := store.AddConfirmed(ctx, "ours.test", capture.FreemailKindExtra, true)
	wantRefusal(t, err, http.StatusUnprocessableEntity, capture.CodeOwnDomain)

	_, err = store.AddConfirmed(ctx, "gmail.com", capture.FreemailKindExtra, true)
	refusal := wantRefusal(t, err, http.StatusUnprocessableEntity, capture.CodeAlreadyShipped)
	if refusal.Detail != "gmail.com is already on the shipped list" {
		t.Errorf("detail = %q", refusal.Detail)
	}

	_, err = store.Add(ctx, "demo.test", capture.FreemailKindExtra)
	question := wantRefusal(t, err, http.StatusConflict, capture.CodeNeedsConfirmation)
	if question.Details["company_name"] != "Demo GmbH" {
		t.Errorf("the question names %v, want Demo GmbH", question.Details["company_name"])
	}
	entries, err := store.List(ctx)
	if err != nil || len(entries) != 0 {
		t.Fatalf("a refused or unanswered entry stored something: %v %v", entries, err)
	}

	if _, err := store.AddConfirmed(ctx, "demo.test", capture.FreemailKindExtra, true); err != nil {
		t.Fatalf("the confirmed entry: %v", err)
	}
	if again, err := store.Add(ctx, "demo.test", capture.FreemailKindExtra); err != nil || again.Domain != "demo.test" {
		t.Errorf("re-adding an added domain asked again: %v %v", again, err)
	}
	if _, err := store.Add(ctx, "ours.test", capture.FreemailKindNever); err != nil {
		t.Errorf("a carve-out of an own domain must stay possible: %v", err)
	}
}

func TestOtherAddressesRefuseConsumerDomainAndAskForCustomerValues(t *testing.T) {
	ctx, db := domainFixture(t)
	store := capture.NewOwnerIdentityStore(db)

	_, err := store.AddConfirmed(ctx, capture.IdentityKindDomain, "gmail.com", true)
	wantRefusal(t, err, http.StatusUnprocessableEntity, capture.CodeConsumerMailDomain)

	_, err = store.Add(ctx, capture.IdentityKindDomain, "demo.test")
	question := wantRefusal(t, err, http.StatusConflict, capture.CodeNeedsConfirmation)
	if question.Details["company_name"] != "Demo GmbH" {
		t.Errorf("the question names %v, want Demo GmbH", question.Details["company_name"])
	}
	_, err = store.Add(ctx, capture.IdentityKindAddress, "Alice@demo.test")
	question = wantRefusal(t, err, http.StatusConflict, capture.CodeNeedsConfirmation)
	if question.Details["role"] != string(capture.DomainRoleContactAddress) {
		t.Errorf("role = %v", question.Details["role"])
	}
	listed, err := store.List(ctx)
	if err != nil || len(listed) != 0 {
		t.Fatalf("an unanswered question stored something: %v %v", listed, err)
	}

	for _, claim := range []struct{ kind, value string }{
		{capture.IdentityKindDomain, "demo.test"},
		{capture.IdentityKindAddress, "alice@demo.test"},
		{capture.IdentityKindAddress, "me@gmail.com"},
		{capture.IdentityKindDomain, "private.example"},
	} {
		confirmed := claim.value == "demo.test" || claim.value == "alice@demo.test"
		if _, err := store.AddConfirmed(ctx, claim.kind, claim.value, confirmed); err != nil {
			t.Errorf("%s %s: %v", claim.kind, claim.value, err)
		}
	}
}

func TestOwnDomainListStillRefusesConsumerMail(t *testing.T) {
	ctx, db := domainFixture(t)
	_, err := capture.NewOwnDomainStore(db).Add(ctx, "gmail.com")
	if !errors.Is(err, capture.ErrConsumerMailDomain) {
		t.Fatalf("want ErrConsumerMailDomain, got %v", err)
	}
	if _, err := capture.NewFreemailDomains(db).Add(ctx, "regional.example", capture.FreemailKindExtra); err != nil {
		t.Fatal(err)
	}
	_, err = capture.NewOwnDomainStore(db).Add(ctx, "regional.example")
	if !errors.Is(err, capture.ErrConsumerMailDomain) {
		t.Fatalf("an added consumer domain must be refused as own: %v", err)
	}
}

func TestAnUnreadableCompanyIsNeitherNamedNorAskedAbout(t *testing.T) {
	base, db := bootstrapInternalMailWorkspace(t, "ours.test")
	seedDemoCustomer(base, t, db)
	identities := capture.NewOwnerIdentityStore(db)

	noGrant := domainSeatWith(base, t, db, principal.RowScopeAll, false)
	if _, err := identities.Add(noGrant, capture.IdentityKindDomain, "demo.test"); err != nil {
		t.Fatalf("a seat that cannot read companies was asked about one: %v", err)
	}
	if _, err := capture.NewFreemailDomains(db).Add(noGrant, "demo.test", capture.FreemailKindExtra); err != nil {
		t.Fatalf("a seat that cannot read companies was asked about one on the consumer list: %v", err)
	}
}

func TestACompanyOutsideTheSeatsRowScopeIsNeitherNamedNorAskedAbout(t *testing.T) {
	base, db := bootstrapInternalMailWorkspace(t, "ours.test")
	if err := database.WithWorkspaceTx(base, db.Pool(), func(tx pgx.Tx) error {
		var owner, company ids.UUID
		if err := tx.QueryRow(base, `
			INSERT INTO app_user (email, display_name, status)
			VALUES ('owner@example.test', 'Owner', 'active') RETURNING id`).Scan(&owner); err != nil {
			return err
		}
		if err := tx.QueryRow(base, `
			INSERT INTO company (display_name, owner_id, visibility, source, captured_by)
			VALUES ('Private GmbH', $1, 'owner', 'manual', 'human:test') RETURNING id`, owner).Scan(&company); err != nil {
			return err
		}
		_, err := tx.Exec(base, `
			INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by)
			VALUES ($1, 'private-gmbh.test', true, 'manual', 'human:test')`, company)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	narrow := domainSeatWith(base, t, db, principal.RowScopeOwn, true)
	if _, err := capture.NewOwnerIdentityStore(db).Add(narrow, capture.IdentityKindDomain, "private-gmbh.test"); err != nil {
		t.Fatalf("a company outside the row scope was asked about: %v", err)
	}
}

func TestACompanyOnAParentOrChildDomainIsAskedAbout(t *testing.T) {
	ctx, db := domainFixture(t)
	store := capture.NewOwnerIdentityStore(db)

	_, err := store.Add(ctx, capture.IdentityKindDomain, "shop.demo.test")
	wantRefusal(t, err, http.StatusConflict, capture.CodeNeedsConfirmation)

	if err := database.WithWorkspaceTx(ctx, db.Pool(), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by)
			SELECT id, 'eu.regional.test', false, 'manual', 'human:test'
			  FROM company WHERE display_name = 'Demo GmbH'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, err = store.Add(ctx, capture.IdentityKindDomain, "regional.test")
	question := wantRefusal(t, err, http.StatusConflict, capture.CodeNeedsConfirmation)
	if question.Details["company_name"] != "Demo GmbH" {
		t.Errorf("the child-domain question names %v", question.Details["company_name"])
	}
}

func TestAContactAddressIsFoundInEitherSpellingOfItsDomain(t *testing.T) {
	ctx, db := domainFixture(t)
	if err := database.WithWorkspaceTx(ctx, db.Pool(), func(tx pgx.Tx) error {
		var contact ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO contact (full_name, visibility, source, captured_by)
			VALUES ('Berta Buch', 'workspace', 'manual', 'human:test') RETURNING id`).Scan(&contact); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
			VALUES ($1, 'berta@bücher.example', true, 'manual', 'human:test')`, contact)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	store := capture.NewOwnerIdentityStore(db)
	for _, address := range []string{"berta@bücher.example", "berta@xn--bcher-kva.example", "BERTA@Bücher.Example"} {
		_, err := store.Add(ctx, capture.IdentityKindAddress, address)
		wantRefusal(t, err, http.StatusConflict, capture.CodeNeedsConfirmation)
	}
}

func TestAPunycodeContactAddressIsFoundByItsUnicodeSpelling(t *testing.T) {
	ctx, db := domainFixture(t)
	if err := database.WithWorkspaceTx(ctx, db.Pool(), func(tx pgx.Tx) error {
		var contact ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO contact (full_name, visibility, source, captured_by)
			VALUES ('Carl Buch', 'workspace', 'manual', 'human:test') RETURNING id`).Scan(&contact); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
			VALUES ($1, 'carl@xn--bcher-kva.example', true, 'manual', 'human:test')`, contact)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, err := capture.NewOwnerIdentityStore(db).Add(ctx, capture.IdentityKindAddress, "carl@bücher.example")
	wantRefusal(t, err, http.StatusConflict, capture.CodeNeedsConfirmation)
}

func TestARestoredIdentityIsJudgedAgain(t *testing.T) {
	ctx, db := domainFixture(t)
	store := capture.NewOwnerIdentityStore(db)
	declared, err := store.Add(ctx, capture.IdentityKindDomain, "later.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(ctx, declared.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := capture.NewFreemailDomains(db).Add(ctx, "later.example", capture.FreemailKindExtra); err != nil {
		t.Fatal(err)
	}
	_, err = store.RestoreRemoval(ctx, declared.ID)
	wantRefusal(t, err, http.StatusUnprocessableEntity, capture.CodeConsumerMailDomain)

	asked, err := store.Add(ctx, capture.IdentityKindAddress, "kept@home.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(ctx, asked.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(ctx, db.Pool(), func(tx pgx.Tx) error {
		var contact ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO contact (full_name, visibility, source, captured_by)
			VALUES ('Home Contact', 'workspace', 'manual', 'human:test') RETURNING id`).Scan(&contact); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
			VALUES ($1, 'kept@home.example', true, 'manual', 'human:test')`, contact)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, err = store.RestoreRemoval(ctx, asked.ID)
	wantRefusal(t, err, http.StatusConflict, capture.CodeNeedsConfirmation)
}

// waitsOnAnAdvisoryLock reports whether some other session is queued on an
// advisory lock, or finished first; it polls on tx, which holds the lock.
func waitsOnAnAdvisoryLock(ctx context.Context, t *testing.T, tx pgx.Tx, finished <-chan struct{}) bool {
	t.Helper()
	for range 20000 {
		select {
		case <-finished:
			return false
		default:
		}
		var waiting bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype = 'advisory' AND NOT granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return true
		}
	}
	return false
}

// The consumer-list writer queues behind a company-domain writer that holds the
// domain, so one of them sees the other's row.
func TestTheConsumerListWaitsForTheCompanyDomainWriter(t *testing.T) {
	ctx, db := domainFixture(t)
	finished := make(chan struct{})
	var addErr error
	waited := false
	err := database.WithWorkspaceTx(ctx, db.Pool(), func(tx pgx.Tx) error {
		if err := storekit.LockConsumerDomain(ctx, tx, "mail.race-provider.example"); err != nil {
			return err
		}
		go func() {
			defer close(finished)
			_, addErr = capture.NewFreemailDomains(db).Add(ctx, "race-provider.example", capture.FreemailKindExtra)
		}()
		waited = waitsOnAnAdvisoryLock(ctx, t, tx, finished)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-finished
	if !waited {
		t.Error("the consumer-list write did not wait for the lock a company-domain write holds")
	}
	if addErr != nil {
		t.Errorf("the queued write failed once released: %v", addErr)
	}
}
