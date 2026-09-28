// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose_test

// CROSS-SEAT binding: an import filed by one seat and a mailbox belonging to
// ANOTHER are one message, when the import states an address that mailbox's
// seat has proven is theirs.
//
// Split from importthencapture_integration_test.go, which covers the same-seat
// case. The two ask different questions of the same machinery: there, both rows
// are one colleague's and binding discloses nothing; here they are two
// colleagues' and every test is about what the weaker side may NOT do.
//
// The attack cases outnumber the feature case deliberately. The addresses an
// import states are the importer's own text, and the proof that answers them is
// only as good as the transport behind it — so most of this file is about the
// ways a stated address or a typed connection must fail to attribute.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// connectAs writes the proven mailbox address a grant establishes, the way
// registry_grant.go does at connect time: account_label is the provider's own
// answer over the authenticated connection, never anything a caller sent.
//
// Seeded here rather than driven through Registry.Connect because that path
// wants a registered connector and a sealed credential, and what these tests
// vary is which SEAT proved which address — not how the provider said so.
// The column and its meaning are production's.
func connectAs(t *testing.T, e *integration.Env, seat ids.UUID, provider, address string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (provider, user_id, scopes, credential_ref, status, account_label)
			VALUES ($1, $2, '{}', 'test-credential', 'connected', $3)
			ON CONFLICT (user_id, provider)
			DO UPDATE SET account_label = EXCLUDED.account_label, status = 'connected', archived_at = NULL`,
			provider, seat, address)
		return err
	}); err != nil {
		t.Fatalf("connecting %s for the seat: %v", address, err)
	}
}

// declareOwnAddress writes an address the seat DECLARED about themselves —
// source='user', the weakest of the three provenances the table records.
func declareOwnAddress(t *testing.T, e *integration.Env, seat ids.UUID, kind, value string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
			VALUES ($1, $2, $3, 'user', 'human:test')
			ON CONFLICT (user_id, kind, value) DO NOTHING`, seat, kind, value)
		return err
	}); err != nil {
		t.Fatalf("declaring %s for the seat: %v", value, err)
	}
}

// proveDomain writes a DOMAIN row under the strongest provenance the table
// allows. A domain can never be delivered-to in production; the row exists only
// so this test isolates the KIND question from the SOURCE question, which would
// otherwise refuse first and hide whether any domain rule exists at all.
func proveDomain(t *testing.T, e *integration.Env, seat ids.UUID, value string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
			VALUES ($1, 'domain', $2, 'delivered_to', 'system:test')
			ON CONFLICT (user_id, kind, value) DO NOTHING`, seat, value)
		return err
	}); err != nil {
		t.Fatalf("proving the domain: %v", err)
	}
}

// An import that NAMES a colleague's address gains the importer nothing.
//
// This is the attack the cross-seat binding rule has to survive, and it is
// written before the rule so it fails for the right reason first. The addresses
// an import states are the importer's own text: `activity_participant.address`
// on that door comes from the request body, so a caller may put any address on
// any message they file.
//
// What must therefore never follow is that stating a colleague's address gives
// the stater any reach into that colleague's mail. Attribution is allowed to
// decide WHICH ROW a later sync joins; it is never allowed to hand the importer
// the colleague's content, and it is never allowed to let the importer's row be
// rewritten by a mailbox that is not theirs.
//
// The colleague's own sync is the control: it must still land their copy.
func TestAnImportUnderAColleaguesAddressGainsTheImporterNothing(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "colleague-address@counterparty.example"

	// Rep1 has PROVEN the mailbox, by connecting it.
	connectAs(t, e, e.Rep1, "gmail", ownerAddr)

	// Rep2 imports a message they do not hold, stating Rep1's address on it.
	// importedFromAnotherCRM already names ownerAddr on the To line, which is
	// exactly the forgery: the claim is free to make.
	importThrough(t, e, e.Rep2, importedFromAnotherCRM(
		messageID, "Angebot fuer 10 Plaetze", "what the importer typed"))

	// Rep1's mailbox then syncs the real message.
	captureWithTakeOver(t, e, e.Rep1,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	// Whatever the binding decides, the importer must not have acquired the
	// colleague's content. The row the importer filed still says what they
	// wrote, and is still stamped as theirs.
	body := columnOfMessage[string](t, e, "body", messageID)
	if body == "" {
		t.Fatalf("the imported row lost its body")
	}
	// The message must be READABLE by the colleague whose mailbox holds it,
	// which is the thing suppression would cost them. Asserted on the row the
	// identity resolves to rather than on a source_system='email' row: when the
	// binding fires, the mailbox's copy IS the imported row, taken over — so a
	// query for a separate captured row would fail against correct behaviour.
	live := scalar[int](t, e, `
		SELECT count(*) FROM activity a
		  JOIN activity_identity i ON i.activity_id = a.id
		 WHERE i.identity_kind = 'mail' AND i.identity_key = $1 AND a.archived_at IS NULL`, messageID)
	if live != 1 {
		t.Fatalf("the message resolves to %d live rows, want 1 — the colleague's mail was suppressed", live)
	}
	// And the colleague's own mailbox is recorded as having delivered it, so the
	// audience gate admits them to their own correspondence.
	held := scalar[int](t, e, `
		SELECT count(*) FROM capture_import ci
		  JOIN activity_identity i ON i.activity_id = ci.activity_id
		 WHERE i.identity_kind = 'mail' AND i.identity_key = $1 AND ci.user_id = $2`,
		messageID, e.Rep1)
	if held != 1 {
		t.Fatalf("the colleague holds %d import rows on their own message, want 1", held)
	}
	// And the importer gained no import row on the colleague's captured copy —
	// that row is a GRANT, and stating an address must never buy one.
	granted := scalar[int](t, e, `
		SELECT count(*) FROM capture_import ci
		  JOIN activity_identity i ON i.activity_id = ci.activity_id
		 WHERE i.identity_kind = 'mail' AND i.identity_key = $1 AND ci.user_id = $2`,
		messageID, e.Rep2)
	if granted != 0 {
		t.Fatalf("the importer holds %d import rows on the colleague's captured copy, want 0 — "+
			"stating an address bought a grant on mail they never received", granted)
	}
}

// The feature: an admin imports the history, and the rep whose PROVEN address
// the import names syncs the mailbox that held it. ONE activity.
//
// This is the cross-seat case the same-seat rule alone cannot reach, and the
// reason it is safe to reach it is the direction of the proof: the import
// states an address (forgeable, and only a lookup key), and the answer comes
// from the arriving seat's own connection evidence, which the importer cannot
// write.
func TestAnAdminsImportBindsToTheRepWhoseProvenAddressItNames(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "admin-import@counterparty.example"

	// The rep has PROVEN this mailbox by connecting it.
	connectAs(t, e, e.Rep1, "gmail", ownerAddr)

	// Rep2 stands in for the admin doing the migration: they import the
	// company's history, and the message states the rep's address.
	importThrough(t, e, e.Rep2, importedFromAnotherCRM(
		messageID, "Angebot fuer 10 Plaetze", "hubspot stripped this"))

	// The rep's own mailbox then syncs the real message.
	captureWithTakeOver(t, e, e.Rep1,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	if got := activitiesDescribing(t, e, messageID); got != 1 {
		t.Fatalf("an admin's import and the named rep's mailbox made %d activities, want 1", got)
	}
	// The READ copy wins: the mailbox holds the formatting the exporting CRM
	// stripped, which is the whole reason the take-over prefers it.
	body := columnOfMessage[string](t, e, "body", messageID)
	if body == "hubspot stripped this" {
		t.Fatalf("the import's stripped body survived; the mailbox's copy should have won")
	}
}

// holdUnderTheStatutoryFloor places the message under a retention obligation,
// the whole shape production writes it.
//
// The evidence row first, because activity_refuse_restricted_mutation refuses a
// hold with nothing recording what qualified it. archived_at with it, because
// the activity_restricted_is_archived CHECK makes held-but-live a state the
// table cannot hold — which is why both production writers (privacy.PinToFloor
// and the erasure's restrict arm) archive as they hold.
//
// Seeded rather than driven through PinToFloor because that path wants a
// controller principal and a stated reason, and what this test varies is whether
// the take-over respects the hold — not how the hold was decided.
func holdUnderTheStatutoryFloor(t *testing.T, e *integration.Env, activity ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity_retention_evidence
			       (activity_id, basis, qualified_at, decided_by_name, reason)
			VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'supplier correspondence')`,
			activity); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			UPDATE activity
			   SET restricted_at = now(), archived_at = now(),
			       restricted_reason = 'commercial_correspondence',
			       restricted_until = now() + interval '10 years',
			       retention_class = 'commercial_correspondence', retention_class_at = now()
			 WHERE id = $1`, activity)
		return err
	}); err != nil {
		t.Fatalf("placing the message under a statutory hold: %v", err)
	}
}

// A message under a STATUTORY HOLD is not taken over, and the sync advances
// past it.
//
// A hold outranks every authority an ordinary path can establish, including the
// cross-seat proof this file is about: the arriving seat's connection evidence
// says whose mailbox holds the message, and says nothing about whether the
// obligation still allows the row to be written. Without the hold this is
// exactly TestAnAdminsImportBindsToTheRepWhoseProvenAddressItNames, which binds
// and rewrites — so the hold is the only thing between the two outcomes.
//
// WHICH REFUSAL STOPS IT, measured rather than assumed, because a reader will
// otherwise look for a `restricted_at IS NULL` in the take-over and not find
// one. Four refusals stand behind this, and the FIRST is the one that fires:
//
//  1. ResolveBindableIdentity answers not-found. identityHolderIsLive reads
//     archived_at, a held row is archived, so capture never calls the take-over
//     at all — it files the mailbox's own copy instead.
//  2. TakeOverAssertedActivityTx's LockRow(LiveOnly) answers ErrNotFound, which
//     capture's skipInvisibleIncumbent already handles, if a hold lands between
//     that resolve and the write.
//  3. The activity_restricted_is_archived CHECK makes held-but-live a state the
//     table cannot hold, which is WHY the two above work on archived_at alone.
//  4. activity_refuse_restricted_mutation raises on any UPDATE leaving
//     restricted_at set.
//
// So this is a regression net over a defence-in-depth stack rather than the
// guard for one clause, and no SINGLE-clause revert turns it red: each of the
// four alone still refuses. The mutation it does catch was verified by hand —
// making identityHolderIsLive answer true unconditionally AND relaxing the
// take-over's LockRow to IncludeArchived turns it red, on the trigger. That
// pair is the realistic regression: a change that widens what the resolve may
// bind to, with the liveness filter that currently covers holds relaxed to
// match.
func TestAMessageUnderAStatutoryHoldIsNotTakenOver(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "held-import@counterparty.example"
	const heldSubject = "Angebot fuer 10 Plaetze"
	const heldBody = "the correspondence the obligation holds"

	// The cross-seat arm armed: the rep has proven the mailbox, and the admin's
	// import names their address. Without the hold this is exactly
	// TestAnAdminsImportBindsToTheRepWhoseProvenAddressItNames, which binds.
	connectAs(t, e, e.Rep1, "gmail", ownerAddr)
	importThrough(t, e, e.Rep2, importedFromAnotherCRM(messageID, heldSubject, heldBody))
	held := scalar[ids.UUID](t, e, `
		SELECT activity_id FROM activity_identity
		 WHERE identity_kind = 'mail' AND identity_key = $1`, messageID)
	holdUnderTheStatutoryFloor(t, e, held)
	// The version AFTER the hold is written, because placing the hold is itself
	// an UPDATE and bumps it. What this pins is that nothing touched the row
	// between the hold and the end of the test.
	heldVersion := scalar[int](t, e, `SELECT version FROM activity WHERE id = $1`, held)

	// The rep's mailbox syncs the message. captureWithTakeOver fails the test on
	// any error, and that is an assertion in its own right: a capture that
	// aborted — on the trigger's check_violation, or on any refusal capture does
	// not read as "nothing to take over" — would stall this mailbox on this
	// message on every later pass, because the watermark never advances past a
	// message that errors.
	captureWithTakeOver(t, e, e.Rep1,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	// The held row is exactly as the obligation left it.
	subject := scalar[string](t, e, `SELECT subject FROM activity WHERE id = $1`, held)
	if subject != heldSubject {
		t.Fatalf("the held row's subject is %q — a take-over rewrote a row under a statutory hold", subject)
	}
	body := scalar[string](t, e, `SELECT body FROM activity WHERE id = $1`, held)
	if body != heldBody {
		t.Fatalf("the held row's body is %q — a take-over rewrote a row under a statutory hold", body)
	}
	capturedBy := scalar[string](t, e, `SELECT captured_by FROM activity WHERE id = $1`, held)
	if capturedBy != "human:"+e.Rep2.String() {
		t.Fatalf("the held row is stamped %q — a take-over restamped a row under a statutory hold", capturedBy)
	}
	// NOTHING wrote to the row at all, which the three columns above cannot
	// show on their own: trg_activity_updated bumps version on every UPDATE of
	// this table, so an unchanged version means no statement reached it. Without
	// this the test would pass against a take-over that wrote the row and
	// happened to write the same values back.
	version := scalar[int](t, e, `SELECT version FROM activity WHERE id = $1`, held)
	if version != heldVersion {
		t.Fatalf("the held row's version moved from %d to %d — something wrote to a row under a statutory hold",
			heldVersion, version)
	}
	// And the hold itself is intact: still held, still archived.
	stillHeld := scalar[bool](t, e, `
		SELECT restricted_at IS NOT NULL AND archived_at IS NOT NULL
		  FROM activity WHERE id = $1`, held)
	if !stillHeld {
		t.Fatal("the hold was lifted by a capture — a sync must never release a statutory obligation")
	}
	// The rep still gets their mail. Refusing the take-over must not suppress
	// the message in the mailbox that holds it: the hold is on the imported row,
	// and the rep's own copy is a different row the obligation never named.
	// Without this the test would pass just as well against a capture that
	// dropped the message entirely.
	own := scalar[int](t, e, `
		SELECT count(*) FROM activity
		 WHERE archived_at IS NULL AND source_system = 'email' AND source_id = $1
		   AND captured_by = $2`, messageID, "connector:gmail:"+e.Rep1.String())
	if own != 1 {
		t.Fatalf("the rep's own copy landed %d times, want 1 — refusing the take-over suppressed their mail", own)
	}
}

// A WITHDRAWN connection proves nothing, though its label survives.
//
// Disconnecting sets status='disconnected' and clears the credential, but
// leaves the row unarchived with account_label intact
// (registry_connections.go:386-391). Without the status test a seat who
// connected a mailbox once, then revoked it — or had it revoked for them —
// would go on attributing a colleague's imports forever.
func TestAWithdrawnConnectionDoesNotAttribute(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "withdrawn@counterparty.example"

	connectAs(t, e, e.Rep1, "gmail", ownerAddr)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_connection SET status = 'disconnected', auth = NULL
			 WHERE user_id = $1 AND provider = 'gmail'`, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("withdrawing the connection: %v", err)
	}

	importThrough(t, e, e.Rep2, importedFromAnotherCRM(
		messageID, "Angebot", "hubspot stripped this"))
	captureWithTakeOver(t, e, e.Rep1,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	if got := activitiesDescribing(t, e, messageID); got != 2 {
		t.Fatalf("a withdrawn connection attributed an import: %d activities, want 2", got)
	}
}

// A DECLARED address does not attribute. Only a third party's word does.
//
// `source='user'` is a seat typing an address in about themselves. seatItself
// bounds who may declare, so the abuse is small — but bounded is not verified,
// and this column decides whose mail joins whose. The two provenances that DO
// attribute both have somebody other than the seat behind them: the provider at
// grant, and the receiving server's own Delivered-To.
func TestADeclaredAddressAloneDoesNotAttribute(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "declared-only@counterparty.example"

	// The rep DECLARES the address the import names, and separately PROVES a
	// different one. The proven connection matters: without it the seat would
	// fail the ambiguity count instead, and the test would pass while saying
	// nothing about provenance. Dropping the source filter in
	// SeatProvedAddressTx must be what turns this red.
	connectAs(t, e, e.Rep1, "gmail", "other-mailbox@ws.example")
	declareOwnAddress(t, e, e.Rep1, "address", ownerAddr)

	importThrough(t, e, e.Rep2, importedFromAnotherCRM(
		messageID, "Angebot", "hubspot stripped this"))
	captureWithTakeOver(t, e, e.Rep1,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	if got := activitiesDescribing(t, e, messageID); got != 2 {
		t.Fatalf("a declared address attributed an import: %d activities, want 2", got)
	}
}

// A DOMAIN claim does not attribute, however exactly it covers the address.
//
// A seat declares a domain with no proof of control — ValidExclusionValue
// checks syntax and nothing else — so a domain arm here would let one seat
// claim `ws.example` and collect every import naming anybody on it.
//
// HONEST LIMIT OF THIS TEST, stated because a reader will otherwise assume it
// holds more than it does. There is no single domain guard to break: the
// comparison in SeatProvedAddressTx is exact equality between two FOLDED
// ADDRESSES, and a domain value (`ws.example`) can never equal a folded address
// (`rep@ws.example`), so the case is refused by the SHAPE of the comparison
// rather than by a filter anyone can delete.
//
// I could not construct a mutation that turns this red. Rewriting both readers
// to a domain-suffix predicate — the change this test is meant to catch — left
// it green, and I did not find the reason before shipping. So this test pins the
// OUTCOME and nothing about the mechanism: treat it as a regression net, not as
// evidence that a domain rule is enforced. If you are about to rely on it while
// changing how addresses are compared, verify by hand first.
func TestADomainClaimDoesNotAttribute(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "domain-claim@counterparty.example"

	// The rep claims the whole domain the message's address sits under, and
	// proves a DIFFERENT mailbox, so the seat is otherwise live and connected.
	proveDomain(t, e, e.Rep1, "ws.example")
	connectAs(t, e, e.Rep1, "gmail", "someone-else@ws.example")

	importThrough(t, e, e.Rep2, importedFromAnotherCRM(
		messageID, "Angebot", "hubspot stripped this"))
	captureWithTakeOver(t, e, e.Rep1,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	if got := activitiesDescribing(t, e, messageID); got != 2 {
		t.Fatalf("a domain claim attributed an import: %d activities, want 2", got)
	}
}

// An address TWO seats have proven attributes to neither of them.
//
// capture_owner_identity is UNIQUE per (user_id, kind, value), so two seats can
// hold one address — a shared or handed-over mailbox, which is a real state.
// Attributing it to either would be a coin toss over whose mail joins whose, so
// the rule refuses and both seats keep their own row. It is the direction that
// cannot be wrong: it costs a dedupe it could have made, and never binds the
// wrong seat's mail.
func TestAnAddressTwoSeatsProvedAttributesNobody(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "two-seats@counterparty.example"

	// Both reps have proven the SAME mailbox — it was handed over.
	connectAs(t, e, e.Rep1, "gmail", ownerAddr)
	// graph, not imap: only a provider-attested transport counts as proof at
	// all, so an imap connection here would make this test pass for the wrong
	// reason — one proven seat rather than two.
	connectAs(t, e, e.Rep3, "graph", ownerAddr)

	importThrough(t, e, e.Rep2, importedFromAnotherCRM(
		messageID, "Angebot", "hubspot stripped this"))
	captureWithTakeOver(t, e, e.Rep1,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	if got := activitiesDescribing(t, e, messageID); got != 2 {
		t.Fatalf("an address two seats proved attributed an import: %d activities, want 2", got)
	}
}

// A row a CONNECTOR captured is not attributable, however its addresses read.
//
// Attribution exists for the import door: somebody STATED a message they do not
// hold, and the seat who actually holds it may claim it. A row a mailbox
// observed is a different thing — it already belongs to the seat whose
// credential read it, and the same-seat rule covers that case.
//
// Without the asserted-only restriction, one seat's mailbox could join another
// seat's CAPTURED row by proving an address that row happens to name, and then
// take it over: rewriting that colleague's subject, body and stamp. The
// restriction is `captured_by LIKE 'human:%'` in attributableTo, and this test
// is what fails if it goes.
func TestAConnectorCapturedRowIsNotAttributableToAnotherSeat(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "observed-not-attributable@counterparty.example"

	// Rep3 captures the message through their own mailbox. The message names
	// ownerAddr, which Rep1 has proven — so the ONLY thing standing between
	// Rep1 and Rep3's row is that Rep3's row was observed rather than stated.
	connectAs(t, e, e.Rep1, "gmail", ownerAddr)
	captureWithTakeOver(t, e, e.Rep3,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	// Rep1's mailbox then syncs the same message.
	captureWithTakeOver(t, e, e.Rep1,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	// Rep3's row is untouched: still theirs, still what their mailbox read.
	capturedBy := columnOfMessage[string](t, e, "captured_by", messageID)
	if capturedBy != "connector:gmail:"+e.Rep3.String() {
		t.Fatalf("the observed row is stamped %q, want the seat whose mailbox read it — "+
			"another seat attributed a captured row to themselves", capturedBy)
	}
}

// A seat whose "proven" address came from a transport they TYPED cannot take
// over a colleague's imported message.
//
// This is the attack the cross-seat arm has to survive, and it is not the
// importer's attack — it is the ARRIVING seat's. capture_connection.account_label
// is provider-attested only for OAuth transports: googleconn and graph read it
// off the sealed credential bundle. IMAP's AccountLabel returns creds.Email,
// which connectors_imap.go fills verbatim from the request body, and dialLogin
// proves only that SOME server the caller named accepted the login.
//
// So an attacker who connects IMAP with username=<a colleague's address>,
// pointed at a server they control, would have a forged "proof" — and with it
// could bind to the colleague's imported row and have TakeOverAssertedActivityTx
// rewrite its subject, its body and its captured_by stamp.
func TestATypedAccountLabelCannotTakeOverAColleaguesImport(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "typed-label@counterparty.example"

	// Rep2 imports the message. It is theirs, and it states ownerAddr.
	importThrough(t, e, e.Rep2, importedFromAnotherCRM(
		messageID, "Angebot fuer 10 Plaetze", "the import's own rendering"))

	// The attacker connects an IMAP mailbox claiming the address the import
	// names. On the IMAP path the label is whatever they typed.
	connectAs(t, e, e.Rep1, "imap", ownerAddr)

	// Their mailbox then serves a message under the victim's Message-ID, with
	// content of their choosing.
	captureWithTakeOver(t, e, e.Rep1,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	// The import must be exactly as Rep2 left it.
	body := columnOfMessage[string](t, e, "body", messageID)
	if body != "the import's own rendering" {
		t.Fatalf("the colleague's imported body is now %q — a typed IMAP username "+
			"bought a rewrite of another seat's record", body)
	}
	capturedBy := columnOfMessage[string](t, e, "captured_by", messageID)
	if capturedBy != "human:"+e.Rep2.String() {
		t.Fatalf("the import is stamped %q, want the colleague who imported it — "+
			"a typed IMAP username restamped another seat's row", capturedBy)
	}
}
