// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// The identity read carries its own row scope, rather than trusting the id it
// was handed.
//
// `contactIdentity` is where a contact's name, title and address are read, and
// six surfaces reach it. Five pass ids that came from a scoped roster read; the
// intro draft passes the id a CLIENT named. So the safety was positional — a
// property of what the callers happened to pass — and the function's own
// comment claimed a row scope the statement did not have.
//
// What that clause carries for a contact is CAPTURE PRIVACY, not ownership:
// customer identity is workspace-readable by design, but an unpromoted contact
// a colleague's mailbox sync invented is theirs alone, and the boundary yields
// to neither row_scope=all nor admin.
//
// Driven through the intro draft because it is the surface that takes the id
// from the request, and asserted against a draft that SUCCEEDS first: without
// the baseline, a refusal proves only that the fixture was incomplete.

import (
	"context"
	"errors"
	"strings"
	"testing"

	company360svc "github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedIntroducibleContact puts one contact on one account with everything the
// intro draft needs to write about them: an employment, a message they
// authored, and a colleague who corresponds with them.
func seedIntroducibleContact(t *testing.T, e *integration.Env, name string) (company, contact ids.UUID) {
	t.Helper()
	company = e.SeedCompany(t, "Brandt GmbH", &e.Rep1)
	contact = e.SeedContact(t, name, nil)
	employ(t, e, contact, company, "Chief Financial Officer")
	wrote(t, e, contact, company, "Re: Angebot", "We will review the scope this week.", 2)
	e.WsExec(t, `INSERT INTO graph_interaction_edge
			(user_id, contact_id, last_at, count_90d, in_count_90d, out_count_90d)
		VALUES ($1, $2, $3, 20, 10, 10)`,
		e.Rep2, contact, company360Clock.AddDate(0, 0, -2))
	return company, contact
}

// draftIntro asks for the introduction the way the endpoint does, as whichever
// seat the case is about.
func draftIntro(as context.Context, t *testing.T, e *integration.Env, company, contact ids.UUID) (string, error) {
	t.Helper()
	svc := company360Service(e)
	draft, err := svc.IntroRequestDraft(as, nil, ids.CompanyID{UUID: company},
		company360svc.IntroRequest{
			ContactID: ids.From[ids.ContactKind](contact),
			ViaUserID: ids.From[ids.UserKind](e.Rep2),
		})
	return draft.Body, err
}

// A colleague's unpromoted captured contact is not one this caller may be told
// about, even naming them by id.
func TestAnIntroDraftWillNotNameAColleaguesUnpromotedContact(t *testing.T) {
	e := integration.Setup(t)
	company, contact := seedIntroducibleContact(t, e, "Ute Sommer")
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, company360NoDealPerms)

	body, err := draftIntro(rep, t, e, company, contact)
	if err != nil {
		t.Fatalf("the baseline draft was refused, so this fixture proves nothing about privacy: %v", err)
	}
	if !strings.Contains(body, "Ute Sommer") {
		t.Fatalf("the baseline draft does not name the contact, so a later refusal would prove nothing:\n%s", body)
	}

	// The same contact, now a colleague's unpromoted capture. Nothing else moves.
	e.WsExec(t, `UPDATE contact SET owner_id = $1, visibility = 'owner' WHERE id = $2`, e.Rep3, contact)

	if _, err := draftIntro(rep, t, e, company, contact); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the draft answered %v for a colleague's unpromoted contact, want not-found — the "+
			"identity read takes the id from the request, so naming one returns their name and title "+
			"and spends the model budget writing a letter about them", err)
	}
}

// And the boundary does not yield to admin, which is the whole of what makes it
// a property of the row rather than a scope tier.
//
// Its own case because row_scope=all takes the ordinary predicate away
// entirely: a clause that were merely an owner check would be absent here, and
// the test above would still pass.
func TestNotEvenAnAdminDraftsAboutAColleaguesUnpromotedContact(t *testing.T) {
	e := integration.Setup(t)
	company, contact := seedIntroducibleContact(t, e, "Ute Sommer")

	if _, err := draftIntro(e.Admin(), t, e, company, contact); err != nil {
		t.Fatalf("the baseline draft was refused for an admin, so this fixture proves nothing: %v", err)
	}

	e.WsExec(t, `UPDATE contact SET owner_id = $1, visibility = 'owner' WHERE id = $2`, e.Rep3, contact)

	if _, err := draftIntro(e.Admin(), t, e, company, contact); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an admin drafted about a colleague's unpromoted contact (%v) — capture privacy is a "+
			"property of the row and does not yield to row_scope=all", err)
	}
}

// An archived contact is not on the account any more, and naming them by id
// does not bring them back.
//
// Its own case because the two halves are separate clauses and a statement can
// gain one without the other: the roster reads never pass an archived id, so
// nothing else on this path would notice.
func TestAnIntroDraftWillNotNameAnArchivedContact(t *testing.T) {
	e := integration.Setup(t)
	company, contact := seedIntroducibleContact(t, e, "Ute Sommer")
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, company360NoDealPerms)

	if _, err := draftIntro(rep, t, e, company, contact); err != nil {
		t.Fatalf("the baseline draft was refused, so this fixture proves nothing about archiving: %v", err)
	}

	e.WsExec(t, `UPDATE contact SET archived_at = now() WHERE id = $1`, contact)

	if _, err := draftIntro(rep, t, e, company, contact); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the draft answered %v for an archived contact, want not-found — a retired record "+
			"still reachable by id is one the product goes on writing letters about", err)
	}
}
