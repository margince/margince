// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A markdown fence around a model's JSON is presentation, and no site may read
// it as a malformed answer.
//
// A model asked for structured output usually gives it; when one wraps the
// document in ```json instead, it has still answered correctly. ai.Unfence is
// the single reduction that settles this, and internal/modules/ai/output.go
// says so outright: "one reduction defines what every downstream shape check
// and gate parses — the callers must not each invent their own trim."
//
// That claim went false. Six model-reply parsers reached production reading
// their reply with a bare json.Unmarshal or a strings.TrimSpace, so a fencing
// model lost those sites entirely: the parser reported malformed JSON and the
// caller dropped to its deterministic floor, which reads to an operator exactly
// like a weak model. It surfaced as two `invalid` verdicts in a certification
// run — the cheapest possible place to find it, and only because the candidate
// happened to fence where the incumbent bindings do not.
//
// Two halves, because either alone fails short:
//
//   - The BEHAVIOUR half drives each parser with a fenced reply and an unfenced
//     one and demands the same outcome. A spelling check cannot see a parser
//     that calls ai.Unfence and then unmarshals the untouched string.
//   - The CENSUS half reads the tree for model-reply parsers and fails when one
//     is not in the table below. Without it this gate covers the six that were
//     found and says nothing about the seventh, which is how the first six got
//     here.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/meetingbrief"
	"github.com/margince/margince/backend/internal/compose/proposeroles"
	"github.com/margince/margince/backend/internal/compose/weekly/learnings"
	"github.com/margince/margince/backend/internal/compose/weekly/narrative"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// replyParser is one site's reading of a model reply, reduced to what this gate
// can compare: the refusal reason, or empty when the reply was accepted.
//
// An error STRING rather than a bool, because "both refused" is not agreement —
// a parser that refuses the unfenced reply for citing nothing and the fenced one
// for a backtick has changed its answer, and a bool cannot tell.
type replyParser struct {
	// site names the invocation site an operator would see fail, not the Go
	// function, because that is what a reader has to go and look at.
	site string
	// pkgFunc is the parser this row drives, spelled as the census reports it.
	// The two halves agreeing about which function is covered is held by
	// TestTheCensusSeesEveryKnownModelReplyParser, not by matching spellings
	// here.
	pkgFunc string
	read    func(reply string) string
}

// errText reduces a parser's outcome to what this gate compares. Shared by
// every row so the rows differ only in which parser they drive.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// modelReplyParsers are the model-reply parsers this gate can drive directly.
//
// NOT the whole set, and the difference matters: runner.parseStep is unexported
// in another package, so nothing here can call it. The complete set is the
// census's business, and it covers what this table cannot.
//
// The replies are minimal and deliberately NOT grounded in any Input: this gate
// is about the fence and nothing else, so each row is driven with a zero Input
// and both calls are refused for the same grounding reason. What may not differ
// is that reason.
//
// The agent loop's runner.parseStep is absent from this table and NOT from the
// census: it is unexported in another package, so nothing here can call it. Its
// behaviour half is TestAStepSurvivesTheManners, beside the parser. Said out
// loud because "not in the table" is otherwise indistinguishable from "not
// covered", which is the reading that lets a site go unchecked.
func modelReplyParsers() []replyParser {
	return []replyParser{
		{
			site:    "meeting_brief",
			pkgFunc: "meetingbrief.ParseBriefSections",
			read: func(reply string) string {
				_, err := meetingbrief.ParseBriefSections(reply, meetingbrief.Input{})
				return errText(err)
			},
		},
		{
			site:    "meeting_plan",
			pkgFunc: "meetingbrief.ParsePlan",
			read: func(reply string) string {
				_, err := meetingbrief.ParsePlan(reply, meetingbrief.Input{}, meetingbrief.Plan{})
				return errText(err)
			},
		},
		{
			site:    "deal_health",
			pkgFunc: "dealstatus.ParseStatus",
			read: func(reply string) string {
				_, err := dealstatus.ParseStatus(reply, dealstatus.StatusInput{})
				return errText(err)
			},
		},
		{
			site:    "propose_roles",
			pkgFunc: "proposeroles.Parse",
			read: func(reply string) string {
				_, err := proposeroles.Parse(reply)
				return errText(err)
			},
		},
		{
			site:    "weekly_review",
			pkgFunc: "narrative.Parse",
			read: func(reply string) string {
				_, err := narrative.Parse(reply, narrative.Input{})
				return errText(err)
			},
		},
		{
			site:    "weekly_learnings",
			pkgFunc: "learnings.Parse",
			read: func(reply string) string {
				_, err := learnings.Parse(reply, learnings.Input{})
				return errText(err)
			},
		},
	}
}

// fencedRepliesReadTheSameAsPlainOnes is the behaviour half.
//
// Held by: TestAFenceNeverChangesWhatASiteReads (backend/gates/modelreplyfence_test.go) — this test.
func TestAFenceNeverChangesWhatASiteReads(t *testing.T) {
	t.Parallel()
	// A shape no parser here declares, so every row refuses it for its own
	// reason rather than one row's schema happening to accept it. The point is
	// that the reason is the SAME either way.
	const reply = `{"unrelated":"payload"}`
	for _, parserRow := range modelReplyParsers() {
		t.Run(parserRow.site, func(t *testing.T) {
			t.Parallel()
			plain := parserRow.read(reply)
			fenced := parserRow.read("```json\n" + reply + "\n```")
			if plain != fenced {
				t.Errorf("%s (%s): a fence changed the reading.\n  unfenced: %s\n  fenced:   %s\n"+
					"A ```json fence is presentation — the model answered. Reduce through ai.Unfence "+
					"before json.Unmarshal, as internal/modules/ai/output.go says every caller must.",
					parserRow.site, parserRow.pkgFunc, orAccepted(plain), orAccepted(fenced))
			}
		})
	}
}

// orAccepted renders an empty refusal as words, so a failure message never reads
// as though a parser returned nothing at all.
func orAccepted(refusal string) string {
	if refusal == "" {
		return "(accepted)"
	}
	return refusal
}

// replyParamNames are the parameter names a model reply arrives under at the
// sites this gate protects.
//
// A NAME-based census is the weak point and is stated rather than hidden: a
// parser taking its reply as `s` would not be seen. The names are read off the
// six known parsers and the ones that already reduce correctly, which is the
// vocabulary this tree actually uses; a new spelling is the case to add here,
// and TestTheCensusSeesEveryKnownModelReplyParser is what fails if the census
// stops finding the parsers the table drives.
var replyParamNames = []string{"reply", "raw", "text", "modelText", "content", "output", "replyText"}

// modelReplyUnmarshalSites reads the tree for a function that takes a model
// reply as a string and json.Unmarshals it, and reports whether that unmarshal
// reduces through ai.Unfence.
//
// Production files only: a _test.go file may unmarshal a literal on purpose.
func modelReplyUnmarshalSites(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	sites := map[string]bool{}
	// internal/, not internal/compose. The first version of this census walked
	// compose alone and so never saw the agent loop's own step parser, which
	// lives in a module — the walk-root blind spot this file warned about, found
	// by widening it rather than by reasoning about it.
	//
	// Relative to backend/, not to this package: gates_test.go's TestMain chdirs
	// one level up so every gate reads the tree from the same place.
	root := "internal"
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			// FAIL, never skip: a file this gate cannot read is a file it cannot
			// clear, and treating it as clean is the under-recognition this
			// census exists to avoid.
			t.Fatalf("parsing %s: %v", path, parseErr)
		}
		pkg := file.Name.Name
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			if _, takesAReply := replyParamOf(fn); !takesAReply {
				continue
			}
			// A decoder is usually built in one statement and read in the
			// next, so its SOURCE has to be carried between them before any
			// decode can be judged.
			sources := decoderSources(fn.Body)
			// The reduction is often a statement of its own —
			// `cleaned := modelreply.Unfence(text)` — so reaching it means
			// following what a name was assigned from.
			assigned := assignedFrom(fn.Body)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, isCall := n.(*ast.CallExpr)
				if !isCall {
					return true
				}
				decoded, decodes := decodedExpr(call, sources)
				if !decodes {
					return true
				}
				// EVERY unmarshal in the body, not only one whose argument
				// spells the parameter. Requiring the name matched the reply
				// variable made this census fail short on its first run:
				// narrative.Parse assigns strings.TrimSpace(reply) to a local
				// and unmarshals THAT, so the site went unseen — the exact
				// under-recognition that reports PASS with nothing failing.
				// Following the alias would be one more shape to miss, so the
				// rule is coarser and errs loudly: a parser handed a model
				// reply reduces everything it decodes, and a genuine unrelated
				// decode is a waiver with a reason rather than a silent gap.
				name := pkg + "." + fn.Name.Name
				// OR, not assignment: one function may unmarshal twice (a
				// re-ask path), and a single reduced site does not clear the
				// other. Any un-reduced read is a finding.
				sites[name] = sites[name] || !reducesThroughUnfence(decoded, assigned)
				return true
			})
		}
		return nil
	}); err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return sites
}

// replyParamOf answers which parameter of fn carries the model reply, if any.
func replyParamOf(fn *ast.FuncDecl) (string, bool) {
	if fn.Type.Params == nil {
		return "", false
	}
	for _, field := range fn.Type.Params.List {
		ident, isIdent := field.Type.(*ast.Ident)
		if !isIdent || ident.Name != "string" {
			continue
		}
		for _, name := range field.Names {
			if slices.Contains(replyParamNames, name.Name) {
				return name.Name, true
			}
		}
	}
	return "", false
}

// decoderSources maps each json.NewDecoder assigned to a variable in body to
// the READER it was built over, so `dec := json.NewDecoder(r)` followed by
// `dec.Decode(&v)` can be judged on r.
//
// Needed because the interesting decoder in this tree is exactly that shape:
// runner.parseStep builds one in its own statement so it can set
// DisallowUnknownFields. A check that only understood the chained spelling
// walked straight past it — this census reported 36 sites and NONE of them was
// parseStep, while a comment above claimed the census covered what the
// behaviour table could not.
func decoderSources(body *ast.BlockStmt) map[string]ast.Expr {
	sources := map[string]ast.Expr{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, isAssign := n.(*ast.AssignStmt)
		if !isAssign || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		name, isIdent := assign.Lhs[0].(*ast.Ident)
		call, isCall := assign.Rhs[0].(*ast.CallExpr)
		if !isIdent || !isCall || !isNewDecoder(call) || len(call.Args) == 0 {
			return true
		}
		sources[name.Name] = call.Args[0]
		return true
	})
	return sources
}

// assignedFrom maps each name assigned a single value in body to that value, so
// a check can follow `cleaned := modelreply.Unfence(text)` to the call that
// reduced it.
func assignedFrom(body *ast.BlockStmt) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, isAssign := n.(*ast.AssignStmt)
		if !isAssign || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		if name, isIdent := assign.Lhs[0].(*ast.Ident); isIdent {
			out[name.Name] = assign.Rhs[0]
		}
		return true
	})
	return out
}

// reducesThroughUnfence reports whether what a decode reads came through the
// reduction, following assignments a bounded number of hops.
//
// BOUNDED rather than exhaustive, and the direction of the error is the point:
// a chain longer than this reports the site as unreduced, which is a finding a
// reader can dismiss in a second. Following forever, or giving up and calling
// it clean, is the under-recognition this census exists to avoid.
func reducesThroughUnfence(expr ast.Expr, assigned map[string]ast.Expr) bool {
	const hops = 4
	frontier := []ast.Expr{expr}
	for range hops {
		var next []ast.Expr
		for _, e := range frontier {
			if expressionMentions(e, "Unfence") {
				return true
			}
			ast.Inspect(e, func(n ast.Node) bool {
				if name, isIdent := n.(*ast.Ident); isIdent {
					if from, known := assigned[name.Name]; known {
						next = append(next, from)
					}
				}
				return true
			})
		}
		if len(next) == 0 {
			return false
		}
		frontier = next
	}
	return false
}

// isNewDecoder reports whether call is json.NewDecoder(...).
func isNewDecoder(call *ast.CallExpr) bool {
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != "NewDecoder" {
		return false
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	return isIdent && pkg.Name == "json"
}

// decodedExpr answers the expression a JSON decode READS, in either spelling
// this tree uses, and whether call decodes at all.
//
// The expression, not the call's first argument: json.Unmarshal takes the bytes
// it reads, but Decode takes the DESTINATION — `dec.Decode(&step)` — and asking
// whether &step mentions Unfence would clear every decoder in the tree for the
// wrong reason. What a Decode reads is the reader its decoder was built over.
func decodedExpr(call *ast.CallExpr, sources map[string]ast.Expr) (ast.Expr, bool) {
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, false
	}
	if sel.Sel.Name == "Unmarshal" {
		pkg, isIdent := sel.X.(*ast.Ident)
		if isIdent && pkg.Name == "json" && len(call.Args) > 0 {
			return call.Args[0], true
		}
		return nil, false
	}
	if sel.Sel.Name != "Decode" {
		return nil, false
	}
	// `dec.Decode(&v)`, where dec was built earlier in this function.
	if name, isIdent := sel.X.(*ast.Ident); isIdent {
		if source, built := sources[name.Name]; built {
			return source, true
		}
		return nil, false
	}
	// `json.NewDecoder(r).Decode(&v)`, chained.
	if inner, isCall := sel.X.(*ast.CallExpr); isCall && isNewDecoder(inner) && len(inner.Args) > 0 {
		return inner.Args[0], true
	}
	return nil, false
}

// expressionMentions reports whether ident appears anywhere inside expr — the reply
// variable, or the Unfence call that should be wrapping it.
func expressionMentions(expr ast.Expr, ident string) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if name, isIdent := n.(*ast.Ident); isIdent && name.Name == ident {
			found = true
		}
		return !found
	})
	return found
}

// notAModelReply names a site the census matches that does not read a model
// reply at all, with the reason it does not.
//
// gatekit.Waive rather than a bare map: it holds every entry to a reason, and
// AssertAllMatched reports one that has stopped matching — so a waiver cannot
// outlive its subject and quietly widen into a hole. A hand-rolled map plus a
// staleness test of my own was a second implementation of this package, which
// is the duplication the census itself is about.
//
// The census matches on parameter NAME, and `content` is the vocabulary an
// upload uses as readily as a completion does. That is the whole cost of a
// name-based scan, paid here in one line rather than by narrowing the scan and
// missing a real site.
var notAModelReply = gatekit.Waive(map[string]string{
	"ai.decodeTranscriptItems": "reads the transcript an OPERATOR uploaded to the voice corpus, not a completion — " +
		"its `content` is a form field, and there is no model whose manners could have wrapped it",
})

// The census half: no model-reply parser reads its reply without reducing it.
//
// Held by: TestNoModelReplyParserSkipsTheFenceReduction (backend/gates/modelreplyfence_test.go) — this test.
func TestNoModelReplyParserSkipsTheFenceReduction(t *testing.T) {
	t.Parallel()
	var unreduced []string
	for name, skipsUnfence := range modelReplyUnmarshalSites(t) {
		// The offence is decided FIRST and the waiver asked about it second.
		// Asked as a pre-filter this would discard the entry's site whatever it
		// did, including a real regression introduced there later — gatekit's
		// Waived says so outright, and TestEveryWaiverIsAskedAboutAnOffenderNotACandidate
		// holds the rest of the tree to it.
		if !skipsUnfence {
			continue
		}
		if notAModelReply.Waived(t, name) {
			continue
		}
		unreduced = append(unreduced, name)
	}
	// A waiver describing a site that no longer offends, or no longer exists, is
	// reported here rather than left to widen.
	notAModelReply.AssertAllMatched(t)
	slices.Sort(unreduced)
	if len(unreduced) > 0 {
		t.Errorf("%d model-reply parser(s) json.Unmarshal their reply without ai.Unfence: %s\n"+
			"A model that wraps its JSON in a ```json fence has answered correctly, and these sites "+
			"read that as malformed — the caller then serves its deterministic floor and the deployment "+
			"looks like a weak model. internal/shared/kernel/modelreply owns the reduction.",
			len(unreduced), strings.Join(unreduced, ", "))
	}
}

// The census must find the parsers the behaviour table drives.
//
// This is the half that keeps the other half honest. A census that silently
// stops matching — a renamed parameter, a moved directory, an AST shape it does
// not walk — reads a smaller tree, finds nothing, and reports PASS with no
// failing assertion anywhere. Pinning it to the table means the table's own rows
// are the floor: the census may find MORE than these, never fewer.
//
// Held by: TestTheCensusSeesEveryKnownModelReplyParser (backend/gates/modelreplyfence_test.go) — this test.
func TestTheCensusSeesEveryKnownModelReplyParser(t *testing.T) {
	t.Parallel()
	seen := modelReplyUnmarshalSites(t)
	// runner.parseStep by NAME, because no row can drive it: it is unexported in
	// another package. It is also the parser this whole gate came from, and the
	// census could not see it for a while — it builds its decoder in its own
	// statement, and a check that understood only the chained spelling reported
	// 36 sites with this one absent. Named here so that cannot recur quietly.
	if _, found := seen["runner.parseStep"]; !found {
		t.Error("the census does not see runner.parseStep, the agent loop's own step parser — " +
			"it decodes with json.NewDecoder assigned to a variable, so a scan that reads only " +
			"json.Unmarshal or the chained decoder walks past it")
	}
	for _, parserRow := range modelReplyParsers() {
		if _, found := seen[parserRow.pkgFunc]; !found {
			t.Errorf("the census did not find %s, which the behaviour table drives as site %s — "+
				"so it is reading a smaller tree than it claims and would report PASS over a "+
				"parser that skipped the reduction. Check replyParamNames and the walk root.",
				parserRow.pkgFunc, parserRow.site)
		}
	}
	if len(seen) < len(modelReplyParsers()) {
		t.Errorf("the census found %d model-reply parser(s) and the table names %d; "+
			"a census that can find fewer than the known set has already failed short",
			len(seen), len(modelReplyParsers()))
	}
	// Said out loud so the number is not mistaken for a ceiling: the tree has
	// more correct parsers than the table has broken ones, and the census should
	// be seeing those too.
	t.Logf("census sees %d model-reply parser(s) across internal/", len(seen))
	if testing.Verbose() {
		names := make([]string, 0, len(seen))
		for name := range seen {
			names = append(names, name)
		}
		slices.Sort(names)
		t.Log(strings.Join(names, "\n"))
	}
}
