// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/webread"
)

// minRateExtractConfidence is the floor below which a model-stated rate is not
// staged: a low-confidence reading is a guess, and a proposal is a claim.
const minRateExtractConfidence = 0.5

// pageFetcher is the webread seam (production passes webread.New(); tests stub
// it, since webread's SSRF guard rightly refuses loopback test servers).
type pageFetcher interface {
	Fetch(ctx context.Context, rawURL string) (webread.Doc, error)
}

// CountPassages reports how many passages numberPassages would emit for text.
//
// The rule is stated ONCE, here, beside the numbering it has to agree with: the
// probe reports a passage count in two places, and a second copy of "non-empty
// lines" would drift from the production rule the moment either changed.
// TestCountPassagesAgreesWithTheNumbering holds the two together.
func CountPassages(text string) int {
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// numberPassages prefixes each non-empty line with a passage id ([s0], [s1], …)
// — the format the aicert corpus grounds against, so the model can cite an id.
// The page text is numbered, not edited: the caller wraps it in a nonce
// boundary the page's author has never seen, so a forged marker in the page is
// inert and the numbered passages still read exactly as published (a bad
// extraction only ever STAGES a proposal a human must approve).
func numberPassages(text string) string {
	var b strings.Builder
	n := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fmt.Fprintf(&b, "[s%d] %s\n", n, line)
		n++
	}
	return b.String()
}
