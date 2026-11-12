// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Lifting a restriction of processing (GDPR Art. 18(3)): the admin's lift
// queues a notice, and the restriction ends only once the dispatcher hands that
// notice to the mail provider. A notice that does not go out leaves it.

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/testsmtp"
)

// restrictLaneContact records a restriction on the lane's contact and answers
// the stop.
func restrictLaneContact(t *testing.T, l *confirmLaneEnv) ids.UUID {
	t.Helper()
	if err := l.store.Suppress(l.e.Admin(), consent.SuppressInput{
		ContactID: ids.From[ids.ContactKind](l.contact), Kind: "processing_restriction",
	}); err != nil {
		t.Fatalf("recording the restriction: %v", err)
	}
	var stop ids.UUID
	if err := l.e.Pool.QueryRow(context.Background(), `SELECT id FROM communication_suppression
		 WHERE contact_id = $1 AND kind = 'processing_restriction' ORDER BY recorded_at DESC, id DESC LIMIT 1`, l.contact).Scan(&stop); err != nil {
		t.Fatal(err)
	}
	return stop
}

func liveRestrictions(t *testing.T, l *confirmLaneEnv) int {
	t.Helper()
	return l.e.WsCount(t, `SELECT count(*) FROM communication_suppression
		 WHERE contact_id = '`+l.contact.String()+`' AND kind = 'processing_restriction' AND revoked_at IS NULL`)
}

// askToLift is the admin pressing Lift the restriction.
func askToLift(t *testing.T, l *confirmLaneEnv, stop ids.UUID) ids.UUID {
	t.Helper()
	if err := l.store.RequestRestrictionLift(l.e.Admin(), consent.LiftInput{
		ContactID: ids.From[ids.ContactKind](l.contact), SuppressionID: stop, Reason: "the dispute is settled",
	}); err != nil {
		t.Fatalf("asking to lift: %v", err)
	}
	var delivery ids.UUID
	if err := l.e.Pool.QueryRow(context.Background(), `SELECT id FROM comms_outbound
		 WHERE template_key = 'restriction_lift' AND status = 'pending'`).Scan(&delivery); err != nil {
		t.Fatalf("the lift queued no notice: %v", err)
	}
	return delivery
}

func stopIsLive(t *testing.T, l *confirmLaneEnv, stop ids.UUID) bool {
	t.Helper()
	return l.e.WsCount(t, `SELECT count(*) FROM communication_suppression WHERE id = '`+stop.String()+`' AND revoked_at IS NULL`) == 1
}

// liftNoticeOf reads the restriction's row on the stops panel, as the admin.
func liftNoticeOf(t *testing.T, l *confirmLaneEnv, stop ids.UUID, offered bool) *crmcontracts.RestrictionLiftNotice {
	t.Helper()
	stops, err := l.store.ListStops(l.e.Admin(), ids.From[ids.ContactKind](l.contact))
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range stops {
		if ids.UUID(listed.Id) == stop {
			if listed.MayLiftRestriction == nil || *listed.MayLiftRestriction != offered {
				t.Errorf("may_lift_restriction = %v, want %v", listed.MayLiftRestriction, offered)
			}
			return listed.LiftNotice
		}
	}
	t.Fatal("the restriction is missing from the stops panel")
	return nil
}

func dispatchNotice(t *testing.T, l *confirmLaneEnv, delivery ids.UUID, relay mailer.Mailer) comms.Outcome {
	t.Helper()
	dispatcher := newSendDispatcher(l.e.Pool, nil, sendPace{limit: 10, window: time.Minute, maxAge: time.Hour}, nil, nil, nil).
		WithControllerRelay(ControllerRelayFor(relay), l.vault)
	outcome, _, err := dispatcher.DispatchWithWait(sendWorkerScope(l.ctx), delivery)
	if err != nil && outcome != comms.OutcomeRetry {
		t.Fatalf("dispatching the notice: %s %v", outcome, err)
	}
	return outcome
}

// The contact asked twice, so two restriction rows stand. The lift ends both,
// as the notice says their data is processed again.
func TestARestrictionEndsOnceItsLiftNoticeIsSentAndHeldMailIsReadThen(t *testing.T) {
	l := setupConfirmLane(t)
	restrictLaneContact(t, l)
	stop := restrictLaneContact(t, l)
	if n := liveRestrictions(t, l); n != 2 {
		t.Fatalf("live restrictions = %d, want 2", n)
	}
	held := mailFromAddress(t, l.e, "sent while restricted", "subject@example.test")
	classifier := NewCaptureClassifier(l.e.Pool, &scriptedClassifyBrain{}, slog.New(slog.DiscardHandler))
	pass := principal.WithWorkspaceID(context.Background(), l.e.WS)
	if err := classifier.RunWorkspace(pass, 0); err != nil {
		t.Fatal(err)
	}
	if labelOf(t, l.e, held) != nil {
		t.Fatal("mail of a restricted contact was read")
	}

	delivery := askToLift(t, l, stop)
	if !stopIsLive(t, l, stop) {
		t.Fatal("the restriction ended before its notice went out")
	}
	if notice := liftNoticeOf(t, l, stop, false); notice == nil || notice.Status != crmcontracts.RestrictionLiftNoticeStatusQueued {
		t.Fatalf("the stops panel shows the notice as %+v, want queued", notice)
	}

	relay := testsmtp.Start(t, false)
	host, port := relay.Address(t)
	if outcome := dispatchNotice(t, l, delivery, mailer.SMTP{Host: host, Port: port, FromAddress: "privacy@example.test"}); outcome != comms.OutcomeSent {
		t.Fatalf("the notice was not sent: %s", outcome)
	}
	if n := liveRestrictions(t, l); n != 0 {
		t.Fatalf("%d restrictions stayed after the lift notice was sent", n)
	}
	if err := classifier.RunWorkspace(pass, 0); err != nil {
		t.Fatal(err)
	}
	if labelOf(t, l.e, held) == nil {
		t.Fatal("mail held during the restriction was not read after the lift")
	}
}

// A restriction asked for while the lift notice waits is a newer objection.
// The notice answers only the restrictions that stood when the admin asked.
func TestARestrictionRecordedWhileTheLiftNoticeWaitsStays(t *testing.T) {
	l := setupConfirmLane(t)
	stop := restrictLaneContact(t, l)
	delivery := askToLift(t, l, stop)
	newer := restrictLaneContact(t, l)
	// The test clock stands still, so the order is set by hand. First comes the
	// restriction, then the request, then the newer restriction, then the send.
	for _, step := range []struct {
		sql string
		id  ids.UUID
	}{
		{`UPDATE communication_suppression SET recorded_at = recorded_at - interval '3 minutes' WHERE id = $1`, stop},
		{`UPDATE comms_outbound SET created_at = created_at - interval '2 minutes' WHERE id = $1`, delivery},
		{`UPDATE communication_suppression SET recorded_at = recorded_at - interval '1 minute' WHERE id = $1`, newer},
	} {
		if _, err := l.e.Pool.Exec(context.Background(), step.sql, step.id); err != nil {
			t.Fatal(err)
		}
	}
	relay := testsmtp.Start(t, false)
	host, port := relay.Address(t)
	if outcome := dispatchNotice(t, l, delivery, mailer.SMTP{Host: host, Port: port, FromAddress: "privacy@example.test"}); outcome != comms.OutcomeSent {
		t.Fatalf("the notice was not sent: %s", outcome)
	}
	if stopIsLive(t, l, stop) {
		t.Fatal("the restriction the notice answers stayed")
	}
	if !stopIsLive(t, l, newer) {
		t.Fatal("a restriction recorded after the lift was asked for ended with it")
	}
}

func TestARestrictionStaysWhileItsLiftNoticeFailsAndSaysWhy(t *testing.T) {
	l := setupConfirmLane(t)
	stop := restrictLaneContact(t, l)
	delivery := askToLift(t, l, stop)

	if outcome := dispatchNotice(t, l, delivery, &countingMailer{fail: errors.New("relay refused the message")}); outcome == comms.OutcomeSent {
		t.Fatal("a refused notice was recorded as sent")
	}
	if !stopIsLive(t, l, stop) {
		t.Fatal("the restriction ended although its notice did not go out")
	}
	if notice := liftNoticeOf(t, l, stop, false); notice == nil || notice.Reason == nil || *notice.Reason == "" {
		t.Fatalf("the stops panel does not say why the notice is not out: %+v", notice)
	}

	store := comms.NewStore(InstallationDB(l.e.Pool), func() time.Time { return clocktest.Now(t) }, activities.NewStore(InstallationDB(l.e.Pool)))
	if err := store.Park(sendWorkerScope(l.ctx), delivery, "the recipient's server rejected the address"); err != nil {
		t.Fatal(err)
	}
	notice := liftNoticeOf(t, l, stop, true)
	if notice == nil || notice.Status != crmcontracts.RestrictionLiftNoticeStatusFailed {
		t.Fatalf("a notice given up on shows as %+v, want failed", notice)
	}
	if !stopIsLive(t, l, stop) {
		t.Fatal("the restriction ended with its notice given up on")
	}
}

// Only a human admin lifts a restriction, as only one records it.
func TestOnlyAnAdminSignedInAsksToLiftARestriction(t *testing.T) {
	l := setupConfirmLane(t)
	stop := restrictLaneContact(t, l)
	for name, ctx := range map[string]context.Context{
		"a rep":             l.ctx,
		"the admin's agent": agentOfTheAdmin(t, l.e),
	} {
		err := l.store.RequestRestrictionLift(ctx, consent.LiftInput{
			ContactID: ids.From[ids.ContactKind](l.contact), SuppressionID: stop, Reason: "settled",
		})
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s: %v, want permission denied", name, err)
		}
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE template_key = 'restriction_lift'`); n != 0 {
		t.Fatalf("a refused lift queued %d notices", n)
	}
}

// The receipt commits on its own. When the lift that follows it fails, the stop
// shows the notice as sent, no second lift is offered, and the sweep ends it.
func TestTheSweepEndsARestrictionWhoseSentNoticeTheReceiptCouldNotLift(t *testing.T) {
	l := setupConfirmLane(t)
	stop := restrictLaneContact(t, l)
	delivery := askToLift(t, l, stop)

	failing := comms.NewStore(InstallationDB(l.e.Pool), func() time.Time { return clocktest.Now(t) },
		activities.NewStore(InstallationDB(l.e.Pool)),
		comms.WithSentEffect(func(context.Context, string) error { return errors.New("the database went away") }))
	if err := failing.RecordSent(sendWorkerScope(l.ctx), delivery, connector.SendReceipt{ProviderMessageID: "relay-accepted"}); err != nil {
		t.Fatalf("a failing lift must not fail the receipt: %v", err)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE id = '`+delivery.String()+`' AND status = 'sent'`); n != 1 {
		t.Fatal("the receipt did not commit")
	}
	if !stopIsLive(t, l, stop) {
		t.Fatal("the failing lift ended the restriction")
	}
	if notice := liftNoticeOf(t, l, stop, false); notice == nil || notice.Status != crmcontracts.RestrictionLiftNoticeStatusSent {
		t.Fatalf("the stops panel shows %+v, want the notice as sent", notice)
	}

	if err := DriveControllerPayloadSweepForTest(context.Background(), l.e.Pool, nil, clocktest.Now(t)); err != nil {
		t.Fatalf("the sweep: %v", err)
	}
	if stopIsLive(t, l, stop) {
		t.Fatal("the sweep left a restriction whose lift notice was sent")
	}
}

// A second lift while a notice waits would send the contact a second notice.
func TestASecondLiftIsRefusedWhileTheNoticeWaits(t *testing.T) {
	l := setupConfirmLane(t)
	stop := restrictLaneContact(t, l)
	askToLift(t, l, stop)

	err := l.store.RequestRestrictionLift(l.e.Admin(), consent.LiftInput{
		ContactID: ids.From[ids.ContactKind](l.contact), SuppressionID: stop, Reason: "again",
	})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a second lift answered %v, want a conflict", err)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE template_key = 'restriction_lift'`); n != 1 {
		t.Fatalf("%d lift notices queued, want 1", n)
	}
	if notice := liftNoticeOf(t, l, stop, false); notice == nil {
		t.Fatal("the stops panel lost the waiting notice")
	}
}
