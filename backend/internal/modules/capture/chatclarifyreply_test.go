// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestAPhoneNumberAnswersOnlyAsAShortTextWithOneNumber(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]string{
		"+49 151 12345678":                    "4915112345678",
		"(0151) 1234-5678":                    "015112345678",
		"  +84 90 123 4567 ":                  "84901234567",
		"call 12345 or 67890":                 "",
		"12345":                               "",
		"hello":                               "",
		"":                                    "",
		strings.Repeat("1", maxPhoneDigits+1): "",
		"here is the number you asked me for, +49 151 12345678": "",
	} {
		if got := phoneInTextOnly(text); got != want {
			t.Errorf("phoneInTextOnly(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestACustomerIsNamedInTheNetworksOwnIdentity(t *testing.T) {
	t.Parallel()
	card := &connector.ChatSharedContact{DisplayName: "Anna", Phones: []string{"+49 151 12345678"}, UserID: "u-1"}
	cases := map[string]struct {
		binding connector.ChatBinding
		msg     connector.ChatMessage
		want    connector.ChannelIdentity
		named   bool
	}{
		"whatsapp card":       {connector.ChatBinding{Provider: "whatsapp"}, connector.ChatMessage{SharedContact: card}, connector.ChannelIdentity{Provider: "whatsapp", ChannelUserID: "4915112345678"}, true},
		"whatsapp text":       {connector.ChatBinding{Provider: "whatsapp"}, connector.ChatMessage{Body: "+4915112345678"}, connector.ChannelIdentity{Provider: "whatsapp", ChannelUserID: "4915112345678"}, true},
		"telegram card":       {connector.ChatBinding{Provider: "telegram"}, connector.ChatMessage{SharedContact: card}, connector.ChannelIdentity{Provider: "telegram", ChannelUserID: "u-1"}, true},
		"telegram text":       {connector.ChatBinding{Provider: "telegram"}, connector.ChatMessage{Body: "+4915112345678"}, connector.ChannelIdentity{}, false},
		"telegram phone only": {connector.ChatBinding{Provider: "telegram"}, connector.ChatMessage{SharedContact: &connector.ChatSharedContact{Phones: []string{"+4915112345678"}}}, connector.ChannelIdentity{}, false},
		"zalo card":           {connector.ChatBinding{Provider: "zalo_oa", AccountScope: "88"}, connector.ChatMessage{SharedContact: card}, connector.ChannelIdentity{Provider: "zalo_oa", AccountScope: "88", ChannelUserID: "u-1"}, true},
		"another network":     {connector.ChatBinding{Provider: "signal"}, connector.ChatMessage{SharedContact: card}, connector.ChannelIdentity{}, false},
	}
	for name, tc := range cases {
		got, _, named := customerNamedBy(tc.binding, tc.msg)
		if named != tc.named || got != tc.want {
			t.Errorf("%s: customerNamedBy = %+v, %v; want %+v, %v", name, got, named, tc.want, tc.named)
		}
	}
}

func TestAnAnswerIsBoundToTheBatchItQuotesOrToTheOneThatWaitsAndNeverGuesses(t *testing.T) {
	t.Parallel()
	older, newer := ids.NewV7(), ids.NewV7()
	held := func(batch ids.UUID, ask string, pending bool) askedMessage {
		return askedMessage{batch: batch, askIDs: []string{ask}, activity: ids.NewV7(), pending: pending}
	}
	two := []askedMessage{held(newer, "q2", true), held(older, "q1", true), held(older, "q1", true)}

	target, quoted, ambiguous := answeredBatch(two, "q1")
	if len(target) != 2 || !quoted || ambiguous {
		t.Fatalf("a quoted answer = %d messages, quoted %v, ambiguous %v; want the older batch's two", len(target), quoted, ambiguous)
	}
	if _, _, ambiguous := answeredBatch(two, ""); !ambiguous {
		t.Fatal("two waiting batches and no quote must be ambiguous")
	}
	if _, _, ambiguous := answeredBatch(two, "something the account never wrote"); !ambiguous {
		t.Fatal("a quote of an unknown message must not pick a batch")
	}
	one := []askedMessage{held(older, "q1", true), held(newer, "q2", false)}
	if target, _, ambiguous := answeredBatch(one, ""); len(target) != 1 || ambiguous {
		t.Fatalf("one waiting batch and no quote = %d messages, ambiguous %v", len(target), ambiguous)
	}
	if target, quoted, _ := answeredBatch(one, "q2"); len(target) != 0 || !quoted {
		t.Fatalf("a quote of a filed batch = %d messages, quoted %v; want none and quoted", len(target), quoted)
	}
}

func TestTheQuestionIsPutInTheColleaguesLanguageAndAsksForAPhoneOnlyWhereOneWorks(t *testing.T) {
	t.Parallel()
	if got := clarifyQuestion("", "whatsapp"); got != "Who wrote this? Reply with their contact card or phone number." {
		t.Errorf("English WhatsApp question = %q", got)
	}
	if got := clarifyQuestion("en", "telegram"); strings.Contains(got, "phone") {
		t.Errorf("a Telegram question asks for a phone number: %q", got)
	}
	for _, locale := range []string{"de", "vi"} {
		if clarifyQuestion(locale, "whatsapp") == clarifyQuestion("en", "whatsapp") {
			t.Errorf("no %s copy for the question", locale)
		}
		list := followUpQuestion(locale, []pickItem{{At: time.Date(2026, 11, 26, 9, 5, 0, 0, time.UTC), Count: 2, Excerpt: "price for 20 seats?"}})
		if !strings.Contains(list, "1. 2026-11-26 09:05 UTC") || !strings.Contains(list, "price for 20 seats?") {
			t.Errorf("%s follow-up does not list the batch: %q", locale, list)
		}
	}
}
