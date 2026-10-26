// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the Senders page says about each sender beyond the decision. The
// contact behind them follows the rule that opens a contact. The mail figures
// count only the seat's own stored mail.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sendersReader is a human seat that reads activity, and contact only when
// told to.
func sendersReader(e *integration.Env, user ids.UUID, contactRead bool) context.Context {
	objects := map[string]principal.ObjectGrant{"activity": {Read: true}}
	if contactRead {
		objects["contact"] = principal.ObjectGrant{Read: true}
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType:    principal.SeatFull,
		Permissions: principal.Permissions{Objects: objects, RowScope: principal.RowScopeAll},
	})
}

func sendersByAddress(ctx context.Context, t *testing.T, e *integration.Env) map[string]capture.SenderDecision {
	t.Helper()
	list, err := capture.SendersFor(ctx, InstallationDB(e.Pool), capture.DefaultPersonalPurgeWindows())
	if err != nil {
		t.Fatalf("listing senders: %v", err)
	}
	out := map[string]capture.SenderDecision{}
	for _, d := range list {
		if _, twice := out[d.Address]; twice {
			t.Errorf("%s is listed twice: one sender is one row", d.Address)
		}
		out[d.Address] = d
	}
	return out
}

func seedSenderContact(t *testing.T, e *integration.Env, address string) ids.UUID {
	t.Helper()
	id := e.SeedContact(t, "Sender "+address, nil)
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, $2, 'gmail', 'connector:gmail')`, id, address)
	return id
}

func TestASenderNamesOnlyAContactItsReaderMaySee(t *testing.T) {
	e := integration.Setup(t)
	const open, private, shared, none = "open@kunde.example", "private@kunde.example",
		"shared@kunde.example", "none@kunde.example"
	for _, address := range []string{open, private, shared, none} {
		seedPendingDisposition(t, e, address, "kunde.example", seedCapturedMail(t, e, address, "Betreff"))
	}
	openContact := seedSenderContact(t, e, open)
	e.MakeCapturePrivate(t, "contact", seedSenderContact(t, e, private), e.Rep2)
	sharedContact := seedSenderContact(t, e, shared)
	e.MakeCapturePrivate(t, "contact", sharedContact, e.Rep2)
	e.WsExec(t, `INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
		VALUES ('contact', $1, 'user', $2, 'read', $3)`, sharedContact, e.Rep1, e.Rep2)

	got := sendersByAddress(sendersReader(e, e.Rep1, true), t, e)
	if d := got[open]; !d.RecordExists || d.ContactID == nil || *d.ContactID != openContact || d.ContactName != "Sender "+open {
		t.Errorf("%s = %+v, want the workspace contact named, with its name", open, d)
	}
	if d := got[private]; d.RecordExists || d.ContactID != nil {
		t.Errorf("%s = %+v: a colleague's private contact was disclosed", private, d)
	}
	if d := got[shared]; !d.RecordExists || d.ContactID == nil || *d.ContactID != sharedContact {
		t.Errorf("%s = %+v, want the contact shared with the reader named", shared, d)
	}
	if d := got[none]; d.RecordExists || d.ContactID != nil {
		t.Errorf("%s = %+v, want no contact", none, d)
	}

	for address, d := range got {
		if (d.ContactID == nil) != (d.ContactName == "") {
			t.Errorf("%s carries a contact name without its id, or the reverse (%+v)", address, d)
		}
	}
	for address, d := range sendersByAddress(sendersReader(e, e.Rep1, false), t, e) {
		if d.RecordExists || d.ContactID != nil || d.ContactName != "" {
			t.Errorf("a seat without contact read learned of %s's contact (%+v)", address, d)
		}
	}
}

func TestASendersMailIsCountedFromTheSeatsOwnMailbox(t *testing.T) {
	e := integration.Setup(t)
	const sender = "zaehlen@kunde.example"
	now := clocktest.Now(t).UTC().Truncate(time.Microsecond)
	older, newer := now.Add(-48*time.Hour), now.Add(-time.Hour)
	first := seedCapturedMail(t, e, sender, "Erste")
	second := seedCapturedMail(t, e, sender, "Zweite")
	archived := seedCapturedMail(t, e, sender, "Archiviert")
	restricted := seedCapturedMail(t, e, sender, "Gesperrt")
	theirs := seedCapturedMail(t, e, sender, "Fremd")
	seedPendingDisposition(t, e, sender, "kunde.example", first)
	e.WsExec(t, `UPDATE activity SET occurred_at = $2 WHERE id = $1`, first, older)
	e.WsExec(t, `UPDATE activity SET occurred_at = $2 WHERE id = $1`, second, newer)
	e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, archived)
	e.WsExec(t, `UPDATE activity SET occurred_at = $2 WHERE id = $1`, restricted, now)
	restrict(t, e, restricted)
	for _, id := range []ids.UUID{first, second, archived, restricted} {
		e.WsExec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, id, e.Rep1)
	}
	e.WsExec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, theirs, e.Rep2)

	d := sendersByAddress(sendersReader(e, e.Rep1, true), t, e)[sender]
	if d.MessageCount != 2 {
		t.Errorf("message count = %d, want 2: the seat's own stored mail, archived, restricted and a colleague's import excluded",
			d.MessageCount)
	}
	if d.LastSeenAt == nil || !d.LastSeenAt.Equal(newer) {
		t.Errorf("last seen = %v, want the newest message at %v", d.LastSeenAt, newer)
	}
	if d.LastSeenPrecision != "instant" {
		t.Errorf("last seen precision = %q, want the newest message's instant", d.LastSeenPrecision)
	}
}

// The ledger keeps a closed question beside the one its sender's next message
// opened. The page shows the sender once, with the newest answer.
func TestOneSenderIsOneRowAcrossLedgerRows(t *testing.T) {
	e := integration.Setup(t)
	const sender = "wieder@kunde.example"
	closed := seedPendingDisposition(t, e, sender, "kunde.example", seedCapturedMail(t, e, sender, "Alt"))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_pending_counterparty
			   SET status = 'rejected', resolved_at = now(), created_at = now() - interval '2 days'
			 WHERE id = $1`, closed)
		return err
	}); err != nil {
		t.Fatalf("closing the first question: %v", err)
	}
	seedPendingDisposition(t, e, sender, "kunde.example", seedCapturedMail(t, e, sender, "Neu"))

	if d := sendersByAddress(sendersReader(e, e.Rep1, true), t, e)[sender]; d.Status != "pending" {
		t.Errorf("status = %q, want the newest question's pending", d.Status)
	}
}

// The sweep reads every ledger row of the seat for an address. A personal
// verdict behind a fresh question still names its deletion, and the override
// still cancels it.
func TestAnOlderPersonalVerdictStillNamesItsDeletion(t *testing.T) {
	e := integration.Setup(t)
	const sender = "mama@familie.example"
	resolvePersonal(t, e, sender, seedPurgeableMail(t, e, sender, "Sonntag", e.Rep1), e.Rep1, true)
	e.WsExec(t, `UPDATE capture_pending_counterparty SET created_at = now() - interval '2 days' WHERE email = $1`, sender)
	seedPendingDisposition(t, e, sender, "familie.example", seedCapturedMail(t, e, sender, "Neu"))

	d := sendersByAddress(sendersReader(e, e.Rep1, true), t, e)[sender]
	if d.Status != "pending" || d.DeletesAt.IsZero() {
		t.Errorf("%s = %+v, want the fresh question shown and the older verdict's deletion named", sender, d)
	}
	overruleAsBusiness(t, e, e.Rep1, sender)
	if d := sendersByAddress(sendersReader(e, e.Rep1, true), t, e)[sender]; !d.DeletesAt.IsZero() {
		t.Errorf("deletes at %v after the owner said business, want no deletion", d.DeletesAt)
	}
}

// An exclusion names the member who added it, and nobody for a rule no member
// added or one whose member has left.
func TestAnExclusionNamesTheMemberWhoAddedIt(t *testing.T) {
	e := integration.Setup(t)
	store := capture.NewExclusionStore(InstallationDB(e.Pool))
	seat := purgeCtx(e, e.Rep1)
	added, err := store.Add(seat, capture.ExclusionScopeUser, capture.ExclusionKindDomain, "newsletter.example")
	if err != nil {
		t.Fatalf("adding the rule: %v", err)
	}
	name := e.WsScalar(t, `SELECT display_name FROM app_user WHERE id = $1`, e.Rep1)
	if added.CreatedByName == nil || *added.CreatedByName != name {
		t.Errorf("the added rule names %v, want %q", added.CreatedByName, name)
	}
	e.WsExec(t, `INSERT INTO capture_exclusion (scope, user_id, kind, value, created_by)
		VALUES ('user', $1, 'domain', 'system.example', 'system:seed')`, e.Rep1)

	names := func() map[string]*string {
		rules, err := store.List(seat)
		if err != nil {
			t.Fatalf("listing the rules: %v", err)
		}
		out := map[string]*string{}
		for _, rule := range rules {
			out[rule.Value] = rule.CreatedByName
		}
		return out
	}
	if got := names()["system.example"]; got != nil {
		t.Errorf("a rule no member added names %q", *got)
	}
	e.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep1)
	if got := names()["newsletter.example"]; got != nil {
		t.Errorf("a rule names %q, who has been deactivated", *got)
	}
}
