// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// Done on a message we sent goes through PUT /activities/{id}/disposition and
// the set_activity_disposition tool alike. Both take only the reader's own
// newest message to a contact, before anyone wrote again: the message the
// contact page's waiting card rests on.
func TestDoneOnASentMessageIsTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "sent-done-doors", "read", "write")
	now := clocktest.Now(t)
	mail := func(contact, direction string, ago time.Duration) string {
		t.Helper()
		return createdID(t, d.e, "/v1/activities", AnyMap{
			"kind": "email", "direction": direction, "subject": "The quote", "source": "manual",
			"occurred_at": now.Add(-ago).Format(time.RFC3339),
			"links":       []AnyMap{{"entity_type": "contact", "entity_id": contact}},
		})
	}
	contact := func(name string) string {
		t.Helper()
		return createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": name, "source": "manual"})
	}
	viaREST := func(message string, want int) {
		t.Helper()
		d.restStatus(t, "PUT", "/v1/activities/"+message+"/disposition", AnyMap{"disposition": "handled"}, want)
	}
	toolOn := func(message string) map[string]any {
		return map[string]any{"action": "set_aside", "id": message, "request_body": map[string]any{"disposition": "handled"}}
	}

	viaREST(mail(contact("Rest Waiting"), "outbound", 2*24*time.Hour), http.StatusNoContent)
	d.tool(t, "change_activities", toolOn(mail(contact("Tool Waiting"), "outbound", 2*24*time.Hour)))

	colleagues := mail(contact("Colleague Waiting"), "outbound", 2*24*time.Hour)
	if _, err := d.e.Owner.Exec(t.Context(),
		`UPDATE activity_participant SET user_id = NULL, address = 'colleague@ourco.test' WHERE activity_id = $1 AND role = 'from'`,
		colleagues); err != nil {
		t.Fatalf("handing the message to a colleague: %v", err)
	}
	again := contact("Written Again")
	older := mail(again, "outbound", 3*24*time.Hour)
	mail(again, "outbound", 2*24*time.Hour)
	replied := contact("Replied")
	answered := mail(replied, "outbound", 3*24*time.Hour)
	mail(replied, "inbound", 2*24*time.Hour)

	for name, message := range map[string]string{
		"a colleague's message": colleagues, "an older message": older, "an answered message": answered,
	} {
		var refusal AnyMap
		if status := d.e.Call(t, "PUT", "/v1/activities/"+message+"/disposition",
			AnyMap{"disposition": "handled"}, d.bearer, &refusal); status != http.StatusUnprocessableEntity {
			t.Errorf("Done on %s over REST → %d %v, want 422", name, status, refusal)
		}
		if text := d.mcp.CallRefused(t, "change_activities", toolOn(message)); !strings.Contains(text, "your own newest message to a contact") {
			t.Errorf("Done on %s over the tool was refused with %q, want the refusal the route gives", name, text)
		}
	}
}
