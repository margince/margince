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
// KNOWN GAP, ratified below rather than hidden: consent's confirm-details link
// is mailed directly. The eight-PR consent plan's PR 3 was to move it onto the
// durable lane and never landed, so the message that asks somebody to check
// what is held about them is itself unrecorded. It is waived because refusing
// it would delete a working feature, and the waiver names the cost so the debt
// is legible instead of forgotten.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// mailerPackage is the seam this gate polices.
const mailerPackage = "github.com/margince/margince/backend/internal/platform/mailer"

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

	"internal/modules/consent/confirmmail.go": "KNOWN DEBT, not a settled exemption. The confirm-details " +
		"link asks somebody to check what is held about them and is mailed straight to SMTP, so the one " +
		"message most owed a record has none: it writes no comms_outbound row, takes no authorization " +
		"decision and appears in no subject-access export. The consent plan's controller-mail lane was " +
		"to fix this and never landed. Waived because refusing it would delete a working feature",
})

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
			continue
		}
		t.Errorf("%s hands a message straight to the relay: it writes no comms_outbound row, so the "+
			"send takes no authorization decision, leaves no audit entry and appears in no "+
			"subject-access export. Route it through the delivery lane, or ratify it in "+
			"directMailHolders with what the bypass costs", f.Path)
	}
	directMailHolders.AssertAllMatched(t)
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
