// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// maskableCatalog is what this build can withhold, one "<object> <field>" per
// line — the owner of this census's subject, and the only place it is named.
const maskableCatalog = "migrations/testdata/maskable_fields.txt"

// dealBaseAmount is the deal's money in its converted column, and no catalog
// entry: an administrator configures the FIELD, and this is its second column.
const dealBaseAmount = "amount_minor_base"

// notMoneyFields names the maskable deal fields this census does NOT read, and
// says which reads answer for each instead.
//
// A field the catalog gains is money until this register says otherwise, so the
// default is over-recognition. That direction costs a declaration; the other one
// is a census that reads a narrower tree and reports PASS with no assertion left
// to notice, which is the one way it may not fail.
var notMoneyFields = gatekit.Waive(map[string]string{
	"company_id":         "which account a deal hangs off, a pointer rather than a figure. It reaches a reader through the company the deal is listed under, so the read that has to withhold it is the one serving that name — and the same foreign key sits on a dozen other tables, which this census's compound subject cannot tell apart from the deal's own",
	"currency":           "the unit the money is quoted in. It never travels alone: auth's mask groups take the currency out with the amount, so a statement rendering the figure through the mask has already dropped it, and one that names the currency beside no figure discloses nothing about the size of the deal",
	"partner_company_id": "which partner is credited on the deal. The margin that makes the credit worth withholding is the partner's own field on its own object, and the pointer is a join key here in the same way company_id is",
	"project_id":         "which delivery the deal rolls up to, a pointer the filing and lock paths follow rather than a figure a surface prints. A reader who may not see the project does not reach it through this column: the project's own reads carry that gate",
})

// dealMaskedColumns is a deal's money however a statement names it.
//
// DERIVED from the catalog rather than restated here, because a gate that
// spells out part of its subject has become a second copy of it — and a money
// pair added to the catalog widens this census with no second edit.
//
// dealBaseAmount is the arm the catalog cannot give, and it is the SAME money
// converted. A mask on the field has to withhold both or the base column is the
// way around it, so a census reading the catalog spelling alone would read
// green over the total that discloses it.
//
// No arm for the report engine's late-bound token, and that is a decision about
// the ENGINE rather than about this pattern: the project report binds a mask
// CLAUSE beside the column rather than a rendering that replaces it, precisely
// so the figure keeps spelling itself in the statement this census reads. A
// token standing in for the whole expression would have taken the project
// report out of sight on the day it was first guarded.
func dealMaskedColumns(t testing.TB) *regexp.Regexp {
	t.Helper()
	columns, err := maskedColumnsIn(maskableCatalog, maskGate.object,
		func(field string) bool { return notMoneyFields.Waived(t, field) })
	if err != nil {
		t.Fatalf("deriving this census's subject: %v", err)
	}
	notMoneyFields.AssertAllMatched(t)
	return columns
}

// maskedColumnsIn compiles the object's money columns out of the catalog, over
// every field answeredElsewhere does not claim.
//
// It refuses rather than narrows. A catalog that is absent, unreadable, spelt
// in a way catalogPair cannot read, or naming this object nowhere would
// otherwise leave a pattern short of the columns it names, and the census would
// sweep the same tree and report PASS over every one it dropped.
func maskedColumnsIn(catalog, object string, answeredElsewhere func(string) bool) (*regexp.Regexp, error) {
	body, err := os.ReadFile(catalog)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", catalog, err)
	}
	columns := []string{dealBaseAmount}
	for number, line := range strings.Split(string(body), "\n") {
		named, field, lineErr := catalogPair(line)
		if lineErr != nil {
			return nil, fmt.Errorf("%s line %d: %w", catalog, number+1, lineErr)
		}
		if named == object && !answeredElsewhere(field) {
			columns = append(columns, field)
		}
	}
	if len(columns) == 1 {
		return nil, fmt.Errorf("%s offers no %s field this census reads, leaving %s the whole of the subject",
			catalog, object, dealBaseAmount)
	}
	sort.Strings(columns)
	return regexp.MustCompile(`(?i)\b(` + strings.Join(columns, "|") + `)\b`), nil
}

// catalogName is a column or object as the catalog may spell one.
var catalogName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// catalogPair reads one catalog line, empty names for a blank or a comment.
//
// A line it cannot read is an ERROR and never a line it skips, which is the
// whole reason it is a function. A tab between the pair, or a note after it,
// would otherwise leave an arm no statement can match — and the count above
// cannot notice, because the converted column seeds the list and keeps it from
// ever being short. The census would then sweep a narrower tree and report PASS
// with nothing to say so. Refusing a name outright also keeps a regexp
// metacharacter out of an alternation this compiles.
func catalogPair(line string) (object, field string, err error) {
	parts := strings.Fields(line)
	if len(parts) == 0 || strings.HasPrefix(parts[0], "#") {
		return "", "", nil
	}
	if len(parts) != 2 || !catalogName.MatchString(parts[0]) || !catalogName.MatchString(parts[1]) {
		return "", "", fmt.Errorf("%q is not an \"<object> <field>\" pair of identifiers", strings.TrimSpace(line))
	}
	return parts[0], parts[1], nil
}

// TestTheCensusSubjectIsDerivedAndRefusesACatalogItCannotRead holds the one
// direction this census may not fail in, and the widening that pays for it.
//
// A subject taken from a catalog that came back absent, empty, about some other
// object, or spelt in a way the parse drops would sweep exactly the same tree
// and report PASS over every column nobody masked, leaving no assertion to
// notice — so the derivation refuses instead. The admitting cases are here
// beside the refusals because a derivation that refused everything would
// satisfy them on its own and take the census with it, and because a field the
// catalog gains has to land in the subject without a second edit.
func TestTheCensusSubjectIsDerivedAndRefusesACatalogItCannotRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	catalogHolding := func(name, body string) string {
		written := filepath.Join(dir, name)
		if err := os.WriteFile(written, []byte(body), 0o600); err != nil {
			t.Fatalf("writing the catalog fixture: %v", err)
		}
		return written
	}
	const answeredElsewhere = "currency"
	for _, c := range []struct {
		name      string
		catalog   string
		subjects  []string
		strangers []string
	}{
		{name: "a catalog that is not there", catalog: filepath.Join(dir, "absent.txt")},
		{name: "a catalog of comments alone", catalog: catalogHolding("comments.txt", "# what this build can withhold\n\n")},
		{name: "a catalog naming only another object", catalog: catalogHolding("other.txt", "partner margin_tier\n")},
		{name: "a catalog whose only deal field is answered elsewhere", catalog: catalogHolding("elsewhere.txt", "deal "+answeredElsewhere+"\n")},
		// A line carrying anything past the pair. A cut on the first space read
		// these as a field with a passenger — an arm matching no statement,
		// while the count stayed long enough for the refusal above to pass.
		{name: "a pair carrying a note", catalog: catalogHolding("note.txt", "deal amount_minor # the headline figure\n")},
		{name: "a field that is not an identifier", catalog: catalogHolding("punctuated.txt", "deal amount.minor\n")},
		{
			name:      "the object's money, the converted column, and a pair the catalog has just gained",
			catalog:   catalogHolding("deal.txt", "# a comment\ndeal amount_minor\ndeal "+answeredElsewhere+"\ndeal retainer_minor\npartner margin_tier\n"),
			subjects:  []string{"d.amount_minor", "sum(d.amount_minor_base)", "d.retainer_minor"},
			strangers: []string{"d." + answeredElsewhere, "p.margin_tier", "d.stage_id"},
		},
		// Whitespace between the pair is read rather than refused, which is the
		// safe half of the same rule: a tab dropped the pair where a cut on the
		// first space was the parse, and dropping it is what the census cannot
		// survive. Refusing here would be honest and reading it is better.
		{
			name:      "a pair the catalog separates with a tab or pads with spaces",
			catalog:   catalogHolding("spaced.txt", "deal\tamount_minor\n   deal   expected_arr_minor   \n\n"),
			subjects:  []string{"d.amount_minor", "sum(d.amount_minor_base)", "d.expected_arr_minor"},
			strangers: []string{"d.stage_id"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			columns, err := maskedColumnsIn(c.catalog, "deal",
				func(field string) bool { return field == answeredElsewhere })
			if len(c.subjects) == 0 {
				if err == nil {
					t.Fatalf("this catalog compiled to %s rather than being refused — a subject that "+
						"narrows without saying so reads the same as a tree with nothing to mask", columns)
				}
				return
			}
			if err != nil {
				t.Fatalf("deriving the subject: %v", err)
			}
			for _, statement := range c.subjects {
				if !columns.MatchString(statement) {
					t.Errorf("%q is outside the derived subject, so this census would never read it", statement)
				}
			}
			for _, statement := range c.strangers {
				if columns.MatchString(statement) {
					t.Errorf("%q is inside the derived subject, which costs a declaration saying it is not", statement)
				}
			}
		})
	}
}

// WHAT THIS CENSUS CANNOT SEE, and why the thing it cannot see is covered
// anyway.
//
// It reads STATEMENTS. A package that takes the figure as a Go VALUE is
// invisible to it by construction — companybrief folding `deal.Amount` out of
// a Company360, the slipping tool folding it out of a listed Deal, meetingbrief
// reading it off a row it did not select. None of those names a column, and no
// widening of this pattern would find them.
//
// They are covered by COMPOSITION rather than by luck, and the composition has
// two halves. A contracts money field is only ever produced by SQL; every such
// statement lives under `internal`, which is this census's Roots. So a value
// that reaches a grounding package has already passed through a statement this
// census subjected — and the masks it carries are the ones that were applied.
//
// The second half is a premise, so it is asserted rather than assumed:
// TestNoDealAmountStatementLivesWhereTheCensusCannotSeeIt refuses a deal-amount
// statement anywhere outside `internal`. Without it, a reader placed in cmd/,
// pkg/ or an extension unit would not fail this census — it would be one this
// census never reads, and it would go on answering PASS.
//
// And it reads a mask's PRESENCE rather than its placement: a declaration
// rendering one somewhere in its body reads as gated even where the aggregate
// beside it folds the raw column. Telling those apart needs a statement's shape
// and not its text, so what holds it is an integration test per surface —
// company360's deals band and the project header each have one.
//
// The compound subject also wants both halves in ONE declaration, and the deal's
// own single read splits them: deals/deal_singleread.go names every column in
// the `dealColumns` var and the table in readDeal's own literal, so neither
// declaration is a site and the file is not in this census. The deal read and
// the deal list are covered instead by deals/fieldmask.go — auth.ApplyFieldMasks
// over dealWithholds, at the wire boundary rather than in the statement — which
// is a whole-record pass with its own tests. So inMaskOwner below says the
// module is reachable, not that its most direct reads are the ones reached.

var dealTableRead = gatekit.TableReadPattern("deal")

// maskOwner is the module that OWNS the deal object and the mask, and the one
// place this census may not stop reaching.
//
// It was exempt WHOLESALE once, on the reading that the owner of a mask applies
// it. What that bought was a project-header total summing every deal filed
// under a project with no mask on it at all — an aggregate the census could not
// see because of where it lived, while every sibling total outside the module
// was guarded. Owning the mask is the reason to read the module closely, not a
// reason to skip it: this is where a statement reaches the column most directly.
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
// No literal: this census reads the gate for its admission alone, and its
// subject is compiled per run from the catalog rather than pinned to a field.
var maskGate = objectGate{
	object: "deal",
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

// calleeGatedMaskedAmountReads: the declaration's own body holds no mask
// spelling, and what applies it sits elsewhere — bound into the statement after
// this file hands it over, or run across the rows once they are read.
var calleeGatedMaskedAmountReads = gatekit.Waive(map[string]string{
	"internal/compose/reportprojects.go":            "the project report's won-deal money total. It sums d.amount_minor_base under a FILTER on reportDealMaskToken, which reportsql.go binds from auth.MaskExcludedClause beside the row-scope token next to it — so the statement here is a template and the mask is resolved at bind time. The engine's own mask pass cannot cover this one: it asks the masks on the SPEC's entity, and the spec is projects while the sum is over deals",
	"internal/modules/deals/dealfigures.go:Figures": "the Worklist's batched card read. It selects d.amount_minor bare and maskFigures, the declaration above it in the same file, withholds the figure afterwards in Go — auth.MaskedFields over auth.WritableSubset — because a card carries no masked_fields list for a rendered NULL to explain itself in. The mask is a whole-row decision here rather than a rendering, which is the one shape this census reads as ungated",
})

// writePreimageMaskedAmountReads: the statement reads the deal's own figures
// inside a WRITE — for the patch's pre-image, or for the decision whether to
// assign the column at all — rather than to answer a reader.
//
// The verdict is about the STATEMENT and reaches no further. What each of these
// records lands in an audit image, and whether a trail serving that image
// withholds the right columns is that trail's own read to answer; this register
// does not vouch for it, and a reason here that claimed to would be a mitigation
// nobody can check from the statement it is written beside.
var writePreimageMaskedAmountReads = gatekit.Waive(map[string]string{
	"internal/modules/deals/basecurrencyfreezewrite.go:frozenBaseBefore": "the frozen base amount a re-price or a reopen is about to overwrite. amount_minor_base is an internal column on no contract, so the writer has no pre-image to hand over and reads the row for one; recording nil instead would write \"there was no converted amount\" into the audit diff of every reopen, which is the row a reversal reads to put the old figure back",
	"internal/modules/deals/forecasthistory.go:recordForecastMovement":   "INSERT INTO deal_forecast_history … SELECT FROM deal: the deal's state copied into its forecast trail, in the write's own transaction and after the patch landed. The statement answers a row count and no figure. Nothing in this tree READS that table yet, so the obligation belongs to whoever writes the first such read rather than to somebody who has already met it",
	"internal/modules/deals/offer_dealsync.go:syncDealAmountFromOffer":   "the deal's current price under a row lock, so an accepted offer's gross can be compared against it and the audit diff can name what it replaced. The comparison is what keeps an accept re-pricing at the figure already held out of the forecast trail. The pre-image it reads reaches only that diff: the money the function returns in p.After() is the offer's own gross, which its caller supplied",
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
// never spell the deal table in their source — but a statement that reads a
// FROZEN figure and joins the deal for its name matches both halves. A frozen contribution is not
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
	{"write-preimage", writePreimageMaskedAmountReads},
	{"not-the-amount", notTheDealAmount},
	{"ruled", ruledMaskedAmountReads},
	{"deferred", deferredMaskedAmountReads},
}

const wantMinimumMaskedAmountSites = 10

// maskedAmountScope is built around the derived builder set, so the sweep and
// the site extraction ask one question rather than two that can drift.
func maskedAmountScope(columns *regexp.Regexp, builders map[string]bool) gatekit.Scope {
	return gatekit.Scope{
		Roots: []string{"internal"},
		Subject: func(filePath string, file *ast.File) bool {
			return readsADealAmount(filePath, file, columns, builders)
		},
		Exempt: gatekit.Waive(map[string]string{}),
	}
}

// readsADealAmount is the compound subject, and the compound is the point.
//
// `amount_minor` is not a rare column name: forecast_call, finance_payment,
// commission_entry and the forecast snapshot each carry one, and a mask on
// deal.amount_minor says nothing about most of them. A pattern matching the
// column alone found forty reads that were not about a deal at all, which
// would have cost forty declarations saying so — noise that buries the signal
// this census exists to carry.
//
// Most, not all: a commission entry's basis IS the deal's amount, copied at
// accrual, and the ledger reads it under a mask arm resolved on the deal row
// (commissions/entryfieldmask.go). That one is held by its own test rather than
// by this census, which cannot see it — the reach is a runtime clause, not a
// FROM clause this scan could match.
//
// So a file is a subject when it holds a DECLARATION whose SQL both reads the
// deal table and names an amount column. Per declaration rather than per
// literal, because this tree routinely composes one statement from several
// constants, and a literal-level test would stop seeing a read the moment
// somebody moved its FROM clause into a fragment.
func readsADealAmount(filePath string, file *ast.File, columns *regexp.Regexp, builders map[string]bool) bool {
	if strings.HasSuffix(filePath, "_gen.go") {
		return false
	}
	for _, decl := range file.Decls {
		if len(dealAmountReadsIn(decl, columns, builders)) > 0 {
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
func dealAmountBuilderNames(t testing.TB, columns *regexp.Regexp) map[string]bool {
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
			if name, isBuilder := builderName(decl, columns); isBuilder {
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
func builderName(decl ast.Decl, columns *regexp.Regexp) (string, bool) {
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
		if columns.MatchString(statement) && sqlFragmentMarker.MatchString(statement) {
			return fn.Name.Name, true
		}
	}
	return "", false
}

// dealAmountReadsIn is the site extraction the subject predicate above states.
func dealAmountReadsIn(decl ast.Decl, columns *regexp.Regexp, builders map[string]bool) []gatekit.TableRead {
	amounts := gatekit.DeclReads(decl, columns)
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
	columns := dealMaskedColumns(t)
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
			name, isBuilder := builderName(file.Decls[0], columns)
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
	columns := dealMaskedColumns(t)
	builders := dealAmountBuilderNames(t, columns)
	files := maskedAmountScope(columns, builders).Files(t)
	gated := maskGate.gatedFunctionsByPackage(t, files)
	consts := constantTable{}

	var satisfied, inMaskOwner int
	for _, parsed := range files {
		pkg := path.Dir(parsed.Path)
		if strings.HasPrefix(parsed.Path, maskOwner) {
			inMaskOwner++
		}
		for _, decl := range parsed.File.Decls {
			reads := dealAmountReadsIn(decl, columns, builders)
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
					"writePreimageMaskedAmountReads / ruledMaskedAmountReads with the reason "+
					"it needs neither.\n"+
					"  The read: %s", subject, gatekit.FirstLineOf(reads[0].SQL))
			}
		}
	}
	if satisfied < wantMinimumMaskedAmountSites {
		t.Errorf("only %d deal-amount reads carry the mask, want at least %d — an extractor that "+
			"stopped recognising this tree's SQL would report exactly this, and it reads the same "+
			"as a tree where every figure is guarded", satisfied, wantMinimumMaskedAmountSites)
	}
	if inMaskOwner == 0 {
		t.Errorf("no file under %s is in this census, though the module owns the deal, the column "+
			"and the mask. That module was exempt wholesale once and an unguarded project-header "+
			"total lived inside it the whole time: a subject that stops reaching the owner sweeps "+
			"a smaller tree and reports PASS with nothing left to notice", maskOwner)
	}
	t.Logf("deal-amount reads: %d masked (%d files in the mask owner), %d predicate, %d lifecycle, "+
		"%d callee-gated, %d write-preimage, %d ruled, %d DEFERRED",
		satisfied, inMaskOwner, len(predicateMaskedAmountReads.Subjects()), len(lifecycleMaskedAmountReads.Subjects()),
		len(calleeGatedMaskedAmountReads.Subjects()), len(writePreimageMaskedAmountReads.Subjects()),
		len(ruledMaskedAmountReads.Subjects()), len(deferredMaskedAmountReads.Subjects()))

	predicateMaskedAmountReads.AssertAllMatched(t)
	lifecycleMaskedAmountReads.AssertAllMatched(t)
	calleeGatedMaskedAmountReads.AssertAllMatched(t)
	writePreimageMaskedAmountReads.AssertAllMatched(t)
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
