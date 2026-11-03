// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The lane the installation's own mail rides, asserted on the rows it actually
// writes.
//
// This is the seam where three modules meet, so it is tested here rather than in
// any of them: activities files the timeline row, comms stages the delivery,
// consent takes the decision, and the whole point is that they commit together.
//
// The obligation this suite exists for is the one the old direct-SMTP call could
// not meet. That call wrote nothing: no delivery row, no authorization decision,
// no timeline entry — so the one message the installation composes entirely by
// itself appeared in no subject-access export and no erasure reached it.

import (
	"context"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"github.com/margince/margince/backend/internal/shared/testsmtp"
)

// sealingVault stands where compose's keyvault-backed one stands. It keeps the
// plaintext so a test can assert what was sealed rather than only that something
// was.
type sealingVault struct{ sealed string }

func (v *sealingVault) Put(_ context.Context, secret string) (string, error) {
	v.sealed = secret
	return "sealed-ref", nil
}

// Delete destroys the sealed link, as an erasure asks the vault to.
func (v *sealingVault) Delete(context.Context, string) error {
	v.sealed = ""
	return nil
}

// laneFromAddress is the relay sender the lane under test is configured with.
const laneFromAddress = "notices@crm.example.test"

// confirmLaneEnv is one subject with a live address and the real lane wired.
type confirmLaneEnv struct {
	e       *integration.Env
	ctx     context.Context
	contact ids.UUID
	store   *consent.Store
	vault   *sealingVault
}

// setupConfirmLane wires the REAL machinery, not a double: the delivery staging
// enqueues its dispatch in the same transaction, so a test that replaced it
// would prove nothing about the property this suite is about.
func setupConfirmLane(t *testing.T) *confirmLaneEnv {
	t.Helper()
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	inserter, err := jobs.NewInserter(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("jobs.NewInserter: %v", err)
	}
	contact := e.SeedContact(t, "Confirm Subject", &e.Rep1)
	if _, err := e.Pool.Exec(context.Background(), `
		INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, 'subject@example.test', true, 'manual', 'human:test')`, contact); err != nil {
		t.Fatal(err)
	}
	vault := &sealingVault{}
	return &confirmLaneEnv{
		e:       e,
		ctx:     e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms),
		contact: contact,
		vault:   vault,
		store: consent.NewStore(InstallationDB(e.Pool)).
			WithConfirmationLane(NewControllerMailQueue(e.Pool, inserter, laneFromAddress), vault, "https://crm.example.test"),
	}
}

// TestAConfirmLinkStagesADeliveryADecisionAndATimelineRow is the central claim:
// the installation's own mail leaves the same evidence every other message does.
func TestAConfirmLinkStagesADeliveryADecisionAndATimelineRow(t *testing.T) {
	l := setupConfirmLane(t)

	issued, err := l.store.IssueConfirmToken(l.ctx, ids.From[ids.ContactKind](l.contact), "")
	if err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}
	if !issued.Staged {
		t.Fatal("the link was minted but no message was staged")
	}

	if n := l.e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE sender_kind = 'controller'`); n != 1 {
		t.Errorf("%d controller deliveries, want 1: without a delivery row the message appears in "+
			"no subject-access export and erasure cannot reach it", n)
	}
	// The decision is what makes the lane NOT a side door. A controller message
	// that transmitted without one would be the installation exempting itself
	// from the engine it applies to every rep.
	if n := l.e.WsCount(t, `
		SELECT count(*) FROM communication_decision d
		 JOIN comms_outbound o ON o.id = d.delivery_id AND o.sender_kind = 'controller'`); n != 1 {
		t.Errorf("%d staging decisions for the controller delivery, want 1", n)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM activity WHERE origin = 'system_notice'`); n != 1 {
		t.Errorf("%d notice activities, want 1: privacy's erasure and the subject-access export "+
			"both reach comms_outbound THROUGH the activity", n)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM river_job`); n != 1 {
		t.Errorf("%d dispatch jobs, want 1: a staged message nothing will carry is a link the "+
			"subject never receives", n)
	}
}

// TestTheEngineAllowsAConfirmationOnItsOwnEvidence pins WHAT the decision says.
//
// A row saying "denied" would satisfy the count above while the message parked
// forever, so the count alone is not the property. The basis is asserted for the
// same reason it is legal_obligation in the validator: a confirmation that
// rested on consent could never be sent to somebody who has not consented yet.
func TestTheEngineAllowsAConfirmationOnItsOwnEvidence(t *testing.T) {
	l := setupConfirmLane(t)

	if _, err := l.store.IssueConfirmToken(l.ctx, ids.From[ids.ContactKind](l.contact), ""); err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}

	var verdict, category, basis string
	if err := l.e.Pool.QueryRow(context.Background(), `
		SELECT d.verdict, d.resolved_category, coalesce(d.basis, '')
		  FROM communication_decision d
		  JOIN comms_outbound o ON o.id = d.delivery_id AND o.sender_kind = 'controller'
		 ORDER BY d.decided_at DESC LIMIT 1`).Scan(&verdict, &category, &basis); err != nil {
		t.Fatalf("reading the staging decision: %v", err)
	}
	if verdict != "allow" {
		t.Errorf("the engine answered %q for a confirmation with a live link — the installation "+
			"cannot ask somebody to check what is held about them", verdict)
	}
	if category != "record_confirmation" {
		t.Errorf("resolved category %q, want record_confirmation", category)
	}
	if basis != "legal_obligation" {
		t.Errorf("basis %q, want legal_obligation", basis)
	}
}

// TestTheStagedMessageCarriesNoLiveLink holds the plaintext out of every row a
// human or an export can read.
func TestTheStagedMessageCarriesNoLiveLink(t *testing.T) {
	l := setupConfirmLane(t)

	if _, err := l.store.IssueConfirmToken(l.ctx, ids.From[ids.ContactKind](l.contact), ""); err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}
	if l.vault.sealed == "" {
		t.Fatal("nothing was sealed, so there is no link for the dispatcher to substitute")
	}

	// The token itself, as it appears in the sealed link. Searching for it
	// rather than for a URL shape is what makes this catch a leak into a
	// column that stores the token alone.
	_, token, found := lastCut(l.vault.sealed, "#/confirm/")
	if !found {
		t.Fatalf("the sealed value is not a confirm link: %q", l.vault.sealed)
	}

	for _, probe := range []struct {
		what  string
		query string
	}{
		{"the delivery row", `SELECT count(*) FROM comms_outbound WHERE body LIKE '%' || $1 || '%'`},
		{"the timeline", `SELECT count(*) FROM activity WHERE coalesce(body, '') LIKE '%' || $1 || '%'`},
		// Every jsonb an audit row carries, not one of them: the token could
		// land in any, and a probe naming a single column would pass while the
		// leak sat in the next one along.
		{"an audit payload", `
			SELECT count(*) FROM audit_log
			 WHERE coalesce(before::text, '') || coalesce(after::text, '') || coalesce(evidence::text, '')
			       LIKE '%' || $1 || '%'`},
		{"an outbox event", `SELECT count(*) FROM event_outbox WHERE envelope::text LIKE '%' || $1 || '%'`},
	} {
		var n int
		if err := l.e.Pool.QueryRow(context.Background(), probe.query, token).Scan(&n); err != nil {
			t.Fatalf("scanning %s: %v", probe.what, err)
		}
		if n != 0 {
			t.Errorf("the live confirm token reached %s (%d row(s)). Anyone who can read that "+
				"column can open the subject's record without ever holding their mailbox", probe.what, n)
		}
	}
}

// lastCut splits on the final occurrence of sep, so a link whose ORIGIN happens
// to contain the separator still yields the token.
func lastCut(s, sep string) (before, after string, found bool) {
	at := lastIndex(s, sep)
	if at < 0 {
		return s, "", false
	}
	return s[:at], s[at+len(sep):], true
}

func lastIndex(s, sub string) int {
	for i := len(s) - len(sub); i >= 0; i-- {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// ackJurisdiction is a code no shipped pack holds, so registering its rules
// here cannot collide with a compiled-in pack.
const ackJurisdiction jurisdiction.Code = "qz"

// TestAOnePurposeUnsubscribeIsAcknowledgedThroughTheRealEngine presses the
// unsubscribe link a marketing send carries, under a pack that owes an
// acknowledgement. The press writes a withdrawal and no suppression, and the
// engine must still admit the acknowledgement it owes.
func TestAOnePurposeUnsubscribeIsAcknowledgedThroughTheRealEngine(t *testing.T) {
	l := setupConfirmLane(t)
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: ackJurisdiction, Version: 1, OptOutAcknowledgement: true, OptOutAcknowledgementWithin: 24 * time.Hour,
	})
	store := l.store.WithInstallationCountry(consent.InstallationCountryFunc(
		func(context.Context, pgx.Tx) (jurisdiction.Code, error) { return ackJurisdiction, nil }))
	if _, err := l.e.Pool.Exec(context.Background(), `
		INSERT INTO consent_purpose (key, label, class)
		VALUES ('newsletter', 'Newsletter', 'marketing')
		ON CONFLICT (key) DO NOTHING`); err != nil {
		t.Fatalf("seeding the newsletter: %v", err)
	}

	withdrawn, err := store.PublicWithdrawAll(l.ctx, ids.From[ids.ContactKind](l.contact), []string{"newsletter"})
	if err != nil {
		t.Fatalf("the press failed: %v", err)
	}
	if len(withdrawn) != 1 {
		t.Fatalf("the press withdrew %v, want the newsletter", withdrawn)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM communication_suppression`); n != 0 {
		t.Fatalf("%d suppressions, want none: this test is about a press that writes only a withdrawal", n)
	}
	if n := l.e.WsCount(t, `
		SELECT count(*) FROM comms_outbound
		 WHERE sender_kind = 'controller' AND template_key = 'optout_acknowledgement'`); n != 1 {
		t.Errorf("%d acknowledgement deliveries, want 1: the engine refused what the decree owes", n)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM river_job`); n != 1 {
		t.Errorf("%d dispatch jobs, want 1", n)
	}
	var dueAt *time.Time
	var expiry *time.Time
	if err := l.e.Pool.QueryRow(l.ctx, `SELECT delivery_due_at,payload_expires_at FROM comms_outbound WHERE template_key='optout_acknowledgement'`).Scan(&dueAt, &expiry); err != nil {
		t.Fatal(err)
	}
	if dueAt == nil || expiry != nil {
		t.Fatal("receipt deadline and link expiry were conflated")
	}
	now := clocktest.Now(t)
	count, err := comms.NewStore(InstallationDB(l.e.Pool), func() time.Time { return now }, nil).OverdueControllerReceipts(sendWorkerScope(l.ctx), dueAt.Add(time.Second))
	if err != nil || count != 1 {
		t.Fatalf("overdue receipt was invisible: %d %v", count, err)
	}
}

func TestControllerAlternativesAreStagedWithoutPersonalMaterial(t *testing.T) {
	lane := setupConfirmLane(t)
	if _, err := lane.store.IssueConfirmToken(lane.ctx, ids.From[ids.ContactKind](lane.contact), ""); err != nil {
		t.Fatal(err)
	}
	var text, markup string
	if err := lane.e.Pool.QueryRow(lane.ctx, "SELECT body,html_body FROM comms_outbound WHERE sender_kind='controller'").Scan(&text, &markup); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{text, markup} {
		if !strings.Contains(body, "{{confirmation-link}}") || strings.Contains(body, lane.vault.sealed) {
			t.Fatal("staging lost the placeholder or exposed personal material")
		}
	}
}

func (v *sealingVault) Get(context.Context, string) (string, error) { return v.sealed, nil }

func TestControllerStagingReachesReceivedAlternativesAndRetiresItsLink(t *testing.T) {
	lane := setupConfirmLane(t)
	if _, err := lane.store.IssueConfirmToken(lane.ctx, ids.From[ids.ContactKind](lane.contact), ""); err != nil {
		t.Fatal(err)
	}
	link := lane.vault.sealed
	var deliveryID ids.UUID
	var messageID string
	if err := lane.e.Pool.QueryRow(lane.ctx, `SELECT id,message_id FROM comms_outbound WHERE sender_kind='controller'`).Scan(&deliveryID, &messageID); err != nil {
		t.Fatal(err)
	}
	relay := testsmtp.Start(t, false)
	host, port := relay.Address(t)
	dispatcher := newSendDispatcher(lane.e.Pool, nil, sendPace{limit: 10, window: time.Minute, maxAge: time.Hour}, nil, nil, nil).
		WithControllerRelay(ControllerRelayFor(mailer.SMTP{Host: host, Port: port, FromAddress: "privacy@example.test"}), lane.vault)
	outcome, _, err := dispatcher.DispatchWithWait(sendWorkerScope(lane.ctx), deliveryID)
	if err != nil || outcome != comms.OutcomeSent {
		t.Fatalf("controller dispatch: %s %v", outcome, err)
	}
	received, err := mail.ReadMessage(strings.NewReader(relay.Received()))
	if err != nil {
		t.Fatal(err)
	}
	if received.Header.Get("Message-ID") != "<"+messageID+">" {
		t.Fatal("received identity changed")
	}
	kind, params, err := mime.ParseMediaType(received.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/alternative" {
		t.Fatalf("received content: %s %v", kind, err)
	}
	parts := multipart.NewReader(received.Body, params["boundary"])
	count := 0
	for {
		part, err := parts.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), link) || strings.Contains(string(body), "{{confirmation-link}}") {
			t.Fatal("received alternative lost its action")
		}
		count++
	}
	if count != 2 || lane.vault.sealed != "" {
		t.Fatalf("received %d alternatives; sealed link remains %v", count, lane.vault.sealed != "")
	}
}

func TestControllerHTMLCannotChangeAfterStagingAuthorization(t *testing.T) {
	lane := setupConfirmLane(t)
	if _, err := lane.store.IssueConfirmToken(lane.ctx, ids.From[ids.ContactKind](lane.contact), ""); err != nil {
		t.Fatal(err)
	}
	var id ids.UUID
	if err := lane.e.Pool.QueryRow(lane.ctx, `UPDATE comms_outbound SET html_body=html_body||'<p>Changed instruction</p>' WHERE sender_kind='controller' RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	relay := &countingMailer{}
	dispatcher := newSendDispatcher(lane.e.Pool, nil, sendPace{limit: 10, window: time.Minute, maxAge: time.Hour}, nil, nil, nil).
		WithControllerRelay(ControllerRelayFor(relay), lane.vault)
	outcome, _, err := dispatcher.DispatchWithWait(sendWorkerScope(lane.ctx), id)
	if err != nil || outcome != comms.OutcomeParked || relay.count() != 0 {
		t.Fatalf("changed markup reached transmission: %s %v %d", outcome, err, relay.count())
	}
}

func TestAReceiptPreparationSQLFailureCannotRollBackTheWithdrawal(t *testing.T) {
	lane := setupConfirmLane(t)
	const country jurisdiction.Code = "qh"
	messagingrules.Register(messagingrules.Rules{Jurisdiction: country, Version: 1, OptOutAcknowledgement: true})
	store := lane.store.WithInstallationCountry(consent.InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) { return country, nil })).
		WithMailLanguage(consent.MailLanguageFunc(func(ctx context.Context, tx pgx.Tx) (string, error) {
			_, err := tx.Exec(ctx, `SELECT 1/0`)
			return "", err
		}))
	if _, err := lane.e.Pool.Exec(lane.ctx, `INSERT INTO consent_purpose(key,label,class) VALUES('receipt_test','Receipt test','marketing')`); err != nil {
		t.Fatal(err)
	}
	withdrawn, err := store.PublicWithdrawAll(lane.ctx, ids.From[ids.ContactKind](lane.contact), []string{"receipt_test"})
	if err != nil || len(withdrawn) != 1 {
		t.Fatalf("receipt error lost the refusal: %v %v", withdrawn, err)
	}
	if count := lane.e.WsCount(t, `SELECT count(*) FROM comms_outbound`); count != 0 {
		t.Fatalf("failed preparation staged %d receipts", count)
	}
}
