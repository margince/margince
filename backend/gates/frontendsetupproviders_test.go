// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

//go:build !integration

package gates

// Onboarding offers a first-time admin a provider and a model, and the server
// has to be able to price and serve exactly what it offered.
//
// The frontend cannot read Go, so `frontend/src/screens/setup-providers.ts` is a
// declared MIRROR — and this is what makes "mirror" true rather than "two lists
// that happen to agree today". It compares both directions: a model the screen
// offers that SeedModelRates does not price fails, and so does a provider name
// SelectBrain would not accept.
//
// An unpriced model is not a crash, which is why it needs a gate: the binding
// works, and every call it serves reports UNPRICED instead of a cost. Nobody
// notices until someone opens the usage report and finds the first week of
// their installation missing from it.

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
)

const setupProvidersFile = "../frontend/src/screens/setup-providers.ts"

// setupOffer is one entry of the screen's table, as this gate reads it.
type setupOffer struct {
	id, provider, chatModel, embedModel string
}

// providerEntry matches one preset block. Anchored on the fields rather than on
// the whole literal so reformatting the file — biome moving a brace — does not
// silently stop the gate matching, which is the one failure a mirror must not
// have.
var providerEntry = regexp.MustCompile(
	`(?s)(\w+):\s*\{\s*label:[^}]*?provider:\s*"([^"]+)"[^}]*?chatModel:\s*"([^"]+)"[^}]*?embedModel:\s*"([^"]+)"`)

func readSetupOffers(t *testing.T) []setupOffer {
	t.Helper()
	raw, err := os.ReadFile(setupProvidersFile)
	if err != nil {
		t.Fatalf("read %s: %v", setupProvidersFile, err)
	}
	// Only the table, never the doc comment above it: the prose names models as
	// examples, and matching those would have this gate check sentences.
	body := string(raw)
	if i := strings.Index(body, "const PRESETS"); i >= 0 {
		body = body[i:]
	}
	matches := providerEntry.FindAllStringSubmatch(body, -1)
	offers := make([]setupOffer, 0, len(matches))
	for _, m := range matches {
		offers = append(offers, setupOffer{id: m[1], provider: m[2], chatModel: m[3], embedModel: m[4]})
	}
	// NOT a tolerated zero: the screen offers providers, so an empty read means
	// the shape moved and this gate reports PASS having compared nothing.
	if len(offers) == 0 {
		t.Fatalf("no provider presets parsed from %s — the mirror has gone blind", setupProvidersFile)
	}
	return offers
}

// gateSeedDay pins the day this file hands SeedModelRates. Nothing here asserts
// on a date; a real clock would only give the gate a way to differ between runs.
var gateSeedDay = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

// seedSheet is the seeded price sheet keyed the way this gate asks about it:
// what lane, if any, "provider/model" is filed under.
func seedSheet() map[string]ai.Lane {
	sheet := map[string]ai.Lane{}
	for _, r := range ai.SeedModelRates(gateSeedDay) {
		sheet[r.Provider+"/"+r.ModelID] = r.Lane
	}
	return sheet
}

func TestOnboardingOffersOnlyModelsTheServerPrices(t *testing.T) {
	t.Parallel()
	sheet := seedSheet()
	for _, o := range readSetupOffers(t) {
		for lane, model := range map[ai.Lane]string{ai.LaneChat: o.chatModel, ai.LaneEmbeddings: o.embedModel} {
			filed, priced := sheet[o.provider+"/"+model]
			if !priced {
				t.Errorf("onboarding offers %s as %s's %s lane, and SeedModelRates does not price %s/%s — "+
					"every call an admin makes in their first week would report UNPRICED",
					model, o.id, lane, o.provider, model)
				continue
			}
			// Priced is half the question. The sheet is also the CATALOGUE the
			// routing form's picker reads, so a model filed under the other lane
			// is offered on every screen except the one that needs it — and the
			// preset that seeded it serves a lane it cannot answer.
			if filed != lane {
				t.Errorf("onboarding seeds %s as %s's %s model, and the sheet files %s/%s under %q",
					model, o.id, lane, o.provider, model, filed)
			}
		}
	}
}

// A provider onboarding offers must be able to fill BOTH fields from the sheet,
// or its picker opens empty on the one screen a first-time admin cannot skip —
// which reads as a broken installation rather than as a list nobody wrote.
//
// Derived from the presets and the sheet rather than listed here: a provider
// added to onboarding is asked the same question without anyone remembering to
// come back and add it.
func TestEveryOfferedProviderCanFillBothLanesFromTheSheet(t *testing.T) {
	t.Parallel()
	lanes := map[string]map[ai.Lane]int{}
	for _, r := range ai.SeedModelRates(gateSeedDay) {
		if lanes[r.Provider] == nil {
			lanes[r.Provider] = map[ai.Lane]int{}
		}
		lanes[r.Provider][r.Lane]++
	}
	for _, o := range readSetupOffers(t) {
		for _, lane := range []ai.Lane{ai.LaneChat, ai.LaneEmbeddings} {
			if lanes[o.provider][lane] == 0 {
				t.Errorf("onboarding offers %s on provider %q, and the sheet prices no %s model for it — "+
					"that field's picker opens with nothing in it",
					o.id, o.provider, lane)
			}
		}
	}
}

func TestOnboardingOffersOnlyProvidersTheServerServes(t *testing.T) {
	t.Parallel()
	known := map[string]bool{}
	for _, p := range ai.KnownProviders() {
		known[p] = true
	}
	for _, o := range readSetupOffers(t) {
		if !known[o.provider] {
			t.Errorf("onboarding offers provider %q for %s, which SelectBrain does not serve — "+
				"the first write would be refused with a message about an adapter the reader never named",
				o.provider, o.id)
		}
	}
}
