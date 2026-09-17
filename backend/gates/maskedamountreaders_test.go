// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// dealAmountColumn is a deal's money however a statement names it.
//
// Three spellings, and each was learned from a way the figure could have left
// this census's sight:
//
//   - `amount_minor`, the column and the field a mask names.
//   - `amount_minor_base`, the SAME money converted. A mask on the field has to
//     withhold both or the base column is the way around it, so a census that
//     saw only the first would read green over the total that discloses it.
//
// Not a third for the report engine's late-bound token, and that is a decision
// about the ENGINE rather than about this pattern: the project report binds a
// mask CLAUSE beside the column rather than a rendering that replaces it,
// precisely so the figure keeps spelling itself in the statement this census
// reads. A token standing in for the whole expression would have taken the
// project report out of sight on the day it was first guarded.
var dealAmountColumn = regexp.MustCompile(`(?i)\b(amount_minor_base|amount_minor)\b`)

var dealTableRead = gatekit.TableReadPattern("deal")

// maskOwner is the module that OWNS the deal object and the mask.
const maskOwner = "internal/modules/deals/"

// The seeds are LISTED rather than derived from platform/auth's Mask* surface,
// and the direction of the failure is why. A new primitive that nobody adds
// here does not go unnoticed: every statement using it reports as unmasked, so
// the gate refuses a tree it cannot yet read. A derivation over the exported
// names would admit whatever a future function is called, which is the same
// list with the safe direction reversed.
// A mask spelling is the WHOLE admission here, so objectGateSatisfies stays
// off and auth.Require(ctx, "deal", …) vouches for nothing — the field's own
// comment says why the object half cannot answer this question.
var maskGate = objectGate{
	object:  "deal",
	literal: dealAmountColumn,
	gateSeeds: []string{
		"MaskedColumnSQL", "MaskedExpressionSQL", "MaskExcludedClause",
		"MaskedFields", "MasksAnyRowOf", "maskedRowSelects",
	},
}

// predicateMaskedAmountReads: the statement names an amount column but no
// figure reaches a reader who could be masked.
var predicateMaskedAmountReads = gatekit.Waive(map[string]string{})

// lifecycleMaskedAmountReads: the read runs as PrincipalSystem, which
// auth.Unbounded answers for outright — a mask asked there would be inert, and
// the entry records why asking was never the point.
var lifecycleMaskedAmountReads = gatekit.Waive(map[string]string{
	"internal/compose/assuranceseam.go":                         "the assurance scan's subject statement, selecting each deal's owner, amount and currency so a finding can be raised against it. The pass runs as PrincipalSystem over the whole installation (assurancebundle.go records it), and what a HUMAN later reads is AssuranceExceptions, which is a report surface and carries the engine's own mask pass. A mask applied to the scan would hide a finding from everybody rather than a figure from one reader",
	"internal/modules/privacy/sarsections.go:sarRecordSections": "the deal rows AS the Art. 15 export: what the installation holds about the data subject, assembled under the system principal on a request a human already authorised. A field mask narrows what a COLLEAGUE may read about a record; applying one here would make the subject's own answer incomplete, which is the defect the export exists to prevent",
})

// calleeGatedMaskedAmountReads: the statement carries the mask, but the
// spelling that applies it is bound elsewhere.
var calleeGatedMaskedAmountReads = gatekit.Waive(map[string]string{
	"internal/compose/reportprojects.go": "the project report's won-deal money total. It sums d.amount_minor_base under a FILTER on reportDealMaskToken, which reportsql.go binds from auth.MaskExcludedClause beside the row-scope token next to it — so the statement here is a template and the mask is resolved at bind time. The engine's own mask pass cannot cover this one: it asks the masks on the SPEC's entity, and the spec is projects while the sum is over deals",
})

// ruledMaskedAmountReads: a read the product has ruled may print the figure
// whatever the caller's masks say. Empty, and the emptiness is the finding: no
// surface has earned an exemption from a mask an administrator set.
var ruledMaskedAmountReads = gatekit.Waive(map[string]string{})

// deferredMaskedAmountReads: a read that still prints a masked figure, each
// naming the issue that will close it.
var deferredMaskedAmountReads = gatekit.Waive(map[string]string{})

// notTheDealAmount: the statement names an amount column belonging to another
// table, and joins `deal` for something else.
//
// The compound subject keeps most of these out — forecast_call,
// finance_payment and commission_entry each carry their own amount_minor and
// never mention the deal table — but a statement that reads a FROZEN figure and
// joins the deal for its name matches both halves. A frozen contribution is not
// the deal's current amount: it is what the snapshot recorded, governed by the
// recipient clause the share was minted with.
var notTheDealAmount = gatekit.Waive(map[string]string{
	"internal/compose/analyticssharesnapshot.go:ReadSharedSnapshot": "the aggregates of ONE frozen forecast snapshot: every amount here is `c.amount_minor` on forecast_contribution, the figure recorded when the snapshot was taken, and `deal` is joined for the name. What bounds it is sharedVisibilityClause — the per-recipient clause the share was minted with — and the statement also reports whether anything was kept back rather than leaving a short number unexplained. The live deal's mask is a different question about a different row, and the file DOES ask it where it reads one (MaskExcludedClause, same file)",
	"internal/compose/analyticssharesnapshot.go:SharedSnapshotRows": "the same frozen contributions, listed rather than summed, under that same recipient clause and for the same reason",
})

var maskVerdicts = []namedVerdict{
	{"predicate", predicateMaskedAmountReads},
	{"lifecycle", lifecycleMaskedAmountReads},
	{"callee-gated", calleeGatedMaskedAmountReads},
	{"not-the-amount", notTheDealAmount},
	{"ruled", ruledMaskedAmountReads},
	{"deferred", deferredMaskedAmountReads},
}

const wantMinimumMaskedAmountSites = 10

// maskedAmountScope is built around the derived builder set, so the sweep and
// the site extraction ask one question rather than two that can drift.
func maskedAmountScope(builders map[string]bool) gatekit.Scope {
	return gatekit.Scope{
		Roots: []string{"internal"},
		Subject: func(filePath string, file *ast.File) bool {
			return readsADealAmount(filePath, file, builders)
		},
		Exempt: gatekit.Waive(map[string]string{}),
	}
}

// readsADealAmount is the compound subject, and the compound is the point.
//
// `amount_minor` is not a rare column name: forecast_call, finance_payment,
// commission_entry and the forecast snapshot each carry one, and a mask on
// deal.amount_minor says nothing about any of them. A pattern matching the
// column alone found forty reads that were not about a deal at all, which
// would have cost forty declarations saying so — noise that buries the signal
// this census exists to carry.
//
// So a file is a subject when it holds a DECLARATION whose SQL both reads the
// deal table and names an amount column. Per declaration rather than per
// literal, because this tree routinely composes one statement from several
// constants, and a literal-level test would stop seeing a read the moment
// somebody moved its FROM clause into a fragment.
func readsADealAmount(filePath string, file *ast.File, builders map[string]bool) bool {
	if strings.HasPrefix(filePath, maskOwner) || strings.HasSuffix(filePath, "_gen.go") {
		return false
	}
	for _, decl := range file.Decls {
		if len(dealAmountReadsIn(decl, builders)) > 0 {
			return true
		}
	}
	return false
}

// A deal-amount BUILDER renders the figure as SQL for somebody else to compose.
// It is part of the subject because its text is not: a statement built from one
// names no amount column of its own, so a literal-only subject cannot see the
// money it sums. The project report's open-value total was exactly that —
// `sum(deals.OpenDealBaseValueSQL(...))` over every open deal, with no mask and
// nothing here to say so.
//
// DERIVED from the tree rather than listed, and the list is why. It was two
// names, written down when a reviewer found the first gap; it was already
// missing a third. `briefs.briefBaseValueSQL` is a CHARACTER-IDENTICAL second
// spelling of compose.BaseValueSQL, held so by TestOneSpellingOfADealsBaseValue
// because the import direction forbids the call — so this tree states that the
// two are one expression, and a hand-kept list saw one of them. Two statements
// selecting a deal's money sat outside the census for as long as that list was
// the answer.
//
// A builder is a function whose one result is a string and whose body names an
// amount column INSIDE A STATEMENT FRAGMENT. Both halves are load-bearing:
// deals.AmountCurrencyPairError.Error() names `amount_minor` in an English
// sentence, and a rule reading the column alone would make every caller of an
// error's Error() a money read. TestTheBuilderDerivationReadsFragmentsAndNotProse
// holds both directions.
var sqlFragmentMarker = regexp.MustCompile(`(?i)\b(SELECT|CASE|WHEN|COALESCE|SUM|ROUND|JOIN|WHERE)\b`)

// dealAmountBuilderNames walks the same tree the scope sweeps, INCLUDING the
// mask's owner and the packages a subject may not live in: a builder is named
// where it is declared and called where it is composed, and the two are
// routinely different modules — that separation is the whole reason the census
// cannot see the read.
func dealAmountBuilderNames(t testing.TB) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.Walk("internal", func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(filePath, ".go") ||
			strings.HasSuffix(filePath, "_test.go") || strings.HasSuffix(filePath, "_gen.go") {
			return err
		}
		file, parseErr := parser.ParseFile(fset, filePath, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, decl := range file.Decls {
			if name, isBuilder := builderName(decl); isBuilder {
				names[name] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("deriving the deal-amount builders: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("no deal-amount builder found in the tree — this derivation has stopped recognising " +
			"the shape it reads, and a census whose subject got smaller reports PASS either way")
	}
	return names
}

// builderName reports the declaration's name when it renders a deal amount as
// SQL. The two halves of that question are asked here and nowhere else, so a
// reader changing what counts as a builder changes it in one place.
func builderName(decl ast.Decl) (string, bool) {
	fn, isFunc := decl.(*ast.FuncDecl)
	if !isFunc || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		return "", false
	}
	result, isIdent := fn.Type.Results.List[0].Type.(*ast.Ident)
	if !isIdent || result.Name != "string" {
		return "", false
	}
	// Statements rather than literals, because a fragment assembled with `+` is
	// one statement to Postgres and several strings to the parser — and the
	// halves are exactly where the two patterns fall: `"CASE WHEN " + alias +
	// ".amount_minor IS NULL"` names the column in one piece and the keyword in
	// another, and a per-literal reading sees neither whole.
	for _, statement := range gatekit.SQLStatementsOf(fn) {
		if dealAmountColumn.MatchString(statement) && sqlFragmentMarker.MatchString(statement) {
			return fn.Name.Name, true
		}
	}
	return "", false
}

// dealAmountReadsIn is the site extraction the subject predicate above states.
func dealAmountReadsIn(decl ast.Decl, builders map[string]bool) []gatekit.TableRead {
	amounts := gatekit.DeclReads(decl, dealAmountColumn)
	if len(amounts) == 0 {
		amounts = builtAmountsIn(decl, builders)
	}
	if len(amounts) == 0 || len(gatekit.DeclReads(decl, dealTableRead)) == 0 {
		return nil
	}
	return amounts
}

// builtAmountsIn reports a site for a declaration that composes one of the
// builders rather than naming a column, so the report reads as a read.
func builtAmountsIn(decl ast.Decl, builders map[string]bool) []gatekit.TableRead {
	name := ""
	if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
		name = fn.Name.Name
	}
	var found []gatekit.TableRead
	ast.Inspect(decl, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if builders[calleeName(call)] {
			found = append(found, gatekit.TableRead{Function: name, SQL: calleeName(call) + "(...)"})
		}
		return true
	})
	return found
}

// TestTheBuilderDerivationReadsFragmentsAndNotProse holds the rule the
// derivation above turns on, in both directions.
//
// Under-recognition is the failure that matters: a builder this misses takes
// every statement composing it out of the census, and the census then reports
// PASS over a smaller tree. Over-recognition costs the other way and is not
// harmless either — deals.AmountCurrencyPairError.Error() names `amount_minor`
// in an English sentence, and admitting it would make every caller of an
// error's Error() a money read and bury the ones that are.
func TestTheBuilderDerivationReadsFragmentsAndNotProse(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		source  string
		want    string
		builder bool
	}{
		{
			name:    "a fold rendering the column is a builder",
			source:  "func baseValueSQL(alias string) string { return \"CASE WHEN \" + alias + \".amount_minor IS NULL THEN NULL END\" }",
			want:    "baseValueSQL",
			builder: true,
		},
		{
			name:    "the converted column counts the same",
			source:  "func total() string { return \"sum(d.amount_minor_base) FILTER (WHERE TRUE)\" }",
			want:    "total",
			builder: true,
		},
		{
			name:   "an error message naming the column is prose",
			source: "func (e *pairError) Error() string { return \"currency comes with amount_minor, and neither figure comes without it\" }",
		},
		{
			name:   "a fixture's JSON body is not a statement",
			source: "func createDeal() string { return `{\"amount_minor\": 25000}` }",
		},
		{
			name:   "SQL naming no amount column is somebody else's fragment",
			source: "func stageJoin() string { return \"JOIN stage s ON s.id = d.stage_id\" }",
		},
		{
			name:   "a function returning rows rather than SQL is not a builder",
			source: "func read() ([]string, error) { return nil, nil }",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package p\n"+c.source, 0)
			if err != nil {
				t.Fatalf("parsing the fixture: %v", err)
			}
			if len(file.Decls) != 1 {
				t.Fatalf("the fixture must hold exactly one declaration, it holds %d", len(file.Decls))
			}
			name, isBuilder := builderName(file.Decls[0])
			if isBuilder != c.builder {
				t.Fatalf("builderName = (%q, %v), want a builder: %v", name, isBuilder, c.builder)
			}
			if c.builder && name != c.want {
				t.Fatalf("builderName = %q, want %q", name, c.want)
			}
		})
	}
}

func TestEveryReaderOfADealAmountCarriesTheMaskOrAVerdict(t *testing.T) {
	t.Parallel()
	builders := dealAmountBuilderNames(t)
	files := maskedAmountScope(builders).Files(t)
	gated := maskGate.gatedFunctionsByPackage(t, files)
	consts := constantTable{}

	var satisfied int
	for _, parsed := range files {
		pkg := path.Dir(parsed.Path)
		for _, decl := range parsed.File.Decls {
			reads := dealAmountReadsIn(decl, builders)
			if len(reads) == 0 {
				continue
			}
			refs := referencesIn(decl, consts.of(t, pkg))
			subject := parsed.Path
			if reads[0].Function != "" {
				subject += ":" + reads[0].Function
			}
			// The declaration's OWN body, and deliberately NOT "reaches a mask
			// somewhere in this package".
			//
			// The other censuses in this directory resolve a gate through a
			// helper, because an admission taken at an entry point covers every
			// read below it. A MASK is not like that: it is a rendering of one
			// column inside one statement, so a sibling that applies it proves
			// nothing about this statement. The first version of this gate did
			// follow callees, and it read green over company360's deals band —
			// which calls the masked closedTotals and then selects
			// d.amount_minor unmasked in its own query. An integration test
			// against a real database is what caught it.
			carriesGate := maskGate.holdsSeedGate(refs, pkg)
			if reads[0].Function == "" {
				// A package-level statement has no declaration of its own to
				// ask, so the FILE is judged as a whole — the shape
				// restrictedreaders_test.go takes for the same case. This
				// tree composes a const query and renders its masked columns
				// in a sibling function, which is the arrangement that would
				// otherwise read as ungated.
				carriesGate = carriesGate || maskGate.fileHoldsAGatedFunction(t, parsed, gated[pkg], consts.of(t, pkg))
			}
			verdict := verdictIn(t, subject, maskVerdicts)
			switch {
			case carriesGate && verdict != "":
				t.Errorf("%s carries the field mask AND a %s verdict — remove the verdict, it now "+
					"describes code that is masked", subject, verdict)
			case carriesGate:
				satisfied++
			case verdict == "":
				t.Errorf("%s reads a deal's amount without the field mask and without a verdict.\n"+
					"  A mask withholds the figure on the deal list; a second surface that prints "+
					"it — or a total that sums it — hands back what the list withheld.\n"+
					"  Either render the column through auth.MaskedColumnSQL, filter the rows with "+
					"auth.MaskExcludedClause, or declare it in predicateMaskedAmountReads / "+
					"lifecycleMaskedAmountReads / calleeGatedMaskedAmountReads / "+
					"ruledMaskedAmountReads with the reason it needs neither.\n"+
					"  The read: %s", subject, gatekit.FirstLineOf(reads[0].SQL))
			}
		}
	}
	if satisfied < wantMinimumMaskedAmountSites {
		t.Errorf("only %d deal-amount reads carry the mask, want at least %d — an extractor that "+
			"stopped recognising this tree's SQL would report exactly this, and it reads the same "+
			"as a tree where every figure is guarded", satisfied, wantMinimumMaskedAmountSites)
	}
	t.Logf("deal-amount reads: %d masked, %d predicate, %d lifecycle, %d callee-gated, %d ruled, %d DEFERRED",
		satisfied, len(predicateMaskedAmountReads.Subjects()), len(lifecycleMaskedAmountReads.Subjects()),
		len(calleeGatedMaskedAmountReads.Subjects()), len(ruledMaskedAmountReads.Subjects()),
		len(deferredMaskedAmountReads.Subjects()))

	predicateMaskedAmountReads.AssertAllMatched(t)
	lifecycleMaskedAmountReads.AssertAllMatched(t)
	calleeGatedMaskedAmountReads.AssertAllMatched(t)
	notTheDealAmount.AssertAllMatched(t)
	ruledMaskedAmountReads.AssertAllMatched(t)
	deferredMaskedAmountReads.AssertAllMatched(t)
}

// TestTheObjectGateDoesNotVouchForAMask plants the shape this census could not
// see, in both directions.
//
// auth.Require(ctx, "deal", …) is the whole admission the deal-TABLE census
// asks for and no answer at all to this one — every masked read takes it too —
// and while one flag served both, a statement that ranked an account's deals by
// the figure a mask withholds reported as guarded.
//
// Both directions, because what was wrong is a distinction and not a
// tightening: a walk that stopped recognising the object gate where it IS the
// admission would fail every read dealreaders_test.go admits, and a census that
// fails what it should admit teaches its readers to distrust it.
func TestTheObjectGateDoesNotVouchForAMask(t *testing.T) {
	t.Parallel()
	const asksTheObjectGate = `func readDeals(ctx context.Context, tx pgx.Tx) error {
		if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
			return err
		}
		_, err := tx.Query(ctx, "SELECT d.amount_minor FROM deal d")
		return err
	}`
	const rendersTheMask = `func readDeals(ctx context.Context, arg func(any) int) (string, error) {
		return auth.MaskedColumnSQL(ctx, "deal", "amount_minor", "d", "amount_minor", arg)
	}`
	// Both gates are about the deal, so the census is named here rather than
	// read off gate.object — a failure saying "the deal census" would leave a
	// reader to guess which of the two it is.
	for _, c := range []struct {
		name    string
		census  string
		source  string
		gate    objectGate
		holding bool
	}{
		{
			name:   "the object gate answers nothing about a mask",
			census: "deal-amount mask",
			source: asksTheObjectGate,
			gate:   maskGate,
		},
		{
			name:    "the same body IS gated for the census the object gate answers",
			census:  "deal-table read",
			source:  asksTheObjectGate,
			gate:    dealGate,
			holding: true,
		},
		{
			name:    "a mask rendering is what this census asks for",
			census:  "deal-amount mask",
			source:  rendersTheMask,
			gate:    maskGate,
			holding: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package p\n"+c.source, 0)
			if err != nil {
				t.Fatalf("parsing the fixture: %v", err)
			}
			refs := referencesIn(file.Decls[0], map[string]string{})
			if got := c.gate.holdsSeedGate(refs, "internal/compose/company360"); got != c.holding {
				t.Errorf("the %s census reads this body as gated=%v, want %v", c.census, got, c.holding)
			}
		})
	}
}
