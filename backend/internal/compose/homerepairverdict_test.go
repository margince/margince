// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestRepairDigestUsesTheExecutedTypedScope(t *testing.T) {
	base := datatask.ReviewRequest{RuleVersion: "test", Limit: 1, AsOf: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
	scopes := []string{
		`{"tasks":[{"id":"00000000-0000-0000-0000-000000000001","version":1,"source":"verdict","action":"reclassify_message","reason":"classification_changed","before_verdict":"asks_us","after_verdict":"informs_us"}]}`,
		` { "tasks" : [{"after_verdict":"informs_us","before_verdict":"asks_us","reason":"classification_changed","action":"reclassify_message","source":"verdict","version":1,"id":"00000000-0000-0000-0000-000000000001"}] } `,
		`{"tasks":[{"id":"00000000-0000-0000-0000-000000000001","version":1,"source":"verdict","action":"reclassify_message","reason":"classification_changed","before_verdict":"asks_us","after_verdict":"asks_us","after_verdict":"informs_us"}]}`,
	}
	var expected string
	for _, scope := range scopes {
		base.Scope = json.RawMessage(scope)
		got, err := homeRepairDigest(base)
		if err != nil {
			t.Fatal(err)
		}
		if expected == "" {
			expected = got
		}
		if got != expected {
			t.Fatal("equivalent executed scopes produce different digests")
		}
	}
	base.Scope = json.RawMessage(`{"tasks":[{"id":"00000000-0000-0000-0000-000000000001","version":1,"source":"verdict","action":"reclassify_message","reason":"classification_changed","before_verdict":"asks_us","after_verdict":"informs_us","after_verdict":"asks_us"}]}`)
	changed, err := homeRepairDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	if changed == expected {
		t.Fatal("different executed outcome retained the digest")
	}
}

func TestClassificationReviewDistinguishesMissingChecksFromUncertainty(t *testing.T) {
	candidate := activities.OwedReviewCandidate{Verdict: activities.OwedVerdictAsksUs, Pristine: true, ContextReadable: true, EvidenceComplete: true}
	for _, test := range []struct {
		prediction, reason string
		action             homeRepairAction
	}{
		{"", "classification_not_attempted", repairHumanReview},
		{reviewPredictionUncertain, "classification_uncertain", repairHumanReview},
		{reviewPredictionDeclined, "classification_declined", repairHumanReview},
		{activities.OwedVerdictAsksUs, "classification_current", repairLeaveCurrent},
		{activities.OwedVerdictInformsUs, "classification_changed", repairReclassifyMessage},
	} {
		t.Run(test.reason, func(t *testing.T) {
			result := homeRepairVerdictRef(candidate, test.prediction)
			if result.Reason != test.reason || result.Action != test.action {
				t.Fatalf("result=%+v", result)
			}
			if test.action == repairHumanReview && result.AfterVerdict != "" {
				t.Fatal("unresolved result became an accepted verdict")
			}
		})
	}
}

func TestRepairWireRefusesAnUncountedAction(t *testing.T) {
	review := datatask.ReviewRequest{Scope: json.RawMessage(`{"tasks":[{"action":"unknown"}]}`)}
	if _, err := homeRepairReviewWire(review); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("uncounted review action: %v", err)
	}
}
