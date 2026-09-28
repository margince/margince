// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The viewer and the server must fold whitespace by the SAME set, or a citation
// opens a document with nothing marked.
//
// The server locates a claim's quote under `claims.CollapseSpace`, which splits
// on `strings.Fields` and therefore on `unicode.IsSpace`. The viewer marks the
// span under its own spelling. Written the obvious way — `/\s+/` — those two
// sets are not equal: `\s` MISSES U+0085 NEL, which Go folds, and ADDS U+FEFF
// BOM, which Go does not. A quoted span carrying either one normalises to two
// different strings, the viewer finds no match, and the reader is shown the
// right document with the wrong sentence unmarked — or, worse, the line
// fallback marks a neighbouring block as though it were the evidence.
//
// Neither character is exotic in this corpus: NEL arrives in mainframe and
// EBCDIC exports, and a BOM sits at the head of most Windows-authored files,
// which is exactly the first line a handbook's title lives on.
//
// So the TypeScript spells the set out as a literal class rather than `\s`, and
// this gate reads that class and compares it against `unicode.IsSpace` itself.
// The Go side is DERIVED, not copied: there is no table here to fall out of
// date. It fails in both directions — a character Go folds and the class omits,
// and a character the class carries and Go does not.

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

const frontendCollapseSpace = "../frontend/src/design-system/markdown-highlight.ts"

// The class is read as its own literal so a character can only enter the
// comparison by being IN it. Ranges are admitted because the class needs
// U+2000-U+200A, and eleven separate escapes there would be a line nobody
// checks.
var (
	collapseClass = regexp.MustCompile(`(?s)const SERVER_SPACE_CHAR\s*=\s*/\[(.*?)\]/`)
	classAtom     = regexp.MustCompile(`\\u([0-9a-fA-F]{4})-\\u([0-9a-fA-F]{4})|\\u([0-9a-fA-F]{4})|\\([tnvfr])| `)
)

var namedEscape = map[string]rune{"t": '\t', "n": '\n', "v": '\v', "f": '\f', "r": '\r'}

func TestTheViewerFoldsTheSameWhitespaceTheServerDoes(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendCollapseSpace)
	if err != nil {
		t.Fatalf("reading the frontend whitespace class: %v", err)
	}
	found := collapseClass.FindSubmatch(source)
	if found == nil {
		t.Fatalf("%s no longer declares SERVER_SPACE_CHAR as a character class — this gate is reading a shape that is gone, and a gate that reads nothing agrees with everything", frontendCollapseSpace)
	}

	inTS := map[rune]bool{}
	for _, atom := range classAtom.FindAllStringSubmatch(string(found[1]), -1) {
		switch {
		case atom[1] != "":
			for r := hexRune(t, atom[1]); r <= hexRune(t, atom[2]); r++ {
				inTS[r] = true
			}
		case atom[3] != "":
			inTS[hexRune(t, atom[3])] = true
		case atom[4] != "":
			inTS[namedEscape[atom[4]]] = true
		default:
			inTS[' '] = true
		}
	}
	if len(inTS) == 0 {
		t.Fatal("no characters parsed out of the frontend class — under-recognition here reports PASS with nothing compared")
	}

	// The BMP, because that is where every whitespace character Unicode defines
	// lives; a scan to 0x10FFFF would be four hundred times the work to prove
	// the same empty tail.
	var missing, extra []string
	for r := rune(0); r <= 0xFFFF; r++ {
		switch {
		case unicode.IsSpace(r) && !inTS[r]:
			missing = append(missing, fmt.Sprintf("U+%04X", r))
		case !unicode.IsSpace(r) && inTS[r]:
			extra = append(extra, fmt.Sprintf("U+%04X", r))
		}
	}
	if len(missing) > 0 {
		t.Errorf("%s does not fold %s, which claims.CollapseSpace does:\n\ta quote carrying one normalises to two different strings, so the viewer marks nothing where the server found a match", frontendCollapseSpace, strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		t.Errorf("%s folds %s, which claims.CollapseSpace does not:\n\tthe viewer would match a span the server never checked the quote against", frontendCollapseSpace, strings.Join(extra, ", "))
	}
}

func hexRune(t *testing.T, hex string) rune {
	t.Helper()
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		t.Fatalf("%s: %q is not a codepoint", frontendCollapseSpace, hex)
	}
	return rune(n)
}
