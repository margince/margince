// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A judge of a meeting's attendees waits for an attendee insert on the same
// meeting to commit, and then counts the new attendee. A row lock cannot hold
// off an inserted row, so without the attendee lock the judge reads the old set.

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// whileABuyerJoins holds capture's insert of an outside attendee open, runs
// judge beside it, and commits once judge waits behind the insert.
func (f *authorshipFixture) whileABuyerJoins(meeting ids.UUID, judge func() error) {
	f.t.Helper()
	inserted, release, written := make(chan int32, 1), make(chan struct{}), make(chan error, 1)
	var released sync.Once
	commit := func() { released.Do(func() { close(release) }) }
	defer commit()
	go func() {
		written <- database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			buyer := []connector.MessageParticipant{{Email: "ines@customer.test", Role: connector.ParticipantRoleAttendee}}
			if err := capture.StampFurtherParticipants(f.Admin(), tx, ids.From[ids.ActivityKind](meeting), "meeting", "", true, buyer); err != nil {
				close(inserted)
				return err
			}
			var pid int32
			if err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				close(inserted)
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
	f := newAuthorshipFixture(t)
	meeting := f.heldMeeting("rep@ourco.test", "colleague@ourco.test")
	var facts deals.ActivityAuthorship
	f.whileABuyerJoins(meeting, func() error {
		return database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			var err error
			facts, err = deals.ReadActivityAuthorship(f.Admin(), tx, meeting, ownDomainReader{store: f.own})
			return err
		})
	})
	if facts.BuyerParticipants != 1 {
		t.Errorf("the judge counted %d buyers, want the 1 who joined while it waited", facts.BuyerParticipants)
	}
}

func TestTheMeetingProofPassWaitsForAnAttendeeJoiningTheMeeting(t *testing.T) {
	f := newAuthorshipFixture(t)
	f.heldCriterion()
	meeting := f.heldMeeting("rep@ourco.test", "colleague@ourco.test")
	claim := f.heldClaim(meeting, deals.ExtractedByDeterministic)
	f.WsExec(t, `UPDATE deal_stage_evidence SET met = false WHERE id = $1`, claim)
	worker := newEvidenceAuthorshipWorker(f.Pool, slog.New(slog.DiscardHandler))
	f.whileABuyerJoins(meeting, func() error {
		return worker.Work(f.Admin(), &river.Job[EvidenceAuthorshipArgs]{})
	})
	if got := f.met(claim); got != "true" {
		t.Errorf("the claim is met = %s after the pass, want true for the buyer who joined while it waited", got)
	}
}
