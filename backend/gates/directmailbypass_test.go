// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Who may hand a message straight to the SMTP relay, bypassing comms_outbound.
//
// A send through platform/mailer skips everything the delivery lane provides: no
// comms_outbound row, so no authorization decision, no audit entry, no outbox
// event, no bounce accounting, no suppression check, and nothing in the
// subject-access export saying the message was ever sent. That is correct for a
// few messages and wrong for correspondence, and the difference is not visible
// at the call site — `mailer.Send(ctx, to, subject, body)` reads identically
// either way.
//
// So the population is enumerated here WITH the reason each one is not
// correspondence, and a new holder of the seam fails until somebody writes that
// sentence. The gate does not judge whether a reason is good; it makes an
// exemption a claim a reviewer can disagree with rather than an omission nobody
// noticed.
//
// The rule the reasons are measured against: a direct send is admissible when
// the message is the installation acting on its own account — an operator
// digest, a credential the recipient asked for, a link that IS the act — and
// never when it is correspondence with a data subject about the relationship,
// which is what the consent engine exists to authorize.
//
// THAT KNOWN GAP IS CLOSED. Consent's confirm-details link used to be mailed
// directly, and this comment used to say so — the message asking somebody to
// check what is held about them was itself unrecorded. It now stages a delivery
// through the controller lane (consent.stageConfirmMail → the compose queue),
// so it is no longer in the census and no longer needs a waiver naming its cost.
//
// EVERY WAIVER ALSO NAMES ITS PURPOSE, from the closed set below. The prose was
// doing two jobs at once — saying which admissible category a sender falls
// under, and arguing why. Prose alone cannot be counted, so a fifth category
// could arrive one plausible paragraph at a time and nobody would see the
// population had grown a new KIND of exemption rather than another instance of
// an existing one. Naming the purpose separates the two: the category is a
// decision somebody has to make deliberately, and the sentence beside it is
// still where the argument lives.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// directMailHolders ratifies each file that reaches the relay directly, keyed by
// its path from the backend root, with what bypassing the delivery lane costs.
var directMailHolders = gatekit.Waive(map[string]string{
	"internal/compose/weeklymailjobs.go": "the operator's own weekly digest, addressed to a seat in " +
		"this installation rather than to a data subject: there is no consent question about telling " +
		"a colleague what their own week looked like, and routing it through comms_outbound would file " +
		"an internal report on a customer's timeline",

	"internal/compose/briefmailjobs.go": "the operator's own daily brief, addressed to a seat, for the " +
		"reason the weekly digest carries: internal reporting is not correspondence with a subject",

	"internal/modules/identity/handlers_users.go": "a set-password invite to a colleague an admin has " +
		"just given a seat: the same act as the reset below, at the other end of an account's life, and " +
		"a new member has no account through which any other route could reach them",

	"internal/modules/identity/reset.go": "a password-reset link to a seat's own address, which must " +
		"reach them when the account is unreachable by every other route — a reset that depended on " +
		"the delivery lane could be blocked by the very configuration the reset exists to repair",

	"internal/modules/dealrooms/invitemail.go": "a single-use credential to a buyer a human has just " +
		"granted access to: the link IS the act the operator performed, not a message about the " +
		"relationship, and the recipient cannot use the room without receiving it",

	"internal/compose/controllermailwiring.go": "the TRANSPORT UNDER the lane, not a way around it. " +
		"This is the adapter the dispatcher's controller seam calls to put a message on the wire, and " +
		"it runs only after comms_outbound holds the row, consent has recorded the staging decision and " +
		"the transmit gate has answered again — the same position the Gmail and Graph adapters occupy " +
		"for a rep's mail. Every other entry here is a caller that reaches the relay INSTEAD of staging " +
		"a delivery; this one is what a staged delivery is finally handed to",
})

// directMailPurpose is the closed set of reasons a message may skip the
// delivery lane. CLOSED is the point: adding a fifth is a decision about what
// this product considers admissible to send unrecorded, and it belongs to
// whoever owns that question rather than to whoever is adding a mailer.
type directMailPurpose string

const (
	// purposeSeatCredential is a credential reaching a colleague's own address:
	// an invite, a reset. It must work when the account is unreachable by every
	// other route, which is exactly what the delivery lane cannot promise.
	purposeSeatCredential directMailPurpose = "seat_credential"
	// purposeOperatorDigest is internal reporting addressed to a seat in this
	// installation — a digest of their own week. There is no consent question
	// about telling a colleague what they did.
	purposeOperatorDigest directMailPurpose = "operator_digest"
	// purposeBuyerRoomCredential is a single-use link that IS the act a human
	// just performed, not a message about the relationship.
	purposeBuyerRoomCredential directMailPurpose = "buyer_room_credential"
	// purposeLaneTransport is the adapter UNDER the lane rather than a way
	// around it: it runs only after the delivery row exists and the gate has
	// answered.
	purposeLaneTransport directMailPurpose = "lane_transport"
)

// admissibleDirectMailPurposes is what the enum admits, read by the gate below
// so a purpose invented at a call site fails rather than passing unnoticed.
var admissibleDirectMailPurposes = map[directMailPurpose]bool{
	purposeSeatCredential:      true,
	purposeOperatorDigest:      true,
	purposeBuyerRoomCredential: true,
	purposeLaneTransport:       true,
}

// directMailPurposes says which category each ratified sender falls under.
//
// Kept beside the reasons rather than folded into them because gatekit's waiver
// values are prose, and a category that lives inside a paragraph cannot be
// counted. Every key here must have a reason in directMailHolders and the other
// way round; the gate holds both directions.
//
// A DATA SUBJECT IS NEVER A PURPOSE. The rule these categories serve is that a
// message to a data subject about the relationship goes through the consent
// engine — every entry here is addressed to a colleague, or is a credential the
// recipient asked for, or is the transport itself.
var directMailPurposes = map[string]directMailPurpose{
	"internal/compose/weeklymailjobs.go":          purposeOperatorDigest,
	"internal/compose/briefmailjobs.go":           purposeOperatorDigest,
	"internal/modules/identity/handlers_users.go": purposeSeatCredential,
	"internal/modules/identity/reset.go":          purposeSeatCredential,
	"internal/modules/dealrooms/invitemail.go":    purposeBuyerRoomCredential,
	"internal/compose/controllermailwiring.go":    purposeLaneTransport,
}

func TestOnlyRatifiedCodeMailsWithoutTheDeliveryLane(t *testing.T) {
	t.Parallel()

	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: sendsThroughTheRelay,
	}
	files := scope.Files(t)

	// Under-recognition is the failure this must not have. A walk that stopped
	// finding the seam — a moved package path, a renamed method — would report
	// PASS over a tree full of unrecorded sends.
	if len(files) < len(directMailHolders.Subjects()) {
		t.Fatalf("found %d files reaching the relay, want at least the %d ratified: the gate has "+
			"stopped seeing its subject", len(files), len(directMailHolders.Subjects()))
	}

	for _, f := range files {
		if directMailHolders.Waived(t, f.Path) {
			assertPurposeIsNamed(t, f.Path)
			continue
		}
		t.Errorf("%s hands a message straight to the relay: it writes no comms_outbound row, so the "+
			"send takes no authorization decision, leaves no audit entry and appears in no "+
			"subject-access export. Route it through the delivery lane, or ratify it in "+
			"directMailHolders with what the bypass costs", f.Path)
	}
	directMailHolders.AssertAllMatched(t)
	assertEveryPurposeHasAReason(t)
}

// assertPurposeIsNamed refuses a ratified sender whose category nobody chose.
func assertPurposeIsNamed(t *testing.T, path string) {
	t.Helper()
	purpose, named := directMailPurposes[path]
	if !named {
		t.Errorf("%s is ratified with a reason but names no purpose: say which of the four "+
			"admissible categories it falls under in directMailPurposes, or route it through the "+
			"delivery lane. A reason argues; the purpose is what can be counted", path)
		return
	}
	if !admissibleDirectMailPurposes[purpose] {
		t.Errorf("%s claims purpose %q, which is not one this product admits. The set is closed: "+
			"a fifth category is a decision about what may be sent unrecorded, and it is not made "+
			"by adding a mailer", path, purpose)
	}
}

// assertEveryPurposeHasAReason holds the other direction. A purpose left behind
// after its sender moved onto the delivery lane would otherwise sit here
// forever, describing a file that no longer bypasses anything.
func assertEveryPurposeHasAReason(t *testing.T) {
	t.Helper()
	reasons := map[string]bool{}
	for _, subject := range directMailHolders.Subjects() {
		reasons[subject] = true
	}
	for path := range directMailPurposes {
		if !reasons[path] {
			t.Errorf("directMailPurposes names %s, which holds no waiver in directMailHolders — "+
				"the sender was routed through the lane or moved, and its purpose outlived it", path)
		}
	}
}

// TestNoExtensionMailsWithoutTheDeliveryLane closes the census's other side.
//
// An extension is Go in this repository that composes into the same binary and
// can hold the same seam, so a census walking only internal/ would report a
// clean tree over an extension mailing directly. None does today, and that is
// precisely why this lands now: a walk widened AFTER the first extension mailer
// exists is a walk widened to ratify it.
//
// A SEPARATE TEST rather than a second root on the census above, because
// gatekit refuses a root that matches nothing — a root finding nothing
// certifies nothing, which is the right rule and the reason the census is worth
// trusting. Extensions legitimately hold no sender, so the assertion here is
// the opposite shape: not "every sender is ratified" but "there is no sender".
// The day one appears, this fails and somebody decides whether it belongs on
// the delivery lane or in the ratified population above.
func TestNoExtensionMailsWithoutTheDeliveryLane(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "extensions")
	var senders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return fmt.Errorf("parsing %s: %w", path, parseErr)
		}
		if callsSendOnAMailerField(file) {
			senders = append(senders, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the extensions: %v", err)
	}
	// The walk itself is proven, so an empty answer means "no sender" rather
	// than "found no files". Without this a renamed directory would report a
	// clean tree for the same reason a stale root does.
	if !walkedAnyGo(t, root) {
		t.Fatalf("%s holds no Go at all — the extension tree has moved and this check is "+
			"certifying nothing", root)
	}
	for _, path := range senders {
		t.Errorf("%s hands a message straight to the relay from an extension: it writes no "+
			"comms_outbound row, so the send takes no authorization decision and appears in no "+
			"subject-access export. Route it through the delivery lane, or ratify it in "+
			"directMailHolders beside the core senders and name its purpose", path)
	}
}

// walkedAnyGo reports whether the tree holds Go at all, so an empty census
// cannot be mistaken for a clean one.
func walkedAnyGo(t *testing.T, root string) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return found
}

// sendsThroughTheRelay reports whether a file calls the relay.
//
// It matches the CALL alone, deliberately, and an earlier draft of this gate
// that also required the file to name mailer.Mailer is why the rule is written
// down: identity/reset.go declares its field in handlers.go next door and
// references the package not once, so requiring both halves silently dropped a
// live sender from the corpus. A census that can fail short has already failed,
// and the arity guard below is what keeps this looser match honest.
func sendsThroughTheRelay(path string, file *ast.File) bool {
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	return callsSendOnAMailerField(file)
}

// callsSendOnAMailerField reports whether the file calls .Send( with three
// arguments after the context — the relay's shape.
//
// Matched on the ARITY and the method name rather than on the receiver's
// spelling: every holder names its field differently (confirmMailer,
// resetMailer, inviteMailer, mail.Mailer), and a gate keyed on those names
// would stop matching the moment somebody renamed a field, reporting PASS.
func callsSendOnAMailerField(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Send" || len(call.Args) != 4 {
			return true
		}
		found = true
		return false
	})
	return found
}

// TestTheRelaySeamStillHasTheShapeThisGateMatches holds the assumption the
// census rests on.
//
// The walk finds a send by its method name and its four arguments. If the seam
// grew a parameter — an html body, a message id — every call site would stop
// matching at once and the census would report PASS over a tree it could no
// longer see. This reads the interface and fails instead.
func TestTheRelaySeamStillHasTheShapeThisGateMatches(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	path := filepath.Join("internal", "platform", "mailer", "mailer.go")
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing the relay seam: %v", err)
	}

	params, ok := relaySendParams(file)
	if !ok {
		t.Fatal("no Send method found on the Mailer interface: the seam this gate matches on has moved")
	}
	if want := 4; params != want {
		t.Errorf("Mailer.Send takes %d parameters, want %d: callsSendOnAMailerField matches on that "+
			"arity, so every call site has silently stopped being seen — update both together",
			params, want)
	}
}

// relaySendParams counts the parameters of Mailer.Send as declared.
func relaySendParams(file *ast.File) (count int, found bool) {
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || spec.Name.Name != "Mailer" {
			return true
		}
		iface, ok := spec.Type.(*ast.InterfaceType)
		if !ok {
			return true
		}
		for _, m := range iface.Methods.List {
			fn, ok := m.Type.(*ast.FuncType)
			if !ok || len(m.Names) != 1 || m.Names[0].Name != "Send" {
				continue
			}
			for _, p := range fn.Params.List {
				// One field may declare several names (to, subject, body string).
				count += max(len(p.Names), 1)
			}
			found = true
		}
		return false
	})
	return count, found
}
