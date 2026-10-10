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
	"go/ast"
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
	destroys bool
	calls    map[string]bool
}

func TestEveryActivityTextDestroyerOutsidePrivacyFinishesTheErasure(t *testing.T) {
	t.Parallel()
	finishers := map[string]bool{}
	var outside []contentArmFunc
	byName := map[string][]contentArmFunc{}
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", where, err)
		}
		inPrivacy := strings.HasPrefix(where, privacyPackageDir)
		for _, decl := range file.Decls {
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
			arm := contentArmFunc{where: where + ": " + fn.Name.Name, calls: calls}
			arm.destroys = slices.ContainsFunc(gatekit.SQLStatementsOf(fn), gatekit.DestroysActivityBody)
			outside = append(outside, arm)
			byName[fn.Name.Name] = append(byName[fn.Name.Name], arm)
		}
	}
	if len(finishers) == 0 {
		t.Fatalf("privacy exports no function that calls %s, so no arm outside it can finish an erasure", sharedContentPurge)
	}

	destroyers := 0
	for _, arm := range outside {
		if !arm.destroys {
			continue
		}
		destroyers++
		name := arm.where[strings.LastIndex(arm.where, " ")+1:]
		callers := 0
		for _, caller := range outside {
			if !caller.calls[name] {
				continue
			}
			callers++
			if !callsAny(caller.calls, finishers) {
				t.Errorf("%s empties an activity's text through %s and never calls the eraser (%v): the claims, readings, proposals and stage evidence quoting the message keep its words",
					caller.where, name, sortedKeys(finishers))
			}
		}
		if callers == 0 {
			t.Errorf("%s empties an activity's text and nothing outside privacy calls it, so no caller runs the eraser (%v) beside it",
				arm.where, sortedKeys(finishers))
		}
	}
	// A census that reads no destroyer reports a clean pass over nothing. The
	// noise redaction in activities is one today.
	if destroyers == 0 {
		t.Fatal("no function outside privacy empties activity.body; the reader has gone quiet, so this gate proves nothing")
	}
}

func callsAny(calls, names map[string]bool) bool {
	for name := range names {
		if calls[name] {
			return true
		}
	}
	return false
}
