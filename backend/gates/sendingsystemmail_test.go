// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A query that reads captured mail through the company reach set must leave out
// mail from a sending system.
//
// activity.sender_is_machine stores whether mail came from a notification tool.
// A reader that walks the company reach set or the scoring kind group without
// the arm counts a file-share notice as contact. It may also file a finding on
// the notification tool's company. The subjects come from the tree. They are
// the top-level declarations that render the reach set or the kind group.
// Each is judged on whether the same declaration reaches the arm.

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	machineSenderColumn = "sender_is_machine"
)

// armRenderers are the relstrength renderers that carry the machine-sender arm.
var armRenderers = map[string]bool{
	"InteractionCountsSQL": true,
	"WrittenByHumanSQL":    true,
}

// subjectRenderers are the renderers whose use makes a declaration a reader of
// captured mail for contact and company features.
var subjectRenderers = map[string]bool{
	"CompanyReachSet":         true,
	"InteractionKindSQLGroup": true,
}

// countsSendingSystemMail are the readers that keep mail from a
// sending system. Each says why that is right.
var countsSendingSystemMail = gatekit.Waive(map[string]string{
	"internal/compose/dealscoutsql.go :: scoutEvidenceSQL": "reads outbound mail only, and a sending system never receives a reply from us as its own mail, so no row of it carries the stamp",
})

type declUnit struct {
	relPath string
	name    string
	node    ast.Node
}

// TestEveryCompanyReachReaderLeavesOutSendingSystems is the census.
func TestEveryCompanyReachReaderLeavesOutSendingSystems(t *testing.T) {
	t.Parallel()
	defer countsSendingSystemMail.AssertAllMatched(t)

	var reachReaders int
	for _, pkg := range goPackagesUnder(t, ".") {
		units := declUnits(pkg)
		carriers := armCarriers(units)
		for _, unit := range units {
			renderers := subjectCalls(unit.node)
			if len(renderers) == 0 {
				continue
			}
			if renderers["CompanyReachSet"] {
				reachReaders++
			}
			if carriers[unit.name] {
				continue
			}
			key := unit.relPath + " :: " + unit.name
			if countsSendingSystemMail.Waived(t, key) {
				continue
			}
			t.Errorf("%s reads captured mail through the company reach set or the scoring kind group without leaving out mail from a sending system — add NOT <alias>.%s, or read through relstrength.InteractionCountsSQL. If counting that mail is right here, say why in countsSendingSystemMail under the key %q", unit.relPath, machineSenderColumn, key)
		}
	}
	if reachReaders == 0 {
		t.Error("no declaration in the tree renders the company reach set — this census sees nothing")
	}
}

// declUnits lists the package's top-level functions and var/const specs.
func declUnits(pkg goPackage) []declUnit {
	var out []declUnit
	for _, file := range pkg.files {
		for _, decl := range file.ast.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				out = append(out, declUnit{file.relPath, typed.Name.Name, typed})
			case *ast.GenDecl:
				if typed.Tok != token.VAR && typed.Tok != token.CONST {
					continue
				}
				for _, spec := range typed.Specs {
					if value, isValue := spec.(*ast.ValueSpec); isValue && len(value.Names) > 0 {
						out = append(out, declUnit{file.relPath, value.Names[0].Name, value})
					}
				}
			}
		}
	}
	return out
}

// subjectCalls names the subject renderers a declaration references, whether
// called bare (inside the activities package) or through a package qualifier.
func subjectCalls(node ast.Node) map[string]bool {
	found := map[string]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		if name := callTargetName(n); subjectRenderers[name] {
			found[name] = true
		}
		return true
	})
	return found
}

// callTargetName is the name of a call's function, bare or package-qualified.
func callTargetName(n ast.Node) string {
	call, isCall := n.(*ast.CallExpr)
	if !isCall {
		return ""
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

// armCarriers is the set of declaration names that reach the machine-sender arm.
// A declaration reaches it by the column's name in a string, by an arm renderer,
// or by calling a bare package-local carrier. It is a fixpoint, so a helper that reaches the arm through
// another helper counts.
func armCarriers(units []declUnit) map[string]bool {
	carriers := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for _, unit := range units {
			if !carriers[unit.name] && reachesArm(unit.node, carriers) {
				carriers[unit.name] = true
				changed = true
			}
		}
	}
	return carriers
}

func reachesArm(node ast.Node, carriers map[string]bool) bool {
	reached := false
	ast.Inspect(node, func(n ast.Node) bool {
		switch typed := n.(type) {
		case *ast.BasicLit:
			// bool_or(NOT x.sender_is_machine) only reports the flag; it excludes nothing.
			text, isString := gatekit.LiteralText(typed)
			if isString && strings.Contains(strings.ReplaceAll(text, "bool_or(NOT", ""), machineSenderColumn) {
				reached = true
			}
		case *ast.CallExpr:
			switch fun := typed.Fun.(type) {
			case *ast.SelectorExpr:
				reached = reached || armRenderers[fun.Sel.Name]
			case *ast.Ident:
				reached = reached || carriers[fun.Name]
			}
		}
		return !reached
	})
	return reached
}
