// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// Two claims that a struct literal cannot hold on its own.
//
// The promote preview answers "who is this lead already?" and the promotion
// then acts on that answer. They agree only while the CANDIDATE is derived once
// — two literals naming the same fields still disagree when the values fed into
// them are worked out differently, and the preview would name a person the
// promotion does not land on. That is worse than a plain wrong answer, because
// the preview is what a human read before agreeing.
//
// A lead update carries a gesture JSON erases: null means clear the override,
// absent means leave it. Only LeadUpdateRequest records the difference, so a
// transport decoding the bare contract type reads a clear as a no-op — the
// caller's request succeeds and the override they asked to remove is still
// there.

// candidateType is the ladder's input, built in one place or not at all.
const candidateType = "PersonCandidate"

// candidateBuilder is where that one place is.
const candidateBuilder = "leadPersonCandidate"

// leadType is what makes a derivation be about THIS candidate. The ladder
// matches candidates built from several sources — a channel identity, a
// resolve, a manual dedupe — and those are different derivations of a different
// thing. The claim is about the one the preview and the promotion share, and
// its input is what identifies it.
const leadType = "crmcontracts.Lead"

func TestTheLadderCandidateIsDerivedInOnePlace(t *testing.T) {
	// Every function handed a lead is placed, rather than a subset being
	// selected and judged. Under-recognition is then loud: a derivation written
	// in a spelling this does not know still lands in the population, and the
	// only way out of it is to be neither a build nor a change.
	population, deriving := 0, map[string]string{}
	forEachModuleFunc(t, func(_ moduleFile, fn *ast.FuncDecl) {
		if !takesA(fn, leadType) {
			return
		}
		population++
		switch {
		case constructs(fn.Body, candidateType):
			deriving[fn.Name.Name] = "builds"
		case mutatesResultOf(fn.Body, candidateBuilder):
			// A function that takes what the one builder returned and writes a
			// field on it has produced a second answer, in a form that reads at
			// the call site as though it were still the first.
			deriving[fn.Name.Name] = "changes what " + candidateBuilder + " returned in"
		}
	})
	if population == 0 {
		t.Fatalf("no function in this module is handed a %s, so this gate judged nothing and the "+
			"derivation it holds has moved", leadType)
	}
	if deriving[candidateBuilder] == "" {
		t.Fatalf("%s does not derive a %s, so this gate is watching the wrong function and the "+
			"derivation it was meant to hold is somewhere else now", candidateBuilder, candidateType)
	}
	others := make([]string, 0, len(deriving))
	for name, how := range deriving {
		if name != candidateBuilder {
			others = append(others, name+" ("+how+" it)")
		}
	}
	if len(others) == 0 {
		return
	}
	sort.Strings(others)
	t.Errorf("a %s is derived from a lead by %s as well as by %s.\n\nThe preview and the "+
		"promotion agree only while "+
		"the candidate is derived once: same fields, different working-out, and the preview names "+
		"a person the promotion does not land on — after a human read the preview and agreed. "+
		"Take %s.", candidateType, strings.Join(others, ", "), candidateBuilder, candidateBuilder)
}

// contractLeadUpdate is the generated request type, which drops the
// null-vs-absent gesture on decode.
const contractLeadUpdate = "UpdateLeadRequest"

// leadUpdateWrapper is the type that keeps it.
const leadUpdateWrapper = "LeadUpdateRequest"

// leadUpdateAssembler is what every transport hands its decoded request to. It
// is what makes the population derivable: a surface that updates a lead reaches
// this, whatever else it does, so the transports do not have to be counted or
// named here.
const leadUpdateAssembler = "leadUpdateInput"

func TestEveryLeadUpdateDecodeKeepsTheNullGesture(t *testing.T) {
	wrapperSeen := false
	forEachModuleFile(t, func(_ string, _ *token.FileSet, file *ast.File) {
		ast.Inspect(file, func(node ast.Node) bool {
			if spec, isSpec := node.(*ast.TypeSpec); isSpec && spec.Name.Name == leadUpdateWrapper {
				wrapperSeen = true
			}
			return !wrapperSeen
		})
	})
	if !wrapperSeen {
		t.Fatalf("this module declares no %s, so the gesture has no keeper and this gate judged "+
			"nothing", leadUpdateWrapper)
	}

	// The claim is universal over the transports rather than counted at two of
	// them. A third surface is judged the day it is written, and the gate never
	// has to be told how many there are supposed to be.
	transports := 0
	forEachModuleFunc(t, func(parsed moduleFile, fn *ast.FuncDecl) {
		if !callsFunc(fn, leadUpdateAssembler) {
			return
		}
		transports++
		if bare := zeroValuesOf(fn.Body, "crmcontracts."+contractLeadUpdate); len(bare) > 0 {
			t.Errorf("%s (%s) decodes a lead update into the bare %s.\n\nJSON erases the "+
				"difference between null and absent on a pointer field, and only %s records "+
				"it — decoded this way, a caller asking to CLEAR an override gets a successful "+
				"response and keeps the override.",
				fn.Name.Name, parsed.name, contractLeadUpdate, leadUpdateWrapper)
			return
		}
		if len(zeroValuesOf(fn.Body, leadUpdateWrapper)) == 0 {
			t.Errorf("%s (%s) assembles a lead update without decoding into %s.\n\nThe gesture is "+
				"kept by that type and by nothing else, so a transport reaching the assembler "+
				"another way has already lost the difference between clearing an override and "+
				"leaving it.", fn.Name.Name, parsed.name, leadUpdateWrapper)
		}
	})
	if transports == 0 {
		t.Fatalf("nothing in this module calls %s, so the population this gate judges is empty "+
			"and the transports have moved", leadUpdateAssembler)
	}
}

// A third derivation lives one field away from the two above, and it fails in
// the opposite direction: not two answers to who the lead is, but two readings
// of whether it is anybody.
//
// The promotion refuses a lead with no identity, and the ladder candidate works
// out the name to match on. Both turn on the same question — what is this lead
// called — and while each answers it for itself, the guard admits leads the
// candidate cannot name. `FullName != nil` is true of a full_name that is
// present and empty, so a lead carrying one and no email clears the guard and
// promotes into a person with no name at all: a row nobody searching for that
// person will ever match, created by a verb whose whole job is to name them.

// identityRefusal is the refusal a lead with nothing to be called earns.
const identityRefusal = "PromoteNeedsIdentityError"

// leadNaming is the one place a lead's name is worked out.
const leadNaming = "leadIdentityName"

// leadNameField is the field whose present-but-empty reading is the defect. A
// function in the corpus reading it OFF THE LEAD has started a second answer,
// whatever it then does with it.
//
// Off the lead specifically: the same field name selected on the candidate is a
// read of what the ladder already matched on, which is the correct thing to do
// and the whole point of deriving the candidate once.
const leadNameField = "FullName"

func TestTheIdentityGuardAndTheCandidateReadOneName(t *testing.T) {
	var guards, builders int
	forEachModuleFunc(t, func(parsed moduleFile, fn *ast.FuncDecl) {
		if fn.Name.Name == leadNaming {
			return
		}
		// Both halves are found by what they DO — one refuses a lead for
		// having no identity, the other derives the candidate the ladder
		// matches on — so a third function joining either half is judged
		// the day it is written rather than the day somebody remembers
		// this test.
		guard := refusesWith(fn.Body, identityRefusal)
		derives := takesA(fn, leadType) &&
			(constructs(fn.Body, candidateType) || mutatesResultOf(fn.Body, candidateBuilder))
		if !guard && !derives {
			return
		}
		if guard {
			guards++
		}
		if derives {
			builders++
		}
		if !callsFunc(fn, leadNaming) {
			t.Errorf("%s (%s) decides what a lead is called without %s.\n\nThe guard and the "+
				"candidate agree on whether a lead has an identity only while ONE function "+
				"answers it. Take %s.", fn.Name.Name, parsed.name, leadNaming, leadNaming)
		}
		if pos := readsFieldOf(fn, leadType, leadNameField); pos.IsValid() {
			t.Errorf("%s reads the lead's .%s directly at %s.\n\nThat is the second reading: "+
				"`!= nil` and `== \"\"` disagree about a full_name that is present and empty, "+
				"and the lead that falls between them promotes into a person with no name. "+
				"Read %s.", fn.Name.Name, leadNameField, parsed.fset.Position(pos), leadNaming)
		}
	})
	// Either half at zero means this gate examined a population that no longer
	// exists — the refusal renamed, the candidate moved — and a gate holding
	// nothing reports exactly what a gate holding everything does.
	if guards == 0 {
		t.Fatalf("nothing in this module constructs a %s, so no verb refuses a lead for having "+
			"no identity and this gate is watching a rule that has moved", identityRefusal)
	}
	if builders == 0 {
		t.Fatalf("no function derives a %s from a lead, so this gate is watching a derivation "+
			"that has moved", candidateType)
	}
}

// refusesWith reports whether body constructs the named error type. Unlike
// constructs it counts an EMPTY literal: a sentinel error carries its meaning
// in its type, and `&PromoteNeedsIdentityError{}` is the whole refusal.
func refusesWith(body *ast.BlockStmt, typeName string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		lit, isLit := node.(*ast.CompositeLit)
		if isLit && lit.Type != nil && typeText(lit.Type) == typeName {
			found = true
		}
		return !found
	})
	return found
}

// callsFunc reports whether fn calls the named function of this package,
// written bare or through a receiver it holds.
func callsFunc(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if call, isCall := node.(*ast.CallExpr); isCall && callsNamed(call, name) {
			found = true
		}
		return !found
	})
	return found
}

// holdersOf names the identifiers in fn bound to the named type — its receiver
// and its parameters. A field read is only about that type when it is selected
// on one of these.
func holdersOf(fn *ast.FuncDecl, typeName string) map[string]bool {
	held := map[string]bool{}
	fields := []*ast.Field{}
	if fn.Recv != nil {
		fields = append(fields, fn.Recv.List...)
	}
	if fn.Type.Params != nil {
		fields = append(fields, fn.Type.Params.List...)
	}
	for _, field := range fields {
		if typeText(field.Type) != typeName {
			continue
		}
		for _, name := range field.Names {
			held[name.Name] = true
		}
	}
	return held
}

// readsFieldOf answers where fn selects the named field ON A VALUE OF THE NAMED
// TYPE, or an invalid position when it does not.
//
// The type matters. Selecting `.FullName` on the candidate is reading what the
// ladder matched on; selecting it on the LEAD is the second reading this holds
// against. A gate matching the field name alone cannot tell them apart, and
// would refuse the correct one.
func readsFieldOf(fn *ast.FuncDecl, typeName, field string) token.Pos {
	holders := holdersOf(fn, typeName)
	if len(holders) == 0 {
		return token.NoPos
	}
	var at token.Pos
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		sel, isSel := node.(*ast.SelectorExpr)
		if !isSel || sel.Sel == nil || sel.Sel.Name != field {
			return true
		}
		if base, isIdent := sel.X.(*ast.Ident); isIdent && holders[base.Name] {
			at = sel.Sel.Pos()
		}
		return !at.IsValid()
	})
	return at
}
