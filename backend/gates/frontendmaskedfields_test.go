// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A field the server can withhold is drawn as WITHHELD, or the screen states
// something false.
//
// The wire says it twice: the value comes back null and `masked_fields` names
// the field. A client reading only the null draws an empty cell, and an empty
// cell is a claim — this deal is unpriced, this partner agreed no tier. That is
// the opposite of what happened, and it is worse than drawing nothing at all
// because it reads as an answer.
//
// So the screens are a declared mirror of the catalog rather than a second
// answer, and this compares both directions: a pair the catalog gains with no
// cell testing for it fails, and so does a cell testing for a field the server
// never withholds — a refusal drawn for a fact nobody hides.
//
// Row-withheld objects are outside the subject, and the catalog is what keeps
// them out. A commission entry answers the partner's mask by leaving the ROW
// out of the read, because its rate and amount are required integers with no
// null to send, so there is no cell to draw and no pair to offer;
// TestNoCrossObjectConsequenceIsAlsoOfferedForConfiguration is what holds those
// pairs out of the catalog this gate reads.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const maskedFieldScreens = "../frontend/src"

// maskedGuardRender is the shared control that says withheld rather than
// absent, in either spelling: a cell that is only ever a refusal, and one whose
// mode turns on what the payload said.
var maskedGuardRender = regexp.MustCompile(`<FieldGuard\b[^>]*mode=[^>]*"masked"`)

// withheldMessage is the same refusal where no element fits — a read-only
// reason, a picker's option label, a group rendered as one string — and a
// shared token either way, which is what keeps it from being a screen's own
// words.
var withheldMessage = regexp.MustCompile(`t\("record\.notShown"\)`)

// maskDrawing is one DECLARATION's answer about one field: the screen it sits
// in, and whether that same declaration states the refusal. Per declaration
// and not per file, because a component drawing one withheld field says
// nothing about the next field down the same screen.
type maskDrawing struct {
	file    string
	refuses bool
}

// drawnWithoutOffer ratifies a field the screens test for and the catalog does
// not offer. Empty, and the emptiness is the finding: no screen draws a refusal
// for a fact this build cannot withhold.
var drawnWithoutOffer = gatekit.Waive(map[string]string{})

// offeredWithoutCell ratifies a catalog pair no screen tests for. Empty, and
// the emptiness is the finding: every pair an administrator can configure
// reaches a reader as a refusal rather than as a blank.
var offeredWithoutCell = gatekit.Waive(map[string]string{})

func TestEveryMaskableFieldIsDrawnAsWithheld(t *testing.T) {
	t.Parallel()
	drawn := maskedFieldsDrawn(t)
	offered := make(map[string]bool)
	for _, pair := range catalogPairs(t) {
		offered[pair.field] = true
	}
	for field := range offered {
		if len(drawn[field]) > 0 || offeredWithoutCell.Waived(t, field) {
			continue
		}
		t.Errorf("%s offers a mask on %q and no screen under %s tests masked_fields for it: the "+
			"value arrives null and is drawn as an empty cell, which says the record holds no such "+
			"fact rather than that this reader may not have it", maskableCatalog, field, maskedFieldScreens)
	}
	for field, drawings := range drawn {
		if offered[field] || drawnWithoutOffer.Waived(t, field) {
			continue
		}
		t.Errorf("%s draws a refusal for %q and %s offers no mask naming it, so the branch is a "+
			"promise nothing keeps: either the catalog stopped offering the pair and the screen "+
			"still claims it, or the field name is misspelt and the real refusal is drawn as a "+
			"blank", screensOf(drawings), field, maskableCatalog)
	}
	for field, drawings := range drawn {
		if !offered[field] || anyRefuses(drawings) {
			continue
		}
		t.Errorf("%q is tested for in %s and no declaration testing for it draws the refusal "+
			"itself: a screen that knows the field is withheld and draws its own words for it is a "+
			"second spelling of the refusal, and one that draws the shared one for a NEIGHBOURING "+
			"field leaves this one blank", field, screensOf(drawings))
	}
	offeredWithoutCell.AssertAllMatched(t)
	drawnWithoutOffer.AssertAllMatched(t)
}

// drawsWithheld reports whether this declaration states the refusal, in either
// of the shared spellings the product has for it.
func drawsWithheld(declaration string) bool {
	return maskedGuardRender.MatchString(declaration) || withheldMessage.MatchString(declaration)
}

// anyRefuses reports whether one of the declarations testing for a field draws
// the refusal, which is the association this mirror holds: the neighbours of a
// declaration answer for their own fields and not for this one.
func anyRefuses(drawings []maskDrawing) bool {
	return slices.ContainsFunc(drawings, func(drawing maskDrawing) bool { return drawing.refuses })
}

// screensOf is the files these drawings sit in, for a report that says where to
// look.
func screensOf(drawings []maskDrawing) string {
	var files []string
	for _, drawing := range drawings {
		if !slices.Contains(files, drawing.file) {
			files = append(files, drawing.file)
		}
	}
	return strings.Join(files, ", ")
}

// maskedFieldsDrawn maps each field the screens test a record's masked_fields
// for to the declarations testing it, refusing a walk that reads nothing: a
// scan finding no screen at all sweeps the same tree and agrees with every
// catalog there could be.
func maskedFieldsDrawn(t *testing.T) map[string][]maskDrawing {
	t.Helper()
	drawn, err := maskedFieldsDrawnUnder(maskedFieldScreens)
	if err != nil {
		t.Fatalf("reading the screens that draw a refusal: %v", err)
	}
	return drawn
}

func maskedFieldsDrawnUnder(root string) (map[string][]maskDrawing, error) {
	drawn := make(map[string][]maskDrawing)
	walkErr := fs.WalkDir(os.DirFS(root), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !isMaskDrawingSource(entry.Name()) {
			return err
		}
		source, readErr := os.ReadFile(filepath.Join(root, path)) // #nosec G304 -- a path from walking the source tree
		if readErr != nil {
			return readErr
		}
		for _, declaration := range declarationsIn(string(source)) {
			drawing := maskDrawing{file: path, refuses: drawsWithheld(declaration)}
			for _, field := range maskedFieldTests(declaration) {
				drawn[field] = append(drawn[field], drawing)
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walking %s: %w", root, walkErr)
	}
	if len(drawn) == 0 {
		return nil, fmt.Errorf("no source under %s tests a record's masked_fields, so either the "+
			"client stopped drawing refusals or this scan stopped reading them", root)
	}
	for _, drawings := range drawn {
		slices.SortStableFunc(drawings, func(a, b maskDrawing) int { return strings.Compare(a.file, b.file) })
	}
	return drawn, nil
}

// declarationsIn is the top-level declarations of one source, which is the unit
// a field's test and its refusal have to share. The tree is formatted with
// every declaration opening in the first column and its body indented, so a
// line in the first column opens the next one — a source this splits too finely
// reports a refusal it cannot see rather than one it invented.
func declarationsIn(source string) []string {
	var declarations []string
	current := strings.Builder{}
	for _, line := range strings.Split(tsComment.ReplaceAllString(source, " "), "\n") {
		if current.Len() > 0 && line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			declarations = append(declarations, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	return append(declarations, current.String())
}

// isMaskDrawingSource keeps tests and stories out: both BUILD a masked payload
// rather than deciding about one, so a fixture naming a field the server never
// withholds is the input to what is under test.
func isMaskDrawingSource(name string) bool {
	if strings.Contains(name, ".test.") || strings.Contains(name, ".stories.") {
		return false
	}
	return strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".tsx")
}

var (
	// maskedAnchor is where a masked-fields expression starts: the wire list, or
	// the binding a screen reads it into.
	maskedAnchor = regexp.MustCompile(`\bmasked(_fields)?\b`)
	// membershipTest is the question asked of that list. The array form is the
	// second spelling — `[…].includes(key)` over the list's own entries — and
	// reading only the first would drop the deal's currency and ARR, which are
	// tested exactly that way.
	membershipTest = regexp.MustCompile(`\.includes\(`)
	// maskedFieldLiteral is a field as the wire spells one, which keeps a window's
	// other literals out: a type guard's "string" is not a field.
	maskedFieldLiteral = regexp.MustCompile(`"([a-z][a-z0-9]*(?:_[a-z0-9]+)*)"`)
)

// maskedFieldTests is the field names one source asks a record's masked_fields
// about.
//
// It reads EXPRESSIONS rather than lines: each anchor is followed to the close
// of the call it sits in, so a literal further down the file cannot be read as
// a field this screen tests for, and a test spread over five lines still is.
func maskedFieldTests(source string) []string {
	body := tsComment.ReplaceAllString(source, " ")
	var fields []string
	for _, anchor := range maskedAnchor.FindAllStringIndex(body, -1) {
		for _, field := range fieldsAskedIn(balancedWindow(body[anchor[0]:])) {
			if !slices.Contains(fields, field) {
				fields = append(fields, field)
			}
		}
	}
	slices.Sort(fields)
	return fields
}

// fieldsAskedIn is the names a membership test in this window asks about: the
// literal arguments, and the literals of an array whose membership is asked.
func fieldsAskedIn(window string) []string {
	var fields []string
	for _, test := range membershipTest.FindAllStringIndex(window, -1) {
		asked := balancedWindow(window[test[1]-1:])
		if receiver := strings.TrimRight(window[:test[0]], " \t\n"); strings.HasSuffix(receiver, "]") {
			asked += openingBracketed(receiver)
		}
		for _, name := range maskedFieldLiteral.FindAllStringSubmatch(asked, -1) {
			fields = append(fields, name[1])
		}
	}
	return fields
}

// maskedWindowCap bounds a window that never closes — a bracket inside a string
// literal the comment strip did not reach. Well past the longest real test.
const maskedWindowCap = 800

// balancedWindow is the expression beginning at the head of text: everything up
// to the close of the call or subscript it opens, or to the end of the
// statement when it opens none.
func balancedWindow(text string) string {
	depth := 0
	for i, r := range text {
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth <= 0 {
				return text[:i+1]
			}
		case ';':
			if depth == 0 {
				return text[:i]
			}
		}
		if i >= maskedWindowCap {
			return text[:i]
		}
	}
	return text
}

// openingBracketed is the array literal ending at the close of text, read back
// from its closing bracket so a nested one inside it is not mistaken for the
// start.
func openingBracketed(text string) string {
	depth := 0
	for i := len(text) - 1; i >= 0; i-- {
		switch text[i] {
		case ']', ')', '}':
			depth++
		case '[', '(', '{':
			depth--
			if depth == 0 {
				return text[i:]
			}
		}
	}
	return ""
}

// TestTheMaskedCellScanReadsTheSpellingsTheScreensUse holds the direction this
// mirror may not fail in. A scan that stopped recognising a form would report
// the screens drawing no refusal for a field, which reads as a clean sweep of a
// smaller tree; the strangers are the other half, since a scan admitting
// anything would satisfy the catalog with literals that test nothing.
func TestTheMaskedCellScanReadsTheSpellingsTheScreensUse(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "the wire list asked directly",
			source: `if (deal.masked_fields?.includes("amount_minor")) return <FieldGuard mode="masked" />;`,
			want:   []string{"amount_minor"},
		},
		{
			name:   "a binding the screen reads it into",
			source: "const masked = deal.masked_fields ?? [];\nif (masked.includes(\"margin_tier\")) draw();",
			want:   []string{"margin_tier"},
		},
		{
			name:   "membership asked of an array, over several lines",
			source: "maskedFields={masked\n  .filter((key) =>\n    [\"expected_arr_minor\", \"currency\"].includes(key),\n  )\n  .map((key) => (key === \"expected_arr\" ? \"arr\" : key))}",
			want:   []string{"currency", "expected_arr_minor"},
		},
		{
			name:   "a type guard beside the list",
			source: `record.masked_fields.filter((key): key is string => typeof key === "string")`,
		},
		{
			name:   "a commented-out test",
			source: "// deal.masked_fields?.includes(\"amount_minor\")\nconst masked = [];",
		},
		{
			name:   "a literal further down the file",
			source: "const masked = deal.masked_fields ?? [];\nconst stage = stages.find((s) => s.id === \"stage_id\");",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := maskedFieldTests(c.source); !slices.Equal(got, c.want) {
				t.Fatalf("read %v, want %v", got, c.want)
			}
		})
	}
}

// TestTheMaskedCellMirrorRefusesAScreenTreeItReadsNothingFrom holds the
// direction the walk may not fail in. The catalog half of the corpus is refused
// by the reader the mask gates share, which
// TestTheCrossingCensusRefusesACorpusItCannotSweep drives.
func TestTheMaskedCellMirrorRefusesAScreenTreeItReadsNothingFrom(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := maskedFieldsDrawnUnder(root); err == nil {
		t.Fatal("an empty screen tree swept clean rather than being refused")
	}
	// Tests and stories are not screens: a tree holding only fixtures has no
	// refusal drawn in it, and reading one as a screen would satisfy the
	// catalog with a payload nobody renders.
	fixture := `deal.masked_fields?.includes("amount_minor")`
	if err := os.WriteFile(filepath.Join(root, "deals.test.tsx"), []byte(fixture), 0o600); err != nil {
		t.Fatalf("writing the fixture screen: %v", err)
	}
	if _, err := maskedFieldsDrawnUnder(root); err == nil {
		t.Fatal("a tree of test fixtures alone read as screens drawing a refusal")
	}
	if err := os.WriteFile(filepath.Join(root, "deals.tsx"), []byte(fixture), 0o600); err != nil {
		t.Fatalf("writing the fixture screen: %v", err)
	}
	drawn, err := maskedFieldsDrawnUnder(root)
	if err != nil {
		t.Fatalf("reading the fixture screen: %v", err)
	}
	if screensOf(drawn["amount_minor"]) != "deals.tsx" {
		t.Fatalf("the fixture screen read as %v — a screen dropped here is a field this mirror "+
			"stops holding", drawn)
	}
}

// TestTheRefusalIsReadPerDeclarationAndNotPerScreen plants the shape this
// mirror could not see: one component drawing its own field through the shared
// control vouched for every other field named anywhere in the same file, so a
// screen could name a withheld field and draw its own words for it.
func TestTheRefusalIsReadPerDeclarationAndNotPerScreen(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	screen := `function CompanyCell({ deal }) {
  if (deal.masked_fields?.includes("company_id")) {
    return <FieldGuard mode="masked" />;
  }
}

function MarginTierRow({ partner }) {
  if (partner.masked_fields?.includes("margin_tier")) {
    return <span>no tier for you</span>;
  }
}
`
	if err := os.WriteFile(filepath.Join(root, "partners.tsx"), []byte(screen), 0o600); err != nil {
		t.Fatalf("writing the fixture screen: %v", err)
	}
	drawn, err := maskedFieldsDrawnUnder(root)
	if err != nil {
		t.Fatalf("reading the fixture screen: %v", err)
	}
	if !anyRefuses(drawn["company_id"]) {
		t.Fatal("the declaration rendering the shared control read as drawing no refusal")
	}
	if anyRefuses(drawn["margin_tier"]) {
		t.Fatal("a field drawn in the screen's own words was vouched for by its neighbour's control")
	}
}
