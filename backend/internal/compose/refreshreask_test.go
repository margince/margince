// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The two pricing-page extractors ask through ai.Ask, so a reply their own
// parse refuses goes back to the model rather than leaving the sheet unrefreshed
// with a warning nobody reads.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai/aitest"
	"github.com/margince/margince/backend/internal/platform/webread"
)

// pageServing answers a fetch with fixed text: these cases are about the model
// reply, and a real fetcher would make them about the network.
type pageServing struct{ text string }

func (f pageServing) Fetch(context.Context, string) (webread.Doc, error) {
	return webread.Doc{Text: f.text}, nil
}

func TestTheFxExtractorReAsksThroughItsOwnParse(t *testing.T) {
	t.Parallel()
	lane := &aitest.ReAsking{
		// Valid JSON, refused by parseFxExtraction alone: pairs is an object
		// where the site reads a list.
		First:  `{"pairs":{"from_currency":"USD"}}`,
		Second: `{"pairs":[{"from_currency":"USD","to_currency":"EUR","rate":"0.92","evidence":"USD 1 = EUR 0.92"}]}`,
	}
	refresh := fxRefresh{fetcher: pageServing{text: "USD 1 = EUR 0.92"}, brain: lane, url: "https://example.test/fx"}

	pairs, err := refresh.extract(t.Context())
	if err != nil {
		t.Fatalf("extracting: %v", err)
	}
	if lane.Refused == nil {
		t.Fatal("the site accepted a reply parseFxExtraction refuses, so its validator is looser than its own read")
	}
	if lane.Bare != 0 {
		t.Errorf("the bare lane was taken %d time(s) on a lane that can re-ask", lane.Bare)
	}
	if len(pairs) != 1 || pairs[0].ToCurrency != "EUR" {
		t.Fatalf("the re-asked pairs did not come back: %+v", pairs)
	}
}
