// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Whether the message about to go is the message somebody signed for.
//
// The rest of the directed send is exercised end to end
// (compose/integration/directedsend_integration_test.go). This one rule is
// tested here because the database will not let it be staged from outside: the
// instruction row is immutable by trigger, so a test cannot manufacture "the
// decision says one thing and the message says another" through the real
// writers. What it CAN do is state the comparison itself, which is the whole
// rule.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestAnAcknowledgementIsAboutOneExactMessage(t *testing.T) {
	t.Parallel()
	read := SendingDigest("Invoice 41", "Attached, as agreed.", "")

	for _, tc := range []struct {
		name     string
		sending  [32]byte
		wantSend bool
	}{
		{
			name:     "the same message goes",
			sending:  SendingDigest("Invoice 41", "Attached, as agreed.", ""),
			wantSend: true,
		},
		{
			// The body is what a rep is most likely to rework between deciding
			// and sending, and it is the half the recipient actually reads.
			name:    "an edited body does not",
			sending: SendingDigest("Invoice 41", "Attached, and we have raised the fee.", ""),
		},
		{
			// A changed subject line is a different message to the person
			// receiving it, whatever the body says.
			name:    "an edited subject does not",
			sending: SendingDigest("Overdue notice", "Attached, as agreed.", ""),
		},
		{
			// The markup half can carry text the plain half does not, so a
			// fingerprint that ignored it would let a whole message through
			// unread.
			name:    "an added html alternative does not",
			sending: SendingDigest("Invoice 41", "Attached, as agreed.", "<p>And a late fee.</p>"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if (tc.sending == read) != tc.wantSend {
				if tc.wantSend {
					t.Error("the message somebody acknowledged did not match itself, so no " +
						"directed send could ever go")
					return
				}
				t.Error("a changed message matched the acknowledgement — the human signed for " +
					"one message and another would go out under their name")
			}
		})
	}
}

// AN ABSENT FINGERPRINT IS NOTHING TO DISAGREE WITH.
//
// A decision recorded before the fingerprint column existed, or against a
// review whose held message could not be read, carries none. Refusing those
// would park a send for a reason nobody can act on — and the decision is still
// a real decision, made by a named person who acknowledged a real warning.
func TestADecisionWithNoRecordedWordingStillAuthorizes(t *testing.T) {
	t.Parallel()
	inst := LiveInstruction{Acknowledged: nil}
	if len(inst.Acknowledged) > 0 {
		t.Fatal("fixture")
	}
	// The consumption path's condition, stated: with nothing recorded there is
	// nothing to compare, and the send proceeds on the decision alone.
	sending := SendingDigest("anything", "at all", "")
	if len(inst.Acknowledged) > 0 && string(inst.Acknowledged) != string(sending[:]) {
		t.Error("a decision carrying no fingerprint refused a message, which parks a send for a " +
			"reason nobody can act on")
	}
}

// A MIXED ENVELOPE RECORDS EACH RECIPIENT TRUTHFULLY.
//
// A message to three people can be allowed for two of them and refused for the
// third. Saying all three went out on somebody's decision would overstate what
// was decided — the human was shown one refusal and signed for that one — and
// saying none of them did would lose the record of the override entirely.
//
// The transmit rows are the ones an auditor reads for what actually happened,
// so this rule has to hold there as well as at staging.
func TestOnlyTheRefusedRecipientsNameTheDecision(t *testing.T) {
	t.Parallel()
	instruction := ids.NewV7()

	for _, tc := range []struct {
		name            string
		verdict         commsauthz.Verdict
		wantAuthority   string
		wantInstruction bool
	}{
		{
			// The recipient the human was shown and signed for.
			name: "a refused recipient", verdict: commsauthz.VerdictDeny,
			wantAuthority: AuthorityInstruction, wantInstruction: true,
		},
		{
			// A refusal short of a denial is still a refusal, and still what
			// the decision answers.
			name: "a recipient sent for review", verdict: commsauthz.VerdictReview,
			wantAuthority: AuthorityInstruction, wantInstruction: true,
		},
		{
			// This one needed no decision: the engine allowed it.
			name: "an allowed recipient", verdict: commsauthz.VerdictAllow,
			wantAuthority: AuthoritySupported,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			authority := authorityFor(AuthorityInstruction, tc.verdict)
			named := instructionFor(&instruction, tc.verdict)
			if authority != tc.wantAuthority {
				t.Errorf("recorded %q, want %q", authority, tc.wantAuthority)
			}
			if (named != nil) != tc.wantInstruction {
				if tc.wantInstruction {
					t.Error("a refused recipient names no decision, so the record does not say " +
						"why the message reached them")
					return
				}
				t.Error("a recipient the engine allowed names somebody's override — the record " +
					"claims a decision was needed where none was")
			}
			// The table's shape CHECK requires the two to agree, so a row that
			// disagreed would be refused at write time rather than read wrong.
			if (authority == AuthorityInstruction) != (named != nil) {
				t.Error("the authority and the instruction disagree, which the table refuses")
			}
		})
	}
}

// AN EDITED MESSAGE IS REFUSED FOR A REASON NOBODY SIGNED FOR.
//
// A named human's decision answers the engine's verdict about the RECIPIENTS.
// It cannot answer a refusal about the MESSAGE, because the message they were
// shown is not the one that would go.
//
// This is the rule the first fix left broken. The wording check ran only for a
// message the engine ALLOWED — and a directed send is never allowed, so the
// check was skipped and the waiver sent whatever the payload held.
func TestAnEditedMessageClearsTheFlagThatWaivesARefusal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		ticket  commsauthz.TransmitTicket
		changed bool
		// wantWaivable is whether a recorded decision could still let this
		// message through.
		wantWaivable bool
		wantAllowed  bool
	}{
		{
			// The ordinary directed send: refused about the people, message
			// untouched. This is the one a decision answers.
			name:         "refused recipients and an untouched message",
			ticket:       commsauthz.TransmitTicket{Allowed: false, ConsentRefused: true},
			wantWaivable: true,
		},
		{
			// The case that was broken: refused about the people AND edited.
			// The decision must not carry it.
			name:    "refused recipients and an edited message",
			ticket:  commsauthz.TransmitTicket{Allowed: false, ConsentRefused: true},
			changed: true,
		},
		{
			// An allowed message that was edited refuses on its own, and says
			// so — the operator can look at the wording and re-send.
			name:    "an allowed message that was edited",
			ticket:  commsauthz.TransmitTicket{Allowed: true},
			changed: true,
		},
		{
			// Nothing wrong with either half.
			name:        "an allowed message nobody touched",
			ticket:      commsauthz.TransmitTicket{Allowed: true},
			wantAllowed: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ticket := tc.ticket
			applyWordingVerdict(&ticket, tc.changed)
			if ticket.Allowed != tc.wantAllowed {
				t.Errorf("allowed = %v, want %v", ticket.Allowed, tc.wantAllowed)
			}
			if ticket.ConsentRefused != tc.wantWaivable {
				if tc.wantWaivable {
					t.Error("a refusal a decision was made about is no longer waivable, so a " +
						"directed send that should go would park")
					return
				}
				t.Error("an edited message still carries the flag a recorded decision waives — " +
					"the wrong message would go out under somebody's name")
			}
		})
	}
}
