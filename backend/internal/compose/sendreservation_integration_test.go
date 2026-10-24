// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Every delayed send reads its payload and reserves it through reserveSend.
// The cases below change what a payload names with its real writer. They do
// it after the first rendering, or after the second, just before the hold.

// changeAfterRendering wraps a reading so change runs once, after the given
// rendering. It counts the renderings made.
func changeAfterRendering[P any](read sendReading[P], after int, change func()) (sendReading[P], *int) {
	renders := new(int)
	render := read.render
	read.render = func(excluded map[ids.UUID]bool) (P, sendRefs, bool, error) {
		payload, refs, wanted, err := render(excluded)
		*renders++
		if *renders == after {
			change()
		}
		return payload, refs, wanted, err
	}
	return read, renders
}

func systemMailCtx(ws ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: notificationMailActor})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

func liveSeatCtx(t *testing.T, e *integration.Env, seat ids.UUID) context.Context {
	t.Helper()
	ctx, live, err := seatContext(systemMailCtx(e.WS), identity.NewService(e.Pool), e.WS, seat)
	if err != nil || !live {
		t.Fatalf("the seat's authority did not resolve: live=%v err=%v", live, err)
	}
	return ctx
}

// A decision whose source moves between renderings is sent with its new
// words only. After the first rendering, the second one disagrees. After the
// second, the hold refuses the versions it was read at. The other decision is
// sent once.
func TestAnApprovalMailChangedWhileItIsReadSendsOnlyTheNewWords(t *testing.T) {
	for name, after := range map[string]int{"after the first rendering": 1, "just before the reservation": 2} {
		t.Run(name, func(t *testing.T) {
			a, _, worker := newDecisionMailEnv(t)
			moved := stagePromisedDecision(t, a, a.e.Rep1, "Send the pricing")
			other := stagePromisedDecision(t, a, a.e.Rep1, "Send the contract")
			var items []approvalMailItem
			for _, notice := range []ids.UUID{moved.notice, other.notice} {
				attempt, claimed, err := worker.notices.ClaimEmailAttempt(systemMailCtx(a.e.WS), notice)
				if err != nil || !claimed {
					t.Fatalf("claiming the notice: claimed=%v err=%v", claimed, err)
				}
				items = append(items, approvalMailItem{attempt: attempt})
			}
			seatCtx := liveSeatCtx(t, a.e, a.e.Rep1)
			words := recipientMailCopy(seatCtx, a.e.Pool)
			before, _, _, err := worker.approvalReading(seatCtx, items, words).render(nil)
			if err != nil {
				t.Fatal(err)
			}
			read, renders := changeAfterRendering(worker.approvalReading(seatCtx, items, words), after, func() {
				day := clocktest.Now(t).AddDate(0, -2, 0)
				if _, err := a.e.Activities.UpdateActivity(a.e.Admin(), ids.From[ids.ActivityKind](moved.source), activities.UpdateActivityInput{OccurredAt: &day}); err != nil {
					t.Error(err)
				}
			})
			sent, won, err := reserveSend(seatCtx, a.e.Pool, read, reserveInOwnTx[[]approvalMailItem](seatCtx, a.e.Pool))
			if err != nil || !won {
				t.Fatalf("the send was not reserved: won=%v err=%v", won, err)
			}
			fresh, _, _, err := worker.approvalReading(seatCtx, items, words).render(nil)
			if err != nil {
				t.Fatal(err)
			}
			_, oldMessage := approvalMailMessage(before, noticeMailOrigin, words)
			_, sentMessage := approvalMailMessage(sent, noticeMailOrigin, words)
			_, freshMessage := approvalMailMessage(fresh, noticeMailOrigin, words)
			if oldMessage.Text == freshMessage.Text {
				t.Fatal("the change did not alter the words, so the test proves nothing")
			}
			if sentMessage.Text != freshMessage.Text {
				t.Fatalf("the words sent are not the ones read at the versions held:\n%s", sentMessage.Text)
			}
			if len(sent) != 2 || strings.Count(sentMessage.Text, other.summary) != strings.Count(freshMessage.Text, other.summary) {
				t.Fatalf("the other decision was not sent once beside the changed one: %d items", len(sent))
			}
			if after == 2 && *renders < 4 {
				t.Fatalf("the reservation took a basis read before the change (%d renderings)", *renders)
			}
		})
	}
}

// setupDecisionDigest is a digest seat that decides deal corrections, with
// those decisions batched into its morning.
func setupDecisionDigest(t *testing.T) (*digestEnv, approvalNotifyEnv) {
	t.Helper()
	d := setupDigest(t)
	db := InstallationDB(d.Pool)
	a := approvalNotifyEnv{e: d.Env, db: db, svc: approvals.NewService(db), queue: &recordingNoticeMailQueue{}}
	a.notify = NewApprovalNotify(d.Pool, db, a.queue)
	a.grantRole(t, d.Rep1)
	d.batch(t, notices.ClassApprovalPending)
	return d, a
}

// digestDecision stages a close-date correction on a deal of its own, named
// by summary, with its notice raised before the morning.
func digestDecision(t *testing.T, d *digestEnv, a approvalNotifyEnv, summary string) (ids.ApprovalID, ids.UUID) {
	t.Helper()
	id, deal, _ := stageDealDecision(t, a, summary, d.Rep1)
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `UPDATE notice SET created_at = $2 WHERE recipient_user_id = $1`, d.Rep1, digestRecordedAt); err != nil {
		t.Fatal(err)
	}
	return id, deal
}

// stageDealDecision stages a close-date correction on a new deal the owner
// decides, and returns the decision, its deal and the notice it raised.
func stageDealDecision(t *testing.T, a approvalNotifyEnv, summary string, owner ids.UUID) (ids.ApprovalID, ids.UUID, ids.UUID) {
	t.Helper()
	a.deal = dealBeside(t, a, summary, owner)
	proposal, err := json.Marshal(deals.CloseDateCorrection{DealID: ids.From[ids.DealKind](a.deal), ExpectedCloseDate: "2026-12-01", Basis: "the date has passed"})
	if err != nil {
		t.Fatal(err)
	}
	id, env := a.stage(a.proposerCtx(ids.Nil), t, approvals.StageInput{
		Kind: deals.CloseDateCorrectionKind, ProposedChange: proposal, DiffHash: "h-" + a.deal.String(),
		TargetType: string(recordTypeDeal), TargetID: a.deal, Summary: summary,
	})
	a.deliver(t, env)
	for _, n := range a.delivered(t) {
		if n.subject == summary {
			return id, a.deal, n.id
		}
	}
	t.Fatalf("no notice was written for %q", summary)
	return id, a.deal, ids.Nil
}

// dealBeside is a new deal the digest seat owns, so its corrections are the
// seat's to decide. It sits in the pipeline of the env's seeded deal.
func dealBeside(t *testing.T, a approvalNotifyEnv, name string, owner ids.UUID) ids.UUID {
	t.Helper()
	var pipeline, stage ids.UUID
	err := integration.OwnerConn(t).QueryRow(context.Background(), `SELECT pipeline_id, stage_id FROM deal WHERE name = 'Fleet renewal' LIMIT 1`).Scan(&pipeline, &stage)
	if errors.Is(err, pgx.ErrNoRows) {
		a.seedDeal(t)
		err = integration.OwnerConn(t).QueryRow(context.Background(), `SELECT pipeline_id, stage_id FROM deal WHERE name = 'Fleet renewal' LIMIT 1`).Scan(&pipeline, &stage)
	}
	if err != nil {
		t.Fatal(err)
	}
	holder := ids.From[ids.UserKind](owner)
	deal, err := a.e.Deals.CreateDeal(a.e.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: ids.From[ids.PipelineKind](pipeline), StageID: ids.From[ids.StageKind](stage), OwnerID: &holder, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(deal.Id)
}

func digestWindow(seatCtx context.Context, t *testing.T, d *digestEnv) notices.Window {
	t.Helper()
	var window notices.Window
	if err := database.WithWorkspaceTx(seatCtx, d.Pool, func(tx pgx.Tx) error {
		var err error
		window, err = notificationWindow(seatCtx, tx, digestMorning)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return window
}

func quotes(lines digestLines, summary string) int {
	n := 0
	for _, line := range lines.quotable {
		if strings.Contains(line.text, summary) {
			n++
		}
	}
	return n
}

func withdraw(t *testing.T, a approvalNotifyEnv, id ids.ApprovalID) {
	t.Helper()
	ctx := principal.SystemActing(a.e.Admin(), approvalNoticeRetractActor)
	if err := a.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := a.svc.WithdrawInTx(ctx, tx, id, "The correction is no longer needed")
		return err
	}); err != nil {
		t.Error(err)
	}
}

// A digest decision withdrawn, or whose deal was archived, between the
// digest's reading and its reservation is left out. The
// reservation refuses the basis and the digest is read again; the other
// decision is still quoted once.
func TestADigestDecisionChangedBeforeItsReservationIsLeftOut(t *testing.T) {
	changes := map[string]func(t *testing.T, a approvalNotifyEnv, id ids.ApprovalID, deal ids.UUID){
		"it was withdrawn": func(t *testing.T, a approvalNotifyEnv, id ids.ApprovalID, _ ids.UUID) { withdraw(t, a, id) },
		"its deal was archived": func(t *testing.T, a approvalNotifyEnv, _ ids.ApprovalID, deal ids.UUID) {
			if _, err := a.e.Deals.ArchiveDeal(a.e.Admin(), ids.From[ids.DealKind](deal), nil); err != nil {
				t.Error(err)
			}
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			d, a := setupDecisionDigest(t)
			gone, deal := digestDecision(t, d, a, "Fleet renewal close date has passed")
			digestDecision(t, d, a, "Depot renewal close date has passed")
			seatCtx := liveSeatCtx(t, d.Env, d.Rep1)
			seat := ids.From[ids.UserKind](d.Rep1)
			read, renders := changeAfterRendering(d.worker.digestReading(seatCtx, seat, digestWindow(seatCtx, t, d)), 2, func() { change(t, a, gone, deal) })
			first, _, _, err := read.render(nil)
			if err != nil || len(first.quotable) != 2 {
				t.Fatalf("the digest did not quote both decisions before the change: %+v err=%v", first.quotable, err)
			}
			*renders = 0
			lines, won, err := reserveSend(seatCtx, d.Pool, read, reserveInOwnTx[digestLines](seatCtx, d.Pool))
			if err != nil || !won {
				t.Fatalf("the digest was not reserved: won=%v err=%v", won, err)
			}
			if *renders < 4 {
				t.Fatalf("the reservation took the basis read before the change (%d renderings)", *renders)
			}
			if lines.total != 1 || len(lines.quotable) != 1 || quotes(lines, "Depot") != 1 {
				t.Fatalf("the digest quotes %+v", lines.quotable)
			}
		})
	}
}

// The real digest jobs, alone and combined with the morning, send one
// message. A decision withdrawn before the send is left out of it, and the
// one still waiting is quoted once.
func TestADigestWithAValidDecisionIsSentOnceAloneAndCombined(t *testing.T) {
	for _, combined := range []bool{false, true} {
		t.Run(map[bool]string{false: "alone", true: "combined"}[combined], func(t *testing.T) {
			d, a := setupDecisionDigest(t)
			if combined {
				a.e.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES ('digest_routine', 'Routine reader',
					'{"objects":{"deal":{"read":true},"installation_settings":{"read":true}},"row_scope":"all"}'::jsonb)`)
				a.e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM role WHERE key = 'digest_routine'`, d.Rep1)
				yes, none := true, identity.DeliveryNone
				if _, err := d.worker.users.SaveMyDelivery(d.seatCtx(d.Rep1), identity.Delivery{CombineRoutine: &yes, MorningBrief: &none}); err != nil {
					t.Fatal(err)
				}
			}
			gone, _ := digestDecision(t, d, a, "Fleet renewal close date has passed")
			digestDecision(t, d, a, "Depot renewal close date has passed")
			withdraw(t, a, gone)
			d.run(t)
			d.run(t)
			if d.relay.count() != 1 {
				t.Fatalf("the digest was sent %d times, want once", d.relay.count())
			}
			body := d.relay.bodies[0]
			if strings.Contains(body, "Fleet renewal close date") || !strings.Contains(body, "Depot renewal close date") ||
				strings.Count(body, "/#/home?approval=") != 1 {
				t.Fatalf("the digest body is wrong:\n%s", body)
			}
		})
	}
}

// A claimless decision rests on the recorded admission of its promise. When
// that admission records the promise as kept, the basis no longer carries the
// decision and a reservation over the old basis is refused.
func TestAClaimlessDecisionWhoseAdmissionChangedIsNotReserved(t *testing.T) {
	a, _, _ := newDecisionMailEnv(t)
	note, _, err := a.e.Activities.LogActivity(a.e.Admin(), activities.LogActivityInput{Kind: "note", Body: new("We will send the pricing."), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	source := ids.UUID(note.Id)
	decision := a.stageCommitmentFor(t, a.e.Rep1, source, "pricing")
	reader := a.e.As(a.e.Rep1, nil, integration.AdminPerms)
	refs := sendRefs{approvals: []ids.UUID{decision.UUID}}
	basis := readBasis(reader, t, a.e, a.e.Rep1, refs, clocktest.Now(t))
	if len(basis.excluded) != 0 || !hasRow(basis, commitmentAdmissionTable) {
		t.Fatalf("the basis does not rest on the admission: %+v", basis)
	}
	if err := holdBasis(reader, a.e, basis, clocktest.Now(t)); err != nil {
		t.Fatalf("an unchanged admission refused the reservation: %v", err)
	}
	kept := crmcontracts.CommitmentAdmission{
		Locator: "pricing", Summary: "Send the pricing", SourceQuote: "We will send the pricing.",
		Outcome: string(CommitmentProposed), CurrentWork: true, RuleVersion: activities.CommitmentAdmissionVersion,
		EvidenceIds: []crmcontracts.Id{crmcontracts.Id(source)}, EvaluatedAt: clocktest.Now(t),
		Meaning: &crmcontracts.CommitmentMeaning{State: "done", DeadlinePrecision: "none"},
	}
	if err := a.e.DB().Tx(a.e.Admin(), func(tx pgx.Tx) error {
		return activities.RecordCommitmentAdmissionTx(a.e.Admin(), tx, ids.From[ids.ActivityKind](source), kept)
	}); err != nil {
		t.Fatal(err)
	}
	if err := holdBasis(reader, a.e, basis, clocktest.Now(t)); !errors.Is(err, errSendBasisChanged) {
		t.Fatalf("a reservation over a changed admission returned %v", err)
	}
	if again := readBasis(reader, t, a.e, a.e.Rep1, refs, clocktest.Now(t)); !again.excluded[decision.UUID] {
		t.Fatal("a decision whose promise was recorded as kept is still in the basis")
	}
}

// The reservation checks expiry against the clock it reads after its locks.
// A decision that expired while the send waited is refused; at a time before
// its expiry the same basis holds.
func TestADecisionThatExpiredWhileTheSendWaitedIsRefused(t *testing.T) {
	a, _, _ := newDecisionMailEnv(t)
	decision := stagePromisedDecision(t, a, a.e.Rep1, "Send the pricing")
	var expires time.Time
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `SELECT expires_at FROM approval WHERE id = $1`, decision.approval).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	reader := a.e.As(a.e.Rep1, nil, integration.AdminPerms)
	basis := readBasis(reader, t, a.e, a.e.Rep1, sendRefs{approvals: []ids.UUID{decision.approval}}, expires.Add(-time.Hour))
	if err := holdBasis(reader, a.e, basis, expires.Add(-time.Minute)); err != nil {
		t.Fatalf("a decision not yet expired refused the reservation: %v", err)
	}
	if err := holdBasis(reader, a.e, basis, expires.Add(time.Minute)); !errors.Is(err, errSendBasisChanged) {
		t.Fatalf("a decision that expired while the send waited returned %v", err)
	}
}

// A reservation holds the recipient's team memberships, so taking the
// recipient off a team waits for the reservation to commit.
func TestTakingTheRecipientOffATeamWaitsForTheReservation(t *testing.T) {
	a, _, _ := newDecisionMailEnv(t)
	decision := stagePromisedDecision(t, a, a.e.Rep1, "Send the pricing")
	reader := a.e.As(a.e.Rep1, []ids.UUID{a.e.Team1}, integration.AdminPerms)
	basis := readBasis(reader, t, a.e, a.e.Rep1, sendRefs{approvals: []ids.UUID{decision.approval}}, clocktest.Now(t))
	held, release := make(chan struct{}), make(chan struct{})
	reserved, ended := make(chan error, 1), make(chan struct{})
	var holder int32
	go func() {
		defer close(ended)
		reserved <- database.WithWorkspaceTx(reader, a.e.Pool, func(tx pgx.Tx) error {
			if err := holdSendBasisTx(reader, tx, basis, func() time.Time { return clocktest.Now(t) }); err != nil {
				return err
			}
			if err := tx.QueryRow(reader, `SELECT pg_backend_pid()`).Scan(&holder); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-reserved:
		t.Fatalf("the reservation ended before it held the memberships: %v", err)
	}
	removed, returned := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(returned)
		admin := identity.Identity{UserID: ids.UserID{UUID: a.e.AdminUser}, WorkspaceID: ids.WorkspaceID{UUID: a.e.WS}, Permissions: integration.AdminPerms, Roles: []string{"admin"}, SeatType: "full"}
		removed <- identity.NewServiceFor(a.e.DB()).SetTeamMember(a.e.Admin(), admin, a.e.Team1, a.e.Rep1, false)
	}()
	var lift sync.Once
	t.Cleanup(func() {
		lift.Do(func() { close(release) })
		<-ended
		<-returned
	})
	testdb.AwaitQueuedBehind(t, integration.OwnerConn(t), holder, 1, returned, "the membership change")
	lift.Do(func() { close(release) })
	if err := <-reserved; err != nil {
		t.Fatal(err)
	}
	if err := <-removed; err != nil {
		t.Fatal(err)
	}
}

// A recipient who reads one decision's source only through a team is taken off
// that team between the first rendering and the reservation. The context the
// send started with still names the team. The reservation reads the recipient's
// authority again, so that decision's words are withheld and the decision the
// recipient reads directly is sent once.
func TestARecipientTakenOffTheSourcesTeamBeforeTheReservationGetsOnlyTheDirectDecision(t *testing.T) {
	a, _, worker := newDecisionMailEnv(t)
	teamOnly := stagePromisedDecision(t, a, a.e.Rep1, "Send the pricing")
	direct := stagePromisedDecision(t, a, a.e.Rep1, "Send the contract")
	if _, err := a.e.Activities.SetAudience(a.e.Admin(), ids.From[ids.ActivityKind](teamOnly.source), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "team", SubjectID: a.e.Team1}},
	}); err != nil {
		t.Fatal(err)
	}
	items := claimedItems(t, worker, a, teamOnly.notice, direct.notice)
	seatCtx := liveSeatCtx(t, a.e, a.e.Rep1)
	words := recipientMailCopy(seatCtx, a.e.Pool)
	first, _, _, err := worker.approvalReading(seatCtx, items, words).render(nil)
	if err != nil || len(first) != 2 {
		t.Fatalf("the team member did not read both decisions, so the test proves nothing: %d err=%v", len(first), err)
	}
	read, _ := changeAfterRendering(worker.approvalReading(seatCtx, items, words), 1, func() {
		admin := identity.Identity{UserID: ids.UserID{UUID: a.e.AdminUser}, WorkspaceID: ids.WorkspaceID{UUID: a.e.WS}, Permissions: integration.AdminPerms, Roles: []string{"admin"}, SeatType: "full"}
		if err := identity.NewServiceFor(a.e.DB()).SetTeamMember(a.e.Admin(), admin, a.e.Team1, a.e.Rep1, false); err != nil {
			t.Error(err)
		}
	})
	sent, won, err := reserveSend(seatCtx, a.e.Pool, read, reserveInOwnTx[[]approvalMailItem](seatCtx, a.e.Pool))
	if err != nil || !won {
		t.Fatalf("the send was not reserved: won=%v err=%v", won, err)
	}
	_, message := approvalMailMessage(sent, noticeMailOrigin, words)
	if len(sent) != 1 || strings.Contains(message.Text, teamOnly.summary) || !strings.Contains(message.Text, direct.summary) {
		t.Fatalf("after the team removal the message carried %d decision(s):\n%s", len(sent), message.Text)
	}
}

// claimedItems claims the notices as the mail worker does, in order.
func claimedItems(t *testing.T, worker *notificationMailWorker, a approvalNotifyEnv, notices ...ids.UUID) []approvalMailItem {
	t.Helper()
	var items []approvalMailItem
	for _, notice := range notices {
		attempt, claimed, err := worker.notices.ClaimEmailAttempt(systemMailCtx(a.e.WS), notice)
		if err != nil || !claimed {
			t.Fatalf("claiming the notice: claimed=%v err=%v", claimed, err)
		}
		items = append(items, approvalMailItem{attempt: attempt})
	}
	return items
}

// A digest line that reports a past notice about a contact is history, but its
// reader must still be allowed to see that contact. Handed to a colleague after
// the second rendering, the line is left out at the reservation. A readable
// line beside it is still quoted once.
func TestAHistoricDigestLineWhoseContactWasHandedOnIsLeftOut(t *testing.T) {
	d := setupDigest(t)
	handed, kept := d.capturedContact(t, "Pricing contact", d.Rep1), d.capturedContact(t, "Contract contact", d.Rep1)
	d.raise(t, "Pricing contact replied", contactRef(handed))
	d.raise(t, "Contract contact replied", contactRef(kept))
	seatCtx := liveSeatCtx(t, d.Env, d.Rep1)
	seat := ids.From[ids.UserKind](d.Rep1)
	reading := d.worker.digestReading(seatCtx, seat, digestWindow(seatCtx, t, d))
	first, _, _, err := reading.render(nil)
	if err != nil || quotes(first, "Pricing contact") != 1 || quotes(first, "Contract contact") != 1 {
		t.Fatalf("the digest did not quote both lines before the change: %+v err=%v", first.quotable, err)
	}
	read, renders := changeAfterRendering(reading, 2, func() { d.reassign(t, handed, d.Rep2) })
	lines, won, err := reserveSend(seatCtx, d.Pool, read, reserveInOwnTx[digestLines](seatCtx, d.Pool))
	if err != nil || !won {
		t.Fatalf("the digest was not reserved: won=%v err=%v", won, err)
	}
	if *renders < 4 {
		t.Fatalf("the reservation took the basis read before the change (%d renderings)", *renders)
	}
	if quotes(lines, "Pricing contact") != 0 || quotes(lines, "Contract contact") != 1 {
		t.Fatalf("the digest quotes %+v", lines.quotable)
	}
}

// A combined morning carries its digest's lines in the same basis as its body.
// A historic line whose contact is handed to a colleague before the hold is
// refused there. With the contact unchanged, the same basis holds.
func TestACombinedMorningHoldsItsHistoricDigestLines(t *testing.T) {
	d := setupDigest(t)
	handed := d.capturedContact(t, "Pricing contact", d.Rep1)
	d.raise(t, "Pricing contact replied", contactRef(handed))
	seatCtx := liveSeatCtx(t, d.Env, d.Rep1)
	lines, _, _, err := d.worker.digestReading(seatCtx, ids.From[ids.UserKind](d.Rep1), digestWindow(seatCtx, t, d)).render(nil)
	if err != nil || quotes(lines, "Pricing contact") != 1 {
		t.Fatalf("the digest did not quote the line: %+v err=%v", lines.quotable, err)
	}
	refs := morningRefs(nil, lines)
	if len(refs.records) != 1 {
		t.Fatalf("the morning's refs dropped the digest's record: %+v", refs)
	}
	basis := readBasis(seatCtx, t, d.Env, d.Rep1, refs, digestMorning)
	if err := holdBasis(seatCtx, d.Env, basis, digestMorning); err != nil {
		t.Fatalf("an unchanged historic line refused the reservation: %v", err)
	}
	d.reassign(t, handed, d.Rep2)
	if err := holdBasis(seatCtx, d.Env, basis, digestMorning); !errors.Is(err, errSendBasisChanged) {
		t.Fatalf("a historic line whose contact was handed on returned %v", err)
	}
}

// An activity a line reports on is held through the recipient's content scope.
// Narrowed to someone else before the hold, it refuses the reservation; left
// readable, the same basis holds.
func TestAReportedActivityNarrowedBeforeTheHoldIsRefused(t *testing.T) {
	a, _, _ := newDecisionMailEnv(t)
	body := "Notes from the pricing call."
	note, _, err := a.e.Activities.LogActivity(a.e.Admin(), activities.LogActivityInput{Kind: "note", Body: &body, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	reported := ids.UUID(note.Id)
	reader := liveSeatCtx(t, a.e, a.e.Rep1)
	refs := sendRefs{records: []notices.Target{{Type: string(recordTypeActivity), ID: reported}}}
	basis := readBasis(reader, t, a.e, a.e.Rep1, refs, clocktest.Now(t))
	if basis.excluded[reported] || !hasRow(basis, string(recordTypeActivity)) {
		t.Fatalf("the reported activity is not held in the basis: %+v", basis)
	}
	if err := holdBasis(reader, a.e, basis, clocktest.Now(t)); err != nil {
		t.Fatalf("a readable reported activity refused the reservation: %v", err)
	}
	if _, err := a.e.Activities.SetAudience(a.e.Admin(), ids.From[ids.ActivityKind](reported), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: a.e.AdminUser}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := holdBasis(reader, a.e, basis, clocktest.Now(t)); !errors.Is(err, errSendBasisChanged) {
		t.Fatalf("a reported activity narrowed to someone else returned %v", err)
	}
}

// A recipient whose role loses activity read may no longer be mailed what a
// source says. This holds even when the row and its audience still reach them.
// Lost after the first or second rendering, the commitment decision and a
// reported activity are left out. The deal decision is still sent once. With
// the role restored before the reservation, both decisions are sent.
func TestARecipientWhoLostActivityReadGetsOnlyTheDealDecision(t *testing.T) {
	for name, after := range map[string]int{"after the first rendering": 1, "after the second rendering": 2, "restored after the first rendering": 0} {
		t.Run(name, func(t *testing.T) {
			a, _, worker := newDecisionMailEnv(t)
			a.grantRole(t, a.e.Rep1)
			promise := stagePromisedDecision(t, a, a.e.Rep1, "Send the pricing")
			_, _, dealNotice := stageDealDecision(t, a, "Depot renewal close date has passed", a.e.Rep1)
			role := "commitmentdecider-" + a.e.Rep1.String()
			revoke := func() {
				a.e.WsExec(t, `UPDATE role SET permissions = '{"objects":{"contact":{"read":true,"update":true}},"row_scope":"all"}'::jsonb WHERE key = $1`, role)
			}
			restore := func() {
				a.e.WsExec(t, `UPDATE role SET permissions = $2::jsonb WHERE key = $1`, role, commitmentDeciderGrants)
			}
			items := claimedItems(t, worker, a, promise.notice, dealNotice)
			seatCtx := liveSeatCtx(t, a.e, a.e.Rep1)
			words := recipientMailCopy(seatCtx, a.e.Pool)
			read := worker.approvalReading(seatCtx, items, words)
			if after > 0 {
				read, _ = changeAfterRendering(read, after, revoke)
			} else {
				revoke()
				read, _ = changeAfterRendering(read, 1, restore)
			}
			sent, won, err := reserveSend(seatCtx, a.e.Pool, read, reserveInOwnTx[[]approvalMailItem](seatCtx, a.e.Pool))
			if err != nil || !won {
				t.Fatalf("the send was not reserved: won=%v err=%v", won, err)
			}
			_, message := approvalMailMessage(sent, noticeMailOrigin, words)
			want := 2
			if after > 0 {
				want = 1
			}
			if len(sent) != want || !strings.Contains(message.Text, "Depot renewal close date") ||
				(after > 0) == strings.Contains(message.Text, "We will send the pricing.") {
				t.Fatalf("the message carried %d decision(s), want %d:\n%s", len(sent), want, message.Text)
			}
			if after == 0 {
				return
			}
			reported := sendRefs{records: []notices.Target{{Type: string(recordTypeActivity), ID: promise.source}}}
			if basis := readBasis(seatCtx, t, a.e, a.e.Rep1, reported, clocktest.Now(t)); !basis.excluded[promise.source] {
				t.Fatal("a reported activity stayed in the basis of a recipient without activity read")
			}
		})
	}
}

// A decision quoting a mail rests on that mail being read in full. A mail found
// read only in part refuses the reservation even where its version matches.
func TestADecisionWhoseMailIsNoLongerReadInFullIsNotReserved(t *testing.T) {
	a, _, _ := newDecisionMailEnv(t)
	decision := stagePromisedDecision(t, a, a.e.Rep1, "Send the pricing")
	a.e.WsExec(t, `UPDATE activity SET kind = 'email', direction = 'inbound', subject = 'Pricing' WHERE id = $1`, decision.source)
	reader := a.e.As(a.e.Rep1, nil, integration.AdminPerms)
	refs := sendRefs{approvals: []ids.UUID{decision.approval}}
	basis := readBasis(reader, t, a.e, a.e.Rep1, refs, clocktest.Now(t))
	if basis.excluded[decision.approval] {
		t.Fatal("a mail read in full did not qualify, so the test proves nothing")
	}
	if err := holdBasis(reader, a.e, basis, clocktest.Now(t)); err != nil {
		t.Fatalf("a mail read in full refused the reservation: %v", err)
	}
	// No writer marks a complete mail partial, and every update moves the
	// version. So the basis is aligned with the version the update left: the
	// hold then sees an unchanged row, and only the completeness test refuses.
	owner := integration.OwnerConn(t)
	var moved int64
	if err := owner.QueryRow(context.Background(), `UPDATE activity SET body_completeness = 'partial' WHERE id = $1 RETURNING version`, decision.source).Scan(&moved); err != nil {
		t.Fatal(err)
	}
	for i := range basis.rows {
		if basis.rows[i].table == string(recordTypeActivity) && basis.rows[i].id == decision.source {
			basis.rows[i].version = moved
		}
	}
	if err := holdBasis(reader, a.e, basis, clocktest.Now(t)); !errors.Is(err, errSendBasisChanged) {
		t.Fatalf("a mail read only in part returned %v", err)
	}
}

func readBasis(reader context.Context, t *testing.T, e *integration.Env, seat ids.UUID, refs sendRefs, now time.Time) sendBasis {
	t.Helper()
	var basis sendBasis
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		var err error
		basis, err = readSendBasis(reader, tx, seat, nil, refs, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return basis
}

func holdBasis(reader context.Context, e *integration.Env, basis sendBasis, at time.Time) error {
	return database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		return holdSendBasisTx(reader, tx, basis, func() time.Time { return at })
	})
}

func hasRow(basis sendBasis, table string) bool {
	for _, row := range basis.rows {
		if row.table == table {
			return true
		}
	}
	return false
}
