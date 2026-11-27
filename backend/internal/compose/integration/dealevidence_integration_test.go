// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A thread the rep's mailbox holds as personal keeps no deal warm: it moves no
// deal clock. A business meeting marked private on the rep's calendar still
// counts, because a privacy mark is not a judgement that it is personal.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealclockrefold"
	"github.com/margince/margince/backend/internal/compose/dealprivatemail"
	"github.com/margince/margince/backend/internal/compose/dealverdict"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/threadverdict"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type dealEvidenceWorld struct {
	e        *Env
	now      time.Time
	deal     ids.DealID
	contact  ids.UUID
	calendar string
	mailbox  string
}

// newDealEvidenceWorld seats the rep's calendar and a classified mailbox, so a
// pull records the rep's import of each entry and message.
func newDealEvidenceWorld(t *testing.T) dealEvidenceWorld {
	t.Helper()
	e := Setup(t)
	company := e.SeedCompany(t, "Acme Consulting", nil)
	w := dealEvidenceWorld{
		e: e, now: clocktest.Now(t),
		deal:     seedRepDeal(t, e, ids.From[ids.CompanyKind](company), e.Rep1, "AI consulting"),
		contact:  e.SeedContact(t, "Ada Friend", nil),
		calendar: "rep-" + ids.NewV7().String() + "@seller.test",
		mailbox:  "rep-mail-" + ids.NewV7().String() + "@seller.test",
	}
	e.WsExec(t, `INSERT INTO capture_connection (provider, user_id, status, account_label) VALUES ($1, $2, 'connected', $3)`,
		CalendarSystem, e.Rep1, w.calendar)
	e.WsExec(t, `INSERT INTO capture_connection (provider, user_id, status, account_label, credential_ref, mail_posture)
		VALUES ('gmail', $1, 'connected', $2, 'vault:test', 'classified')`, e.Rep1, w.mailbox)
	return w
}

// meeting captures one calendar entry filed on the deal and the contact, the
// way a calendar pull lands it.
func (w dealEvidenceWorld) meeting(t *testing.T, event string, at time.Time, private bool) ids.UUID {
	t.Helper()
	return CalendarMeeting{
		Event: event + "-" + ids.NewV7().String(), At: at, Seat: w.e.Rep1, Private: private, DeliveredTo: w.calendar,
		Links: []datasource.EntityRef{
			{Type: datasource.EntityDeal, ID: w.deal.UUID}, {Type: datasource.EntityContact, ID: w.contact},
		},
	}.Capture(t, w.e)
}

// mail captures one inbound message from Ada on thread, filed on the deal,
// through the sink a mail pull uses. The classified mailbox holds it and asks
// for a verdict on its thread.
func (w dealEvidenceWorld) mail(t *testing.T, thread string, at time.Time) ids.UUID {
	t.Helper()
	sourceID := "dinner-" + ids.NewV7().String()
	ref, err := capture.NewSink(w.e.DB()).WithAudienceRecompute(activities.RecomputeAudienceTx).Upsert(
		w.e.ConnectorOwnerCtx(w.e.Rep1, "gmail"), connector.NormalizedRecord{
			EntityType: "activity", NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: sourceID},
			Fields: capture.ActivityFields{
				Kind: "email", Subject: "Dinner on Friday?", Body: "Shall we try the new place?",
				Direction: connector.DirectionInbound, OccurredAt: at,
			},
			Source: "gmail:" + sourceID, CapturedBy: "connector:gmail", ThreadKey: thread,
			Raw:          []byte("From: ada@buyer.test\r\nSubject: Dinner on Friday?\r\n\r\nBody."),
			Counterparty: connector.Counterparty{Email: "ada@buyer.test", Domain: "buyer.test", Direction: connector.DirectionInbound},
			Addresses:    []string{"ada@buyer.test", w.mailbox},
			DeliveredTo:  w.mailbox,
			Links: []datasource.EntityRef{
				{Type: datasource.EntityDeal, ID: w.deal.UUID}, {Type: datasource.EntityContact, ID: w.contact},
			},
		})
	if err != nil {
		t.Fatalf("capturing the mail: %v", err)
	}
	return ref.ID
}

// judgeThread answers the mailbox's open question on thread through the
// writers the confidentiality engine applies a verdict with.
func (w dealEvidenceWorld) judgeThread(t *testing.T, thread, kind string) {
	t.Helper()
	store := capture.NewThreadVerdictStore(w.e.DB())
	due, err := store.ClaimDue(w.e.Admin(), 10)
	if err != nil {
		t.Fatalf("claiming the thread question: %v", err)
	}
	var row capture.PendingThread
	for _, d := range due {
		if d.ThreadKey == thread {
			row = d
		}
	}
	if row.ID == ids.Nil {
		t.Fatalf("the mailbox asked no question about thread %s", thread)
	}
	if err := database.WithWorkspaceTx(w.e.Admin(), w.e.Pool, func(tx pgx.Tx) error {
		won, err := store.ResolveAs(w.e.Admin(), tx, row, capture.VerdictHeld, kind, 0.95, nil, "test")
		if err != nil || !won {
			return err
		}
		outcome, err := store.RecordOutcomeOnWholeThreadTx(w.e.Admin(), tx, row.ThreadKey, row.UserID, capture.VerdictHeld, kind, nil)
		if err != nil {
			return err
		}
		stamped := []ids.ActivityID{ids.From[ids.ActivityKind](row.ActivityID)}
		for _, id := range outcome.Stamped {
			stamped = append(stamped, ids.From[ids.ActivityKind](id))
		}
		return activities.RecomputeAudiencesTx(w.e.Admin(), tx, stamped)
	}); err != nil {
		t.Fatalf("judging thread %s %s: %v", thread, kind, err)
	}
}

func (w dealEvidenceWorld) dealClock(t *testing.T) *time.Time {
	t.Helper()
	deal, err := w.e.Deals.GetDeal(w.e.Admin(), w.deal, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	return deal.LastActivityAt
}

func (w dealEvidenceWorld) meetingAhead(t *testing.T) bool {
	t.Helper()
	return w.e.WsScalar(t, `SELECT (`+deals.MeetingAheadSQL("d")+`)::text FROM deal d WHERE d.id = $1`, w.deal.UUID) == "true"
}

// healthEvidence is the activity deal health cites for its recency factor.
func (w dealEvidenceWorld) healthEvidence(t *testing.T) *ids.UUID {
	t.Helper()
	health, err := w.e.Deals.DealHealth(w.e.Admin(), w.deal, w.now)
	if err != nil {
		t.Fatal(err)
	}
	return health.Evidence.MostRecentActivityID
}

func TestABusinessMeetingMarkedPrivateStillCountsForTheDeal(t *testing.T) {
	t.Parallel()
	w := newDealEvidenceWorld(t)
	at := w.now.AddDate(0, 0, -2)
	review := w.meeting(t, "budget review", at, true)
	w.meeting(t, "proposal walkthrough", w.now.AddDate(0, 0, 5), true)
	if n := w.e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id = $1 AND admission_blocked`, review); n != 1 {
		t.Fatalf("the review records %d private calendar marks, want one", n)
	}

	if clock := w.dealClock(t); clock == nil || !clock.Equal(at) {
		t.Errorf("deal last_activity_at = %v, want the private-marked review at %v", clock, at)
	}
	if !w.meetingAhead(t) {
		t.Error("a private-marked walkthrough ahead does not defer the quiet deal")
	}
	if cited := w.healthEvidence(t); cited == nil || *cited != review {
		t.Errorf("deal health cites %v, want the private-marked review %s", cited, review)
	}
}

// A personal verdict on the thread takes its mail off the deal clock. The same
// mail judged legal is confidential business and still counts.
func TestAThreadHeldAsPersonalKeepsNoDealWarm(t *testing.T) {
	t.Parallel()
	for kind, counts := range map[string]bool{threadverdict.KindPersonal: false, "legal": true} {
		t.Run(kind, func(t *testing.T) {
			w := newDealEvidenceWorld(t)
			thread, at := "thread-"+ids.NewV7().String(), w.now.AddDate(0, 0, -2)
			dinner := w.mail(t, thread, at)
			w.judgeThread(t, thread, kind)
			if got := w.e.WsScalar(t, `SELECT coalesce(audience_reason, '') FROM activity WHERE id = $1`, dinner); got != threadverdict.ReasonForKind(kind) {
				t.Fatalf("the judged mail carries reason %q, want %q", got, threadverdict.ReasonForKind(kind))
			}
			if clock := w.dealClock(t); (clock != nil) != counts {
				t.Errorf("deal last_activity_at = %v after a %s verdict, want counted=%v", clock, kind, counts)
			}
		})
	}
}

// Each half of the check stands alone. One case keeps the personal reason after
// the seat's import is gone. The other keeps a hold the reason does not name.
func TestEitherSignOfAPersonalThreadKeepsTheDealClockStill(t *testing.T) {
	t.Parallel()
	for name, strip := range map[string]string{
		"the reason without the hold": `DELETE FROM capture_import WHERE activity_id = $1`,
		"the hold without the reason": `UPDATE activity SET audience_reason = 'posture' WHERE id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			w := newDealEvidenceWorld(t)
			thread := "thread-" + ids.NewV7().String()
			dinner := w.mail(t, thread, w.now.AddDate(0, 0, -2))
			w.judgeThread(t, thread, threadverdict.KindPersonal)
			w.e.WsExec(t, strip, dinner)
			w.e.WsExec(t, `SELECT move_last_activity('deal'::regclass, $1)`, w.deal.UUID)
			if clock := w.dealClock(t); clock != nil {
				t.Errorf("deal last_activity_at = %v with %s, want none", clock, name)
			}
		})
	}
}

// Ending both signs of a personal thread gives the deal its date back, in
// either order. The second change alone moves the stored clock, so each order
// proves one trigger: the verdict's, then the activity's.
func TestEndingAPersonalHoldMovesTheDealClockBack(t *testing.T) {
	t.Parallel()
	clearReason := `UPDATE activity SET audience_reason = 'posture' WHERE id = $1`
	deleteHold := `DELETE FROM capture_thread_verdict WHERE thread_key = $1`
	for name, last := range map[string]string{"the verdict last": deleteHold, "the reason last": clearReason} {
		t.Run(name, func(t *testing.T) {
			w := newDealEvidenceWorld(t)
			thread, at := "thread-"+ids.NewV7().String(), w.now.AddDate(0, 0, -2)
			dinner := w.mail(t, thread, at)
			w.judgeThread(t, thread, threadverdict.KindPersonal)
			if last == deleteHold {
				w.e.WsExec(t, clearReason, dinner)
			} else {
				w.e.WsExec(t, deleteHold, thread)
			}
			if clock := w.dealClock(t); clock != nil {
				t.Fatalf("deal last_activity_at = %v while one sign stands, want none", clock)
			}
			if last == deleteHold {
				w.e.WsExec(t, deleteHold, thread)
			} else {
				w.e.WsExec(t, clearReason, dinner)
			}
			if clock := w.dealClock(t); clock == nil || !clock.Equal(at) {
				t.Errorf("deal last_activity_at = %v with %s, want the mail at %v", clock, name, at)
			}
		})
	}
}

// The deal verdict reads the dates of the deal's participants-only mail. A
// thread held as personal gives it none; one held as legal still does.
func TestTheDealVerdictReadsNoPersonalMailDate(t *testing.T) {
	t.Parallel()
	for kind, counts := range map[string]bool{threadverdict.KindPersonal: false, "legal": true} {
		t.Run(kind, func(t *testing.T) {
			w := newDealEvidenceWorld(t)
			thread := "thread-" + ids.NewV7().String()
			w.mail(t, thread, w.now.AddDate(0, 0, -2))
			w.judgeThread(t, thread, kind)
			var in dealverdict.Input
			var newest time.Time
			if err := database.WithWorkspaceTx(w.e.Admin(), w.e.Pool, func(tx pgx.Tx) error {
				return dealprivatemail.Read(context.Background(), tx, w.deal.UUID, 20, &in, &newest)
			}); err != nil {
				t.Fatal(err)
			}
			if got := len(in.PrivateMail) == 1; got != counts {
				t.Errorf("the verdict reads %d private mail dates after a %s verdict, want counted=%v", len(in.PrivateMail), kind, counts)
			}
		})
	}
}

// A deal clock stored before the rule still names the personal mail. The data
// task refolds it, and a second pass changes nothing.
func TestTheDataTaskTakesAPersonalMailOffAStoredDealClock(t *testing.T) {
	t.Parallel()
	w := newDealEvidenceWorld(t)
	thread, at := "thread-"+ids.NewV7().String(), w.now.AddDate(0, 0, -2)
	w.mail(t, thread, at)
	w.judgeThread(t, thread, threadverdict.KindPersonal)
	w.e.WsExec(t, `UPDATE deal SET last_activity_at = $2 WHERE id = $1`, w.deal.UUID, at)

	step := dealclockrefold.PersonalMeetingsTask().Steps[0]
	ctx := principal.SystemActing(w.e.Admin(), "system:datatask-test")
	for pass, want := range []int{1, 0} {
		if err := database.WithWorkspaceTx(ctx, w.e.Pool, func(tx pgx.Tx) error {
			out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 200, RunID: ids.NewV7()})
			if err == nil && out.Changed != want {
				t.Errorf("pass %d refolded %d deal clocks, want %d", pass+1, out.Changed, want)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if clock := w.dealClock(t); clock != nil {
		t.Errorf("deal last_activity_at = %v after the refold, want none", clock)
	}
}
