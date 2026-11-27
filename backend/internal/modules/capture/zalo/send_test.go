// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zalo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func zaloSender(t *testing.T, answer string) (*Sender, *string) {
	t.Helper()
	var request string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		request = r.URL.Path + " " + r.Header.Get("access_token") + " " + string(raw)
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(srv.Close)
	return NewSender(NewClient(srv.Client(), srv.URL, srv.URL)), &request
}

func zaloMessage() connector.ChannelMessage {
	return connector.ChannelMessage{
		Recipient:      connector.ChannelIdentity{Provider: Provider, ChannelUserID: "zu-1"},
		Body:           "Who wrote this?",
		IdempotencyKey: "ask:1",
	}
}

func TestTheSenderSendsACustomerServiceTextAsTheOfficialAccount(t *testing.T) {
	sender, request := zaloSender(t, `{"error":0,"data":{"message_id":"m-1"}}`)
	receipt, err := sender.SendMessage(context.Background(), connector.ChatSendAuth{Token: "oa-token"}.Auth(), zaloMessage())
	if err != nil || receipt.ProviderMessageID != "m-1" {
		t.Fatalf("SendMessage = %+v, %v", receipt, err)
	}
	if !strings.HasPrefix(*request, "/v3.0/oa/message/cs oa-token ") || !strings.Contains(*request, `"user_id":"zu-1"`) {
		t.Fatalf("request = %s", *request)
	}
}

func TestTheSenderSaysWhatBecameOfASendInTheSharedVocabulary(t *testing.T) {
	for name, tc := range map[string]struct {
		answer string
		want   error
	}{
		"grant expired":      {`{"error":-124,"message":"x"}`, connector.ErrAuthRejected},
		"outside the window": {`{"error":-213,"message":"x"}`, connector.ErrOutsideReplyWindow},
		"unknown recipient":  {`{"error":-201,"message":"x"}`, connector.ErrRecipientUnreachable},
		"throttled":          {`{"error":-32,"message":"x"}`, connector.ErrRateLimited},
		"no usable answer":   {`oops`, connector.ErrSendOutcomeUnknown},
	} {
		t.Run(name, func(t *testing.T) {
			sender, _ := zaloSender(t, tc.answer)
			if _, err := sender.SendMessage(context.Background(), connector.ChatSendAuth{Token: "t"}.Auth(), zaloMessage()); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestABusinessCardAndAQuoteAreDecodedForTheAnswerPath(t *testing.T) {
	ev, err := Decode([]byte(`{"event_name":"user_send_business_card","timestamp":"1790000000000","sender":{"id":"zu-1"},"recipient":{"id":"88"},
		"message":{"msg_id":"m-2","quote_msg_id":"m-1","attachments":[{"type":"business_card","payload":{"user_id":"z-customer","phone":"84901234567"}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	chat, err := ChatMessage(ev)
	if err != nil {
		t.Fatal(err)
	}
	if chat.ReplyToID != "m-1" || chat.SharedContact == nil || chat.SharedContact.UserID != "z-customer" || chat.SharedContact.Phones[0] != "84901234567" {
		t.Fatalf("chat message = %+v card = %+v", chat, chat.SharedContact)
	}
}
