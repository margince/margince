// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// One domain fold. owndomain.Fold is how a mail domain is compared, and every
// other reader asks it (mailidentity.NormalizeDomain, freemail's normalize).
// Five folds once answered the same question differently. A unicode and a
// punycode spelling of one domain matched in one check and not the next.
// A file that imports golang.org/x/net/idna is writing its own fold, so it
// must be the fold itself or say here why it is not one.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const idnaImportPath = "golang.org/x/net/idna"

var idnaReadersAdmitted = gatekit.Waive(map[string]string{
	"internal/platform/owndomain/owndomain.go":   "is the fold itself, and the Spellings that a SQL comparison against stored header addresses needs because Postgres cannot fold IDNA. Cost: none, every other domain fold delegates here",
	"internal/modules/capture/owndomainstore.go": "validates an own domain before it is stored and refuses one IDNA cannot read, where Fold keeps it. It stores the same ASCII form Fold gives a readable domain. Cost: a second IDNA call, kept so a typo is refused rather than stored as protection that never fires",
	"internal/modules/capture/strangermail.go":   "adds the IDNA spelling of a sender for the stranger lookup and the Unicode spelling for the stored-counterparty repair. It is a comparison, not display. Routing it through mailidentity.AddressSpellings waits on one IDNA-folded correspondence lock (#9083), because widening the lookup first lets a punycode inbound and a Unicode outbound miss each other's lock. Cost: a second spelling helper until then",
	"internal/platform/freemail/displayname.go":  "renders a domain for display with idna.ToUnicode; it never compares. Cost: none for matching, a display-only conversion",
})

func TestOnlyTheSharedFoldReadsIDNA(t *testing.T) {
	t.Parallel()
	defer idnaReadersAdmitted.AssertAllMatched(t)
	for file, imports := range goImports(t, "internal") {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, path := range imports {
			if path != idnaImportPath {
				continue
			}
			rel := filepath.ToSlash(file)
			if idnaReadersAdmitted.Waived(t, rel) {
				continue
			}
			t.Errorf("%s imports %s: fold domains through owndomain.Fold (or mailidentity.NormalizeDomain / FoldAddress) instead of a fold of its own", rel, idnaImportPath)
		}
	}
}
