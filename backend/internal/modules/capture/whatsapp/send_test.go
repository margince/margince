// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func channelMessage(to string) connector.ChannelMessage {
	return connector.ChannelMessage{
		Recipient:      connector.ChannelIdentity{Provider: Provider, ChannelUserID: to},
		Body:           "Who wrote this?",
		IdempotencyKey: "ask:1",
	}
}

func TestTheSenderSendsTextThroughTheNumberItsCredentialNames(t *testing.T) {
	c, got := sendServer(t, 200, `{"messages":[{"id":"wamid.Q1"}]}`, nil)
	auth := connector.ChatSendAuth{Token: testToken, Account: "PNID1"}.Auth()
	receipt, err := NewSender(c).SendMessage(context.Background(), auth, channelMessage("491701234567"))
	if err != nil || receipt.ProviderMessageID != "wamid.Q1" {
		t.Fatalf("SendMessage = %+v, %v", receipt, err)
	}
	if got.path != "/v23.0/PNID1/messages" || got.auth != "Bearer "+testToken || got.body["to"] != "491701234567" {
		t.Fatalf("request = %+v", got)
	}
}

func TestTheSenderSaysWhatBecameOfASendInTheSharedVocabulary(t *testing.T) {
	envelope := func(code int) string { return fmt.Sprintf(`{"error":{"message":"m","code":%d}}`, code) }
	for name, tc := range map[string]struct {
		status int
		answer string
		want   error
	}{
		"token refused":      {401, envelope(190), connector.ErrAuthRejected},
		"outside the window": {400, envelope(131047), connector.ErrOutsideReplyWindow},
		"recipient refused":  {400, envelope(131026), connector.ErrRecipientUnreachable},
		"throttled":          {429, envelope(4), connector.ErrRateLimited},
		"no usable answer":   {503, `oops`, connector.ErrSendOutcomeUnknown},
		"accepted, no id":    {200, `{"messages":[]}`, connector.ErrSendOutcomeUnknown},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := sendServer(t, tc.status, tc.answer, nil)
			auth := connector.ChatSendAuth{Token: testToken, Account: "PNID1"}.Auth()
			if _, err := NewSender(c).SendMessage(context.Background(), auth, channelMessage("1")); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestTheSenderRefusesACredentialThatNamesNoNumberAndFiles(t *testing.T) {
	c, _ := sendServer(t, 200, `{}`, nil)
	sender := NewSender(c)
	if _, err := sender.SendMessage(context.Background(), connector.ChatSendAuth{Token: testToken}.Auth(), channelMessage("1")); !errors.Is(err, connector.ErrAuthRejected) {
		t.Fatalf("a credential with no phone number id = %v", err)
	}
	withFile := channelMessage("1")
	withFile.Files = []connector.OutboundFile{{}}
	if _, err := sender.SendMessage(context.Background(), connector.ChatSendAuth{Token: testToken, Account: "P"}.Auth(), withFile); !errors.Is(err, connector.ErrFilesNotCarried) {
		t.Fatalf("a message with a file = %v", err)
	}
}

func TestAContactsMessageIsDecodedAsASharedContactAndAReplyNamesItsQuote(t *testing.T) {
	body := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"W","changes":[{"field":"messages","value":{
		"metadata":{"phone_number_id":"109"},"contacts":[{"wa_id":"491701234567","profile":{"name":"Rep"}}],
		"messages":[{"id":"wamid.R1","from":"491701234567","timestamp":"1790000000","type":"contacts",
		 "context":{"id":"wamid.Q1"},
		 "contacts":[{"name":{"formatted_name":"Anna Weber"},"phones":[{"phone":"+49 151 1234 5678","wa_id":"4915112345678","type":"CELL"}]}]}]}}]}]}`)
	res, err := Decode(body)
	if err != nil || len(res.Messages) != 1 {
		t.Fatalf("Decode = %+v, %v", res, err)
	}
	chat, err := ChatMessage(res.Messages[0])
	if err != nil {
		t.Fatal(err)
	}
	card := chat.SharedContact
	if chat.ReplyToID != "wamid.Q1" || card == nil || card.DisplayName != "Anna Weber" || len(card.Phones) != 1 || card.Phones[0] != "4915112345678" {
		t.Fatalf("chat message = %+v, card = %+v", chat, card)
	}
}
