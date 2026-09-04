// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// What booking a meeting CLAIMS and what it DOES, held against each other.
//
// `POST /bookings` accepted `attendee_emails`, decoded them into a struct field,
// and dropped them. The contract said it "sends an invite" and the screen said
// "the invite is on its way". Neither was true for any deployment of this build:
// the calendar integrations are capture-only and `platform/mailer` is the
// password-reset seam, so there was no transport to configure. A rep typed a
// client's address, was told the client had been told, and the client had not.
//
// Fixing the words alone is a fix that expires. Whoever wires a real transport
// later has no reason to look at the contract, and the sentence that was
// removed for being false stays removed while it is true — so `book_meeting`
// would then send an invite that its own tool description denies, and an agent
// reading that description would tell a user no invite went out.
//
// So this holds the two together and FAILS IN BOTH DIRECTIONS:
//
//   - a contract that claims delivery while nothing reads the addresses is the
//     original defect;
//   - a tree that reads the addresses while the contract denies delivery is the
//     same defect inverted, and it is the one that will actually happen next.
//
// The capability is derived from the code rather than declared: whether any
// non-test file OUTSIDE the request decoder does anything with the attendee
// addresses. That is what "there is a transport" means here, and it needs no
// list to maintain.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noSendStatement is the contract's own sentence saying no invite goes out.
//
// The CLAIM side is held by requiring this rather than by hunting for promises,
// because the promises are open-ended: "sends an invite" was the one that
// shipped, and "emails an invitation" or "notifies the attendee" would have
// passed a gate that only knew the first. A disclaimer that must be PRESENT is
// closed — there is one sentence to keep, and deleting it is what fails.
const noSendStatement = "No invite is sent"

// deliveryClaims are promises that contradict it. A short list and the weaker
// half of this gate: it catches prose that says both things at once, which the
// disclaimer requirement alone cannot. It does not pretend to be exhaustive,
// which is why it is not the thing the gate rests on.
var deliveryClaims = []string{
	"sends an invite", "sends the invite", "sends an invitation",
	"emails an invitation", "emails the attendee", "notifies the attendee",
	"invites the attendee",
}

// attendeeField is the parsed request value.
const attendeeField = "AttendeeEmails"

// attendeeDecoder is the function that legitimately names the addresses without
// doing anything with them: the HTTP handler that decodes the request.
//
// That it is the only such function is not asserted here, it is what the test
// below MEASURES — every other function naming the field is reported, so a
// second decoder appearing is a finding rather than a silent exemption.
//
// Held by: TestTheBookingContractDoesNotPromiseAnInviteItCannotSend
// (this file)
//
// A function rather than a file. `scheduling.go` holds both that decoder and
// `Store.BookMeeting`, which is the most likely place a transport would be
// wired — so excluding the file excluded the very function whose change this
// gate exists to notice.
const attendeeDecoder = "method Handlers.BookMeeting"

func TestTheBookingContractDoesNotPromiseAnInviteItCannotSend(t *testing.T) {
	t.Parallel()

	contract, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	described := bookMeetingDescription(t, string(contract))
	disclaims := strings.Contains(described, noSendStatement)
	consumers := attendeeConsumers(t)

	switch {
	case len(consumers) == 0 && !disclaims:
		t.Errorf("nothing outside %s does anything with the attendee addresses, and "+
			"crm.yaml no longer says %q. A booking accepts them, holds them for four "+
			"lines and drops them — so the contract is describing a feature this build "+
			"does not have, and it is the tool description an agent reads before telling "+
			"somebody the invite went out.", attendeeDecoder, noSendStatement)
	case len(consumers) > 0 && disclaims:
		t.Errorf("%v now do something with the attendee addresses, but crm.yaml still "+
			"says %q. Whoever wired the transport has left the operation denying it, and "+
			"an agent reading that will tell a user nothing was sent.",
			consumers, noSendStatement)
	}

	// The weaker half: prose that promises delivery alongside the disclaimer.
	if len(consumers) == 0 {
		for _, claim := range deliveryClaims {
			if strings.Contains(described, claim) {
				t.Errorf("crm.yaml says %q while nothing reads the attendee addresses — "+
					"the disclaimer and this promise cannot both be true, and a reader "+
					"believes whichever they meet first", claim)
			}
		}
	}
}

// attendeeConsumers answers the FUNCTIONS that do something with the booking's
// attendee addresses, other than the one that decodes them.
//
// Per function, not per file. The decoder and `Store.BookMeeting` live in one
// file, and a transport would most naturally be wired in the second — so a
// file-level exclusion blinded this gate to the exact change it exists to
// notice.
func attendeeConsumers(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	var found []string
	err := filepath.Walk("internal", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return err
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			name := funcLabel(fn)
			if name == attendeeDecoder || !namesAttendees(fn.Body) {
				continue
			}
			found = append(found, filepath.ToSlash(path)+":"+name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking for readers of the attendee addresses: %v", err)
	}
	return found
}

// namesAttendees reports whether a body mentions the parsed attendee field.
func namesAttendees(body *ast.BlockStmt) bool {
	mentioned := false
	ast.Inspect(body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == attendeeField {
			mentioned = true
		}
		return !mentioned
	})
	return mentioned
}

// bookMeetingDescription cuts the one operation's prose out of the contract.
//
// Scoped rather than searched whole, because these phrases are ordinary English
// and the file is thirty thousand lines: an unrelated operation that legitimately
// does send an invite would hold this gate red for ever, and — as the first
// draft proved — so would a sentence explaining that booking does not.
func bookMeetingDescription(t *testing.T, contract string) string {
	const op = "operationId: bookMeeting"
	start := strings.Index(contract, op)
	if start < 0 {
		t.Fatalf("no %q in the contract, so this gate is reading for a phrase in an "+
			"operation that has moved or been renamed, and would pass having compared "+
			"nothing", op)
	}
	rest := contract[start:]
	end := strings.Index(rest, "x-mcp-tool:")
	if end < 0 {
		t.Fatal("bookMeeting declares no x-mcp-tool, which is where its prose ends — " +
			"re-derive this gate's bounds from the operation's current shape")
	}
	return rest[:end]
}
