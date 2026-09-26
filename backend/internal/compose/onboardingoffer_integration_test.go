// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The offer slot against the real site_read row: the reply that offers writes
// it, the bare yes that follows is granted exactly the offered pair and spends
// it, and the audit trail names the offer the yes accepted.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const (
	offeringQuestion = "What name does the website use?"
	offeringMessage  = "The home page names Acme. Shall I use that as the display name?"
	offeringReply    = `{"kind":"answer","message":"` + offeringMessage + `","proposed_changes":[],` +
		`"offers":[{"field":"display_name","value":"Acme","source_ids":["S1"]}],"source_ids":["S1"]}`
	acceptingReply = `{"kind":"correction","message":"I'm proposing Acme as the display name.",` +
		`"proposed_changes":[{"field":"display_name","value":"Acme","reason":"You agreed to the home page's name.",` +
		`"source_ids":["S1"]}],"offers":[],"source_ids":["S1"]}`
)

func TestABareYesAcceptsTheOfferTheServerRecorded(t *testing.T) {
	env := integration.Setup(t)
	read := onboardingDraft(t, env)
	human := env.As(env.Rep1, nil, integration.AdminPerms)
	brain := &replyBrainStub{response: model.Response{Text: offeringReply}}
	engine := &deepReadEngine{contacts: env.Contacts, brain: brain, runtime: ai.NewRunTransparency(env.DB())}
	sendAs := func(who context.Context, message string, history ...crmcontracts.CompanySiteReadConversationTurn) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		engine.messageCompanySiteRead(recorder, offerMessageRequest(who, t, read.ID.String(), message, history), openapi_types.UUID(read.ID))
		return recorder
	}
	send := func(message string, history ...crmcontracts.CompanySiteReadConversationTurn) *httptest.ResponseRecorder {
		t.Helper()
		return sendAs(human, message, history...)
	}

	if got := send(offeringQuestion); got.Code != http.StatusOK {
		t.Fatalf("the offering turn → %d %s", got.Code, got.Body.String())
	}
	if field := env.WsScalar(t, `SELECT conversation_offer->>'field' FROM site_read WHERE id = $1`, read.ID); field != fieldDisplayName {
		t.Fatalf("the slot holds %q, want the offered display_name", field)
	}
	if n := env.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'site_read' AND entity_id = $1
		AND action = 'update' AND after->'conversation_offer'->>'value' = 'Acme'`, read.ID); n != 1 {
		t.Fatalf("the recorded offer has %d audit rows, want 1", n)
	}

	history := []crmcontracts.CompanySiteReadConversationTurn{
		{Role: crmcontracts.CompanySiteReadConversationTurnRoleUser, Message: offeringQuestion},
		{Role: crmcontracts.CompanySiteReadConversationTurnRoleAssistant, Message: offeringMessage},
	}
	brain.response = model.Response{Text: acceptingReply}
	// An offer is made to one human: another administrator replaying the same
	// conversation is granted nothing by it, and leaves it standing.
	bystander := env.As(env.Rep2, nil, integration.AdminPerms)
	if got := sendAs(bystander, "Yes", history...); got.Code == http.StatusOK {
		t.Fatalf("another administrator's yes accepted an offer made to Rep1: %s", got.Body.String())
	}
	if field := env.WsScalar(t, `SELECT conversation_offer->>'field' FROM site_read WHERE id = $1`, read.ID); field != fieldDisplayName {
		t.Fatalf("a refused bystander's yes changed the slot to %q", field)
	}
	accepted := send("Yes", history...)
	if accepted.Code != http.StatusOK {
		t.Fatalf("the yes → %d %s", accepted.Code, accepted.Body.String())
	}
	var reply crmcontracts.CompanySiteReadMessageReply
	if err := json.Unmarshal(accepted.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if len(reply.ProposedChanges) != 1 || reply.ProposedChanges[0].Value != "Acme" {
		t.Fatalf("the yes proposed %+v, want exactly the offered display name", reply.ProposedChanges)
	}
	if !strings.Contains(brain.request.Messages[0].Content, `"your_previous_offer"`) {
		t.Fatalf("the model was not shown the offer it made: %s", brain.request.Messages[0].Content)
	}
	if n := env.WsCount(t, `SELECT count(*) FROM site_read WHERE id = $1 AND conversation_offer IS NULL`, read.ID); n != 1 {
		t.Fatal("an accepted offer survived its answer")
	}
	if n := env.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'site_read' AND entity_id = $1
		AND evidence->'accepted_offer'->>'field' = 'display_name'`, read.ID); n != 1 {
		t.Fatalf("the audit names the accepted offer %d times, want 1", n)
	}

	// The same yes replayed over the same conversation finds nothing standing:
	// the offer was good for one reply.
	if replayed := send("Yes", history...); replayed.Code == http.StatusOK {
		t.Fatalf("a spent offer granted a second change: %s", replayed.Body.String())
	}
}

func offerMessageRequest(ctx context.Context, t *testing.T, readID, message string, history []crmcontracts.CompanySiteReadConversationTurn) *http.Request {
	t.Helper()
	body, err := json.Marshal(crmcontracts.CompanySiteReadMessageRequest{Message: message, History: &history})
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewRequest(http.MethodPost, "/v1/company/site-reads/"+readID+"/messages", strings.NewReader(string(body))).WithContext(ctx)
}
