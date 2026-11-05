// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package imap

import (
	"strings"
	"testing"
	"time"

	imapv2 "github.com/emersion/go-imap/v2"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/capturetest"
)

func TestStandingSyncStoresArrivalWithoutReplacingItOnReplay(t *testing.T) {
	mailbox := capturetest.NewMailbox(t, "imap", memUser)
	raw := []byte(strings.ReplaceAll(string(inboundFixture()), "me@myco.com", memUser))
	addr, user := startMemServer(t, 0)
	received := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	if _, err := user.Append("INBOX", &memLiteral{b: raw}, &imapv2.AppendOptions{Time: received}); err != nil {
		t.Fatal(err)
	}
	ctx := mailbox.Context
	sink := capture.NewSink(mailbox.DB)
	connection := NewStanding().withDialer(plainDialer(addr))
	cursor, err := connection.Sync(ctx, standingAuth(t), nil, sink)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := user.Append("INBOX", &memLiteral{b: raw}, &imapv2.AppendOptions{Time: received.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Sync(ctx, standingAuth(t), cursor, sink); err != nil {
		t.Fatal(err)
	}
	if got := mailbox.Arrival(t, "abc123@acme.com"); !got.Equal(received) {
		t.Fatalf("arrival after replay = %v, want %v", got, received)
	}
}
