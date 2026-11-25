// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func botChatBinding() connector.ChatBinding {
	return connector.ChatBinding{
		Provider: "telegram", CapturedBy: "connector:telegram",
		KeyScope: "42", ThreadScope: "42", Ownership: connector.ChatOwnedByWorkspace,
	}
}

func businessChatBinding(account string) connector.ChatBinding {
	b := botChatBinding()
	b.KeyScope, b.ThreadScope = "42:biz:"+account, "biz:"+account
	return b
}

func aChatMessage() connector.ChatMessage {
	return connector.ChatMessage{
		ConversationID: "1001", IdempotencyKey: "7", Direction: connector.DirectionInbound,
		Counterparty: connector.ChatParty{UserID: "1001", DisplayName: "Cora"},
		Body:         "hello", OccurredAt: time.Unix(1690000000, 0).UTC(),
	}
}

func recordOf(t *testing.T, b connector.ChatBinding, m connector.ChatMessage) connector.NormalizedRecord {
	t.Helper()
	rec, err := chatRecord(b, m)
	if err != nil {
		t.Fatalf("chatRecord: %v", err)
	}
	return rec
}

func TestAChatRecordKeepsTheKeysEachNetworkAlreadyFiledUnder(t *testing.T) {
	bot := recordOf(t, botChatBinding(), aChatMessage())
	if bot.NaturalKey.SourceID != "42:1001:7" || bot.ThreadKey != "telegram:42:1001" || bot.Source != "telegram:42:1001:7" {
		t.Errorf("bot record keyed %q / %q / %q, want 42:1001:7, telegram:42:1001 and telegram:42:1001:7",
			bot.NaturalKey.SourceID, bot.ThreadKey, bot.Source)
	}
	biz := recordOf(t, businessChatBinding("880001"), aChatMessage())
	if biz.NaturalKey.SourceID != "42:biz:880001:1001:7" || biz.ThreadKey != "telegram:biz:880001:1001" {
		t.Errorf("business record keyed %q / %q, want 42:biz:880001:1001:7 and telegram:biz:880001:1001",
			biz.NaturalKey.SourceID, biz.ThreadKey)
	}
	if !bot.NaturalKey.SourceIDNamesAContact || bot.NaturalKey.SourceSystem != "telegram" {
		t.Errorf("natural key = %+v, want telegram and the contact-naming trace flag", bot.NaturalKey)
	}
}

func TestTwoBusinessAccountsNeverShareAKeyAndABotKeyNeverEqualsABusinessKey(t *testing.T) {
	seen := map[string]string{}
	for name, b := range map[string]connector.ChatBinding{
		"bot": botChatBinding(), "account a": businessChatBinding("880001"), "account b": businessChatBinding("880002"),
	} {
		rec := recordOf(t, b, aChatMessage())
		for _, key := range []string{rec.NaturalKey.SourceID, rec.ThreadKey} {
			if other, dup := seen[key]; dup {
				t.Errorf("%s and %s both file under %q", name, other, key)
			}
			seen[key] = name
		}
	}
	// A reconnect hands the same account a new connection id, which the key
	// does not contain, so the same message keeps its key.
	again := recordOf(t, businessChatBinding("880001"), aChatMessage())
	if first := recordOf(t, businessChatBinding("880001"), aChatMessage()); first.NaturalKey != again.NaturalKey {
		t.Errorf("the same account's message keyed %q then %q", first.NaturalKey.SourceID, again.NaturalKey.SourceID)
	}
}

func TestAChatIDHoldingAColonIsRefusedBecauseItsKeyWouldBeAmbiguous(t *testing.T) {
	for _, tweak := range []func(*connector.ChatMessage){
		func(m *connector.ChatMessage) { m.ConversationID = "10:01" },
		func(m *connector.ChatMessage) { m.IdempotencyKey = "7:1" },
		func(m *connector.ChatMessage) { m.ConversationID = "" },
		func(m *connector.ChatMessage) { m.Counterparty.UserID = "" },
		func(m *connector.ChatMessage) { m.Direction = "sideways" },
	} {
		m := aChatMessage()
		tweak(&m)
		if _, err := chatRecord(botChatBinding(), m); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("message %+v: got %v, want an invalid-argument refusal", m, err)
		}
	}
}

func TestOnlyAMemberOwnedBindingMarksTheMessageAsTheMembers(t *testing.T) {
	workspace := recordOf(t, botChatBinding(), aChatMessage())
	if workspace.Fields.(ActivityFields).memberOwned {
		t.Error("a workspace-owned message is marked as a member's")
	}
	member := botChatBinding()
	member.Ownership, member.OwnerUserID = connector.ChatOwnedByMember, ids.NewV7()
	if !recordOf(t, member, aChatMessage()).Fields.(ActivityFields).memberOwned {
		t.Error("a member-owned message is not marked as its owner's")
	}
}

func TestAnAdapterCannotNameTheAudienceItself(t *testing.T) {
	// ChatMessage and ChatBinding carry no audience field; the only input that
	// narrows a message is the ownership of the authenticated binding.
	for _, in := range []any{connector.ChatMessage{}, connector.ChatBinding{}, connector.ChatParty{}} {
		typ := reflect.TypeOf(in)
		for field := range typ.Fields() {
			if strings.Contains(strings.ToLower(field.Name), "audience") {
				t.Errorf("%s.%s names an audience", typ.Name(), field.Name)
			}
		}
	}
}

func TestABindingWithoutAKnownOwnershipOrAMemberIsRefused(t *testing.T) {
	for name, tweak := range map[string]func(*connector.ChatBinding){
		"zero ownership":    func(b *connector.ChatBinding) { b.Ownership = "" },
		"unknown ownership": func(b *connector.ChatBinding) { b.Ownership = "team" },
		"member, no member": func(b *connector.ChatBinding) { b.Ownership = connector.ChatOwnedByMember },
	} {
		t.Run(name, func(t *testing.T) {
			b := botChatBinding()
			tweak(&b)
			if _, err := chatRecord(b, aChatMessage()); !errors.Is(err, apperrors.ErrInvalidArgument) {
				t.Errorf("chatRecord error = %v, want ErrInvalidArgument", err)
			}
		})
	}
	b := botChatBinding()
	b.Ownership, b.OwnerUserID = connector.ChatOwnedByMember, ids.NewV7()
	if _, err := chatRecord(b, aChatMessage()); err != nil {
		t.Errorf("a member-owned binding naming its member: %v", err)
	}
}
