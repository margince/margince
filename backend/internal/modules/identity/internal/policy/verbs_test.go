// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package policy

import (
	"go/ast"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOfferedVerbsDropWhatNoShippedRoleHolds(t *testing.T) {
	cases := map[string][]principal.Action{
		"contact":      {principal.ActionRead, principal.ActionCreate, principal.ActionUpdate, principal.ActionDelete},
		"audit_log":    {principal.ActionRead},
		"seat_usage":   {principal.ActionRead},
		"system_reset": {principal.ActionDelete},
		"tax_rate":     {principal.ActionRead, principal.ActionCreate},
	}
	for object, want := range cases {
		if got := OfferedVerbs(object); !slices.Equal(got, want) {
			t.Errorf("%s offers %v, want %v", object, got, want)
		}
	}
}

func TestAnExtensionObjectOffersAllFourVerbs(t *testing.T) {
	t.Cleanup(ResetRegisteredForTest)
	if err := Register("ext_demo_ledger"); err != nil {
		t.Fatal(err)
	}
	if got := OfferedVerbs("ext_demo_ledger"); len(got) != 4 {
		t.Errorf("an extension object offers %v, want all four", got)
	}
	if _, ok := OfferedVerbsByObject()["ext_demo_ledger"]; !ok {
		t.Error("the verb catalog omits a registered extension object")
	}
}

func TestOnlyAnObjectThatOffersReadNeedsIt(t *testing.T) {
	if !RequiresRead("contact") {
		t.Error("contact writes must need read")
	}
	if RequiresRead("system_reset") {
		t.Error("system_reset offers no read, so its delete needs none")
	}
}

// Every shipped role holds only verbs its object offers, and the offered set is
// what the editor draws, so a shipped grant can never sit on a hidden switch.
func TestShippedGrantsStayInsideOfferedVerbs(t *testing.T) {
	for roleKey := range defaults {
		shipped, _ := ShippedGrants(roleKey)
		for object, g := range shipped {
			for _, verb := range []principal.Action{principal.ActionCreate, principal.ActionRead, principal.ActionUpdate, principal.ActionDelete} {
				held := map[principal.Action]bool{
					principal.ActionCreate: g.Create, principal.ActionRead: g.Read,
					principal.ActionUpdate: g.Update, principal.ActionDelete: g.Delete,
				}[verb]
				if held && !slices.Contains(OfferedVerbs(object), verb) {
					t.Errorf("role %q holds %s on %q, which the object does not offer", roleKey, verb, object)
				}
			}
		}
	}
}

// A verb the code checks must be one the editor can grant. The scan reads the
// checks written with a literal object and a literal action, and a hidden
// switch for a checked verb would make that check unsatisfiable by any new role.
func TestEveryLiteralGateCheckIsOffered(t *testing.T) {
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { return len(literalChecks(file)) > 0 },
	}
	checked := 0
	for _, parsed := range scope.Files(t) {
		for _, c := range literalChecks(parsed.File) {
			checked++
			if !IsCoreObject(c.object) {
				continue
			}
			if !slices.Contains(OfferedVerbs(c.object), c.action) {
				t.Errorf("%s checks %s on %q, which the shipped roles never grant, so the editor would not draw it",
					parsed.Path, c.action, c.object)
			}
		}
	}
	if checked < 100 {
		t.Fatalf("read only %d literal checks; the call shapes this gate derives from have changed", checked)
	}
}

type literalCheck struct {
	object string
	action principal.Action
}

// literalChecks finds f(..., "<object>", principal.ActionX, ...) calls.
func literalChecks(file *ast.File) []literalCheck {
	var out []literalCheck
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		for i := 0; i+1 < len(call.Args); i++ {
			lit, ok := call.Args[i].(*ast.BasicLit)
			if !ok {
				continue
			}
			object, err := strconv.Unquote(lit.Value)
			if err != nil {
				continue
			}
			for _, arg := range call.Args[i+1:] {
				sel, ok := arg.(*ast.SelectorExpr)
				if !ok || !strings.HasPrefix(sel.Sel.Name, "Action") {
					break
				}
				out = append(out, literalCheck{object, principal.Action(strings.ToLower(strings.TrimPrefix(sel.Sel.Name, "Action")))})
			}
		}
		return true
	})
	return out
}
