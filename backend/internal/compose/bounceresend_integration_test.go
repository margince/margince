// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// "Send again" on a bounce, driven through the real send path: the real
// delivery machinery, the real consent gate and the comms rows production
// writes. Nothing is hand-inserted that a writer produces.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	bouncedAddress   = "old@buyer.test"
	correctedAddress = "new@buyer.test"
)

// bounceFixture is one rep's send that hard-bounced, as production writes it.
type bounceFixture struct {
	e        *integration.Env
	svc      bounceResendService
	contact  ids.UUID
	purpose  consent.Purpose
	consents *consent.Store
	rep      context.Context
	bounceID ids.UUID
}

func newBounceFixture(t *testing.T) bounceFixture {
	t.Helper()
	e := integration.Setup(t)
	send := SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: realDeliveryStager(t, e)}
	f := bounceFixture{
		e: e, svc: newBounceResendService(e.Pool, send.Delivery, send),
		consents: consent.NewStore(InstallationDB(e.Pool)),
		rep:      e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms),
	}
	f.contact = e.SeedContact(t, "Buyer Contact", &e.Rep1)
	addContactEmail(t, e, f.contact, bouncedAddress)
	purpose, err := f.consents.CreatePurpose(e.Admin(), "newsletter", "Newsletter", false, "")
	if err != nil {
		t.Fatalf("create the purpose: %v", err)
	}
	f.purpose = purpose
	f.setConsent(t, "granted")

	anchor := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
			VALUES ($1, 'email', 'inbound', 'Retrofit quote', now(), 'manual', 'human:x')`, anchor); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
			VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), anchor, f.contact)
		return err
	}); err != nil {
		t.Fatalf("seed the thread: %v", err)
	}
	if _, err := f.svc.sends.SendEmail(f.rep, activities.FromActivity(ids.From[ids.ActivityKind](anchor)), activities.SendEmailInput{
		Recipients: []string{bouncedAddress}, Subject: "Retrofit quote",
		Body: "Here is the quote.", HTMLBody: "<p>Here is the quote.</p>",
		ConsentPurpose: "newsletter",
	}, f.svc.gate, f.svc.delivery); err != nil {
		t.Fatalf("the original send: %v", err)
	}
	f.bounceID = ids.MustParse(e.WsScalar(t, `SELECT id::text FROM comms_outbound`))
	f.bounce(t, f.bounceID, bouncedAddress)
	return f
}

// bounce records a hard bounce the way the mailbox connector does.
func (f bounceFixture) bounce(t *testing.T, id ids.UUID, address string) {
	t.Helper()
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), f.e.WS), principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: f.e.Rep1, OnBehalfOf: f.e.Rep1,
	})
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	marked, err := f.svc.bounces.RecordBounce(ctx, connector.BounceReport{
		MessageID: f.e.WsScalar(t, `SELECT message_id FROM comms_outbound WHERE id = $1`, id),
		Recipient: address, Kind: connector.BounceHard, Reason: "550 5.1.1 user unknown",
	})
	if err != nil || !marked {
		t.Fatalf("recording the bounce: marked=%v err=%v", marked, err)
	}
}

func (f bounceFixture) setConsent(t *testing.T, state string) {
	t.Helper()
	if _, err := f.consents.Record(f.e.Admin(), consent.RecordInput{
		ContactID: ids.From[ids.ContactKind](f.contact), PurposeID: f.purpose.ID, NewState: state, PolicyText: &grantedWording,
	}); err != nil {
		t.Fatalf("record %s: %v", state, err)
	}
}

func (f bounceFixture) canSendAgain(t *testing.T, id ids.UUID) bool {
	t.Helper()
	bounced, err := f.svc.bounces.HardBouncesFor(f.rep, time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("reading the bounce lane: %v", err)
	}
	for _, b := range bounced {
		if b.ID == id {
			return b.CanSendAgain
		}
	}
	t.Fatalf("bounce %s is not on the lane: %+v", id, bounced)
	return false
}

func (f bounceFixture) liveAddresses(t *testing.T) []string {
	t.Helper()
	rows, err := f.e.Pool.Query(f.e.Admin(), `SELECT email FROM contact_email WHERE contact_id = $1 AND archived_at IS NULL ORDER BY email`, f.contact)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func TestSendAgainSendsTheSameMessageToTheCorrectedAddressUnderTheOriginalPurpose(t *testing.T) {
	f := newBounceFixture(t)
	if !f.canSendAgain(t, f.bounceID) {
		t.Fatal("a hard bounce of the rep's own send is not offered Send again")
	}

	sent, err := f.svc.SendAgain(f.rep, f.bounceID, " "+strings.ToUpper(correctedAddress)+" ")
	if err != nil {
		t.Fatalf("SendAgain: %v", err)
	}

	assertDelivered(t, f.e, 2, "the first send and its repeat")
	var purpose, subject, body, html, resentFrom, recipients string
	if err := f.e.Pool.QueryRow(f.e.Admin(), `
		SELECT consent_purpose, subject, body, html_body, resent_from::text, recipients::text
		  FROM comms_outbound WHERE activity_id = $1`, ids.UUID(sent.Id)).Scan(&purpose, &subject, &body, &html, &resentFrom, &recipients); err != nil {
		t.Fatalf("reading the repeat: %v", err)
	}
	if purpose != "newsletter" {
		t.Errorf("consent purpose = %q, want the original send's newsletter", purpose)
	}
	if subject != "Retrofit quote" || !strings.HasPrefix(body, "Here is the quote.") || !strings.HasPrefix(html, "<p>Here is the quote.</p>") {
		t.Errorf("the repeat is not the same message: subject %q body %q html %q", subject, body, html)
	}
	if resentFrom != f.bounceID.String() {
		t.Errorf("resent_from = %s, want the bounced delivery %s", resentFrom, f.bounceID)
	}
	if !strings.Contains(recipients, correctedAddress) || strings.Contains(recipients, bouncedAddress) {
		t.Errorf("recipients = %s, want only the corrected address", recipients)
	}
	// The sign-off and the unsubscribe footer are added by the send. Starting
	// from the stored wire text would put each of them on the mail twice.
	for _, once := range []string{"Best regards", "Unsubscribe:"} {
		if n := strings.Count(body, once); n != 1 {
			t.Errorf("%q appears %d times in the repeat's text, want once:\n%s", once, n, body)
		}
	}
	if got := f.liveAddresses(t); len(got) != 1 || got[0] != correctedAddress {
		t.Errorf("the contact's live addresses = %v, want only %s", got, correctedAddress)
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id = $2`, ids.UUID(sent.Id), f.contact); got != 1 {
		t.Errorf("the repeat is filed under the contact %d times, want once", got)
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1`, ids.UUID(sent.Id)); got == 0 {
		t.Error("the repeat left no audit row")
	}
}

func TestABouncedMessageIsSentAgainOnceAndTheRepeatIsNotOfferedAgain(t *testing.T) {
	f := newBounceFixture(t)
	if _, err := f.svc.SendAgain(f.rep, f.bounceID, correctedAddress); err != nil {
		t.Fatalf("SendAgain: %v", err)
	}
	if f.canSendAgain(t, f.bounceID) {
		t.Error("the bounce is still offered Send again after it was sent again")
	}
	if _, err := f.svc.SendAgain(f.rep, f.bounceID, "third@buyer.test"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a second Send again on one bounce = %v, want not found", err)
	}

	repeat := ids.MustParse(f.e.WsScalar(t, `SELECT id::text FROM comms_outbound WHERE resent_from IS NOT NULL`))
	f.bounce(t, repeat, correctedAddress)
	if f.canSendAgain(t, repeat) {
		t.Error("a repeat that bounced is offered a third send")
	}
	if _, err := f.svc.SendAgain(f.rep, repeat, "fourth@buyer.test"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("Send again on a repeat's bounce = %v, want not found", err)
	}
	assertDelivered(t, f.e, 2, "the three presses")
}

func TestSendAgainIsRefusedWhenConsentWasWithdrawnSinceTheBounce(t *testing.T) {
	f := newBounceFixture(t)
	f.setConsent(t, "withdrawn")

	_, err := f.svc.SendAgain(f.rep, f.bounceID, correctedAddress)
	if !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Fatalf("SendAgain after a withdrawal = %v, want the consent refusal", err)
	}
	assertDelivered(t, f.e, 1, "a refused repeat")
	if !f.canSendAgain(t, f.bounceID) {
		t.Error("a refused repeat used up the bounce's one Send again")
	}
}

func TestSendAgainIsRefusedWhenTheContactWasSuppressedSinceTheBounce(t *testing.T) {
	f := newBounceFixture(t)
	if err := f.consents.Suppress(f.e.Admin(), consent.SuppressInput{
		ContactID: ids.From[ids.ContactKind](f.contact), Kind: "subject_request", Reason: "asked to stop",
	}); err != nil {
		t.Fatalf("suppress: %v", err)
	}

	if _, err := f.svc.SendAgain(f.rep, f.bounceID, correctedAddress); err == nil {
		t.Fatal("SendAgain reached a suppressed contact")
	}
	assertDelivered(t, f.e, 1, "a repeat to a suppressed contact")
}

func TestOnlySomeoneWhoMaySendAndChangeTheContactMaySendAgain(t *testing.T) {
	cases := []struct {
		name  string
		ctx   func(f bounceFixture) context.Context
		is    error
		check string
	}{
		{"a seat that may not send", func(f bounceFixture) context.Context {
			perms := integration.SchedulerPerms
			perms.Objects = map[string]principal.ObjectGrant{
				"contact": {Read: true, Update: true}, "activity": {Read: true},
			}
			return f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, perms)
		}, apperrors.ErrPermissionDenied, "send"},
		{"a seat that may read the contact and not change it", func(f bounceFixture) context.Context {
			perms := integration.SchedulerPerms
			perms.Objects = map[string]principal.ObjectGrant{
				"contact": {Read: true}, "activity": {Create: true, Read: true},
			}
			return f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, perms)
		}, apperrors.ErrPermissionDenied, "contact"},
		{"an AI agent acting for the rep", func(f bounceFixture) context.Context {
			return f.e.AgentFor(t, f.e.Rep1, []ids.UUID{f.e.Team1}, integration.SchedulerPerms)
		}, apperrors.ErrPermissionDenied, "agent"},
		{"another rep", func(f bounceFixture) context.Context {
			return f.e.As(f.e.Rep2, []ids.UUID{f.e.Team1}, integration.SchedulerPerms)
		}, apperrors.ErrNotFound, "other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBounceFixture(t)
			if _, err := f.svc.SendAgain(tc.ctx(f), f.bounceID, correctedAddress); !errors.Is(err, tc.is) {
				t.Fatalf("SendAgain = %v, want %v", err, tc.is)
			}
			assertDelivered(t, f.e, 1, tc.name)
			if got := f.liveAddresses(t); len(got) != 1 || got[0] != bouncedAddress {
				t.Errorf("the refusal left the contact with %v, want the address untouched", got)
			}
		})
	}
}

func TestSendAgainRefusesAnAddressThatCannotReplaceTheBouncedOne(t *testing.T) {
	f := newBounceFixture(t)
	for _, typed := range []string{bouncedAddress, strings.ToUpper(bouncedAddress), "not an address", "Name <x@buyer.test>", ""} {
		_, err := f.svc.SendAgain(f.rep, f.bounceID, typed)
		var fault apperrors.FieldFault
		if !errors.As(err, &fault) {
			t.Errorf("SendAgain(%q) = %v, want a refusal naming the address field", typed, err)
		}
	}
	assertDelivered(t, f.e, 1, "refused addresses")
}

// linkSecondContact files the bounced send under another contact the rep can
// see, whose id sorts before the bounced contact's, so a read that took "the
// first linked contact" would pick it.
func (f bounceFixture) linkSecondContact(t *testing.T, address string) ids.UUID {
	t.Helper()
	other := ids.MustParse("00000000-0000-7000-8000-000000000001")
	activity := f.e.WsScalar(t, `SELECT activity_id::text FROM comms_outbound WHERE id = $1`, f.bounceID)
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		for _, statement := range []string{
			`INSERT INTO contact (id, full_name, source, captured_by, owner_id) VALUES ($1, 'Other Contact', 'test', 'human:x', '` + f.e.Rep1.String() + `')`,
			`INSERT INTO contact_email (contact_id, email, email_type, is_primary, source, captured_by) VALUES ($1, '` + address + `', 'work', true, 'manual', 'human:x')`,
		} {
			if _, err := tx.Exec(context.Background(), statement, other); err != nil {
				return err
			}
		}
		_, err := tx.Exec(context.Background(),
			`INSERT INTO activity_link (id, activity_id, entity_type, contact_id) VALUES ($1, $2::uuid, 'contact', $3)`, ids.NewV7(), activity, other)
		return err
	}); err != nil {
		t.Fatalf("link the second contact: %v", err)
	}
	return other
}

func (f bounceFixture) addressesOf(t *testing.T, contact ids.UUID) []string {
	t.Helper()
	rows, err := f.e.Pool.Query(f.e.Admin(), `SELECT email FROM contact_email WHERE contact_id = $1 AND archived_at IS NULL ORDER BY email`, contact)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func TestSendAgainChangesOnlyTheContactWhoseAddressBounced(t *testing.T) {
	f := newBounceFixture(t)
	other := f.linkSecondContact(t, "other@buyer.test")

	if _, err := f.svc.SendAgain(f.rep, f.bounceID, correctedAddress); err != nil {
		t.Fatalf("SendAgain: %v", err)
	}
	if got := f.addressesOf(t, other); len(got) != 1 || got[0] != "other@buyer.test" {
		t.Errorf("the other linked contact now holds %v, want its own address untouched", got)
	}
	if got := f.addressesOf(t, f.contact); len(got) != 1 || got[0] != correctedAddress {
		t.Errorf("the bounced contact holds %v, want only %s", got, correctedAddress)
	}
}

func TestSendAgainChecksConsentForTheContactWhoseAddressBounced(t *testing.T) {
	f := newBounceFixture(t)
	other := f.linkSecondContact(t, "other@buyer.test")
	// The linked contact that did not bounce has consent. A send that put the
	// corrected address on it would pass the gate.
	if _, err := f.consents.Record(f.e.Admin(), consent.RecordInput{
		ContactID: ids.From[ids.ContactKind](other), PurposeID: f.purpose.ID, NewState: "granted", PolicyText: &grantedWording,
	}); err != nil {
		t.Fatalf("grant the other contact: %v", err)
	}
	f.setConsent(t, "withdrawn")

	if _, err := f.svc.SendAgain(f.rep, f.bounceID, correctedAddress); !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Fatalf("SendAgain = %v, want the bounced contact's withdrawn consent to refuse it", err)
	}
	assertDelivered(t, f.e, 1, "a refused repeat")
}

func TestSendAgainIsRefusedWhenNoVisibleLinkedContactHoldsTheBouncedAddress(t *testing.T) {
	f := newBounceFixture(t)
	elsewhere := f.e.SeedContact(t, "Unlinked Contact", &f.e.Rep1)
	f.e.WsExec(t, `UPDATE contact_email SET contact_id = $1 WHERE email = $2`, elsewhere, bouncedAddress)

	if f.canSendAgain(t, f.bounceID) {
		t.Error("Send again is offered although no linked contact holds the bounced address")
	}
	if _, err := f.svc.SendAgain(f.rep, f.bounceID, correctedAddress); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("SendAgain = %v, want not found", err)
	}
	if got := f.addressesOf(t, f.contact); len(got) != 0 {
		t.Errorf("the linked contact gained %v, want nothing appended", got)
	}
	assertDelivered(t, f.e, 1, "a refused repeat")
}
