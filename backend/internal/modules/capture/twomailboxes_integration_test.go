// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// sharedMessageMailbox is a Gmail whose account is named by its credential, so
// one seat can connect two. Every sync delivers the same message, one
// Message-ID under one provider message id, as two mailboxes on one thread do.
type sharedMessageMailbox struct {
	checkpointHistory
	// containers are the folders the message sits in, in every mailbox.
	containers []string
}

const sharedMessageID = "both-mailboxes@customer.test"

func (c *sharedMessageMailbox) Sync(ctx context.Context, _ connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	rec := mailRecord(sharedMessageID, "buyer@customer.test", "buyer@customer.test", "work@mail.example", "other@mail.example")
	rec.CapturedBy = "connector:gmail"
	rec.ProviderMessageID = "provider-17"
	rec.Containers = c.containers
	if _, err := sink.Upsert(ctx, rec); err != nil && !errors.Is(err, connector.ErrSkip) {
		return nil, err
	}
	return cursor, nil
}

type twoMailboxes struct {
	ctx         context.Context
	db          *database.DB
	connector   *sharedMessageMailbox
	reg         *capture.Registry
	seat        ids.UserID
	work, other ids.UUID
}

// connectTwoMailboxes connects two Gmail accounts for one seat.
func connectTwoMailboxes(t *testing.T) twoMailboxes {
	t.Helper()
	ctx, _, vault, ws := newCaptureRegistryFixture(t)
	_, pool := setupCaptureDB(t)
	db := database.BindTo(pool, ws)
	reg := capture.NewRegistry(db, capture.NewSink(db), fixtureAuthority{}, vault)
	mailbox := &sharedMessageMailbox{}
	reg.Register(mailbox)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture actor missing")
	}
	work, err := reg.Connect(ctx, "gmail", connector.Auth("work"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := reg.Connect(ctx, "gmail", connector.Auth("other"))
	if err != nil {
		t.Fatal(err)
	}
	if work == other {
		t.Fatal("a second account landed on the first account's row")
	}
	return twoMailboxes{ctx: ctx, db: db, connector: mailbox, reg: reg, seat: ids.From[ids.UserKind](actor.UserID), work: work, other: other}
}

func connectionStatus(t *testing.T, id ids.UUID) (status, label string) {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	if err := owner.QueryRow(context.Background(),
		`SELECT status, account_label FROM capture_connection WHERE id = $1`, id).Scan(&status, &label); err != nil {
		t.Fatal(err)
	}
	return status, label
}

func TestConnectingTheSameAccountAgainRepairsItsRow(t *testing.T) {
	t.Parallel()
	m := connectTwoMailboxes(t)
	again, err := m.reg.Connect(m.ctx, "gmail", connector.Auth("work"))
	if err != nil {
		t.Fatal(err)
	}
	if again != m.work {
		t.Fatalf("the same account made a third mailbox %s, want its own row %s", again, m.work)
	}
	views, err := m.reg.Connections(m.ctx)
	if err != nil || len(views) != 2 {
		t.Fatalf("roster = %d rows, %v; want the two mailboxes", len(views), err)
	}
}

func TestDisconnectingOneMailboxLeavesTheOtherSyncing(t *testing.T) {
	t.Parallel()
	m := connectTwoMailboxes(t)
	if err := m.reg.Disconnect(m.ctx, capture.ProviderMailbox("gmail")); !errors.Is(err, capture.ErrMailboxAmbiguous) {
		t.Fatalf("a provider-only disconnect with two live mailboxes = %v, want the ask to name one", err)
	}
	if err := m.reg.Disconnect(m.ctx, capture.Mailbox{Provider: "gmail", ID: m.work}); err != nil {
		t.Fatal(err)
	}
	if status, _ := connectionStatus(t, m.work); status != "disconnected" {
		t.Fatalf("the disconnected mailbox is %q", status)
	}
	if status, _ := connectionStatus(t, m.other); status != "connected" {
		t.Fatalf("disconnecting one mailbox left the other %q", status)
	}
	if err := m.reg.SyncOnce(m.ctx, m.other); err != nil {
		t.Fatalf("the remaining mailbox no longer syncs: %v", err)
	}
	if err := m.reg.SyncOnce(m.ctx, m.work); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the withdrawn mailbox still syncs: %v", err)
	}
}

func TestAReconnectThatReturnsAnotherAccountIsRefused(t *testing.T) {
	t.Parallel()
	m := connectTwoMailboxes(t)
	if _, err := m.reg.Grant(m.ctx, "gmail", connector.Auth("stranger"), m.work); !errors.Is(err, capture.ErrReconnectOtherAccount) {
		t.Fatalf("a reconnect returning another account = %v, want it refused", err)
	}
	if _, label := connectionStatus(t, m.work); label != "work@mail.example" {
		t.Fatalf("the refused reconnect rebound the mailbox to %q", label)
	}
	views, err := m.reg.Connections(m.ctx)
	if err != nil || len(views) != 2 {
		t.Fatalf("roster = %d rows, %v; a refused reconnect must add no mailbox", len(views), err)
	}
	if _, err := m.reg.Grant(m.ctx, "gmail", connector.Auth("WORK"), m.work); err != nil {
		t.Fatalf("the same account in other case is the same mailbox: %v", err)
	}
}

func TestTheSameMessageThroughTwoMailboxesIsStoredOnce(t *testing.T) {
	t.Parallel()
	m := connectTwoMailboxes(t)
	for _, id := range []ids.UUID{m.work, m.other} {
		if err := m.reg.SyncOnce(m.ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	owner, _ := setupCaptureDB(t)
	var activities, imports, deliveries, keys int
	if err := owner.QueryRow(context.Background(), `
		SELECT count(DISTINCT a.id), count(ci.id), max(cardinality(ci.delivered_by)), max(cardinality(ci.provider_message_ids))
		  FROM activity a JOIN capture_import ci ON ci.activity_id = a.id AND ci.user_id = $1
		 WHERE a.source_id LIKE $2 || '%'`, m.seat, sharedMessageID).Scan(&activities, &imports, &deliveries, &keys); err != nil {
		t.Fatal(err)
	}
	if activities != 1 || imports != 1 {
		t.Fatalf("one message through two mailboxes = %d activities, %d import rows; want one of each", activities, imports)
	}
	if deliveries != 2 {
		t.Fatalf("the import names %d delivering mailboxes, want both", deliveries)
	}
	// One provider id under two mailboxes is two keys: each names its mailbox,
	// so a removal in one cannot reach the other's copy.
	if keys != 2 {
		t.Fatalf("the import holds %d provider message keys, want one per mailbox", keys)
	}
}

func TestAHistoryImportOnOneMailboxLeavesTheOther(t *testing.T) {
	t.Parallel()
	m := connectTwoMailboxes(t)
	work := capture.Mailbox{Provider: "gmail", ID: m.work}
	other := capture.Mailbox{Provider: "gmail", ID: m.other}
	if _, err := m.reg.StartBackfill(m.ctx, capture.ProviderMailbox("gmail"), m.seat, 3, connector.BackfillEstimate{Messages: 20},
		func(context.Context, pgx.Tx, ids.UUID) error { return nil }); !errors.Is(err, capture.ErrMailboxAmbiguous) {
		t.Fatalf("a provider-only import with two mailboxes = %v, want the ask to name one", err)
	}
	run, err := m.reg.StartBackfill(m.ctx, work, m.seat, 3, connector.BackfillEstimate{Messages: 20},
		func(context.Context, pgx.Tx, ids.UUID) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if run.ConnectionID != m.work {
		t.Fatalf("the import runs on %s, want the mailbox it named", run.ConnectionID)
	}
	untouched, err := m.reg.BackfillStatus(m.ctx, other, m.seat)
	if err != nil || untouched != nil {
		t.Fatalf("the other mailbox has run %v, %v; want none", untouched, err)
	}
	if _, err := m.reg.CancelBackfill(m.ctx, other, m.seat); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("cancelling on a mailbox with no import = %v, want nothing to cancel", err)
	}
	still, err := m.reg.BackfillStatus(m.ctx, work, m.seat)
	if err != nil || still == nil || still.ID != run.ID || still.Status == "cancelled" {
		t.Fatalf("cancelling on the other mailbox reached this one's run: %+v, %v", still, err)
	}
}

func TestAReplyGoesOutFromTheMailboxTheMessageArrivedIn(t *testing.T) {
	t.Parallel()
	m := connectTwoMailboxes(t)
	if err := m.reg.SyncOnce(m.ctx, m.other); err != nil {
		t.Fatal(err)
	}
	owner, _ := setupCaptureDB(t)
	var anchor ids.ActivityID
	if err := owner.QueryRow(context.Background(),
		`SELECT activity_id FROM capture_import WHERE user_id = $1`, m.seat).Scan(&anchor); err != nil {
		t.Fatal(err)
	}
	everyMailbox := func(capture.SendingMailbox) (bool, error) { return true, nil }
	choose := func(from ids.UUID, anchor ids.ActivityID) ids.UUID {
		t.Helper()
		chosen, err := m.reg.ChooseSendingMailbox(m.ctx, m.seat, from, anchor, []string{"gmail"}, everyMailbox)
		if err != nil {
			t.Fatal(err)
		}
		return chosen.ID
	}
	if got := choose(ids.Nil, anchor); got != m.other {
		t.Errorf("a reply goes out from %s, want the mailbox the message arrived in", got)
	}
	if got := choose(ids.Nil, ids.ActivityID{}); got != m.work {
		t.Errorf("a new mail goes out from %s, want the mailbox connected first", got)
	}
	if got := choose(m.work, anchor); got != m.work {
		t.Errorf("a chosen mailbox gave way to %s", got)
	}
	if got := choose(ids.NewV7(), anchor); !got.IsZero() {
		t.Errorf("a mailbox that is not the sender's chose %s, want none", got)
	}
}

// importsOf counts the seat's import rows, the mail its mailboxes brought in.
func importsOf(t *testing.T, seat ids.UserID) int {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	var n int
	if err := owner.QueryRow(context.Background(), `SELECT count(*) FROM capture_import WHERE user_id = $1`, seat).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAFolderRuleKeepsMailOutOfTheMailboxItWasPickedFromOnly(t *testing.T) {
	t.Parallel()
	m := connectTwoMailboxes(t)
	m.connector.containers = []string{"gmail:Label_7"}
	store := capture.NewExclusionStore(m.db)
	if _, err := store.Add(m.ctx, capture.ExclusionScopeUser, capture.ExclusionKindContainer, "gmail:Label_7", ids.Nil); !errors.Is(err, capture.ErrMailboxAmbiguous) {
		t.Fatalf("a folder rule naming no mailbox among two = %v, want the ask to name one", err)
	}
	if _, err := store.Add(m.ctx, capture.ExclusionScopeUser, capture.ExclusionKindContainer, "gmail:Label_7", m.work); err != nil {
		t.Fatal(err)
	}
	if err := m.reg.SyncOnce(m.ctx, m.work); err != nil {
		t.Fatal(err)
	}
	if got := importsOf(t, m.seat); got != 0 {
		t.Fatalf("the excluded folder let %d messages in", got)
	}
	if err := m.reg.SyncOnce(m.ctx, m.other); err != nil {
		t.Fatal(err)
	}
	if got := importsOf(t, m.seat); got != 1 {
		t.Fatalf("the same label id in the other mailbox kept out %d of 1 message", 1-got)
	}
}

func TestTheMailboxThatCapturedAMessageIsTheOneThatDeliveredIt(t *testing.T) {
	t.Parallel()
	m := connectTwoMailboxes(t)
	owner, _ := setupCaptureDB(t)
	capturing := func() []ids.UUID {
		t.Helper()
		rows, err := owner.Query(context.Background(), `
			SELECT c.id FROM capture_connection c
			  JOIN capture_import ci ON ci.user_id = $1
			  JOIN activity a ON a.id = ci.activity_id
			 WHERE `+storekit.CapturingMailboxSQL("c", "a")+` ORDER BY c.created_at`, m.seat)
		if err != nil {
			t.Fatal(err)
		}
		got, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if err := m.reg.SyncOnce(m.ctx, m.other); err != nil {
		t.Fatal(err)
	}
	if got := capturing(); len(got) != 1 || got[0] != m.other {
		t.Fatalf("capturing mailboxes = %v, want only the one that delivered it", got)
	}
	if err := m.reg.SyncOnce(m.ctx, m.work); err != nil {
		t.Fatal(err)
	}
	if got := capturing(); len(got) != 2 {
		t.Fatalf("capturing mailboxes = %v, want both once both delivered it", got)
	}
}

func TestTheDefaultMailboxSendsNewMail(t *testing.T) {
	t.Parallel()
	m := connectTwoMailboxes(t)
	other := capture.Mailbox{Provider: "gmail", ID: m.other}
	if _, err := m.reg.SetMailboxSettings(m.ctx, other, capture.MailboxSettings{}); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("a settings change naming nothing = %v, want it refused", err)
	}
	view, err := m.reg.SetMailboxSettings(m.ctx, other, capture.MailboxSettings{DefaultSender: true})
	if err != nil || !view.DefaultSender {
		t.Fatalf("marking the default = %+v, %v", view, err)
	}
	chosen, err := m.reg.ChooseSendingMailbox(m.ctx, m.seat, ids.Nil, ids.ActivityID{}, []string{"gmail"},
		func(capture.SendingMailbox) (bool, error) { return true, nil })
	if err != nil || chosen.ID != m.other {
		t.Fatalf("a new mail goes out from %s, %v; want the marked default", chosen.ID, err)
	}
	if _, err := m.reg.SetMailboxSettings(m.ctx, capture.Mailbox{Provider: "gmail", ID: m.work}, capture.MailboxSettings{DefaultSender: true}); err != nil {
		t.Fatal(err)
	}
	views, err := m.reg.Connections(m.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.DefaultSender != (v.ID == m.work) {
			t.Fatalf("after moving the mark, %s default = %v", v.ID, v.DefaultSender)
		}
	}
}

func TestAColleaguesMailboxIsNotReachableByItsID(t *testing.T) {
	t.Parallel()
	m := connectTwoMailboxes(t)
	owner, _ := setupCaptureDB(t)
	colleague, theirs := ids.NewV7(), ids.NewV7()
	if _, err := owner.Exec(context.Background(), `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`,
		colleague, "colleague-"+colleague.String()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(context.Background(), `INSERT INTO capture_connection (id, user_id, provider, status, account_label, provider_scopes)
		VALUES ($1, $2, 'gmail', 'connected', 'colleague@mail.example', '{}')`, theirs, colleague); err != nil {
		t.Fatal(err)
	}
	mailbox := capture.Mailbox{Provider: "gmail", ID: theirs}
	if _, err := m.reg.SetSignatureEnrichment(m.ctx, mailbox, nil); !errors.Is(err, capture.ErrNoConnection) {
		t.Fatalf("setting a colleague's mailbox = %v, want it absent", err)
	}
	if _, err := m.reg.SetMailboxSettings(m.ctx, mailbox, capture.MailboxSettings{DefaultSender: true}); !errors.Is(err, capture.ErrNoConnection) {
		t.Fatalf("marking a colleague's mailbox as default = %v, want it absent", err)
	}
	if _, _, _, err := m.reg.SenderFor(m.ctx, m.seat, mailbox); !errors.Is(err, capture.ErrNoConnection) {
		t.Fatalf("sending through a colleague's mailbox = %v, want no connection", err)
	}
	if err := m.reg.Disconnect(m.ctx, mailbox); err != nil {
		t.Fatal(err)
	}
	if status, _ := connectionStatus(t, theirs); status != "connected" {
		t.Fatalf("disconnecting by a colleague's id left their mailbox %q", status)
	}
}
