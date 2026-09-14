// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A company name is the field that turns every other record into an
// identifiable one, and row scope alone does not keep it back: under
// row_scope=all the row half admits every account, so a surface that takes only
// the row half hands the customer list to a seat whose role grants no company
// read at all.
//
// These are the surfaces that did exactly that. Each is asked by a seat holding
// everything ELSE it needs, so what the test isolates is the company grant and
// nothing else about the caller.

// withoutCompanyRead grants a full-scope seat every object these surfaces need
// and deliberately omits `company`.
//
// RowScopeAll on purpose: it is the configuration that makes the row half a
// no-op, so a fixture that scoped rows down would pass for the wrong reason —
// the read would be empty because of scope, and the object half could go on
// being absent unnoticed.
func withoutCompanyRead(e *Env, user ids.UUID, objects ...string) context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, o := range objects {
		if o == "company" {
			panic("withoutCompanyRead was handed the company grant it exists to withhold")
		}
		grants[o] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	}
	return e.As(user, nil, principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll})
}

// The companies on a project are named by display_name, so the section is
// withheld from a seat that may not read companies — rather than the project
// being refused, which would take a record away over one band of it.
func TestAProjectsCompanySectionIsWithheldWithoutTheCompanyGrant(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	customer := e.SeedCompany(t, "Acme Werke", nil)
	partner := e.SeedCompany(t, "Beta Systeme", nil)
	p := seedProject(admin, t, e, "Joint rollout", customer, nil)

	if _, err := e.Contacts.SetProjectCompany(admin, contacts.SetProjectCompanyInput{
		ProjectID: p.ID, CompanyID: companyIDOf(partner), Role: "partner",
	}); err != nil {
		t.Fatalf("put the partner on the project: %v", err)
	}

	// The same write, by a seat holding the two grants SetProjectCompany asks
	// for and no company grant. The write is theirs to make; the names are not
	// theirs to read.
	narrow := withoutCompanyRead(e, e.AdminUser, "relationship", "project")
	got, err := e.Contacts.SetProjectCompany(narrow, contacts.SetProjectCompanyInput{
		ProjectID: p.ID, CompanyID: companyIDOf(partner), Role: "partner",
	})
	if err != nil {
		t.Fatalf("the write itself must still be admitted: %v", err)
	}
	for _, one := range got {
		t.Errorf("a seat with no company grant was handed %q (%s) on the project",
			one.DisplayName, one.CompanyID)
	}
}

// And the section is still there for a seat that DOES hold the grant, which is
// what stops the guard above from being satisfied by an empty list for every
// caller.
func TestAProjectsCompanySectionIsServedWithTheCompanyGrant(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	customer := e.SeedCompany(t, "Acme Werke", nil)
	p := seedProject(admin, t, e, "Joint rollout", customer, nil)
	partner := e.SeedCompany(t, "Beta Systeme", nil)

	got, err := e.Contacts.SetProjectCompany(admin, contacts.SetProjectCompanyInput{
		ProjectID: p.ID, CompanyID: companyIDOf(partner), Role: "partner",
	})
	if err != nil {
		t.Fatalf("put the partner on the project: %v", err)
	}
	var named bool
	for _, one := range got {
		if one.DisplayName == "Beta Systeme" {
			named = true
		}
	}
	if !named {
		t.Fatalf("a granted seat was not served the section: %+v", got)
	}
}

// An intro path REFUSES rather than withholding, and the difference is not
// arbitrary: the company's name is the proposal ("ask X to introduce you at
// Y"), so a path with the account removed is not a smaller answer but a wrong
// one.
func TestAnIntroPathIsRefusedWithoutTheCompanyGrant(t *testing.T) {
	e := SetupSearch(t)
	store := signalStore(e)
	admin := e.adminSignals()

	company := e.seedCompanyWithDomain(t, "Intro Co", "intro.example")
	e.seedEmployedContact(t, company, "Ivy Intro", "ivy@intro.example")
	sigID := createRaw(t, store, admin, "intro.example")
	if _, err := store.Resolve(admin, sigID); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	narrow := signalActorWithout(e, ids.NewV7(), "company")
	if _, err := store.IntroPath(narrow, sigID, time.Now().UTC()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("intro path for a seat with no company grant = %v, want ErrPermissionDenied", err)
	}
}

// signalActorWithout is signalActor's seat minus the named objects, so a test
// can isolate one grant without respelling the rest of the fixture.
func signalActorWithout(e *SearchEnv, user ids.UUID, without ...string) context.Context {
	dropped := map[string]bool{}
	for _, o := range without {
		dropped[o] = true
	}
	grants := map[string]principal.ObjectGrant{}
	for _, o := range []string{"signal", "contact", "company", "deal", "lead", "relationship"} {
		if dropped[o] {
			continue
		}
		grants[o] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll},
	})
}
