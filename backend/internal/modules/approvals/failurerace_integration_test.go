// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// Two attempts at one approval's work, overlapping. Only the attempt that ran
// the work may say how it went, so the loser never leaves a failure behind.

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scriptedEffect runs each call's own step, in call order, so a test can hold
// one attempt inside its effect while another runs.
type scriptedEffect struct {
	mu    sync.Mutex
	steps []ApprovedEffect
	calls int
}

func (s *scriptedEffect) run(ctx context.Context, id ids.ApprovalID, change json.RawMessage, diffHash string) error {
	s.mu.Lock()
	step := s.steps[s.calls]
	s.calls++
	s.mu.Unlock()
	return step(ctx, id, change, diffHash)
}

// decideAsync approves the row on its own goroutine, and the channel carries
// the answer.
func (e *stagingEnv) decideAsync(ctx context.Context, id ids.ApprovalID) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := e.svc.Decide(ctx, id, true, nil)
		done <- err
	}()
	return done
}

// The second retry finds the approval already redeemed by the first, which is
// still running. Its refusal says nothing about the work, so the mark stays.
func TestARetryThatLosesTheRedemptionLeavesNoFailure(t *testing.T) {
	e := setupStaging(t)
	deciding := e.asHumanWith(decidesEverything())
	id := e.failedDecision(deciding, t, (&flakyEffect{e: e, failing: true}).run)
	mark := e.markOf(t, id)

	aIn, aRedeemed, aGo1, aGo2, bIn, bGo := make(chan struct{}), make(chan struct{}),
		make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	script := &scriptedEffect{steps: []ApprovedEffect{
		func(ctx context.Context, id ids.ApprovalID, _ json.RawMessage, hash string) error {
			close(aIn)
			<-aGo1
			err := e.redeems(ctx, id, hash)
			close(aRedeemed)
			<-aGo2
			return err
		},
		func(ctx context.Context, id ids.ApprovalID, _ json.RawMessage, hash string) error {
			close(bIn)
			<-bGo
			return e.redeems(ctx, id, hash)
		},
	}}
	e.svc.WithEffect(kindSiteLead, script.run)

	aDone := e.decideAsync(deciding, id)
	<-aIn
	bDone := e.decideAsync(deciding, id)
	<-bIn
	close(aGo1)
	<-aRedeemed
	close(bGo)
	if err := <-bDone; err == nil {
		t.Fatal("the losing retry reported success")
	}
	if now := e.markOf(t, id); !now.Equal(mark) {
		t.Fatalf("the losing retry rewrote the failure mark (%v, was %v) while the winner still ran", now, mark)
	}
	close(aGo2)
	if err := <-aDone; err != nil {
		t.Fatalf("the winning retry: %v", err)
	}
	if at, _ := e.effectFailureOf(t, id); at != nil {
		t.Errorf("the work ran and the approval still reports a failure at %v", at)
	}
}

// A failure from an attempt that started before the work ran is out of date
// once it has run, and may not mark the row.
func TestAFailureFromBeforeTheWorkRanIsNotRecorded(t *testing.T) {
	e := setupStaging(t)
	deciding := e.asHumanWith(decidesEverything())
	id := e.stageInto(deciding, t, ids.NewV7(), e.company(t), kindSiteLead, "lead-anna")

	aIn, aGo, bIn, bGo := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	script := &scriptedEffect{steps: []ApprovedEffect{
		func(ctx context.Context, id ids.ApprovalID, _ json.RawMessage, hash string) error {
			close(aIn)
			<-aGo
			return e.redeems(ctx, id, hash)
		},
		func(context.Context, ids.ApprovalID, json.RawMessage, string) error {
			close(bIn)
			<-bGo
			return errors.New("the capture sink refused this lead")
		},
	}}
	e.svc.WithEffect(kindSiteLead, script.run)

	aDone := e.decideAsync(deciding, id)
	<-aIn
	bDone := e.decideAsync(deciding, id)
	<-bIn
	close(aGo)
	if err := <-aDone; err != nil {
		t.Fatalf("the decision whose work ran: %v", err)
	}
	close(bGo)
	if err := <-bDone; err == nil {
		t.Fatal("the failing attempt reported success")
	}
	if at, _ := e.effectFailureOf(t, id); at != nil {
		t.Errorf("an attempt that started before the work ran marked the row failed at %v", at)
	}
}
