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
//   - A different person wrote the incumbent → binding would make one person's
//     mail reachable through the other's, on the strength of a header the
//     sender typed. See BindableTo.
//   - The kinds disagree → a Message-ID on a note and the same one on an email
//     are not one message.
func boundToKnownMessage(ctx context.Context, tx pgx.Tx, in LogActivityInput) (crmcontracts.Activity, bool, error) {
	kind, key := identityOf(in)
	if key == "" {
		return crmcontracts.Activity{}, false, nil
	}
	incumbent, found, err := ResolveIdentity(ctx, tx, kind, key)
	if err != nil || !found {
		return crmcontracts.Activity{}, false, err
	}
	bindable, err := BindableTo(ctx, tx, incumbent)
	if err != nil || !bindable {
		return crmcontracts.Activity{}, false, err
	}
	same, err := sameKindAs(ctx, tx, incumbent, in.Kind)
	if err != nil || !same {
		return crmcontracts.Activity{}, false, err
	}
	out, err := readActivity(ctx, tx, incumbent, storekit.IncludeArchived)
	if errors.Is(err, apperrors.ErrNotFound) {
		// Out of scope. The message stays hidden and this caller files their
		// own copy, which is the same answer capture reaches for an incumbent
		// it may not see.
		return crmcontracts.Activity{}, false, nil
	}
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	return out, true, nil
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
