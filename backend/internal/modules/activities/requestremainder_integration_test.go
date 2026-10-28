// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// settlementRow is what the verdict table holds for one request.
func settlementRow(t *testing.T, e *loadEnv, request ids.UUID) (verdict string, remaining *string, rule string) {
	t.Helper()
	if err := e.owner.QueryRow(e.as(), `SELECT verdict, remaining, rule_version
		  FROM activity_request_settlement WHERE request_activity_id = $1`,
		request).Scan(&verdict, &remaining, &rule); err != nil {
		t.Fatalf("reading the settlement: %v", err)
	}
	return verdict, remaining, rule
}

// The request asks for a report. Our reply sends feedback and asks them a
// question. A remaining text that is our question is work for them, so it must
// never retitle our reminder; a quote of their own ask does.
func TestOnlyAQuoteOfTheirRequestRetitlesTheReminder(t *testing.T) {
	for name, tc := range map[string]struct {
		remaining, wantSubject string
		wantRemaining          bool
	}{
		"our question":       {"Which prospects did you speak to?", "Prospects", false},
		"their original ask": {"send the report", "send the report", true},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupLoad(t)
			request := seedEmailRequest(t, e, "Prospects", "commitment", OwedVerdictAsksUs)
			replyTo(t, e, request, "The setup was clear. Which prospects did you speak to?", requestInstant.Add(time.Hour))
			mintReminder(t, e, requestInstant.Add(2*time.Hour))

			candidate := repliedIDs(t, e, requestInstant.Add(48*time.Hour))[request]
			if !candidate.TaskMachineMinted {
				t.Fatal("the reminder the owed pass filed does not read as the machine's to retitle")
			}
			if err := storeKnowing(e).SettleRequest(asClassifier(e), RequestSettlementInput{
				Request: candidate, Verdict: RequestStillOwed, Remaining: tc.remaining,
				Confidence: 0.9, DecidedBy: "test-model",
			}); err != nil {
				t.Fatal(err)
			}

			if _, subject, _, _ := taskFor(t, e, request); subject != tc.wantSubject {
				t.Errorf("the reminder is titled %q, want %q", subject, tc.wantSubject)
			}
			verdict, remaining, rule := settlementRow(t, e, request)
			if verdict != RequestStillOwed || rule != RequestSettlementRule {
				t.Errorf("recorded %q under rule %q, want still_owed under %q", verdict, rule, RequestSettlementRule)
			}
			if got := remaining != nil; got != tc.wantRemaining {
				t.Errorf("remaining recorded = %v, want %v", got, tc.wantRemaining)
			}
		})
	}
}

// A still_owed verdict reached before the remaining text had to quote their
// request may name our own question as work. It is judged again, and the
// reminder it retitled can be retitled once more. A settled verdict from the
// same time is not judged again for this rule.
func TestAStillOwedVerdictFromAnOlderRuleIsJudgedAgain(t *testing.T) {
	for name, tc := range map[string]struct {
		verdict, remaining string
		offered            bool
	}{
		"still owed": {RequestStillOwed, "send the report", true},
		"unsure":     {RequestUnsure, "", false},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupLoad(t)
			request := seedEmailRequest(t, e, "Older rule", "commitment", OwedVerdictAsksUs)
			replyTo(t, e, request, "I will check.", requestInstant.Add(time.Hour))
			mintReminder(t, e, requestInstant.Add(2*time.Hour))
			asOf := requestInstant.Add(48 * time.Hour)
			if err := storeKnowing(e).SettleRequest(asClassifier(e), RequestSettlementInput{
				Request: repliedIDs(t, e, asOf)[request], Verdict: tc.verdict, Remaining: tc.remaining,
				Confidence: 0.9, DecidedBy: "test-model",
			}); err != nil {
				t.Fatal(err)
			}
			if _, ok := repliedIDs(t, e, asOf)[request]; ok {
				t.Fatal("a verdict under the current rule was offered again with nothing new to read")
			}

			// The column's default is what every verdict written before it holds.
			e.exec(t, `UPDATE activity_request_settlement SET rule_version = DEFAULT WHERE request_activity_id = $1`, request)
			candidate, ok := repliedIDs(t, e, asOf)[request]
			if ok != tc.offered {
				t.Fatalf("offered again = %v, want %v", ok, tc.offered)
			}
			if tc.offered && !candidate.TaskMachineMinted {
				t.Error("the reminder an older verdict retitled no longer reads as the machine's to retitle")
			}
		})
	}
}
