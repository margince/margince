// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CardPhotoUploadBytes is the body ceiling of POST /contacts/card-photos: the
// largest image a model reads, plus room for the multipart framing around it.
const CardPhotoUploadBytes = imagenorm.MaxReadImageBytes + cardPhotoFramingBytes

// cardPhotoFramingBytes is the multipart framing allowed beside the photo: the
// boundary lines and part headers, which a browser keeps under a kilobyte.
const cardPhotoFramingBytes = 64 << 10

// cardPhotoField is the multipart part that carries the photo, and the field a
// refusal of it names.
const cardPhotoField = "file"

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
	return cardPhotoField, e.Code, e.Message
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
	photo, ok := readCardPhotoUpload(w, r)
	if !ok {
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

// readCardPhotoUpload reads the photo part, bounded before anything is
// buffered: the body by CardPhotoUploadBytes, the part by the image bound. A
// larger photo is a 413 and never reaches the reading. It writes the refusal
// itself and reports false.
func readCardPhotoUpload(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, CardPhotoUploadBytes)
	// upload:route /v1/contacts/card-photos
	// The ceiling is granted to that path in compose.uploadCeilings, and
	// TestEveryMultipartParseNamesItsRoute holds the two together.
	//nolint:gosec // G120 wants a bound, and the MaxBytesReader above is it: this argument is the spill threshold.
	if err := r.ParseMultipartForm(vcardSpillBytes); err != nil {
		httperr.WriteMultipartRefusal(w, r, err, CardPhotoUploadBytes)
		return nil, false
	}
	file, _, err := r.FormFile(cardPhotoField)
	if err != nil {
		httperr.Write(w, r, httperr.Validation(cardPhotoField, "missing", "Attach the photo to read as `file`."))
		return nil, false
	}
	defer func(ctx context.Context) {
		if cerr := file.Close(); cerr != nil {
			slog.WarnContext(ctx, "closing the uploaded card photo part", "err", cerr)
		}
	}(r.Context())
	photo, err := io.ReadAll(io.LimitReader(file, imagenorm.MaxReadImageBytes+1))
	if err != nil {
		httperr.Write(w, r, httperr.Validation(cardPhotoField, "unreadable", "The photo could not be read; send it again."))
		return nil, false
	}
	if len(photo) > imagenorm.MaxReadImageBytes {
		httperr.Write(w, r, httperr.BodyTooLargeRefusal(fmt.Sprintf(
			"the photo exceeds the %s limit; send a smaller photo", httperr.Megabytes(imagenorm.MaxReadImageBytes))))
		return nil, false
	}
	return photo, true
}
