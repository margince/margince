// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The two exact lanes a messaging channel needs, siblings of
// exactContactByEmail (dedupe.go): a previously established channel binding,
// and an E.164 phone number. Both share the email lane's contract — live
// rows only, lowest contact id on a tie, so the same candidate resolves the
// same way on every run.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// exactContactByChannelIdentity is the ladder's first lane: a
// (provider, channel_user_id) pair already bound to a live contact.
//
// blocked_at is deliberately not read. Blocking is reachability — the fact
// "telegram user 123 is contact A" stays true while the user has the bot
// blocked, and a lane that missed on it would create a SECOND contact the
// moment they unblocked and wrote again, which the partial unique index
// (0146) happily admits.
func exactContactByChannelIdentity(ctx context.Context, tx pgx.Tx, identities []connector.ChannelIdentity) (ids.ContactID, bool, error) {
	if len(identities) == 0 {
		return ids.ContactID{}, false, nil
	}
	providers := make([]string, 0, len(identities))
	channelUserIDs := make([]string, 0, len(identities))
	for _, ci := range identities {
		if ci.Provider == "" || ci.ChannelUserID == "" {
			// Half a key is no key: it can only match by accident, and the
			// unique index never admitted such a row in the first place.
			continue
		}
		providers = append(providers, ci.Provider)
		channelUserIDs = append(channelUserIDs, ci.ChannelUserID)
	}
	if len(providers) == 0 {
		return ids.ContactID{}, false, nil
	}
	var id ids.ContactID
	err := tx.QueryRow(ctx, `
		SELECT pci.contact_id
		  FROM contact_channel_identity pci
		  JOIN unnest($1::text[], $2::text[]) AS k(provider, channel_user_id)
		    ON k.provider = pci.provider AND k.channel_user_id = pci.channel_user_id
		 WHERE pci.archived_at IS NULL
		 ORDER BY pci.contact_id
		 LIMIT 1`, providers, channelUserIDs).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ContactID{}, false, nil
	}
	if err != nil {
		return ids.ContactID{}, false, fmt.Errorf("dedupe contact channel-identity tier: %w", err)
	}
	return id, true, nil
}

// exactContactByPhone matches on the E.164 form contact_phone stores. The
// candidate side is normalized here so the comparison is like for like;
// a number that cannot be normalized is dropped rather than compared,
// because it can never equal a stored E.164 value and refusing the whole
// resolution over one malformed provider field would drop the message.
func exactContactByPhone(ctx context.Context, tx pgx.Tx, phones []string) (ids.ContactID, bool, error) {
	if len(phones) == 0 {
		return ids.ContactID{}, false, nil
	}
	normalized := make([]string, 0, len(phones))
	for _, raw := range phones {
		parsed, err := values.ParsePhone(raw)
		if err != nil {
			continue
		}
		normalized = append(normalized, parsed.String())
	}
	if len(normalized) == 0 {
		return ids.ContactID{}, false, nil
	}
	var id ids.ContactID
	err := tx.QueryRow(ctx, `
		SELECT contact_id FROM contact_phone
		WHERE phone = ANY($1) AND archived_at IS NULL
		ORDER BY contact_id
		LIMIT 1`, normalized).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ContactID{}, false, nil
	}
	if err != nil {
		return ids.ContactID{}, false, fmt.Errorf("dedupe contact phone tier: %w", err)
	}
	return id, true, nil
}
