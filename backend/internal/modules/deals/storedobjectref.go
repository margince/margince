// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"time"

	"github.com/margince/margince/backend/internal/platform/storedobject"
)

// offerPDFObjectKind is the key segment a rendered offer PDF is stored under.
const offerPDFObjectKind = "offer_pdf"

// StoredObjectReference declares the column an offer records its rendered PDF
// in. An hour is far past the one transaction between the put and the row.
func StoredObjectReference() storedobject.Reference {
	return storedobject.Reference{
		Kind:    offerPDFObjectKind,
		Columns: []storedobject.Column{{Table: "offer", Name: "pdf_asset_ref"}},
		Grace:   time.Hour,
	}
}
