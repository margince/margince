// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestAnExceptionSendDiscardsItsSavedDraftWithoutDiscardingNewerLegacyWork(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		legacy bool
		edited bool
	}{
		{name: "identified draft"},
		{name: "older held message", legacy: true},
		{name: "newer draft after an older hold", legacy: true, edited: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			c := setupConsent(t)
			c.useReviewableMarketingProfile(t)
			draft := AnyMap{
				"anchor_type": "activity", "anchor_id": c.activityID,
				"to": []string{"subject@consent.test"}, "subject": "Re: Inbound question", "body": "The requested information.",
			}
			var saved wireDraft
			if status := c.Call(t, "PUT", "/v1/mail-drafts", draft, nil, &saved); status != http.StatusOK {
				t.Fatalf("saving the draft answered %d", status)
			}
			if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
				"to": []string{"subject@consent.test"}, "subject": draft["subject"], "body": draft["body"],
				"consent_purpose": "marketing_email", "mail_draft_id": saved.ID,
			}, nil, nil); status != http.StatusConflict {
				t.Fatalf("the initial send answered %d, want refusal", status)
			}
			draftURL := "/v1/mail-drafts?anchor_type=activity&anchor_id=" + c.activityID
			if status := c.Call(t, "GET", draftURL, nil, nil, nil); status != http.StatusOK {
				t.Fatalf("a refusal discarded the draft: %d", status)
			}
			review := liveReviewID(t, c)
			if scenario.legacy {
				if _, err := c.Owner.Exec(context.Background(), `
					UPDATE scheduled_send SET payload = payload - 'pending_draft_id'
					 WHERE id = (SELECT delivery_intent_id FROM communication_review WHERE id = @review)`, pgx.NamedArgs{"review": review}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.edited {
				draft["body"] = "An unsent second message."
				if status := c.Call(t, "PUT", "/v1/mail-drafts", draft, map[string]string{"If-Match": strconv.FormatInt(saved.Version, 10)}, nil); status != http.StatusOK {
					t.Fatalf("editing the draft answered %d", status)
				}
			}
			if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), nil, nil); status != http.StatusCreated {
				t.Fatalf("sending the recorded exception answered %d", status)
			}
			var remaining wireDraft
			status := c.Call(t, "GET", draftURL, nil, nil, &remaining)
			if scenario.edited {
				if status != http.StatusOK || remaining.Body != draft["body"] {
					t.Fatalf("the older send discarded newer work: %d, %+v", status, remaining)
				}
			} else if status != http.StatusNotFound {
				t.Fatalf("the sent message can still be restored as a draft: %d", status)
			}
			if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), nil, nil); status == http.StatusCreated {
				t.Fatal("a repeated exception sent the message twice")
			}
			var deliveries int
			if err := c.Owner.QueryRow(context.Background(), `SELECT count(*) FROM comms_outbound`).Scan(&deliveries); err != nil {
				t.Fatal(err)
			}
			if deliveries != 1 {
				t.Fatalf("one exception staged %d deliveries", deliveries)
			}
		})
	}
}
