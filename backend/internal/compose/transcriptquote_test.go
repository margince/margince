// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// A quote the claim writer files must be words the transcript holds. Lines the
// model cited across a gap are not, so only the first adjacent run is quoted.
func TestAQuoteAcrossACitedGapKeepsTheFirstAdjacentRun(t *testing.T) {
	t.Parallel()

	lines := strings.Split("one\ntwo\nthree\nfour\nfive\nsix\nseven", "\n")
	body := strings.Join(lines, "\n")
	for name, cited := range map[string][]int{
		"a gap":               {3, 7},
		"out of order":        {7, 3},
		"a run then a gap":    {2, 3, 6},
		"a repeated line":     {3, 3},
		"one line":            {5},
		"a run out of order":  {4, 3, 2},
		"adjacent from start": {1, 2},
	} {
		quote := quotedFromTranscript(proposedStep{SourceLines: cited}, lines)
		if !values.Quoted(body, quote) {
			t.Errorf("%s: quote %q is not in the transcript", name, quote)
		}
	}
	if got := quotedFromTranscript(proposedStep{SourceLines: []int{3, 7}}, lines); got != "three" {
		t.Errorf("lines 3 and 7 quote %q, want only line 3", got)
	}
}
