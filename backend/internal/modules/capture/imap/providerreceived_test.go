// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imap

import (
	"context"
	"testing"
	"time"

	imapv2 "github.com/emersion/go-imap/v2"
)

func TestStandingSyncPreservesInternalDate(t *testing.T) {
	addr, user := startMemServer(t, 0)
	received := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	if _, err := user.Append("INBOX", &memLiteral{b: inboundFixture()}, &imapv2.AppendOptions{Time: received}); err != nil {
		t.Fatal(err)
	}
	sink := &recordingSink{}
	if _, err := NewStanding().withDialer(plainDialer(addr)).Sync(context.Background(), standingAuth(t), nil, sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.records) != 1 {
		t.Fatalf("captured %d messages", len(sink.records))
	}
	if !sink.records[0].ProviderReceivedAt.Equal(received) {
		t.Fatalf("provider arrival = %v, want %v", sink.records[0].ProviderReceivedAt, received)
	}
}
