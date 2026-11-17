// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var testConnection = ids.MustParse("019a0000-0000-7000-8000-000000000001")

func inbound() PushMessage {
	return PushMessage{
		MessageID: "wamid.1", Network: "whatsapp", Body: "Hi",
		OccurredAt: time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC),
		From:       Party{Account: "8490@s.whatsapp.net", Name: "An Tran"},
		To:         Party{Account: "me@s.whatsapp.net", Name: "Me"},
	}
}

func TestTheNetworkIsFoldedIntoTheIdentity(t *testing.T) {
	n, err := Normalize(testConnection, inbound())
	if err != nil {
		t.Fatal(err)
	}
	if n.Account != "whatsapp/8490@s.whatsapp.net/"+testConnection.String() {
		t.Fatalf("account = %q", n.Account)
	}
	want := "personal_channels:" + testConnection.String() + ":" + n.Account
	if n.ThreadKey != want {
		t.Fatalf("thread key = %q, want %q", n.ThreadKey, want)
	}
	if n.Direction != DirectionInbound || n.Network != "whatsapp" {
		t.Fatalf("direction %q network %q", n.Direction, n.Network)
	}
}

func TestAMessageWithNoNetworkIsGeneric(t *testing.T) {
	m := inbound()
	m.Network = ""
	n, err := Normalize(testConnection, m)
	if err != nil || n.Network != "generic" || n.Account != "generic/8490@s.whatsapp.net/"+testConnection.String() {
		t.Fatalf("n = %+v, %v", n, err)
	}
}

func TestAnOutboundMessageNamesTheCustomerAsItsCounterparty(t *testing.T) {
	m := inbound()
	m.Direction = DirectionOutbound
	m.From, m.To = m.To, m.From
	n, err := Normalize(testConnection, m)
	if err != nil || n.Account != "whatsapp/8490@s.whatsapp.net/"+testConnection.String() || n.Direction != DirectionOutbound {
		t.Fatalf("n = %+v, %v", n, err)
	}
}

func TestAGroupMessageThreadsPerSenderWithinTheGroup(t *testing.T) {
	m := inbound()
	m.GroupID = "group-9"
	n, err := Normalize(testConnection, m)
	if err != nil {
		t.Fatal(err)
	}
	want := "personal_channels:" + testConnection.String() + ":" + n.Account + ":group-9"
	if n.ThreadKey != want {
		t.Fatalf("thread key = %q, want %q", n.ThreadKey, want)
	}
}

func TestAMessageThatCannotBeKeyedIsRefused(t *testing.T) {
	cases := map[string]func(*PushMessage){
		"colon in account":  func(m *PushMessage) { m.From.Account = "8490:12@s.whatsapp.net" },
		"colon in group":    func(m *PushMessage) { m.GroupID = "g:1" },
		"bad network":       func(m *PushMessage) { m.Network = "WhatsApp!" },
		"no message id":     func(m *PushMessage) { m.MessageID = " " },
		"no counterparty":   func(m *PushMessage) { m.From.Account = "" },
		"unknown direction": func(m *PushMessage) { m.Direction = "sideways" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			m := inbound()
			change(&m)
			if _, err := Normalize(testConnection, m); !errors.Is(err, apperrors.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
		})
	}
}

func TestTheSameAccountFromTwoConnectionsIsTwoIdentities(t *testing.T) {
	other := ids.MustParse("019a0000-0000-7000-8000-000000000002")
	a, errA := Normalize(testConnection, inbound())
	b, errB := Normalize(other, inbound())
	if errA != nil || errB != nil || a.Account == b.Account {
		t.Fatalf("identities %q and %q must differ per connection", a.Account, b.Account)
	}
	network, account, ok := SplitIdentity(a.Account)
	if !ok || network != "whatsapp" || account != "8490@s.whatsapp.net" {
		t.Fatalf("SplitIdentity(%q) = %q, %q, %v", a.Account, network, account, ok)
	}
}
