// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// gen-composition writes the index of downloaded extension units, and the
// frontend notice plugin and the backend notice script each read it. The path
// is typed in all three languages, and a reader that looks somewhere else sees
// no index and, for a plain checkout, no units.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var (
	goFetchedUnitsDir   = regexp.MustCompile(`fetchedUnitsDir\s*=\s*"([^"]+)"`)
	goFetchedUnitsIndex = regexp.MustCompile(`fetchedUnitsIndex\s*=\s*fetchedUnitsDir\s*\+\s*"([^"]+)"`)
)

func TestEveryReaderOfTheFetchedUnitsIndexNamesTheComposersPath(t *testing.T) {
	t.Parallel()
	composer, err := gatekit.ReadFile("tools/gen-composition/moduleunits.go")
	if err != nil {
		t.Fatalf("reading the composer: %v", err)
	}
	dir, suffix := goFetchedUnitsDir.FindSubmatch(composer), goFetchedUnitsIndex.FindSubmatch(composer)
	if dir == nil || suffix == nil {
		t.Fatal("the composer no longer declares fetchedUnitsDir and fetchedUnitsIndex in the form this gate reads")
	}
	path := string(dir[1]) + string(suffix[1])
	for _, reader := range []string{"../frontend/scripts/vite-licenses.ts", "../scripts/backend-notices.sh"} {
		body, err := gatekit.ReadFile(reader)
		if err != nil {
			t.Fatalf("reading %s: %v", reader, err)
		}
		if !strings.Contains(string(body), path) {
			t.Errorf("%s does not name the composer's index path %q", reader, path)
		}
	}
}
