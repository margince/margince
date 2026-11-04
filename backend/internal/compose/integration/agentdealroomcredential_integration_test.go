// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The link in an invitation signs in to the room as the buyer. An agent sends
// invitations and never holds that link. A signed-in colleague is handed it.
func TestAnAgentSendsARoomInvitationWithoutHoldingItsLink(t *testing.T) {
	d := newTwoDoors(t, "room-credential", "read", "write", "send")
	deal := openDeal(t, d.e, apptest.DiscoverSeededPipeline(t, d.e), "Credential renewal")
	room := createdRecord(t, d.e, "/v1/deal-rooms", AnyMap{"deal_id": deal, "title": "Credential room", "source": "manual"})

	type issued struct {
		Credential  string `json:"credential"`
		Participant struct {
			ID string `json:"id"`
		} `json:"participant"`
	}
	decode := func(what string, raw json.RawMessage) issued {
		t.Helper()
		var out issued
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("%s: %v\n%s", what, err, raw)
		}
		if out.Participant.ID == "" {
			t.Fatalf("%s named no participant: %s", what, raw)
		}
		return out
	}

	invited := decode("the agent's invitation", d.tool(t, "send_outbound", map[string]any{
		"as": "deal_room_invite", "id": room,
		"request_body": map[string]any{"full_name": "Ada Buyer", "email": "ada@room-credential.test", "source": "manual"},
	}))
	if invited.Credential != "" {
		t.Errorf("the agent's invitation handed it the buyer's link %q", invited.Credential)
	}
	resent := decode("the agent's resend", d.tool(t, "send_outbound", map[string]any{
		"as": "deal_room_resend", "id": room, "participantId": invited.Participant.ID,
	}))
	if resent.Credential != "" {
		t.Errorf("the agent's resend handed it the buyer's link %q", resent.Credential)
	}

	var byHuman json.RawMessage
	if status := d.e.Call(t, http.MethodPost, "/v1/deal-rooms/"+room+"/participants/"+invited.Participant.ID+"/resend", nil, nil, &byHuman); status != http.StatusCreated {
		t.Fatalf("the colleague's resend answered %d %s", status, byHuman)
	}
	if decode("the colleague's resend", byHuman).Credential == "" {
		t.Error("a signed-in colleague must still be handed the link to pass on")
	}
}
