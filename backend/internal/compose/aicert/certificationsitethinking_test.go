// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// A site the contract moves off the binding's thinking level is certified at
// its own level, and the document must file its record under the label the
// report prints for that row, or the page loses the site the census certified.
func TestASiteWithItsOwnThinkingLevelKeepsItsRecordOnThePage(t *testing.T) {
	rec := aicert.Record{
		Task: string(ai.TaskColdStart), Provider: "gemini", ServedModel: "gemini-3.1-flash-lite",
		EnvClass: "cloud_frontier", SiteThinking: map[string]string{"sitereadmessage": "low"},
	}
	thinks := aicert.ReadinessRow{
		Site:   aitasks.Site{Task: ai.TaskColdStart, Variant: "sitereadmessage"},
		Record: rec, Certified: true, Standing: aicert.Standing{Measured: 2, Total: 2},
	}
	plain := thinks
	plain.Site = aitasks.Site{Task: ai.TaskColdStart, Variant: "acts"}

	const want = "gemini · gemini-3.1-flash-lite · cloud_frontier · thinking low"
	if got := thinks.Binding(); got != want {
		t.Fatalf("the report labels the thinking site %q, want %q", got, want)
	}
	doc := aiCertDoc{Sites: []aiCertSite{
		{Key: thinks.SiteKey(), Records: []aiCertRecord{buildAICertRecord(thinks, 2)}},
		{Key: plain.SiteKey(), Records: []aiCertRecord{buildAICertRecord(plain, 2)}},
	}}
	assertAICertCoverageMatchesTheLibrary(t, doc, []aicert.ReadinessRow{thinks, plain})
}
