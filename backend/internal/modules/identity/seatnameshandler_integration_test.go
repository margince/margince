// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Which seats a naming read names, and that it names them to a caller who
// administers nobody. SeatNames itself is covered in
// rosterteams_integration_test.go, which holds that a non-member is refused;
// some cases here call it directly and the rest go through its HTTP handler.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatNamePassword is shared across this file's invited seats: nothing here
// asserts about the password itself, so one value is enough to redeem with.
const seatNamePassword = "a colleague password!"

// inviteAndLogin provisions a member through the real invite → redeem →
// login path, the sequence rosterteams_integration_test.go spells out: a
// hand-built Identity would prove only that this file spelled a role key
// right, not that the seat can actually sign in. Every colleague this suite
// names is a rep: TestTheAnswerCarriesNothingButTheName is the proof that the
// naming read does not even disclose a role, so there is nothing here for a
// second role to exercise.
func inviteAndLogin(t *testing.T, e *revocationEnv, email, name string) Identity {
	t.Helper()
	_, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: email, DisplayName: name, Role: "rep",
	})
	if err != nil {
		t.Fatalf("inviting %s: %v", email, err)
	}
	redeemCtx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws.UUID), ids.NewV7())
	if err := e.svc.RedeemPasswordReset(redeemCtx, rawToken, seatNamePassword); err != nil {
		t.Fatalf("redeeming %s's invite token: %v", email, err)
	}
	identity, _, err := e.svc.Login(principal.WithWorkspaceID(context.Background(), e.ws.UUID), email, seatNamePassword, noDevice)
	if err != nil {
		t.Fatalf("%s login: %v", email, err)
	}
	return identity
}

// archiveSeat marks a seat archived through the same raw statement
// oauth_useraccess_integration_test.go uses: the identity module writes no
// application-level archive path for app_user, only the invite/deactivate/
// reactivate lifecycle that goes through the service.
func archiveSeat(t *testing.T, e *revocationEnv, id ids.UserID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE app_user SET archived_at = now() WHERE id = $1`, id); err != nil {
		t.Fatalf("archiving the seat: %v", err)
	}
}

// A seat that no longer works here still owns the records it made, so a rep
// looking at one gets a name rather than a uuid. This is the reading the roster
// walk could not produce: it stops at the seats that may still become active.
func TestANonAdminNamesADeactivatedColleague(t *testing.T) {
	e := setupRevocationEnv(t, "seat-names-deactivated")
	rep := inviteAndLogin(t, e, "rep@acme.test", "Rep One")

	departed, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "departed@acme.test", DisplayName: "Dana Kessler", Role: "rep",
	})
	if err != nil {
		t.Fatalf("inviting the colleague who later leaves: %v", err)
	}
	if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: departed}); err != nil {
		t.Fatalf("deactivating them: %v", err)
	}

	named, err := e.svc.SeatNames(e.wsCtx(rep), []ids.UserID{departed})
	if err != nil {
		t.Fatalf("naming a deactivated colleague: %v", err)
	}
	if named[departed.UUID] != "Dana Kessler" {
		t.Fatalf("a deactivated colleague is named; the read says %q", named[departed.UUID])
	}
}

// A colleague who has not yet redeemed their invite still owns whatever was
// recorded under their name before they ever signed in, so a non-admin caller
// sees that name rather than a uuid.
func TestANonAdminNamesAnInvitedColleague(t *testing.T) {
	e := setupRevocationEnv(t, "seat-names-invited")
	rep := inviteAndLogin(t, e, "rep@acme.test", "Rep One")

	invited, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "newcomer@acme.test", DisplayName: "Noor Newcomer", Role: "rep",
	})
	if err != nil {
		t.Fatalf("inviting a colleague who has not signed in yet: %v", err)
	}

	named, err := e.svc.SeatNames(e.wsCtx(rep), []ids.UserID{invited})
	if err != nil {
		t.Fatalf("naming an invited colleague: %v", err)
	}
	if named[invited.UUID] != "Noor Newcomer" {
		t.Fatalf("an invited colleague is named; the read says %q", named[invited.UUID])
	}
}

// An archived seat is the one that stops resolving, which is what lets a name
// be withdrawn without rewriting the history that used it.
func TestAnArchivedSeatIsNotNamed(t *testing.T) {
	e := setupRevocationEnv(t, "seat-names-archived")

	gone, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "gone@acme.test", DisplayName: "Archived Seat", Role: "rep",
	})
	if err != nil {
		t.Fatalf("inviting the seat that is later archived: %v", err)
	}
	archiveSeat(t, e, gone)

	named, err := e.svc.SeatNames(e.wsCtx(e.admin), []ids.UserID{gone})
	if err != nil {
		t.Fatalf("asking about an archived seat: %v", err)
	}
	if _, present := named[gone.UUID]; present {
		t.Fatal("an archived seat was named")
	}
}

// The ceiling is the handler's, because the contract's maxItems is documentation:
// nothing generated from it checks the length at runtime.
func TestNamingMoreThanTheCeilingIsRefused(t *testing.T) {
	e := setupRevocationEnv(t, "seat-names-ceiling")

	atTheLimit := make([]openapi_types.UUID, maxNamedSeats)
	for i := range atTheLimit {
		atTheLimit[i] = openapi_types.UUID(ids.NewV7())
	}
	if status := nameSeatsStatus(t, e, e.admin, atTheLimit); status != http.StatusOK {
		t.Fatalf("naming %d colleagues is allowed; the handler answered %d", maxNamedSeats, status)
	}

	overTheLimit := make([]openapi_types.UUID, len(atTheLimit), len(atTheLimit)+1)
	copy(overTheLimit, atTheLimit)
	overTheLimit = append(overTheLimit, openapi_types.UUID(ids.NewV7()))
	if status := nameSeatsStatus(t, e, e.admin, overTheLimit); status != http.StatusUnprocessableEntity {
		t.Fatalf("naming %d colleagues is refused; the handler answered %d", len(overTheLimit), status)
	}
}

// One request, one order. Ranging the map SeatNames returns would answer the
// same request differently between runs.
func TestTheAnswerFollowsTheOrderAsked(t *testing.T) {
	e := setupRevocationEnv(t, "seat-names-order")
	first := inviteAndLogin(t, e, "first@acme.test", "Ada First")
	second := inviteAndLogin(t, e, "second@acme.test", "Bo Second")
	third := inviteAndLogin(t, e, "third@acme.test", "Cy Third")
	fourth := inviteAndLogin(t, e, "fourth@acme.test", "Dee Fourth")

	// Asked as an interleaving (neither the creation order above nor its
	// reverse) and across four seats rather than two: with only two,
	// a handler that ranged the map SeatNames returns instead of walking the
	// request would still match this order about half the time.
	asked := []openapi_types.UUID{
		openapi_types.UUID(third.UserID.UUID),
		openapi_types.UUID(first.UserID.UUID),
		openapi_types.UUID(fourth.UserID.UUID),
		openapi_types.UUID(second.UserID.UUID),
	}
	body := nameSeatsBody(t, e, e.admin, asked)
	if len(body.Data) != 4 {
		t.Fatalf("four colleagues were asked about; %d came back", len(body.Data))
	}
	wantOrder := []string{"Cy Third", "Ada First", "Dee Fourth", "Bo Second"}
	for i, want := range wantOrder {
		if body.Data[i].DisplayName != want {
			t.Fatalf("the answer follows the order asked; position %d is %q, want %q",
				i, body.Data[i].DisplayName, want)
		}
	}
}

// The shape is the disclosure decision: a naming read hands back a name, and a
// later "while we are here" addition of the email has to argue with this.
func TestTheAnswerCarriesNothingButTheName(t *testing.T) {
	e := setupRevocationEnv(t, "seat-names-shape")
	rep := inviteAndLogin(t, e, "rep@acme.test", "Rep One")

	raw := nameSeatsRawJSON(t, e, rep, []openapi_types.UUID{openapi_types.UUID(rep.UserID.UUID)})
	for _, leaked := range []string{"email", "status", "seat_type", "is_agent", "roles", "team_ids"} {
		if strings.Contains(raw, `"`+leaked+`"`) {
			t.Fatalf("a naming read disclosed %q: %s", leaked, raw)
		}
	}
	if !strings.Contains(raw, `"display_name"`) {
		t.Fatalf("a naming read carried no name at all: %s", raw)
	}
}

// A repeated id is one question, and an id nobody holds is an absence rather
// than an error: the two readings the loader on the client depends on.
func TestARepeatIsOneRowAndAnUnknownIdIsNoRow(t *testing.T) {
	e := setupRevocationEnv(t, "seat-names-repeat")
	rep := inviteAndLogin(t, e, "rep@acme.test", "Rep One")
	stranger := openapi_types.UUID(ids.NewV7())

	body := nameSeatsBody(t, e, e.admin, []openapi_types.UUID{
		openapi_types.UUID(rep.UserID.UUID),
		openapi_types.UUID(rep.UserID.UUID),
		stranger,
	})
	if len(body.Data) != 1 {
		t.Fatalf("one colleague was named twice and one is unknown; %d rows came back", len(body.Data))
	}
	if body.Data[0].DisplayName != "Rep One" {
		t.Fatalf("the named colleague is Rep One; the read says %q", body.Data[0].DisplayName)
	}
}

// nameSeatsRecorder is the one request+recorder builder the three readers
// below share, so the ceiling, order and shape assertions differ only in
// what they read back rather than in how they call the handler.
// An external Deal Room participant is not a member of the installation whose
// colleagues these are. SeatNames holds that gate itself
// (rosterteams_integration_test.go proves it), and this is the proof that the
// refusal reaches a caller of the ROUTE as the 403 the contract publishes,
// rather than as an empty answer that reads as a workspace with nobody in it.
func TestNamingRefusesADealRoomBuyer(t *testing.T) {
	e := setupRevocationEnv(t, "seat-names-buyer")

	buyerCtx := principal.WithActor(
		principal.WithWorkspaceID(context.Background(), e.ws.UUID),
		principal.Principal{Type: principal.PrincipalBuyer, ID: "buyer:room-guest"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/users/names", nil).WithContext(buyerCtx)
	NewHandlers(e.svc).NameSeats(rec, req, crmcontracts.NameSeatsParams{
		Id: []openapi_types.UUID{openapi_types.UUID(e.admin.UserID.UUID)},
	})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("a Deal Room buyer naming a colleague = %d, want %d: %s",
			rec.Code, http.StatusForbidden, rec.Body)
	}
}

func nameSeatsRecorder(t *testing.T, e *revocationEnv, caller Identity, wanted []openapi_types.UUID) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/users/names", nil).
		WithContext(withIdentity(e.wsCtx(caller), caller))
	NewHandlers(e.svc).NameSeats(rec, req, crmcontracts.NameSeatsParams{Id: wanted})
	return rec
}

func nameSeatsStatus(t *testing.T, e *revocationEnv, caller Identity, wanted []openapi_types.UUID) int {
	t.Helper()
	return nameSeatsRecorder(t, e, caller, wanted).Code
}

func nameSeatsBody(t *testing.T, e *revocationEnv, caller Identity, wanted []openapi_types.UUID) crmcontracts.SeatNameListResponse {
	t.Helper()
	rec := nameSeatsRecorder(t, e, caller, wanted)
	if rec.Code != http.StatusOK {
		t.Fatalf("naming status = %d: %s", rec.Code, rec.Body)
	}
	var body crmcontracts.SeatNameListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return body
}

func nameSeatsRawJSON(t *testing.T, e *revocationEnv, caller Identity, wanted []openapi_types.UUID) string {
	t.Helper()
	rec := nameSeatsRecorder(t, e, caller, wanted)
	if rec.Code != http.StatusOK {
		t.Fatalf("naming status = %d: %s", rec.Code, rec.Body)
	}
	return rec.Body.String()
}
