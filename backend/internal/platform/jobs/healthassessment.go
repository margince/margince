// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import "time"

// HealthAssessment distinguishes current queue state from completion evidence lost to retention.
type HealthAssessment struct {
	Condition   string
	Code        string
	Urgency     int
	Count       int64
	Expectation time.Duration
}

// AssessHealth uses declared cadence and actual retry state instead of an assumed timeout.
func AssessHealth(k KindHealth) HealthAssessment {
	out := HealthAssessment{Condition: "unknown", Code: "queue_checked", Urgency: 2}
	if spec, ok := SpecFor(k.Kind); ok && spec.Cadence.Fixed > 0 {
		// Missing a full declared tick while still runnable warrants investigation.
		out.Expectation = spec.Cadence.Fixed
	}
	switch {
	case out.Expectation > 0 && k.OldestWaitingAgeSeconds != nil && *k.OldestWaitingAgeSeconds > out.Expectation.Seconds():
		out.Condition, out.Code, out.Urgency, out.Count = "technical", "queue_overdue", 0, k.Runnable
	case k.Discarded > 0:
		out.Condition, out.Code, out.Urgency, out.Count = "unverified", "queue_unverified", 2, k.Discarded
	case k.OldestWaitingAgeSeconds != nil && out.Expectation == 0:
		out.Condition, out.Code, out.Urgency, out.Count = "unknown", "queue_expectation_unknown", 2, k.Runnable
	case k.FutureRetries > 0 && k.NextAttemptAt != nil:
		out.Condition, out.Code, out.Urgency, out.Count = "retrying", "queue_retrying", 3, k.FutureRetries
	case k.Waiting+k.Running+k.Retrying > 0:
		out.Condition, out.Code, out.Count = "processing", "queue_processing", k.Waiting+k.Running+k.Retrying
	}
	return out
}
