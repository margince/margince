// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Address use is a conservative signal of invitation delivery. Decoder reads
// and display-only intended-attendee projections are checked separately.

import (
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
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

// The decoder anchors the field census; a rename must fail rather than shorten it.
const attendeeDecoder = "internal/modules/activities/booking.go:method Handlers.BookMeeting"

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
		t.Errorf("%v use attendee addresses beyond an intended-attendee display, but crm.yaml still "+
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
	var found []string
	decoderMatches := 0
	decoderNamesField := false
	err := filepath.Walk("internal", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return err
		}
		file, parseErr := gatekit.ParseFile(path, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			name := funcLabel(fn)
			if filepath.ToSlash(path)+":"+name == attendeeDecoder {
				decoderMatches++
				decoderNamesField = namesAttendees(fn.Body)
				continue
			}
			if !attendeeDeliveryMention(fn) {
				continue
			}
			found = append(found, filepath.ToSlash(path)+":"+name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking for readers of the attendee addresses: %v", err)
	}
	if decoderMatches != 1 || !decoderNamesField {
		t.Fatal("booking decoder no longer names AttendeeEmails; re-derive the gate anchor")
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
	before, _, ok := strings.Cut(rest, "x-mcp-tool:")
	if !ok {
		t.Fatal("bookMeeting declares no x-mcp-tool, which is where its prose ends — " +
			"re-derive this gate's bounds from the operation's current shape")
	}
	return before
}
