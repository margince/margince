// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestReviewQueueTotalIncludesRowsBeyondTheLimit(t *testing.T) {
	c := setupConsent(t)
	for i := range 2 {
		if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
			"subject": fmt.Sprintf("Refused message %d", i), "body": "answer",
			"to": []string{"subject@consent.test"}, "consent_purpose": "marketing_email",
		}, nil, nil); status != http.StatusConflict {
			t.Fatalf("refusing a send: status %d, want 409", status)
		}
		var reviewID string
		if err := c.Owner.QueryRow(context.Background(), `
			SELECT id::text FROM communication_review
			WHERE state <> 'awaiting_decision' AND resolved_at IS NULL`).Scan(&reviewID); err != nil {
			t.Fatalf("reading the new refusal: %v", err)
		}
		if status := c.Call(t, "POST", "/v1/communication-reviews/"+reviewID+"/request-decision",
			AnyMap{}, nil, nil); status != http.StatusCreated {
			t.Fatalf("routing a refusal: status %d, want 201", status)
		}
	}

	for _, limit := range []int{1, 0, -1, 101, 2147483647} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			var listed struct {
				Data  []struct{ ID string } `json:"data"`
				Total int                   `json:"total"`
			}
			path := fmt.Sprintf("/v1/communication-reviews?limit=%d", limit)
			if status := c.Call(t, "GET", path, nil, nil, &listed); status != http.StatusOK {
				t.Fatalf("reading the queue: status %d, want 200", status)
			}
			wantRows := 2
			if limit == 1 {
				wantRows = 1
			}
			if len(listed.Data) != wantRows || listed.Total != 2 {
				t.Errorf("queue has %d rows and total %d, want %d rows and total 2",
					len(listed.Data), listed.Total, wantRows)
			}
		})
	}
}
