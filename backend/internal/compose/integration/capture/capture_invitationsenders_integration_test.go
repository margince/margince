// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/invitationsenders"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// organisedInvite is colleagueInvite whose calendar part names organizer.
func organisedInvite(from, msgID, organizer string) []byte {
	return []byte(strings.Replace(string(colleagueInvite(from, msgID)), "BEGIN:VCALENDAR\r\nEND:VCALENDAR",
		"BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:"+msgID+"\r\nORGANIZER:mailto:"+organizer+"\r\nEND:VEVENT\r\nEND:VCALENDAR", 1))
}

// importerLogin is the login of Rep1, whose mailbox the capture harness reads.
// An invitation from it names no colleague.
const importerLogin = "rep0@search.test"

// forgetInvitationSenders leaves the rows as capture stored them before it
// recorded an invitation's sender.
func forgetInvitationSenders(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE activity SET invitation_sender = NULL`)
		return err
	}); err != nil {
		t.Fatalf("clearing the stored senders: %v", err)
	}
}

func runInvitationSenders(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	engine, err := datatask.New(e.Pool, []datatask.Task{invitationsenders.DataTask()}, "test",
		func() time.Time { return captureNow }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func senderAudits(t *testing.T, e *integration.SearchEnv) int {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity'
		AND after ? 'invitation_sender' AND evidence ? 'data_task_run'`)
}

// An invitation stored before capture named its sender gets the colleague back
// from its stored original, by capture's own rule. A second pass changes nothing.
func TestAnOlderInvitationNamesItsColleagueFromItsOriginal(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	claimOwnDomain(t, e, "search.test")
	env.sync(t,
		authenticated(colleagueInvite(colleagueLogin, "old-invite@search.test"), "search.test"),
		colleagueInvite(colleagueLogin, "old-unauth@search.test"),
		authenticated(organisedInvite(colleagueLogin, "old-forwarded@search.test", "buyer@outside.example"), "search.test"),
		authenticated(colleagueInvite(colleagueLogin, "old-other@search.test"), "search.test"),
		authenticated(colleagueInvite(importerLogin, "old-own@search.test"), "search.test"))
	captured := listedAuthor(t, e, e.Rep1, "old-invite@search.test")
	if captured == nil || captured.Kind != crmcontracts.ActivityAuthorKindColleague || ids.UUID(*captured.UserId) != e.Rep3 {
		t.Fatalf("capture names author %+v, want the colleague, so the repair below has nothing to restore", captured)
	}
	forgetInvitationSenders(t, e)
	// A row whose stored sender is not its original's From is not that message.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE activity SET counterparty_email = 'buyer@outside.example'
			WHERE source_id = 'old-other@search.test'`)
		return err
	}); err != nil {
		t.Fatalf("restating the sender of one invitation: %v", err)
	}
	if author := listedAuthor(t, e, e.Rep1, "old-invite@search.test"); author != nil &&
		author.Kind == crmcontracts.ActivityAuthorKindColleague {
		t.Fatalf("with no stored sender the invitation still names colleague %+v, so the check below proves nothing", author)
	}

	runInvitationSenders(t, e)

	author := listedAuthor(t, e, e.Rep1, "old-invite@search.test")
	if author == nil || author.Kind != crmcontracts.ActivityAuthorKindColleague ||
		author.UserId == nil || ids.UUID(*author.UserId) != e.Rep3 {
		t.Errorf("after the repair the invitation names author %+v, want the colleague who organised it", author)
	}
	for _, other := range []string{"old-unauth@search.test", "old-forwarded@search.test", "old-other@search.test", "old-own@search.test"} {
		if author := listedAuthor(t, e, e.Rep1, other); author != nil && author.Kind == crmcontracts.ActivityAuthorKindColleague {
			t.Errorf("the repair credited %s to colleague %+v; capture names nobody there", other, author)
		}
	}
	if n := senderAudits(t, e); n != 1 {
		t.Fatalf("the repair wrote %d audit rows, want one for the one invitation it named", n)
	}

	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := invitationsenders.DataTask().Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		if err == nil && (out.Changed != 0 || out.Scanned != 4 || !out.Done) {
			t.Errorf("a replay scanned %d and changed %d, want the 4 uncredited invitations and no change", out.Scanned, out.Changed)
		}
		return err
	}); err != nil {
		t.Fatalf("replaying the batch: %v", err)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		restamped, err := activities.StampInvitationSenderTx(ctx, tx, activityIDOf(t, e, "old-invite@search.test"), e.Rep1, ids.NewV7())
		if err == nil && restamped {
			t.Error("a second stamp replaced the sender the invitation already names")
		}
		return err
	}); err != nil {
		t.Fatalf("stamping a named invitation again: %v", err)
	}
	if n := senderAudits(t, e); n != 1 {
		t.Errorf("a replay wrote %d more audit rows", n-1)
	}
}

// An agent reading the project page is told who wrote each row. A colleague's
// invitation arrived inbound, and the agent reads it as our side's.
func TestTheProjectToolNamesTheSideThatWroteARow(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	claimOwnDomain(t, e, "search.test")
	env.sync(t, authenticated(colleagueInvite(colleagueLogin, "project-invite@search.test"), "search.test"))
	invitation := activityIDOf(t, e, "project-invite@search.test")
	project := fileUnderProject(t, e)

	out, err := compose.NewRegistry(e.Pool, compose.SendPath{}).
		Invoke(seatWriter(e), "read_project_360", fmt.Appendf(nil, `{"project_id":%q}`, project.String()))
	if err != nil {
		t.Fatalf("read_project_360: %v", err)
	}
	var envelope struct {
		Data struct {
			Activities struct {
				Items []struct {
					ActivityID ids.UUID `json:"activity_id"`
					Direction  string   `json:"direction"`
				} `json:"items"`
			} `json:"activities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		t.Fatalf("reading the tool's answer: %v\n%s", err, out)
	}
	for _, item := range envelope.Data.Activities.Items {
		if item.ActivityID != invitation {
			continue
		}
		if item.Direction != "outbound" {
			t.Errorf("the agent reads the colleague's invitation as %q, want outbound: our side wrote it", item.Direction)
		}
		return
	}
	t.Fatalf("read_project_360 does not carry the invitation:\n%s", out)
}
