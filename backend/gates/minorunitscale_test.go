// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A currency's minor-unit scale — 100 for EUR, 1000 for KWD, 1 for VND — is one
// fact, and the table that holds it lives in shared/kernel/values. The table
// was already shared. The ARITHMETIC that turns it into a multiplier was not:
// four functions each wrote their own `for i := 0; i < digits; i++ { s *= 10 }`
// and then divided by it, and one of them reimplemented values.MajorUnits from
// scratch to render the result.
//
// The four agreed, which is exactly how long that lasts. They were compared
// over 272 amount×currency pairs including math.MinInt64 and none disagreed —
// and a second copy stays harmless right up to the commit that corrects the
// digit count for one currency in the table and reaches only the callers
// somebody remembered.
//
// The obligation is narrow on purpose: a function that ASKS the table for a
// digit count must not then build the power of ten by hand. It may ask for
// other reasons — how many decimals to render, whether a currency has a minor
// unit at all — and those are not this gate's business. What it may not do is
// take the digits and do values' job with them, because values exports every
// shape a caller here has needed: MinorUnitScale for the multiplier,
// MajorUnits for the decimal string, WholeMajorUnits for the truncated integer.
//
// This is a census of ZERO, and that is the hard kind to write honestly. A gate
// asserting a shape is absent passes identically when the shape is absent and
// when the detector is broken, so the census below is worth nothing without
// TestTheHandScaledDetectorSeesWhatItClaimsTo beneath it — which plants all
// four shapes that actually shipped, plus the lookalikes that must NOT be
// findings, and reads the detector's answer directly.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

const (
	digitsFunc  = "MinorUnitDigits"
	valuesOwner = "internal/shared/kernel/values"
	scaleAdvice = "values.MinorUnitScale (the multiplier), values.MajorUnits (the decimal string) or values.WholeMajorUnits (the truncated integer)"
)

// handScaled names the functions in a file that do values' job with a
// currency's minor-unit digit count instead of asking values to do it.
//
// Two arms, because the four copies came in two shapes. The ARITHMETIC arm
// catches a function that asks for the digit count and then builds the power
// of ten — the loop —
//
//	scale := int64(1)
//	for i := 0; i < digits; i++ { scale *= 10 }
//
// which is what three of the four copies wrote, plus a direct math.Pow10,
// which nothing writes here today but is the obvious next spelling. The
// RENDERER arm catches the fourth, which built no power of ten at all and was
// therefore invisible to the first — see rendersADecimalPoint.
//
// It judges per FUNCTION, not per file. A file may hold a renderer that asks
// the table and a separate helper that scales something unrelated by ten;
// asking whether both appear somewhere in the same file reports a pairing
// nobody wrote. Per-declaration is also the reading that fails safe: it can
// miss a defect split across two functions, and it cannot invent one.
func handScaled(file *ast.File) []string {
	var found []string
	valuesQualifier := importedAs(file)
	// A bare `MinorUnitDigits(...)` is values' own call only inside values. In
	// any other package it names something else entirely, and reporting it
	// sends somebody to disprove a finding.
	//
	// `asksValues` gates the ARITHMETIC arm only. The renderer arm is keyed on
	// the parameter and never asks values anything — that is the whole reason
	// it can see the fourth copy — so short-circuiting the file here would
	// re-blind the census to exactly the shape the arm was added for.
	insideValues := file.Name != nil && file.Name.Name == "values"
	if insideValues {
		valuesQualifier = ""
	}
	asksValues := insideValues || valuesQualifier != ""
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if asksValues && asksForDigits(fn, valuesQualifier) && buildsAPowerOfTen(fn, importAlias(file, "math")) {
			found = append(found, fn.Name.Name)
			continue
		}
		if rendersADecimalPoint(fn) {
			found = append(found, fn.Name.Name)
		}
	}
	return found
}

// rendersADecimalPoint reports whether the declaration is a second
// values.MajorUnits: it takes a minor-unit digit count as a parameter and
// splices a decimal point into a formatted integer.
//
// It is a separate arm because the fourth copy was not a loop at all. It built
// the string by hand —
//
//	s := strconv.FormatInt(minor, 10)
//	point := len(s) - exponent
//	s = s[:point] + "." + s[point:]
//
// — and it was found only because deleting it made an exported wrapper dead.
// The arithmetic arm cannot see it: it computes no power of ten. Without this,
// the comment beside its call site claiming values.MajorUnits is the one
// renderer would be a uniqueness claim with nothing holding it, which is the
// one thing this repo's rulebook says a comment may not be.
//
// Keyed on the PARAMETER and not on a call, because a renderer is handed the
// count rather than asking for it — that is what made it invisible to a census
// that looked for the asking.
func rendersADecimalPoint(fn *ast.FuncDecl) bool {
	if digitParamName(fn) == "" {
		return false
	}
	// The digit count must reach the rendering, not merely sit in the
	// signature. A function that takes a `digits int` for something else and
	// happens to format a decimal elsewhere is not a second renderer, and
	// reporting it would send somebody to disprove a finding.
	point, formats, uses := false, false, false
	name := digitParamName(fn)
	// fn.Body and not fn: walking the whole declaration includes Type.Params,
	// so the parameter's own identifier in the SIGNATURE set `uses` and the
	// guard could never fail. A guard that cannot fail is the thing this file
	// exists to stop shipping.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.BasicLit:
			if v.Kind == token.STRING && (v.Value == `"."` || strings.Contains(v.Value, `.%0`)) {
				point = true
			}
		case *ast.Ident:
			if v.Name == name {
				uses = true
			}
		case *ast.SelectorExpr:
			switch v.Sel.Name {
			case "FormatInt", "Itoa", "Sprintf", "Sprint":
				formats = true
			}
		}
		return true
	})
	return point && formats && uses
}

// digitParamName returns the name of the declaration's minor-unit digit-count
// parameter, or "" if it has none. The names are the ones this tree has
// actually used; a renderer that calls it something else is out of reach, and
// the census says so rather than implying otherwise.
func digitParamName(fn *ast.FuncDecl) string {
	if fn.Type.Params == nil {
		return ""
	}
	for _, field := range fn.Type.Params.List {
		ident, ok := field.Type.(*ast.Ident)
		if !ok || ident.Name != "int" {
			continue
		}
		for _, n := range field.Names {
			switch n.Name {
			case "digits", "exponent", "decimals", "minorUnits":
				return n.Name
			}
		}
	}
	return ""
}

// importedAs returns the local name the values package is bound to in this
// file, or "" if the file does not import it. A DOT import returns "" too, and
// that is a stated gap rather than an oversight: a dot-imported call is a bare
// identifier, indistinguishable from a local function of the same name without
// type information, and the census would rather miss it than name the wrong
// function. Nothing in this tree dot-imports values.
//
// It exists so the census asks about VALUES' MinorUnitDigits and not about any
// selector that happens to end in that name. Matching by suffix alone would
// report a different package's identically-named method, and a census that
// reports the wrong subject is worse than one that reports none: somebody has
// to go and disprove it.
func importedAs(file *ast.File) string {
	return importAlias(file, "github.com/margince/margince/backend/"+valuesOwner)
}

// importAlias returns the local name an import path is bound to in this file,
// or "" if the file does not import it. A DOT import returns "" too, for the
// reason importedAs states.
//
// One resolver and not one per package: two functions that both iterate
// file.Imports, trim the quoted path and fall back to the last segment are two
// answers to one question, which is the thing this PR is about. They had
// already diverged on the dot-import case by the time it was pointed out.
func importAlias(file *ast.File, path string) string {
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) != path {
			continue
		}
		if spec.Name != nil {
			if spec.Name.Name == "." {
				return ""
			}
			return spec.Name.Name
		}
		if i := strings.LastIndex(path, "/"); i >= 0 {
			return path[i+1:]
		}
		return path
	}
	return ""
}

// asksForDigits reports whether the declaration calls values.MinorUnitDigits.
// An empty qualifier means the caller has established this file IS the values
// package, where the function is declared and the only place it is reachable
// unqualified.
func asksForDigits(fn *ast.FuncDecl, qualifier string) bool {
	asks := false
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch f := call.Fun.(type) {
		case *ast.Ident:
			asks = asks || (f.Name == digitsFunc && qualifier == "")
		case *ast.SelectorExpr:
			pkg, ok := f.X.(*ast.Ident)
			asks = asks || (ok && f.Sel.Name == digitsFunc && qualifier != "" && pkg.Name == qualifier)
		}
		return !asks
	})
	return asks
}

// buildsAPowerOfTen reports whether the declaration REPEATEDLY multiplies an
// accumulator by ten, or reaches for math.Pow10.
//
// Repeatedly — the multiplication must sit inside a loop. A power of ten is
// built by iterating the digit count; a lone `x * 10` is a different thing
// entirely, and counting it made the gate fire on any function that read the
// digit count for a legitimate reason and happened to multiply something by
// ten elsewhere. The detector still cannot prove the loop is bounded BY the
// digit count — that needs data flow — but "in a loop" is the part that
// distinguishes a scale from arithmetic, and it is checkable.
func buildsAPowerOfTen(fn *ast.FuncDecl, mathQualifier string) bool {
	builds := false
	ast.Inspect(fn, func(n ast.Node) bool {
		// math.Pow10 and not any `x.Pow10`: a name-only match reported an
		// unrelated package's method as this defect.
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Pow10" {
			if pkg, isIdent := sel.X.(*ast.Ident); isIdent && pkg.Name == mathQualifier {
				builds = true
				return false
			}
		}
		body := loopBody(n)
		if body == nil {
			return !builds
		}
		builds = builds || multipliesByTen(body)
		return !builds
	})
	return builds
}

// loopBody returns the body of a for or range statement, or nil.
func loopBody(n ast.Node) *ast.BlockStmt {
	switch v := n.(type) {
	case *ast.ForStmt:
		return v.Body
	case *ast.RangeStmt:
		return v.Body
	}
	return nil
}

// multipliesByTen reports whether the block multiplies something by the
// literal ten.
func multipliesByTen(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		v, ok := n.(*ast.AssignStmt)
		if ok {
			if v.Tok == token.MUL_ASSIGN && anyLiteralTen(v.Rhs) {
				found = true
			}
			// `scale = scale * 10` written out. NOT the `:=` that seeds the
			// accumulator — `scale := int64(1)` has no binary right-hand side
			// and could never match here, and a comment saying it did was one
			// more claim with nothing behind it.
			if v.Tok == token.ASSIGN || v.Tok == token.DEFINE {
				for _, rhs := range v.Rhs {
					if bin, isBin := rhs.(*ast.BinaryExpr); isBin && bin.Op == token.MUL && anyLiteralTen([]ast.Expr{bin.X, bin.Y}) {
						found = true
					}
				}
			}
		}
		return !found
	})
	return found
}

// anyLiteralTen reports whether either operand is the literal 10. Only ten: a
// scale is built by repeated multiplication BY TEN, and a `* 100` in the same
// function is check-money-scale.sh's subject, not this one. Two gates claiming
// one shape is how a finding gets reported twice and fixed neither time.
func anyLiteralTen(exprs []ast.Expr) bool {
	for _, expr := range exprs {
		if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.INT && lit.Value == "10" {
			return true
		}
	}
	return false
}

// handWrittenGoFiles walks the module for source a person maintains. Generated
// files are excluded because nobody edits them and a generator that emitted
// this shape would be the generator's defect, not a call site's.
func handWrittenGoFiles(t *testing.T) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name := entry.Name(); name == "node_modules" || name == "testdata" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_gen.go") || strings.HasSuffix(name, ".gen.go") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module for Go source: %v", err)
	}
	// A walk that finds nothing certifies nothing. The floor is deliberately
	// far below the real count (~3700) so it catches a broken walk, not a
	// changing tree.
	if len(paths) < 500 {
		t.Fatalf("the walk found only %d Go files, so this census covered almost nothing", len(paths))
	}
	return paths
}

func TestNobodyBuildsAMinorUnitScaleByHand(t *testing.T) {
	fset := token.NewFileSet()
	var findings []string
	for _, path := range handWrittenGoFiles(t) {
		// values is where the arithmetic belongs, so it is the one place the
		// shape is correct. Keyed to the FILE that owns the table and not the
		// package: a second file in that package writing its own loop is the
		// same defect wearing the right import path.
		if filepath.ToSlash(path) == valuesOwner+"/minorunits.go" {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, fn := range handScaled(file) {
			findings = append(findings, fmt.Sprintf("%s: %s", path, fn))
		}
	}
	if len(findings) == 0 {
		return
	}
	t.Errorf("these functions take a currency's minor-unit digit count and do values' job with it — "+
		"building the power of ten, or rendering the decimal string:\n  %s\n\nvalues already owns both "+
		"steps: %s. Four copies existed and agreed over 272 amount-by-currency pairs, which is how long "+
		"a second copy stays harmless: until the commit that corrects one currency's digit count reaches "+
		"only the callers somebody remembered.", strings.Join(findings, "\n  "), scaleAdvice)
}

// TestTheHandScaledDetectorSeesWhatItClaimsTo is the half that makes the census
// above mean anything. A census of zero passes identically over a clean tree
// and over a detector that has stopped detecting, so the detector is read
// directly here — against the shapes that actually shipped, and against the
// lookalikes it must leave alone.
// handScaledProbe is one planted source file and the answer the detector must
// give for it. Extracted from the test because the table is the substance and a
// 170-line function is not navigable — the test below is the four lines that
// read it.
type handScaledProbe struct {
	name  string
	fires bool
	// mode picks the file the probe is parsed as, because the detector's
	// answers depend on it and a probe that guesses wrong asks a different
	// question than the tree does:
	//
	//   ""         package probe, importing values — an ordinary caller
	//   "values"   package values, no import — the one place a bare call
	//              to MinorUnitDigits is values' own
	//   "noimport" package probe, no import — a package that does not use
	//              values at all and may well declare its own
	mode string
	src  string
}

var handScaledProbes = []handScaledProbe{
	{"the loop that shipped in compose", true, "", `
func priceEvidenced(currency string, priceMinor int64) bool {
	digits := values.MinorUnitDigits(currency)
	scale := int64(1)
	for i := 0; i < digits; i++ {
		scale *= 10
	}
	return priceMinor/scale > 0
}`},
	{"the loop that shipped in overlay, digits read inline", true, "", `
func amountFor(code, amount string) int64 {
	scale := int64(1)
	for i := 0; i < values.MinorUnitDigits(code); i++ {
		scale *= 10
	}
	return toMinor(amount, scale)
}`},
	{"a range loop rather than a counter", true, "", `
func scaleOf(currency string) int64 {
	scale := int64(1)
	for range values.MinorUnitDigits(currency) {
		scale *= 10
	}
	return scale
}`},
	{"the multiplication written out", true, "", `
func scaleOf(currency string) int64 {
	digits, scale := values.MinorUnitDigits(currency), int64(1)
	for i := 0; i < digits; i++ {
		scale = scale * 10
	}
	return scale
}`},
	// The fourth copy, verbatim as it stood in overlay/hubspot before this
	// change. It builds no power of ten, so the arithmetic arm cannot see
	// it — and it was found only because deleting it made an exported
	// wrapper dead, which is not a way of finding things.
	{"the string-splice renderer that shipped in hubspot", true, "", `
func minorToDecimalString(minor int64, exponent int) string {
	s := strconv.FormatInt(minor, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	if exponent > 0 {
		for len(s) <= exponent {
			s = "0" + s
		}
		point := len(s) - exponent
		s = s[:point] + "." + s[point:]
	}
	if neg {
		s = "-" + s
	}
	return s
}`},
	{"the same renderer written with Sprintf", true, "", `
func render(minor int64, digits int) string {
	return fmt.Sprintf("%d.%0*d", minor/100, digits, minor%100)
}`},
	{"math.Pow10, the next spelling nobody has written yet", true, "", `
func scaleOf(currency string) float64 {
	return math.Pow10(values.MinorUnitDigits(currency))
}`},
	{"an unqualified call, as values itself would write it", true, "values", `
func scaleOf(currency string) int64 {
	scale := int64(1)
	for range MinorUnitDigits(currency) {
		scale *= 10
	}
	return scale
}`},

	// The other direction, which is where a census of zero usually goes
	// wrong: a detector that fires on everything also reports nothing, in
	// the sense that nobody can act on it.
	{"asking the table without scaling", false, "", `
func decimalsFor(currency string) int {
	return values.MinorUnitDigits(currency)
}`},
	{"asking, then delegating the scaling to values", false, "", `
func majorOf(currency string, amountMinor int64) string {
	if values.MinorUnitDigits(currency) == 0 {
		return strconv.FormatInt(amountMinor, 10)
	}
	return values.MajorUnits(amountMinor, currency)
}`},
	{"scaling by ten with nothing to do with currency", false, "", `
func decimate(n int64) int64 {
	scale := int64(1)
	for i := 0; i < 3; i++ {
		scale *= 10
	}
	return n / scale
}`},
	{"a hundred, which is the money-scale gate's subject", false, "", `
func majorOf(currency string, amountMinor int64) int64 {
	_ = values.MinorUnitDigits(currency)
	return amountMinor / 100
}`},
	{"a function taking a digit count that renders nothing", false, "", `
func padTo(s string, digits int) string {
	for len(s) < digits {
		s = "0" + s
	}
	return s
}`},
	// The detector cannot prove the loop counts the DIGITS — that needs data
	// flow. What it can require is that the multiplication happens in a
	// loop at all, which is what separates building a scale from ordinary
	// arithmetic. Without it, reading the digit count for a legitimate
	// reason and multiplying anything by ten elsewhere was a finding.
	{"reading the digit count beside an unrelated * 10", false, "", `
func widthFor(currency string, ratio int) (int, int) {
	return values.MinorUnitDigits(currency), ratio * 10
}`},
	{"a different package's MinorUnitDigits is not values'", false, "", `
func scaleOf(c legacy.Code) int64 {
	scale := int64(1)
	for range c.MinorUnitDigits() {
		scale *= 10
	}
	return scale
}`},
	// The digit count must reach the RENDER, not merely sit in the signature.
	// A package that does not import values and declares its OWN
	// MinorUnitDigits is not calling values'.
	// The renderer arm does not ask values anything — it is keyed on the
	// parameter — so a file that does not import values must still be judged by
	// it. Short-circuiting the whole file on "does not import values" re-blinded
	// the census to the one shape that arm exists for.
	{"a renderer in a file that does not import values", true, "noimport", `
func minorToDecimalString(minor int64, exponent int) string {
	s := strconv.FormatInt(minor, 10)
	point := len(s) - exponent
	return s[:point] + "." + s[point:]
}`},
	{"another package's own unqualified MinorUnitDigits", false, "noimport", `
func scaleOf(currency string) int64 {
	scale := int64(1)
	for range MinorUnitDigits(currency) {
		scale *= 10
	}
	return scale
}`},
	{"a digit count the render ignores", false, "", `
func label(name string, digits int) string {
	return fmt.Sprintf("%s/%s", name, "eur")
}`},
	// The digit count must be USED in the body. Walking the whole
	// declaration includes the signature, so the parameter's own
	// identifier there satisfied the guard and it could never fail.
	{"a renderer whose digit count is signature-only", false, "", `
func render(minor int64, digits int) string {
	return fmt.Sprintf("%d.%02d", minor/100, minor%100)
}`},
	{"a decimal point with no digit count in sight", false, "", `
func label(name string) string {
	return fmt.Sprintf("%s.%s", name, "eur")
}`},
	{"the two halves in DIFFERENT functions of one file", false, "", `
func decimalsFor(currency string) int { return values.MinorUnitDigits(currency) }

func decimate(n int64) int64 {
	scale := int64(1)
	scale *= 10
	return n / scale
}`},
}

func TestTheHandScaledDetectorSeesWhatItClaimsTo(t *testing.T) {
	fset := token.NewFileSet()
	for _, tc := range handScaledProbes {
		t.Run(tc.name, func(t *testing.T) {
			// The probe carries the same imports the real call sites do, because
			// the detector resolves the values qualifier through them — a probe
			// without the import asks a different question than the tree does.
			head := "package probe\n"
			switch tc.mode {
			case "values":
				head = "package values\n"
			case "noimport":
			default:
				head += "import (\n\t\"fmt\"\n\t\"math\"\n\t\"strconv\"\n\t\"strings\"\n\n\t\"github.com/margince/margince/backend/" + valuesOwner + "\"\n)\n"
			}
			file, err := parser.ParseFile(fset, "probe.go", head+tc.src, 0)
			if err != nil {
				t.Fatalf("the probe does not parse, so it proves nothing about the detector: %v", err)
			}
			found := handScaled(file)
			if tc.fires && len(found) == 0 {
				t.Errorf("the detector missed a hand-built scale — the census would read green over this:\n%s", tc.src)
			}
			if !tc.fires && len(found) > 0 {
				t.Errorf("the detector reported %v, which is not a hand-built scale:\n%s", found, tc.src)
			}
		})
	}
}
