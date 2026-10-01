// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"time"

	"github.com/margince/margince/backend/internal/platform/storedobject"
)

// attachmentObjectKind is the key segment every attachment's bytes are stored
// under, by an upload and by a capture alike.
const attachmentObjectKind = "attachment"

// ProvisionalObjectGrace is how long an attachment's key may stay provisional
// before the reap treats it as an orphan.
//
// Generous against the window it has to clear: the transaction between the put
// and the row is a handful of statements, and the capture path holds one open
// across an object-store write bounded at capturedFileStoreTimeout. An hour is
// far past both.
const ProvisionalObjectGrace = time.Hour

// StoredObjectReference declares the one column an attachment's key is
// recorded in, so the reap can tell a provisional attachment from a live one.
func StoredObjectReference() storedobject.Reference {
	return storedobject.Reference{
		Kind:    attachmentObjectKind,
		Columns: []storedobject.Column{{Table: "attachment", Name: "storage_key"}},
		Grace:   ProvisionalObjectGrace,
	}
}
