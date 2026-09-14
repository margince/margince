// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Which version an agent's write is conditioned on, decided once for both doors.
//
// Three places can establish one — the caller's own pin, the pin a redeemed
// approval was granted against, and the pin the gate read when it admitted the
// call at the auto-execute tier — and the precedence between them is the same
// rule whichever transport the call arrived on. It lived as two spellings, in
// modules/agents and in compose, and the two came to disagree about the case
// below. One spelling is the fix for that, and the reason this is in platform:
// it is the only package both doors already reach.

// PinSource names which of the three established the version, for a caller that
// has to say so in a log line or a refusal.
type PinSource int

const (
	// PinNone means nothing conditioned this write. The ordinary case for a
	// static tier on an unapproved call, and an answer rather than an absence.
	PinNone PinSource = iota
	// PinCaller means the caller supplied it. It is bound into the diff_hash a
	// redemption verified, which is what ties it to the record a human judged.
	PinCaller
	// PinReleased means the approval this call redeemed was granted against it.
	PinReleased
	// PinAdmitted means the gate read it to resolve a dynamic tier.
	PinAdmitted
)

// WritePin is the resolved precondition, and where it came from.
type WritePin struct {
	// Version is the version to condition the write on; meaningful only when
	// Source is not PinNone.
	Version int64
	Source  PinSource
	// DisagreedWithAdmitted marks a pin that does not match the version the
	// gate read — the case both doors can only discover AFTER the approval is
	// spent, whether the pin came from the redemption or from the caller.
	//
	// REPORTED, NEVER REFUSED, and the order of events is the whole reason. On
	// both doors the redemption commits before this rule runs, so by the time
	// either could notice, the human's single-use approval is already gone —
	// and a refusal there destroys it on a call that never ran and can never be
	// redeemed again. The agent is told to re-read and retry with nothing left
	// to retry with. Forwarding the released pin is no weaker: the store
	// re-checks it inside the transaction that mutates, and refuses there
	// unless the row is at the version the approval was granted against.
	//
	// It is not swallowed either. Through the real stager this is unreachable —
	// a pin is taken server-side only for a concrete, version-checkable target,
	// versions are monotonic, the gate's read precedes the redemption's so
	// admitted ≤ current = released, and a row that genuinely moved fails the
	// redemption's own target re-check first. A condition the system believes
	// cannot happen is exactly the one that needs a witness when it does, which
	// is what this field is for.
	//
	// If it ever fires, the fix is to move this comparison BEFORE the
	// redemption commits: a genuine disagreement could then refuse without
	// destroying anything. That is not done now because it reorders the
	// redemption path on both doors for a case nothing can reach.
	DisagreedWithAdmitted bool
}

// WritePinInputs is the state one write's precondition is resolved from.
//
// A struct rather than five parameters, and not only for the two booleans a
// positional call would leave a reader decoding: the three pins and the two
// conditions are ONE state, and a caller assembling it names each half at the
// site where it knows what it means.
type WritePinInputs struct {
	// CallerPin is the version the caller conditioned its own call on, nil when
	// it named none.
	CallerPin *int64
	// ReleasedPin is the version a redeemed approval was granted against, nil
	// when the call redeemed none or the approval carried no pin.
	ReleasedPin *int64
	// Admitted is the version the gate read to resolve a dynamic tier, and is
	// meaningful only when GateRead is true.
	Admitted int64
	// GateRead says whether the gate resolved this call's tier by reading a
	// record at all. False for a static tier, a tier raised to confirm-first,
	// and a human principal.
	GateRead bool
	// ApprovalSpent says whether a human's single-use approval has ALREADY been
	// consumed for this call. It is what turns a refusal into a loss.
	ApprovalSpent bool
}

// ResolveWritePin applies the precedence: the caller's pin, then the released
// one, then the admitted one.
//
// THE CALLER'S PIN WINS when it supplied one, and is CHECKED against the gate's
// read rather than trusted. The caller controls this argument, so a version the
// gate never saw is a version nothing proved — and a caller naming the version
// a racing close will PRODUCE would walk straight through the guard, because
// the store's compare then passes on precisely the record the tier decision does
// not describe. That disagreement answers skew, and a caller holding a stale
// version learns it is stale and can retry.
//
// UNLESS AN APPROVAL HAS ALREADY BEEN SPENT ON IT, which is the same rule the
// released-versus-admitted case below turns on and the reason they are decided
// together. Both doors commit the redemption BEFORE this runs, so a refusal
// here destroys a human's single-use yes on a call that never ran — and the
// agent is told to re-read and retry with nothing left to retry with. The skew
// is still not swallowed: the pin is forwarded and the store re-checks it
// inside the transaction that mutates, which refuses there without consuming
// anything, and the disagreement is reported.
//
// THE RELEASED PIN COMES NEXT. Redemption commits its own transaction and the
// handler then opens a fresh one, so the check inside the redemption proves the
// row was at the approved version when the approval was consumed — not that it
// still is when the effect lands, and the agent controls both sides of that
// window. Carrying the pin into the write moves the compare inside the
// transaction that actually mutates.
//
// THE ADMITTED PIN closes the same window where there is no approval to carry
// one. A dynamic tier is resolved by READING the record, and that read commits
// before the write just as a redemption does; without it a close landing in the
// window reopens a won deal one tier down.
func ResolveWritePin(in WritePinInputs) (WritePin, error) {
	callerPin, releasedPin, admitted := in.CallerPin, in.ReleasedPin, in.Admitted
	if callerPin != nil {
		disagrees := in.GateRead && *callerPin != admitted
		if disagrees && !in.ApprovalSpent {
			return WritePin{}, fmt.Errorf(
				"version %d is not the version this record was read at (%d) — re-read it and retry: %w",
				*callerPin, admitted, apperrors.ErrVersionSkew)
		}
		return WritePin{
			Version: *callerPin, Source: PinCaller,
			DisagreedWithAdmitted: disagrees,
		}, nil
	}
	if releasedPin != nil {
		return WritePin{
			Version:               *releasedPin,
			Source:                PinReleased,
			DisagreedWithAdmitted: in.GateRead && *releasedPin != admitted,
		}, nil
	}
	if in.GateRead {
		return WritePin{Version: admitted, Source: PinAdmitted}, nil
	}
	return WritePin{Source: PinNone}, nil
}
