// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gradionhq/margince/backend/internal/platform/freemail"
)

// The subject set comes from the OWNER, not from a sample.
//
// The first draft of this file carried a hand-written 25-domain catalog and a
// comment claiming every entry was in the shipped dataset. Two reviewers named
// the same thing at once, and they were right twice over: it was a SECOND
// hand-maintained consumer-mail list inside the gate that exists to forbid
// second consumer-mail lists, and it was incomplete — `[]string{"laposte.net",
// "hotmail.fr"}` is a two-provider list the sample could not see, so the file
// holding it was dropped before the census ever parsed it.
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
			// The pair CodeRabbit named: both are in the shipped dataset and
			// neither was in the sampled catalog this file used to carry, so a
			// file holding exactly this was dropped by the prefilter before the
			// census ever parsed it.
			name: "two providers the old hand-written sample could not see",
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
			// Found by a bot: de-duplicating a hand-written map through
			// constants is the natural next spelling of this bug, and it puts
			// only ONE string literal in the list.
			name: "a list assembled from a named constant",
			code: `package p
const gmail = "gmail.com"
var providers = []string{gmail, "outlook.com"}`,
			want: 1,
		},
		{
			// The escape dimension: the detector unquotes, so it counts this —
			// and a prefilter comparing the RAW captured text dropped the file
			// before anything parsed it.
			name: "providers written with escapes, which unquote to the real names",
			code: `package p
var providers = []string{"gmail\x2ecom", "outlook\u002ecom"}`,
			want: 1,
		},
		{
			name: "a raw-string list, which the prefilter regex once dropped",
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
		// A file that names no provider cannot declare a list of them, and
		// parsing it anyway is what this gate would otherwise cost.
		if !namesAnyProvider(code) {
			continue
		}
		found = append(found, consumerMailListsInSource(t, path, code)...)
	}
	sort.Strings(found)
	return found
}

// namesAnyProvider is the parse-cost shortcut, and it is derived from the SAME
// set the census counts against. A prefilter narrower than the detector is how
// a real list gets dropped before anything looks at it — which is exactly what
// the sampled version did to `laposte.net`.
//
// It reads the file's own string tokens and asks the set about each, rather
// than asking the file about each of 8,758 domains: same answer, and it turns
// a whole-tree sweep from a minute into a second.
//
// The shortcut must be at least as BROAD as the detector in every dimension the
// detector reads, and there are two:
//
//   - the STRING FORM. Go has two, and `strconv.Unquote` accepts both, so a
//     pattern matching only double quotes drops a file spelling
//     "`gmail.com`" before anything parses it.
//   - the ESCAPE. `"gmail\x2ecom"` unquotes to `gmail.com` — the detector
//     counts it, and a prefilter testing the RAW captured text does not. So the
//     token is unquoted here too, by the same function, rather than compared as
//     written.
//
// Both were wrong in the first draft, in the same direction, which is the
// failure this whole file describes: a shortcut narrower than the thing it
// feeds reports nothing and looks exactly like a clean tree.
var stringToken = regexp.MustCompile("\"(?:[^\"\\\n]|\\\\.)*\"|`[^`]*`")

func namesAnyProvider(code string) bool {
	for _, token := range stringToken.FindAllString(code, -1) {
		value, err := strconv.Unquote(token)
		if err != nil {
			// Unparseable as a literal: admit the file rather than drop it. A
			// prefilter that guesses wrong must guess toward PARSING, since the
			// census behind it is the thing that decides.
			return true
		}
		if looksLikeADomain(value) {
			return true
		}
	}
	return false
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
				if value, ok := stringValue(spec.Values[i], constants); ok {
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
				value, ok := stringValue(inner, constants)
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

// stringValue reads a node's string value: a literal directly, and a named
// constant through the file's own declarations.
func stringValue(node ast.Node, constants map[string]string) (string, bool) {
	if basic, ok := node.(*ast.BasicLit); ok && basic.Kind == token.STRING {
		value, err := strconv.Unquote(basic.Value)
		return value, err == nil
	}
	if ident, ok := node.(*ast.Ident); ok {
		value, known := constants[ident.Name]
		return value, known
	}
	return "", false
}
