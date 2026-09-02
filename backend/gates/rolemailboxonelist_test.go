// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

package gates

// One role-mailbox list, held by a test rather than by a comment.
//
// `platform/mailrole` exists because two modules ask the same question from
// opposite ends of the capture path: the tier ladder decides whether an address
// may become a contact, and people's name parser decides whether a local part
// may become somebody's name. Before it there were two lists, and they
// disagreed in front of a user — `people.roleLocalParts` knew `billing` and
// `support`, capture knew neither, so the ladder created a contact the name
// parser then refused to name. A founder found departments in his CRM called
// "Billing" and "support".
//
// The subject is DERIVED from the owner: every composite literal in the tree
// naming three or more role tokens, with "is a role token" answered by
// `platform/mailrole` itself rather than by a sample kept here.
//
// THREE, not two, and the threshold was measured rather than chosen. At two the
// census flagged eight innocent literals: a test fixture whose mail subject is
// "Newsletter" and whose body is "hello", a table of German template words
// holding "Rechnung", a careers-page label map holding `jobs`/`careers`/
// `karriere`. Role words are ordinary words, so two of them co-occur by
// coincidence in prose and in unrelated vocabularies; three in one literal is a
// list. The map that shipped the defect held twelve.
//
// The under-recognition this trades away is real and bounded: a two-token role
// list would pass. That is the right side to err on here — this gate's job is
// to catch a second COPY of the vocabulary, and a copy worth the name is not
// two words long.
//
// WHAT THIS DOES NOT CATCH, deliberately: a list built from a file, a constant
// declared in another package, or a database read. A gate that followed those
// would have to run them, and the defect this exists over was a literal — as
// is every plausible next one.

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

	"github.com/margince/margince/backend/internal/platform/mailrole"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The subject set comes from the OWNER, not from a sample. A hand-written
// sample would be a second role list inside the gate that forbids second role
// lists, and an incomplete one.
var roleTokenSet = sync.OnceValue(func() map[string]struct{} {
	set := make(map[string]struct{})
	for _, token := range mailrole.Tokens() {
		set[token] = struct{}{}
	}
	return set
})

// The package that OWNS the answer, including its own tests: the vocabulary
// lives there and its suite has to name tokens to assert about them.
const roleMailboxOwner = "internal/platform/mailrole/"

// How many role tokens in one literal make it a list rather than a coincidence.
// See the package comment above for why this is three and not two.
const roleListThreshold = 3

// looksLikeARoleToken screens a string literal before the vocabulary is
// consulted. A bare word only: `support@acme.com` in a fixture is an address
// naming one mailbox, not somebody re-declaring the list.
func looksLikeARoleToken(value string) bool {
	if value == "" || strings.ContainsAny(value, "@/\\ \t.") {
		return false
	}
	_, known := roleTokenSet()[strings.ToLower(value)]
	return known
}

func TestOnlyOnePackageDeclaresRoleMailboxes(t *testing.T) {
	t.Parallel()
	found := roleMailboxListsIn(t, "internal")
	if len(found) > 0 {
		t.Fatalf("a second role-mailbox list — ask platform/mailrole instead:\n%s",
			strings.Join(found, "\n"))
	}
}

// The gate's own defect test. A census of zero cannot tell a clean tree from a
// blind detector, and this one is written over the exact shape that shipped:
// the map that used to sit in people/personname.go.
func TestRoleMailboxCensusSeesTheShapeThatShipped(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		code string
		want int
	}{
		{
			name: "the map people/personname.go really carried",
			code: `package p
var roleLocalParts = map[string]bool{
	"admin": true, "billing": true, "support": true, "info": true,
}`,
			want: 1,
		},
		{
			name: "a slice of tokens",
			code: `package p
var refuse = []string{"support", "billing", "invoice"}`,
			want: 1,
		},
		{
			name: "assembled through constants, one indirection deep",
			code: `package p
const support = "support"
const alias = support
var refuse = []string{alias, "billing", "invoice"}`,
			want: 1,
		},
		{
			name: "two tokens co-occur in prose, which is not a list",
			code: `package p
var fixture = map[string]string{"subject": "Newsletter", "body": "hello"}`,
			want: 0,
		},
		{
			name: "addresses name mailboxes, not a vocabulary",
			code: `package p
var fixtures = []string{"support@acme.com", "billing@acme.com", "info@acme.com"}`,
			want: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := roleMailboxListsInSource(t, "planted.go", tc.code)
			if len(got) != tc.want {
				t.Fatalf("wanted %d findings, got %d: %v", tc.want, len(got), got)
			}
		})
	}
}

func roleMailboxListsIn(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	for _, path := range goSourceFiles(t, root) {
		if strings.Contains(filepath.ToSlash(path), roleMailboxOwner) {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		found = append(found, roleMailboxListsInSource(t, path, string(raw))...)
	}
	sort.Strings(found)
	return found
}

// roleMailboxListsInSource reports `<file>:<line> (<n> tokens)` for every
// composite literal in one source file naming two or more DISTINCT role tokens.
func roleMailboxListsInSource(t *testing.T, name, code string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, code, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	// Constants declared in this file, resolved to a fixed point so that
	// `const a = "support"; const b = a` cannot hide a list behind one
	// indirection. Same reasoning, and the same helper, as the consumer-mail
	// census next door.
	constants := map[string]string{}
	for {
		learned := false
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, ident := range spec.Names {
				if i >= len(spec.Values) {
					continue
				}
				if _, known := constants[ident.Name]; known {
					continue
				}
				if value, ok := gatekit.StringExpr(spec.Values[i], constants, gatekit.FoldStrict); ok {
					constants[ident.Name] = value
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
		// DISTINCT, over the literal's whole subtree: a token repeated is one
		// token, and a list nested in a struct literal is still a list.
		distinct := map[string]bool{}
		for _, element := range lit.Elts {
			ast.Inspect(element, func(inner ast.Node) bool {
				value, ok := gatekit.StringExpr(inner, constants, gatekit.FoldStrict)
				if ok && looksLikeARoleToken(value) {
					distinct[strings.ToLower(value)] = true
				}
				return true
			})
		}
		if len(distinct) >= roleListThreshold {
			found = append(found, fmt.Sprintf("%s:%d (%d tokens)",
				name, fset.Position(lit.Pos()).Line, len(distinct)))
		}
		return true
	})
	return found
}
