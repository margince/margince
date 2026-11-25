// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package telegram

import (
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// businessUpdateJSON is a business_message in a private chat with customer 1001.
// from is who typed it: the customer, or the account owner replying in their app.
func businessUpdateJSON(from string, chatType string, chatID string) string {
	return `{
		"update_id": 300,
		"business_message": {
			"business_connection_id": "bc-1",
			"message_id": 9,
			"chat": {"id": ` + chatID + `, "type": "` + chatType + `", "first_name": "Cora", "last_name": "Kunde", "username": "cora"},
			"from": ` + from + `,
			"date": 1690000000,
			"text": "is it in stock?"
		}
	}`
}

const (
	customerFrom = `{"id": 1001, "first_name": "Cora", "last_name": "Kunde", "username": "cora"}`
	ownerFrom    = `{"id": 880001, "first_name": "Mia", "username": "mia"}`
)

var approvedLink = &BusinessLink{AccountID: "880001", Ownership: connector.ChatOwnedByWorkspace}

func TestACustomersBusinessMessageIsInboundFromTheChat(t *testing.T) {
	p, err := parseUpdate(t, businessUpdateJSON(customerFrom, "private", "1001"), approvedLink)
	if err != nil {
		t.Fatalf("ParseChatMessage: %v", err)
	}
	if p.msg.Direction != connector.DirectionInbound {
		t.Errorf("direction = %q, want inbound", p.msg.Direction)
	}
	want := connector.ChatParty{UserID: "1001", DisplayName: "Cora Kunde", Username: "cora"}
	if p.msg.Counterparty != want {
		t.Errorf("counterparty = %+v, want %+v", p.msg.Counterparty, want)
	}
}

func TestTheOwnersNativeReplyIsOutboundToTheSameChat(t *testing.T) {
	p, err := parseUpdate(t, businessUpdateJSON(ownerFrom, "private", "1001"), approvedLink)
	if err != nil {
		t.Fatalf("ParseChatMessage: %v", err)
	}
	if p.msg.Direction != connector.DirectionOutbound {
		t.Errorf("direction = %q, want outbound: the sender is the account owner", p.msg.Direction)
	}
	want := connector.ChatParty{UserID: "1001", DisplayName: "Cora Kunde", Username: "cora"}
	if p.msg.Counterparty != want || p.msg.ConversationID != "1001" {
		t.Errorf("counterparty = %+v in conversation %q, want the customer %+v: the owner is not the counterparty", p.msg.Counterparty, p.msg.ConversationID, want)
	}
}

func TestABusinessMessageKeysOnTheAccountNotTheConnection(t *testing.T) {
	p, err := parseUpdate(t, businessUpdateJSON(customerFrom, "private", "1001"), approvedLink)
	if err != nil {
		t.Fatalf("ParseChatMessage: %v", err)
	}
	if p.binding.KeyScope != "42:biz:880001" || p.binding.ThreadScope != "biz:880001" {
		t.Errorf("scopes = %q and %q, want the bot and the owner's account, with no connection id", p.binding.KeyScope, p.binding.ThreadScope)
	}
}

func TestOnlyAPrivateBusinessChatIsParsed(t *testing.T) {
	for _, chat := range []struct{ kind, id string }{{"group", "-1001"}, {"supergroup", "-1002"}, {"private", "-1003"}, {"", "1001"}} {
		_, err := parseUpdate(t, businessUpdateJSON(customerFrom, chat.kind, chat.id), approvedLink)
		if !errors.Is(err, connector.ErrSkip) {
			t.Errorf("chat %q %s: got %v, want an ErrSkip-wrapped refusal", chat.kind, chat.id, err)
		}
	}
}

func TestAMessageKindThatDoesNotMatchTheLinkIsSkipped(t *testing.T) {
	if _, err := parseUpdate(t, businessUpdateJSON(customerFrom, "private", "1001"), nil); !errors.Is(err, connector.ErrSkip) {
		t.Errorf("a business_message with no link: got %v, want ErrSkip", err)
	}
	if _, err := parseUpdate(t, telegramUpdateFixture, approvedLink); !errors.Is(err, connector.ErrSkip) {
		t.Errorf("a bot message read through a link: got %v, want ErrSkip", err)
	}
}

func TestAnAmbiguousAccountIDIsRefused(t *testing.T) {
	for _, account := range []string{"", "880001:1001"} {
		_, err := parseUpdate(t, businessUpdateJSON(customerFrom, "private", "1001"), &BusinessLink{AccountID: account})
		if err == nil || errors.Is(err, connector.ErrSkip) {
			t.Errorf("account %q: got %v, want a refusal that is not a skip", account, err)
		}
	}
}

func TestABusinessMessageNamesItsConnection(t *testing.T) {
	id, isBusiness, err := BusinessConnectionIDOf([]byte(businessUpdateJSON(customerFrom, "private", "1001")))
	if err != nil || !isBusiness || id != "bc-1" {
		t.Errorf("= %q, %v, %v, want bc-1 on a business message", id, isBusiness, err)
	}
	if _, isBusiness, _ := BusinessConnectionIDOf([]byte(telegramUpdateFixture)); isBusiness {
		t.Error("a message to the bot reads as a business message")
	}
}

func TestABusinessMessagesSubjectIsTheCustomerWhoeverTyped(t *testing.T) {
	for name, from := range map[string]string{"customer": customerFrom, "owner": ownerFrom} {
		got, err := InScopeSubjects([]byte(businessUpdateJSON(from, "private", "1001")))
		if err != nil || !slices.Equal(got, []string{"1001"}) {
			t.Errorf("%s typed: subjects = %v, %v, want [1001]", name, got, err)
		}
	}
	if got, _ := InScopeSubjects([]byte(businessUpdateJSON(customerFrom, "supergroup", "-1002"))); len(got) != 0 {
		t.Errorf("a business message in a group names %v, want none", got)
	}
	if got, _ := InScopeSubjects([]byte(businessUpdateJSON(`{"id": 0}`, "private", "1001"))); len(got) != 0 {
		t.Errorf("a business message with no addressable sender names %v, want none", got)
	}
}

func TestTheBusinessMessageKindIsSubscribed(t *testing.T) {
	if !slices.Contains(AllowedUpdates(), "business_message") {
		t.Error("business_message is not subscribed, so Telegram would never send the owner's chats")
	}
}
