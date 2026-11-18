// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package sendmetrics

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func humanCtx() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
	})
}

func agentCtx() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:drafter", PassportID: ids.NewV7(), OnBehalfOf: ids.NewV7(),
	})
}

func switchedTo(on bool) func(context.Context) (bool, error) {
	return func(context.Context) (bool, error) { return on, nil }
}

// refusedSend is a staging refusal as the HTTP door returns it: the review it
// opened, the actions its caller may take and the refused recipients.
func refusedSend(category commsauthz.Category, actions ...string) error {
	return &consent.SendRefusedError{
		ReviewID: ids.NewV7(),
		Cause:    fmt.Errorf("consent: 1 of 1 recipients may not be written to: %w", apperrors.ErrConsentNotGranted),
		Actions:  actions,
		Refusals: []consent.RefusedRecipient{{
			Address: "anna.private@customer.test", ReasonCode: commsauthz.ReasonNoEvidence, Category: string(category),
		}},
	}
}

func scrape(c *collector) string {
	var b strings.Builder
	c.writePrometheus(&b)
	return b.String()
}

func sample(t *testing.T, scraped, series string) string {
	t.Helper()
	for line := range strings.SplitSeq(scraped, "\n") {
		if value, ok := strings.CutPrefix(line, series+" "); ok {
			return value
		}
	}
	t.Fatalf("no series %s in\n%s", series, scraped)
	return ""
}

func TestEachFirstSendCountsUnderItsFlowAndOutcome(t *testing.T) {
	c := newCollector()
	c.observeSend(humanCtx(), switchedTo(true), activities.SendOutcome{}, nil)
	c.observeSend(humanCtx(), switchedTo(false), activities.SendOutcome{Scheduled: &activities.ScheduledSend{}}, nil)
	c.observeSend(humanCtx(), switchedTo(true), activities.SendOutcome{}, refusedSend(commsauthz.CategoryMarketing, consent.ActionDirectSend))
	c.observeSend(humanCtx(), switchedTo(true), activities.SendOutcome{}, refusedSend(commsauthz.CategoryReplyToInbound))

	got := scrape(c)
	for series, want := range map[string]string{
		`margince_email_send_outcomes_total{flow="inline",outcome="queued"}`:    "1",
		`margince_email_send_outcomes_total{flow="legacy",outcome="scheduled"}`: "1",
		`margince_email_send_outcomes_total{flow="inline",outcome="review"}`:    "1",
		`margince_email_send_outcomes_total{flow="inline",outcome="repair"}`:    "1",
		`margince_email_send_outcomes_total{flow="legacy",outcome="queued"}`:    "0",
		`margince_email_send_reviews_total{flow="inline",context="outreach"}`:   "1",
		`margince_email_send_reviews_total{flow="inline",context="routine"}`:    "1",
		`margince_email_send_reviews_total{flow="legacy",context="routine"}`:    "0",
	} {
		if value := sample(t, got, series); value != want {
			t.Errorf("%s = %s, want %s", series, value, want)
		}
	}
}

// A refusal whose review could not be recorded is still a repair outcome, and
// opened no review to count.
func TestARefusalWithoutAReviewCountsAsRepairOnly(t *testing.T) {
	c := newCollector()
	c.observeSend(humanCtx(), switchedTo(false), activities.SendOutcome{}, fmt.Errorf("refused: %w", apperrors.ErrConsentNotGranted))

	got := scrape(c)
	if value := sample(t, got, `margince_email_send_outcomes_total{flow="legacy",outcome="repair"}`); value != "1" {
		t.Errorf("repair = %s, want 1", value)
	}
	for _, class := range contexts {
		if value := sample(t, got, `margince_email_send_reviews_total{flow="legacy",context="`+class+`"}`); value != "0" {
			t.Errorf("a refusal with no review counted a %s review: %s", class, value)
		}
	}
}

func TestSendsThatAreNoFirstHumanSendAreNotCounted(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ctx    context.Context
		inline func(context.Context) (bool, error)
		err    error
	}{
		{name: "an agent's send", ctx: agentCtx(), inline: switchedTo(true)},
		{name: "no principal", ctx: context.Background(), inline: switchedTo(true)},
		{name: "a fault that is no outcome", ctx: humanCtx(), inline: switchedTo(true), err: &activities.RequiredFieldError{Field: "subject"}},
		{name: "an unreadable switch", ctx: humanCtx(), inline: func(context.Context) (bool, error) {
			return false, errors.New("settings store unreachable")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newCollector()
			c.observeSend(tc.ctx, tc.inline, activities.SendOutcome{}, tc.err)
			for k, n := range c.outcomes {
				if n != 0 {
					t.Errorf("%+v counted %d", k, n)
				}
			}
		})
	}
}

func TestTheDecisionHistogramTimesOnlyAHumansFreshSend(t *testing.T) {
	c := newCollector()
	shared, c = c, shared
	t.Cleanup(func() { shared = c })

	ObserveDecision(humanCtx(), ids.UUID{}, 300*time.Millisecond, true)
	ObserveDecision(humanCtx(), ids.UUID{}, 2*time.Second, false)
	ObserveDecision(humanCtx(), ids.NewV7(), time.Second, true)
	ObserveDecision(agentCtx(), ids.UUID{}, time.Second, true)

	got := scrape(shared)
	for series, want := range map[string]string{
		`margince_email_send_decision_seconds_count{verdict="allowed"}`:            "1",
		`margince_email_send_decision_seconds_bucket{verdict="allowed",le="0.5"}`:  "1",
		`margince_email_send_decision_seconds_bucket{verdict="allowed",le="0.25"}`: "0",
		`margince_email_send_decision_seconds_count{verdict="refused"}`:            "1",
		`margince_email_send_decision_seconds_bucket{verdict="refused",le="1"}`:    "0",
	} {
		if value := sample(t, got, series); value != want {
			t.Errorf("%s = %s, want %s", series, value, want)
		}
	}
}

func TestEachSendAnywayConfirmationCountsWhatItCameTo(t *testing.T) {
	c := newCollector()
	c.observeSendAnyway(consent.InlineWarningVersion, nil)
	c.observeSendAnyway(consent.OverrideWarningVersion, nil)
	c.observeSendAnyway(consent.InlineWarningVersion, &consent.InstructionStaleError{Why: "the email warnings changed"})
	c.observeSendAnyway(consent.OverrideWarningVersion, &consent.ValidationError{Field: "warning_version", Reason: "not served"})
	c.observeSendAnyway(consent.InlineWarningVersion, refusedSend(commsauthz.CategoryMarketing))
	c.observeSendAnyway(consent.InlineWarningVersion, errors.New("connection reset"))
	c.observeSendAnyway(consent.InlineWarningVersion, fmt.Errorf("mail provider: %w", apperrors.ErrProviderUnusable))

	got := scrape(c)
	for series, want := range map[string]string{
		`margince_email_send_anyway_total{flow="inline",result="sent"}`:           "1",
		`margince_email_send_anyway_total{flow="legacy",result="sent"}`:           "1",
		`margince_email_send_anyway_total{flow="inline",result="stale_revision"}`: "1",
		`margince_email_send_anyway_total{flow="legacy",result="wrong_warning"}`:  "1",
		`margince_email_send_anyway_total{flow="inline",result="refused"}`:        "1",
	} {
		if value := sample(t, got, series); value != want {
			t.Errorf("%s = %s, want %s (a server fault must not count)", series, value, want)
		}
	}
}

// Every label value a scrape carries comes from a closed set, and nothing the
// sender typed or the engine stored about a contact reaches the exposition.
func TestEveryLabelValueIsFromAClosedSet(t *testing.T) {
	c := newCollector()
	c.observeSend(humanCtx(), switchedTo(true), activities.SendOutcome{}, refusedSend(commsauthz.CategoryMarketing, consent.ActionDirectSend))
	c.observeSend(humanCtx(), switchedTo(false), activities.SendOutcome{}, nil)
	c.observeDecision(true, 40*time.Millisecond)
	c.observeSendAnyway("override-v99 Anna asked by phone", &consent.InstructionStaleError{Why: "anna.private@customer.test"})
	got := scrape(c)

	closed := map[string][]string{
		"flow": flows, "outcome": outcomes, "context": contexts, "verdict": verdicts, "result": confirms,
	}
	pairs := regexp.MustCompile(`([a-z]+)="([^"]*)"`)
	seen := 0
	for line := range strings.SplitSeq(got, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "margince_email_send_") {
			t.Errorf("a line outside the send families: %q", line)
		}
		for _, m := range pairs.FindAllStringSubmatch(line, -1) {
			seen++
			if m[1] == "le" {
				continue
			}
			allowed, known := closed[m[1]]
			if !known || !slices.Contains(allowed, m[2]) {
				t.Errorf("label %s=%q is not from a closed set: %q", m[1], m[2], line)
			}
		}
	}
	if seen == 0 {
		t.Fatal("the scrape carried no labels, so this test checked nothing")
	}
	for _, private := range []string{"anna", "customer.test", "phone", "v99"} {
		if strings.Contains(got, private) {
			t.Errorf("the scrape carries %q", private)
		}
	}
}
