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
// The subject is DERIVED: every composite literal in the tree that names two or
// more consumer mailbox providers. Two is the threshold because one provider in
// a literal is a test fixture or a single named case, while two is a list.
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
	"strconv"
	"strings"
	"testing"
)

// A sample of the platform baseline wide enough to catch a real list. It does
// not need to be exhaustive, and deliberately is not: a hand-written
// consumer-mail list that avoids ALL of these is not a consumer-mail list.
// Every entry here is in `platform/freemail`'s shipped dataset.
var knownConsumerProviders = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "yahoo.com": true,
	"yahoo.co.uk": true, "outlook.com": true, "hotmail.com": true,
	"live.com": true, "icloud.com": true, "me.com": true, "aol.com": true,
	"gmx.de": true, "gmx.net": true, "web.de": true, "t-online.de": true,
	"proton.me": true, "protonmail.com": true, "zoho.com": true,
	"yandex.ru": true, "mail.com": true, "fastmail.com": true,
	"tutanota.com": true, "mail.ru": true, "qq.com": true,
	"naver.com": true, "seznam.cz": true,
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
			name: "the same list as a slice",
			code: `package p
var providers = []string{"gmail.com", "outlook.com"}`,
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

func namesAnyProvider(code string) bool {
	for domain := range knownConsumerProviders {
		if strings.Contains(code, domain) {
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
				basic, ok := inner.(*ast.BasicLit)
				if !ok || basic.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(basic.Value)
				if err == nil && knownConsumerProviders[strings.ToLower(value)] {
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
