// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"path"
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

var maskGate = objectGate{
	object:    "deal",
	literal:   dealAmountColumn,
	gateSeeds: []string{"MaskedColumnSQL", "MaskExcludedClause", "MaskedFields", "MasksAnyRowOf", "maskedRowSelects"},
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

var maskedAmountScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: readsADealAmount,
	Exempt:  gatekit.Waive(map[string]string{}),
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
func readsADealAmount(filePath string, file *ast.File) bool {
	if strings.HasPrefix(filePath, maskOwner) || strings.HasSuffix(filePath, "_gen.go") {
		return false
	}
	for _, decl := range file.Decls {
		if len(dealAmountReadsIn(decl)) > 0 {
			return true
		}
	}
	return false
}

// dealAmountReadsIn is the site extraction the subject predicate above states.
func dealAmountReadsIn(decl ast.Decl) []gatekit.TableRead {
	amounts := gatekit.DeclReads(decl, dealAmountColumn)
	if len(amounts) == 0 || len(gatekit.DeclReads(decl, dealTableRead)) == 0 {
		return nil
	}
	return amounts
}

func TestEveryReaderOfADealAmountCarriesTheMaskOrAVerdict(t *testing.T) {
	t.Parallel()
	files := maskedAmountScope.Files(t)
	gated := maskGate.gatedFunctionsByPackage(t, files)
	consts := constantTable{}

	var satisfied int
	for _, parsed := range files {
		pkg := path.Dir(parsed.Path)
		for _, decl := range parsed.File.Decls {
			reads := dealAmountReadsIn(decl)
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
