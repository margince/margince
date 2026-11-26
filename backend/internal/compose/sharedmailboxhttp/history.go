// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package sharedmailboxhttp

import (
	"context"
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// History starts a shared mailbox's history import on the personal import's
// scheduler and answers it the way the personal start does.
type History struct {
	Registry *capture.Registry
	// Months maps the wire window onto months; false for a window not offered.
	Months func(window string) (int, bool)
	// Schedule runs one start with the enqueue that schedules its pager.
	Schedule func(context.Context, ids.UUID, func(context.Context, capture.EnqueueBackfill) (capture.BackfillRun, error)) (capture.BackfillRun, error)
	// Refuse answers a start that opened no run.
	Refuse func(http.ResponseWriter, *http.Request, error)
	// Accepted renders a queued run.
	Accepted func(*capture.BackfillRun) crmcontracts.BackfillStatus
}

// Start queues the import for one window.
func (h History) Start(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.SharedMailboxHistoryImportRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	months, ok := h.Months(string(req.Window))
	if !ok {
		h.Refuse(w, r, capture.ErrWindowInvalid)
		return
	}
	ws, ok := principal.WorkspaceID(r.Context())
	if !ok {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusInternalServerError, Code: "workspace_missing",
			Detail: "The request carries no workspace context; nothing was started. Try again.",
		})
		return
	}
	run, err := h.Schedule(r.Context(), ws, func(ctx context.Context, enqueue capture.EnqueueBackfill) (capture.BackfillRun, error) {
		return h.Registry.StartSharedBackfill(ctx, ids.UUID(id), months, connector.BackfillEstimate{}, enqueue) // no estimate: counts only
	})
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		httperr.Write(w, r, err)
	case err != nil:
		h.Refuse(w, r, err)
	default:
		httperr.WriteJSON(w, http.StatusAccepted, h.Accepted(&run))
	}
}
