// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose_test

// An import and a mailbox describing one message land ONE activity.
//
// This is the onboarding sequence, not an edge: a customer hands over their
// CRM history and THEN connects the mailbox that already held it. Before the
// change these tests hold, the two doors filed under different namespaces —
// the import under the exporting system's name, the mailbox under `email` —
// so every message in the overlap appeared twice.
//
// Both drive the REAL mappers. The import goes through the contract mapper
// every client door shares (activities.LogActivityInputFrom), and the capture
// through mailmap into the production sink, because the namespace decision
// lives in the first and the take-over in the second: a hand-built input would
// assert the fixture's opinion of the key instead of the mapper's.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ownerAddr is the mailbox every test in this file syncs. One address, because
// what these tests vary is the DOOR a message arrives through and the SEAT
// behind it, never which mailbox it landed in.
const ownerAddr = "rep@ws.example"

// importedFromAnotherCRM is the create an importer sends: the exporting
// system's own bookkeeping, and the message's own Message-ID. The subject and
// body are that system's rendering — stripped of the formatting the mailbox
// still holds, which is why the read copy is the better one.
func importedFromAnotherCRM(messageID, subject, body string) crmcontracts.CreateActivityRequest {
	direction := crmcontracts.CreateActivityRequestDirectionInbound
	from := "pat@counterparty.example"
	system := "hubspot"
	sourceID := "engagement-4471"
	return crmcontracts.CreateActivityRequest{
		Kind:         crmcontracts.CreateActivityRequestKindCreateActivityRequestKindEmail,
		Subject:      &subject,
		Body:         &body,
		Direction:    &direction,
		SourceSystem: &system,
		SourceId:     &sourceID,
		Source:       "hubspot_import:145347700",
		RfcMessageId: &messageID,
		Participants: &struct {
			Cc   *[]string `json:"cc,omitempty"`
			From *string   `json:"from,omitempty"`
			To   *[]string `json:"to,omitempty"`
		}{From: &from, To: &[]string{"rep@ws.example"}},
	}
}

// captureWithTakeOver runs one message through the adapter's own mapping and a
// sink wired the way compose wires the production one.
//
// The seam is the point of these tests, so a bare sink would prove nothing: it
// would exercise a sink that cannot take a row over and report the old
// behaviour as the new one. This mirrors compose.capture.go's
// WithAssertedTakeOver and nothing else — the rest of that wiring is other
// tests' subject.
func captureWithTakeOver(
	t *testing.T, e *integration.Env, owner ids.UUID, raw []byte,
) {
	t.Helper()
	// One adapter, because what these tests vary is the DOOR a message arrives
	// through, never which mailbox provider it came from.
	const adapter = "gmail"
	parsed, err := mailmap.Parse(raw, ownerAddr)
	if err != nil {
		t.Fatalf("%s: parsing the message: %v", adapter, err)
	}
	// The two seams compose injects for this path, spelled the same way
	// newCaptureSink spells them. A sink built without WithMessageIdentity
	// files no cross-door identity, so a test that omitted it would prove the
	// take-over and nothing about the deduplication it depends on.
	sink := capture.NewSink(e.DB()).
		WithAssertedTakeOver(activities.TakeOverAssertedActivityTx).
		WithMessageIdentity(
			activities.IdentityKindMail,
			// Meetings resolve on the iCal UID plus the occurrence: a provider's
			// own event id differs per calendar, so two colleagues on one meeting
			// sync two ids for it.
			activities.IdentityKindMeeting,
			activities.MeetingIdentityKey,
			// The proving variant, spelled the way compose/capture.go spells it:
			// a sink wired with the plain resolve cannot bind across seats at
			// all, so a test using one would report the same-seat behaviour as
			// though it were the new rule.
			activities.ResolveBindableIdentityProving(capture.ProvedUnambiguouslyTx),
			activities.ClaimIdentity,
		)
	if _, err := sink.Upsert(connectorCtx(e, adapter, owner), parsed.ToRecord(adapter, raw)); err != nil {
		t.Fatalf("%s: capturing the message: %v", adapter, err)
	}
}

// A connector whose granting human may not update an activity cannot take one
// over, however the identity resolves.
//
// The same-seat resolve answers WHICH row this capture may touch; it says
// nothing about whether this principal may perform an update at all. That is the
// object permission's question, and the two are independent: a passport granted
// create-and-read for ingestion must not gain the ability to rewrite existing
// content because a Message-ID happened to match.
func TestAConnectorWithoutUpdateCannotTakeOverItsOwnImport(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	const messageID = "no-update-grant@counterparty.example"

	importThrough(t, e, owner, importedFromAnotherCRM(
		messageID, "Angebot", "the import's own rendering"))

	// The same seat's mailbox, on a passport that may ingest but not update.
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail",
		UserID: owner, OnBehalfOf: owner,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true},
				"contact":  {Create: true, Read: true},
				"company":  {Create: true, Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	raw := theSameMessage(messageID, "pat@counterparty.example", ownerAddr)
	parsed, err := mailmap.Parse(raw, ownerAddr)
	if err != nil {
		t.Fatalf("parsing the message: %v", err)
	}
	sink := capture.NewSink(e.DB()).
		WithAssertedTakeOver(activities.TakeOverAssertedActivityTx).
		WithMessageIdentity(
			activities.IdentityKindMail,
			// Meetings resolve on the iCal UID plus the occurrence: a provider's
			// own event id differs per calendar, so two colleagues on one meeting
			// sync two ids for it.
			activities.IdentityKindMeeting,
			activities.MeetingIdentityKey,
			activities.ResolveBindableIdentity,
			activities.ClaimIdentity,
		)
	if _, err := sink.Upsert(ctx, parsed.ToRecord("gmail", raw)); err == nil {
		t.Fatal("a connector with no update grant took over an activity")
	}
	// And the import is exactly as it was: the refusal wrote nothing.
	body := columnOfMessage[string](t, e, "body", messageID)
	if body != "the import's own rendering" {
		t.Fatalf("the import's body is %q — a refused take-over still wrote", body)
	}
	capturedBy := columnOfMessage[string](t, e, "captured_by", messageID)
	if capturedBy != "human:"+owner.String() {
		t.Fatalf("the import is stamped %q — a refused take-over still restamped it", capturedBy)
	}
}

// An erasure that lands BETWEEN the resolve and the take-over advances the
// sync rather than stalling it forever.
//
// The resolve reads the holder as live, then an erasure archives it, then the
// take-over's LockRow(LiveOnly) refuses. That refusal is correct — the content
// must not be written back — but it arrives as ErrNotFound, and a capture that
// propagated it would fail this mailbox on this message on every later pass:
// the watermark never advances past a message that errors, so the sync stops
// permanently at one row.
//
// The window is real but narrow, so the test drives it deterministically: the
// injected take-over archives the row inside the capture's own transaction,
// immediately before delegating to the production function. That reproduces the
// exact interleaving through the real seam rather than asserting a stub.
func TestAnErasureRacingTheTakeOverSkipsInsteadOfStalling(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	const messageID = "erasure-race@counterparty.example"

	importThrough(t, e, owner, importedFromAnotherCRM(
		messageID, "Angebot", "the words that were erased"))

	raw := theSameMessage(messageID, "pat@counterparty.example", ownerAddr)
	parsed, err := mailmap.Parse(raw, ownerAddr)
	if err != nil {
		t.Fatalf("parsing the message: %v", err)
	}
	racing := func(
		ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
		subject, body, wasCapturedBy, capturedBy string,
	) error {
		// The erasure wins the race, in this same transaction so the take-over
		// below sees exactly what a concurrent commit would have left it.
		if _, err := tx.Exec(ctx, `
			UPDATE activity SET archived_at = now(), body = NULL, subject = NULL
			 WHERE id = $1`, activityID); err != nil {
			return err
		}
		return activities.TakeOverAssertedActivityTx(
			ctx, tx, activityID, subject, body, wasCapturedBy, capturedBy)
	}
	sink := capture.NewSink(e.DB()).
		WithAssertedTakeOver(racing).
		WithMessageIdentity(
			activities.IdentityKindMail,
			// Meetings resolve on the iCal UID plus the occurrence: a provider's
			// own event id differs per calendar, so two colleagues on one meeting
			// sync two ids for it.
			activities.IdentityKindMeeting,
			activities.MeetingIdentityKey,
			activities.ResolveBindableIdentity,
			activities.ClaimIdentity,
		)
	_, err = sink.Upsert(connectorCtx(e, "gmail", owner), parsed.ToRecord("gmail", raw))

	// A SKIP, not a failure: the connector advances its watermark past this
	// message. Anything else stalls the mailbox here forever.
	if !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("an erasure racing the take-over answered %v, want a skip so the sync advances", err)
	}
}

// A SENT message is not rewritten by a colleague's mailbox syncing it.
//
// This is the SECOND producer of a natural-key collision, and the one the
// cross-seat import test cannot reach. An outbound send writes its timeline row
// under the mail natural key — ('email', <minted Message-ID>) — stamped
// `human:<sender>`, and claims no activity_identity row, because it sets
// SourceID without RFCMessageID (activities/outboundmessage.go).
//
// So a colleague copied on that message syncs it, misses the identity resolve
// entirely, and collides on the natural key instead. That collision is gated by
// EnsureActivityVisible — a DISCOVER check admitting any row the seat can see,
// and being copied on the message is enough. If the take-over fired on that id
// it would overwrite the sender's subject and body with the connector's
// rendering and restamp captured_by to the syncing seat, handing them write
// authority over the sender's row for every later writability check.
//
// The take-over therefore runs only for an id the identity resolve vetted. This
// collision falls through to replayClaimIsProvenTx, which is the right question
// for two mailboxes holding one message.
func TestASentMessageIsNotRewrittenByAColleaguesMailbox(t *testing.T) {
	e := integration.Setup(t)
	sender, colleague := e.Rep1, e.Rep2
	const messageID = "sent-then-colleague@ws.example"
	const sentBody = "the sender's own words"

	// The sent row as the send path writes it: the mail natural key, the
	// sender's own stamp, and no identity claim.
	sent := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, occurred_at, direction,
			                      source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'Angebot', $2, now(), 'outbound',
			        'email', $3, 'manual', $4)`,
			sent, sentBody, messageID, "human:"+sender.String())
		return err
	}); err != nil {
		t.Fatalf("seeding the sent message: %v", err)
	}

	// The colleague's mailbox holds the same message, because they were copied
	// on it, and syncs it.
	//
	// The sync does not land a row, and that is the pre-existing behaviour for
	// this collision rather than anything this change decides:
	// replayClaimIsProvenTx cannot prove the colleague's mailbox holds the same
	// message, so the capture is skipped and its watermark advances. What
	// matters here is that the refusal costs the SENDER nothing. (The skip
	// itself is tracked separately: an unprovable collision should arguably land
	// as the syncing seat's own row, which needs a per-seat replay key.)
	raw := theSameMessage(messageID, "pat@counterparty.example", ownerAddr)
	parsed, err := mailmap.Parse(raw, ownerAddr)
	if err != nil {
		t.Fatalf("parsing the message: %v", err)
	}
	sink := capture.NewSink(e.DB()).
		WithAssertedTakeOver(activities.TakeOverAssertedActivityTx).
		WithMessageIdentity(
			activities.IdentityKindMail,
			// Meetings resolve on the iCal UID plus the occurrence: a provider's
			// own event id differs per calendar, so two colleagues on one meeting
			// sync two ids for it.
			activities.IdentityKindMeeting,
			activities.MeetingIdentityKey,
			activities.ResolveBindableIdentity,
			activities.ClaimIdentity,
		)
	if _, err := sink.Upsert(connectorCtx(e, "gmail", colleague), parsed.ToRecord("gmail", raw)); err != nil &&
		!errors.Is(err, connector.ErrSkip) {
		t.Fatalf("the colleague's sync failed with %v, want a skip or a clean capture", err)
	}

	// The sender's row is untouched: same body, same stamp.
	body := scalar[string](t, e, `SELECT body FROM activity WHERE id = $1`, sent)
	if body != sentBody {
		t.Fatalf("the sent message's body is %q — a colleague's mailbox rewrote it", body)
	}
	capturedBy := scalar[string](t, e, `SELECT captured_by FROM activity WHERE id = $1`, sent)
	if capturedBy != "human:"+sender.String() {
		t.Fatalf("the sent message is stamped %q — a colleague's sync restamped it to themselves, "+
			"which hands them write authority over the sender's row", capturedBy)
	}
}

// A colleague's import is NOT taken over by another seat's mailbox.
//
// This is the limitation the same-seat rule buys, pinned so it stays
// deliberate: an admin who imports the company's history and a rep who then
// connects their own mailbox get two rows for the messages they share. Solving
// it needs a server-derived fact about which seat an imported row belongs to,
// which the importer does not supply today — every column that looks like one
// is written verbatim from the request body.
//
// The duplicate is the SAFE failure. Binding on the evidence available would
// mean one seat's mailbox rewriting another seat's record on the strength of a
// header the message's sender typed.
func TestAColleaguesImportIsNotTakenOverByAnotherSeatsMailbox(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "cross-seat@counterparty.example"

	// Rep2 imports the history; Rep1 connects the mailbox that held the message.
	importThrough(t, e, e.Rep2, importedFromAnotherCRM(
		messageID, "Angebot fuer 10 Plaetze", "hubspot stripped this"))

	captureWithTakeOver(t, e, e.Rep1,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	if got := activitiesDescribing(t, e, messageID); got != 2 {
		t.Fatalf("a colleague's import and this seat's mailbox made %d activities, want 2", got)
	}
	// The importing seat's row is untouched — same stamp, same body.
	capturedBy := columnOfMessage[string](t, e, "captured_by", messageID)
	if capturedBy != "human:"+e.Rep2.String() {
		t.Fatalf("the import is stamped %q, want the colleague who imported it", capturedBy)
	}
	body := columnOfMessage[string](t, e, "body", messageID)
	if body != "hubspot stripped this" {
		t.Fatalf("the import's body is %q — another seat's mailbox rewrote it", body)
	}
}

// eraseImportedMessage archives the row and destroys its content, the way the
// Art. 17 engine does (privacy/activitycontenterasure.go).
//
// keepIdentity says whether the `activity_identity` row survives. The engine
// deletes it, so false is what production does — but the two cases exercise
// different guards, and only the surviving-identity case reaches the resolve's
// liveness filter at all.
func eraseImportedMessage(t *testing.T, e *integration.Env, messageID string, keepIdentity bool) ids.UUID {
	t.Helper()
	imported := scalar[ids.UUID](t, e, `
		SELECT activity_id FROM activity_identity
		 WHERE identity_kind = 'mail' AND identity_key = $1`, messageID)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			UPDATE activity SET archived_at = now(), body = NULL, subject = NULL
			 WHERE id = $1`, imported); err != nil {
			return err
		}
		if keepIdentity {
			return nil
		}
		_, err := tx.Exec(context.Background(),
			`DELETE FROM activity_identity WHERE activity_id = $1`, imported)
		return err
	}); err != nil {
		t.Fatalf("erasing the imported message: %v", err)
	}
	return imported
}

// assertErasedStaysErased reads the tombstone back and fails if a later sync
// wrote the message onto it, or if the sync did not land its own live row.
func assertErasedStaysErased(t *testing.T, e *integration.Env, erased ids.UUID, messageID string) {
	t.Helper()
	var body *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT body FROM activity WHERE id = $1`, erased).Scan(&body)
	}); err != nil {
		t.Fatalf("reading the erased row back: %v", err)
	}
	if body != nil {
		t.Fatalf("the erased row's body is %q — a later sync resurrected erased content", *body)
	}
	// The sync still landed the message as its own live row, rather than failing
	// on it forever: an erasure must not stall that mailbox.
	live := scalar[int](t, e, `
		SELECT count(*) FROM activity
		 WHERE archived_at IS NULL AND source_system = 'email' AND source_id = $1`, messageID)
	if live != 1 {
		t.Fatalf("the later sync landed %d live rows, want 1", live)
	}
}

// An erased message is not resurrected by a later sync.
//
// Erasure archives the row, destroys its content and retires its identity, so
// the mailbox resolves nothing and files its own row.
func TestAnErasedImportIsNotResurrectedByALaterSync(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	const messageID = "erased@counterparty.example"

	importThrough(t, e, owner, importedFromAnotherCRM(
		messageID, "Angebot", "the words that were erased"))
	erased := eraseImportedMessage(t, e, messageID, false)

	captureWithTakeOver(t, e, owner,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	assertErasedStaysErased(t, e, erased, messageID)
}

// An identity row that OUTLIVES its erased activity must not hand the tombstone
// to a later sync.
//
// This is the case the resolve's liveness filter exists for, and the one the
// test above cannot see: there the identity is gone, so the resolve finds
// nothing and the filter is never reached. Here the identity still points at the
// archived row, so the resolve must refuse it on liveness alone.
//
// It is reachable in production two ways — an erasure that failed partway
// through its transaction, and any future archiving path that forgets to retire
// identities. Either way the consequence is the same and severe: the take-over
// would write the message back over content retention destroyed.
func TestAnIdentityOutlivingItsErasedActivityIsNotBoundTo(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	const messageID = "orphan-identity@counterparty.example"

	importThrough(t, e, owner, importedFromAnotherCRM(
		messageID, "Angebot", "the words that were erased"))
	erased := eraseImportedMessage(t, e, messageID, true)

	captureWithTakeOver(t, e, owner,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	assertErasedStaysErased(t, e, erased, messageID)
}

// activitiesDescribing counts the live timeline rows that describe ONE message,
// however each door keyed them.
//
// Asking by source_id alone would not answer the question these tests ask. An
// import that failed to file under the shared mail identity keeps its exporting
// system's key — `engagement-4471` rather than the Message-ID — so a count
// keyed on source_id would not see that row at all and would report ONE
// activity while two sat on the timeline. That is the defect reporting itself
// absent, so the count reaches the row by either key: the message's own
// identity, or the conversation it opened.
func activitiesDescribing(t *testing.T, e *integration.Env, messageID string) int {
	t.Helper()
	return scalar[int](t, e, `
		SELECT count(*) FROM activity
		 WHERE archived_at IS NULL
		   AND (source_id = $1 OR thread_key = $1)`, messageID)
}

// importThrough runs the create through the shared contract mapper and the
// real store, as the HTTP handler and the tool surface both do.
func importThrough(
	t *testing.T, e *integration.Env, seat ids.UUID, req crmcontracts.CreateActivityRequest,
) {
	t.Helper()
	in, err := activities.LogActivityInputFrom(req)
	if err != nil {
		t.Fatalf("mapping the import: %v", err)
	}
	if _, _, err := activities.NewStore(e.DB()).LogActivity(seatCtx(t, e, seat), in); err != nil {
		t.Fatalf("importing the message: %v", err)
	}
}

// seatCtx is the human door: an ordinary authenticated member, which is what
// makes the imported row an ASSERTION rather than an observation.
func seatCtx(t *testing.T, e *integration.Env, seat ids.UUID) context.Context {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(),
		UserID: seat, OnBehalfOf: seat,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true, Update: true},
				"company":  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// The case the customer actually runs: import the history, then connect the
// mailbox. One activity, and the mailbox's reading of the message is the one
// that survives.
func TestImportThenMailboxLandsOneActivityWithTheReadCopy(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	const messageID = "import-then-mailbox@counterparty.example"

	importThrough(t, e, owner, importedFromAnotherCRM(
		messageID, "Angebot fuer 10 Plaetze", "Anbei das Angebot."))

	captureWithTakeOver(t, e, owner,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	if got := activitiesDescribing(t, e, messageID); got != 1 {
		t.Fatalf("one message through both doors made %d activities, want 1", got)
	}

	// The connector's provenance replaced the importer's: the row now says a
	// mailbox read this message, which is what it did.
	capturedBy := columnOfMessage[string](t, e, "captured_by", messageID)
	if capturedBy != "connector:gmail:"+owner.String() {
		t.Fatalf("captured_by is %q, want the connector that read the message", capturedBy)
	}
}

// The reverse order still lands one activity, and must not take the row over:
// here the incumbent is the connector's own reading, and the import has
// nothing better to say about it.
func TestMailboxThenImportLandsOneActivityAndKeepsTheReadCopy(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	const messageID = "mailbox-then-import@counterparty.example"

	captureWithTakeOver(t, e, owner,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	importThrough(t, e, owner, importedFromAnotherCRM(
		messageID, "Angebot fuer 10 Plaetze", "hubspot stripped this"))

	if got := activitiesDescribing(t, e, messageID); got != 1 {
		t.Fatalf("one message through both doors made %d activities, want 1", got)
	}
	body := columnOfMessage[string](t, e, "body", messageID)
	if body == "hubspot stripped this" {
		t.Fatal("the import overwrote the mailbox's own reading of the message")
	}
}

// The import keeps its own bookkeeping. Filing the row under the message's
// identity must not cost the field that says where the row came from —
// "which import brought this in" is a question the source column answers.
func TestAnImportKeepsItsOwnBookkeepingUnderTheSharedIdentity(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "bookkeeping@counterparty.example"

	importThrough(t, e, e.Rep1, importedFromAnotherCRM(
		messageID, "Angebot", "Anbei."))

	// The exporting CRM's own key, untouched. The shared identity is carried
	// in activity_identity beside the row, never by rewriting the row's key:
	// re-keying it would route the import through the natural-key replay
	// path, which answers a taken key with 409 and a free one with 201 — an
	// oracle telling any caller which Message-IDs this installation holds.
	system := columnOfMessage[string](t, e, "source_system", messageID)
	if system != "hubspot" {
		t.Fatalf("source_system is %q, want the exporting system's own", system)
	}
	sourceID := columnOfMessage[string](t, e, "source_id", messageID)
	if sourceID != "engagement-4471" {
		t.Fatalf("source_id is %q, want the exporting system's own record id", sourceID)
	}
	source := columnOfMessage[string](t, e, "source", messageID)
	if source != "hubspot_import:145347700" {
		t.Fatalf("source is %q, want the importer's own bookkeeping", source)
	}
}

// columnOfMessage reads one column of the single activity a Message-ID
// resolves to, through activity_identity — the way production finds it.
//
// Not `WHERE source_id = <the Message-ID>`. Each door keeps its OWN natural
// key, which is the whole point of the shared identity: the import's row is
// still keyed `engagement-4471` by its exporting CRM. A query on source_id
// finds that row only when something has re-keyed it, so it would assert the
// opposite of the design and fail against the correct behaviour.
func columnOfMessage[T any](t *testing.T, e *integration.Env, column, messageID string) T {
	t.Helper()
	return scalar[T](t, e, `
		SELECT `+column+` FROM activity a
		  JOIN activity_identity i ON i.activity_id = a.id
		 WHERE i.identity_kind = 'mail' AND i.identity_key = $1`, messageID)
}

// A planted row must not suppress the message it names, and must not be
// rewritten by the seat that receives it either.
//
// A Message-ID is a header the sender types, so a caller can post an activity
// under one they guessed. Two outcomes are both unacceptable and the test pins
// the third:
//
//   - The victim's capture YIELDS to the planted row: their mail never lands
//     and the planter has suppressed it.
//   - The victim's capture TAKES OVER the planted row: one seat's write has
//     rewritten another seat's record on the strength of a guessed header, and
//     the planter learns which Message-IDs the victim receives.
//
// So the seats do not bind at all. The victim's message lands as their own row,
// the planter keeps the row they filed, and the forgery buys a duplicate that a
// human can see and delete rather than access to anybody's mail.
func TestAPlantedMessageIDNeitherSuppressesNorTakesOverTheRealMessage(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	const messageID = "planted@counterparty.example"

	// A different seat plants a row under a Message-ID they do not hold.
	planter := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Planter')`,
			planter, "planter-"+planter.String()+"@ws.example")
		return err
	}); err != nil {
		t.Fatalf("seeding the planter: %v", err)
	}
	importThrough(t, e, planter, importedFromAnotherCRM(
		messageID, "nothing to see", "planted body"))

	captureWithTakeOver(t, e, owner,
		theSameMessage(messageID, "pat@counterparty.example", ownerAddr))

	// Two rows: the planter's and the victim's. The duplicate IS the intended
	// outcome here — refusing to bind across seats is what costs it.
	if got := activitiesDescribing(t, e, messageID); got != 2 {
		t.Fatalf("a planted row and a real message made %d activities, want 2 — "+
			"one each, because the seats must not bind", got)
	}
	// The planter's row still holds the identity and still says what they wrote:
	// the victim's capture did not reach it.
	body := columnOfMessage[string](t, e, "body", messageID)
	if body != "planted body" {
		t.Fatalf("the planted row's body is %q — the victim's capture rewrote another seat's row", body)
	}
	capturedBy := columnOfMessage[string](t, e, "captured_by", messageID)
	if capturedBy != "human:"+planter.String() {
		t.Fatalf("the planted row is stamped %q, want the planter who wrote it", capturedBy)
	}
	// And the victim's own message landed, under the mail namespace, as theirs.
	victims := scalar[int](t, e, `
		SELECT count(*) FROM activity
		 WHERE archived_at IS NULL AND source_system = 'email' AND source_id = $1
		   AND captured_by = $2`, messageID, "connector:gmail:"+owner.String())
	if victims != 1 {
		t.Fatalf("the victim's own row count is %d, want 1 — their mail was suppressed", victims)
	}
}
