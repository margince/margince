// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package attendeelockit_test proves that a judge of a meeting's attendees waits
// for capture's insert of an attendee on the same meeting, then counts them. A
// row lock cannot hold off an inserted row; the attendee lock does. A writer
// takes that lock before its row locks, so a waiting judge cannot deadlock it.
package attendeelockit_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ownDomains reads the own-domain list through the capture store, as the
// evidence worker's reader does.
type ownDomains struct{ store *capture.OwnDomainStore }

func (o ownDomains) Domains(ctx context.Context, tx pgx.Tx) ([]string, error) {
	return o.store.ColleagueDomainsTx(ctx, tx)
}

func (o ownDomains) MeetingParties(ctx context.Context, tx pgx.Tx) (owndomain.Parties, error) {
	return o.store.MeetingPartiesTx(ctx, tx)
}

type meetingFixture struct {
	*integration.Env
	t       *testing.T
	own     ownDomains
	meeting ids.UUID
	claim   ids.UUID
}

// newMeetingFixture writes a held meeting on a deal with only colleagues on it,
// and a machine event_held claim on it that is not met.
func newMeetingFixture(t *testing.T) *meetingFixture {
	t.Helper()
	e := integration.Setup(t)
	f := &meetingFixture{Env: e, t: t, own: ownDomains{store: capture.NewOwnDomainStore(e.DB())}}
	pipeline, err := e.Deals.CreatePipeline(e.Admin(), deals.CreatePipelineInput{Name: "Sales"})
	if err != nil {
		t.Fatal(err)
	}
	pipelineID := ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))
	stage, err := e.Deals.CreateStage(e.Admin(), deals.CreateStageInput{PipelineID: pipelineID, Name: "Qualified", Semantic: string(deals.SemanticOpen)})
	if err != nil {
		t.Fatal(err)
	}
	stageID := ids.From[ids.StageKind](ids.UUID(stage.Id))
	required := true
	criterion, err := e.Deals.CreateStageExitCriterion(e.Admin(), deals.CreateCriterionInput{
		StageID: stageID, Key: "held", Label: "held", Kind: string(deals.CriterionEventHeld), Required: &required,
	})
	if err != nil {
		t.Fatal(err)
	}
	dealID := ids.From[ids.DealKind](e.SeedDeal(t, "Rollout", pipelineID, stageID, &e.AdminUser))
	held := clocktest.Now(t).Add(-time.Hour)
	f.meeting = ids.MustParse(e.WsScalar(t, `INSERT INTO activity (kind, subject, meeting_status, occurred_at, source, captured_by)
		VALUES ('meeting', 'Review', 'held', $1, 'gcal:seed', 'connector:gcal') RETURNING id::text`, held))
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, user_id, address, address_domain_folded, role)
		VALUES ($1, $2, 'rep@ourco.test', 'ourco.test', 'from'), ($1, NULL, 'colleague@ourco.test', 'ourco.test', 'attendee')`, f.meeting, e.AdminUser)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, f.meeting, dealID.UUID)
	ev, err := e.Deals.RecordStageEvidence(e.Admin(), deals.EvidenceInput{
		DealID: dealID, CriterionID: ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id)), SourceType: deals.SourceActivity,
		SourceID: f.meeting, AuthorSide: deals.AuthorUnknown, Commitment: deals.CommitmentAgreed, ObservedAt: held,
		ExtractedBy: deals.ExtractedByDeterministic,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.claim = ids.UUID(ev.Id)
	return f
}

// whileABuyerJoins holds capture's insert of an outside attendee open, runs
// judge beside it, and commits once judge waits behind the insert.
func (f *meetingFixture) whileABuyerJoins(judge func() error) {
	f.t.Helper()
	inserted, release, written := make(chan int32, 1), make(chan struct{}), make(chan error, 1)
	var released sync.Once
	commit := func() { released.Do(func() { close(release) }) }
	defer commit()
	go func() {
		written <- database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			defer close(inserted)
			buyer := []connector.MessageParticipant{{Email: "ines@customer.test", Role: connector.ParticipantRoleAttendee}}
			if err := capture.StampFurtherParticipants(f.Admin(), tx, ids.From[ids.ActivityKind](f.meeting), "meeting", "", true, buyer); err != nil {
				return err
			}
			var pid int32
			if err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			inserted <- pid
			<-release
			return nil
		})
	}()
	pid, ok := <-inserted
	if !ok {
		f.t.Fatalf("inserting the buyer: %v", <-written)
	}
	judged, returned := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(returned)
		judged <- judge()
	}()
	testdb.AwaitQueuedBehind(f.t, integration.OwnerConn(f.t), pid, 1, returned, "the attendee judge")
	commit()
	if err := <-written; err != nil {
		f.t.Fatalf("committing the buyer: %v", err)
	}
	if err := <-judged; err != nil {
		f.t.Fatalf("judging the meeting: %v", err)
	}
}

func TestTheEvidenceWriterWaitsForAnAttendeeJoiningTheMeeting(t *testing.T) {
	f := newMeetingFixture(t)
	var facts deals.ActivityAuthorship
	f.whileABuyerJoins(func() error {
		return database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			var err error
			facts, err = deals.ReadActivityAuthorship(f.Admin(), tx, f.meeting, f.own)
			return err
		})
	})
	if facts.BuyerParticipants != 1 {
		t.Errorf("the judge counted %d buyers, want the 1 who joined while it waited", facts.BuyerParticipants)
	}
}

func TestTheMeetingProofPassWaitsForAnAttendeeJoiningTheMeeting(t *testing.T) {
	f := newMeetingFixture(t)
	system := principal.SystemActing(f.Admin(), "attendee-lock-test")
	f.whileABuyerJoins(func() error {
		_, err := f.Deals.ReconcileEvidenceAuthorship(system, f.own, deals.EvidenceAuthorshipScope{})
		return err
	})
	if got := f.WsScalar(t, `SELECT met::text FROM deal_stage_evidence WHERE id = $1`, f.claim); got != "true" {
		t.Errorf("the claim is met = %s after the pass, want true for the buyer who joined while it waited", got)
	}
}

// A contact relink moves the meeting's attendee rows. It waits for a judge's
// shared lock before it locks the meeting row, so the judge can still take the
// row lock it reads under. Locked the other way round, the two deadlock.
func TestARelinkWaitsForTheJudgeBeforeItLocksTheMeeting(t *testing.T) {
	f := newMeetingFixture(t)
	contact := f.SeedContact(t, "Ines Buyer", &f.AdminUser)
	ctx := context.Background()
	judge, err := f.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := judge.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	var judgePID int32
	if err := judge.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&judgePID); err != nil {
		t.Fatal(err)
	}
	if err := storekit.LockMeetingAttendeeChecks(ctx, judge, f.meeting); err != nil {
		t.Fatal(err)
	}
	relinked, returned := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(returned)
		_, err := f.Activities.RelinkActivity(f.Admin(), ids.From[ids.ActivityKind](f.meeting),
			activities.RelinkActivityInput{EntityType: "contact", EntityID: contact})
		relinked <- err
	}()
	testdb.AwaitQueuedBehind(t, integration.OwnerConn(t), judgePID, 1, returned, "the contact relink")
	// The row lock the meeting proof pass takes next. NOWAIT asks whether the
	// relink holds the meeting while it waits for the judge.
	if _, err := judge.Exec(ctx, `SELECT 1 FROM activity WHERE id = $1 FOR SHARE NOWAIT`, f.meeting); err != nil {
		t.Fatalf("the relink holds the meeting row while it waits for the judge: %v", err)
	}
	if err := judge.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-relinked; err != nil {
		t.Fatalf("relinking the meeting: %v", err)
	}
}
