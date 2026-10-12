// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What an offer_draft scenario asserts and how a staged draft is held to it: the
// figure each cited line must carry, the refusal of a figure no draft could
// reach, and the disagreements a run reports when a draft misses one.

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
)

// offerDraftExpectedLine is one line as the corpus asserts it: the price the
// staged line must carry and whether the ladder must have grounded it. Both,
// because they are different claims — a scenario pinning only the number would
// pass a draft that guessed it, and price_grounded is what the offer shows a
// human as the difference between an evidenced price and a placeholder.
//
// The number is the unit price, or — when LineNetMinor is set — the line's net,
// quantity × unit price as deals.LineTotals derives it. A conversation that
// agrees "10 hours at 150.00, 1500.00 total" is drafted equally well as ten
// units at 150.00 or one at 1500.00, and only the net is the same in both.
type offerDraftExpectedLine struct {
	UnitPriceMinor int64  `json:"unit_price_minor,omitempty"`
	LineNetMinor   *int64 `json:"line_net_minor,omitempty"`
	PriceGrounded  bool   `json:"price_grounded"`
}

// pinned is the one figure this expectation asserts, with the words a
// disagreement puts before it — none for the unit price every line carries.
func (want offerDraftExpectedLine) pinned() (string, int64) {
	if want.LineNetMinor != nil {
		return "a line net of ", *want.LineNetMinor
	}
	return "", want.UnitPriceMinor
}

// figureOf reads the pinned figure off a staged line: the unit price as staged,
// or the net the totals engine derives from it. A quantity the engine refuses
// is a line no offer can total, so it is reported rather than matched.
func (want offerDraftExpectedLine) figureOf(line deals.StagedOfferLineInput) (int64, error) {
	if want.LineNetMinor == nil {
		return line.UnitPriceMinor, nil
	}
	figures, err := deals.LineTotals(deals.OfferLineInput{
		Quantity: line.Quantity, UnitPriceMinor: line.UnitPriceMinor, DiscountPct: "0", TaxRate: line.TaxRate,
	})
	return figures.NetMinor, err
}

// refuseUnstageableExpectation names an expectation this site's own gate could
// never let a draft satisfy: a citation the context does not carry is dropped on
// every reply, an ungrounded line is priced at zero on every reply, and a
// grounded price is only ever the one the cited evidence states or the one a
// rate-card product in the offer's currency charges. Each would measure nothing
// for as long as it stayed in the corpus. Naming it here costs a parse; finding
// it later costs a paid run.
//
// Sorted so an expectation with two offences names the same one every time.
func refuseUnstageableExpectation(
	want map[string]offerDraftExpectedLine,
	dealContext []dealContextItem,
	catalog []crmcontracts.Product,
	currency string,
) error {
	captured := make(map[string]string, len(dealContext))
	for _, item := range dealContext {
		captured[item.SourceID] = item.Snippet
	}
	for _, sourceID := range slices.Sorted(maps.Keys(want)) {
		line := want[sourceID]
		_, figure := line.pinned()
		snippet, known := captured[sourceID]
		switch {
		case !known:
			return fmt.Errorf(
				"%s: the scenario expects a line citing %q, which the fixture never captures",
				offerDraftSite, sourceID)
		case line.LineNetMinor != nil && line.UnitPriceMinor != 0:
			return fmt.Errorf(
				"%s: the scenario pins both a unit price and a line net for %q, and a line is judged by one",
				offerDraftSite, sourceID)
		case !line.PriceGrounded && figure != 0:
			return fmt.Errorf(
				"%s: the scenario expects %q priced %d and ungrounded, and the ladder prices every ungrounded line at zero",
				offerDraftSite, sourceID, figure)
		case line.PriceGrounded && !groundableFigure(line, snippet, catalog, currency):
			return fmt.Errorf(
				"%s: the scenario expects %q grounded at %d, which neither the cited context states nor any %s "+
					"rate-card product charges", offerDraftSite, sourceID, figure, currency)
		}
	}
	return nil
}

// groundableFigure asks groundablePrice's question of either figure. A line net
// is reachable as one unit at that price, or as whole units of a rate-card
// product whose price divides it.
func groundableFigure(
	want offerDraftExpectedLine, snippet string, catalog []crmcontracts.Product, currency string,
) bool {
	_, figure := want.pinned()
	if groundablePrice(figure, snippet, catalog, currency) {
		return true
	}
	if want.LineNetMinor == nil {
		return false
	}
	for _, product := range catalog {
		if product.Currency == currency && product.UnitPriceMinor > 0 && figure%product.UnitPriceMinor == 0 {
			return true
		}
	}
	return false
}

// groundablePrice asks the ladder's own question of a scenario: is there a rung
// that could reach this price at all? The conversation rung needs the amount
// inside the text a line cites — and a cited snippet is a substring of that
// text, so the price appearing nowhere in the item means it appears in no
// citation of it — and the rate-card rung needs a live product charging it in
// the offer's own currency.
func groundablePrice(priceMinor int64, snippet string, catalog []crmcontracts.Product, currency string) bool {
	if priceEvidencedInSnippet(snippet, priceMinor, currency) {
		return true
	}
	for _, product := range catalog {
		if product.Currency == currency && product.UnitPriceMinor == priceMinor {
			return true
		}
	}
	return false
}

// disagreements names every expected line the staged draft does not carry. All
// of them, not the first: a draft that priced one line right and two wrong is
// not the near miss one line would read as.
//
// A line is identified by the context item it cites, and ANY staged line citing
// that item can satisfy the scenario — every staged line reaches the offer, so
// there is no first-wins rule to mirror here.
//
// Sorted so a run with two disagreements names them in the same order every time.
func (c *offerDraftCase) disagreements(staged []deals.StagedOfferLineInput) []string {
	var out []string
	for _, sourceID := range slices.Sorted(maps.Keys(c.expected)) {
		want := c.expected[sourceID]
		label, wantFigure := want.pinned()
		var priced []string
		satisfied := false
		for _, line := range staged {
			if line.Evidence.SourceID != sourceID {
				continue
			}
			figure, err := want.figureOf(line)
			switch {
			case err != nil:
				priced = append(priced, fmt.Sprintf("with no line net (%v)", err))
			case figure == wantFigure && line.PriceGrounded == want.PriceGrounded:
				satisfied = true
			default:
				priced = append(priced, offerPriceLine(label, figure, line.PriceGrounded))
			}
		}
		expect := offerPriceLine(label, wantFigure, want.PriceGrounded)
		switch {
		case satisfied:
		case len(priced) == 0:
			out = append(out, fmt.Sprintf("no staged line cites %q, which the scenario expects priced %s",
				sourceID, expect))
		default:
			out = append(out, fmt.Sprintf("the line citing %q is priced %s where the scenario expects %s",
				sourceID, strings.Join(priced, " and "), expect))
		}
	}
	return out
}

// offerPriceLine renders a price for a human reading a disagreement, in the
// minor units the offer stores and the ladder's own two words for where the
// number came from.
func offerPriceLine(label string, priceMinor int64, grounded bool) string {
	if grounded {
		return fmt.Sprintf("%s%d minor units, grounded", label, priceMinor)
	}
	return fmt.Sprintf("%s%d minor units, ungrounded", label, priceMinor)
}
