// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatlinks

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

type availableGroupClient struct {
	*receiptClient
	opened      atomic.Int64
	unavailable bool
}

func (c *availableGroupClient) Conversation(ctx context.Context, id string) (messaging.Conversation, error) {
	if c.unavailable {
		return messaging.Conversation{}, messaging.ErrUnavailable
	}
	return c.receiptClient.Conversation(ctx, id)
}

func (c *availableGroupClient) OpenPeople(ctx context.Context, _ []string) (messaging.Conversation, error) {
	c.opened.Add(1)
	return c.receiptClient.Conversation(ctx, "ROOM")
}

func TestAnUnavailableBoundGroupDoesNotCreateAnotherConversation(t *testing.T) {
	e, _ := setupMessengerSend(t, &receiptClient{})
	client := &availableGroupClient{receiptClient: &receiptClient{}}
	vault := e.store.vault()
	e.store.WithMessenger(func() keyvault.Vault { return vault }, func(context.Context, MemberGrant, []byte) (messaging.Client, error) { return client, nil })

	if _, err := e.store.MessengerOpenMembers(e.seat(e.rep), RecordDeal, e.deal, "slack", []string{"COLLEAGUE"}); err != nil {
		t.Fatal(err)
	}
	client.unavailable = true
	if _, err := e.store.MessengerOpenMembers(e.seat(e.rep), RecordDeal, e.deal, "slack", []string{"COLLEAGUE"}); !errors.Is(err, messaging.ErrUnavailable) {
		t.Fatalf("group read failure: %v", err)
	}
	if client.opened.Load() != 1 {
		t.Fatalf("created %d conversations for the same unavailable binding", client.opened.Load())
	}
	if count := e.count(t, "SELECT count(*) FROM chat_member_room"); count != 1 {
		t.Fatalf("stored %d group bindings", count)
	}
}
