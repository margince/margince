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
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// importerLogin is the login of Rep1, whose mailbox the capture harness reads.
// An invitation from it names no colleague.
const importerLogin = "rep0@search.test"

// organisedInvite is colleagueInvite whose calendar part names organizer.
func organisedInvite(from, msgID, organizer string) []byte {
	return []byte(strings.Replace(string(colleagueInvite(from, msgID)), "BEGIN:VCALENDAR\r\nEND:VCALENDAR",
		"BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:"+msgID+"\r\nORGANIZER:mailto:"+organizer+"\r\nEND:VEVENT\r\nEND:VCALENDAR", 1))
}

// groupInvite is an invitation a mailing group relayed: From is the group, and
// the group names the original sender in X-Original-From.
func groupInvite(original, msgID string) []byte {
	relayed := strings.Replace(string(colleagueInvite("team@search.test", msgID)), "From: team@search.test",
		"From: team@search.test\r\nX-Google-Group-Id: 1001\r\nList-ID: <team.search.test>\r\nX-Original-From: "+original, 1)
	return []byte(relayed)
}

// ownerExec runs one statement on the harness's owner connection.
func ownerExec(t *testing.T, e *integration.SearchEnv, sql string, args ...any) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// seedSeatSince adds a seat holding login since the given time.
func seedSeatSince(t *testing.T, e *integration.SearchEnv, login string, since time.Time) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	ownerExec(t, e, `INSERT INTO app_user (id, email, display_name, email_domain_folded, created_at) VALUES ($1, $2, 'Rep', $3, $4)`,
		id, login, owndomain.StoredOfAddress(login), since)
	return id
}

// wsExec runs one statement in the workspace as its admin.
func wsExec(t *testing.T, e *integration.SearchEnv, sql string, args ...any) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), sql, args...)
		return err
	}); err != nil {
		t.Fatalf("%s: %v", sql, err)
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
	for range 20 {
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

const namedSenders = `SELECT count(*) FROM activity WHERE invitation_sender IS NOT NULL`

// olderInvitations captures the invitations of the back-fill test, then leaves
// them as capture stored them before it recorded an invitation's sender.
//
// Capture names a sender on seven of them. The task must restore one. One gets
// another stored sender below, and three are out of its reach: held, archived
// and another transport. Two name a seat that no longer holds the address.
func olderInvitations(t *testing.T, env captureEnv) {
	t.Helper()
	e := env.e
	longBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ownerExec(t, e, `UPDATE app_user SET created_at = $2 WHERE id = $1`, e.Rep3, longBefore)
	gone := seedSeatSince(t, e, "gone@search.test", longBefore)
	renamed := seedSeatSince(t, e, "renamed@search.test", longBefore)
	sent := func(from, id string) []byte { return authenticated(colleagueInvite(from, id), "search.test") }
	env.sync(t,
		sent(colleagueLogin, "old-invite@search.test"),
		colleagueInvite(colleagueLogin, "old-unauth@search.test"),
		authenticated(colleagueInvite(colleagueLogin, "old-otherdomain@search.test"), "other.example"),
		authenticated(organisedInvite(colleagueLogin, "old-forwarded@search.test", "buyer@outside.example"), "search.test"),
		sent(colleagueLogin, "old-other@search.test"),
		sent(importerLogin, "old-own@search.test"),
		authenticated(groupInvite(colleagueLogin, "old-group@search.test"), "search.test"),
		sent(colleagueLogin, "old-held@search.test"),
		sent(colleagueLogin, "old-archived@search.test"),
		sent(colleagueLogin, "old-imap@search.test"),
		sent("gone@search.test", "old-gone@search.test"),
		sent("renamed@search.test", "old-renamed@search.test"),
		sent("late@search.test", "old-late@search.test"))
	if n := countRows(t, e, namedSenders); n != 7 {
		t.Fatalf("capture named a sender on %d invitations, want 7, so the checks below prove nothing", n)
	}
	wsExec(t, e, `UPDATE activity SET invitation_sender = NULL`)
	// A row whose stored sender is not its original's From is not that message.
	wsExec(t, e, `UPDATE activity SET counterparty_email = 'buyer@outside.example' WHERE source_id = 'old-other@search.test'`)
	wsExec(t, e, `UPDATE activity SET archived_at = now() WHERE source_id = 'old-archived@search.test'`)
	wsExec(t, e, `UPDATE activity SET captured_by = replace(captured_by, ':gmail', ':imap') WHERE source_id = 'old-imap@search.test'`)
	held := activityIDOf(t, e, "old-held@search.test")
	wsExec(t, e, `INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'litigation hold')`, held)
	wsExec(t, e, `UPDATE activity SET restricted_at = now(), archived_at = coalesce(archived_at, now()), restricted_reason = 'litigation hold',
		restricted_until = now() + interval '1 year', retention_class = 'commercial_correspondence', retention_class_at = now() WHERE id = $1`, held)
	ownerExec(t, e, `UPDATE app_user SET status = 'deactivated', removed_at = now() WHERE id = $1`, gone)
	ownerExec(t, e, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after, occurred_at)
		VALUES ('human', 'human:test', 'update', 'user', $1, '{"email":"earlier@search.test"}', '{"email":"renamed@search.test"}', $2)`,
		renamed, captureNow.Add(24*time.Hour))
	seedSeatSince(t, e, "late@search.test", captureNow.Add(24*time.Hour))
}

// An invitation stored before capture named its sender gets the colleague back
// from its stored original, by capture's own rule. A second pass changes nothing.
func TestAnOlderInvitationNamesItsColleagueFromItsOriginal(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	claimOwnDomain(t, e, "search.test")
	olderInvitations(t, env)
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
	if n := countRows(t, e, namedSenders); n != 1 {
		t.Errorf("the repair named a sender on %d invitations, want only the one capture's rule proves", n)
	}
	if n := senderAudits(t, e); n != 1 {
		t.Fatalf("the repair wrote %d audit rows, want one for the one invitation it named", n)
	}

	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := invitationsenders.DataTask().Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		if err == nil && (out.Changed != 0 || out.Scanned != 9 || !out.Done) {
			t.Errorf("a replay scanned %d and changed %d, want the 9 uncredited live Gmail invitations and no change", out.Scanned, out.Changed)
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

// A mailing group's own pass is not proof for the sender it names. Capture
// credits no colleague with an invitation a group relayed.
func TestAGroupRelayedInvitationNamesNoColleague(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	claimOwnDomain(t, e, "search.test")
	env.sync(t, authenticated(groupInvite(colleagueLogin, "group-invite@search.test"), "search.test"))
	activityIDOf(t, e, "group-invite@search.test")
	if n := countRows(t, e, namedSenders); n != 0 {
		t.Errorf("capture named a sender on %d group-relayed invitations, want none", n)
	}
}

// projectRows is what read_project_360 tells one reader about each timeline row.
func projectRows(t *testing.T, e *integration.SearchEnv, reader context.Context, project ids.ProjectID) map[ids.UUID]string {
	t.Helper()
	out, err := compose.NewRegistry(e.Pool, compose.SendPath{}).
		Invoke(reader, "read_project_360", fmt.Appendf(nil, `{"project_id":%q}`, project.String()))
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
	rows := map[ids.UUID]string{}
	for _, item := range envelope.Data.Activities.Items {
		rows[item.ActivityID] = item.Direction
	}
	return rows
}

// An agent reading the project page is told who wrote each row. A colleague's
// invitation arrived inbound, and the agent reads it as our side's. A reader
// who may not open the row gets its stored direction, since the voice is withheld.
func TestTheProjectToolNamesTheSideThatWroteARow(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	claimOwnDomain(t, e, "search.test")
	env.sync(t, authenticated(colleagueInvite(colleagueLogin, "project-invite@search.test"), "search.test"))
	invitation := activityIDOf(t, e, "project-invite@search.test")
	project := fileUnderProject(t, e)

	side, listed := projectRows(t, e, seatWriter(e), project)[invitation]
	if !listed {
		t.Fatal("read_project_360 does not carry the invitation")
	}
	if side != "outbound" {
		t.Errorf("the agent reads the colleague's invitation as %q, want outbound: our side wrote it", side)
	}

	if readsContent(t, e, e.Rep3, invitation) {
		t.Fatal("the second seat may open the invitation, so the withheld case below proves nothing")
	}
	side, listed = projectRows(t, e, projectReader(e, e.Rep3), project)[invitation]
	if !listed {
		t.Fatal("read_project_360 does not list the withheld invitation to the second seat")
	}
	if side != "inbound" {
		t.Errorf("a reader who may not open the invitation reads %q, want its stored direction inbound", side)
	}
}
