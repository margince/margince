// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which addresses are the READER's own, for telling a message written to them
// from one written to a colleague that reached the same mailbox.
//
// Its own file because it answers a different question from the identities
// beside it: those gate what capture may create, this one only reports who a
// message named, and the queue that asks it never excludes on the answer.

package capture

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReaderAddressesTx is one seat's own email addresses: their login address,
// what they declared about themselves, and the account label of every mailbox
// they have connected.
//
// The waiting queue asks it to tell "this message was written to me" from "this
// message was written to a colleague and reached my mailbox". Neither
// participant row answers that alone — capture stamps the owner with a user_id
// and no address, and mailmap drops the owner's own address from the header
// list — so the queue asks whether every named recipient is somebody else, and
// needs this set to know who "somebody else" is not.
//
// ALL THREE SOURCES, folded the way ownerIdentitiesTx folds them. A seat who
// signs in as one address and connects a mailbox at another has two, and a
// connection label is stored in display form: "Reader <reader@ours.test>" never
// equals a bare header address, so it is parsed rather than lowercased. Missing
// either source does not fail open — the list is non-empty and simply wrong,
// which reads as "written to somebody else" about the reader's own mail.
//
// SELF ONLY. These are private aliases, and a store entry point that answered
// for any id would hand one seat another's. The queue passes the acting
// reader's id; anything else is refused.
func (s *OwnDomainStore) ReaderAddressesTx(
	ctx context.Context, tx pgx.Tx, user ids.UUID,
) ([]string, error) {
	if user == ids.Nil {
		return nil, nil
	}
	if err := selfOnly(ctx, user); err != nil {
		return nil, err
	}
	login, err := seatLoginAddressTx(ctx, tx, user)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT value FROM capture_owner_identity
		 WHERE user_id = $1 AND kind = 'address'
		 UNION
		SELECT account_label FROM capture_connection
		 WHERE user_id = $1 AND coalesce(account_label, '') <> '' AND archived_at IS NULL
`, user)
	if err != nil {
		return nil, fmt.Errorf("capture: reading a reader's own addresses: %w", err)
	}
	defer rows.Close()
	folded := map[string]bool{}
	if login != "" {
		folded[strings.ToLower(login)] = true
	}
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		if address := strings.ToLower(strings.TrimSpace(bareAddress(label))); address != "" {
			folded[address] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(folded))
	for address := range folded {
		out = append(out, address)
	}
	sort.Strings(out)
	return out, nil
}

// selfOnly refuses a read of somebody else's private aliases.
//
// A system principal is admitted: the overnight assembly runs as one and reads
// for a named seat, and there is no acting human to compare against. What
// bounds it there is the caller — the pass resolves the id from the reader it
// is assembling for, never from a request.
func selfOnly(ctx context.Context, user ids.UUID) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type == principal.PrincipalSystem {
		return nil
	}
	if actor.UserID == user {
		return nil
	}
	return fmt.Errorf("capture: a seat's own addresses are theirs alone: %w",
		apperrors.ErrPermissionDenied)
}
