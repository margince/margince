// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatMailboxes connects one mailbox per seat under its login address, so a
// To line naming that address names the seat.
func seatMailboxes(t *testing.T, e *integration.Env, seats ...ids.UUID) map[ids.UUID]string {
	t.Helper()
	addresses := map[ids.UUID]string{}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for _, seat := range seats {
			var address string
			if err := tx.QueryRow(context.Background(), `SELECT email FROM app_user WHERE id=$1`, seat).Scan(&address); err != nil {
				return err
			}
			addresses[seat] = address
			if _, err := tx.Exec(context.Background(), `INSERT INTO capture_connection (user_id, provider, mail_posture, account_label) VALUES ($1,'gmail','shared',$2)`, seat, address); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return addresses
}

// seedAskingMail logs one inbound request addressed to the given mailboxes.
func seedAskingMail(t *testing.T, e *integration.Env, at time.Time, thread string, to ...string) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, "Buyer on "+thread, &e.AdminUser)
	subject, body, direction := "Please send the offer "+thread, "Please send the offer for review.", "inbound"
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
		ThreadKey: thread, EmailFrom: "buyer@customer.test", EmailTo: to,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(logged.Id)
	classifier := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:owed_verdict", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
	if _, err := e.Activities.SetOwedVerdict(classifier, id, activities.OwedVerdictAsksUs, "fixture", at); err != nil {
		t.Fatal(err)
	}
	return id
}

func waitingIDs(rows []activities.WaitingReply) []ids.UUID {
	out := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ActivityID)
	}
	return out
}

// The team's narrowed scan is the unnarrowed scan kept to the mail a member
// answers for, row for row. A mail addressed to two members is nobody's. A
// colleague outside the team keeps their own.
func TestTheTeamWaitingScanIsTheWorkspaceScanKeptToTheTeamsMail(t *testing.T) {
	e := integration.Setup(t)
	at := quietLaneNow.AddDate(0, 0, -1)
	mailbox := seatMailboxes(t, e, e.Rep1, e.Rep2, e.Rep3)
	member := seedAskingMail(t, e, at, "member", mailbox[e.Rep1])
	otherMember := seedAskingMail(t, e, at.Add(time.Minute), "other-member", mailbox[e.Rep2])
	outsider := seedAskingMail(t, e, at.Add(2*time.Minute), "outsider", mailbox[e.Rep3])
	shared := seedAskingMail(t, e, at.Add(3*time.Minute), "shared", mailbox[e.Rep1], mailbox[e.Rep2])
	team := []ids.UUID{e.Rep1, e.Rep2}

	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	asOf := quietLaneNow
	every, err := e.Activities.WaitingRepliesBefore(reader, asOf, activities.WaitingPosition{})
	if err != nil {
		t.Fatal(err)
	}
	var want []activities.WaitingReply
	for _, row := range every {
		if row.Kind != "email" || slices.Contains(team, row.OwnerID) {
			want = append(want, row)
		}
	}
	got, err := e.Activities.WaitingRepliesAnsweredByBefore(reader, asOf, activities.WaitingPosition{}, team)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(waitingIDs(got), waitingIDs(want)) {
		t.Fatalf("the team's scan = %v, want the workspace scan kept to the team %v", waitingIDs(got), waitingIDs(want))
	}
	for i := range got {
		if got[i].OwnerID != want[i].OwnerID || got[i].AddressedElsewhere != want[i].AddressedElsewhere {
			t.Errorf("row %s reads as %+v in the team's scan, want %+v as in the workspace scan", got[i].ActivityID, got[i], want[i])
		}
	}
	gotIDs := waitingIDs(got)
	for _, id := range []ids.UUID{member, otherMember} {
		if !slices.Contains(gotIDs, id) {
			t.Errorf("member mail %s missing from the team's scan %v", id, gotIDs)
		}
	}
	for _, id := range []ids.UUID{outsider, shared} {
		if slices.Contains(gotIDs, id) {
			t.Errorf("mail %s is not the team's, but the team's scan has it: %v", id, gotIDs)
		}
	}
}

// The team page shows a member's waiting mail and quiet contact, and neither
// of a colleague on another team.
func TestTheTeamWorklistShowsItsMembersWaitingMailAndQuietContactsOnly(t *testing.T) {
	e := integration.Setup(t)
	at := quietLaneNow.AddDate(0, 0, -1)
	mailbox := seatMailboxes(t, e, e.Rep1, e.Rep2, e.Rep3)
	member := seedAskingMail(t, e, at, "member", mailbox[e.Rep2])
	outsider := seedAskingMail(t, e, at.Add(time.Minute), "outsider", mailbox[e.Rep3])
	shared := seedAskingMail(t, e, at.Add(2*time.Minute), "shared", mailbox[e.Rep1], mailbox[e.Rep2])
	memberContact := e.SeedContact(t, "Quiet with a member", &e.Rep2)
	outsiderContact := e.SeedContact(t, "Quiet with an outsider", &e.Rep3)
	seedQuietExchange(t, e, e.Rep2, ids.UUID{}, memberContact, 60)
	seedQuietExchange(t, e, e.Rep3, ids.UUID{}, outsiderContact, 60)

	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return quietLaneNow })
	page, err := feed.Worklist(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), "team", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range page.SourcesUnavailable {
		if missing.Source == "customer_waiting" || missing.Source == "relationship_decay" {
			t.Fatalf("the team page could not read %s: %s", missing.Source, missing.Reason)
		}
	}
	shown := map[string]bool{}
	for _, row := range page.Queue {
		if row.Source == "customer_waiting" {
			shown[row.Id] = true
		}
	}
	if !shown[member.String()] {
		t.Errorf("the member's waiting mail is missing from the team page: %v", shown)
	}
	for _, id := range []ids.UUID{outsider, shared} {
		if shown[id.String()] {
			t.Errorf("mail %s is not the team's, but the team page shows it", id)
		}
	}
	decayed := map[ids.UUID]bool{}
	rows := page.Queue
	if page.OptionalRelationships != nil {
		rows = append(rows, *page.OptionalRelationships...)
	}
	for _, row := range rows {
		if row.Source == "relationship_decay" && row.Subject != nil {
			decayed[ids.UUID(row.Subject.Id)] = true
		}
	}
	if !decayed[memberContact] || decayed[outsiderContact] {
		t.Errorf("quiet contacts on the team page = %v, want the member's %s and not the outsider's %s", decayed, memberContact, outsiderContact)
	}
}
