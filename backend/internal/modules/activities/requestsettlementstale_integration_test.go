// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"
)

// A verdict is reached over the answers read before the model call. A newer
// answer that lands before the commit was never read. So the verdict is
// dropped, and the request stays eligible for a pass that reads it.
func TestASettledVerdictOverStaleAnswersIsDropped(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	request := seedEmailRequest(t, e, "Two answers", "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "I will check.", requestInstant.Add(time.Hour))
	mintReminder(t, e, requestInstant.Add(2*time.Hour))
	asOf := requestInstant.Add(48 * time.Hour)
	candidate, ok := repliedIDs(t, e, asOf)[request]
	if !ok || candidate.TaskID == nil {
		t.Fatal("the answered request with a reminder was not offered")
	}
	// Between the read and the write: our second reply, which the verdict
	// below never saw.
	replyTo(t, e, request, "Actually, one part is still open.", requestInstant.Add(24*time.Hour))
	if err := store.SettleRequest(asClassifier(e), RequestSettlementInput{
		Request: candidate, Verdict: RequestSettled, Confidence: 0.95, DecidedBy: "test-model",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, done, _ := taskFor(t, e, request); done {
		t.Fatal("a verdict over answers that were no longer the newest completed the reminder")
	}
	var judged int
	if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity_request_settlement WHERE request_activity_id = $1`, request).Scan(&judged); err != nil {
		t.Fatal(err)
	}
	if judged != 0 {
		t.Fatal("a stale verdict was recorded as the request's judgement")
	}
	again, ok := repliedIDs(t, e, asOf)[request]
	if !ok || again.NewestAnswerID == candidate.NewestAnswerID {
		t.Fatalf("the request is not offered again over its newest answer: %+v", again)
	}
}
