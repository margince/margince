// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// maskMailPatch masks the credentials in an edit to a mail's subject and body,
// so an edit cannot put one back on a row capture masked.
func maskMailPatch(current crmcontracts.Activity, in *UpdateActivityInput) {
	maskTextOfKind(current.Kind, &in.Subject, &in.Body)
}

// maskTextOfKind masks the credentials in the texts a row of kind stores, unless
// the kind keeps its text as typed. Create and edit both pass here.
func maskTextOfKind(kind crmcontracts.ActivityKind, texts ...**string) {
	if keepsTypedText(kind) {
		return
	}
	for _, text := range texts {
		if *text != nil {
			masked := credentialmask.MaskText(**text)
			*text = &masked
		}
	}
}

// keepsTypedText reports whether a kind stores its subject and body as the
// member typed them. A mail and a channel message are stored masked, as capture
// stores them. A note, a task and the other kinds keep the text, because the
// member chose to store it there; only their audit images are masked, in
// storekit.
func keepsTypedText(kind crmcontracts.ActivityKind) bool {
	return kind != crmcontracts.ActivityKindEmail && kind != crmcontracts.ActivityKindMessage
}

// KeepsTypedTextTx reports whether the live activity stores the text its
// member typed, so a masked value from its history is not what it held.
func KeepsTypedTextTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	var kind crmcontracts.ActivityKind
	err := tx.QueryRow(ctx, `SELECT kind FROM activity WHERE id = $1 AND archived_at IS NULL`, id).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, apperrors.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	return keepsTypedText(kind), nil
}
