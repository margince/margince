// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTheStagePromptNamesSpansByRefAndAClaimReadsBack(t *testing.T) {
	req := stageEvidenceRequest(evidenceCriteria, evidenceSpans, "en")
	for _, span := range evidenceSpans {
		if strings.Contains(req.Messages[0].Content+string(req.ResponseSchema), span.SourceID) {
			t.Fatalf("the request shows the span id %s", span.SourceID)
		}
	}
	claim := groundedClaim()
	claim.SourceID = "s1"
	read := readStageEvidenceRefs([]stageEvidenceClaim{claim}, evidenceSpans)
	if read[0].SourceID != evidenceSpans[0].SourceID {
		t.Errorf("s1 read back as %q, want %q", read[0].SourceID, evidenceSpans[0].SourceID)
	}
}

func TestAStageClaimCitingAnUnshownRefOrARawSpanIDIsRefused(t *testing.T) {
	valid := stageEvidenceValid(evidenceCriteria, evidenceSpans)
	for _, cited := range []string{"s9", evidenceSpans[0].SourceID} {
		claim := groundedClaim()
		claim.SourceID = cited
		raw, err := json.Marshal(stageEvidencePayload{Claims: &[]stageEvidenceClaim{claim}})
		if err != nil {
			t.Fatal(err)
		}
		if valid(string(raw)) == nil {
			t.Errorf("a claim citing %q was accepted", cited)
		}
	}
}
