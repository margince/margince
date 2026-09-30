// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Which folders or labels a seat's own mailbox has.
//
// The one read behind the exclusion card's container picker. It exists because
// a container rule names a provider's own token — a Gmail label id, a Graph
// folder id — and asking somebody to type one is asking them to look it up.
//
// A seat's OWN mailbox and no other. The rule this feeds is scope `user` by
// database constraint (a label lives in one mailbox and means nothing in
// anybody else's), so a listing that reached a colleague's folders would be
// offering a rule that cannot be written, from a mailbox the caller has no
// business enumerating.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ErrContainersUnsupported marks a provider whose connector cannot enumerate
// folders — a channel transport, or a mail connector that has not grown the
// verb. Its own sentinel so the transport answers "this provider has no
// folders" rather than a fault the caller should retry.
var ErrContainersUnsupported = errors.New("capture: this provider does not list folders")

// ListContainers returns the folders or labels of this seat's connection.
//
// The listing is LIVE rather than stored, and that is the point: a folder
// somebody made this morning is one they may want excluded this morning, and a
// cached list would offer yesterday's mailbox. It costs one provider round
// trip, which is why the picker asks for it when it opens rather than on every
// keystroke.
func (r *Registry) ListContainers(ctx context.Context, name string, userID ids.UserID) ([]connector.NamedContainer, error) {
	conn, err := r.connector(name)
	if err != nil {
		return nil, err
	}
	lister, ok := conn.(connector.ContainerLister)
	if !ok {
		return nil, ErrContainersUnsupported
	}
	var credentialRef *string
	var authBytes []byte
	if err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT credential_ref, auth FROM capture_connection
			 WHERE user_id = $1 AND provider = $2 AND status <> 'disconnected'
			   AND archived_at IS NULL`,
			userID, name).Scan(&credentialRef, &authBytes)
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ErrNotFound
		}
		return nil, fmt.Errorf("capture: reading the connection to list its folders: %w", err)
	}
	auth, resolveErr := r.resolveCredential(ctx, credentialRef, authBytes)
	if resolveErr != nil {
		return nil, resolveErr
	}
	return lister.ListContainers(ctx, auth)
}
