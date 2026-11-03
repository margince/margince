// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every reason a row gives is one the contract declares and a client can render.
//
// A `because` entry is a typed pair — the server names the FACT and the client
// writes the phrase, because the product ships three languages and a sentence
// composed on the server would reach a German reader in English. That split has
// a cost: the kind is the whole of the agreement, and nothing enforces it.
//
// `WorklistReasonKind` is a string alias, so `reason("stale_deal", …)` compiles
// exactly as `reason("stale", …)` does. The wrong one is not a crash and not a
// validation error. The row renders, its rank is unchanged, and the one symptom
// is a line of evidence missing from the queue — the reader is simply not told
// why the row is there, on the surface whose entire job is to say why.
//
// So: every kind a producer emits must be declared, and every kind the contract
// declares must be renderable. The client drops a kind missing from
// `KNOWN_REASONS` (lib/worklistreasoncensus.ts) without a word, so the second
// direction is read from that census and the English catalog.

import (
	"go/ast"
	"go/parser"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// whereReasonsAreMinted is the package whose producers this gate reads.
//
// Named as a directory rather than as a list of files: a lane added tomorrow
// gets a new file, and a gate naming today's files would not read it — which is
// the shape of under-recognition this check must not have, since a smaller
// corpus reports PASS with nothing to notice.
const whereReasonsAreMinted = "internal/compose/attention"

const (
	frontendReasonCensus  = "../frontend/src/lib/worklistreasoncensus.ts"
	frontendReasonCatalog = "../frontend/src/i18n/en.ts"
)

// tsReasonCopyKey reads one `"worklist.because.<kind>":` key out of the English
// catalog. de.ts and vi.ts are held to the same keys by i18n.test.ts.
var tsReasonCopyKey = regexp.MustCompile(`"worklist\.because\.([a-z_]+)"\s*:`)

// TestEveryDeclaredReasonKindIsOneTheClientCanSay fails both ways. A declared
// kind the census lacks is a reason the row drops. A census entry the contract
// does not declare is one no server sends.
func TestEveryDeclaredReasonKindIsOneTheClientCanSay(t *testing.T) {
	t.Parallel()
	declared := crmYAMLEnum(t, "WorklistReason", "kind")
	census := clientReasonCensus(t)
	catalog, err := os.ReadFile(frontendReasonCatalog)
	if err != nil {
		t.Fatalf("reading the English catalog: %v", err)
	}
	worded := map[string]bool{}
	for _, m := range tsReasonCopyKey.FindAllStringSubmatch(tsComment.ReplaceAllString(string(catalog), " "), -1) {
		worded[m[1]] = true
	}
	if len(declared) == 0 || len(census) == 0 || len(worded) == 0 {
		t.Fatalf("read %d declared kinds, %d census entries and %d sentences: a census that reads nothing agrees with everything",
			len(declared), len(census), len(worded))
	}
	for _, kind := range declared {
		if !slices.Contains(census, kind) {
			t.Errorf("crm.yaml declares the reason kind %q and KNOWN_REASONS in %s lacks it: "+
				"the client drops it, and the row no longer says why it is there", kind, frontendReasonCensus)
		}
		if !worded[kind] {
			t.Errorf("the reason kind %q has no \"worklist.because.%s\" sentence in %s (add it to de.ts and vi.ts too)",
				kind, kind, frontendReasonCatalog)
		}
	}
	for _, kind := range census {
		if !slices.Contains(declared, kind) {
			t.Errorf("KNOWN_REASONS carries %q, which crm.yaml does not declare and no server sends: "+
				"remove it, or declare it in WorklistReason.kind", kind)
		}
	}
}

// clientReasonCensus reads the keys of the KNOWN_REASONS literal. Comments are
// stripped, so a kind only named in prose does not count as an entry.
func clientReasonCensus(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile(frontendReasonCensus)
	if err != nil {
		t.Fatalf("reading the client's reason census: %v", err)
	}
	const opener = "export const KNOWN_REASONS = {"
	start := strings.Index(string(source), opener)
	if start < 0 {
		t.Fatalf("no %q in %s: this gate reads that literal and cannot judge what it cannot find", opener, frontendReasonCensus)
	}
	body := string(source)[start+len(opener):]
	if end := strings.Index(body, "\n}"); end >= 0 {
		body = body[:end]
	}
	var kinds []string
	for _, m := range focusSurfaceEntry.FindAllStringSubmatch(tsComment.ReplaceAllString(body, ""), -1) {
		kinds = append(kinds, m[1])
	}
	slices.Sort(kinds)
	return slices.Compact(kinds)
}

func TestEveryReasonKindAProducerEmitsIsDeclared(t *testing.T) {
	t.Parallel()
	declared := crmYAMLEnum(t, "WorklistReason", "kind")
	if len(declared) == 0 {
		t.Fatal("read no reason kinds from crm.yaml: a census over an empty vocabulary passes without looking at anything")
	}
	emitted := reasonKindsEmitted(t)
	// The corpus must not be able to come back empty. A walk that matched
	// nothing — a renamed constructor, a moved package — would leave this gate
	// green over a tree it never read.
	if len(emitted) == 0 {
		t.Fatalf("found no reason(...) calls under %s: the walk reads nothing, so this gate "+
			"would pass whatever the producers emit", whereReasonsAreMinted)
	}
	for _, kind := range emitted {
		if !slices.Contains(declared, kind) {
			t.Errorf("a producer emits the reason kind %q, which crm.yaml does not declare: "+
				"the row would reach a reader with an evidence line no client can write. "+
				"Add it to WorklistReason.kind, or use one of %v", kind, declared)
		}
	}
}

// reasonKindsEmitted is every literal kind handed to `reason(...)` in the
// package that mints them.
//
// Held by: TestEveryReasonKindAProducerEmitsIsDeclared
// (backend/gates/worklistreasonkinds_test.go), whose own corpus check fails
// when this walk comes back empty — a renamed constructor or a moved package
// would otherwise leave it reading nothing and reporting PASS.
//
// Read as SYNTAX rather than as text. A grep for `reason("` matches the word in
// a comment and misses a call split across lines by the formatter; the AST
// matches the call itself, which is the thing this gate is about.
//
// Only literal arguments are collected, and that is honest rather than
// complete: a kind computed at runtime cannot be checked here, and the two that
// exist — the batch summariser's "routine" and "repeated_failure" — are
// returned from a helper rather than passed to `reason` directly. Those are
// covered by the frontend's own vocabulary check, and a comment claiming this
// gate saw them would be false.
func reasonKindsEmitted(t *testing.T) []string {
	t.Helper()
	found := map[string]bool{}
	root := whereReasonsAreMinted
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Tests are excluded deliberately: a fixture may mint a deliberately
		// wrong kind to prove a refusal, and failing on one would make this
		// gate argue with the tests that hold the behaviour.
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		parsed, parseErr := parser.ParseFile(gatekit.SourceFileSet(), path, src, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			name, ok := call.Fun.(*ast.Ident)
			if !ok || name.Name != "reason" {
				return true
			}
			if kind, literal := kindLiteral(call.Args[0]); literal {
				found[kind] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("reading the reason producers under %s: %v", whereReasonsAreMinted, err)
	}
	kinds := make([]string, 0, len(found))
	for kind := range found {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	return kinds
}

// kindLiteral reads the kind out of one argument, through the named conversion
// the call site may wrap it in.
//
// The reading itself is gatekit.StringExpr under FoldStrict — "is this
// definitely this string" — so this census never judges text it half-invented,
// and it resolves a constant handed to `reason` rather than seeing only literals.
//
// What this adds is the unwrapping of `crmcontracts.WorklistReasonKind("…")`.
// StringExpr follows the BUILTIN `string(…)` conversion and deliberately not a
// package-qualified one, since in general that names a function it would have
// to run. Here the outer name is the kind's own type, whose conversion is the
// identity on a string constant — so unwrapping exactly one such call and
// handing the inside back to StringExpr is sound.
//
// Held by mutation: writing a wrapped, undeclared literal into a producer fails
// TestEveryReasonKindAProducerEmitsIsDeclared.
func kindLiteral(arg ast.Expr) (string, bool) {
	if call, ok := arg.(*ast.CallExpr); ok && len(call.Args) == 1 {
		if _, qualified := call.Fun.(*ast.SelectorExpr); qualified {
			arg = call.Args[0]
		}
	}
	return gatekit.StringExpr(arg, nil, gatekit.FoldStrict)
}
