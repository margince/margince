// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

func TestSignalExtractReportsTheConfidenceOfItsCommitmentsOnly(t *testing.T) {
	prepared, err := signalExtractCases{}.Prepare(
		json.RawMessage(`[{"direction":"inbound","body":"a"},{"direction":"outbound","body":"b"}]`),
		json.RawMessage(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := prepared.(*signalExtractCase)
	if !ok {
		t.Fatalf("Prepare returned %T, want *signalExtractCase", prepared)
	}
	first, second := c.thread.Messages[0].ID, c.thread.Messages[1].ID
	reply := `{"events":[
		{"kind":"new_opportunity","message_id":"` + first.String() + `","summary":"s","confidence":0.99},
		{"kind":"commitment_made","message_id":"` + second.String() + `","summary":"s","confidence":0.6},
		{"kind":"commitment_made","message_id":"` + second.String() + `","summary":"t","confidence":0.9}]}`

	low, high, reported := c.AnswerConfidence(aitasks.Trace{Output: reply})

	if !reported || low != 0.6 || high != 0.9 {
		t.Fatalf("got (%v, %v, %v), want the commitments' own range (0.6, 0.9, true)", low, high, reported)
	}
}

func TestTranscriptProposeReportsNoConfidenceForAnEmptyOrUnparseableReply(t *testing.T) {
	c := &transcriptProposeCase{lines: []string{"a: x"}}

	_, _, emptyReported := c.AnswerConfidence(aitasks.Trace{Output: `{"proposals":[]}`})
	_, _, garbledReported := c.AnswerConfidence(aitasks.Trace{Output: `not json`})
	low, high, reported := c.AnswerConfidence(aitasks.Trace{Output: `{"proposals":[{"summary":"s","owner":"a","source_lines":[1],"confidence":0.8}]}`})

	if emptyReported || garbledReported {
		t.Fatalf("an empty or unparseable reply reported a confidence (empty=%v, garbled=%v)", emptyReported, garbledReported)
	}
	if !reported || low != 0.8 || high != 0.8 {
		t.Fatalf("got (%v, %v, %v), want (0.8, 0.8, true)", low, high, reported)
	}
}
