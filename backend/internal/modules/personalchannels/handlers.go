// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Handlers serves /personal-channels for the caller's own connection.
type Handlers struct {
	store *Store
}

// NewHandlers serves store's connection, replies and uploads.
func NewHandlers(store *Store) Handlers {
	return Handlers{store: store}
}

func connectionWire(c Connection) crmcontracts.PersonalChannelConnection {
	return crmcontracts.PersonalChannelConnection{
		Enabled:              c.Enabled,
		RepliesWaiting:       c.RepliesWaiting,
		OldestReplyWaitingAt: c.OldestReplyWaitingAt,
		LastPullAt:           c.LastPullAt,
	}
}

// GetPersonalChannelConnection implements (GET /personal-channels/connection).
func (h Handlers) GetPersonalChannelConnection(w http.ResponseWriter, r *http.Request) {
	c, err := h.store.Connection(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, connectionWire(c))
}

// UpdatePersonalChannelConnection implements (PATCH /personal-channels/connection).
func (h Handlers) UpdatePersonalChannelConnection(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.UpdatePersonalChannelConnectionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	c, err := h.store.SetEnabled(r.Context(), req.Enabled)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, connectionWire(c))
}

// UploadPersonalChannelFile implements (POST /personal-channels/uploads).
func (h Handlers) UploadPersonalChannelFile(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r.Context(), rbacObject, principal.ActionCreate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// upload:route /v1/personal-channels/uploads (compose.uploadCeilings bounds the body).
	//nolint:gosec // G120: the route's own MaxBytesReader is the bound; this is the spill threshold.
	if err := r.ParseMultipartForm(uploadSpillBytes); err != nil {
		if tooLarge, over := errors.AsType[*http.MaxBytesError](err); over {
			httperr.WriteMultipartRefusal(w, r, err, tooLarge.Limit)
			return
		}
		httperr.Write(w, r, httperr.Validation("file", "unreadable", "Send the file as a multipart form field named `file`."))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httperr.Write(w, r, httperr.Validation("file", "missing", "Attach the file as `file`."))
		return
	}
	defer func(ctx context.Context) {
		if cerr := file.Close(); cerr != nil {
			slog.WarnContext(ctx, "closing an uploaded chat file", "err", cerr)
		}
	}(r.Context())
	up, err := h.store.StageUpload(r.Context(), header.Filename, file)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, crmcontracts.PersonalChannelUpload{
		UploadId: openapi_types.UUID(up.ID), Filename: up.Filename, ContentType: up.ContentType,
		Size: int(up.Size), Sha256: up.SHA256,
	})
}

// uploadSpillBytes is how much of a multipart upload stays in memory before it
// spills to disk.
const uploadSpillBytes = 1 << 20
