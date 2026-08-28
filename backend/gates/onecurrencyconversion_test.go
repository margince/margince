// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// Converting money to the base currency has one implementation.
//
// It had two, in two languages: exact decimal over big.Int in the hierarchy
// rollup, and a LEFT JOIN LATERAL on fx_rate with round(amount × rate)::bigint
// on the company page. Both encoded the same four decisions — the direction,
// the as-of cutoff, newest-wins, and the multiply-and-round — and they agreed.
//
// Nothing made them keep agreeing. The predicted first divergence was the
// rounding: half-away-from-zero against Postgres round(), which is a
// one-minor-unit disagreement between two pages about the same account, and the
// kind of defect nobody can reproduce on demand.
//
// What is NOT shared, and must not be, is the missing-rate policy: the rollup
// refuses the whole read, the company page prices what it can and counts the
// rest. Both are right for their own surface, so the engine answers "is there a
// rate, and what does it make of this amount" and leaves the rest to the caller.
//
// This gate is about the ENGINE, so it hunts a second one: a read of fx_rate
// outside the module that owns it, and an arithmetic conversion spelled in SQL.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// fxRateOwner is the module whose store owns fx_rate, and so the one place the
// lookup is written. tableOwners agrees; this names it for the failure message.
const fxRateOwner = "internal/modules/deals"

// readsFXRate matches a statement that goes to the rate table itself.
var readsFXRate = regexp.MustCompile(`(?i)\bfrom\s+fx_rate\b|\bjoin\s+fx_rate\b`)

// readsAnInterpolatedTable matches a FROM or JOIN whose table name this gate
// cannot see, because the flattening substitutes a space for every operand that
// is not a literal. The trailing anchor is what keeps it off an ordinary
// `FROM deal d`: only a gap where a name belongs matches.
var readsAnInterpolatedTable = regexp.MustCompile(`(?i)\b(from|join)\s{2,}\b|(?i)\b(from|join)\s+$`)

// convertsInSQL matches the arithmetic: an amount multiplied by a rate. It is
// the half that makes a second read a second ENGINE rather than a lookup — a
// query that reads a rate and hands it to Go is converting through
// deals.ConvertToBase like everything else.
var convertsInSQL = regexp.MustCompile(`(?i)amount_minor\s*\*|\*\s*[a-z_.]*rate\b`)

// fxConversionExempt ratifies the files that may spell either half.
var fxConversionExempt = gatekit.Waive(map[string]string{
	"internal/modules/deals/fxconvert.go": "the engine itself: the one lookup and the one arithmetic every " +
		"caller reaches money conversion through",
	"internal/modules/deals/fxrate_store.go": "the rate table's own store — reading and writing the rows the " +
		"engine later looks up is what owning the table means",
	"internal/modules/deals/basecurrencyfreeze.go": "the CLOSE-time freeze, which is a different question: a " +
		"closed deal stores the rate it was converted at, where this engine answers what an OPEN one converts " +
		"at today",
	"internal/modules/deals/deal_advance.go": "the same freeze at the moment it happens — advancing to won " +
		"reads the rate once and stores it on the deal, which is why a closed figure never moves again",
	"internal/modules/quotas/attainment.go": "a THIRD conversion, and one the module DAG forces: quotas is a " +
		"module and a module never imports a sibling (ADR-0054 §3), so it cannot reach deals.FXRates and must " +
		"ask fx_rate itself. Its own comment already names the two implementations it mirrors. Holding it " +
		"identical to them is an obligation this gate cannot discharge — what it can do is stop a FOURTH " +
		"appearing anywhere the engine IS reachable",
	"internal/compose/rateproposals_integration_test.go": "a test seeding and asserting on rate rows. It reads " +
		"the table to arrange and to check, never to convert an amount for a reader",
	"gates/onecurrencyconversion_test.go": "this gate itself: the probes below are planted defects, and " +
		"judging them would report its own evidence as a finding. The file holds nothing but the gate, so " +
		"skipping it whole costs no coverage — unlike a census whose probes sit beside real code, where the " +
		"exemption belongs on the declaration",
	"internal/compose/briefs/briefrank.go": "a FOURTH conversion, and the only ratified one that could reach " +
		"the engine — it is in compose. It converts inside a larger ranking query and answers a wider " +
		"question than the engine does: a CLOSED deal reads its frozen amount_minor_base, which is a stored " +
		"figure rather than a conversion. Moving it means restructuring that query and teaching the engine " +
		"the frozen case, which is its own change rather than a line in this one. Recorded so it is a known " +
		"copy rather than an unnoticed one",
})

func TestMoneyConvertsToTheBaseCurrencyInOnePlace(t *testing.T) {
	t.Parallel()
	defer fxConversionExempt.AssertAllMatched(t)

	var offences []string
	judged := 0
	// The WHOLE tree, walked directly rather than through a Scope: a second
	// conversion is only interesting where nobody thought to look, and the
	// roots a gate names are exactly where it did.
	fset := token.NewFileSet()
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		statements := fxSQLStatementsIn(file)
		namesFXRate := false
		for _, sql := range statements {
			if strings.Contains(strings.ToLower(sql), "fx_rate") {
				namesFXRate = true
			}
		}
		for _, sql := range statements {
			found, why := conversionIn(where, sql, namesFXRate)
			if !found {
				continue
			}
			judged++
			if fxConversionExempt.Waived(t, where) {
				continue
			}
			offences = append(offences, where+" "+why)
		}
	}
	// A census that judged nothing certifies nothing: the rate table is read by
	// the engine and its store at minimum, so a zero here is a broken scan.
	if judged == 0 {
		t.Fatal("no statement reading fx_rate was found at all, so this gate is reading a tree shape " +
			"that is gone rather than a clean one")
	}
	if len(offences) > 0 {
		t.Errorf("money converts to the base currency in more than one place:\n  %s\n\n"+
			"Two implementations of one conversion agree until they do not, and the first thing to "+
			"diverge is the rounding — a one-minor-unit disagreement between two pages about the same "+
			"account, which nobody can reproduce on demand.", strings.Join(offences, "\n  "))
	}
}

// conversionIn reports whether one statement converts money outside the engine,
// and what it did. Extracted from the walk so the shapes it must catch can be
// PLANTED and asserted: a census over a clean tree passes identically over a
// detector that has stopped detecting.
func conversionIn(where, sql string, fileNamesFXRate bool) (bool, string) {
	if !readsFXRate.MatchString(sql) {
		// A FROM whose table this gate cannot read — the name is interpolated
		// from a variable, so the flattening left a gap where it belongs.
		//
		// Reported only when the FILE also names fx_rate in a literal
		// somewhere, and that pairing is what makes it worth reporting rather
		// than noise: a dynamic FROM alone is ordinary (this tree builds them
		// over lists of tables), and a file naming the rate table is ordinary,
		// but a file doing both may be reading fx_rate through a name this
		// gate cannot follow. The answer is a waiver saying which table it is,
		// which is a sentence somebody has to write down.
		if fileNamesFXRate && readsAnInterpolatedTable.MatchString(sql) {
			return true, "builds a FROM whose table name this gate cannot resolve, in a file that also " +
				"names fx_rate — say which table it is in a waiver, so a read through an interpolated " +
				"name cannot hide here"
		}
		return false, ""
	}
	if convertsInSQL.MatchString(sql) {
		return true, "converts an amount at a rate in SQL; the arithmetic is deals.ConvertToBase, which " +
			"rounds half away from zero over exact decimal digits, and the lookup is deals.FXRates"
	}
	if strings.HasPrefix(where, fxRateOwner+"/") {
		return true, "reads fx_rate outside the engine, inside the module that owns it — the lookup is " +
			"deals.FXRates"
	}
	return true, "reads fx_rate directly; the lookup is deals.FXRates, which memoizes one per currency and " +
		"answers the same rate the other surface converts at"
}

// fxSQLStatementsIn returns the statements a file holds, with CONCATENATIONS
// FLATTENED.
//
// Flattened, because a statement is routinely assembled — `"SELECT … FROM " +
// table` is the shape in this tree — and a scan over bare literals sees each
// half alone: neither fragment carries both the FROM and the table name, so a
// second engine built that way is invisible. Under-recognition is the one
// direction a prohibition may not fail in, and this is the shape it fails in.
//
// Each concatenation contributes its joined text AND its pieces stay available
// through the walk, so a literal that is a whole statement is still read as
// one.
func fxSQLStatementsIn(file *ast.File) []string {
	var out []string
	joined := map[ast.Node]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.BinaryExpr:
			if typed.Op != token.ADD {
				return true
			}
			text, ok := flattenConcatenation(typed, joined)
			if ok {
				out = append(out, text)
			}
			return true
		case *ast.BasicLit:
			if joined[typed] {
				return true
			}
			if text, isText := gatekit.LiteralText(typed); isText {
				out = append(out, text)
			}
		}
		return true
	})
	return out
}

// flattenConcatenation joins the string operands of one `+` chain, marking each
// literal it consumed so the walk does not report it a second time on its own.
//
// A non-literal operand — a variable, a call — contributes a space rather than
// nothing: it stands where text the gate cannot see would be, and joining
// around it would fuse two words that are not adjacent in the statement.
func flattenConcatenation(expr *ast.BinaryExpr, joined map[ast.Node]bool) (string, bool) {
	var parts []string
	var walk func(ast.Expr)
	walk = func(node ast.Expr) {
		switch typed := node.(type) {
		case *ast.BinaryExpr:
			if typed.Op == token.ADD {
				walk(typed.X)
				walk(typed.Y)
				return
			}
			parts = append(parts, " ")
		case *ast.BasicLit:
			if text, isText := gatekit.LiteralText(typed); isText {
				joined[typed] = true
				parts = append(parts, text)
				return
			}
			parts = append(parts, " ")
		default:
			parts = append(parts, " ")
		}
	}
	walk(expr)
	return strings.Join(parts, ""), len(parts) > 0
}

// TestTheDetectorSeesEachShapeAConversionIsWrittenIn is the positive control.
//
// The census above passes identically over a clean tree and over a detector
// that has stopped detecting: it reports nothing either way. These read the
// detector directly, on shapes taken from how this tree actually writes
// queries, so the census means something.
//
// The concatenated case is why this exists. A statement assembled as
// `"SELECT … FROM " + table` splits the FROM and the table name across two
// literals, and a scan over bare literals sees neither — a second engine
// written that way was invisible, which is the one direction a prohibition may
// not fail in.
func TestTheDetectorSeesEachShapeAConversionIsWrittenIn(t *testing.T) {
	t.Parallel()
	const elsewhere = "internal/compose/somewhere/read.go"
	for _, probe := range []struct {
		what  string
		src   string
		fires bool
	}{
		{"a plain read", "package p\nvar q = `SELECT rate FROM fx_rate WHERE from_currency = $1`\n", true},
		{"a join", "package p\nvar q = `SELECT d.id FROM deal d JOIN fx_rate r ON r.from_currency = d.currency`\n", true},
		{
			"a conversion in SQL",
			"package p\nvar q = `SELECT round(d.amount_minor * r.rate)::bigint FROM fx_rate r`\n",
			true,
		},
		{
			// The shape the first version of this gate could not see: the table
			// name comes from a variable, so the flattening leaves a gap where
			// it belongs. Caught because the FILE also names fx_rate — a
			// dynamic FROM alone is ordinary here, and so is naming the table;
			// doing both is what earns the question.
			"a read whose table name is interpolated, in a file that names the rate table",
			"package p\nvar q = `SELECT rate FROM ` + rateTable + ` WHERE from_currency = $1`\nvar rateTable = `fx_rate`\n",
			true,
		},
		{
			// And the near miss that keeps it from being noise: a dynamic FROM
			// in a file with no interest in rates at all.
			"a dynamic FROM in a file that never names the rate table",
			"package p\nvar q = `SELECT count(*) FROM ` + table + ` WHERE archived_at IS NULL`\nvar table = `deal`\n",
			false,
		},
		{
			"a concatenation whose halves are both literal",
			"package p\nvar q = `SELECT rate ` + `FROM fx_rate WHERE from_currency = $1`\n",
			true,
		},
		// Near misses. A gate widened until it matches ordinary prose is a gate
		// somebody turns off.
		{"a read of another table", "package p\nvar q = `SELECT amount_minor FROM deal WHERE id = $1`\n", false},
		{
			"the words in a comment",
			"package p\n\n// This reads FROM fx_rate and multiplies amount_minor * rate.\nvar q = 1\n",
			false,
		},
		{
			"a write to the rate table, which is not a conversion",
			"package p\nvar q = `INSERT INTO fx_rate (from_currency, rate) VALUES ($1, $2)`\n",
			false,
		},
	} {
		t.Run(probe.what, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "probe.go", probe.src, parser.ParseComments)
			if err != nil {
				t.Fatalf("parsing the probe: %v", err)
			}
			statements := fxSQLStatementsIn(file)
			namesRate := false
			for _, sql := range statements {
				if strings.Contains(strings.ToLower(sql), "fx_rate") {
					namesRate = true
				}
			}
			fired := false
			for _, sql := range statements {
				if found, _ := conversionIn(elsewhere, sql, namesRate); found {
					fired = true
				}
			}
			if fired != probe.fires {
				t.Errorf("%s: the detector answered %t, want %t — %q", probe.what, fired, probe.fires, probe.src)
			}
		})
	}
}
