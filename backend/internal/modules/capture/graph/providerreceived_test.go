// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"testing"
	"time"
)

func TestDeltaCapturePreservesProviderArrival(t *testing.T) {
	received := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	for _, resume := range []bool{false, true} {
		t.Run(map[bool]string{false: "anchor", true: "resume"}[resume], func(t *testing.T) {
			api := &fakeAPI{email: owner, initIDs: []string{"dated", "unknown"}, deltaIDs: []string{"dated", "unknown"}, receivedAt: map[string]time.Time{"dated": received}, raws: map[string][]byte{"dated": rawMsg("dated@test.example", "sender@test.example"), "unknown": rawMsg("unknown@test.example", "sender@test.example")}}
			sink := &recordingSink{}
			from := ""
			if resume {
				from = "https://graph/delta?token=prior"
			}
			if _, err := pinnedConn(api).pullFolder(context.Background(), "access", folderInbox, from, time.Time{}, sink, owner, false); err != nil {
				t.Fatal(err)
			}
			if len(sink.recs) != 2 {
				t.Fatalf("captured %d messages", len(sink.recs))
			}
			if !sink.recs[0].ProviderReceivedAt.Equal(received) {
				t.Fatalf("provider arrival = %v", sink.recs[0].ProviderReceivedAt)
			}
			if !sink.recs[1].ProviderReceivedAt.IsZero() {
				t.Fatal("missing provider arrival was invented")
			}
		})
	}
}

func TestDeltaAPIKeepsArrivalForEachMessage(t *testing.T) {
	received := anchorAfter.Add(time.Hour)
	held := 1
	srv, _ := datedAnchorStub(t, []map[string]any{{"id": "dated", "receivedDateTime": received.Format(time.RFC3339)}, {"id": "unknown"}}, &held)
	api := NewAPI(srv.Client(), srv.URL)
	messages, link, err := api.DeltaInit(context.Background(), "access", folderInbox, anchorAfter)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || !messages[0].ReceivedAt.Equal(received) || !messages[1].ReceivedAt.IsZero() {
		t.Fatalf("anchor messages = %+v", messages)
	}
	messages, _, _, err = api.Delta(context.Background(), "access", link)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || !messages[0].ReceivedAt.Equal(received) || !messages[1].ReceivedAt.IsZero() {
		t.Fatalf("delta messages = %+v", messages)
	}
}
