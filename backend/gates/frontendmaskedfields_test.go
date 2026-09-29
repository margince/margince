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

// maskDrawing is one BRANCH's answer about one field: the screen it sits in,
// and whether the code deciding that field's cell states the refusal. Per
// branch and not per declaration, because a component drawing one withheld
// field through the shared control says nothing about the next field it tests
// for in its own body.
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
		t.Errorf("%q is tested for in %s and no branch testing for it draws the refusal itself: a "+
			"screen that knows the field is withheld and draws its own words for it is a second "+
			"spelling of the refusal, and one that draws the shared one for a NEIGHBOURING field "+
			"leaves this one blank", field, screensOf(drawings))
	}
	offeredWithoutCell.AssertAllMatched(t)
	drawnWithoutOffer.AssertAllMatched(t)
}

// drawsWithheld reports whether this stretch of a screen states the refusal,
// in either of the shared spellings the product has for it.
func drawsWithheld(branch string) bool {
	return maskedGuardRender.MatchString(branch) || withheldMessage.MatchString(branch)
}

// anyRefuses reports whether one of the branches testing for a field draws the
// refusal, which is the association this mirror holds: the branches beside one
// answer for their own fields and not for this one.
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
			for _, test := range maskedFieldTests(declaration) {
				drawn[test.field] = append(drawn[test.field],
					maskDrawing{file: path, refuses: drawsWithheld(test.branch)})
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
	// ifCondition is the keyword a bracket group opens the condition of, whose
	// consequent is the cell that condition decides.
	ifCondition = regexp.MustCompile(`\bif\s*$`)
	// conditionOperator is what a test the screen BRANCHES on is followed by.
	// Optional chaining and a default are neither, and both sit exactly there.
	conditionOperator = regexp.MustCompile(`^(\?[^.?]|&&|\|\|)`)
)

// maskedFieldTest is one field a screen asks a record's masked_fields about,
// and the code answering for it.
type maskedFieldTest struct {
	field  string
	branch string
}

// maskedFieldTests is the fields one source asks a record's masked_fields
// about, each with the branch drawing its cell.
//
// It reads EXPRESSIONS rather than lines: each anchor is followed to the close
// of the call it sits in, so a literal further down the file cannot be read as
// a field this screen tests for, and a test spread over five lines still is.
func maskedFieldTests(source string) []maskedFieldTest {
	body := tsComment.ReplaceAllString(source, " ")
	var tests []maskedFieldTest
	for _, anchor := range maskedAnchor.FindAllStringIndex(body, -1) {
		window := balancedWindow(body[anchor[0]:])
		branch := branchDeciding(body, anchor[0], window)
		for _, field := range fieldsAskedIn(window) {
			test := maskedFieldTest{field: field, branch: branch}
			if !slices.Contains(tests, test) {
				tests = append(tests, test)
			}
		}
	}
	slices.SortStableFunc(tests, func(a, b maskedFieldTest) int { return strings.Compare(a.field, b.field) })
	return tests
}

// branchDeciding is the code answering for the field tested at anchor: the
// conditional whose test names it, or the whole declaration where the test is
// not a condition at all — a screen handing the withheld list on to a control
// asks one question for every key in it, and the control drawing them is not
// in this declaration to read.
//
// It only ever NARROWS what a refusal is read from, so a shape it cannot place
// falls back to the declaration: the widest reading, and the one a field drawn
// in a screen's own words beside a guarded neighbour escapes through.
func branchDeciding(declaration string, anchor int, window string) string {
	if branch, ok := ifBranchAround(declaration, anchor); ok {
		return branch
	}
	rest := declaration[anchor+len(window):]
	if conditionOperator.MatchString(strings.TrimLeft(rest, " \t\n")) {
		return window + consequentAfter(rest)
	}
	return declaration
}

// ifBranchAround is the if statement whose condition holds an index: the
// condition and the cell it guards, where the index sits in one at all.
func ifBranchAround(declaration string, at int) (string, bool) {
	opener, condition, ok := enclosingGroup(declaration, at)
	if !ok || declaration[opener] != '(' || !ifCondition.MatchString(declaration[:opener]) {
		return "", false
	}
	return condition + consequentAfter(declaration[opener+len(condition):]), true
}

// enclosingGroup is the innermost bracket group holding an index: where it
// opens, and the text from there to its close. Absent where the index sits at
// the declaration's own level, and absent where the group never closes — a
// bracket inside a string literal miscounts the depth, and a branch read short
// is a refusal this mirror stops seeing.
func enclosingGroup(declaration string, at int) (int, string, bool) {
	var open []int
	for i, r := range declaration[:at] {
		switch r {
		case '(', '[', '{':
			open = append(open, i)
		case ')', ']', '}':
			// A closer with nothing open shuts a group above this text: a
			// declaration opens at the first line in the first column, which is
			// the close of a signature wherever one is spread over several lines.
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
		}
	}
	if len(open) == 0 {
		return 0, "", false
	}
	opener := open[len(open)-1]
	// The declaration bounds itself: a branch is as long as the cell it draws,
	// which is well past the cap one field test is read under.
	group, closed := balancedFrom(declaration[opener:], len(declaration))
	return opener, group, closed
}

// consequentAfter is the cell a condition draws: the block or arm following
// it, or the single statement a screen wrote no braces around.
func consequentAfter(text string) string {
	statement, _ := balancedFrom(strings.TrimLeft(text, " \t\n"), len(text))
	return statement
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
	window, _ := balancedFrom(text, maskedWindowCap)
	return window
}

// balancedFrom is that window under a bound of the caller's choosing, and
// whether it ended on the close of a group the text itself opened.
func balancedFrom(text string, bound int) (string, bool) {
	depth := 0
	for i, r := range text {
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth <= 0 {
				return text[:i+1], depth == 0
			}
		case ';':
			if depth == 0 {
				return text[:i], false
			}
		}
		if i >= bound {
			return text[:i], false
		}
	}
	return text, false
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
			var got []string
			for _, test := range maskedFieldTests(c.source) {
				got = append(got, test.field)
			}
			if !slices.Equal(got, c.want) {
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

// TestTheRefusalIsReadPerBranchAndNotPerDeclaration plants the shape one
// declaration can still hide: two withheld fields tested by the same
// component, one drawn through the shared control and one in the screen's own
// words, where the guarded branch answered for both.
//
// The last case is the edge of that reading. A declaration handing the whole
// withheld list to a control tests no field in a branch of its own, so the one
// refusal it states answers for every key in the list — which is what the
// control does with them.
func TestTheRefusalIsReadPerBranchAndNotPerDeclaration(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		screen  string
		refused []string
		blank   []string
	}{
		{
			name: "sibling cells of one facts strip",
			screen: `function DealFacts({ deal }) {
  const masked = deal.masked_fields ?? [];
  return (
    <RecordFacts>
      {masked.includes("amount_minor") ? (
        <FieldGuard mode="masked" />
      ) : (
        <Money minor={deal.amount_minor} />
      )}
      {masked.includes("currency") ? (
        <span>no currency for you</span>
      ) : (
        <span>{deal.currency}</span>
      )}
    </RecordFacts>
  );
}
`,
			refused: []string{"amount_minor"},
			blank:   []string{"currency"},
		},
		{
			name: "sibling readings of one cell",
			screen: `function AmountCell({ deal }) {
  if (deal.masked_fields?.includes("amount_minor")) {
    return <FieldGuard mode="masked" />;
  }
  if (deal.masked_fields?.includes("currency")) {
    return <span>no currency for you</span>;
  }
  return null;
}
`,
			refused: []string{"amount_minor"},
			blank:   []string{"currency"},
		},
		{
			name: "sibling readings named before the return",
			screen: `function DealCells({ deal }) {
  const masked = deal.masked_fields ?? [];
  const amount = masked.includes("amount_minor") ? <FieldGuard mode="masked" /> : deal.amount_minor;
  const currency = masked.includes("currency") ? <span>no currency for you</span> : deal.currency;
  return <Row amount={amount} currency={currency} />;
}
`,
			refused: []string{"amount_minor"},
			blank:   []string{"currency"},
		},
		{
			name: "the withheld list handed to a control",
			screen: `function DealForm({ deal, t }) {
  const masked = deal.masked_fields ?? [];
  return (
    <RecordFields
      readOnlyFields={Object.fromEntries(
        masked.map((key) => [key, t("record.notShown")]),
      )}
      maskedFields={masked.filter((key) =>
        ["amount_minor", "currency"].includes(key),
      )}
    />
  );
}
`,
			refused: []string{"amount_minor", "currency"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "deal.tsx"), []byte(c.screen), 0o600); err != nil {
				t.Fatalf("writing the fixture screen: %v", err)
			}
			drawn, err := maskedFieldsDrawnUnder(root)
			if err != nil {
				t.Fatalf("reading the fixture screen: %v", err)
			}
			for _, field := range c.refused {
				if !anyRefuses(drawn[field]) {
					t.Errorf("the branch drawing %q through the shared control read as drawing no refusal", field)
				}
			}
			for _, field := range c.blank {
				if anyRefuses(drawn[field]) {
					t.Errorf("%q is drawn in the screen's own words and was vouched for by a guarded "+
						"branch of the same declaration", field)
				}
			}
		})
	}
}
