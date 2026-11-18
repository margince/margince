// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/sendmetrics"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func setInlineSendReview(t *testing.T, c *consentEnv, on bool) {
	t.Helper()
	if _, err := c.Owner.Exec(t.Context(), `
		INSERT INTO setting (key, value, updated_at) VALUES ('activities.inline_send_review', to_jsonb($1::boolean), now())
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = now()`, on); err != nil {
		t.Fatalf("setting the inline review switch to %v: %v", on, err)
	}
}

// sendSeries reads one series of this process's send families.
func sendSeries(t *testing.T, series string) int {
	t.Helper()
	var b strings.Builder
	sendmetrics.WriteProcessMetrics(&b)
	for line := range strings.SplitSeq(b.String(), "\n") {
		if value, ok := strings.CutPrefix(line, series+" "); ok {
			n, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("%s reads %q", series, value)
			}
			return n
		}
	}
	t.Fatalf("no series %s", series)
	return 0
}

// The pilot switch can be turned off with work in flight. A send refused under
// the inline composer still resolves through the legacy Send anyway dialog,
// what the rep left behind stays readable, and the message goes once.
func TestTurningTheInlineReviewOffLeavesItsOpenReviewResolvable(t *testing.T) {
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	setInlineSendReview(t, c, true)

	const (
		inlineReview = `margince_email_send_outcomes_total{flow="inline",outcome="review"}`
		inlineQueued = `margince_email_send_outcomes_total{flow="inline",outcome="queued"}`
		legacySent   = `margince_email_send_anyway_total{flow="legacy",result="sent"}`
		refusedTimed = `margince_email_send_decision_seconds_count{verdict="refused"}`
	)
	reviewsBefore, queuedBefore := sendSeries(t, inlineReview), sendSeries(t, inlineQueued)
	sentBefore, timedBefore := sendSeries(t, legacySent), sendSeries(t, refusedTimed)

	otherAnchor := logConsentInbound(t, c.AppEnv, c.contactID)
	if status := c.Call(t, "PUT", "/v1/mail-drafts", AnyMap{
		"anchor_type": "activity", "anchor_id": otherAnchor,
		"to": []string{"subject@consent.test"}, "subject": "Re: Separate inbound question", "body": "Unsent work.",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("saving a draft answered %d", status)
	}
	var scheduled struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
		"to": []string{"subject@consent.test"}, "subject": "Re: Later", "body": "Scheduled before the rollback.",
		"consent_purpose": "marketing_email",
		"scheduled_at":    clocktest.Now(t).Add(2 * time.Hour).UTC().Format(time.RFC3339), "scheduled_tz": "Europe/Berlin",
	}, nil, &scheduled); status != http.StatusCreated {
		t.Fatalf("scheduling a send answered %d", status)
	}

	refused := AnyMap{
		"to": []string{"subject@consent.test"}, "subject": "Re: Inbound question", "body": "The offer you did not ask for.",
		"consent_purpose": "marketing_email",
	}
	var problem struct {
		Details struct {
			AvailableActions []string `json:"available_actions"`
		} `json:"details"`
	}
	if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", refused, nil, &problem); status != http.StatusConflict {
		t.Fatalf("the inline Send answered %d, want the 409 that opens a review", status)
	}
	if !slices.Equal(problem.Details.AvailableActions, []string{"direct_send"}) {
		t.Fatalf("the refusal offers %v, want Send anyway", problem.Details.AvailableActions)
	}
	if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", refused, nil, nil); status != http.StatusConflict {
		t.Fatalf("pressing Send again answered %d, want the same refusal", status)
	}
	if got := sendSeries(t, inlineReview) - reviewsBefore; got != 1 {
		t.Errorf("one first Send and one repeat press counted %d inline reviews, want 1", got)
	}
	if got := sendSeries(t, refusedTimed) - timedBefore; got != 2 {
		t.Errorf("the engine decided two presses and the histogram timed %d", got)
	}
	review := liveReviewID(t, c)

	setInlineSendReview(t, c, false)
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("the legacy dialog could not read the review: %d", status)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("the legacy Send anyway answered %d, want 201", status)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), nil, nil); status == http.StatusCreated {
		t.Fatal("a second Send anyway sent the message again")
	}
	var deliveries int
	if err := c.Owner.QueryRow(context.Background(), `SELECT count(*) FROM comms_outbound`).Scan(&deliveries); err != nil {
		t.Fatal(err)
	}
	if deliveries != 1 {
		t.Fatalf("the reviewed message was staged %d times, want once", deliveries)
	}
	if got := sendSeries(t, legacySent) - sentBefore; got != 1 {
		t.Errorf("the legacy confirmation counted %d sent, want 1", got)
	}
	if got := sendSeries(t, inlineQueued) - queuedBefore; got != 0 {
		t.Errorf("a confirmation counted as %d first Sends", got)
	}

	for what, path := range map[string]string{
		"the review":         "/v1/communication-reviews/" + review,
		"the draft":          "/v1/mail-drafts?anchor_type=activity&anchor_id=" + otherAnchor,
		"the scheduled send": "/v1/scheduled-sends/" + scheduled.ID,
	} {
		if status := c.Call(t, "GET", path, nil, nil, nil); status != http.StatusOK {
			t.Errorf("%s is not readable after the rollback: %d", what, status)
		}
	}
	for _, entity := range []string{"communication_instruction", "scheduled_send"} {
		var page struct {
			Data []struct {
				Action string `json:"action"`
			} `json:"data"`
		}
		if status := c.Call(t, "GET", "/v1/audit-log?entity_type="+entity, nil, nil, &page); status != http.StatusOK || len(page.Data) == 0 {
			t.Errorf("the %s audit is not readable after the rollback: %d, %d entries", entity, status, len(page.Data))
		}
	}
}
