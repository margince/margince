// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Discharging a duty against a real database: the send moves the case, the
// route decides which cases it moves, and a second send inside the cooldown
// moves nothing.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedNoticeCase writes one live case for this contact, owed and never sent to.
//
// attempts stays 0, which is what a freshly opened case holds — the cooldown is
// driven by discharging and then discharging again, not by dating the seed,
// because a case that has never had a disclosure sent must never be inside one.
func seedNoticeCase(
	t *testing.T, e *channelConsentEnv, kind string, routes []string,
) ids.UUID {
	t.Helper()
	acq := seedAcquisition(t, e, kind)
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		INSERT INTO privacy_notice_case
		       (contact_id, acquisition_id, rule, due_at, allowed_routes, state)
		VALUES ($1, $2, 'art14', now() + interval '30 days', $3, 'open')
		RETURNING id`, e.contact, acq, routes).Scan(&id); err != nil {
		t.Fatalf("seeding the notice case: %v", err)
	}
	return id
}

func noticeCaseState(t *testing.T, e *channelConsentEnv, id ids.UUID) (string, int) {
	t.Helper()
	var state string
	var attempts int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT state, attempts FROM privacy_notice_case WHERE id = $1`, id).
		Scan(&state, &attempts); err != nil {
		t.Fatalf("reading the notice case: %v", err)
	}
	return state, attempts
}

// discharge sends the record-confirmation disclosure and answers how many
// duties it moved. The route is fixed: it is the only one any writer honours,
// and the reply route is exercised by seeding a case that names it instead.
func discharge(t *testing.T, e *channelConsentEnv, now time.Time) int {
	t.Helper()
	delivery := seedDelivery(t, e)
	var moved int
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		moved, err = dischargeNoticeCases(e.ctx, tx, e.contact,
			noticeRouteRecordConfirmation, now, delivery)
		return err
	}); err != nil {
		t.Fatalf("discharging: %v", err)
	}
	return moved
}

// seedDelivery writes one sent comms_outbound row and answers its id.
//
// A real row, not a fresh uuid: the case's delivery_id is a foreign key, which
// is what makes "show me the message you say you sent" answerable at all. A
// fixture pointing at nothing would pass the Go and fail the database.
//
// On the CONTROLLER lane, carrying the record-confirmation template and no
// consent_purpose, because that is what a disclosure is: the table refuses a
// purpose on anything but a user's own mail, and requires a template on
// anything that is not.
func seedDelivery(t *testing.T, e *channelConsentEnv) ids.UUID {
	t.Helper()
	activityID := ids.New[ids.ActivityKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, source, captured_by)
		VALUES ($1, 'email', 'test', 'human:x')`, activityID); err != nil {
		t.Fatalf("seeding the disclosure's activity: %v", err)
	}
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO comms_outbound
		    (id, activity_id, provider, message_id, recipients, subject, body,
		     status, sender_kind, template_key, template_version)
		VALUES ($1, $2, 'controller', $3, '["subject@notice.test"]'::jsonb,
		        'Your details', 'body', 'sent', 'controller', $4, 1)`,
		id, activityID, "notice-"+activityID.String()+"@test.invalid",
		TemplateRecordConfirmation); err != nil {
		t.Fatalf("seeding the disclosure's delivery: %v", err)
	}
	return id
}

// TestSendingTheDisclosureMovesTheDuty is the whole point of the route: before
// this, allowed_routes named a way to discharge a case that no writer honoured,
// so a duty stayed open forever however many disclosures went out.
func TestSendingTheDisclosureMovesTheDuty(t *testing.T) {
	e := setupChannelConsent(t)
	owed := seedNoticeCase(t, e, "purchased_or_imported",
		[]string{noticeRouteRecordConfirmation})

	if moved := discharge(t, e, time.Now()); moved != 1 {
		t.Fatalf("the send discharged %d duties, want 1", moved)
	}
	state, attempts := noticeCaseState(t, e, owed)
	// QUEUED rather than completed: the mail is staged, not received, and the
	// delivery row is what learns the difference.
	if state != string(NoticeQueued) {
		t.Errorf("the discharged case rests in %q, want %q", state, NoticeQueued)
	}
	if attempts != 1 {
		t.Errorf("the case counted %d attempts, want 1", attempts)
	}
}

// TestADutyIsOnlyDischargedByARouteItNamed holds the route match. Without it a
// record-confirmation mail would settle an Art. 13 form case whose disclosure
// rides a reply somebody else has to write — a duty marked handled by a message
// that does not contain it.
func TestADutyIsOnlyDischargedByARouteItNamed(t *testing.T) {
	e := setupChannelConsent(t)
	byReply := seedNoticeCase(t, e, "event_or_form", []string{noticeRouteReply})

	if moved := discharge(t, e, time.Now()); moved != 0 {
		t.Fatalf("a record-confirmation mail discharged %d reply-route duties, want 0", moved)
	}
	if state, _ := noticeCaseState(t, e, byReply); state != string(NoticeOpen) {
		t.Errorf("the reply-route case moved to %q; only its own route may discharge it", state)
	}
}

// TestASecondSendInsideTheCooldownMovesNothing holds the cooldown. A rep
// clicking twice, or a screen retrying, is one disclosure — counting it as two
// would let attempts read as effort nobody made and would keep re-dating a case
// whose subject received a single mail.
func TestASecondSendInsideTheCooldownMovesNothing(t *testing.T) {
	e := setupChannelConsent(t)
	owed := seedNoticeCase(t, e, "purchased_or_imported",
		[]string{noticeRouteRecordConfirmation})
	now := time.Now()

	if moved := discharge(t, e, now); moved != 1 {
		t.Fatalf("the first send discharged %d duties, want 1", moved)
	}
	if moved := discharge(t, e, now.Add(time.Hour)); moved != 0 {
		t.Fatalf("a second send an hour later discharged %d duties, want 0", moved)
	}
	if _, attempts := noticeCaseState(t, e, owed); attempts != 1 {
		t.Errorf("the case counted %d attempts after two clicks, want 1", attempts)
	}

	// And past the cooldown it moves again: the window bounds a double-click,
	// it does not retire the duty. A case still queued a month later was not
	// discharged by whatever went out, and a fresh send is real progress.
	past := now.Add(noticeResendCooldown + time.Hour)
	if moved := discharge(t, e, past); moved != 1 {
		t.Fatalf("a send past the cooldown discharged %d duties, want 1", moved)
	}
	if _, attempts := noticeCaseState(t, e, owed); attempts != 2 {
		t.Errorf("the case counted %d attempts after a genuine re-send, want 2", attempts)
	}
}

// TestABlockedDutyIsNotDischargedBySending holds the state filter. A blocked
// case names an obstacle; moving it to queued would leave the row asserting
// both that a disclosure went out and that one could not.
//
// The blocked_shape CHECK also refuses that row, and the filter is what turns
// its refusal into a no-op rather than a failed transaction: without it, one
// blocked case on a contact would make EVERY confirm mail to them fail with a
// raw constraint violation. So the assertion is that the send SUCCEEDS and
// moves nothing — a mutation removing the filter fails here on the error, not
// on the state.
func TestABlockedDutyIsNotDischargedBySending(t *testing.T) {
	e := setupChannelConsent(t)
	acq := seedAcquisition(t, e, "purchased_or_imported")
	var blocked ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		INSERT INTO privacy_notice_case
		       (contact_id, acquisition_id, rule, due_at, allowed_routes, state, blocked_reason)
		VALUES ($1, $2, 'art14', now() + interval '30 days', $3, 'blocked', 'no live address')
		RETURNING id`, e.contact, acq, []string{noticeRouteRecordConfirmation}).Scan(&blocked); err != nil {
		t.Fatalf("seeding the blocked case: %v", err)
	}

	if moved := discharge(t, e, time.Now()); moved != 0 {
		t.Fatalf("a send discharged %d blocked duties, want 0", moved)
	}
	if state, _ := noticeCaseState(t, e, blocked); state != string(NoticeBlocked) {
		t.Errorf("the blocked case moved to %q; the obstacle has to be cleared first", state)
	}
}

// TestTheConfirmMailItselfDischargesTheDuty is the binding the two halves rest
// on: minting a record-confirmation link is what discharges an Art. 14 case.
//
// Asserted through IssueConfirmToken rather than by calling the writer directly,
// because the defect this closes was precisely that nothing called it. A test
// that reaches past the door would pass against a door that never opened.
//
// WITH A LANE WIRED, which the environment does not do by default. The first
// version of this test ran on the bare store, where stageConfirmMail returns
// (false, nil) and no mail exists at all — so a test named for the confirm mail
// passed in the one configuration that has none. The lane is what makes the
// subject of this test real.
func TestTheConfirmMailItselfDischargesTheDuty(t *testing.T) {
	e := setupChannelConsent(t)
	owed := seedNoticeCase(t, e, "purchased_or_imported",
		[]string{noticeRouteRecordConfirmation})
	seedSubjectAddress(t, e)
	// A stager that writes a REAL delivery, because the case will point at it.
	e.store = e.store.WithConfirmationLane(
		&recordingStager{stage: func() ids.UUID { return seedDelivery(t, e) }},
		&recordingVault{}, "https://crm.example.test/")

	issued, err := e.store.IssueConfirmToken(e.ctx, e.contact)
	if err != nil {
		t.Fatalf("mint a confirm link: %v", err)
	}
	if !issued.Staged {
		t.Fatal("the lane is wired and the mail was not staged; this test would prove nothing")
	}
	if issued.NoticeCasesDischarged != 1 {
		t.Fatalf("the mail discharged %d duties, want 1", issued.NoticeCasesDischarged)
	}
	if state, _ := noticeCaseState(t, e, owed); state != string(NoticeQueued) {
		t.Errorf("the case rests in %q after its disclosure was sent, want %q", state, NoticeQueued)
	}
}

// TestAnUnsentDisclosureDischargesNothing is the arm the first version of the
// test above was silently standing in for.
//
// An installation with no relay wired still MINTS the link — refusing would
// invite a retry that supersedes the first — and reports queued=false. Nothing
// was sent, so nothing is discharged: a case moved here would carry an audit row
// asserting a duty was met by a message that was never staged, and its cooldown
// would then suppress the genuine send once an operator fixed the relay.
func TestAnUnsentDisclosureDischargesNothing(t *testing.T) {
	e := setupChannelConsent(t)
	owed := seedNoticeCase(t, e, "purchased_or_imported",
		[]string{noticeRouteRecordConfirmation})

	issued := issueLink(t, e)
	if issued.Staged {
		t.Fatal("the bare environment staged a mail; this test needs the no-lane case")
	}
	if issued.NoticeCasesDischarged != 0 {
		t.Fatalf("a mail that was never staged discharged %d duties, want 0",
			issued.NoticeCasesDischarged)
	}
	if state, _ := noticeCaseState(t, e, owed); state != string(NoticeOpen) {
		t.Errorf("the case moved to %q on a disclosure nobody sent", state)
	}
}

// TestAConsentLinkDischargesNoDisclosureDuty holds the other half of
// noticeRouteFor. A double-opt-in link asks whether the subject wants
// marketing; it does not tell them we hold their data, so the Art. 14 duty
// stands exactly as owed as before.
func TestAConsentLinkDischargesNoDisclosureDuty(t *testing.T) {
	e := setupChannelConsent(t)
	owed := seedNoticeCase(t, e, "purchased_or_imported",
		[]string{noticeRouteRecordConfirmation})
	seedSubjectAddress(t, e)

	seedMarketingPurpose(t, e)
	var purpose ids.PurposeID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM consent_purpose WHERE key = $1`, PurposeMarketingEmail).
		Scan(&purpose); err != nil {
		t.Fatalf("read the marketing purpose: %v", err)
	}
	issued, err := e.store.IssueConsentLink(e.ctx, e.contact, purpose, "")
	if err != nil {
		t.Fatalf("mint a consent link: %v", err)
	}
	if issued.NoticeCasesDischarged != 0 {
		t.Fatalf("a consent link discharged %d disclosure duties, want 0",
			issued.NoticeCasesDischarged)
	}
	if state, _ := noticeCaseState(t, e, owed); state != string(NoticeOpen) {
		t.Errorf("the case moved to %q on a link that carries no disclosure", state)
	}
}

// TestABouncedDisclosureReopensTheDuty is the failure this slice closes.
//
// `queued` is the honest word for "a message is on its way" — it deliberately
// stops short of claiming the subject was told. It was also where the case
// STAYED when that message bounced: the duty was not met, nobody was told, and
// the queue showed a case somebody had handled.
//
// That is the worst of the nine states to get wrong. An open case is visible
// and gets worked. A queued case that silently failed looks BETTER than an open
// one while being worse, so nobody looks at it again.
func TestABouncedDisclosureReopensTheDuty(t *testing.T) {
	e := setupChannelConsent(t)
	owed := seedNoticeCase(t, e, "purchased_or_imported",
		[]string{noticeRouteRecordConfirmation})
	delivery := seedDelivery(t, e)

	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err := dischargeNoticeCases(e.ctx, tx, e.contact,
			noticeRouteRecordConfirmation, time.Now(), delivery)
		return err
	}); err != nil {
		t.Fatalf("sending the disclosure: %v", err)
	}
	if state, _ := noticeCaseState(t, e, owed); state != string(NoticeQueued) {
		t.Fatalf("the case rests in %q after its disclosure was sent, want %q", state, NoticeQueued)
	}

	// The message dies.
	var moved int
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		moved, err = MarkNoticeDeliveryFailedTx(e.ctx, tx, delivery)
		return err
	}); err != nil {
		t.Fatalf("recording the failed delivery: %v", err)
	}
	if moved != 1 {
		t.Errorf("the bounce reopened %d duties, want 1", moved)
	}
	state, _ := noticeCaseState(t, e, owed)
	if state != string(NoticeDeliveryFailed) {
		t.Errorf("the case rests in %q after its disclosure bounced, want %q: the subject "+
			"was not told, so the duty is owed again", state, NoticeDeliveryFailed)
	}
}

// TestAFailedDisclosureIsBackOnTheQueue. Reopening the case is only half of it:
// the point is that somebody sees it again.
func TestAFailedDisclosureIsBackOnTheQueue(t *testing.T) {
	e := setupChannelConsent(t)
	owed := seedNoticeCase(t, e, "purchased_or_imported",
		[]string{noticeRouteRecordConfirmation})
	delivery := seedDelivery(t, e)

	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		if _, err := dischargeNoticeCases(e.ctx, tx, e.contact,
			noticeRouteRecordConfirmation, time.Now(), delivery); err != nil {
			return err
		}
		_, err := MarkNoticeDeliveryFailedTx(e.ctx, tx, delivery)
		return err
	}); err != nil {
		t.Fatalf("sending and failing the disclosure: %v", err)
	}

	owedNow, err := e.store.OpenNoticeCasesDueSoonest(officerCtx(e), 0)
	if err != nil {
		t.Fatalf("reading the queue: %v", err)
	}
	var found bool
	for _, c := range owedNow {
		if c.ID == owed {
			found = true
		}
	}
	if !found {
		t.Error("a duty whose disclosure bounced is not on the queue: it reads as handled " +
			"and is not, which is the one way this control fails invisibly")
	}
}

// TestALateBounceDoesNotDragBackASettledDuty. A case somebody has since excused,
// or that a later disclosure completed, is not reopened by a late report about
// an older message: the state moved on for a reason, and a bounce is evidence
// about one delivery rather than about the duty's whole history.
func TestALateBounceDoesNotDragBackASettledDuty(t *testing.T) {
	e := setupChannelConsent(t)
	owed := seedNoticeCase(t, e, "purchased_or_imported",
		[]string{noticeRouteRecordConfirmation})
	delivery := seedDelivery(t, e)

	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err := dischargeNoticeCases(e.ctx, tx, e.contact,
			noticeRouteRecordConfirmation, time.Now(), delivery)
		return err
	}); err != nil {
		t.Fatalf("sending the disclosure: %v", err)
	}
	if _, err := e.store.ExcuseNoticeCase(officerCtx(e), owed, ExcuseInput{
		State: NoticeProvidedElsewhere,
		Note:  "they were told in person at the workshop",
		Now:   time.Now(),
	}); err != nil {
		t.Fatalf("excusing the duty: %v", err)
	}

	var moved int
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		moved, err = MarkNoticeDeliveryFailedTx(e.ctx, tx, delivery)
		return err
	}); err != nil {
		t.Fatalf("recording the failed delivery: %v", err)
	}
	if moved != 0 {
		t.Errorf("a late bounce reopened %d settled duties, want 0", moved)
	}
	if state, _ := noticeCaseState(t, e, owed); state != string(NoticeProvidedElsewhere) {
		t.Errorf("the excused case moved to %q on a late bounce", state)
	}
}

// TestTheCooldownMeasuresFromTheSendAndNotFromAnyWrite.
//
// The window used to measure from updated_at, which moves for ANY write — a
// merge relinking the contact extended somebody's cooldown by up to a month.
// last_sent_at records when a disclosure actually went, which is the question
// the cooldown is asking.
func TestTheCooldownMeasuresFromTheSendAndNotFromAnyWrite(t *testing.T) {
	e := setupChannelConsent(t)
	owed := seedNoticeCase(t, e, "purchased_or_imported",
		[]string{noticeRouteRecordConfirmation})
	delivery := seedDelivery(t, e)

	// A disclosure sent long ago, well outside the cooldown.
	longAgo := time.Now().Add(-60 * 24 * time.Hour)
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err := dischargeNoticeCases(e.ctx, tx, e.contact,
			noticeRouteRecordConfirmation, longAgo, delivery)
		return err
	}); err != nil {
		t.Fatalf("sending the first disclosure: %v", err)
	}
	// Something else touches the row, the way a merge does.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE privacy_notice_case SET updated_at = now() WHERE id = $1`, owed); err != nil {
		t.Fatalf("touching the case: %v", err)
	}

	if moved := discharge(t, e, time.Now()); moved != 1 {
		t.Errorf("a second disclosure 60 days after the first moved %d duties, want 1: the "+
			"cooldown measures from the SEND, and a merge touching the row is not a send",
			moved)
	}
}

// TestAResendAfterAFailureTracksTheNewMessage. A case reopened by a bounce
// still points at the delivery that died — that is its evidence — and a
// corrected re-send has to move the pointer, or the case would carry the old
// dead message as the thing it is waiting on.
func TestAResendAfterAFailureTracksTheNewMessage(t *testing.T) {
	e := setupChannelConsent(t)
	owed := seedNoticeCase(t, e, "purchased_or_imported",
		[]string{noticeRouteRecordConfirmation})
	first := seedDelivery(t, e)

	// Sent long enough ago that the cooldown has expired, then bounced.
	longAgo := time.Now().Add(-60 * 24 * time.Hour)
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		if _, err := dischargeNoticeCases(e.ctx, tx, e.contact,
			noticeRouteRecordConfirmation, longAgo, first); err != nil {
			return err
		}
		_, err := MarkNoticeDeliveryFailedTx(e.ctx, tx, first)
		return err
	}); err != nil {
		t.Fatalf("sending and failing the first disclosure: %v", err)
	}

	second := seedDelivery(t, e)
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		moved, err := dischargeNoticeCases(e.ctx, tx, e.contact,
			noticeRouteRecordConfirmation, time.Now(), second)
		if err == nil && moved != 1 {
			t.Errorf("the corrected re-send moved %d duties, want 1: a failed case is still "+
				"owed, so a fresh disclosure discharges it", moved)
		}
		return err
	}); err != nil {
		t.Fatalf("the corrected re-send: %v", err)
	}

	var carried ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT delivery_id FROM privacy_notice_case WHERE id = $1`, owed).Scan(&carried); err != nil {
		t.Fatalf("reading the case: %v", err)
	}
	if carried != second {
		t.Errorf("the case points at delivery %v after a re-send, want the new one (%v): a "+
			"case waiting on a message that already died cannot be reopened by the bounce "+
			"of the message it is actually waiting on", carried, second)
	}
}
