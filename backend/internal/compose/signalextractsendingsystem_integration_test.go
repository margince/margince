// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// markSentBySystem stamps a seeded message the way capture does for a sending
// system's mail.
func markSentBySystem(t *testing.T, id ids.UUID) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE activity SET sender_is_machine = true WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

// A thread written only by a sending system is not on the queue, so it costs no
// model call and files no signal. The same fixture from a person is offered, and
// the trace names the arm for the first.
func TestAThreadOfOnlySendingSystemMailIsNotRead(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Notifier Inc", &e.Rep1)
	at := traceReadClock.Add(-48 * time.Hour)
	const machineKey, humanKey = "thread-machine-only", "thread-human"
	machine := seedConversation(t, e, company, machineKey, at, "workspace")
	markSentBySystem(t, machine)
	seedConversation(t, e, company, humanKey, at, "workspace")

	offered := offeredKeys(t, e)
	if offered[machineKey] {
		t.Error("a thread of only sending-system mail was offered to the reader")
	}
	if !offered[humanKey] {
		t.Fatal("the same conversation from a person is not offered, so the skip proves nothing")
	}

	reading, err := NewThreadReadings(e.DB(), func() time.Time { return traceReadClock }).
		ReadThread(e.Admin(), machineKey, machine.String())
	if err != nil {
		t.Fatal(err)
	}
	if !reading.Known || reading.HasAHumanSender {
		t.Errorf("trace: known=%v hasAHumanSender=%v, want a known thread with no human sender",
			reading.Known, reading.HasAHumanSender)
	}
}

// A message from a sending system adds no company to its thread. Here a
// notification sits on another account (its address was imported as a contact
// there) and a person writes from the real one: the thread reaches one account.
func TestASendingSystemMessageDoesNotPullAThreadOntoItsCompany(t *testing.T) {
	e := integration.Setup(t)
	real := e.SeedCompany(t, "Real Customer", &e.Rep1)
	notifier := e.SeedCompany(t, "Notifier Inc", &e.Rep1)
	at := traceReadClock.Add(-48 * time.Hour)
	const key = "thread-mixed"
	seedConversation(t, e, real, key, at, "workspace")
	notice := seedConversation(t, e, notifier, key, at.Add(time.Minute), "workspace")
	markSentBySystem(t, notice)

	if !offeredKeys(t, e)[key] {
		t.Fatal("the notice pulled its own company onto the thread, which now reaches two accounts")
	}
}
