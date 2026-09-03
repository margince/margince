// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser and the server must agree on how many tags one LIST ROW carries,
// or the chip strip's "+N" counts a number nobody has.
//
// The store caps the tags it attaches per row, because a record somebody tagged
// forty times would otherwise turn one page of fifty rows into thousands of
// joined rows. The browser draws two words and counts the rest — and if it
// counts them off that CAPPED array while believing the array is whole, a
// record with forty tags is drawn as "two, and three more". The reader is told
// a number, which is worse than being told nothing: it reads as the answer.
//
// So the TypeScript constant is a deliberate mirror rather than a second
// answer. This gate compares both directions: a cap raised on the server and
// not in the browser fails, and so does the reverse.

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

const frontendRowTags = "../frontend/src/design-system/rowtags.tsx"

// The mirrored declaration, matched by NAME rather than by position: a gate
// that read "the first number in the file" would follow an unrelated edit into
// agreeing with whatever it found.
var tsRowTagCap = regexp.MustCompile(`const WIRE_ROW_TAG_CAP\s*=\s*(\d+)\b`)

func TestFrontendMirrorsRowTagCap(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(frontendRowTags)
	if err != nil {
		t.Fatalf("read %s: %v", frontendRowTags, err)
	}
	found := tsRowTagCap.FindSubmatch(source)
	if found == nil {
		t.Fatalf("%s declares no WIRE_ROW_TAG_CAP: the browser has stopped mirroring the server's row tag cap, so the chip strip's \"+N\" can count off a capped array", frontendRowTags)
	}
	mirrored, err := strconv.Atoi(string(found[1]))
	if err != nil {
		t.Fatalf("WIRE_ROW_TAG_CAP in %s is not a number: %v", frontendRowTags, err)
	}
	if mirrored != storekit.RowTagCap {
		t.Fatalf("row tag cap disagrees: server %d, %s %d — a row page carries the server's number, so the strip counts the rest off an array that stopped early",
			storekit.RowTagCap, frontendRowTags, mirrored)
	}
}
