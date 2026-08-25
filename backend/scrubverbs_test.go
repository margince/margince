// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// Every verb the privacy module writes is judged a scrub or ratified as not one.
//
// `fieldHistoryScrubActions` is the boundary THREE reads of the audit spine take
// — field history, record history, and the compliance log. audit_log is
// append-only, so a scrub cannot rewrite the images it certifies gone; each read
// stops at the newest tombstone instead. That makes the list the single point on
// which all three depend.
//
// It is a hand-written list, and the way it fails is the way that matters: a new
// scrubbing verb nobody adds to it does not break anything. The three reads go
// on returning images the scrub was supposed to put out of reach, every test
// passes, and the disclosure is silent. Under-recognition is the one failure a
// gate must not have, so the corpus here is derived from the WRITERS rather than
// restated: every verb privacy audits under is either in the scrub list or in
// the register below, saying why it certifies nothing.
//
// Not erasureCascadeFiles' question. That gate asks which FILES the Art. 17
// cascade executes SQL from, and says in as many words that it is deliberately
// not the whole package, because letting a retention sweep satisfy "Art. 17
// reaches this table" is the confusion it exists to prevent. This one asks which
// VERBS certify a scrub — and retention's anonymize is one of them, which is
// exactly why the two cannot share an authority.
//
// Scope's negative-space proof does not apply here and the subject says why: a
// file is this gate's business because it is IN the privacy module, not because
// of what it contains, so the root cannot name the wrong tree. Every module in
// the tree audits, and a content-based subject would report all of them as
// subjects lying outside the root — noise, not a proof. What Scope is used for
// is its parsing and its refusal of a root that holds nothing.
//
// What it cannot see: a verb built at run time. `retentionactions.go` audits
// under a column of the policy row, whose CHECK admits archive, anonymize and
// erase — all three of which are judged below, so the runtime path reaches
// nothing this gate has not already ruled on.

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// privacyNonScrubVerbs are the verbs privacy writes that certify NOTHING about
// the record's PII, each with the reason it does not move the boundary.
//
// A verb here is a claim that a reader may still see what the record held before
// it. Getting one wrong leaves images readable that should not be, so each entry
// says what the verb actually did to the row.
// privacyRoot is the module whose verbs this gate judges. A file is a subject
// because it lives here.
const privacyRoot = "internal/modules/privacy"

var privacyNonScrubVerbs = gatekit.Waive(map[string]string{
	"create":  "the record's own creation, which carries no prior state and scrubs nothing that came before it",
	"update":  "an ordinary field change: the images are the point of it, and a reader is entitled to both sides",
	"export":  "a subject-access export READS the record and writes nothing to it, so nothing it held has gone",
	"archive": "retiring a row hides it from the default reads and leaves every value intact, which is why an archived record can still be restored",
	"expire":  "a restriction reaching its end returns the record to ordinary handling; the identifiers the restriction hid were never removed",
	"pin":     "a statutory hold REFUSES deletion for a period, so it is the opposite of a scrub in what it does to the data",
	"release": "lifting a hold restores ordinary retention and moves no value",
})

func TestEveryPrivacyAuditVerbIsJudgedAScrubOrNot(t *testing.T) {
	defer privacyNonScrubVerbs.AssertAllMatched(t)

	consts := privacyPackageConstants(t)
	scrub := scrubVerbSet(t, consts)
	files := gatekit.Scope{
		Roots:   []string{privacyRoot},
		Subject: fileAuditsUnderAVerb,
	}.Files(t)

	// Verbs resolve against the PACKAGE's constants, not each file's own.
	// actionArchive is declared in recordhistory.go and written from erasure's
	// neighbours; per-file resolution would leave those unresolvable, and an
	// unresolvable verb is one this gate skips — the census failing short in the
	// one direction that reports PASS.
	var judged int
	for _, parsed := range files {
		for _, verb := range auditVerbsIn(parsed.File, consts) {
			judged++
			if scrub[verb] || privacyNonScrubVerbs.Waived(t, verb) {
				continue
			}
			t.Errorf("privacy audits under %q, and nothing says whether it certifies a scrub.\n"+
				"\tThree reads of the spine stop at a scrub tombstone; a verb missing from both lists "+
				"silently is not one of them.\n"+
				"\tAdd it to fieldHistoryScrubActions, or to privacyNonScrubVerbs with what it did to the row.",
				verb)
		}
	}

	// A census that judged nothing certifies nothing. The floor sits below the
	// real count, so a walk that stopped working fails rather than reporting a
	// module that audits under no verb at all.
	if judged < 8 {
		t.Errorf("resolved %d audit verb(s) in privacy — the way of finding them has stopped working", judged)
	}
}

// scrubVerbSet reads the list itself, resolved against the PACKAGE's constants:
// the list names actionErase, which erasure.go declares, so reading only the
// declaring file's own constants would leave the boundary looking narrower than
// it is — and a verb this gate thinks is not a scrub is one it demands a reason
// for.
func scrubVerbSet(t *testing.T, consts map[string]string) map[string]bool {
	t.Helper()
	const declaringFile = privacyRoot + "/fieldhistory.go"
	file := gatekit.Scope{
		Roots:   []string{privacyRoot},
		Subject: func(path string, _ *ast.File) bool { return path == declaringFile },
	}.Files(t)
	if len(file) != 1 {
		t.Fatalf("found %d file(s) declaring the scrub verbs, want %s alone", len(file), declaringFile)
	}
	var verbs map[string]bool
	ast.Inspect(file[0].File, func(n ast.Node) bool {
		spec, isValue := n.(*ast.ValueSpec)
		if !isValue || len(spec.Names) == 0 || spec.Names[0].Name != "fieldHistoryScrubActions" {
			return true
		}
		verbs = map[string]bool{}
		for _, value := range spec.Values {
			composite, isComposite := value.(*ast.CompositeLit)
			if !isComposite {
				continue
			}
			for _, element := range composite.Elts {
				if verb, known := privacyString(element, consts); known {
					verbs[verb] = true
				}
			}
		}
		return false
	})
	if len(verbs) == 0 {
		t.Fatalf("fieldHistoryScrubActions resolved to no verbs — this gate would pass over an empty boundary")
	}
	return verbs
}

// privacyPackageConstants collects the package's string constants, from
// whichever file declares each one — a verb written in erasure.go may name a
// constant recordhistory.go declares.
func privacyPackageConstants(t *testing.T) map[string]string {
	t.Helper()
	consts := map[string]string{}
	for _, parsed := range (gatekit.Scope{
		Roots:   []string{privacyRoot},
		Subject: fileAuditsUnderAVerb,
	}).Files(t) {
		for name, value := range privacyStringConstants(parsed.File) {
			consts[name] = value
		}
	}
	return consts
}

// fileAuditsUnderAVerb is the Scope subject: a file that calls an audit door at
// all. The VERB it writes under is judged later, against the package's
// constants — a file whose verb is a sibling's constant is still a subject.
func fileAuditsUnderAVerb(path string, _ *ast.File) bool {
	return strings.HasPrefix(path, privacyRoot+"/") && !strings.HasSuffix(path, "_test.go")
}

// auditVerbsIn reads the verb each audit call in one file writes under. A verb
// this cannot reduce to a constant is skipped: the doc comment above names the
// one such site and why it reaches nothing unjudged.
func auditVerbsIn(file *ast.File, consts map[string]string) []string {
	var verbs []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall || len(call.Args) < 3 {
			return true
		}
		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector || !strings.HasPrefix(selector.Sel.Name, "Audit") {
			return true
		}
		if pkg, isIdent := selector.X.(*ast.Ident); !isIdent || pkg.Name != "storekit" {
			return true
		}
		if verb, known := privacyString(call.Args[2], consts); known {
			verbs = append(verbs, verb)
		}
		return true
	})
	return verbs
}

func privacyStringConstants(file *ast.File) map[string]string {
	consts := map[string]string{}
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
			for i, name := range value.Names {
				if i < len(value.Values) {
					if literal, known := privacyString(value.Values[i], nil); known {
						consts[name.Name] = literal
					}
				}
			}
		}
	}
	return consts
}

func privacyString(expr ast.Expr, consts map[string]string) (string, bool) {
	switch node := expr.(type) {
	case *ast.BasicLit:
		if node.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(node.Value)
		return value, err == nil
	case *ast.Ident:
		value, declared := consts[node.Name]
		return value, declared
	default:
		return "", false
	}
}
