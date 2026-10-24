// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// senderOn files the request's From under a contact with one address, the
// way capture resolves a sender, and returns that contact.
func senderOn(t *testing.T, e *loadEnv, request ids.UUID, address string) ids.UUID {
	t.Helper()
	sender := e.buyer(t)
	e.exec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by) VALUES ($1, $2, 'seed', 'system')`, sender, address)
	e.exec(t, `INSERT INTO activity_participant (activity_id, role, address, contact_id) VALUES ($1, 'from', $2, $3)`, request, address, sender)
	return sender
}

// holdVerdict takes the verdict's locks on the request and keeps them until
// release is called, which a failing test does on its way out too. It returns
// once they are held.
func holdVerdict(t *testing.T, e *loadEnv, request ids.UUID) (release func() error) {
	t.Helper()
	held, let, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- storeKnowing(e).tx(asSettler(e), func(tx pgx.Tx) error {
			if _, err := holdSettlementEvidenceTx(asSettler(e), tx, RepliedRequest{RequestID: request, AsOf: bindingReadAt}); err != nil {
				return err
			}
			close(held)
			<-let
			return nil
		})
	}()
	<-held
	var once sync.Once
	var result error
	release = func() error {
		once.Do(func() {
			close(let)
			result = <-done
		})
		return result
	}
	t.Cleanup(func() {
		if err := release(); err != nil {
			t.Logf("the held verdict ended with %v", err)
		}
	})
	return release
}

// writeOutside runs one statement on its own transaction.
func writeOutside(e *loadEnv, statement string, args ...any) error {
	return storeKnowing(e).tx(asSettler(e), func(tx pgx.Tx) error {
		_, err := tx.Exec(asSettler(e), statement, args...)
		return err
	})
}

// A write that takes an answer's conversation away waits for a verdict that
// holds it, like a write that adds one. The statements are the ones contact
// merge, retention and erasure run on a sender's rows. They lock the keys of
// the row before the change. A row whose contact and address both change is
// caught only by those old keys. The same write on an unrelated contact does
// not wait.
func TestRemovingOrMovingAnAnswersConversationWaitsForTheVerdict(t *testing.T) {
	writes := map[string]string{
		"the sender's address is deleted":             `DELETE FROM contact_email WHERE contact_id = $1`,
		"the sender's address moves to a contact":     `UPDATE contact_email SET contact_id = $2 WHERE contact_id = $1`,
		"the sender's participant moves to a contact": `UPDATE activity_participant SET contact_id = $2 WHERE contact_id = $1`,
		"the sender's participant is deleted":         `DELETE FROM activity_participant WHERE contact_id = $1`,
		"the sender's participant is re-resolved":     `UPDATE activity_participant SET contact_id = $2, address = 'resolved@else.test' WHERE contact_id = $1`,
	}
	for name, statement := range writes {
		t.Run(name, func(t *testing.T) {
			e := setupLoad(t)
			request := seedEmailRequest(t, e, "Conversation moved", "commitment", OwedVerdictAsksUs)
			sender := senderOn(t, e, request, "sender-"+ids.NewV7().String()+"@customer.test")
			unrelated, target := e.buyer(t), e.buyer(t)
			elsewhere := "other-" + ids.NewV7().String() + "@else.test"
			e.exec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by) VALUES ($1, $2, 'seed', 'system')`, unrelated, elsewhere)
			e.exec(t, `INSERT INTO activity_participant (activity_id, role, address, contact_id) VALUES ($1, 'cc', $2, $3)`, unrelatedMessage(t, e), elsewhere, unrelated)
			args := func(moved ids.UUID) []any {
				if strings.Contains(statement, "$2") {
					return []any{moved, target}
				}
				return []any{moved}
			}
			release := holdVerdict(t, e, request)

			if err := writeOutsideBounded(e, statement, args(unrelated)...); err != nil {
				t.Fatalf("a write on an unrelated contact waited for the verdict: %v", err)
			}
			written := make(chan error, 1)
			go func() { written <- writeOutside(e, statement, args(sender)...) }()
			awaitLockWait(t, e)
			select {
			case err := <-written:
				t.Fatalf("the write ran while the verdict held the conversation (err %v)", err)
			default:
			}
			if err := release(); err != nil {
				t.Fatal(err)
			}
			if err := <-written; err != nil {
				t.Fatal(err)
			}
		})
	}
}

// unrelatedMessage is a request from someone else, for the unrelated
// contact's participant.
func unrelatedMessage(t *testing.T, e *loadEnv) ids.UUID {
	t.Helper()
	return seedEmailRequestWithCounterparty(t, e, "Unrelated", "commitment", OwedVerdictAsksUs, "someone@else.test")
}

// writeOutsideBounded is writeOutside that fails instead of waiting.
func writeOutsideBounded(e *loadEnv, statement string, args ...any) error {
	return storeKnowing(e).tx(asSettler(e), func(tx pgx.Tx) error {
		if _, err := tx.Exec(asSettler(e), `SET LOCAL lock_timeout = '2s'`); err != nil {
			return err
		}
		_, err := tx.Exec(asSettler(e), statement, args...)
		return err
	})
}

// An edit locks its answer's row, checks who may write it, and only then
// meets the conversation key in its update. A verdict that starts in between
// locks rows before keys, so the two do not deadlock. The edit commits, and
// the verdict over the evidence it changed is dropped. The audience table
// lock stops the edit after its row lock.
func TestAnEditHoldingItsAnswerAndAVerdictStartedThenBothFinish(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Edit and verdict", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Here is the report.", requestInstant.Add(time.Hour))
	candidate, ok := repliedIDs(t, e, bindingReadAt)[request]
	if !ok {
		t.Fatal("the answered request was not offered")
	}
	answer := ids.From[ids.ActivityKind](newestReplyID(t, e, request))
	locked, release, barrier := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var lift sync.Once
	t.Cleanup(func() { lift.Do(func() { close(release) }) })
	go func() {
		barrier <- storeKnowing(e).tx(asSettler(e), func(tx pgx.Tx) error {
			if _, err := tx.Exec(context.Background(), `LOCK TABLE activity_audience_member IN ACCESS EXCLUSIVE MODE`); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	edited := make(chan error, 1)
	go func() {
		body := "Edited: the report follows next week."
		_, err := storeKnowing(e).UpdateActivity(asEditor(e), answer, UpdateActivityInput{Body: &body})
		edited <- err
	}()
	awaitLockWaiters(t, e, 1)
	settled := make(chan error, 1)
	go func() {
		settled <- storeKnowing(e).SettleRequest(asSettler(e), RequestSettlementInput{Request: candidate, Verdict: RequestSettled, Confidence: 0.9, DecidedBy: "test-model"})
	}()
	awaitLockWaiters(t, e, 2)
	lift.Do(func() { close(release) })
	if err := <-barrier; err != nil {
		t.Fatal(err)
	}
	if err := <-edited; err != nil {
		t.Fatalf("the edit failed beside a verdict: %v", err)
	}
	if err := <-settled; err != nil {
		t.Fatalf("the verdict failed beside an edit: %v", err)
	}
	if got := settlementRows(t, e, request); got != 0 {
		t.Fatal("a verdict over evidence an edit changed was recorded")
	}
}
