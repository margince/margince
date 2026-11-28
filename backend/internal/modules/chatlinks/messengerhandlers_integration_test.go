// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatlinks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestMessengerHTTPReturnsOnlyTheParticipantsProviderReceipt(t *testing.T) {
	client := &receiptClient{}
	e, in := setupMessengerSend(t, client)
	handler := NewHandlers(e.store, nil)
	requestBody, err := json.Marshal(crmcontracts.MessengerSendRequest{
		RequestId: openapi_types.UUID(in.RequestID), GrantVersion: in.GrantVersion, ConversationId: in.Conversation, Text: in.Text, AudienceKey: in.AudienceKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/messenger/slack/deal/"+e.deal.String()+"/messages", strings.NewReader(string(requestBody))).WithContext(e.seat(e.rep))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.SendMessengerMessage(response, request, "slack", "deal", crmcontracts.Id(e.deal))
	if response.Code != http.StatusOK {
		t.Fatalf("send: %d %s", response.Code, response.Body.String())
	}
	var receipt crmcontracts.MessengerAttempt
	if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.State != "accepted" || receipt.ProviderMessageId != "PROVIDER-MESSAGE" {
		t.Fatalf("receipt: %+v", receipt)
	}
	get := httptest.NewRequest(http.MethodGet, "/receipt", nil).WithContext(e.seat(e.rep))
	wrongProvider := httptest.NewRecorder()
	handler.GetMessengerAttempt(wrongProvider, get, "teams", "deal", crmcontracts.Id(e.deal), openapi_types.UUID(in.RequestID))
	if wrongProvider.Code != http.StatusNotFound {
		t.Fatalf("another provider accepted receipt: %d %s", wrongProvider.Code, wrongProvider.Body.String())
	}
	other := httptest.NewRecorder()
	handler.GetMessengerAttempt(other, get.WithContext(e.seat(e.other)), "slack", "deal", crmcontracts.Id(e.deal), openapi_types.UUID(in.RequestID))
	if other.Code != http.StatusNotFound && other.Code != http.StatusForbidden {
		t.Fatalf("another member read receipt: %d %s", other.Code, other.Body.String())
	}
	system := httptest.NewRecorder()
	handler.GetMessengerIdentity(system, get.WithContext(e.system()), "slack")
	if system.Code != http.StatusForbidden {
		t.Fatalf("system read private identity: %d", system.Code)
	}
}

func TestUnlinkedRecordCanOpenMessengerAndKeepsHistoryEmpty(t *testing.T) {
	client := &receiptClient{}
	e, _ := setupMessengerSend(t, client)
	handler := NewHandlers(e.store, nil)
	request := httptest.NewRequest(http.MethodPost, "/conversation", strings.NewReader(`{"conversation_id":"ROOM"}`)).WithContext(e.seat(e.rep))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.OpenMessengerConversation(response, request, "slack", "deal", crmcontracts.Id(e.deal))
	if response.Code != http.StatusOK {
		t.Fatalf("open: %d %s", response.Code, response.Body.String())
	}
	var result crmcontracts.MessengerContext
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Conversation.Id != "ROOM" || result.AudienceKey == "" || result.Sender.Id != "REP" {
		t.Fatalf("conversation context: %+v", result)
	}
	if n := e.count(t, "SELECT count(*) FROM chat_link"); n != 0 {
		t.Fatalf("opening messenger created %d links", n)
	}
	if n := e.count(t, "SELECT count(*) FROM activity"); n != 0 {
		t.Fatalf("opening messenger filed %d activities", n)
	}
}
