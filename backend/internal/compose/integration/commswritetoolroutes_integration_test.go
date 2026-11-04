// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The writes behind a message or a meeting: draft_email, book_meeting,
// send_email and send_company_email. Each test asks the route as a passport and
// the tool over MCP the same question, then compares what they answer, store,
// or refuse. The send paths hold no consent in this lane, so a send is compared
// on the one refusal both doors give.

import (
	"net/http"
	"strings"
	"testing"
)

type sharedDraft struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// A reply drafted to one email is the same text on either door: the same
// subject and the same body, greeting included.
func TestDraftEmailAnswersAsItsRouteDoes(t *testing.T) {
	d := newTwoDoors(t, "draft-write-doors", "read", "write", "draft")
	contact := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": "Pat Draft", "source": "manual"})
	mail := createdID(t, d.e, "/v1/activities", AnyMap{
		"kind": "email", "direction": "inbound", "subject": "Quote?", "body": "Can you send a quote", "source": "manual",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": contact}},
	})

	drafted := sameRecords[sharedDraft](t, "draft",
		d.restStatus(t, "POST", "/v1/activities/"+mail+"/draft-email", AnyMap{"intent": "Quote by Friday"}, http.StatusOK),
		d.tool(t, "draft_email", map[string]any{"activity_id": mail, "intent": "Quote by Friday"}))
	if drafted.Subject != "Re: Quote?" || strings.Contains(drafted.Body, "Quote by Friday") || !strings.Contains(drafted.Body, "Hi Pat") {
		t.Errorf("the draft is %+v, want a reply to the thread that leaves the steering out", drafted)
	}
}

// A send to someone who has not consented is refused the same way on either
// door. That holds for a reply and for a first message, and nothing is sent.
// Only the refusal is compared: a permitted email goes out through a connected
// mailbox, and the harness has none to stand in for it.
func TestSendsAreRefusedAlikeOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "send-write-doors", "read", "write", "send")
	contact := createdID(t, d.e, "/v1/contacts", AnyMap{
		"full_name": "Send Pat", "source": "manual",
		"emails": []AnyMap{{"email": "pat@send.example", "email_type": "work", "is_primary": true}},
	})
	link := []map[string]any{{"entity_type": "contact", "entity_id": contact}}
	mail := createdID(t, d.e, "/v1/activities", AnyMap{
		"kind": "email", "direction": "inbound", "subject": "Quote?", "body": "Can you send a quote", "source": "manual", "links": link,
	})

	// The route words the refusal with a review id; the shared part is what it
	// says before that.
	routeRefusal := func(what, path string, body AnyMap) string {
		t.Helper()
		var refusal fileRefusal
		status := d.e.Call(t, "POST", path, body, d.bearer, &refusal)
		if status != http.StatusConflict || refusal.Code != "consent_not_granted" {
			t.Fatalf("%s over REST → %d %q, want 409 consent_not_granted", what, status, refusal.Code)
		}
		shared, _, _ := strings.Cut(refusal.Detail, " (review")
		return shared
	}
	sameRefusal := func(what, shared, toolText string) {
		t.Helper()
		if !strings.Contains(toolText, "consent_not_granted") || !strings.Contains(toolText, shared) {
			t.Errorf("%s: the route refused with %q and the tool with %q, want one refusal", what, shared, toolText)
		}
	}

	reply := routeRefusal("a reply", "/v1/activities/"+mail+"/send-email",
		AnyMap{"to": []string{"pat@send.example"}, "subject": "Re: Quote?", "body": "Quote attached"})
	sameRefusal("a reply", reply, d.toolRefused(t, "send_email", map[string]any{
		"activity_id": mail, "to": []string{"pat@send.example"}, "subject": "Re: Quote?", "body": "Quote attached",
	}))
	first := routeRefusal("a first message", "/v1/emails",
		AnyMap{"to": []string{"pat@send.example"}, "subject": "Hello", "body": "Nice to meet you", "links": link})
	sameRefusal("a first message", first, d.toolRefused(t, "send_company_email", map[string]any{
		"to": []string{"pat@send.example"}, "subject": "Hello", "body": "Nice to meet you", "links": link,
	}))
}
