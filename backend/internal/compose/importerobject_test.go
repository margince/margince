// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The importer door and the import surface must name the same object.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
)

// auth.DeclaredImporter spells "import_run" as a literal, because platform/auth
// may not import a module and migration owns the constant. csvimport.go takes
// the grant through migration.ImportRunObject.
//
// If the object were renamed, csvimport.go would follow the constant and the
// door would go on asking about an object nobody is granted — refusing every
// import, with nothing failing to say why. This is the only place both
// spellings are visible at once, which is why the pin lives here.
func TestTheImporterDoorNamesTheObjectTheImportSurfaceTakes(t *testing.T) {
	if auth.ImporterObject() != migration.ImportRunObject {
		t.Errorf("the importer door guards %q but the import surface takes %q — "+
			"the door asks about an object nobody holds, and every import is refused",
			auth.ImporterObject(), migration.ImportRunObject)
	}
}
