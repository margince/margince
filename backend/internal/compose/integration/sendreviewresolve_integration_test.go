// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

const sendMetricsToken = "send-metrics-scrape"

// setupScrapedConsent is setupConsent with a /metrics this test may scrape, so
// the counts are read through the exposition the api serves.
func setupScrapedConsent(t *testing.T) *consentEnv {
	t.Helper()
	e := apptest.SetupAppWithOptions(t, compose.WithOperatorMail(apptest.DiscardingMailer{}), compose.WithMetricsToken(sendMetricsToken))
	apptest.BootstrapWorkspaceSession(t, e, "Send Review", "dpo@fable.test", "Admin")
	return seedConsentSubject(t, e)
}

// scrapeSendMetrics reads the served /metrics.
func scrapeSendMetrics(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, e.TS.URL+"/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+sendMetricsToken)
	resp, err := e.TS.Client().Do(req)
	if err != nil {
		t.Fatalf("scraping /metrics: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing the scrape: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("scraping /metrics answered %d: %v", resp.StatusCode, err)
	}
	return string(body)
}

// sendSeriesSum adds up every series whose name and labels start with prefix.
func sendSeriesSum(t *testing.T, e *apptest.AppEnv, prefix string) int {
	t.Helper()
	sum, found := 0, false
	for line := range strings.SplitSeq(scrapeSendMetrics(t, e), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		fields := strings.Fields(line)
		n, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil {
			t.Fatalf("%q does not end in a count", line)
		}
		sum, found = sum+n, true
	}
	if !found {
		t.Fatalf("/metrics serves no series %s", prefix)
	}
	return sum
}

// sendSeries reads one series off the served /metrics.
func sendSeries(t *testing.T, c *consentEnv, series string) int {
	t.Helper()
	return sendSeriesSum(t, c.AppEnv, series+" ")
}

// A Send the engine refuses opens one review, however often it is pressed. Send
// anyway on that review sends the message once, and the send families count
// one first Send and one confirmation.
func TestARefusedSendResolvesOnceThroughItsReview(t *testing.T) {
	c := setupScrapedConsent(t)
	c.useReviewableMarketingProfile(t)

	const (
		reviewed     = `margince_email_send_outcomes_total{outcome="review"}`
		queued       = `margince_email_send_outcomes_total{outcome="queued"}`
		anywaySent   = `margince_email_send_anyway_total{result="sent"}`
		refusedTimed = `margince_email_send_decision_seconds_count{verdict="refused"}`
	)
	reviewsBefore, queuedBefore := sendSeries(t, c, reviewed), sendSeries(t, c, queued)
	sentBefore, timedBefore := sendSeries(t, c, anywaySent), sendSeries(t, c, refusedTimed)

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
		t.Fatalf("the Send answered %d, want the 409 that opens a review", status)
	}
	if !slices.Equal(problem.Details.AvailableActions, []string{"direct_send"}) {
		t.Fatalf("the refusal offers %v, want Send anyway", problem.Details.AvailableActions)
	}
	if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", refused, nil, nil); status != http.StatusConflict {
		t.Fatalf("pressing Send again answered %d, want the same refusal", status)
	}
	if got := sendSeries(t, c, reviewed) - reviewsBefore; got != 1 {
		t.Errorf("one first Send and one repeat press counted %d reviews, want 1", got)
	}
	if got := sendSeries(t, c, refusedTimed) - timedBefore; got != 2 {
		t.Errorf("the engine decided two presses and the histogram timed %d", got)
	}
	review := liveReviewID(t, c)

	if status := c.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("the review is not readable: %d", status)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("Send anyway answered %d, want 201", status)
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
	if got := sendSeries(t, c, anywaySent) - sentBefore; got != 1 {
		t.Errorf("the confirmation counted %d sent, want 1", got)
	}
	if got := sendSeries(t, c, queued) - queuedBefore; got != 0 {
		t.Errorf("a confirmation counted as %d first Sends", got)
	}
}
