// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/messengeraccounts"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const fileChannelMessageTool = "file_channel_message"

type chatSourceHandlers struct {
	pool  *pgxpool.Pool
	notes *activities.Store
	links *chatlinks.Store
	now   func() time.Time
}

func (h chatSourceHandlers) liveReader(ctx context.Context) (context.Context, error) {
	var out context.Context
	err := database.WithTx(ctx, h.pool, func(tx pgx.Tx) error {
		var err error
		out, err = messengeraccounts.MemberAuthority(ctx, tx)
		return err
	})
	return out, err
}

func (h chatSourceHandlers) GetChatSummarySource(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.notes == nil || h.links == nil || h.now == nil {
		httperr.ServiceUnavailable(w, r, "Source verification is unavailable.")
		return
	}
	ctx, err := h.liveReader(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	activity := ids.ActivityID{UUID: ids.UUID(id)}
	binding, err := h.notes.ChatSource(ctx, activity)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return
	}
	out := crmcontracts.ChatSourceAccess{UserId: openapi_types.UUID(actor.UserID)}
	link, proofErr := h.links.VerifySource(ctx, binding.Provider, binding.Connection, binding.Channel, binding.Root)
	if proofErr != nil && !chatSourceUnavailable(proofErr) {
		httperr.Write(w, r, proofErr)
		return
	}
	ctx, err = h.liveReader(ctx)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	current, err := h.notes.ChatSource(ctx, activity)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if proofErr == nil && current == binding && link != "" {
		until := h.now().Add(30 * time.Second)
		out.Available, out.Url, out.ValidUntil = true, &link, &until
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func chatSourceUnavailable(err error) bool {
	return errors.Is(err, messaging.ErrUnauthorized) || errors.Is(err, messaging.ErrUnavailable) || errors.Is(err, messaging.ErrUnsupported) || errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, capture.ErrNoConnection) || errors.Is(err, chatlinks.ErrBotNotConnected)
}
