// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// identityOf names the external identity a create states, if it states one.
//
// Email states its Message-ID; a meeting states its calendar occurrence. Both
// are optional — most activities are neither imported nor synced — and an
// activity that states nothing simply has no identity to resolve.
func identityOf(in LogActivityInput) (kind, key string) {
	switch {
	case in.RFCMessageID != "":
		return IdentityKindMail, in.RFCMessageID
	case in.ICalUID != "" && in.ICalInstance != "":
		return IdentityKindMeeting, MeetingIdentityKey(in.ICalUID, in.ICalInstance)
	default:
		return "", ""
	}
}

// boundToKnownMessage answers the activity this message already is, when the
// other door filed it first.
//
// Three refusals, and each one falls through to an ordinary create rather than
// failing the write. A duplicate row is a visible annoyance; the alternatives
// here are worse:
//
//   - The incumbent is out of this caller's scope → they must not learn it
//     exists, so nothing is disclosed and their own row is created.
//   - A different seat wrote the incumbent, or it is no longer live → both are
//     ResolveBindableIdentity's refusals, and both answer not-found. That
//     function is where the rule about who may bind is stated, for this door and
//     the capture door alike, so neither can drift from the other.
//   - The kinds disagree → a Message-ID on a note and the same one on an email
//     are not one message.
func boundToKnownMessage(ctx context.Context, tx pgx.Tx, in LogActivityInput) (crmcontracts.Activity, bool, error) {
	kind, key := identityOf(in)
	if key == "" {
		return crmcontracts.Activity{}, false, nil
	}
	incumbent, found, err := ResolveBindableIdentity(ctx, tx, kind, key)
	if err != nil || !found {
		return crmcontracts.Activity{}, false, err
	}
	same, err := sameKindAs(ctx, tx, incumbent, in.Kind)
	if err != nil || !same {
		return crmcontracts.Activity{}, false, err
	}
	// LIVE rows only. An archived one is a message that was erased, redacted as
	// noise, or folded into another row, and handing it back would serve a
	// tombstone as though it were the message — and, for an erasure, disclose
	// that the message was erased. The arrival files its own copy instead.
	out, err := readActivity(ctx, tx, incumbent, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		// Out of scope, or no longer live. The message stays hidden and this
		// caller files their own copy, which is the answer capture reaches for
		// an incumbent it may not see.
		return crmcontracts.Activity{}, false, nil
	}
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	return out, true, nil
}

// recognizedMessage answers the activity this message already is, when the
// other door filed it first.
//
// It is the same question the source-key replay asks, over a wider net: that
// one recognises this caller's OWN key, this one recognises the message itself
// however the other door filed it. A recognised meeting takes its new status
// the way a replayed one does — a cancellation arriving through the second door
// is still a cancellation, and dropping it would leave a meeting reading booked
// after it was called off.
func recognizedMessage(ctx context.Context, tx pgx.Tx, in LogActivityInput) (crmcontracts.Activity, bool, error) {
	bound, found, err := boundToKnownMessage(ctx, tx, in)
	if err != nil || !found {
		return crmcontracts.Activity{}, false, err
	}
	moved, err := replayMovedTheMeeting(ctx, tx, bound, in)
	return moved, true, err
}

// recordImportedProvenance writes what an importer stated about a message it
// handed over: who was on it, and which message it is.
//
// The address rows sit beside the contact-link rows the caller already stamped.
// Both belong: one names who this workspace knows, the other names everyone the
// message was actually addressed to.
//
// The identity claim is in THIS transaction on purpose. Two arrivals racing on
// one Message-ID both reach here, the primary key lets one through, and the
// loser is told rather than left to file a second copy of the same message.
func recordImportedProvenance(ctx context.Context, tx pgx.Tx, id ids.ActivityID, in LogActivityInput, by string) error {
	if err := stampSuppliedEmailParticipants(ctx, tx, id, in); err != nil {
		return err
	}
	kind, key := identityOf(in)
	if key == "" {
		return nil
	}
	// A lost claim is not a failure: somebody else's row holds the identity and
	// this one still stands on its own key. Whether we won is deliberately not
	// reported to the caller — see ClaimIdentity.
	_, err := ClaimIdentity(ctx, tx, id, kind, key, by)
	return err
}

// sameKindAs reports whether the incumbent is the same sort of thing as the
// arrival. A Message-ID on a note and the same one on an email are not one
// message, whatever the header says.
func sameKindAs(ctx context.Context, tx pgx.Tx, incumbent ids.ActivityID, kind string) (bool, error) {
	var have string
	if err := tx.QueryRow(ctx, `SELECT kind FROM activity WHERE id = $1`, incumbent).Scan(&have); err != nil {
		return false, err
	}
	return have == kind, nil
}
