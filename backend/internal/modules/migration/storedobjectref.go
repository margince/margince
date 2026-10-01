// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"time"

	"github.com/margince/margince/backend/internal/platform/storedobject"
)

// ImportSourceObjectKind is the key segment an uploaded import source is
// stored under, beside attachments and logos.
const ImportSourceObjectKind = "import"

// importSourceGrace is a day rather than an hour because the row does not
// follow the put within one request: the file is uploaded first, and the run
// that names it is created only once its columns have been mapped.
const importSourceGrace = 24 * time.Hour

// StoredObjectReference declares the column an import run records its source in.
func StoredObjectReference() storedobject.Reference {
	return storedobject.Reference{
		Kind:    ImportSourceObjectKind,
		Columns: []storedobject.Column{{Table: importRunObject, Name: sourceRefColumn}},
		Grace:   importSourceGrace,
	}
}
