// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The Vietnamese opt-out receipt through the real controller lane
// (Decree 91/2020/ND-CP Art. 20 and Art. 11). One refusal queues one receipt,
// which prints both times in Vietnam time. The records stay exportable a year
// on, and a missed receipt shows in System health.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/noticekeys"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// receiptCountry is a code no shipped pack holds, owing the receipt on the
// Vietnamese pack's terms.
const receiptCountry jurisdiction.Code = "qw"

var registerReceiptCountry sync.Once

// vietnamTime is the offset the Vietnamese pack prints a receipt's times at.
var vietnamTime = time.FixedZone("UTC+07:00", 7*60*60)

func owingReceipts() {
	registerReceiptCountry.Do(func() {
		messagingrules.Register(messagingrules.Rules{
			Jurisdiction: receiptCountry, Version: 1, OptOutAcknowledgement: true,
			OptOutAcknowledgementWithin: time.Hour, OptOutAcknowledgementUTCOffset: 7 * time.Hour,
			OptOutRecordsKeep: 366 * 24 * time.Hour,
		})
	})
}

// receiptLaneEnv is a contact under the receipt rules, with the consent store
// on the real controller mail lane.
type receiptLaneEnv struct {
	e       *Env
	ctx     context.Context
	contact ids.UUID
	store   *consent.Store
	queue   consent.ConfirmationSender
}

// unusedLinkVault completes the lane. A receipt carries no link to seal.
type unusedLinkVault struct{}

func (unusedLinkVault) Put(context.Context, string) (string, error) { return "", errNoLinkToSeal }

var errNoLinkToSeal = errors.New("a receipt carries no link to seal")

func setupReceiptLane(t *testing.T) *receiptLaneEnv {
	t.Helper()
	owingReceipts()
	e := Setup(t)
	ApplyRiverSchema(t)
	inserter, err := jobs.NewInserter(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, "Receipt Subject", &e.Rep1)
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, 'receipt-subject@example.test', true, 'manual', 'human:test')`, contact)
	e.WsExec(t, `UPDATE contact SET address_country = upper($2) WHERE id = $1`, contact, string(receiptCountry))
	queue := compose.NewControllerMailQueue(e.Pool, inserter, "privacy@example.test")
	return &receiptLaneEnv{
		e: e, ctx: e.As(e.Rep1, []ids.UUID{e.Team1}, SchedulerPerms), contact: contact, queue: queue,
		store: consent.NewStore(compose.InstallationDB(e.Pool)).
			WithConfirmationLane(queue, unusedLinkVault{}, "https://crm.example.test"),
	}
}

// TestARelayedRefusalIsReceiptedOnceWithBothTimes records a call that arrived
// three hours before a colleague typed it. The one receipt prints the call's
// time and the stop's time in Vietnam time, and names no Margince. The ledger
// refuses a second receipt for the same refusal.
func TestARelayedRefusalIsReceiptedOnceWithBothTimes(t *testing.T) {
	t.Parallel()
	l := setupReceiptLane(t)
	called := clocktest.Now(t).Add(-3 * time.Hour).Truncate(time.Minute)

	if err := l.store.Suppress(l.ctx, consent.SuppressInput{
		ContactID: ids.From[ids.ContactKind](l.contact), Kind: commsauthz.ReasonObjection,
		Reason: "Asked on the phone to stop the newsletter.", ReceivedAt: &called,
	}); err != nil {
		t.Fatalf("recording the refusal: %v", err)
	}
	var received, stopped time.Time
	if err := l.e.Pool.QueryRow(l.ctx, `
		SELECT received_at, recorded_at FROM communication_suppression WHERE contact_id = $1`,
		l.contact).Scan(&received, &stopped); err != nil {
		t.Fatalf("reading the stop: %v", err)
	}
	if !received.Equal(called) || !stopped.After(received) {
		t.Fatalf("the stop holds received %v and stopped %v, want the call's time %v before the stop", received, stopped, called)
	}

	var send consent.ConfirmationSend
	var markup string
	if err := l.e.Pool.QueryRow(l.ctx, `
		SELECT message_id, recipients->>0, subject, body, html_body FROM comms_outbound WHERE template_key = $1`,
		consent.TemplateOptOutAcknowledgement).Scan(&send.MessageID, &send.Recipient,
		&send.Rendered.Subject, &send.Rendered.Body, &markup); err != nil {
		t.Fatalf("reading the staged receipt: %v", err)
	}
	for name, at := range map[string]time.Time{"received": received, "stopped": stopped} {
		if want := at.In(vietnamTime).Format("15:04, 02/01/2006"); !strings.Contains(send.Rendered.Body, want) {
			t.Errorf("the receipt does not print the %s time %s in Vietnam time:\n%s", name, want, send.Rendered.Body)
		}
	}
	if strings.Contains(send.Rendered.Subject+send.Rendered.Body+markup, "Margince") {
		t.Errorf("the receipt names Margince, which Art. 20.4(b) treats as advertising:\n%s\n%s", send.Rendered.Body, markup)
	}

	// A second receipt under the same refusal's message id is refused.
	again, category, err := consent.RenderOptOutAcknowledgement("en", consent.RefusalTimes{ReceivedAt: received, StoppedAt: stopped})
	if err != nil {
		t.Fatal(err)
	}
	send.Rendered, send.Category, send.ContactID = again, category, ids.From[ids.ContactKind](l.contact)
	if err := compose.InstallationDB(l.e.Pool).Tx(l.ctx, func(tx pgx.Tx) error {
		_, err := l.queue.QueueConfirmationTx(l.ctx, tx, send)
		return err
	}); err == nil {
		t.Error("a second receipt for the same refusal was queued")
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE template_key = 'optout_acknowledgement'`); n != 1 {
		t.Errorf("%d receipts for one refusal, want 1", n)
	}

	// Unsent past its alarm time, the receipt is one that System health counts.
	var due time.Time
	if err := l.e.Pool.QueryRow(l.ctx, `SELECT delivery_due_at FROM comms_outbound WHERE message_id = $1`,
		send.MessageID).Scan(&due); err != nil {
		t.Fatal(err)
	}
	for at, want := range map[time.Time]int{due.Add(-time.Minute): 0, due.Add(time.Minute): 1} {
		var overdue int
		if err := compose.InstallationDB(l.e.Pool).Tx(l.ctx, func(tx pgx.Tx) error {
			var err error
			overdue, err = comms.UnraisedOverdueReceiptCountTx(l.ctx, tx, at, noticekeys.OptOutReceiptPrefix)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if overdue != want {
			t.Errorf("at %v, %d overdue receipts, want %d", at, overdue, want)
		}
	}
}

// TestRefusalsAndReceiptsAYearOldAreStillExported ages a registration, a
// refusal, a stop and their receipts by 364 days. The contact's export still
// holds every one of them (Art. 11.7 keeps them a year).
func TestRefusalsAndReceiptsAYearOldAreStillExported(t *testing.T) {
	t.Parallel()
	l := setupReceiptLane(t)
	contact := ids.From[ids.ContactKind](l.contact)
	l.e.WsExec(t, `INSERT INTO consent_purpose (key, label, class) VALUES ('newsletter', 'Newsletter', 'marketing')
		ON CONFLICT (key) DO NOTHING`)
	if _, err := l.store.PublicSaveChoices(l.ctx, contact, []consent.PreferenceChoiceInput{
		{PurposeKey: "newsletter", State: consent.StateGranted, Wording: new("News from time to time.")},
	}); err != nil {
		t.Fatalf("registering: %v", err)
	}
	if _, err := l.store.PublicWithdrawAll(l.ctx, contact, []string{"newsletter"}); err != nil {
		t.Fatalf("refusing: %v", err)
	}
	if _, err := l.store.PublicStop(l.ctx, contact, consent.StopAllMarketing, ""); err != nil {
		t.Fatalf("stopping: %v", err)
	}
	owner := OwnerConn(t)
	for _, aging := range []string{
		`UPDATE consent_event SET captured_at = captured_at - interval '364 days' WHERE contact_id = $1`,
		`UPDATE communication_suppression SET recorded_at = recorded_at - interval '364 days',
		        received_at = received_at - interval '364 days' WHERE contact_id = $1`,
		`UPDATE comms_outbound SET created_at = created_at - interval '364 days'
		  WHERE activity_id IN (SELECT activity_id FROM activity_link WHERE contact_id = $1)`,
	} {
		if _, err := owner.Exec(l.ctx, aging, l.contact); err != nil {
			t.Fatalf("ageing the records: %v", err)
		}
	}

	records, err := l.store.OptOutRecords(l.ctx, contact)
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}
	states := map[string]int{}
	for _, event := range records.ConsentEvents {
		states[string(event.NewState)]++
	}
	if states["granted"] == 0 || states["withdrawn"] == 0 {
		t.Errorf("the export holds consent events %v, want the registration and the refusal", states)
	}
	if len(records.Stops) != 1 {
		t.Errorf("the export holds %d stops, want the one recorded", len(records.Stops))
	}
	if len(records.Receipts) != 2 {
		t.Errorf("the export holds %d receipts, want one for the refusal and one for the stop", len(records.Receipts))
	}
	yearAgo := clocktest.Now(t).Add(-363 * 24 * time.Hour)
	for _, receipt := range records.Receipts {
		if receipt.QueuedAt.After(yearAgo) || receipt.Body == nil {
			t.Errorf("receipt %s is not the aged one with its words: %+v", receipt.MessageId, receipt)
		}
	}
}

// TestAMissedReceiptShowsInSystemHealth refuses advertising over the wire for
// a contact the receipt rules cover, on an app with no mail relay. The receipt
// cannot be staged, and System health reports it under the receipts source.
func TestAMissedReceiptShowsInSystemHealth(t *testing.T) {
	t.Parallel()
	owingReceipts()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := createdRecord(t, e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Refusing Contact",
		"emails": []AnyMap{{"email": "refusing@example.test", "is_primary": true}},
	})
	if _, err := e.Owner.Exec(t.Context(), `UPDATE contact SET address_country = upper($2) WHERE id = $1`,
		contact, string(receiptCountry)); err != nil {
		t.Fatal(err)
	}
	if status := e.Call(t, "POST", "/v1/contacts/"+contact+"/consent/suppress",
		AnyMap{"kind": "marketing_objection"}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("recording the refusal → %d, want 204", status)
	}
	var report crmcontracts.SystemHealth
	if status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("system health → %d", status)
	}
	for _, source := range report.Sources {
		if source.Id != "receipts" {
			continue
		}
		count := -1
		if source.Count != nil {
			count = *source.Count
		}
		if source.Code != "receipts_missed" || count != 1 {
			t.Errorf("the receipts source reads %s with count %d, want receipts_missed with 1", source.Code, count)
		}
		return
	}
	t.Error("System health has no receipts source")
}

// refusingContact seeds one more contact under the receipt rules and records
// its refusal on the link the subject pressed.
func (l *receiptLaneEnv) refusingContact(t *testing.T, name string) ids.UUID {
	t.Helper()
	contact := l.e.SeedContact(t, name, &l.e.Rep1)
	l.e.WsExec(t, `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, $2, true, 'manual', 'human:test')`, contact, contact.String()+"@example.test")
	l.e.WsExec(t, `UPDATE contact SET address_country = upper($2) WHERE id = $1`, contact, string(receiptCountry))
	if _, err := l.store.PublicStop(l.ctx, ids.From[ids.ContactKind](contact), consent.StopAllMarketing, ""); err != nil {
		t.Fatalf("stopping %s: %v", name, err)
	}
	return contact
}

// TestRetentionKeepsAYearsRefusalsAndReleasesOlderOnes runs the real
// retention pass with a one-day contact anonymize and activity erase policy.
// A refusal and receipt inside the pack's year stay whole and exported; one
// past it is anonymized like any other contact.
func TestRetentionKeepsAYearsRefusalsAndReleasesOlderOnes(t *testing.T) {
	t.Parallel()
	l := setupReceiptLane(t)
	young := l.refusingContact(t, "Young Refusal")
	old := l.refusingContact(t, "Old Refusal")
	owner := OwnerConn(t)
	for _, aging := range []string{
		`UPDATE contact SET created_at = now() - interval '3 days', entered_at = now() - interval '3 days'
		  WHERE id = ANY($1)`,
		`UPDATE activity SET created_at = now() - interval '3 days', occurred_at = now() - interval '3 days'
		  WHERE id IN (SELECT activity_id FROM activity_link WHERE contact_id = ANY($1))`,
	} {
		if _, err := owner.Exec(l.ctx, aging, []ids.UUID{young, old}); err != nil {
			t.Fatal(err)
		}
	}
	// The old refusal's year has run out.
	for _, aging := range []string{
		`UPDATE communication_suppression SET kept_until = now() - interval '1 day' WHERE contact_id = $1`,
		`UPDATE comms_outbound SET kept_until = now() - interval '1 day'
		  WHERE activity_id IN (SELECT activity_id FROM activity_link WHERE contact_id = $1)`,
	} {
		if _, err := owner.Exec(l.ctx, aging, old); err != nil {
			t.Fatal(err)
		}
	}
	l.e.WsExec(t, `INSERT INTO retention_policy (object_type, category, retain_days, action)
		VALUES ('contact', 'no_consent_no_deal', 1, 'anonymize'), ('activity', NULL, 1, 'erase')`)

	svc := compose.NewRetentionServiceFor(l.e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(RetentionPassCtx(context.Background(), l.e.WS)); err != nil {
		t.Fatalf("running retention: %v", err)
	}

	kept, err := l.store.OptOutRecords(l.ctx, ids.From[ids.ContactKind](young))
	if err != nil {
		t.Fatal(err)
	}
	if len(kept.Stops) != 1 || len(kept.Receipts) != 1 || kept.Receipts[0].Body == nil {
		t.Errorf("a refusal inside its year lost records to retention: %d stops, %d receipts %+v",
			len(kept.Stops), len(kept.Receipts), kept.Receipts)
	}
	names := map[ids.UUID]string{}
	for _, id := range []ids.UUID{young, old} {
		var name string
		if err := owner.QueryRow(l.ctx, `SELECT full_name FROM contact WHERE id = $1`, id).Scan(&name); err != nil {
			t.Fatal(err)
		}
		names[id] = name
	}
	if names[young] != "Young Refusal" {
		t.Errorf("the contact with a refusal inside its year was anonymized to %q", names[young])
	}
	if names[old] == "Old Refusal" {
		t.Error("the contact whose refusal left its year was not anonymized: the floor never ends")
	}
}

// TestARelinkedReceiptStaysInItsRefusersExport moves a receipt's timeline link
// to another contact. The export finds a receipt by the refusal it answers, so
// it stays with the contact who refused and does not reach the other one.
func TestARelinkedReceiptStaysInItsRefusersExport(t *testing.T) {
	t.Parallel()
	l := setupReceiptLane(t)
	refuser := l.refusingContact(t, "Refusing Subject")
	other := l.e.SeedContact(t, "Other Subject", &l.e.Rep1)
	if _, err := OwnerConn(t).Exec(l.ctx, `UPDATE activity_link SET contact_id = $2
		WHERE contact_id = $1 AND activity_id IN (SELECT activity_id FROM comms_outbound WHERE template_key = 'optout_acknowledgement')`,
		refuser, other); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[ids.UUID]int{refuser: 1, other: 0} {
		records, err := l.store.OptOutRecords(l.ctx, ids.From[ids.ContactKind](id))
		if err != nil {
			t.Fatal(err)
		}
		if len(records.Receipts) != want {
			t.Errorf("contact %s exports %d receipts, want %d", id, len(records.Receipts), want)
		}
	}
}

// TestAMergedAwayRefusersReceiptsReachTheSurvivorsExport merges a contact that
// withdrew and stopped into another. The survivor's export holds both receipts,
// found through the stop its copy came from and the merged record's withdrawals.
func TestAMergedAwayRefusersReceiptsReachTheSurvivorsExport(t *testing.T) {
	t.Parallel()
	l := setupReceiptLane(t)
	retired := ids.From[ids.ContactKind](l.contact)
	l.e.WsExec(t, `INSERT INTO consent_purpose (key, label, class) VALUES ('newsletter', 'Newsletter', 'marketing')
		ON CONFLICT (key) DO NOTHING`)
	if _, err := l.store.PublicSaveChoices(l.ctx, retired, []consent.PreferenceChoiceInput{
		{PurposeKey: "newsletter", State: consent.StateGranted, Wording: new("News from time to time.")},
	}); err != nil {
		t.Fatalf("registering: %v", err)
	}
	if _, err := l.store.PublicWithdrawAll(l.ctx, retired, []string{"newsletter"}); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	if _, err := l.store.PublicStop(l.ctx, retired, consent.StopAllMarketing, ""); err != nil {
		t.Fatalf("stopping: %v", err)
	}
	survivor := ids.From[ids.ContactKind](l.e.SeedContact(t, "Surviving Subject", &l.e.Rep1))
	merger := l.e.Contacts.WithStopCarrier(l.store).WithSatelliteCarriers(l.store, nil)
	if _, err := merger.MergeContact(l.e.Admin(), retired, survivor, nil); err != nil {
		t.Fatalf("merging: %v", err)
	}

	records, err := l.store.OptOutRecords(l.ctx, survivor)
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}
	if len(records.Receipts) != 2 {
		t.Errorf("the survivor's export holds %d receipts, want the merged record's withdrawal and stop receipts: %+v",
			len(records.Receipts), records.Receipts)
	}
}
