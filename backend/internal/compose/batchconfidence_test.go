// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// One answer reporting 90 for "90%" loses only itself, at every batched verdict
// site: the reply validates and the other verdicts survive the read-back.
func TestOneOutOfRangeConfidenceDropsOnlyThatAnswerAtEveryBatchedSite(t *testing.T) {
	t.Parallel()
	bad, good := schema.Confidence(90), schema.Confidence(0.9)

	t.Run("capture_classify", func(t *testing.T) {
		t.Parallel()
		batch := []unlabeledMessage{{ID: ids.NewV7()}, {ID: ids.NewV7()}, {ID: ids.NewV7()}}
		payload := classifyPayload{Results: []classifyResult{
			{ID: "m1", Label: "other", Confidence: good},
			{ID: "m2", Label: "other", Confidence: bad},
			{ID: "m3", Label: "commitment", Confidence: good},
		}}
		if msg := validateClassifyPayload(payload, batch); msg != "" {
			t.Fatalf("a reply with one bad confidence was refused whole: %s", msg)
		}
		got := resolveClassifyRefs(payload.Results, batch)
		if len(got) != 2 || got[0].ID != batch[0].ID.String() || got[1].ID != batch[2].ID.String() {
			t.Fatalf("kept %+v, want the first and third answers only", got)
		}
	})

	t.Run("owed_verdict", func(t *testing.T) {
		t.Parallel()
		batch := []owedCandidate{{ID: ids.NewV7()}, {ID: ids.NewV7()}, {ID: ids.NewV7()}}
		payload := owedPayload{Results: []owedResult{
			{ID: "m1", Verdict: activities.OwedVerdictAsksUs, Confidence: good},
			{ID: "m2", Verdict: activities.OwedVerdictAsksUs, Confidence: bad},
			{ID: "m3", Verdict: activities.OwedVerdictInformsUs, Confidence: good},
		}}
		if msg := validateOwedPayload(payload, batch); msg != "" {
			t.Fatalf("a reply with one bad confidence was refused whole: %s", msg)
		}
		got := resolveOwedResults(payload, batch)
		if len(got) != 2 || got[0].ID != batch[0].ID.String() || got[1].ID != batch[2].ID.String() {
			t.Fatalf("kept %+v, want the first and third answers only", got)
		}
	})

	t.Run("request_settlement", func(t *testing.T) {
		t.Parallel()
		batch := []settleCandidate{
			{Request: activities.RepliedRequest{RequestID: ids.NewV7()}},
			{Request: activities.RepliedRequest{RequestID: ids.NewV7()}},
			{Request: activities.RepliedRequest{RequestID: ids.NewV7()}},
		}
		payload := settlePayload{Results: []settleResult{
			{ID: "r1", Verdict: activities.RequestUnsure, Confidence: good},
			{ID: "r2", Verdict: activities.RequestUnsure, Confidence: bad},
			{ID: "r3", Verdict: activities.RequestUnsure, Confidence: good},
		}}
		if msg := validateSettlePayload(payload, batch); msg != "" {
			t.Fatalf("a reply with one bad confidence was refused whole: %s", msg)
		}
		got := resolveSettleResults(payload, settleRefs(batch))
		if len(got) != 2 || got[0].ID != batch[0].Request.RequestID.String() || got[1].ID != batch[2].Request.RequestID.String() {
			t.Fatalf("kept %+v, want the first and third answers only", got)
		}
	})
}
