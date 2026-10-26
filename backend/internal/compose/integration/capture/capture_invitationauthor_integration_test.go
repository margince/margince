// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// colleagueInvite is the invitation Google Calendar sends for a colleague who
// organised a meeting: From the colleague, To this mailbox and a customer.
func colleagueInvite(from, msgID string) []byte {
	return []byte(strings.Join([]string{
		"Sender: Google Calendar <calendar-notification@google.com>",
		"From: " + from,
		"To: " + captureOwner + ", buyer@outside.example",
		"Subject: Invitation: Kickoff @ Fri 5 Jun 2026 11:15am",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + msgID + ">",
		"Content-Type: multipart/alternative; boundary=\"b1\"",
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"You have been invited to Kickoff.",
		"",
		"--b1",
		"Content-Type: text/calendar; charset=UTF-8; method=REQUEST",
		"",
		"BEGIN:VCALENDAR",
		"END:VCALENDAR",
		"",
		"--b1--",
		"",
	}, "\r\n"))
}

// An invitation a colleague organised stays received mail, and every reader
// names the colleague as its author rather than the other side.
func TestAColleaguesInvitationNamesTheColleagueAsItsAuthor(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	claimOwnDomain(t, e, "search.test")

	env.sync(t, authenticated(colleagueInvite(colleagueLogin, "invite-1@search.test"), "search.test"))
	env.sync(t, colleagueInvite(colleagueLogin, "invite-unauth@search.test"))

	author := listedAuthor(t, e, e.Rep1, "invite-1@search.test")
	if author == nil || author.Kind != crmcontracts.ActivityAuthorKindColleague ||
		author.UserId == nil || ids.UUID(*author.UserId) != e.Rep3 {
		t.Errorf("the invitation names author %+v, want the colleague who organised it", author)
	}
	if author := listedAuthor(t, e, e.Rep1, "invite-unauth@search.test"); author != nil &&
		author.Kind == crmcontracts.ActivityAuthorKindColleague {
		t.Errorf("an unauthenticated invitation names colleague %+v; a sender types From", author)
	}

	// Naming the author grants nothing: the colleague reads the credited copy
	// as they read its uncredited twin, and is on neither as a party.
	credited, uncredited := activityIDOf(t, e, "invite-1@search.test"), activityIDOf(t, e, "invite-unauth@search.test")
	if readsContent(t, e, e.Rep3, credited) != readsContent(t, e, e.Rep3, uncredited) {
		t.Error("naming the colleague as author changed whether they may read the invitation")
	}
	if n := countRows(t, e, fmt.Sprintf(`SELECT count(*) FROM activity_participant
		WHERE activity_id IN ('%s', '%s') AND user_id = '%s'`, credited, uncredited, e.Rep3)); n != 0 {
		t.Errorf("the colleague is a party on %d invitation rows; a party row is read as attendance and as an edge", n)
	}
}

// claimOwnDomain adds domain to the installation's own company, as a human
// confirming it in settings would.
func claimOwnDomain(t *testing.T, e *integration.SearchEnv, domain string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by)
			SELECT id, $1, false, 'manual', 'human:test' FROM company WHERE is_anchor`, domain)
		return err
	}); err != nil {
		t.Fatalf("claiming %s as an own domain: %v", domain, err)
	}
}

// listedAuthor is the author the activity list names on one captured mail,
// read as seat.
func listedAuthor(t *testing.T, e *integration.SearchEnv, seat ids.UUID, sourceID string) *crmcontracts.ActivityAuthor {
	t.Helper()
	reader := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + seat.String(), UserID: seat, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	var listed []crmcontracts.Activity
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		var err error
		listed, _, err = activities.ListActivitiesTx(reader, tx, activities.ListActivitiesInput{})
		return err
	}); err != nil {
		t.Fatalf("listing the captured mail: %v", err)
	}
	for _, a := range listed {
		if a.SourceId != nil && strings.Trim(*a.SourceId, "<>") == sourceID {
			if a.Voice == nil {
				return nil
			}
			return a.Voice.Author
		}
	}
	t.Fatalf("the activity list does not carry %s", sourceID)
	return nil
}
