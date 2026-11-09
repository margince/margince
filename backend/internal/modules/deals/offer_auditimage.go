// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What an offer write records about its money and its lines in the audit row.

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// offerTotalsChange is what recomputeOfferTotals wrote to the offer's three
// money columns: what the row held, and what it now holds. One derivation
// answers both audit images, so no caller sums a total a second time.
type offerTotalsChange struct{ Before, After OfferFigures }

// The three money columns an offer's totals image carries, so the image and the
// statement that writes them spell each one the same way.
const (
	offerKeyNet   = "net_minor"
	offerKeyTax   = "tax_minor"
	offerKeyGross = "gross_minor"
)

// offerTotalsImage renders one side of that transition as an audit image,
// keyed by the column names. Field history then reads three money fields of
// the offer moving.
func offerTotalsImage(f OfferFigures) map[string]any {
	return map[string]any{offerKeyNet: f.NetMinor, offerKeyTax: f.TaxMinor, offerKeyGross: f.GrossMinor}
}

// lineFieldEvidence is the context a line write adds to the offer update it
// causes: which line moved, and what its fields held on either side. It is
// evidence, never an image. A key in an offer's image reads as a field of the
// offer, so a line's description there would read as the offer's own.
//
//craft:ignore naked-any a line image carries whatever SQL types its columns hold, exactly as the audit seam takes them
func lineFieldEvidence(lineID ids.UUID, was, now map[string]any) map[string]any {
	return map[string]any{lineIDKey: lineID, "line_before": was, "line_after": now}
}
