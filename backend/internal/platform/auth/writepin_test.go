// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth_test

// The precedence, and the one rule that overrides it.
//
// This is the PRODUCTION resolver, not a model of it: both doors call exactly
// this function, which is the point of it existing. The disagreement cases it
// decides are unreachable through the real stager — a pin is taken server-side
// only for a concrete, version-checkable target, versions are monotonic, and
// the gate's read precedes the redemption's — so they are stated here rather
// than driven through an engine that cannot produce them. A fixture that
// supplied its own redemption would prove something about the fixture.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func pin(v int64) *int64 { return &v }

func TestTheWritePinPrecedenceIsCallerThenReleasedThenAdmitted(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		caller     *int64
		released   *int64
		admitted   int64
		gateRead   bool
		wantSource auth.PinSource
		wantPin    int64
	}{
		{"the caller's own pin wins", pin(7), pin(9), 7, true, auth.PinCaller, 7},
		{"the released pin comes next", nil, pin(9), 9, true, auth.PinReleased, 9},
		{"the admitted pin is the last resort", nil, nil, 9, true, auth.PinAdmitted, 9},
		{"nothing read a version, nothing pins the write", nil, nil, 0, false, auth.PinNone, 0},
		{"a caller pin with no gate read is taken as given", pin(7), nil, 0, false, auth.PinCaller, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := auth.ResolveWritePin(auth.WritePinInputs{
				CallerPin: tc.caller, ReleasedPin: tc.released,
				Admitted: tc.admitted, GateRead: tc.gateRead,
			})
			if err != nil {
				t.Fatalf("resolving: %v", err)
			}
			if got.Source != tc.wantSource {
				t.Errorf("source = %v, want %v", got.Source, tc.wantSource)
			}
			if got.Source != auth.PinNone && got.Version != tc.wantPin {
				t.Errorf("version = %d, want %d", got.Version, tc.wantPin)
			}
		})
	}
}

// A caller naming a version the gate never saw is refused — while refusing is
// still free.
//
// The caller controls that argument, so a version nothing proved would walk
// through the store's compare on precisely the record the tier decision does
// not describe: a caller naming the version a racing close will PRODUCE.
func TestACallerPinTheGateDidNotReadIsSkew(t *testing.T) {
	t.Parallel()
	_, err := auth.ResolveWritePin(auth.WritePinInputs{CallerPin: pin(7), Admitted: 9, GateRead: true})
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("a caller pin of 7 against an admitted 9 answered %v, want version skew", err)
	}
}

// AND NOT ONCE AN APPROVAL HAS BEEN SPENT ON IT, whichever pin disagrees.
//
// Both doors commit the redemption before this runs, so a refusal here destroys
// a human's single-use yes on a call that never ran and can never be redeemed
// again — the agent is told to re-read and retry with nothing left to retry
// with. The skew is not swallowed: the pin is forwarded, the store re-checks it
// inside the transaction that mutates and refuses there without consuming
// anything, and the disagreement is reported.
func TestNoDisagreementRefusesOnceTheApprovalIsSpent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		caller   *int64
		released *int64
		wantPin  int64
	}{
		{"the caller's pin disagrees", pin(7), nil, 7},
		{"the released pin disagrees", nil, pin(7), 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := auth.ResolveWritePin(auth.WritePinInputs{
				CallerPin: tc.caller, ReleasedPin: tc.released,
				Admitted: 9, GateRead: true, ApprovalSpent: true,
			})
			if err != nil {
				t.Fatalf("a disagreement refused a call whose approval was already spent: %v", err)
			}
			if got.Version != tc.wantPin {
				t.Errorf("version = %d, want %d — the pin is forwarded for the store to adjudicate",
					got.Version, tc.wantPin)
			}
			if !got.DisagreedWithAdmitted {
				t.Error("the disagreement was not reported, so a case the system believes cannot " +
					"happen and no longer refuses has nothing to say it happened")
			}
		})
	}
}

// A pin that AGREES with the gate's read reports nothing. The witness exists for
// the impossible case, and a warning on every ordinary write would bury it.
func TestAnAgreeingPinReportsNothing(t *testing.T) {
	t.Parallel()
	for _, released := range []*int64{pin(9), nil} {
		got, err := auth.ResolveWritePin(auth.WritePinInputs{ReleasedPin: released, Admitted: 9, GateRead: true, ApprovalSpent: true})
		if err != nil {
			t.Fatalf("resolving: %v", err)
		}
		if got.DisagreedWithAdmitted {
			t.Errorf("a pin of 9 against an admitted 9 was reported as a disagreement (released=%v)", released)
		}
	}
}
