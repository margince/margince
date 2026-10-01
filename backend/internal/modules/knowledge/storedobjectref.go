// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"time"

	"github.com/margince/margince/backend/internal/platform/storedobject"
)

// knowledgeObjectKind is the key segment an uploaded corpus document's bytes
// are stored under.
const knowledgeObjectKind = "knowledge"

// StoredObjectReference declares the column a corpus document's key is recorded
// in. An hour is far past the handful of statements between the put and the row.
func StoredObjectReference() storedobject.Reference {
	return storedobject.Reference{
		Kind:    knowledgeObjectKind,
		Columns: []storedobject.Column{{Table: "knowledge_document", Name: "storage_key"}},
		Grace:   time.Hour,
	}
}
