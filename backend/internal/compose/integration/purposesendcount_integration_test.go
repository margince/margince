// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The send counts are read from the engine's own staging decisions, so a send
// counts under what the engine judged, through whichever door it came.

type purposeSendCount struct {
	PurposeKey string `json:"purpose_key"`
	Class      string `json:"class"`
	Sends      int    `json:"sends"`
	Disputed   int    `json:"disputed"`
}

type purposeSendCounts struct {
	Data []purposeSendCount `json:"data"`
	Days int                `json:"days"`
}

// replyAs answers a fresh inbound from the subject under a purpose key and a
// claimed kind. Either may be empty. It fails unless the reply is staged.
func (c *consentEnv) replyAs(t *testing.T, purpose, claim string) {
	t.Helper()
	body := AnyMap{"subject": "Re: Inbound question", "body": "Thanks, here is the answer.", "to": []string{"subject@consent.test"}}
	if purpose != "" {
		body["consent_purpose"] = purpose
	}
	if claim != "" {
		body["communication_context"] = claim
	}
	var problem json.RawMessage
	if status := c.Call(t, "POST", "/v1/activities/"+c.capturedInbound(t, "Question", "Could you help?")+"/send-email", body, nil, &problem); status != http.StatusAccepted {
		t.Fatalf("reply under %q claiming %q → %d %s", purpose, claim, status, problem)
	}
}

// Mutations: drop the disputed filter, the window bound, the archived filter
// of the engine's purpose lookup, or the unknown grouping, and this fails.
func TestPurposeSendCountsReadWhatTheEngineJudged(t *testing.T) {
	t.Parallel()
	d := newTwoDoors(t, "purpose-send-counts", "read")
	c := seedConsentSubject(t, d.e)

	// Each reply answers the subject's own thread, so the engine resolves it as
	// a reply whatever the sender claims or names.
	c.replyAs(t, "business_correspondence", "")
	c.replyAs(t, "", "customer_service")               // a claimed kind with no key
	c.replyAs(t, " Transactional", "customer_service") // a key that disagrees with the claim, as typed
	c.replyAs(t, "Spring campaign 2026", "")           // a key no purpose defines
	if _, err := c.Owner.Exec(context.Background(),
		`UPDATE consent_purpose SET archived_at = now() WHERE key = 'marketing_email'`); err != nil {
		t.Fatal(err)
	}
	c.replyAs(t, "marketing_email", "") // a purpose archived before the send

	want := []purposeSendCount{
		{PurposeKey: "unknown", Sends: 3, Disputed: 1},
		{PurposeKey: "business_correspondence", Class: "business_correspondence", Sends: 1},
		{PurposeKey: "transactional", Class: "transactional", Sends: 1, Disputed: 1},
	}
	var counts purposeSendCounts
	raw := d.rest(t, "GET", "/v1/consent-purposes/send-counts?days=30", nil)
	if err := json.Unmarshal(raw, &counts); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(counts.Data, want) || counts.Days != 30 {
		t.Fatalf("send counts = %+v, want %+v", counts, want)
	}
	if strings.Contains(strings.ToLower(string(raw)), "spring") {
		t.Fatalf("the answer echoes a key the sender typed: %s", raw)
	}
	sameRecords[purposeSendCounts](t, "consent purpose send counts", raw,
		d.tool(t, "read_view", map[string]any{"view": "consent_purpose_send_counts", "days": 30}))

	if _, err := c.Owner.Exec(context.Background(),
		`UPDATE communication_decision SET decided_at = now() - interval '40 days' WHERE purpose_key = 'business_correspondence'`); err != nil {
		t.Fatal(err)
	}
	for days, wantSends := range map[int]int{30: 0, 60: 1} {
		var windowed purposeSendCounts
		if err := json.Unmarshal(d.rest(t, "GET", "/v1/consent-purposes/send-counts?days="+strconv.Itoa(days), nil), &windowed); err != nil {
			t.Fatal(err)
		}
		got := 0
		for _, row := range windowed.Data {
			if row.PurposeKey == "business_correspondence" {
				got = row.Sends
			}
		}
		if got != wantSends {
			t.Fatalf("a %d-day window counts %d correspondence sends from 40 days ago, want %d", days, got, wantSends)
		}
	}
}

// Mutation: drop the auth.Require in PurposeSendCounts and the rep reads it.
func TestASeatWithoutTheComplianceGrantCannotReadTheSendCounts(t *testing.T) {
	t.Parallel()
	c := setupConsent(t)
	const email, password = "rep@purpose-send-counts.test", "rep-purpose-send-counts-password"
	c.DescribeCompany(t)
	var created struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/users", AnyMap{"email": email, "display_name": "Rep", "role": "rep"}, nil, &created); status != http.StatusCreated {
		t.Fatalf("adding the rep → %d", status)
	}
	apptest.SetPasswordByLink(t, c.AppEnv, created.ID, password)
	rep := c.AsSecondSeat(t, email, password)
	if status := rep.Call(t, "GET", "/v1/consent-purposes/send-counts", nil, nil, nil); status != http.StatusForbidden {
		t.Fatalf("a rep reading the send counts → %d, want 403", status)
	}
}
