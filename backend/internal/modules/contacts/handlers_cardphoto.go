// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CardPhotoReader turns a photo into the cards a model read off it, each marked
// ModelRead. It is injected because the model lane lives in a sibling module.
type CardPhotoReader func(ctx context.Context, photo []byte) ([]VCardEntry, error)

// CardPhotoRefusal is a photo the reading cannot use, with a code a client can
// translate. It reaches the client as a 422 on the `file` field.
type CardPhotoRefusal struct{ Code, Message string }

func (e *CardPhotoRefusal) Error() string { return e.Message }

// FieldFault carries the refusal to every surface, as the module's other
// refusals do.
func (e *CardPhotoRefusal) FieldFault() (field, code, message string) {
	return "file", e.Code, e.Message
}

// WithCardPhotoReader wires the model reading behind POST /contacts/card-photos.
// Nil means an installation with no model, and the route then answers 501.
func (h Handlers) WithCardPhotoReader(read CardPhotoReader) Handlers {
	h.readCardPhoto = read
	return h
}

// ImportCardPhoto serves POST /contacts/card-photos: read the photo, propose
// every card it shows, write none of them.
//
// The grants come before the bytes, as in ImportVCards. A seat that may not
// import must not cost a model call.
func (h Handlers) ImportCardPhoto(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r.Context(), entityContact, principal.ActionCreate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := auth.Require(r.Context(), entityContact, principal.ActionUpdate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if h.readCardPhoto == nil {
		httperr.NotImplementedBecause(w, r, "This installation has no model configured to read photos.")
		return
	}
	// upload:route /v1/contacts/card-photos. The ceiling is granted to that path
	// in compose.uploadCeilings, and TestEveryMultipartParseNamesItsRoute holds
	// the two together.
	//nolint:gosec // G120 wants a bound, and the bound is the chassis's own MaxBytesReader on this route: this argument is the spill threshold.
	if err := r.ParseMultipartForm(vcardSpillBytes); err != nil {
		httperr.Write(w, r, httperr.Validation("file", "unreadable",
			"Send the photo as a multipart form field named `file`."))
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		httperr.Write(w, r, httperr.Validation("file", "missing", "Attach the photo to read as `file`."))
		return
	}
	defer func(ctx context.Context) {
		if cerr := file.Close(); cerr != nil {
			slog.WarnContext(ctx, "closing the uploaded card photo part", "err", cerr)
		}
	}(r.Context())
	photo, err := io.ReadAll(file)
	if err != nil {
		httperr.Write(w, r, httperr.Validation("file", "unreadable", "The photo could not be read; send it again."))
		return
	}

	entries, err := h.readCardPhoto(r.Context(), photo)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	results, err := h.store.ProposeCardsFromPhoto(r.Context(), entries)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	h.stageVCardReviews(r.Context(), entries, results)
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.VCardImportReport{
		Results: toContractVCardResults(results),
	})
}
