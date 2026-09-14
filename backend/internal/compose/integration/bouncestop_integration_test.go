// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A dead address stops being written to, end to end.
//
// The engine could always REFUSE on a hard bounce — `hard_bounce` is in
// communication_suppression's kind CHECK and consent maps it to
// ReasonHardBounce — and nothing ever wrote one, so the refusal was
// unreachable. These drive the whole chain the way production does: a real
// send, a real delivery report, the observer compose wires, and then the
// engine's own answer about whether the next message may go.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// bounceEnv is one contact with one sent message, ready for a delivery report.
type bounceEnv struct {
	e          *Env
	comms      *comms.Store
	contactID  ids.UUID
	address    string
	deliveryID ids.UUID
	sender     context.Context
	reporter   context.Context
}

// observedBounceStore is the comms store as COMPOSE builds it for the bounce
// sink — with the observer that stops the address. A store without one marks
// the bounce and stops nothing, which is exactly the silence these tests exist
// to catch, so the fixture must not build the plain store by accident.
func observedBounceStore(e *Env) *comms.Store {
	return comms.NewStore(e.DB(), time.Now, activities.NewStore(e.DB()),
		comms.WithBounceObserver(bounceToConsent{}))
}

// bounceToConsent mirrors compose/capturebounce.go's own adapter. It is spelled
// again here rather than exported, because the production one is unexported and
// making it public to satisfy a test would widen the module's surface for no
// caller.
type bounceToConsent struct{}

func (bounceToConsent) HardBounceTx(ctx context.Context, tx pgx.Tx, fact comms.HardBounceFact) error {
	return consent.RecordHardBounceTx(ctx, tx, consent.HardBounceFact{
		Address: fact.Address, DeliveryID: fact.DeliveryID,
	})
}

func setupBounce(t *testing.T, address string) *bounceEnv {
	t.Helper()
	e := Setup(t)
	owner := OwnerConn(t)

	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Anna Weber", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	activityID := ids.New[ids.ActivityKind]()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, source, captured_by) VALUES ($1, 'email', 'test', 'human:x')`,
		activityID); err != nil {
		t.Fatalf("seeding the send's activity: %v", err)
	}
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity_link (id, activity_id, entity_type, contact_id) VALUES ($1, $2, 'contact', $3)`,
		ids.NewV7(), activityID, ids.UUID(contact.Id)); err != nil {
		t.Fatalf("filing the send under the contact: %v", err)
	}

	env := &bounceEnv{
		e: e, comms: observedBounceStore(e),
		contactID: ids.UUID(contact.Id), address: address,
		sender: e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms),
	}
	var deliveryID ids.UUID
	if err := database.WithWorkspaceTx(env.sender, e.Pool, func(tx pgx.Tx) error {
		var txErr error
		deliveryID, txErr = env.comms.StageTx(env.sender, tx, comms.StageInput{
			ActivityID: activityID, Provider: "gmail", MessageID: "dead@myco.test",
			Recipients: []string{address}, Cc: []string{},
			Subject: "Proposal", Body: "As discussed.", ConsentPurpose: "transactional",
			References: []string{},
		})
		return txErr
	}); err != nil {
		t.Fatalf("staging the send: %v", err)
	}
	env.deliveryID = deliveryID
	if err := env.comms.RecordSent(env.sender, deliveryID, connector.SendReceipt{ProviderMessageID: "prov-1"}); err != nil {
		t.Fatalf("recording the send: %v", err)
	}

	reporter := principal.WithWorkspaceID(context.Background(), e.WS)
	reporter = principal.WithActor(reporter, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail",
		UserID: e.Rep1, OnBehalfOf: e.Rep1,
	})
	env.reporter = principal.WithCorrelationID(reporter, ids.NewV7())
	return env
}

// report hands one delivery report to the observed store.
func (b *bounceEnv) report(t *testing.T, kind connector.BounceKind) bool {
	t.Helper()
	marked, err := b.comms.RecordBounce(b.reporter, connector.BounceReport{
		MessageID: "dead@myco.test", Recipient: b.address,
		Kind: kind, Reason: "550 5.1.1 user unknown",
	})
	if err != nil {
		t.Fatalf("recording the bounce: %v", err)
	}
	return marked
}

// liveStops answers the live suppressions standing against this address.
func (b *bounceEnv) liveStops(t *testing.T) []string {
	t.Helper()
	rows, err := OwnerConn(t).Query(context.Background(), `
		SELECT kind FROM communication_suppression
		 WHERE lower(address) = lower($1) AND revoked_at IS NULL`, b.address)
	if err != nil {
		t.Fatalf("reading the stops: %v", err)
	}
	defer rows.Close()
	var kinds []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, kind)
	}
	return kinds
}

// TestAHardBounceStopsTheAddress is the whole point. Before this the report
// marked the row and the next message went out to the same dead mailbox.
func TestAHardBounceStopsTheAddress(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")

	if !b.report(t, connector.BounceHard) {
		t.Fatal("the delivery report marked nothing")
	}

	stops := b.liveStops(t)
	if len(stops) != 1 || stops[0] != "hard_bounce" {
		t.Fatalf("the address carries stops %v, want exactly one hard_bounce: an address "+
			"that refused permanently must stop the send path, not only the record page",
			stops)
	}
}

// TestTheStopNamesNoContact is the scope decision, held where it can fail.
//
// Naming the contact looks helpful and silently widens the stop: the engine
// matches `contact_id = $1 OR ... lower(address) = ...`, so a row carrying a
// contact refuses EVERY address that contact has. Correcting a typo in one
// would then leave the other two dead, and the row would still read as an
// address stop.
//
// Found by mutation: with the stop written against a deliberately wrong
// address, the send was still refused with reason hard_bounce, because the
// contact arm was matching and the address arm never had to.
func TestTheStopNamesNoContact(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")
	b.report(t, connector.BounceHard)

	var named *ids.UUID
	if err := OwnerConn(t).QueryRow(context.Background(), `
		SELECT contact_id FROM communication_suppression
		 WHERE lower(address) = lower($1) AND revoked_at IS NULL`, b.address).Scan(&named); err != nil {
		t.Fatalf("reading the stop: %v", err)
	}
	if named != nil {
		t.Errorf("the bounce stop names contact %v: the engine matches on the contact arm "+
			"before the address arm, so this stops every address that contact has — and a "+
			"corrected typo would leave the others dead", named)
	}
}

// TestASoftBounceStopsNothing. A full mailbox or a greylisting server is not a
// dead address, and stopping on one would silence somebody who went on holiday.
func TestASoftBounceStopsNothing(t *testing.T) {
	b := setupBounce(t, "anna@full.example")

	if !b.report(t, connector.BounceSoft) {
		t.Fatal("the delivery report marked nothing")
	}
	if stops := b.liveStops(t); len(stops) != 0 {
		t.Errorf("a soft bounce left stops %v, want none: a full mailbox is not a dead "+
			"address, and the next message may well arrive", stops)
	}
}

// TestARedeliveredReportStopsTheAddressOnce. Providers redeliver reports and a
// backfill replays a captured mailbox, so this path is reached repeatedly for
// the same address. A row per report would fill the contact's history with
// identical stops saying nothing the first one did not.
func TestARedeliveredReportStopsTheAddressOnce(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")

	b.report(t, connector.BounceHard)
	// The second report finds the row already marked, so RecordBounce answers
	// false and the observer is never reached — which is itself the first line
	// of defence. The index behind it is what holds when a report arrives for a
	// DIFFERENT message to the same dead address, which is the ordinary case
	// for a mailbox that is gone.
	stopAgain := consent.HardBounceFact{Address: b.address, DeliveryID: ids.NewV7()}
	if err := database.WithWorkspaceTx(b.sender, b.e.Pool, func(tx pgx.Tx) error {
		return consent.RecordHardBounceTx(b.sender, tx, stopAgain)
	}); err != nil {
		t.Fatalf("the second stop: %v", err)
	}

	if stops := b.liveStops(t); len(stops) != 1 {
		t.Errorf("the address carries %d live stops, want one: a mailbox that is gone "+
			"refuses every message sent to it, and a row per refusal is a history nobody "+
			"can read", len(stops))
	}
}

// TestABounceOnOneAddressStopsNobodyElse. The stop is about a MAILBOX, not a
// record: a contact-scoped stop would silence every address that record has, so
// correcting a typo in one would leave the others stopped.
func TestABounceOnOneAddressStopsNobodyElse(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")
	owner := OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO contact_email (id, contact_id, email, email_type, is_primary, source, captured_by)
		VALUES ($1, $2, 'anna@live.example', 'work', false, 'manual', 'human:x')`,
		ids.NewV7(), b.contactID); err != nil {
		t.Fatalf("giving the contact a second address: %v", err)
	}

	b.report(t, connector.BounceHard)

	var live int
	if err := owner.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression
		 WHERE lower(address) = 'anna@live.example' AND revoked_at IS NULL`).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 0 {
		t.Errorf("the contact's OTHER address carries %d stops, want none: a bounce says one "+
			"mailbox is gone and says nothing about whoever owns it", live)
	}
}

// TestAnySeatLiftsABounceStop. A bounce is not a legal act by the subject and
// must not rank with one — an address corrected by a rep has to clear, and a
// subject-level stop would refuse every seat.
func TestAnySeatLiftsABounceStop(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")
	b.report(t, connector.BounceHard)

	var level string
	if err := OwnerConn(t).QueryRow(context.Background(), `
		SELECT decided_by_level FROM communication_suppression
		 WHERE lower(address) = lower($1) AND revoked_at IS NULL`, b.address).Scan(&level); err != nil {
		t.Fatalf("reading the stop: %v", err)
	}
	if level != "machine" {
		t.Errorf("the bounce stop is decided at %q, want machine: ranking it with a "+
			"subject's own decision would make a typo permanent, because CanOverrule "+
			"refuses to rank anything above the subject", level)
	}
}

// TestBothDeadRecipientsOfOneMessageAreStopped is the defect Codex found.
//
// comms_outbound carries ONE bounced_at, so the first recipient's report sets
// it and the second recipient's report matches nothing. Their address is just
// as dead. Tying the stop to whether the ledger still had a mark to set left
// the second dead mailbox live, with nothing anywhere saying so.
func TestBothDeadRecipientsOfOneMessageAreStopped(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")
	// A second dead recipient on the SAME message, staged the way the send path
	// does. Both addresses are on the row, so both reports name a real send.
	second := "boris@dead.example"
	if _, err := OwnerConn(t).Exec(context.Background(), `
		UPDATE comms_outbound SET recipients = recipients || to_jsonb($2::text)
		 WHERE message_id = $1`, "dead@myco.test", second); err != nil {
		t.Fatalf("adding the second recipient: %v", err)
	}

	if !b.report(t, connector.BounceHard) {
		t.Fatal("the first report marked nothing")
	}
	// The second report finds the row already marked and answers false, which
	// is the ledger's honest "nothing left to mark" — and the address still has
	// to be stopped.
	if _, err := b.comms.RecordBounce(b.reporter, connector.BounceReport{
		MessageID: "dead@myco.test", Recipient: second,
		Kind: connector.BounceHard, Reason: "550 5.1.1 user unknown",
	}); err != nil {
		t.Fatalf("the second recipient's report: %v", err)
	}

	var stopped int
	if err := OwnerConn(t).QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression
		 WHERE lower(address) = lower($1) AND kind = 'hard_bounce' AND revoked_at IS NULL`,
		second).Scan(&stopped); err != nil {
		t.Fatal(err)
	}
	if stopped != 1 {
		t.Errorf("the second dead recipient carries %d stops, want 1: one message to two dead "+
			"mailboxes stopped only the first, because the ledger row had one bounced_at and "+
			"the second report looked like nothing", stopped)
	}
}

// TestAForgedReportStopsNothing holds the security property the fallback above
// could have destroyed.
//
// Every field on a delivery report is attacker-writable — a Message-ID is known
// to every recipient of the mail, and anyone can post a report-shaped message
// into a captured mailbox. RecordBounce marks a row only when three facts line
// up, and the fallback that stops a second recipient re-asks the two that
// matter. An unguarded fallback would have let anyone name any address and have
// this installation stop writing to it.
func TestAForgedReportStopsNothing(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")

	// A real message id, a real reporting mailbox, and an address the message
	// never went to. That is the forgery this check exists for.
	if _, err := b.comms.RecordBounce(b.reporter, connector.BounceReport{
		MessageID: "dead@myco.test", Recipient: "victim@elsewhere.example",
		Kind: connector.BounceHard, Reason: "550 5.1.1 user unknown",
	}); err != nil {
		t.Fatalf("the forged report: %v", err)
	}

	var stopped int
	if err := OwnerConn(t).QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression
		 WHERE lower(address) = 'victim@elsewhere.example' AND revoked_at IS NULL`).Scan(&stopped); err != nil {
		t.Fatal(err)
	}
	if stopped != 0 {
		t.Errorf("a report naming an address the message never went to stopped it (%d rows): "+
			"anyone can post a report-shaped message into a captured mailbox, so a report "+
			"that fails the recipient check must record nothing", stopped)
	}
}

// TestACorrectedAddressIsWritableAgain. Without a lift a mistyped address is
// stopped permanently and the only remedy is a database edit — and Lift itself
// cannot reach these rows, because it reads by `id AND contact_id` and a bounce
// stop deliberately names no contact.
func TestACorrectedAddressIsWritableAgain(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")
	b.report(t, connector.BounceHard)

	consentStore := consent.NewStore(b.e.DB())
	if err := consentStore.LiftAddressStop(b.e.Admin(), b.address,
		"the address was mistyped; the correct one is anna@live.example"); err != nil {
		t.Fatalf("lifting the stop on a corrected address: %v", err)
	}
	if stops := b.liveStops(t); len(stops) != 0 {
		t.Errorf("the address still carries stops %v after the lift", stops)
	}
}

// TestLiftingAnAddressLeavesASubjectsOwnStopAlone. The lift clears a stop the
// MACHINERY wrote about a mailbox, and must never reach one somebody made:
// correcting an address does not undo somebody's objection.
func TestLiftingAnAddressLeavesASubjectsOwnStopAlone(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")
	b.report(t, connector.BounceHard)
	// A subject-recorded stop on the same address, of the kind a rep writes
	// when the subject asks by phone.
	if _, err := OwnerConn(t).Exec(context.Background(), `
		INSERT INTO communication_suppression (address, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'subject_request', 'they asked by phone', 'human:rep', 'subject')`,
		b.address); err != nil {
		t.Fatalf("recording the subject's own stop: %v", err)
	}

	consentStore := consent.NewStore(b.e.DB())
	if err := consentStore.LiftAddressStop(b.e.Admin(), b.address, "the address works again"); err != nil {
		t.Fatalf("lifting the bounce stop: %v", err)
	}

	stops := b.liveStops(t)
	if len(stops) != 1 || stops[0] != "subject_request" {
		t.Errorf("after lifting the bounce the address carries %v, want the subject's own "+
			"stop alone: correcting an address does not undo somebody asking to be left alone",
			stops)
	}
}

// TestErasingTheSubjectDestroysTheirBounceStop is the privacy defect Codex
// found.
//
// The stop names no contact, and both erasure and the retention sweep keyed
// their cleanup on contact_id. So an Art. 17 erase destroyed everything else
// about the subject and left their address standing in plaintext in a table
// nothing would ever clean — the one outcome an erasure must not produce.
func TestErasingTheSubjectDestroysTheirBounceStop(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")
	b.report(t, connector.BounceHard)
	if stops := b.liveStops(t); len(stops) != 1 {
		t.Fatalf("the address carries %d stops before the erase, want 1", len(stops))
	}

	if err := privacy.NewEraser(b.e.DB()).EraseContact(b.e.Admin(),
		b.contactID, "test"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	var left int
	if err := OwnerConn(t).QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression WHERE lower(address) = lower($1)`,
		b.address).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d suppression row(s) still carry the erased subject's address: the stop "+
			"names no contact, so a cleanup keyed on contact_id walks past it and the "+
			"address survives an erasure in plaintext", left)
	}
}

// TestTheExportTellsTheSubjectTheirAddressIsStopped is the Art. 15 half.
//
// The stop names no contact, so a SAR query keyed on the subject's own ids
// walks past it — and the subject asking what is held about them would not be
// told that this installation has stopped writing to an address of theirs.
func TestTheExportTellsTheSubjectTheirAddressIsStopped(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")
	b.report(t, connector.BounceHard)

	pkg, err := privacy.AssembleSAR(b.e.Admin(), b.e.DB(), ids.From[ids.ContactKind](b.contactID))
	if err != nil {
		t.Fatalf("assembling the export: %v", err)
	}
	var found bool
	for _, row := range pkg.CommunicationSuppression {
		if row["kind"] == "hard_bounce" {
			found = true
		}
	}
	if !found {
		t.Errorf("the export carries %d suppression rows and none is the bounce stop on the "+
			"subject's own address: a stop naming no contact is invisible to a query keyed "+
			"on the subject's ids, and Art. 15 owes them what is held",
			len(pkg.CommunicationSuppression))
	}
}

// TestABouncedNoticeReopensItsCase is the whole chain: a disclosure goes out, a
// delivery report says it died, and the duty is owed again.
//
// Through the real bounce path rather than by calling the writer, because the
// defect this closes is that nothing connected the two. A test reaching past
// the seam would pass against a seam that was never wired.
func TestABouncedNoticeReopensItsCase(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")
	owner := OwnerConn(t)

	// One acquisition that owes an Art. 14 disclosure, and a case for it whose
	// disclosure was carried by the message about to bounce.
	var acquisition ids.UUID
	if err := owner.QueryRow(context.Background(), `
		INSERT INTO contact_acquisition_evidence (contact_id, kind, captured_by)
		VALUES ($1, 'purchased_or_imported', 'test') RETURNING id`,
		b.contactID).Scan(&acquisition); err != nil {
		t.Fatalf("seeding the acquisition: %v", err)
	}
	var caseID ids.UUID
	if err := owner.QueryRow(context.Background(), `
		INSERT INTO privacy_notice_case
		    (contact_id, acquisition_id, rule, due_at, allowed_routes, state,
		     delivery_id, last_sent_at, attempts)
		VALUES ($1, $2, 'art14', now() + interval '30 days',
		        ARRAY['record_confirmation'], 'queued', $3, now(), 1)
		RETURNING id`, b.contactID, acquisition, b.deliveryID).Scan(&caseID); err != nil {
		t.Fatalf("seeding the queued notice case: %v", err)
	}

	if !b.report(t, connector.BounceHard) {
		t.Fatal("the delivery report marked nothing")
	}

	var state string
	if err := owner.QueryRow(context.Background(),
		`SELECT state FROM privacy_notice_case WHERE id = $1`, caseID).Scan(&state); err != nil {
		t.Fatalf("reading the case: %v", err)
	}
	if state != "delivery_failed" {
		t.Errorf("the case rests in %q after the disclosure it was waiting on bounced, want "+
			"delivery_failed: the subject was never told, so a case reading queued claims "+
			"work that did not happen — and reads as handled, so nobody looks again", state)
	}
}

// TestAControllerMailBounceIsMatched is the defect that made this whole feature
// dead in production.
//
// A disclosure is CONTROLLER mail: the installation's own message, staged with
// user_id NULL because it belongs to no seat. The bounce path matched on
// `user_id = the reporting mailbox owner`, which a controller row can never
// satisfy — so a genuine hard bounce on a real disclosure recorded nothing at
// all. The ledger never marked it, the address was never stopped, and the
// notice case sat in `queued` forever.
//
// The first version of the notice test missed this because it staged through
// the USER path, where user_id is set. A test that reaches the feature by a
// route production does not use proves nothing about production.
func TestAControllerMailBounceIsMatched(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	const address = "anna@dead.example"

	activityID := ids.New[ids.ActivityKind]()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, source, captured_by) VALUES ($1, 'email', 'test', 'system:x')`,
		activityID); err != nil {
		t.Fatalf("seeding the disclosure's activity: %v", err)
	}
	// Staged the way compose stages controller mail: no user_id, no consent
	// purpose, carrying a template.
	deliveryID := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO comms_outbound
		    (id, activity_id, provider, message_id, recipients, subject, body,
		     status, sender_kind, template_key, template_version)
		VALUES ($1, $2, 'operator_relay', $3, $4::jsonb, 'Your details', 'body',
		        'sent', 'controller', 'record_confirmation', 1)`,
		deliveryID, activityID, "disclosure@myco.test",
		`["`+address+`"]`); err != nil {
		t.Fatalf("staging the controller disclosure: %v", err)
	}

	reporter := principal.WithWorkspaceID(context.Background(), e.WS)
	reporter = principal.WithActor(reporter, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail",
		UserID: e.Rep1, OnBehalfOf: e.Rep1,
	})
	reporter = principal.WithCorrelationID(reporter, ids.NewV7())

	marked, err := observedBounceStore(e).RecordBounce(reporter, connector.BounceReport{
		MessageID: "disclosure@myco.test", Recipient: address,
		Kind: connector.BounceHard, Reason: "550 5.1.1 user unknown",
	})
	if err != nil {
		t.Fatalf("recording the bounce: %v", err)
	}
	if !marked {
		t.Fatal("a bounced CONTROLLER mail marked nothing: the installation's own " +
			"disclosures are staged with user_id NULL, so a match on the reporting " +
			"mailbox owner can never find one — and the whole notice-failure path is " +
			"unreachable in production")
	}

	var stopped int
	if err := owner.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression
		 WHERE lower(address) = lower($1) AND kind = 'hard_bounce' AND revoked_at IS NULL`,
		address).Scan(&stopped); err != nil {
		t.Fatal(err)
	}
	if stopped != 1 {
		t.Errorf("the dead address carries %d stops after a controller-mail bounce, want 1", stopped)
	}
}

// TestASecondDeliveryToADeadAddressStillReopensItsDuty.
//
// A dead mailbox refuses every message sent to it, so two outstanding
// deliveries to the same address both bounce. The FIRST writes the address
// stop; the second finds one already there and the writer returns early — which
// used to skip the notice-case reopening with it, leaving the second
// disclosure's duty in `queued` forever.
//
// The stop describes an ADDRESS, so one row covers every message to it. A duty
// describes a MESSAGE, and there is one per delivery.
func TestASecondDeliveryToADeadAddressStillReopensItsDuty(t *testing.T) {
	b := setupBounce(t, "anna@dead.example")
	owner := OwnerConn(t)

	// The address is already known to be dead.
	if err := database.WithWorkspaceTx(b.reporter, b.e.Pool, func(tx pgx.Tx) error {
		return consent.RecordHardBounceTx(b.reporter, tx, consent.HardBounceFact{
			Address: b.address, DeliveryID: b.deliveryID,
		})
	}); err != nil {
		t.Fatalf("the first stop: %v", err)
	}

	// A SECOND delivery to the same address, carrying its own disclosure.
	var acquisition ids.UUID
	if err := owner.QueryRow(context.Background(), `
		INSERT INTO contact_acquisition_evidence (contact_id, kind, captured_by)
		VALUES ($1, 'purchased_or_imported', 'test') RETURNING id`,
		b.contactID).Scan(&acquisition); err != nil {
		t.Fatalf("seeding the acquisition: %v", err)
	}
	second := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO comms_outbound
		    (id, activity_id, provider, message_id, recipients, subject, body,
		     status, sender_kind, template_key, template_version)
		VALUES ($1, (SELECT activity_id FROM comms_outbound WHERE id = $2),
		        'operator_relay', 'second@myco.test', $3::jsonb, 'Your details', 'body',
		        'sent', 'controller', 'record_confirmation', 1)`,
		second, b.deliveryID, `["`+b.address+`"]`); err != nil {
		t.Fatalf("staging the second disclosure: %v", err)
	}
	var caseID ids.UUID
	if err := owner.QueryRow(context.Background(), `
		INSERT INTO privacy_notice_case
		    (contact_id, acquisition_id, rule, due_at, allowed_routes, state,
		     delivery_id, last_sent_at, attempts)
		VALUES ($1, $2, 'art14', now() + interval '30 days',
		        ARRAY['record_confirmation'], 'queued', $3, now(), 1)
		RETURNING id`, b.contactID, acquisition, second).Scan(&caseID); err != nil {
		t.Fatalf("seeding the second queued case: %v", err)
	}

	// The second delivery bounces too. The stop already exists.
	if err := database.WithWorkspaceTx(b.reporter, b.e.Pool, func(tx pgx.Tx) error {
		return consent.RecordHardBounceTx(b.reporter, tx, consent.HardBounceFact{
			Address: b.address, DeliveryID: second,
		})
	}); err != nil {
		t.Fatalf("the second bounce: %v", err)
	}

	var state string
	if err := owner.QueryRow(context.Background(),
		`SELECT state FROM privacy_notice_case WHERE id = $1`, caseID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "delivery_failed" {
		t.Errorf("the second disclosure's case rests in %q, want delivery_failed: the "+
			"address stop covers every message to it while the duty is per message, so "+
			"returning early "+
			"on an existing stop leaves this duty claiming a message is on its way", state)
	}
}
