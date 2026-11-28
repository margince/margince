// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ManagedLink requires management authority before a link can be reviewed.
func (s *Store) ManagedLink(ctx context.Context, kind string, record ids.UUID) (Link, error) {
	if _, err := memberMutationActor(ctx); err != nil {
		return Link{}, err
	}
	var out Link
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		live, err := s.holdMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := requireRecordRead(live, kind); err != nil {
			return err
		}
		if err := requireRecordUpdate(live, kind); err != nil {
			return err
		}
		if err := auth.EnsureWritableLive(live, tx, kind, record); err != nil {
			return err
		}
		var readErr error
		out, readErr = liveLinkOf(live, tx, kind, record)
		if readErr != nil {
			return readErr
		}
		return tx.QueryRow(live, `SELECT channel_label FROM channel_connection WHERE id=@id`, pgx.StrictNamedArgs{"id": out.ConnectionID}).Scan(&out.WorkspaceName)
	})
	return out, err
}

// PreviewChannelDelivery shares the preview engine used by actual outbound delivery.
func (h Handlers) PreviewChannelDelivery(w http.ResponseWriter, r *http.Request, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	if h.preview == nil {
		writeErr(w, r, messaging.ErrUnsupported)
		return
	}
	preview, err := h.preview.PreviewDelivery(r.Context(), string(kind), ids.UUID(id))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, preview)
}
