// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every arm outside privacy that empties an activity's text also destroys the
// copies derived from it.
//
// The text of a message is copied into claims, transcript readings, staged
// proposals, reply verdicts, stage evidence, vectors and the provider original.
// privacy keeps that list in one function, and its own census holds the arms
// inside that package. An arm in another module cannot call the private
// function, so it calls one of the eraser's exported entry points instead. This
// census holds that: a function that nulls activity.body outside privacy is only
// ever called by a function that also calls such an entry point.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	privacyPackageDir = "internal/modules/privacy/"
	// sharedContentPurge is the private function in privacy that holds the list.
	sharedContentPurge = "purgeContentDerivedFrom"
)

// contentArmFunc is one declaration this census read: where it is, whether it
// empties an activity's text, and the names it calls.
type contentArmFunc struct {
	where    string
	name     string
	destroys bool
	calls    map[string]bool
}

// contentArmSource is one parsed non-test source and its path from the module
// root.
type contentArmSource struct {
	where string
	file  *ast.File
}

func TestEveryActivityTextDestroyerOutsidePrivacyFinishesTheErasure(t *testing.T) {
	t.Parallel()
	var sources []contentArmSource
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", where, err)
		}
		sources = append(sources, contentArmSource{where: where, file: file})
	}
	findings, destroyers := judgeContentArms(t, sources)
	for _, finding := range findings {
		t.Error(finding)
	}
	// A census that reads no destroyer reports a clean pass over nothing. The
	// noise redaction in activities is one today.
	if destroyers == 0 {
		t.Fatal("no function outside privacy empties activity.body; the reader has gone quiet, so this gate proves nothing")
	}
}

// A statement held in a package-level constant is a destroyer too. The census
// must find it there, and fail the caller that skips the eraser.
func TestAnActivityTextDestroyerInAPackageConstantIsJudged(t *testing.T) {
	t.Parallel()
	planted := []contentArmSource{
		parseContentArm(t, "internal/modules/privacy/finish.go", `package privacy
func (e *Eraser) FinishTx() error { return e.purgeContentDerivedFrom() }`),
		parseContentArm(t, "internal/modules/mail/wipe.go", `package mail
const wipeBody = "UPDATE activity SET subject = NULL, " + "body = NULL WHERE id = $1"
func (s *Store) WipeTx() error { _, err := s.tx.Exec(s.ctx, wipeBody); return err }`),
		parseContentArm(t, "internal/compose/sweep.go", `package compose
func (e *Engine) Sweep() error { return e.mail.WipeTx() }`),
	}
	findings, destroyers := judgeContentArms(t, planted)
	if destroyers != 1 || len(findings) != 1 || !strings.Contains(findings[0], "Sweep") {
		t.Fatalf("the census did not judge the destroyer held in a package constant: %d destroyer(s), findings %v", destroyers, findings)
	}
}

func parseContentArm(t *testing.T, where, source string) contentArmSource {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), where, source, 0)
	if err != nil {
		t.Fatalf("parsing the planted %s: %v", where, err)
	}
	return contentArmSource{where: where, file: file}
}

// judgeContentArms reports every caller outside privacy that empties an
// activity's text without an eraser entry point beside it, and how many
// destroyers it read.
func judgeContentArms(t *testing.T, sources []contentArmSource) (findings []string, destroyers int) {
	t.Helper()
	destroyingNames := packageLevelDestroyers(sources)
	finishers := map[string]bool{}
	var outside []contentArmFunc
	for _, source := range sources {
		inPrivacy := strings.HasPrefix(source.where, privacyPackageDir)
		held := destroyingNames[path.Dir(source.where)]
		for _, decl := range source.file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			calls := calledNames(fn)
			if inPrivacy {
				// The finishers are derived from the owner: every exported
				// function there that runs the shared list.
				if fn.Name.IsExported() && calls[sharedContentPurge] {
					finishers[fn.Name.Name] = true
				}
				continue
			}
			outside = append(outside, contentArmFunc{
				where: source.where + ": " + fn.Name.Name, name: fn.Name.Name, calls: calls,
				destroys: slices.ContainsFunc(gatekit.SQLStatementsOf(fn), gatekit.DestroysActivityBody) ||
					referencesAny(fn, held),
			})
		}
	}
	if len(finishers) == 0 {
		t.Fatalf("privacy exports no function that calls %s, so no arm outside it can finish an erasure", sharedContentPurge)
	}
	for _, arm := range outside {
		if !arm.destroys {
			continue
		}
		destroyers++
		callers := 0
		for _, caller := range outside {
			if !caller.calls[arm.name] {
				continue
			}
			callers++
			if !callsAny(caller.calls, finishers) {
				findings = append(findings, fmt.Sprintf("%s empties an activity's text through %s and never calls the eraser (%v): the claims, readings, proposals and stage evidence quoting the message keep its words",
					caller.where, arm.name, sortedKeys(finishers)))
			}
		}
		if callers == 0 {
			findings = append(findings, fmt.Sprintf("%s empties an activity's text and nothing outside privacy calls it, so no caller runs the eraser (%v) beside it",
				arm.where, sortedKeys(finishers)))
		}
	}
	return findings, destroyers
}

// packageLevelDestroyers names, per package directory, the package-level
// constants and variables whose SQL empties an activity's text.
func packageLevelDestroyers(sources []contentArmSource) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, source := range sources {
		dir := path.Dir(source.where)
		for _, decl := range source.file.Decls {
			general, isGeneral := decl.(*ast.GenDecl)
			if !isGeneral || (general.Tok != token.CONST && general.Tok != token.VAR) {
				continue
			}
			for _, spec := range general.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue {
					continue
				}
				for i, name := range value.Names {
					if i >= len(value.Values) ||
						!slices.ContainsFunc(gatekit.SQLStatementsOf(value.Values[i]), gatekit.DestroysActivityBody) {
						continue
					}
					if out[dir] == nil {
						out[dir] = map[string]bool{}
					}
					out[dir][name.Name] = true
				}
			}
		}
	}
	return out
}

// referencesAny reports whether the body names any of the given identifiers.
func referencesAny(fn *ast.FuncDecl, names map[string]bool) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if ident, isIdent := n.(*ast.Ident); isIdent && names[ident.Name] {
			found = true
		}
		return !found
	})
	return found
}

func callsAny(calls, names map[string]bool) bool {
	for name := range names {
		if calls[name] {
			return true
		}
	}
	return false
}
