// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every outbound identity has a request behind it.
//
// `platform/outbound` declares how this product names itself to a server it
// calls. Each token is a promise to a stranger: a robots.txt group a site
// wrote, a rate limit an operator granted, an audit line an administrator
// reads. That is why they are constants — the file says so — and it is also
// why one with no caller is worse than dead code.
//
// `MirrorProduct = "margince-mirror"` was exactly that: an identity for reads
// and writes against a customer's OWN CRM, left behind when the mirror program
// was retired. Its comment told the next reader that this product still writes
// to a customer's records, which it does not, and nothing failed — a grep for
// the token returned its own declaration and nothing else.
//
// So the census is a use count, and the failure is a token nobody sends. It is
// the same question the reuse rule asks of a second implementation, asked of a
// first one that has stopped being used.
//
// WHAT IT CANNOT SEE. A token used only by a test — a caller that exercises the
// header without any production path sending it. The count below is of
// NON-TEST users for that reason: a name advertised only inside `go test` is
// advertised to nobody.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// outboundIdentities is the file that declares them, read rather than listed:
// a census keyed on a list is one that agrees with a token added to the file
// and forgotten.
const outboundIdentities = "internal/platform/outbound/useragent.go"

// identityConstant names the token half of each identity. Every one is
// declared beside a `*Header` built from it, and the PAIR is the unit: a
// caller sends the header, so counting only the token would report every
// identity in the file as dead.
var identityConstant = regexp.MustCompile(`^([A-Z]\w*)Product$`)

// declaredIdentities reads the product tokens out of the file that owns them.
func declaredIdentities(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), outboundIdentities, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", outboundIdentities, err)
	}
	var names []string
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			for _, name := range value.Names {
				if found := identityConstant.FindStringSubmatch(name.Name); found != nil {
					names = append(names, found[1])
				}
			}
		}
	}
	sort.Strings(names)
	return names
}

func TestEveryOutboundIdentityHasARequestBehindIt(t *testing.T) {
	t.Parallel()
	identities := declaredIdentities(t)
	// A parse that found nothing reads exactly like a file in which every
	// token is used, which is the direction this must not fail in.
	if len(identities) == 0 {
		t.Fatalf("%s declares no *Product constant, so this census vouches for nothing — "+
			"either the naming moved or the walk no longer reaches the file", outboundIdentities)
	}

	users := map[string]int{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return walkErr
		}
		path = filepath.ToSlash(path)
		// The declaring file is not a user of what it declares, and a name
		// advertised only inside `go test` is advertised to nobody.
		if path == outboundIdentities || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := os.ReadFile(path) // #nosec G304 -- a *.go path from walking the trusted source tree
		if readErr != nil {
			return readErr
		}
		for _, name := range identities {
			// Either half counts, and per FILE rather than per occurrence: the
			// question is whether anything sends this identity, not how often.
			if regexp.MustCompile(`\boutbound\.` + name + `(Product|Header)\b`).Match(source) {
				users[name]++
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree for identity users: %v", err)
	}

	var unused []string
	for _, name := range identities {
		if users[name] == 0 {
			unused = append(unused, name)
		}
	}
	if len(unused) > 0 {
		t.Errorf("%d outbound identit(ies) are declared and never sent: %s\n\n"+
			"A token here is a promise to a stranger — a robots.txt group a site wrote, a rate limit an "+
			"operator granted, an audit line an administrator reads. One with no request behind it tells "+
			"the next reader this product makes a call it does not make, which is how `margince-mirror` "+
			"outlived the program that sent it.\n\nDelete it with the caller that went, or send it.",
			len(unused), strings.Join(unused, ", "))
	}
}
