// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The plaintext confirm token goes into the mail body and nowhere else.
//
// The token is a bearer credential over one person's record: whoever holds it
// can open what is held about them, correct it, and answer for them about
// marketing. The claim that a grant made through it is the SUBJECT'S rests
// entirely on the plaintext having reached only the subject's own mailbox — so
// a copy anywhere an operator can read is not a leak of a secret, it is the
// evidence quietly ceasing to be evidence.
//
// The retired operator-token endpoint is what this prevents returning. It handed
// the plaintext back to the caller, who could paste it straight in again, so one
// person could mint and redeem a confirmation the subject never saw. Those rows
// are still on the proof log and authorize nothing (recordedStateFor's
// issuance_trigger IS NOT NULL clause is what excludes them); this gate is what
// stops the shape coming back.
//
// The check is INVERTED: rather than listing the calls a token must not reach,
// it lists the ones it MAY, and every other use of the plaintext is a finding.
//
// A deny-list of sinks is a census that can fail short. The earlier version of
// this gate carried eleven sink names, and two probes walked past it: an
// fmt.Fprintf to a writer, and — worse, through a sink the list DID hold — a
// slog.InfoContext handed the bare local the token actually lives in, because
// the value match only recognised a ".Token" field selection. Both published a
// bearer credential over one person's record and both read green. Publishing is
// open-ended and cannot be enumerated; the legitimate destinations are named,
// are named in the code, and change only when somebody edits this file.
//
// The plaintext enters the package in two ways: it is MINTED by newConfirmToken,
// and it arrives as a "token" parameter — on the store lookups and, before them,
// on the unauthenticated HTTP handlers a mailed link resolves to.
//
// It may then be laundered (hashPublicToken, confirmLink), handed to a consumer
// that looks it up and returns a record, or placed on the IssuedConfirm the mail
// path reads. Everything else is a finding.
//
import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	// consentDir is the package that mints and spends the plaintext.
	consentDir = "internal/modules/consent"
	// plaintextField is the field of IssuedConfirm carrying an unhashed token.
	plaintextField = "Token"
	// carrierType is the struct that carries the plaintext to the mail path.
	carrierType = "IssuedConfirm"
	// tokenSource mints the plaintext. Its result is what this gate follows.
	tokenSource = "newConfirmToken"
)

// tokenParameterName is what a plaintext token is called where it arrives as a
// parameter. Every function in this package taking one holds a bearer
// credential — the public HTTP handlers read it straight off the URL.
//
// DERIVED from the parameter, not from a list of functions. A first version of
// this gate seeded three named lookups instead, and the review found the two
// most exposed holders were not among them: GetConfirmDetails and
// SubmitConfirmDetails, the unauthenticated handlers a mailed link resolves to.
// A slog line in either published the credential and read green, which is the
// same failure this gate was rewritten to fix. Naming functions was how the
// corpus fell short the first time, and naming the value is narrower: it goes
// stale for ONE spelling rather than for every function added afterwards.
//
// It is not immune. A parameter called credential rather than token is outside
// the walk, and no floor notices one holder lost among several. Closing that
// needs call-graph propagation — a function whose argument at a tainted
// position is itself tainted becomes watched — which is a bigger change than
// this gate. Until then the spelling is the contract, and it is stated here so
// the next author knows it is one.
const tokenParameterName = "token"

// ratifiedDestinations are the calls a plaintext token may be handed to, each
// named for what makes it safe. Anything else is a finding.
//
// Two shapes qualify. A LAUNDERER turns the credential into something safe to
// carry onward — the digest the row holds, or the URL that goes in the mail. A
// CONSUMER looks the token up and hands back a record; the token goes in and
// does not come out. Every consumer here is still checked by this gate in its
// own right, because each takes the value as a "token" parameter and is walked
// like any other holder.
//
// The preference-centre token is a DIFFERENT credential with its own resolver
// and its own storage — it is kept in plaintext, where the confirm token is
// hashed — so ResolvePreferenceToken is ratified here as a consumer and is not
// otherwise this gate's subject.
//
// gatekit:fixture why each destination may receive the plaintext
var ratifiedDestinations = map[string]string{
	"hashPublicToken":         "returns only the digest, which is what the row holds",
	"confirmLink":             "returns the URL that goes in the mail body",
	"ResolveConfirmToken":     "looks the token up and returns a record, never the token",
	"subjectOfConfirmTokenTx": "looks the token up and returns whose link it is",
	"spendConfirmTokenTx":     "marks the link used and returns a record",
	"SubmitConfirmation":      "is the store method the public handler forwards its token to",
	"ResolvePreferenceToken":  "consumes a different credential, the preference-centre token",
	"QueryRow":                "is the parameterised lookup: the token is a bound argument, never text in a statement",

	// The WITHDRAWAL credential travels the same edge and is tracked the same
	// way. It is hashed at rest like the confirm token, so the plaintext is
	// equally short-lived and equally worth following.
	"ResolveWithdrawalToken":            "looks the token up and returns an address and a scope, never the token",
	"resolveWithdrawalTokenTx":          "is the same resolve inside a caller's transaction",
	"legacyPreferenceTokenAsWithdrawal": "looks up an OLD preference token and returns a withdrawal ref, never the token",
	"HasPrefix":                         "reads the credential's family prefix and returns a bool; a prefix test discloses nothing the link's own shape does not",
	"StopForCredential":                 "re-resolves the token inside the transaction that writes and returns nothing about it. It receives the plaintext DELIBERATELY: resolving in the handler and writing in a second transaction let an erasure commit in the gap, after which the write put the erased plaintext address back",

	"stopForCredential": "resolves the token and records the stop it presses, returning nothing about the token itself",

	"oneClickSubject": "resolves the press to the person it acts for, trying both credential families, and returns that person and the withdrawal scope — never the token",
}

// launderers are the two functions whose OWN BODY this gate does not inspect,
// because the body is where the credential legitimately becomes something else.
//
// Keyed RECEIVER-QUALIFIED. calleeName drops the receiver, so a bare name would
// let any method called confirmLink, on any type, both launder the taint at its
// call sites and have its own body skipped — two bypasses out of one name. The
// launderers are a package function and one store method, and that is what the
// keys say.
//
// Deliberately NOT the whole ratified set. Being allowed to RECEIVE the token
// and being exempt from inspection are different permissions, and conflating
// them is exploitable: adding SubmitConfirmation to the ratified set — correct
// on its own terms, it is what the handler forwards to — silently stopped its
// body being checked, and it holds the token throughout. A planted
// fmt.Println(token) inside it then read green. So this map names only the two
// functions that end the credential, and receipt is decided separately above.
var launderers = map[string]bool{
	"hashPublicToken":   true,
	"Store.confirmLink": true,
}

func TestThePlaintextConfirmTokenReachesNoSinkButTheMail(t *testing.T) {
	t.Parallel()

	files := parseConsentPackage(t)
	// Every function that HANDS BACK the carrier. A local bound from one of
	// these holds the plaintext without ever naming the field, which is how the
	// whole struct reaches a sink past a matcher that only knows `.Token`.
	// Derived from the package rather than listed, so a third minting function
	// enrols itself.
	minters := carrierReturningFuncs(files)
	// Under-recognition is how this arm dies quietly: a minters set that came
	// back empty would walk no carrier at all and report PASS, exactly as the
	// taint floor below guards the other half.
	if len(minters) < carrierMinterFloor {
		t.Fatalf("found %d functions returning %s, fewer than the %d this package holds — the "+
			"carrier arm has stopped seeing its subject, and a whole-struct leak would read green",
			len(minters), carrierType, carrierMinterFloor)
	}
	checked, tainted := 0, 0
	for path, file := range files {
		checked++
		for _, fn := range functionsIn(file) {
			if launderers[receiverQualified(fn)] {
				// A launderer's own body is where the plaintext becomes a
				// digest or a URL. Checking it would report the hash call it
				// exists to make.
				continue
			}
			holders := taintedNamesIn(fn)
			carriers := carrierLocalsIn(fn, minters)
			// A function with no tainted LOCAL can still read the credential:
			// `issued.Token` straight into a struct literal names nothing, and
			// skipping on an empty name set let a handler hand the plaintext to
			// WriteJSON with the walk never entering the function. The field
			// read is the taint, whether or not a local ever holds it.
			if len(holders) == 0 && len(carriers) == 0 && !readsTheCarrierField(fn) {
				continue
			}
			tainted++
			for _, finding := range wholeCarrierUses(fn, carriers) {
				t.Errorf("%s: %s hands the WHOLE %s to %s. The struct carries the plaintext in a "+
					"field, so publishing the value publishes the credential — a matcher keyed on "+
					"the field name cannot see this, because the leak names no field. Pass the "+
					"fields the caller actually needs.",
					path, finding.holder, carrierType, finding.call)
			}
			for _, finding := range unratifiedUsesOf(fn, holders) {
				t.Errorf("%s: %s hands the plaintext confirm token to %s, which is not one of "+
					"the destinations it may reach (%s). The token is a bearer credential "+
					"over one person's record, and a copy anywhere an operator can read it ends "+
					"the claim that a grant made through it was the subject's own.",
					path, finding.holder, finding.call, ratifiedList())
			}
		}
	}
	if checked == 0 {
		t.Fatal("no consent sources were walked: this gate is looking in the wrong place")
	}
	// Under-recognition is the one way this gate must not break: a walk that
	// tracked nothing would report PASS while the token flowed anywhere.
	if tainted < taintedFunctionFloor {
		t.Fatalf("followed the plaintext through %d functions, fewer than the %d the package "+
			"holds — the taint walk has stopped seeing its subject", tainted, taintedFunctionFloor)
	}
}

// carrierMinterFloor is how many carrier-returning functions the package holds:
// IssueConfirmToken, IssueConsentLink and the shared issueLink beneath them.
//
// A floor rather than an exact count, so a fourth does not fail the gate — but
// a set that lost one, or came back empty because the detector broke, does. Set
// at what the package actually holds rather than below it: a floor with slack
// is a floor a loss can hide under, which is the whole failure this guards.
const carrierMinterFloor = 3

// taintedFunctionFloor is how many functions the walk must still be following.
// Set below the count the tree holds, so adding a holder does not fail the gate,
// and far enough above zero that a walk which lost its SUBJECT does — a matcher
// narrowed, a spelling changed, the parse pointed elsewhere. It does not notice
// one holder lost among several, and "found at least one" would not notice a
// walk that lost most of them.
const taintedFunctionFloor = 6

// functionsIn lists a file's function declarations.
func functionsIn(file *ast.File) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			out = append(out, fn)
		}
	}
	return out
}

// carrierReturningFuncs names every function in the package whose results
// include the carrier, receiver-independent.
//
// These are what mint or forward the credential. Deriving the set beats listing
// it for the reason this file derives everything else: a list goes short
// silently, and the half that stopped matching still reports PASS.
func carrierReturningFuncs(files map[string]*ast.File) map[string]bool {
	out := map[string]bool{}
	for _, file := range files {
		for _, fn := range functionsIn(file) {
			if declaresTheCarrier(fn) {
				out[fn.Name.Name] = true
			}
		}
	}
	return out
}

// carrierLocalsIn names the locals in one function bound from a call that
// returns the carrier.
//
// The struct is the credential, not merely a box around it: it carries the
// plaintext in a field, so handing the WHOLE value to a log publishes the token
// as surely as reading the field would. A matcher keyed on the field name
// cannot see that — the leak names no selector — and this is the arm that does.
//
// IssuedConfirm.Token carries `json:"-"`, which stops the JSON sinks on its
// own. This arm is what covers the ones no tag reaches: slog, fmt, a template.
func carrierLocalsIn(fn *ast.FuncDecl, minters map[string]bool) map[string]bool {
	out := map[string]bool{}
	bind := func(lhs []ast.Expr, rhs []ast.Expr) {
		if len(rhs) != 1 || len(lhs) == 0 {
			return
		}
		call, isCall := rhs[0].(*ast.CallExpr)
		if !isCall || !minters[calleeName(call)] {
			return
		}
		if name, isIdent := lhs[0].(*ast.Ident); isIdent && name.Name != "_" {
			out[name.Name] = true
		}
	}
	ast.Inspect(fn, func(n ast.Node) bool {
		// BOTH binding forms. `issued, err := mint()` is an AssignStmt and
		// `var issued, err = mint()` is a ValueSpec, and a walk that knew only
		// the first left the whole function untracked for anybody who wrote the
		// second — a spelling difference deciding whether a leak is seen.
		switch v := n.(type) {
		case *ast.AssignStmt:
			bind(v.Lhs, v.Rhs)
		case *ast.ValueSpec:
			names := make([]ast.Expr, 0, len(v.Names))
			for _, name := range v.Names {
				names = append(names, name)
			}
			bind(names, v.Values)
		}
		return true
	})
	return out
}

// wholeCarrierUses reports every call handed a carrier value entire.
//
// The BARE identifier only. `issued.DeliveredTo` is a field read and publishes
// nothing — both confirmation doors build their response that way, and treating
// the local as tainted wherever it appears reported them as leaks, which is the
// cry-wolf shape this file already learned once.
func wholeCarrierUses(fn *ast.FuncDecl, carriers map[string]bool) []sinkFinding {
	var out []sinkFinding
	if len(carriers) == 0 {
		return nil
	}
	ast.Inspect(fn, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		callee := calleeName(call)
		for _, arg := range call.Args {
			if !carrierEscapesIn(arg, carriers) {
				continue
			}
			// A minter handing the carrier onward is the value doing its job.
			if minterReceives(callee) {
				continue
			}
			out = append(out, sinkFinding{holder: fn.Name.Name, call: callee})
		}
		return true
	})
	return out
}

// carrierEscapesIn reports whether an argument hands a carrier value out whole,
// however it is wrapped.
//
// The BARE identifier was the first shape and it was too narrow: `[]IssuedConfirm{issued}`
// and `struct{ C IssuedConfirm }{C: issued}` both publish the value and name no
// selector, so a matcher looking only at the top-level argument let them past.
// json:"-" saves the JSON sinks, but a log line is not tag-aware, so the gate
// has to see the wrapping.
//
// A FIELD READ is still not an escape — `issued.DeliveredTo` inside a literal is
// how both doors legitimately build their response, and treating the local as
// tainted wherever it appears is the cry-wolf shape this file learned once
// already. So a selector on the carrier stops the descent.
func carrierEscapesIn(arg ast.Expr, carriers map[string]bool) bool {
	found := false
	ast.Inspect(arg, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.SelectorExpr:
			// Reading a field off the carrier publishes that field, not the
			// credential. Do not descend into the base.
			if id, isIdent := v.X.(*ast.Ident); isIdent && carriers[id.Name] {
				return false
			}
		case *ast.Ident:
			if carriers[v.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

// minterReceives reports whether a callee legitimately takes the whole carrier.
// Nothing does today; the hook exists so a future forwarder is ratified here by
// name rather than by widening the matcher and losing the arm.
func minterReceives(string) bool { return false }

// readsTheCarrierField reports whether a function reads IssuedConfirm.Token at
// all, independently of whether it binds the value to a name.
//
// The gate walks a function only when it has something to follow, and a name
// set was the whole of that test until a handler read the field directly into a
// composite literal — no local, so no holder, so the function was never
// entered. This is the second way in.
func readsTheCarrierField(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		sel, isSel := n.(*ast.SelectorExpr)
		if isSel && sel.Sel.Name == plaintextField {
			found = true
		}
		return !found
	})
	return found
}

// taintedNamesIn returns the identifiers inside one function that hold a
// plaintext token: the parameters of the functions handed one, the result of
// the mint, and any name assigned from another tainted name.
//
// Field selections of IssuedConfirm.Token count too, so a caller reading the
// struct is followed as well as the local it was built from.
func taintedNamesIn(fn *ast.FuncDecl) map[string]bool {
	names := map[string]bool{}
	if fn.Type.Params != nil {
		for _, p := range fn.Type.Params.List {
			if id, ok := p.Type.(*ast.Ident); !ok || id.Name != "string" {
				continue
			}
			for _, n := range p.Names {
				if n.Name == tokenParameterName {
					names[n.Name] = true
				}
			}
		}
	}
	// Assignments propagate the taint, and a later one can feed an earlier
	// read, so the walk repeats until nothing new is learned.
	for changed := true; changed; {
		changed = false
		ast.Inspect(fn, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			if taintAssignment(assign, names) {
				changed = true
			}
			return true
		})
	}
	return names
}

// taintAssignment binds the left-hand names an assignment puts a plaintext
// token into, and reports whether it learned anything new.
//
// The mint returns (string, error), so a single call filling several names
// taints only the FIRST. Binding them all was the earlier shape, and it made
// the error variable a secret: every errors.Is beside a mint was then reported
// as a leak, which is a gate that cries wolf until somebody deletes it.
func taintAssignment(assign *ast.AssignStmt, names map[string]bool) bool {
	learned := false
	bind := func(e ast.Expr) {
		if id, ok := e.(*ast.Ident); ok && id.Name != "_" && !names[id.Name] {
			names[id.Name] = true
			learned = true
		}
	}
	if len(assign.Rhs) == 1 && len(assign.Lhs) > 1 {
		if carriesPlaintext(assign.Rhs[0], names) {
			bind(assign.Lhs[0])
		}
		return learned
	}
	for i, rhs := range assign.Rhs {
		if i < len(assign.Lhs) && carriesPlaintext(rhs, names) {
			bind(assign.Lhs[i])
		}
	}
	return learned
}

// carriesPlaintext reports whether an expression IS a plaintext token: the
// mint's result, a tainted name, or a read of IssuedConfirm.Token.
//
// A call's RESULT is never the token, even when the token went in.
// ResolveConfirmToken(token) evaluates to a record, hashPublicToken(token) to
// a digest, confirmLink(token) to the URL that goes in the mail. Treating a
// result as tainted was the first shape of this walk, and it reported the
// handlers' own WriteJSON — which carries the resolved card — as a leak of the
// credential. A gate that cries wolf on correct code is one somebody deletes.
//
// A closure is a body, not a value: descending into one makes every name it
// reads part of the expression that merely runs it, which taints the error a
// transaction runner hands back.
func carriesPlaintext(e ast.Expr, names map[string]bool) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			if id, ok := v.Fun.(*ast.Ident); ok && id.Name == tokenSource {
				found = true
			}
			return false
		case *ast.SelectorExpr:
			if v.Sel.Name == plaintextField {
				found = true
			}
		case *ast.Ident:
			if names[v.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

type sinkFinding struct{ holder, call string }

// unratifiedUsesOf reports every call handed a plaintext token that is not one
// of the ratified destinations.
//
// A composite literal is not a call and is not reported: building the
// IssuedConfirm the mail path reads is the value's whole purpose. The struct's
// own field is followed by carriesPlaintext wherever it is read — including
// inside a literal handed to a call, which is how a handler would leak it.
func unratifiedUsesOf(fn *ast.FuncDecl, names map[string]bool) []sinkFinding {
	var out []sinkFinding
	ast.Inspect(fn, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.CallExpr:
			if finding, leaked := callLeaks(fn, v, names); leaked {
				out = append(out, finding)
			}
		case *ast.ReturnStmt:
			if returnLeaks(fn, v, names) {
				out = append(out, sinkFinding{holder: fn.Name.Name, call: "its own return"})
			}
		case *ast.SendStmt:
			if carriesPlaintext(v.Value, names) {
				out = append(out, sinkFinding{holder: fn.Name.Name, call: "a channel"})
			}
		}
		return true
	})
	return out
}

// returnLeaks reports whether a return statement hands the bare plaintext back.
//
// A function that HANDS BACK the token puts it somewhere this walk cannot
// follow: a caller treats a call's result as clean, so a consumer returning the
// credential would launder it on the way out.
//
// Returning the IssuedConfirm is different and is allowed. That struct exists
// to carry the plaintext to the mail path, its Token field is followed by
// carriesPlaintext wherever a caller reads it, and refusing it here would
// report the minting function for doing its job. What is refused is a return
// whose value IS the token: a bare identifier or a field read of it.
func returnLeaks(fn *ast.FuncDecl, stmt *ast.ReturnStmt, names map[string]bool) bool {
	if declaresTheCarrier(fn) {
		return false
	}
	for _, result := range stmt.Results {
		if carriesPlaintext(result, names) {
			return true
		}
	}
	return false
}

// declaresTheCarrier reports whether a function's results include the struct
// that legitimately carries the plaintext to the mail path.
func declaresTheCarrier(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil {
		return false
	}
	for _, r := range fn.Type.Results.List {
		if id, ok := r.Type.(*ast.Ident); ok && id.Name == carrierType {
			return true
		}
	}
	return false
}

// callLeaks reports whether one call publishes the plaintext.
func callLeaks(fn *ast.FuncDecl, call *ast.CallExpr, names map[string]bool) (sinkFinding, bool) {
	callee := calleeName(call)
	if ratifiedCall(call, callee, names) {
		return sinkFinding{}, false
	}
	for _, arg := range call.Args {
		if _, isBody := arg.(*ast.FuncLit); isBody {
			// A closure passed to Tx or WithInfraTx is a body the walk descends
			// into on its own, not a value handed outward. Reading it as an
			// argument would report the transaction runner for every leak
			// inside the closure and name the wrong call.
			continue
		}
		if carriesPlaintext(arg, names) {
			return sinkFinding{holder: fn.Name.Name, call: callee}, true
		}
	}
	return sinkFinding{}, false
}

// ratifiedCall reports whether this call is one the token may be handed to.
//
// A CONSUMER is ratified by name alone: it takes the credential and returns a
// record, and its own body is still walked like any other holder, so a leak
// inside it is still a finding. Nothing is gained by pinning its receiver.
//
// A LAUNDERER is ratified only when called plainly or on a receiver that holds
// the store, because a launderer's name ALSO buys its body an exemption from
// the walk. Keyed on the name alone, any method called confirmLink on any type
// would both launder the credential at its call sites and hide its own body —
// two bypasses out of one name. The probe that found it declared exactly that
// and read green.
func ratifiedCall(call *ast.CallExpr, callee string, names map[string]bool) bool {
	if _, ratified := ratifiedDestinations[callee]; !ratified {
		return false
	}
	if callee == boundArgumentSink {
		// The token is a BOUND argument here, never the statement. Ratifying
		// the call outright would admit tx.QueryRow(ctx, token), where the
		// credential IS the SQL text — a shape this gate would otherwise
		// describe as safe in the words it uses for a parameterised lookup.
		return !carriesPlaintextAt(call, statementArgument, names)
	}
	if !laundererNames[callee] {
		return true
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return endsInStoreReceiver(fun.X)
	}
	return false
}

// laundererNames is launderers keyed by bare name, for asking whether a CALL is
// to a launderer. launderers itself is receiver-qualified, which is the right
// key for asking whether a DECLARATION is one.
var laundererNames = map[string]bool{
	"hashPublicToken": true,
	"confirmLink":     true,
}

// carriesPlaintextAt reports whether one positional argument is the plaintext.
func carriesPlaintextAt(call *ast.CallExpr, pos int, names map[string]bool) bool {
	if pos >= len(call.Args) {
		return false
	}
	return carriesPlaintext(call.Args[pos], names)
}

// endsInStoreReceiver reports whether a selector base resolves to the consent
// store: its own receiver (s) or a handler's field holding it (h.store).
func endsInStoreReceiver(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name == consentStoreReceiver
	case *ast.SelectorExpr:
		return v.Sel.Name == consentStoreField
	}
	return false
}

const (
	// boundArgumentSink takes the token as a bound parameter, after the
	// statement. Its own body is outside this package.
	boundArgumentSink = "QueryRow"
	// statementArgument is where QueryRow's SQL text sits: after the context.
	statementArgument = 1
	// consentStoreReceiver is what every method on *Store names its receiver.
	consentStoreReceiver = "s"
	// consentStoreField is what the handlers call the store they hold.
	consentStoreField = "store"
)

// ratifiedList renders the allowed destinations for a finding.
func ratifiedList() string {
	var parts []string
	for name, why := range ratifiedDestinations {
		parts = append(parts, name+", which "+why)
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

// TestTheConfirmTokenIsMintedOnlyWhereThisGateWatches proves the scope claim the
// census rests on.
//
// The walk above reads one package because the plaintext is built in one place.
// If a second minting site appeared elsewhere in the tree, the census would keep
// reporting PASS while a token escaped through code it never reads. This asserts
// the field's home instead of assuming it.
func TestTheConfirmTokenIsMintedOnlyWhereThisGateWatches(t *testing.T) {
	t.Parallel()

	files := parseConsentPackage(t)
	minting := 0
	for _, file := range files {
		if constructsIssuedConfirm(file) {
			minting++
		}
	}
	if minting == 0 {
		t.Fatal("no file in the consent package constructs an IssuedConfirm: the type this gate " +
			"follows has moved, and the census is watching nothing")
	}
}

// constructsIssuedConfirm reports whether a file builds the type that carries
// the plaintext.
func constructsIssuedConfirm(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		id, ok := lit.Type.(*ast.Ident)
		if !ok || id.Name != "IssuedConfirm" {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == plaintextField {
				found = true
			}
		}
		return true
	})
	return found
}

// parseConsentPackage parses the consent module's non-test sources.
func parseConsentPackage(t *testing.T) map[string]*ast.File {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(consentDir, "*.go"))
	if err != nil {
		t.Fatalf("listing the consent package: %v", err)
	}
	fset := token.NewFileSet()
	out := map[string]*ast.File{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		out[path] = file
	}
	if len(out) == 0 {
		t.Fatalf("no sources found under %s: the gate is looking in the wrong place", consentDir)
	}
	return out
}
