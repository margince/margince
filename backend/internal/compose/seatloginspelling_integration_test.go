// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// loginInUnicode gives a seat a login whose domain is spelled in Unicode, and
// folds it through the backfill every seat written before the column passes.
func loginInUnicode(t *testing.T, e *integration.Env, seat ids.UUID, login string) {
	t.Helper()
	e.WsExec(t, `UPDATE app_user SET email = $2, email_domain_folded = NULL WHERE id = $1`, seat, login)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, _, err := identity.FoldLoginDomainsTx(context.Background(), tx, ids.Nil, 100)
		return err
	}); err != nil {
		t.Fatalf("folding the login domain: %v", err)
	}
}

// A calendar event naming a seat's login in punycode binds the seat whose
// login is stored in Unicode. Both spellings are one mailbox.
func TestAnEventNamingASeatsLoginInEitherSpellingBindsTheSeat(t *testing.T) {
	e := integration.Setup(t)
	loginInUnicode(t, e, e.Rep3, "rep3@bücher.example")
	captureCalendarEvent(t, e, e.Rep1, meetingmap.Event{
		ID: "evt-idna", Subject: "Spelled in punycode", StartsAt: laneDay.Add(2 * time.Hour), EndsAt: laneDay.Add(3 * time.Hour),
		Organizer: meetingmap.Actor{Email: "buyer@customer.test"},
		Attendees: []meetingmap.Actor{{Email: "rep3@xn--bcher-kva.example"}},
	})

	if got := myMeetingsToday(t, e, e.Rep3); fmt.Sprint(got) != "[Spelled in punycode]" {
		t.Errorf("the seat got %v, want the event naming its login's punycode spelling", got)
	}
}

// An import naming an owner by the punycode spelling of a Unicode login
// assigns that seat.
func TestAnImportOwnerInEitherSpellingIsTheSeat(t *testing.T) {
	e := integration.Setup(t)
	loginInUnicode(t, e, e.Rep3, "rep3@bücher.example")
	const owner = "Rep3@xn--bcher-kva.example"
	var seats map[string]ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		seats, err = seatsByEmail(context.Background(), tx, []string{owner})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := seats[foldedEmail(owner)]; got != e.Rep3 {
		t.Errorf("the owner %s resolves to seat %v, want %v", owner, got, e.Rep3)
	}
}
