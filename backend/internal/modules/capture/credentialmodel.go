// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whose credential carried this record, which is what decides whether it is one
// contact's correspondence or the company's.
//
// A transport that spends ONE MEMBER's credential is that member's own account,
// so their chats are correspondence and the rules mail is under reach them. A
// transport that spends the installation's — a bot, an Official Account — serves
// everybody, and its traffic is workspace business.
//
// The registry row is the answer, read in the capture transaction rather than
// from a snapshot compose took at boot. `activity.channel_provider` is a foreign
// key into that row, so the insert this decision precedes already depends on it
// existing — asking the database is asking the same authority the write is held
// to, where an in-memory copy is one a later reconcile can leave behind.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/pkg/extension"
)

// memberBoundCredentialTx answers whether the transport this record arrived on
// spends a credential bound to one member.
//
// An empty provider is a record that travelled on no channel at all — mail, a
// meeting, a hand-logged note — and answers false without a query.
//
// An unregistered provider is an error rather than a false: the insert that
// follows would fail on the foreign key anyway, and answering "not member-bound"
// would quietly publish a message whose transport the installation cannot even
// name.
func memberBoundCredentialTx(ctx context.Context, tx pgx.Tx, provider string) (bool, error) {
	if provider == "" {
		return false, nil
	}
	var model string
	if err := tx.QueryRow(ctx,
		`SELECT credential_model FROM channel_provider WHERE provider = $1`,
		provider).Scan(&model); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, fmt.Errorf("capture: %q is not a registered channel transport", provider)
		}
		return false, fmt.Errorf("capture: reading whose credential %q spends: %w", provider, err)
	}
	return model == string(extension.CredentialPerMember), nil
}
