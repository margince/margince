// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The window the staging gate cannot see.
//
// refuseStagingElsewhere refuses a mirror-held target when the approval is
// STAGED. That is the right first answer and it cannot cover the case this
// suite is about: Handle is entered after the approval is redeemed and re-reads
// nothing, and activating the overlay flips a workspace MODE rather than
// deleting the native rows — so a proposal staged last night against a record
// this installation owned lands this morning against one it does not.
//
// The mode is moved between the two here, which is the only way to be that
// sequence rather than a description of it.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// movingSoR is the installation's system of record, as a seam a case can flip
// between the staging and the redemption.
type movingSoR struct {
	external bool
	err      error
	asked    int
}

func (m *movingSoR) answer(context.Context) (bool, error) {
	m.asked++
	return m.external, m.err
}

func TestAWriteIsRefusedWhenTheRecordStoppedBeingOursBetweenApprovalAndSend(t *testing.T) {
	t.Parallel()
	sor := &movingSoR{external: false}
	// No store, no stager, no gate: the refusal under test comes FIRST, and a
	// case that had to wire the send path to observe it would be proving
	// something about the wiring. A call that got past the guard would panic
	// here, which is the assertion working the other way.
	adapter := commsAdapter{externalSoR: sor.answer}

	// The staging moment: the record is ours, and nothing refuses.
	if err := adapter.refuseIfHeldElsewhere(context.Background(), "send mail"); err != nil {
		t.Fatalf("a native record was refused while the installation still held it: %v", err)
	}

	// An administrator activates the overlay while the approval waits.
	sor.external = true

	for _, tc := range []struct {
		verb string
		call func() error
	}{
		{"send_email", func() error {
			_, err := adapter.SendEmail(context.Background(), ids.NewV7(), agents.SendEmailArgs{})
			return err
		}},
		{"send_account_email", func() error {
			_, err := adapter.SendAccountEmail(context.Background(), nil, agents.SendEmailArgs{})
			return err
		}},
		{"send_message", func() error {
			_, err := adapter.SendMessage(context.Background(), ids.NewV7(), agents.SendMessageArgs{})
			return err
		}},
		{"book_meeting", func() error {
			_, err := adapter.BookMeeting(context.Background(), agents.BookMeetingArgs{})
			return err
		}},
	} {
		t.Run(tc.verb, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
				t.Fatalf("%s ran against a record the installation no longer holds, answering %v — the "+
					"staging gate refused this at approval time and the mode moved afterwards", tc.verb, err)
			}
		})
	}
}

// A SURFACE THAT CANNOT ASK REFUSES, rather than passing.
//
// The optional collaborators beside it read the other way — a nil timer means
// "this surface cannot defer", a nil calendar means "this deployment reads no
// diary" — and both are honest absences a caller can act on. "May I still write
// this record" has no such reading: a guard that passes when it cannot ask is
// the defect it was added for.
func TestASurfaceWithNoSystemOfRecordSeamRefusesRatherThanSends(t *testing.T) {
	t.Parallel()
	if err := (commsAdapter{}).refuseIfHeldElsewhere(context.Background(), "send mail"); !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
		t.Fatalf("an adapter with no system-of-record seam answered %v, want a refusal — a guard that "+
			"passes when it cannot ask is not a guard", err)
	}
}

// A seam that ERRORS is not a licence either. The mode read can fail — it is a
// query — and "we could not find out" is the same answer as "we cannot ask".
func TestAnUnreadableSystemOfRecordRefuses(t *testing.T) {
	t.Parallel()
	sor := &movingSoR{err: errors.New("the mode row is unreachable")}
	err := (commsAdapter{externalSoR: sor.answer}).refuseIfHeldElsewhere(context.Background(), "send mail")
	if err == nil {
		t.Fatal("a send ran while the installation's system of record could not be read")
	}
}

// And the ordinary case still goes through, asked once.
func TestANativeWorkspaceIsAskedOnceAndAdmitted(t *testing.T) {
	t.Parallel()
	sor := &movingSoR{external: false}
	if err := (commsAdapter{externalSoR: sor.answer}).refuseIfHeldElsewhere(context.Background(), "send mail"); err != nil {
		t.Fatalf("a native workspace was refused: %v", err)
	}
	if sor.asked != 1 {
		t.Errorf("the system of record was read %d times for one write, want 1 — the guard is on the "+
			"approved path and a read per link is what this design exists to avoid", sor.asked)
	}
}

// nativeSoR is the answer for a suite whose installation never leaves its own
// system of record, which is every suite but the one above.
//
// Named rather than written as a literal at each call: a case that wired this
// to answer TRUE by accident would have its sends refused for a reason nowhere
// near what it was asserting, and a reader would go looking in the send path.
func nativeSoR(context.Context) (bool, error) { return false, nil }
