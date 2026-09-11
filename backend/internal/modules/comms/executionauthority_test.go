// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// A worker that does not understand why a message may go must not send it.
//
// Deployments roll forward one process at a time, so an older worker can pick
// up a delivery written by a newer API. If a later change adds an execution
// authority this build has never heard of, the safe reading is not "probably
// fine" — it is a message whose permission this process cannot evaluate.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestAnAuthorityThisBuildDoesNotUnderstandIsNotWaivedThrough(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		authority string
		known     bool
	}{
		{
			// Every delivery written before the column existed, and every
			// ordinary send since. The engine allowed it; that is the whole
			// authority and this build understands it.
			name: "an empty value is an ordinary send", authority: "", known: true,
		},
		{name: "the engine's own permission", authority: authoritySupported, known: true},
		{name: "a named human's decision", authority: authorityInstruction, known: true},
		{
			// The case this exists for: a value from a build that has not been
			// written yet.
			name: "anything else", authority: "some_future_authority",
		},
		{
			// Near-misses are not matches. A typo in a later migration must
			// park rather than read as the thing it resembles.
			name: "a near miss on a known value", authority: "instructions",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// THE REAL GATE, not a restatement of its condition. A test that
			// re-derived the answer would agree with itself after somebody
			// changed the gate, which is the only moment it has to disagree.
			//
			// A nil dispatcher is safe here precisely because a recognised
			// authority must not touch it: the known cases return before any
			// parking, and an unknown one reaching for a nil receiver is this
			// test failing loudly rather than passing quietly.
			var d *Dispatcher
			outcome, _, _ := gateKnownAuthority(d, Delivery{ExecutionAuthority: tc.authority})
			known := outcome == outcomeUndecided
			if known != tc.known {
				if tc.known {
					t.Errorf("%q was not recognised, so an ordinary delivery would park",
						tc.authority)
					return
				}
				t.Errorf("%q was read as an authority this build understands — a message whose "+
					"permission this process cannot evaluate would be sent", tc.authority)
			}
		})
	}
}

// gateKnownAuthority calls the gate and reports only whether it let the
// delivery through, so an unknown value's park attempt cannot reach a nil
// dispatcher.
//
// It exists because the park side of the gate writes to the database and this
// test is about the RECOGNITION, which is the half that decides whether a
// message this process cannot account for goes out.
func gateKnownAuthority(d *Dispatcher, del Delivery) (o Outcome, wait int, err error) {
	defer func() {
		if recover() != nil {
			// Reached the park path, which is the refusal this test wants to
			// observe for an unrecognised value.
			o = Outcome("parked")
		}
	}()
	outcome, _, _ := d.gateExecutionAuthority(nil, del) //nolint:staticcheck // nil ctx: the recognised path never uses it, and the park path is caught above
	return outcome, 0, nil
}

// AN INSTRUCTION WAIVES THE CONSENT REFUSAL AND NO OTHER.
//
// The human was shown the engine's answer about the RECIPIENTS and signed for
// that. They were not shown, and cannot have signed for, a message that was
// edited after it was checked — that refusal is about the MESSAGE, and nobody
// has looked at it.
//
// The first spelling of this branch asked only whether the delivery carried an
// instruction, which waived every refusal a transmit can make. A changed-wording
// rejection then sent anyway, under the name of somebody who had approved
// different words.
func TestAnInstructionWaivesTheConsentRefusalAndNoOther(t *testing.T) {
	t.Parallel()
	directed := Delivery{
		ExecutionAuthority: authorityInstruction,
		InstructionID:      ids.NewV7(),
	}
	for _, tc := range []struct {
		name      string
		ticket    commsauthz.TransmitTicket
		del       Delivery
		wantSends bool
	}{
		{
			name:      "the refusal they signed for",
			ticket:    commsauthz.TransmitTicket{Allowed: false, ConsentRefused: true},
			del:       directed,
			wantSends: true,
		},
		{
			// The case the first spelling got wrong.
			name:   "a refusal about the message itself",
			ticket: commsauthz.TransmitTicket{Allowed: false, ConsentRefused: false},
			del:    directed,
		},
		{
			// Without a decision, a consent refusal is simply a refusal.
			name:   "a consent refusal with nobody's decision behind it",
			ticket: commsauthz.TransmitTicket{Allowed: false, ConsentRefused: true},
			del:    Delivery{ExecutionAuthority: authoritySupported},
		},
		{
			// A row claiming the authority and naming no decision is a
			// half-written record, not permission.
			name:   "an authority naming no decision",
			ticket: commsauthz.TransmitTicket{Allowed: false, ConsentRefused: true},
			del:    Delivery{ExecutionAuthority: authorityInstruction},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// THE REAL BRANCH, not a restatement of its condition: a test that
			// re-derived the answer would agree with itself after somebody
			// widened the waiver, which is the only moment it has to disagree.
			sends := waivesRefusal(tc.ticket, tc.del)
			if sends != tc.wantSends {
				if tc.wantSends {
					t.Error("a message somebody decided to send was parked anyway")
					return
				}
				t.Error("a refusal nobody signed for was waived because the delivery carried " +
					"an instruction — the wrong message would go out under their name")
			}
		})
	}
}

// waivesRefusal reports whether gateConsent lets a refused delivery through on
// a recorded decision.
//
// It calls the gate with a nil dispatcher: the waiving path returns before
// anything is written, and the parking path reaches for the dispatcher — so a
// panic IS the park, and catching it is how this test observes the refusal
// without a database.
func waivesRefusal(ticket commsauthz.TransmitTicket, del Delivery) (waived bool) {
	defer func() {
		if recover() != nil {
			waived = false
		}
	}()
	var d *Dispatcher
	_, outcome, _, _ := d.gateConsentDecision(context.Background(), ticket, del)
	return outcome == outcomeUndecided
}

// A WORDING REFUSAL IS NOBODY'S TO WAIVE, even on a directed send.
//
// The engine refuses a directed message about its RECIPIENTS — that is the
// refusal the human read and signed for, and the delivery's instruction answers
// it. A message edited after it was checked is refused for a different reason,
// one nobody has looked at, and the ticket must not carry the flag that waives
// the first into the second.
//
// This is the shape of the defect the first fix left behind: the wording check
// ran only for a message the engine ALLOWED, and a directed send is never
// allowed — so the check was skipped entirely and the waiver then sent whatever
// the payload held.
func TestAWordingRefusalIsNotWaivedByAnInstruction(t *testing.T) {
	t.Parallel()
	directed := Delivery{
		ExecutionAuthority: authorityInstruction,
		InstructionID:      ids.NewV7(),
	}
	// What AuthorizeTransmit produces when the recipients are refused AND the
	// message has been edited since: the wording refusal clears the flag,
	// because it is not the refusal anybody signed for.
	edited := commsauthz.TransmitTicket{Allowed: false, ConsentRefused: false}
	if waivesRefusal(edited, directed) {
		t.Error("an edited message was sent on a decision about a different message — the " +
			"wording check is the one thing a recorded override must not waive")
	}
}
