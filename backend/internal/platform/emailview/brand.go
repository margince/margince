// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package emailview

import _ "embed"

// brandMark is frontend/public/icon.svg drawn at 84px, three times the size
// the letterhead shows it, since mail clients do not render SVG.
//
//go:embed margince-mark.png
var brandMark []byte

// BrandMarkContentID names the Margince mark as an inline part. A send that
// attaches BrandMark under it sets Letterhead.MarkContentID to match.
const BrandMarkContentID = "margince-mark@margince"

// BrandMark is the Margince mark as a PNG, for the send to attach.
func BrandMark() []byte { return brandMark }

// UsesBrandMark reports whether the letterhead speaks as Margince, and so
// draws the Margince mark when the send carries it.
func (h Letterhead) UsesBrandMark() bool {
	return h.LogoContentID == "" && letterheadName(h) == "Margince"
}
