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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bookingClaim is the sentence that says an invite goes out. Matched on the
// verb rather than the whole phrase, so a reworded promise is still caught.
const bookingClaim = "sends an invite"

// attendeeField is the parsed request value. A file that names it and is not
// the decoder is a file doing something with the addresses.
const attendeeField = "AttendeeEmails"

// attendeeDecoder is where the addresses are parsed and held. Reading them here
// is not a transport; it is the four lines the defect was about.
const attendeeDecoder = "internal/modules/activities/scheduling.go"

func TestTheBookingContractDoesNotPromiseAnInviteItCannotSend(t *testing.T) {
	t.Parallel()

	contract, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	claimsDelivery := strings.Contains(bookMeetingDescription(t, string(contract)), bookingClaim)
	consumers := attendeeConsumers(t)

	switch {
	case claimsDelivery && len(consumers) == 0:
		t.Errorf("crm.yaml says %q and nothing outside %s reads the attendee addresses — "+
			"a booking accepts them, holds them for four lines and drops them, so the "+
			"contract is describing a feature this build does not have. It is also the "+
			"tool description an agent reads before telling somebody the invite went out.",
			bookingClaim, attendeeDecoder)
	case !claimsDelivery && len(consumers) > 0:
		t.Errorf("%v now read the attendee addresses, so something is being done with "+
			"them, but crm.yaml no longer says a booking sends an invite — it was removed "+
			"because it was false. Say what the operation does now: an agent reading the "+
			"old wording will tell a user no invite was sent.", consumers)
	}
}

// attendeeConsumers answers the non-test files that do something with the
// booking's attendee addresses, other than parsing them.
func attendeeConsumers(t *testing.T) []string {
	t.Helper()
	var found []string
	err := filepath.Walk("internal", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return err
		}
		if filepath.ToSlash(path) == attendeeDecoder {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(src), attendeeField) {
			found = append(found, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking for readers of the attendee addresses: %v", err)
	}
	return found
}

// bookMeetingDescription cuts the one operation's prose out of the contract.
//
// Scoped rather than searched whole, because the phrase is ordinary English and
// the file is thirty thousand lines: an unrelated operation that legitimately
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
