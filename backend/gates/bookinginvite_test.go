// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// A booking's attendee addresses reach the calendar invitation, and the
// contract says the booking queues one. Both halves fail together: a booking
// that drops the addresses again, or prose that stops matching the delivery.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// attendeeField is the parsed request value.
const attendeeField = "AttendeeEmails"

// queuedStatement is the contract's sentence about what a booking does.
const queuedStatement = "queues a calendar invitation"

// syncDeliveryClaims promise delivery at the moment of the 201, which the
// asynchronous engine cannot: the provider call happens in the worker.
var syncDeliveryClaims = []string{
	"No invite is sent", "sends an invite", "sends the invite", "provider sends the invite",
	"notifies the attendee", "emails the attendee",
}

// bookingFlow lists the steps the attendee addresses take from the request to
// the provider event. Each function must name AttendeeEmails, and the steps
// that hand the addresses on must call the next one.
var bookingFlow = []struct {
	file, fn string
	calls    string
}{
	{"internal/modules/activities/booking.go", "Handlers.BookMeeting", "BookMeeting"},
	{"internal/modules/activities/booking_invitee.go", "Store.bookingInvitation", ""},
	{"internal/modules/activities/scheduling_roster.go", "furtherAttendees", ""},
}

// bookingHandoff is where the mapped request enters the engine.
var bookingHandoff = []struct{ file, fn, calls string }{
	{"internal/modules/activities/booking.go", "Store.BookMeeting", "bookingInvitation"},
	{"internal/modules/activities/booking.go", "Store.BookMeeting", "reserveAndQueueInvitation"},
	{"internal/modules/activities/scheduling_invitation.go", "Store.reserveAndQueueInvitation", "resolveRoster"},
	{"internal/modules/activities/scheduling_roster.go", "Store.resolveRoster", "furtherAttendees"},
}

func TestABookingsAttendeesReachTheCalendarInvitation(t *testing.T) {
	t.Parallel()
	for _, step := range bookingFlow {
		fn := gateFunc(t, step.file, step.fn)
		if !namesAttendees(fn.Body) {
			t.Errorf("%s:%s no longer touches %s; the booking's attendees stop there and never reach the calendar event", step.file, step.fn, attendeeField)
		}
		if step.calls != "" && !bodyCalls(fn.Body, step.calls) {
			t.Errorf("%s:%s no longer calls %s", step.file, step.fn, step.calls)
		}
	}
	if fn := gateFunc(t, "internal/modules/activities/booking_invitee.go", "Store.bookingInvitation"); !assignsAttendees(fn.Body) {
		t.Errorf("Store.bookingInvitation no longer sets the invitation request's %s; a booking's further attendees are dropped", attendeeField)
	}
	for _, step := range bookingHandoff {
		if !bodyCalls(gateFunc(t, step.file, step.fn).Body, step.calls) {
			t.Errorf("%s:%s no longer calls %s, so a booking does not reach the invitation engine", step.file, step.fn, step.calls)
		}
	}
}

func TestTheBookingContractSaysItQueuesAnInvitation(t *testing.T) {
	t.Parallel()
	contract, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	described := bookMeetingDescription(t, string(contract))
	if !strings.Contains(described, queuedStatement) {
		t.Errorf("crm.yaml's bookMeeting no longer says it %q; the tool description an agent reads must say what the booking does", queuedStatement)
	}
	for _, claim := range syncDeliveryClaims {
		if strings.Contains(described, claim) {
			t.Errorf("crm.yaml's bookMeeting says %q, but delivery happens later in the worker; say the invitation is queued", claim)
		}
	}
}

// The planted mutation: a bookingInvitation that reads the addresses but never
// hands them to the request is what the assignment check exists to catch.
func TestTheBookingGateFailsWhenTheMappingIsDropped(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		body string
		want bool
	}{
		"mapped":  {`addresses := normalizedAttendees(in.AttendeeEmails); req.AttendeeEmails = &addresses`, true},
		"dropped": {`addresses := normalizedAttendees(in.AttendeeEmails); _ = addresses`, false},
	} {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture; func check() {"+tc.body+"}", 0)
			if err != nil {
				t.Fatal(err)
			}
			fn, ok := file.Decls[0].(*ast.FuncDecl)
			if !ok {
				t.Fatal("fixture has no function")
			}
			if got := assignsAttendees(fn.Body); got != tc.want {
				t.Fatalf("assignsAttendees=%t want %t", got, tc.want)
			}
		})
	}
}

// gateFunc finds one named function; a rename fails rather than shortening the census.
func gateFunc(t *testing.T, path, label string) *ast.FuncDecl {
	t.Helper()
	file, err := gatekit.ParseFile(path, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Body != nil && bookingFuncLabel(fn) == label {
			return fn
		}
	}
	t.Fatalf("%s has no %s; re-derive the booking flow this gate follows", path, label)
	return nil
}

func bookingFuncLabel(fn *ast.FuncDecl) string {
	if owner := receiverName(fn); owner != "" {
		return owner + "." + fn.Name.Name
	}
	return fn.Name.Name
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

func assignsAttendees(body *ast.BlockStmt) bool {
	assigned := false
	ast.Inspect(body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			for _, lhs := range assign.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == attendeeField {
					assigned = true
				}
			}
		}
		return !assigned
	})
	return assigned
}

func bodyCalls(body *ast.BlockStmt, name string) bool {
	called := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				called = called || fun.Name == name
			case *ast.SelectorExpr:
				called = called || fun.Sel.Name == name
			}
		}
		return !called
	})
	return called
}

// bookMeetingDescription cuts the one operation's prose out of the contract:
// these phrases are ordinary English, and another operation may say them.
func bookMeetingDescription(t *testing.T, contract string) string {
	const op = "operationId: bookMeeting"
	start := strings.Index(contract, op)
	if start < 0 {
		t.Fatalf("no %q in the contract; this gate would pass having read nothing", op)
	}
	before, _, ok := strings.Cut(contract[start:], "\n      parameters:")
	if !ok {
		t.Fatal("bookMeeting declares no parameters, which is where its prose ends; re-derive this gate's bounds")
	}
	return before
}
