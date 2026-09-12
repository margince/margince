// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every obligation a messaging pack declares is one the engine applies, or one
// this file records as not yet applied and says why.
//
// The gate beside this one holds that a declared rule set REACHES the registry.
// Neither it nor the capability census can see what happens after: a field that
// lands in the registry, merges correctly through Strictest, and is then read
// by nobody. Every test in that chain passes, because each one is asking about
// the step it owns.
//
// The consequence is a pack that states a country's law and an installation
// that does not follow it. extensions/vn declares the Decree 91/2020/ND-CP
// subject label, the advertiser-identification disclosure and the acknowledged
// opt-out; the engine applies none of the three today, and no decision records
// the ruleset version it was taken under. Before this gate nothing held that
// apart: the pack's Rules literal reads as a complete statement of the decree,
// and which of its fields reach a consumer could only be learnt by grepping for
// each one. The field comments now say which are unapplied, and this is what
// keeps them true.
//
// So the register below is the answer to "which ones bind": a field is applied
// by a named reader in the engine, or it is listed here with the reason it is
// not. Both halves are checked — an entry for a field that HAS a reader fails
// just as a missing entry does, because a register that keeps stale lines
// stops describing the tree.
//
// WHAT DECIDES THE LIST. The fields come from messaging.Rules itself, for the
// reason the capability census derives its own corpus from extension.Extension:
// a list written here would be a second copy of that struct, and the copy that
// goes short is the one that still passes. A new obligation field is therefore
// a failing test until somebody either wires it or records why not.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	// rulesSurfaceFile publishes the rule shape a pack declares.
	rulesSurfaceFile = "pkg/extension/messaging/messaging.go"
	// rulesType is the declaration whose fields are the obligations.
	rulesType = "Rules"
	// rulesEngineDir is the engine that applies them.
	rulesEngineDir = "internal/modules/consent"
)

// rulesIdentityFields name the rule set rather than binding anything.
//
// Jurisdiction alone. It is the registry's key and the boot's duplicate check
// (internal/compose/extensionpolicy.go), so demanding an engine reader for it
// would be asking the engine to apply a label.
//
// Version is NOT identity, though it reads like it: messaging.Rules says it is
// "stamped onto every decision taken under it", which is an obligation and a
// promise to a subject who later asks which rules their message was judged
// under. Excluding it here was this gate's own first blind spot. The register
// below carried it until consent.rulesetStamp began writing it onto every
// staging and transmit row, so it is an applied obligation now and belongs in
// neither list.
// Instruments names the LAWS the version stands for and binds nothing. It is
// the citation a subject's decision is read back against — "Decree 91/2020/ND-CP",
// not an obligation the engine could discharge — so demanding an engine reader
// for it would be asking the engine to apply a footnote. The obligations those
// instruments carry are the other fields, and each answers to this gate on its
// own.
var rulesIdentityFields = []string{"Jurisdiction", "Instruments"}

// unappliedRules are the obligations the engine does not read yet, each with
// the reason it cannot.
//
// A line here is a statement that the product declares a rule it does not
// follow, so each says what is missing rather than that the work is pending.
// Removing a line is the point of the register; adding one is a decision that
// the gap is worth shipping, and it belongs in the change that opens it.
//
// A gatekit waiver rather than a bare map, so the two ways a line goes stale
// are reported by the shared helper instead of a second hand-written check:
// AssertAllMatched sees both a field that has since gained a reader and one the
// struct no longer declares, because the arm below asks Waived only about a
// field it has already found unapplied.
var unappliedRules = gatekit.Waive(map[string]string{
	"SubjectPrefix": "nothing prepends it. A send composes its own subject — " +
		"activities.SendEmailInput.Subject is caller-supplied text — and no step " +
		"between that and the provider consults the applicable rules, so an " +
		"advertising message leaves unmarked whatever a pack declares",
	"OptOutAcknowledgement": "no acknowledgement is sent. The controller lane is the only " +
		"one that may write to somebody who has just suppressed themselves, and it " +
		"registers no template for this",
})

func TestEveryDeclaredMessagingObligationIsAppliedOrRecorded(t *testing.T) {
	t.Parallel()

	obligations := ruleObligationFields(t)
	readers := engineRuleReaders(t)

	for _, field := range obligations {
		// READ IS NOT APPLIED, and this gate used to conflate them. A function
		// that reads an obligation satisfies a read-census perfectly while
		// nothing calls it, so the field looks applied and no message changes —
		// which is exactly what happened when DisclosuresFor was written and
		// the register line deleted in the same change. unreachedReaders names
		// the fields whose only reader is waiting for a caller, so they stay in
		// the register below rather than passing on a read nobody makes.
		if readers[field] && !unreachedReaders[field] {
			continue
		}
		if unappliedRules.Waived(t, field) {
			continue
		}
		t.Errorf("messaging.Rules.%s is declared and no engine code reads it.\n\n"+
			"A pack declaring this field states an obligation the installation does not "+
			"discharge, and nothing tells an operator which of a country's rules actually "+
			"bind. Either apply it in %s, or add it to unappliedRules with the reason it "+
			"cannot be applied yet.",
			field, rulesEngineDir)
	}
	unappliedRules.AssertAllMatched(t)
}

// unreachedReaders are the obligations whose only engine reader has no caller.
//
// A field here is read and NOT applied: the mapping from obligation to text
// exists and no message carries the result. It stays in unappliedRules, and
// TestEveryRuleReaderIsReachableFromASend below names the function waiting.
//
// An entry leaves this map when a send path calls the reader, which is the same
// moment its register line goes.
var unreachedReaders = map[string]bool{}

// ruleReaderCallers maps a rules FIELD to the file where the seam is crossed.
//
// The key names the field (Rules.Disclosures); the value names the file whose
// call reaches the reader. The check below asks whether that file calls a
// function of the key's name, which is what catches an adapter gutted while its
// wiring stays in place.
//
// An EMPTY value means not yet reached, and the field stays in unappliedRules
// with the matching reason. The two entries are the same gap seen from two
// sides: the register says the obligation is not applied, and this says which
// function is waiting for a caller.
//
// gatekit:fixture the rule readers and the send paths that call them
var ruleReaderCallers = map[string]string{
	// The compose adapter, which is where the seam is actually crossed.
	// activities/unsubscribe.go calls its own store helper, which calls the
	// injected resolver — so naming the module file would pass on a call that
	// never reaches consent if the wiring were removed.
	//
	// The KEY is the field's reader (DisclosuresFor reads Rules.Disclosures);
	// the value names the function the adapter calls to get there, which opens
	// its own transaction around it.
	"Disclosures": "backend/internal/compose/disclosurefooter.go",
}

// TestEveryRuleReaderIsReachableFromASend closes this gate's own blind spot.
//
// The census above asks whether any engine code READS an obligation, which it
// treats as applying it. A function that reads one and nothing calls satisfies
// it perfectly — and that is not a hypothetical: consent.DisclosuresFor was
// written, read Disclosures, let the register line be deleted, and had no
// production caller. The obligation was exactly as unapplied as before, and the
// register said it was done.
//
// Reading a rule is not applying it. Applying it means a message carries the
// consequence, which requires somebody to ask.
func TestEveryRuleReaderIsReachableFromASend(t *testing.T) {
	t.Parallel()
	for reader, caller := range ruleReaderCallers {
		if caller == "" {
			// Recorded rather than failed, because the register above carries
			// the same gap and failing twice for one missing consumer would
			// make the second failure noise. What this adds is the NAME of the
			// function waiting, which the register cannot say.
			t.Logf("%s reads a declared obligation and no send path calls it yet. "+
				"A reader nothing calls satisfies the census above while changing no "+
				"message, so the obligation is recorded as applied and is not. Either "+
				"name the caller here once one exists.", reader)
			continue
		}
		if !sendPathCalls(t, caller, reader) {
			t.Errorf("%s no longer calls %s — the obligation stopped being applied and "+
				"nothing else says so", caller, reader)
		}
	}
}

// composedWithoutDisclosures are the send paths that build a message body
// without asking what it must disclose.
//
// Each is a FIRST CONTACT that plainly owes an Art. 13 disclosure, and each
// composes its own body rather than going through activities' send path — so
// the append that covers the ordinary send cannot reach them.
//
// The register above cannot hold this: its question is whether any engine code
// reads the obligation, and the answer is now yes. This is the narrower one it
// cannot ask — whether every path that sends reaches that reader.
//
// gatekit:fixture the send paths that compose a body without disclosures
var composedWithoutDisclosures = map[string]string{
	"backend/internal/compose/controllermail.go": "the installation's own mail — a privacy " +
		"notice, a confirm link — is rendered by consent.RenderControllerTemplate and staged " +
		"straight onto a delivery. It never reaches activities' send path, so a notice that " +
		"exists to discharge an Art. 14 duty leaves without the controller identity the same " +
		"pack demands on every first contact",
	"backend/internal/modules/activities/channelsend.go": "a channel message builds its own " +
		"outbound row from the caller's body and sends it unchanged. Nothing here is " +
		"email-specific about the obligation — a first message over Telegram is as much a " +
		"first contact as one over mail",
	"backend/internal/modules/dealrooms/invitemail.go": "a buyer invitation goes out through " +
		"the raw mailer, which adds headers rather than composing anything. " +
		"gates/directmailbypass_test.go already ratifies this mailer's existence; what it " +
		"does not say is that an invitation is often the first contact with its recipient",
}

// TestEverySendPathThatComposesABodyAsksWhatItOwes keeps the register above
// honest: every entry names a file that still exists and still says why.
//
// WHAT IT DOES NOT DO, stated plainly because an earlier version of this
// comment claimed otherwise: it does not DISCOVER a new undisclosed send path.
// It reads the map and nothing else, so a path added tomorrow that composes a
// body and asks nothing passes here in silence.
//
// Discovery was tried and withdrawn. Every anchor wide enough to catch these
// three — a Body field in a struct literal, a call named Stage — matches
// dozens of unrelated writers, and the allowlist needed to quiet them would be
// longer than the register it guards. A waived gate reads as coverage, which is
// worse than a register that admits its own limit.
//
// So the register is a RECORD, not a fence: three paths that owe an Art. 13
// disclosure and do not carry it, written down where the next reader finds them
// instead of rediscovering them. Removing an entry by closing its gap is the
// point; the fence that would stop a fourth appearing does not exist yet.
func TestEverySendPathThatComposesABodyAsksWhatItOwes(t *testing.T) {
	t.Parallel()
	for path, reason := range composedWithoutDisclosures {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%s is recorded as composing without disclosures and says no reason. "+
				"A path here ships a message owing an obligation it does not carry, which "+
				"is a decision somebody has to make rather than inherit.", path)
		}
		if _, err := os.Stat(filepath.Join(repoRoot, path)); err != nil {
			t.Errorf("%s no longer exists — this record names a send path that is gone. "+
				"Point it at what replaced the file, or delete the entry if the gap closed "+
				"with it: %v", path, err)
		}
	}
}

// sendPathCalls reports whether the named file CALLS the named function, as
// opposed to merely mentioning it.
//
// A substring match was the first version and it was too weak: gutting the call
// while leaving the type name in a variable declaration still matched, so the
// gate passed over a seam that no longer crossed. This walks the syntax and
// looks for a call expression, which a comment or a type reference is not.
func sendPathCalls(t *testing.T, file, fn string) bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(),
		filepath.Join(repoRoot, file), nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	var called bool
	ast.Inspect(parsed, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			called = called || fun.Name == fn
		case *ast.SelectorExpr:
			called = called || fun.Sel.Name == fn
		}
		return true
	})
	return called
}

// TestTheEngineReadsAtLeastOneRuleField is the positive control for the reader
// scan.
//
// The arm above reports a field applied when the scan finds a selector for it.
// A directory that no longer exists is not the case to worry about —
// parseTreeUnder fatals on that. The quiet one is a scan that still parses and
// stops recognising reads: the lookup renamed, its results reordered, or the
// engine's rule handling moved to a package outside this directory. Every field
// then reports unapplied, which does fail, but as findings naming the wrong
// subject — they read as somebody having deleted the engine's rule handling.
//
// So this asserts the scan still sees the applications the tree is known to
// have, and says which cause to suspect when it does not.
func TestTheEngineReadsAtLeastOneRuleField(t *testing.T) {
	t.Parallel()

	readers := engineRuleReaders(t)
	// All four fields the engine reads today, not a sample: the corpus is small
	// and known, and a scan that kept finding one of them while the rest moved
	// to a subpackage would report the movers unapplied rather than say what
	// happened.
	//
	// No shipped pack narrows the two windows — extensions/de and extensions/vn
	// both declare the core defaults, and vn's own comment says so. What the
	// scan proves is that the reader exists, which is what this gate asks; a
	// pack that one day sends a shorter window reaches an engine that reads it.
	for _, field := range []string{"FrequencyCap", "ReplyWindow", "DealFollowUpWindow", "MarketingExceptions"} {
		if !readers[field] {
			t.Fatalf("the reader scan finds nothing applying messaging.Rules.%s, which %s does apply.\n\n"+
				"The scan has stopped seeing the engine, so every field reads as unapplied and "+
				"the register above is being checked against an empty reading.", field, rulesEngineDir)
		}
	}
}

// ruleObligationFields is the field names of messaging.Rules less the identity
// pair.
func ruleObligationFields(t *testing.T) []string {
	t.Helper()
	file := parseGo(t, rulesSurfaceFile)
	var fields []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || spec.Name.Name != rulesType {
			return true
		}
		structType, isStruct := spec.Type.(*ast.StructType)
		if !isStruct {
			return false
		}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				fields = append(fields, name.Name)
			}
		}
		return false
	})
	if len(fields) == 0 {
		t.Fatalf("%s declares no %s fields — the subject this gate reads has moved, and a census "+
			"that found nothing to judge reports every obligation applied", rulesSurfaceFile, rulesType)
	}
	for _, identity := range rulesIdentityFields {
		if !slices.Contains(fields, identity) {
			t.Fatalf("messaging.%s has no %s field — this census excludes the identity pair by name, "+
				"so a rename here silently changes what it demands of the engine", rulesType, identity)
		}
	}
	return slices.DeleteFunc(fields, func(f string) bool { return slices.Contains(rulesIdentityFields, f) })
}

// rulesLookup is the engine's one entry point to a jurisdiction's rules. Every
// read of a rule field starts at a value this returns.
const rulesLookup = "applicableRules"

// engineRuleReaders reports which rule fields the engine reads.
//
// It counts a selector only when its base is a local bound from rulesLookup, so
// a field is "applied" because the engine read it off a rule set and not
// because some unrelated type in the package happens to carry the same name.
// Matching the bare name was the first shape of this scan and it was wrong in
// the one direction a census may not be: messaging.Rules nests types whose
// field names (Window, Messages, Kind, Version) the engine already selects on
// other values, so an obligation added under any of those names would have been
// born reported-as-applied, with nothing failing.
//
// Where it cannot tell, it REFUSES rather than admits — the rule
// machineryapplied_test.go states for the same choice. A read through a shape
// this does not follow (a rule set stored in a struct field, passed through a
// helper, ranged over) reports the field unapplied, which fails loudly and is
// fixable. The opposite error is the silent one.
func engineRuleReaders(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	read := map[string]bool{}
	for _, file := range parseTreeUnder(t, fset, rulesEngineDir) {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			bound := ruleLocalsIn(fn.Body)
			if len(bound) == 0 {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, isSel := n.(*ast.SelectorExpr)
				if !isSel {
					return true
				}
				if base, isIdent := sel.X.(*ast.Ident); isIdent && bound[base.Name] {
					read[sel.Sel.Name] = true
				}
				return true
			})
		}
	}
	return read
}

// ruleLocalsIn names the locals in one function body that hold a rule set: the
// identifiers assigned from a rulesLookup call.
//
// Only the first result is taken. The lookup returns (Rules, bool, error), and
// binding the name to position zero means a reshuffle of those results stops
// the scan finding readers rather than silently following the wrong one — the
// positive control is what reports that.
func ruleLocalsIn(body *ast.BlockStmt) map[string]bool {
	bound := map[string]bool{}
	rebound := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, isAssign := n.(*ast.AssignStmt)
		if !isAssign || len(assign.Lhs) == 0 {
			return true
		}
		fromLookup := len(assign.Rhs) == 1 && isRulesLookupCall(assign.Rhs[0])
		for i, lhs := range assign.Lhs {
			name, isIdent := lhs.(*ast.Ident)
			if !isIdent || name.Name == "_" {
				continue
			}
			if fromLookup && i == 0 {
				bound[name.Name] = true
				continue
			}
			// Any other binding or assignment of the name: an inner block's
			// own `rules`, a reassignment, a range variable. The scan reads a
			// name and not a scope, so a name that means two things in one
			// function means nothing it can trust.
			rebound[name.Name] = true
		}
		return true
	})
	for _, other := range []map[string]bool{rebound, otherBindingsIn(body)} {
		for name := range other {
			delete(bound, name)
		}
	}
	return bound
}

// otherBindingsIn names identifiers a function binds by some route other than
// an assignment: a range variable, a var declaration, a closure's parameter.
//
// Collected so ruleLocalsIn can DROP a rule local whose name is also bound
// elsewhere in the same function. The scan matches a name, not a lexical scope,
// so a shadowed name would otherwise let a read off an unrelated value count as
// a read of the rule set — the silent direction. Dropping it reports the field
// unapplied instead, which fails loudly and is what the shadowing author must
// then fix by renaming.
func otherBindingsIn(body *ast.BlockStmt) map[string]bool {
	bound := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.ValueSpec:
			for _, name := range node.Names {
				bound[name.Name] = true
			}
		case *ast.RangeStmt:
			for _, expr := range []ast.Expr{node.Key, node.Value} {
				if ident, isIdent := expr.(*ast.Ident); isIdent {
					bound[ident.Name] = true
				}
			}
		case *ast.FuncLit:
			for _, field := range node.Type.Params.List {
				for _, name := range field.Names {
					bound[name.Name] = true
				}
			}
		}
		return true
	})
	return bound
}

// isRulesLookupCall reports whether an expression is a call resolving the
// applicable rules.
func isRulesLookupCall(expr ast.Expr) bool {
	call, isCall := expr.(*ast.CallExpr)
	return isCall && callsRulesLookup(call)
}

// callsRulesLookup reports whether a call resolves the applicable rules,
// through a receiver or bare.
func callsRulesLookup(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name == rulesLookup
	case *ast.SelectorExpr:
		return fun.Sel.Name == rulesLookup
	}
	return false
}
