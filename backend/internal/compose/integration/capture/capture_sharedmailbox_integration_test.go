// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A shared mailbox (sales@) syncs with no seat behind it, records its own
// import row, and is read by its reader team. Every message here goes through
// the real registry and sink.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	sharedMailboxAddress  = "sales@myco.example"
	sharedMailboxProvider = "test_mailbox"
)

// sharedMailbox connects sales@ read by team and returns its connection and a
// sync of it through the env's own registry.
func sharedMailbox(t *testing.T, env captureEnv, team ids.UUID, posture string) (ids.UUID, func(t *testing.T, raws ...[]byte)) {
	t.Helper()
	conn := &mailBatchConnector{name: sharedMailboxProvider, accountLabel: sharedMailboxAddress}
	registry := compose.NewCaptureRegistry(env.e.Pool, newTestKeyvault(t, env.e), compose.CaptureConfig{
		Now: func() time.Time { return captureNow },
	})
	registry.Register(conn)
	id := integration.ConnectSharedMailbox(t, registry, env.e.WS, integration.SharedMailbox{
		Provider: sharedMailboxProvider, Address: sharedMailboxAddress, ReaderTeam: team, ConnectedBy: env.e.Rep1, Posture: posture,
	})
	wsCtx := principal.WithWorkspaceID(context.Background(), env.e.WS)
	return id, func(t *testing.T, raws ...[]byte) {
		t.Helper()
		conn.raws, conn.sent, conn.deals, conn.kinds = raws, nil, nil, nil
		if err := registry.SyncOnce(wsCtx, id); err != nil {
			t.Fatalf("shared mailbox SyncOnce: %v", err)
		}
	}
}

// mailTo is one inbound customer message addressed to the listed mailboxes.
func mailTo(msgID, body string, to ...string) []byte {
	return []byte(strings.Join([]string{
		"From: Anna Acme <anna@acme.example>", "To: " + strings.Join(to, ", "), "Subject: order 42",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000", "Message-ID: <" + msgID + ">",
		"Content-Type: text/plain", "", body, "",
	}, "\r\n"))
}

// readerAs is a member reading the timeline with an ordinary role, its teams
// as identity's loadGrants hands them over.
func readerAs(e *integration.SearchEnv, user ids.UUID, teams ...ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, TeamIDs: teams,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true}, "contact": {Read: true}, "company": {Read: true},
		}, RowScope: principal.RowScopeAll},
	})
}

// readsMail answers whether the reader may open the message, and how many
// emails a list of readable mail counts for them.
func readsMail(reader context.Context, t *testing.T, e *integration.SearchEnv, id ids.UUID) (bool, int) {
	t.Helper()
	var readable bool
	var listed int
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		switch err := auth.EnsureActivityContentVisible(reader, tx, id); {
		case err == nil:
			readable = true
		case errors.Is(err, apperrors.ErrNotFound):
		default:
			return err
		}
		var args []any
		clause, err := auth.ActivityContentClause(reader, "a", func(v any) int { args = append(args, v); return len(args) })
		if err != nil {
			return err
		}
		return tx.QueryRow(reader, `SELECT count(*) FROM activity a WHERE a.kind = 'email' AND `+clause, args...).Scan(&listed)
	}); err != nil {
		t.Fatalf("reading the mail: %v", err)
	}
	return readable, listed
}

// discoversMail counts the emails the reader may learn exist, the gate the
// timeline lists by.
func discoversMail(reader context.Context, t *testing.T, e *integration.SearchEnv, id ids.UUID) int {
	t.Helper()
	var listed int
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		var args []any
		clause, err := auth.ActivityDiscoverClause(reader, "a", func(v any) int { args = append(args, v); return len(args) })
		if err != nil {
			return err
		}
		args = append(args, id)
		return tx.QueryRow(reader, fmt.Sprintf(`SELECT count(*) FROM activity a WHERE a.id = $%d AND %s`, len(args), clause), args...).Scan(&listed)
	}); err != nil {
		t.Fatalf("discovering the mail: %v", err)
	}
	return listed
}

func adminCount(t *testing.T, e *integration.SearchEnv, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, args...).Scan(&n)
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return n
}

// outsider is a live member in no team at all.
func outsider(t *testing.T, e *integration.SearchEnv) ids.UUID {
	t.Helper()
	return e.SeedID(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'outsider@search.test', 'Outsider')`)
}

func emailIDs(t *testing.T, e *integration.SearchEnv) []ids.UUID {
	t.Helper()
	var out []ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT id FROM activity WHERE kind = 'email' ORDER BY id`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestASharedMailboxCapturesMailWithNoSeatBehindIt(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	box, syncShared := sharedMailbox(t, env, e.Team1, capturemod.PostureHeld)
	syncShared(t, mailTo("shared-only-1@acme.example", "Please ship order 42.", sharedMailboxAddress))

	id := oneActivityID(t, e)
	if got := adminCount(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND captured_by = 'connector:test_mailbox'`, id); got != 1 {
		t.Fatal("a shared mailbox's message must name the bare connector as its importer, never a seat")
	}
	if got := adminCount(t, e, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND connection_id = $2 AND user_id IS NULL`, id, box); got != 1 {
		t.Fatalf("the shared mailbox wrote %d import rows of its own, want 1", got)
	}
	if got, reason := audienceOf(t, e, id); got != "participants" || reason != "shared_mailbox" {
		t.Fatalf("a held shared mailbox's message is %q / %q, want participants / shared_mailbox", got, reason)
	}
	for what, query := range map[string]string{
		"contact":                  `SELECT count(*) FROM contact`,
		"lead":                     `SELECT count(*) FROM lead`,
		"task":                     `SELECT count(*) FROM activity WHERE kind = 'task'`,
		"confidentiality question": `SELECT count(*) FROM capture_thread_verdict`,
		"counterparty verdict":     `SELECT count(*) FROM capture_pending_counterparty`,
		"promise":                  `SELECT count(*) FROM activity_commitment_admission`,
		"own-address contact":      `SELECT count(*) FROM contact_email WHERE email ILIKE 'sales@%'`,
	} {
		if got := adminCount(t, e, query); got != 0 {
			t.Errorf("shared-only mail created %d %s rows, want none", got, what)
		}
	}
	// A second sync of the same message is the ordinary replay.
	syncShared(t, mailTo("shared-only-1@acme.example", "Please ship order 42.", sharedMailboxAddress))
	if got := len(emailIDs(t, e)); got != 1 {
		t.Fatalf("a replay in the shared mailbox left %d emails, want 1", got)
	}
}

func TestASeatOutsideTheReaderTeamCannotFindSharedMail(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	_, syncShared := sharedMailbox(t, env, e.Team1, capturemod.PostureHeld)
	syncShared(t, mailTo("shared-reader-1@acme.example", "Invoice attached.", sharedMailboxAddress))
	id := oneActivityID(t, e)

	if readable, listed := readsMail(readerAs(e, e.Rep1, e.Team1), t, e, id); !readable || listed != 1 {
		t.Fatalf("a reader-team member got readable=%v listed=%d, want true and 1", readable, listed)
	}
	if readable, listed := readsMail(readerAs(e, e.Rep3, e.Team2), t, e, id); readable || listed != 0 {
		t.Fatalf("a seat outside the reader team got readable=%v listed=%d, want the answer for mail it never saw", readable, listed)
	}
	// Link-less held mail must not even show that it exists, when it came in.
	if got := discoversMail(readerAs(e, e.Rep3, e.Team2), t, e, id); got != 0 {
		t.Fatal("a seat outside the reader team discovers held shared mail through the link-less default")
	}
	if got := discoversMail(readerAs(e, e.Rep1, e.Team1), t, e, id); got != 1 {
		t.Fatal("the reader team cannot discover its own mailbox's mail")
	}
}

func TestASharedMailboxSetToSharedIsReadByEveryoneWhoSeesTheRecord(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	allowSharedPosture(t, e)
	_, syncShared := sharedMailbox(t, env, e.Team1, capturemod.PostureShared)
	syncShared(t, mailTo("shared-open-1@acme.example", "Opening hours.", sharedMailboxAddress))
	id := oneActivityID(t, e)
	if got, _ := audienceOf(t, e, id); got != "workspace" {
		t.Fatalf("a shared mailbox set to shared births %q, want workspace", got)
	}
	if readable, _ := readsMail(readerAs(e, outsider(t, e)), t, e, id); !readable {
		t.Fatal("a non-member cannot read mail an admin opened to everyone")
	}
}

func TestARemovedMemberLosesSharedMailAndItsCachedHomeAnswerAtOnce(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	_, syncShared := sharedMailbox(t, env, e.Team1, capturemod.PostureHeld)
	syncShared(t, mailTo("shared-removed-1@acme.example", "Contract draft.", sharedMailboxAddress))
	id := oneActivityID(t, e)
	if readable, _ := readsMail(readerAs(e, e.Rep1, e.Team1), t, e, id); !readable {
		t.Fatal("the member must read the shared mail before they are removed")
	}

	fence := adminCount(t, e, `SELECT generation::int FROM home_read_fence`)
	if _, err := e.Owner.Exec(context.Background(), `DELETE FROM team_membership WHERE team_id = $1 AND user_id = $2`, e.Team1, e.Rep1); err != nil {
		t.Fatal(err)
	}
	// The principal still carries the team it was signed in with: the read
	// rule asks team_membership, not the session.
	if readable, listed := readsMail(readerAs(e, e.Rep1, e.Team1), t, e, id); readable || listed != 0 {
		t.Fatalf("a removed member still reads the shared mail: readable=%v listed=%d", readable, listed)
	}
	if after := adminCount(t, e, `SELECT generation::int FROM home_read_fence`); after <= fence {
		t.Fatalf("removing a reader left the Home fence at %d (was %d), so a cached answer would still be served", after, fence)
	}
}

func TestAnArchivedReaderTeamReadsNothingAndStopsTheSync(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	box, syncShared := sharedMailbox(t, env, e.Team1, capturemod.PostureHeld)
	syncShared(t, mailTo("shared-archived-1@acme.example", "Hello.", sharedMailboxAddress))
	id := oneActivityID(t, e)
	if _, err := e.Owner.Exec(context.Background(), `UPDATE team SET archived_at = now() WHERE id = $1`, e.Team1); err != nil {
		t.Fatal(err)
	}
	if readable, _ := readsMail(readerAs(e, e.Rep1), t, e, id); readable {
		t.Fatal("an archived reader team still reads the shared mailbox")
	}
	// Due by its own clock, so only the reader team decides.
	if _, err := e.Owner.Exec(context.Background(), `UPDATE capture_sync_state SET next_sync_at = now() - interval '1 minute' WHERE connection_id = $1`, box); err != nil {
		t.Fatal(err)
	}
	due, err := env.registry.DueConnections(principal.WithWorkspaceID(context.Background(), e.WS), sharedMailboxProvider)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range due {
		if d.ID == box {
			t.Fatal("a shared mailbox whose reader team is archived is still scheduled")
		}
	}
}

func TestASharedMailboxIsScheduledLikeAPersonalOne(t *testing.T) {
	env := newCaptureEnv(t)
	box, _ := sharedMailbox(t, env, env.e.Team1, capturemod.PostureHeld)
	due, err := env.registry.DueConnections(principal.WithWorkspaceID(context.Background(), env.e.WS), sharedMailboxProvider)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(due, func(d capturemod.DueConnection) bool { return d.ID == box }) {
		t.Fatalf("DueConnections answered %v, want the shared mailbox %s among them", due, box)
	}
}

func TestMailInAHeldPrivateMailboxAndASharedOneStaysClosedToEveryoneElse(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureHeld)
	box, syncShared := sharedMailbox(t, env, e.Team2, capturemod.PostureShared)
	msg := mailTo("two-boxes-1@acme.example", "Our private terms.", captureOwner, sharedMailboxAddress)
	env.sync(t, msg)
	syncShared(t, msg)

	id := oneActivityID(t, e)
	if got := adminCount(t, e, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND connection_id = $2`, id, box); got != 1 {
		t.Fatal("the shared mailbox's proven copy must join the message the private mailbox holds")
	}
	if got, _ := audienceOf(t, e, id); got != "participants" {
		t.Fatalf("a shared mailbox opened a message a held private mailbox also holds: %q", got)
	}
	if readable, _ := readsMail(readerAs(e, e.Rep3, e.Team2), t, e, id); !readable {
		t.Fatal("the reader team reads what its mailbox delivered")
	}
	if readable, listed := readsMail(readerAs(e, outsider(t, e)), t, e, id); readable || listed != 0 {
		t.Fatalf("a seat in neither mailbox reads the message: readable=%v listed=%d", readable, listed)
	}
}

func TestAForgedMessageIDInTheSharedMailboxDoesNotJoinPrivateMail(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	setPosture(t, env, e.Rep1, capturemod.PostureHeld)
	box, syncShared := sharedMailbox(t, env, e.Team2, capturemod.PostureHeld)
	env.sync(t, mailTo("forged-1@acme.example", "The private original.", captureOwner))
	private := oneActivityID(t, e)
	syncShared(t, mailTo("forged-1@acme.example", "A forger's copy.", sharedMailboxAddress))

	if got := adminCount(t, e, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND connection_id = $2`, private, box); got != 0 {
		t.Fatal("a forged Message-ID joined the shared mailbox to a colleague's private mail")
	}
	if readable, _ := readsMail(readerAs(e, e.Rep3, e.Team2), t, e, private); readable {
		t.Fatal("the reader team reads the private original through a forged Message-ID")
	}
	if got := len(emailIDs(t, e)); got != 2 {
		t.Fatalf("the shared mailbox's own copy must land under its own key: %d emails, want 2", got)
	}
}

func TestAPersonalSeatsVerdictDoesNotAttachToTheSharedRow(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)
	_, syncShared := sharedMailbox(t, env, e.Team2, capturemod.PostureHeld)
	msg := mailTo("verdict-1@acme.example", "Order details.", captureOwner, sharedMailboxAddress)
	env.sync(t, msg)
	syncShared(t, msg)
	id := oneActivityID(t, e)

	// Rep1's thread is judged ordinary. Joined on the seat, it must not reach
	// the shared row and clear the shared mailbox's own hold.
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO capture_thread_verdict (thread_key, user_id, status, resolved_at)
		SELECT thread_key, $2, 'cleared', now() FROM activity WHERE id = $1`, id, e.Rep1); err != nil {
		t.Fatal(err)
	}
	recompute(t, e, id)
	if got, reason := audienceOf(t, e, id); got != "participants" || reason != "shared_mailbox" {
		t.Fatalf("a personal verdict reached the shared row: %q / %q, want participants / shared_mailbox", got, reason)
	}
}

func TestAPersonalPurgeDoesNotDestroyAMessageTheSharedMailboxAlsoHolds(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	_, syncShared := sharedMailbox(t, env, e.Team2, capturemod.PostureHeld)
	msg := mailTo("purge-1@acme.example", "Both mailboxes hold this.", captureOwner, sharedMailboxAddress)
	env.sync(t, msg)
	syncShared(t, msg)
	id := oneActivityID(t, e)

	noFloor := capturemod.StatutoryFloor{Clause: func(current, carried int) string {
		return fmt.Sprintf("($%d::text IS NULL AND $%d::text IS NULL AND false)", current, carried)
	}}
	var subject capturemod.PurgeSubject
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		subject, err = capturemod.SelectPurgeSubjectTx(context.Background(), tx, e.Rep1, capturemod.IdentityKindAddress, "anna@acme.example", noFloor)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(subject.SoleImports) != 0 || len(subject.SharedImports) != 1 || subject.SharedImports[0] != id {
		t.Fatalf("the purge would destroy %v and release %v; the shared mailbox's copy must only be released", subject.SoleImports, subject.SharedImports)
	}
}

func TestMovingASharedMailboxToOtherReadersDropsCachedHomeAnswers(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	box, _ := sharedMailbox(t, env, e.Team1, capturemod.PostureHeld)
	for _, change := range []string{
		`UPDATE capture_connection SET reader_team_id = $2 WHERE id = $1`,
		`UPDATE capture_connection SET mail_posture = 'shared' WHERE id = $1 AND $2::uuid IS NOT NULL`,
	} {
		fence := adminCount(t, e, `SELECT generation::int FROM home_read_fence`)
		if _, err := e.Owner.Exec(context.Background(), change, box, e.Team2); err != nil {
			t.Fatal(err)
		}
		if after := adminCount(t, e, `SELECT generation::int FROM home_read_fence`); after <= fence {
			t.Fatalf("%s left the Home fence at %d, so a cached answer would outlive the change", change, after)
		}
	}
}

func TestAPersonalPurgeKeepsAContactWhoseMailTheSharedMailboxHolds(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	_, syncShared := sharedMailbox(t, env, e.Team2, capturemod.PostureHeld)
	contact := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO contact (id, full_name, source, captured_by, owner_id, visibility)
			VALUES ($1, 'Anna Acme', 'manual', 'human:x', $2, 'owner')`, contact, e.Rep1); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
			VALUES ($1, 'anna@acme.example', true, 'manual', 'human:x')`, contact)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	msg := mailTo("purge-contact-1@acme.example", "Both mailboxes hold this.", captureOwner, sharedMailboxAddress)
	env.sync(t, msg)
	syncShared(t, msg)

	noFloor := capturemod.StatutoryFloor{Clause: func(current, carried int) string {
		return fmt.Sprintf("($%d::text IS NULL AND $%d::text IS NULL AND false)", current, carried)
	}}
	var purgeable []ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		purgeable, err = capturemod.SelectPurgeableContactsTx(context.Background(), tx, e.Rep1, capturemod.IdentityKindAddress, "anna@acme.example", noFloor)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(purgeable) != 0 {
		t.Fatalf("a personal purge would anonymize %v, a contact whose mail the shared mailbox still holds", purgeable)
	}
}

func TestABrokenSharedMailboxShowsInFleetHealth(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	box, _ := sharedMailbox(t, env, e.Team1, capturemod.PostureHeld)
	if _, err := e.Owner.Exec(context.Background(), `UPDATE capture_connection SET status = 'error' WHERE id = $1`, box); err != nil {
		t.Fatal(err)
	}
	operator := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"job_health": {Read: true}}, RowScope: principal.RowScopeAll},
	})
	var concerns []capturemod.FleetConcern
	if err := database.WithWorkspaceTx(operator, e.Pool, func(tx pgx.Tx) error {
		var err error
		concerns, err = capturemod.FleetConcernsTx(operator, tx, captureNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, concern := range concerns {
		if concern.Connection.ID == box {
			if !concern.Shared || concern.UserID != nil || concern.Connection.AccountLabel == nil || *concern.Connection.AccountLabel != sharedMailboxAddress {
				t.Fatalf("the shared mailbox's concern is %+v, want it marked shared and named by its address", concern)
			}
			return
		}
	}
	t.Fatalf("a broken shared mailbox is missing from fleet health: %+v", concerns)
}
