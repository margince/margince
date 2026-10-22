// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package graph

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/capturetest"
)

func TestDeltaThenBackfillKeepsTheFirstKnownArrival(t *testing.T) {
	mailbox := capturetest.NewMailbox(t, "graph", owner)
	received := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	api := &fakeAPI{email: owner, deltaIDs: []string{"dated", "unknown"}, listIDs: []string{"dated", "unknown"}, receivedAt: map[string]time.Time{"dated": received}, raws: map[string][]byte{"dated": rawMsg("dated@test.example", "sender@test.example"), "unknown": rawMsg("unknown@test.example", "sender@test.example")}}
	ctx := mailbox.Context
	sink := capture.NewSink(mailbox.DB)
	connection := pinnedConn(api)
	if _, err := connection.pullFolder(ctx, "access", folderInbox, "https://graph/delta?token=prior", time.Time{}, sink, owner, false); err != nil {
		t.Fatal(err)
	}
	api.receivedAt["dated"] = received.Add(time.Hour)
	api.receivedAt["unknown"] = received
	if _, err := connection.BackfillPage(ctx, authBytes(t), time.Time{}, "", sink); err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"dated@test.example", "unknown@test.example"} {
		if got := mailbox.Arrival(t, message); !got.Equal(received) {
			t.Errorf("%s arrival = %v, want %v", message, got, received)
		}
	}
}
