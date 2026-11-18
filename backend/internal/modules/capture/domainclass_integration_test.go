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
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// domainSeat is a signed-in admin who may change capture settings and read
// companies and contacts, so a company's name may be said to them.
func domainSeat(ctx context.Context, t *testing.T, db *database.DB) context.Context {
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
				"company":          {Read: true},
				"contact":          {Read: true},
			},
			RowScope: principal.RowScopeAll,
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
