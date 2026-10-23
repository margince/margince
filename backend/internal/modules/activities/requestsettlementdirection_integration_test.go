// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSettlementWriterRefusesOurOutboundQuestionAsRemainingWork(t *testing.T) {
	e := setupLoad(t)
	request := seedEmailRequest(t, e, "Report feedback", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Here is the report. Which prospects did you speak to?", requestInstant.Add(time.Hour))
	mintReminder(t, e, requestInstant.Add(2*time.Hour))
	candidate := repliedIDs(t, e, requestInstant.Add(3*time.Hour))[request]
	err := storeKnowing(e).SettleRequest(asClassifier(e), RequestSettlementInput{Request: candidate, Verdict: RequestStillOwed, Remaining: "Which prospects did you speak to?", Confidence: 1, DecidedBy: "synthetic"})
	if err != nil {
		t.Fatalf("obsolete inference aborted the pass: %v", err)
	}
	_, subject, done, _ := taskFor(t, e, request)
	if subject != "Report feedback" || done {
		t.Fatalf("rejected inference mutated the request: %q, done=%v", subject, done)
	}
	if _, ok := repliedIDs(t, e, requestInstant.Add(3*time.Hour))[request]; !ok {
		t.Fatal("rejected inference advanced the settlement watermark")
	}
}

func TestLegacySettlementIsRecheckedWithoutRewritingHumanWork(t *testing.T) {
	for _, tc := range []struct {
		name    string
		human   bool
		subject string
	}{
		{"automatic", false, "Report feedback"}, {"human held", true, "Report feedback"}, {"no subject", false, ""},
	} {
		human := tc.human
		t.Run(tc.name, func(t *testing.T) {
			e := setupLoad(t)
			request := seedEmailRequest(t, e, tc.subject, "commitment", OwedVerdictAsksUs)
			replyTo(t, e, request, "Here is the report. Which prospects did you speak to?", requestInstant.Add(time.Hour))
			mintReminder(t, e, requestInstant.Add(2*time.Hour))
			asOf := requestInstant.Add(3 * time.Hour)
			candidate := repliedIDs(t, e, asOf)[request]
			task, _, _, _ := taskFor(t, e, request)
			// These rows model the prior writer, whose unconstrained title is no longer accepted.
			legacyDate := requestInstant.Add(24 * time.Hour)
			e.exec(t, `INSERT INTO activity_request_settlement(request_activity_id,judged_through_activity_id,verdict,remaining,confidence,decided_by,due_at) VALUES($1,$2,'still_owed','Answer our prospects question',1,'legacy',$3)`, request, candidate.NewestAnswerID, legacyDate)
			e.exec(t, `UPDATE activity SET subject='Answer our prospects question',due_at=$2 WHERE id=$1`, task, legacyDate)
			if human {
				title := "My chosen follow-up"
				if _, err := storeKnowing(e).UpdateActivity(e.asSeat(e.rep), ids.From[ids.ActivityKind](task), UpdateActivityInput{Subject: &title}); err != nil {
					t.Fatal(err)
				}
			}
			rows, _, err := storeKnowing(e).ListActivities(e.asSeat(e.rep), ListActivitiesInput{Kind: new("task"), AssigneeID: new(ids.From[ids.UserKind](e.rep)), TaskAsOf: &asOf})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range rows {
				if ids.UUID(row.Id) == task {
					found = true
				}
			}
			if found != human {
				t.Fatalf("legacy automatic duty or human adoption misread: visible=%v human=%v", found, human)
			}
			candidate, ok := repliedIDs(t, e, asOf)[request]
			if !ok {
				t.Fatal("legacy verdict was not requeued")
			}
			if err := storeKnowing(e).SettleRequest(asClassifier(e), RequestSettlementInput{Request: candidate, Verdict: RequestUnsure, Confidence: 1, DecidedBy: "synthetic"}); err != nil {
				t.Fatal(err)
			}
			_, title, done, due := taskFor(t, e, request)
			if !human && due != nil {
				t.Fatalf("old model deadline survived repair: %v", due)
			}
			if human && (due == nil || !due.Equal(legacyDate)) {
				t.Fatal("human-held deadline changed")
			}
			want := tc.subject
			if want == "" {
				want = "Reply to email"
			}
			if human {
				want = "My chosen follow-up"
			}
			if title != want || done {
				t.Fatalf("revalidation lost title or silently completed: %q done=%v", title, done)
			}
		})
	}
}
