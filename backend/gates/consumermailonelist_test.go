// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

package gates

// One consumer-mail list, held by a test rather than by a comment.
//
// `platform/freemail`'s own package doc says it exists because "two modules
// need the same answer from opposite ends of the capture path" and "a second
// spelling of the list would be a second answer". That was a claim with nothing
// behind it, and the second spelling was already in the tree: the agent tool
// `qualify_lead` carried a FIFTEEN-domain map compiled into the binary against
// the platform baseline's 8,758 plus the workspace's own administered overlay.
//
// The two doors disagreed in front of a user rather than in a log. An address
// at zoho.com, yandex.ru, fastmail.com or mail.com got a company named after
// the mailbox host from the agent door — "Zoho" as somebody's employer — while
// the web door refused to derive anything from it at all. And an operator who
// marked a host consumer in their own workspace was honoured at one door and
// ignored at the other, because a compiled-in map cannot read a table.
//
// The subject is DERIVED twice over: every composite literal in the tree that
// names two or more consumer mailbox providers, with "is a provider" answered
// by `platform/freemail`'s own shipped dataset rather than by a list kept here.
// Two is the threshold because one provider in a literal is a test fixture or a
// single named case, while two is a list.
//
// WHAT THIS DOES NOT CATCH, deliberately: a list built from a file, a constant,
// or a database read. A gate that tried to follow those would need to run them,
// and nothing in this tree writes one — the defect this exists over was a
// literal, and so is every plausible next one.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The subject set comes from the OWNER, not from a sample.
//
// A hand-written sample of the providers would be a SECOND consumer-mail list
// inside the gate that forbids second consumer-mail lists — and an incomplete
// one, since a sample cannot hold 8,758 entries. `[]string{"laposte.net",
// "hotmail.fr"}` is a real two-provider list that any plausible sample misses.
//
// Asking `platform/freemail` itself is exhaustive by construction (8,758
// entries plus this repo's pins), and it cannot drift: the dataset is
// deliberately re-syncable, and a re-sync now widens this gate rather than
// silently orphaning entries in a copy nobody re-checks.
var baselineDomains = sync.OnceValue(func() map[string]struct{} {
	set := make(map[string]struct{})
	for _, domain := range freemail.Domains() {
		set[domain] = struct{}{}
	}
	return set
})

// THERE IS NO FILE-SKIP SHORTCUT, and the absence is deliberate.
//
// The obvious optimisation is a cheap textual scan deciding which files are
// worth parsing. It does not pay: parsing every file is 2.49s, and adding such
// a scan made it 2.95s — the scan costs more than the parse it avoids. What
// makes this census cheap is the map lookup below; an inner loop over all
// 8,758 baseline domains, replaced by one hash probe, is the difference
// between about a minute and a couple of seconds.
//
// The correctness argument outlives the timing one. A textual scan must
// recognise every spelling the AST reader does, and Go offers many: raw
// backtick strings, escapes that cook to a name the source never spells,
// constants aliasing constants, concatenation at any depth, constant
// conversions, and a comment sitting between an operand and its operator. A
// scan narrower than the reader in ANY of those drops a file before anything
// looks at it — no finding, no error, indistinguishable from a clean tree.
// The planted cases below name each spelling; each is a way to be wrong
// silently.
//
// If a shortcut ever looks necessary: measure first.
//
// The same reasoning applies to the OWNER list below. `platform/freemail` is
// named as the one package allowed to declare providers; widening that to a
// pattern would let a second declaration join the exemption without anybody
// deciding it should.

// looksLikeADomain screens a string literal before the baseline is consulted.
//
// Membership alone is not the question: an ADDRESS at a provider names a
// person, not a list, and `jane@gmail.com` in a fixture is not somebody
// re-declaring the list. So the literal has to be a bare domain — a dot, no
// user part, no path, no space — before it counts.
func looksLikeADomain(value string) bool {
	if !strings.Contains(value, ".") || strings.ContainsAny(value, "@/\\ \t") {
		return false
	}
	_, known := baselineDomains()[strings.ToLower(value)]
	return known
}

// The package that OWNS the answer, including its own tests: the baseline's
// pins live there and its suite has to name providers to assert about them.
// A package PREFIX rather than a file list, because the whole package is the
// owner — but nothing wider, since widening this to a pattern is how a second
// list joins the exemption without anybody deciding it should.
const consumerMailOwner = "internal/platform/freemail/"

func TestOnlyOnePackageDeclaresConsumerMailProviders(t *testing.T) {
	t.Parallel()
	found := consumerMailListsIn(t, "internal")
	if len(found) > 0 {
		t.Fatalf("a second consumer-mail list — ask platform/freemail instead:\n%s",
			strings.Join(found, "\n"))
	}
}

// The gate's own defect test. A census of zero cannot tell a clean tree from a
// blind detector, and this one was written over a literal that had been sitting
// in the tree for months.
func TestConsumerMailCensusSeesTheShapesThatShipped(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		code string
		want int
	}{
		{
			name: "the map qualify_lead really carried",
			code: `package p
var freemailDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "yahoo.com": true,
}`,
			want: 1,
		},
		{
			// Two providers that share no prefix with the commoner ones, so a
			// sampled catalog misses them and anything built from one is blind
			// to a file spelling only these.
			name: "two providers a sampled catalog would miss",
			code: `package p
var providers = []string{"laposte.net", "hotmail.fr"}`,
			want: 1,
		},
		{
			name: "the same list as a slice",
			code: `package p
var providers = []string{"gmail.com", "outlook.com"}`,
			want: 1,
		},
		{
			// De-duplicating a hand-written map through constants puts only
			// ONE string literal in the list itself.
			name: "a list assembled from a named constant",
			code: `package p
const gmail = "gmail.com"
var providers = []string{gmail, "outlook.com"}`,
			want: 1,
		},
		{
			// The EXPRESSION dimension. `"gmail" + ".com"` is a legal spelling
			// of the domain, and a reader of string literals alone sees two
			// fragments and no provider — in BOTH paths: the census counted one
			// provider and reported nothing, and a list written entirely this
			// way was dropped by that shortcut before anything parsed it.
			name: "a constant built by concatenation",
			code: `package p
const gmail = "gmail" + ".com"
var providers = []string{gmail, "outlook.com"}`,
			want: 1,
		},
		{
			name: "a list whose every domain is concatenated",
			code: `package p
var providers = []string{"gmail" + ".com", "outlook" + ".com"}`,
			want: 1,
		},
		{
			// FIVE fragments, because `stringValue` resolves `+` to whatever
			// depth it is written at: any bound on the count is a cliff, not a
			// rule.
			name: "a list spelling every provider in five fragments",
			code: `package p
var providers = []string{"g" + "m" + "a" + "il" + ".com", "out" + "l" + "o" + "ok" + ".com"}`,
			want: 1,
		},
		{
			// `string(x)` on a string constant is the identity, not a call this
			// gate has to run.
			name: "a constant string conversion",
			code: `package p
const gmail = string("gmail.com")
var providers = []string{gmail, string("outlook.com")}`,
			want: 1,
		},
		{
			name: "concatenation in parentheses, and across three parts",
			code: `package p
var providers = []string{("gmail" + ".com"), "out" + "look" + ".com"}`,
			want: 1,
		},
		{
			// The escape dimension: the detector unquotes, so it counts this —
			// and a shortcut comparing the RAW captured text dropped the file
			// before anything parsed it.
			name: "providers written with escapes, which unquote to the real names",
			code: `package p
var providers = []string{"gmail\x2ecom", "outlook\u002ecom"}`,
			want: 1,
		},
		{
			name: "a raw-string list, written with backticks",
			code: "package p\nvar providers = []string{`gmail.com`, `outlook.com`}",
			want: 1,
		},
		{
			name: "a CHAIN of constants, where one indirection hid the list",
			code: `package p
const gmailBase = "gmail.com"
const gmail = gmailBase
var providers = []string{gmail, "outlook.com"}`,
			want: 1,
		},
		{
			name: "providers as VALUES rather than keys",
			code: `package p
var byRegion = map[string]string{"de": "gmx.de", "fr": "gmail.com"}`,
			want: 1,
		},
		{
			name: "a nested literal, which a top-level-only scan would miss",
			code: `package p
var cfg = config{Hosts: []string{"proton.me", "tutanota.com"}}`,
			// The outer literal and the inner one both enclose the pair, and
			// both are reported: naming only the outer would point a reader at
			// the config type rather than at the list.
			want: 2,
		},
		// What must stay invisible.
		{
			name: "ONE provider, which is a named case rather than a list",
			code: `package p
var f = fixture{Email: "jane@gmail.com"}`,
			want: 0,
		},
		{
			name: "the same provider twice, which is still one domain",
			code: `package p
var f = []string{"gmail.com", "gmail.com"}`,
			want: 0,
		},
		{
			name: "addresses rather than domains, which name people not a list",
			code: `package p
var f = []string{"jane@gmail.com", "otto@web.de"}`,
			want: 0,
		},
		{
			name: "the adopted spelling",
			code: `package p
func f(m *freemail.Matcher, d string) bool { return m.IsConsumer(d) }`,
			want: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := consumerMailListsInSource(t, "planted.go", tc.code)
			if len(got) != tc.want {
				t.Fatalf("saw %d lists, want %d: %v", len(got), tc.want, got)
			}
		})
	}
}

// consumerMailListsIn walks the tree and reports every literal outside the
// owning package that names two or more providers.
func consumerMailListsIn(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	for _, path := range goSourceFiles(t, root) {
		if strings.Contains(filepath.ToSlash(path), consumerMailOwner) {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		code := string(raw)
		found = append(found, consumerMailListsInSource(t, path, code)...)
	}
	sort.Strings(found)
	return found
}

// consumerMailListsInSource reports `<file>:<line> (<n> providers)` for every
// composite literal in one source file naming two or more DISTINCT providers.
func consumerMailListsInSource(t *testing.T, name, code string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, code, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	// Constants declared in this file, so a list assembled from named ones is
	// not invisible. `const gmail = "gmail.com"` beside
	// `[]string{gmail, "outlook.com"}` is a two-provider list with one string
	// literal in it, and de-duplicating a hand-written map through constants is
	// the natural next spelling of the very bug this file guards.
	//
	// Same file only. A constant imported from elsewhere would need the type
	// checker, and a list assembled across package boundaries is not a shape
	// anything here writes.
	//
	// Resolved to a FIXED POINT, because `const a = "gmail.com"` and
	// `const b = a` chain: a single pass sees only the literal, and `b` — the
	// name the list actually uses — stays unknown. One indirection is enough to
	// hide a list, so the loop runs until nothing new is learned.
	constants := map[string]string{}
	for {
		learned := false
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, name := range spec.Names {
				if i >= len(spec.Values) {
					continue
				}
				if _, known := constants[name.Name]; known {
					continue
				}
				if value, ok := gatekit.StringExpr(spec.Values[i], constants, gatekit.FoldStrict); ok {
					constants[name.Name] = value
					learned = true
				}
			}
			return true
		})
		if !learned {
			break
		}
	}

	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		// DISTINCT, and counted over the literal's whole subtree: a provider
		// repeated is one domain, and a list nested inside a struct literal is
		// still a list.
		distinct := map[string]bool{}
		for _, element := range lit.Elts {
			ast.Inspect(element, func(inner ast.Node) bool {
				value, ok := gatekit.StringExpr(inner, constants, gatekit.FoldStrict)
				if ok && looksLikeADomain(value) {
					distinct[strings.ToLower(value)] = true
				}
				return true
			})
		}
		if len(distinct) >= 2 {
			found = append(found, fmt.Sprintf("%s:%d (%d providers)",
				name, fset.Position(lit.Pos()).Line, len(distinct)))
		}
		return true
	})
	return found
}
