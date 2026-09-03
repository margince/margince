// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgdossier

// Both writers here ask through ai.Ask, so a reply their own parse refuses goes
// back to the model rather than dropping the reader to the deterministic floor.
//
// The assertion is on ReAsking.Refused, and that is the point of these two
// tests: an ordinary fake never calls the validator, so a site that handed the
// lane a looser check than its own parse would pass every existing test here.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai/aitest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestTheGrowthFitAssessmentReAsksThroughItsOwnParse(t *testing.T) {
	t.Parallel()
	in := sevenOfSeven()
	lane := &aitest.ReAsking{
		First:  "Strong fit, I'd say — they look like a good match.",
		Second: citing("strong", in),
	}
	assessment, err := assessWithModel(t.Context(), lane, in, false, at, string(textlang.English))
	if err != nil {
		t.Fatalf("assessing: %v", err)
	}
	if lane.Refused == nil {
		t.Fatal("the site accepted a reply ParseGrowthFit refuses, so its validator is looser than its own read")
	}
	if lane.Bare != 0 {
		t.Errorf("the bare lane was taken %d time(s) on a lane that can re-ask", lane.Bare)
	}
	if len(assessment.Claims.PositiveFactors) == 0 {
		t.Fatal("the re-asked assessment carried no factor, so the second reply never landed")
	}
}

func TestTheDossierWriterReAsksThroughItsOwnParse(t *testing.T) {
	t.Parallel()
	in := sevenOfSeven()
	lane := &aitest.ReAsking{
		First:  "Here is what I found about them.",
		Second: describing("summary", "They build load-shifting software for industrial sites.", in),
	}
	sections, err := writeWithModel(t.Context(), lane, in, string(textlang.English))
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
	if lane.Refused == nil {
		t.Fatal("the site accepted a reply ParseDossier refuses, so its validator is looser than its own read")
	}
	if len(sections) == 0 {
		t.Fatal("the re-asked dossier carried no section, so the second reply never landed")
	}
}
