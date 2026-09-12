// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The mail that arrived before the seat was known to hold the address it came
// to.
//
// Alias discovery learns a forwarding address FROM the mail arriving at it, so
// there is always mail that landed before the claim — and for that mail the
// seat is nobody. No import row, so no confidentiality question, so under a
// classified mailbox it stays held with nothing scheduled ever to judge it.
//
// That is not hypothetical. A founder's private clinic wrote twice; the second
// thread reached a forwarding address claimed ninety seconds after the message
// was captured, was never judged, and the retraction read "never judged" as
// "business" and kept the clinic in the shared CRM.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// aliasOnlyMail is a message addressed ONLY to the forwarding alias — no
// mailbox address of the seat's anywhere on it.
//
// That is the shape adoption is about, and the shared deliveredMail builder
// cannot make it: that one always puts the seat's own address in To, so its
// messages import at capture time and are never stranded. A message whose only
// route to this seat is the alias is stranded exactly until the alias is
// claimed.
func aliasOnlyMail(deliveredTo, from, msgID string) []byte {
	return []byte(strings.Join([]string{
		"Delivered-To: " + deliveredTo,
		"Received: from mx.example by mail.google.com",
		"From: " + from,
		"To: " + deliveredTo,
		"Subject: results",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + msgID + ">",
		"Content-Type: text/plain", "", "hello", "",
	}, "\r\n"))
}

// The first message is the one nobody could import. Once the second completes
// the claim, the first is adopted: it gets its import row and the question its
// thread was owed.
func TestTheMailThatArrivedBeforeAnAliasWasClaimedIsAdopted(t *testing.T) {
	env := newCaptureEnv(t)
	setPosture(t, env, env.e.Rep1, capturemod.PostureClassified)

	env.sync(t, aliasOnlyMail(forwardingAlias, "buyer@customer.example", "adopt-1@customer.example"))
	first := activityBySource(t, env, "adopt-1@customer.example")
	if importRowsFor(t, env, first, env.e.Rep1) != 0 {
		t.Fatal("the premise is gone: the first message already had an import row, so this test " +
			"can no longer see whether the claim adopts anything")
	}

	env.sync(t, aliasOnlyMail(forwardingAlias, "other@customer.example", "adopt-2@customer.example"))

	if n := importRowsFor(t, env, first, env.e.Rep1); n != 1 {
		t.Fatalf("the message that arrived before the alias was claimed has %d import rows, want 1 — "+
			"it is on the timeline belonging to nobody, held with nothing scheduled to judge it", n)
	}
	if !threadHasAQuestion(t, env, first) {
		t.Fatal("the adopted message's thread was never asked about — under a classified mailbox " +
			"that is mail held forever, which looks exactly like the product being broken")
	}
}

// A SHARED mailbox gets the import row and no question.
//
// The trap this pins: `pending` renders as HELD, and a question is only opened
// for a classified mailbox. So writing `pending` here would hold the message
// forever with nothing scheduled to free it — the very defect adoption exists
// to fix, moved into a different mailbox.
func TestAdoptingIntoASharedMailboxOpensNoQuestionAndHoldsNothing(t *testing.T) {
	env := newCaptureEnv(t)
	allowSharedPosture(t, env.e)
	setPosture(t, env, env.e.Rep1, capturemod.PostureShared)

	env.sync(t, aliasOnlyMail(forwardingAlias, "buyer@customer.example", "shared-1@customer.example"))
	first := activityBySource(t, env, "shared-1@customer.example")
	env.sync(t, aliasOnlyMail(forwardingAlias, "other@customer.example", "shared-2@customer.example"))

	if n := importRowsFor(t, env, first, env.e.Rep1); n != 1 {
		t.Fatalf("a shared mailbox adopted %d rows for the earlier message, want 1", n)
	}
	if threadHasAQuestion(t, env, first) {
		t.Fatal("a shared mailbox opened a confidentiality question — it has already decided " +
			"its mail is the workspace's, and the answer would change nothing")
	}
	if got := verdictStatusOf(t, env, first, env.e.Rep1); got != "" {
		t.Fatalf("the adopted row carries verdict status %q; on a shared mailbox a status of "+
			"`pending` reads as HELD, so the mail would be held with nobody scheduled to free it", got)
	}
}

// A message a sender forged the delivery header on is not adopted, because it
// never becomes an identity in the first place. The claim is what adoption
// keys on, so the forgery refusal covers both.
func TestAForgedDeliveryAdoptsNothing(t *testing.T) {
	env := newCaptureEnv(t)
	setPosture(t, env, env.e.Rep1, capturemod.PostureClassified)
	const victim = "victim@customer.example"

	env.sync(t, forgedDeliveryMail(victim, "attacker@elsewhere.example", "forge-adopt-1@elsewhere.example"))
	first := activityBySource(t, env, "forge-adopt-1@elsewhere.example")
	env.sync(t, forgedDeliveryMail(victim, "attacker@elsewhere.example", "forge-adopt-2@elsewhere.example"))

	if n := importRowsFor(t, env, first, env.e.Rep1); n != 0 {
		t.Fatalf("a sender's own Delivered-To header adopted %d messages into the seat's mailbox — "+
			"a forgery reached the grant that says whose mail this is", n)
	}
}

// forgedDeliveryMail is the attack in the shape adoption would have to refuse:
// the sender writes the Delivered-To themselves, BELOW the receiving hop's
// Received, and addresses the message to nobody but their target.
func forgedDeliveryMail(victim, from, msgID string) []byte {
	return []byte(strings.Join([]string{
		"Received: from mx.example by mail.google.com",
		"Delivered-To: " + victim,
		"From: " + from,
		"To: " + victim,
		"Subject: results",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + msgID + ">",
		"Content-Type: text/plain", "", "hello", "",
	}, "\r\n"))
}

// The sweep is what reaches mail stranded BEFORE adoption shipped. The claim
// path covers everything from the moment it exists; every mailbox that had
// already gained an alias has messages older than it — one founder's held
// forty-nine, each classified-and-held with nothing scheduled to judge it.
//
// The address is declared through the real writer, and adoption is deliberately
// NOT wired into that door, so the message stays stranded until the sweep runs.
// That is the shape the sweep exists for.
func TestTheSweepAdoptsMailStrandedBeforeTheAddressWasKnown(t *testing.T) {
	env := newCaptureEnv(t)
	setPosture(t, env, env.e.Rep1, capturemod.PostureClassified)
	const late = "founder@late-claim.example"

	env.sync(t, aliasOnlyMail(late, "buyer@customer.example", "stranded-1@customer.example"))
	stranded := activityBySource(t, env, "stranded-1@customer.example")
	if importRowsFor(t, env, stranded, env.e.Rep1) != 0 {
		t.Fatal("the premise is gone: the message already imported, so this test cannot see the sweep work")
	}
	declareOwnAddress(t, env, env.e.Rep1, late)

	worker := compose.NewLinkReconcileWorkspaceWorkerForTest(env.e.Pool, contacts.NewStore(compose.InstallationDB(env.e.Pool)))
	if err := worker.ReconcileWorkspaceForTest(context.Background(), env.e.WS); err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}

	if n := importRowsFor(t, env, stranded, env.e.Rep1); n != 1 {
		t.Fatalf("the sweep left %d import rows on mail the seat holds at their own address, want 1 — "+
			"mail nobody imported is held with nothing scheduled to judge it", n)
	}
	if !threadHasAQuestion(t, env, stranded) {
		t.Fatal("the adopted message's thread was still never asked about")
	}
}

// declareOwnAddress adds the address through the real identity writer, which is
// the door a contact uses when they tell the product about their own alias.
func declareOwnAddress(t *testing.T, env captureEnv, seat ids.UUID, address string) {
	t.Helper()
	store := capturemod.NewOwnerIdentityStore(compose.InstallationDB(env.e.Pool))
	if _, err := store.Add(seatContext(env.e, seat), capturemod.IdentityKindAddress, address); err != nil {
		t.Fatalf("declaring %s as the seat's own: %v", address, err)
	}
}

// activityBySource finds the captured message by the Message-ID it carried.
func activityBySource(t *testing.T, env captureEnv, msgID string) ids.ActivityID {
	t.Helper()
	var id ids.ActivityID
	if err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM activity WHERE source_id = $1`, msgID).Scan(&id)
	}); err != nil {
		t.Fatalf("finding the captured message %s: %v", msgID, err)
	}
	return id
}

func importRowsFor(t *testing.T, env captureEnv, id ids.ActivityID, seat ids.UUID) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2`,
			id, seat).Scan(&n)
	}); err != nil {
		t.Fatalf("counting the import rows: %v", err)
	}
	return n
}

func verdictStatusOf(t *testing.T, env captureEnv, id ids.ActivityID, seat ids.UUID) string {
	t.Helper()
	var status string
	if err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT COALESCE(verdict_status, '') FROM capture_import
			  WHERE activity_id = $1 AND user_id = $2`, id, seat).Scan(&status)
	}); err != nil {
		t.Fatalf("reading the adopted row's verdict status: %v", err)
	}
	return status
}

func threadHasAQuestion(t *testing.T, env captureEnv, id ids.ActivityID) bool {
	t.Helper()
	var open bool
	if err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT EXISTS (
			  SELECT 1 FROM capture_thread_verdict v
			    JOIN activity a ON a.thread_key = v.thread_key
			   WHERE a.id = $1)`, id).Scan(&open)
	}); err != nil {
		t.Fatalf("reading whether the thread was asked about: %v", err)
	}
	return open
}
