// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The transport for a company mark a person chooses themselves: take the
// upload, re-encode it, store the bytes, point the record at them — and give
// the field back when they ask for the mark to go.
//
// The bytes are never served as they arrived. Every stored mark is this
// server's own square PNG re-encode, which is what lets the endpoint that
// streams them declare one media type for all of them and serve no
// third-party markup from this origin: an SVG a person uploads is rasterized
// here, exactly as one resolved from a website is.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The in-memory/spill threshold for the upload parse, deliberately far below
// the route's own ceiling (compose.uploadCeilings) so a large image spills to
// disk instead of being held resident.
const companyLogoSpillBytes = 1 << 20

// The edge the stored mark is re-encoded to. It is the size the largest surface
// draws a company at, doubled for a high-density screen, and no more: a mark is
// shown at 32px in the rail and 46px on a record header, so anything beyond
// this is bytes every one of those requests pays for and no reader ever sees.
const companyLogoEdge = 256

// A filename is shown back to a person in the field's history, so it is bounded
// here rather than trusted: browsers send what the file was called, and what a
// file is called is under the control of whoever made it.
const companyLogoNameMax = 120

func (h companyHandlers) UploadCompanyLogo(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "uploadCompanyLogo")
		return
	}
	if h.blob == nil {
		// No object store: the bytes have nowhere to live, and the endpoint
		// that would serve them answers the same way.
		httperr.NotImplemented(w, r, "uploadCompanyLogo")
		return
	}
	// Read the company BEFORE taking the upload apart: an installation that has
	// not described itself yet has no record to give a mark to, and that 404 is
	// worth answering before an image is decoded.
	company, err := h.store.GetCompany(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	png, filename, ok := decodeCompanyLogoUpload(w, r)
	if !ok {
		return
	}
	workspace, found := principal.WorkspaceID(r.Context())
	if !found {
		httperr.Write(w, r, errors.New("compose: a company logo upload outside a workspace context"))
		return
	}
	// A key of this attempt's own, the same rule the resolve lane keeps: two
	// writers of one company's mark must never write the same object, or the
	// stored image and the record's provenance end up describing different
	// pictures.
	key := organizationLogoKey(ids.From[ids.WorkspaceKind](workspace), company.OrganizationID)
	if err := h.blob.Put(r.Context(), key, bytes.NewReader(png), int64(len(png)), imagenorm.ContentType); err != nil {
		httperr.Write(w, r, err)
		return
	}
	superseded, setErr := h.store.SetCompanyLogo(r.Context(), key, filename)
	if setErr != nil {
		// The bytes stay. A failed write here does not prove the transaction
		// did not commit — a cancelled context, a dropped connection — and
		// deleting an object the row may now name would show a broken image
		// where an orphan only costs storage.
		httperr.Write(w, r, setErr)
		return
	}
	h.collectLogoObject(r.Context(), superseded)
	h.writeCompany(w, r)
}

func (h companyHandlers) DeleteCompanyLogo(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "deleteCompanyLogo")
		return
	}
	removed, err := h.store.ClearCompanyLogo(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.collectLogoObject(r.Context(), removed)
	h.writeCompany(w, r)
}

// writeCompany answers both writes with the profile they produced, so the
// caller renders the company's new face from the response it already has
// rather than from a second read that may not see its own write yet.
func (h companyHandlers) writeCompany(w http.ResponseWriter, r *http.Request) {
	company, err := h.store.GetCompany(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractCompany(company))
}

// decodeCompanyLogoUpload turns the request into the bytes that will be stored:
// the file part, decoded as an image and re-encoded as this server's own PNG.
// It writes the refusal itself and reports whether the caller may continue.
func decodeCompanyLogoUpload(w http.ResponseWriter, r *http.Request) (png []byte, filename string, ok bool) {
	// upload:route /v1/company/logo — the ceiling this parse runs under is
	// granted to that path in compose.uploadCeilings, and
	// TestEveryMultipartParseNamesItsRoute holds the two together.
	//nolint:gosec // G120 wants a bound here, and the bound is the route ceiling the chassis already applied: this argument is only the in-memory/spill threshold.
	if err := r.ParseMultipartForm(companyLogoSpillBytes); err != nil {
		httperr.WriteMultipartRefusal(w, r, err, companyLogoUploadBytes)
		return nil, "", false
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httperr.Write(w, r, httperr.Validation("file", "required", "an image file part is required"))
		return nil, "", false
	}
	defer func(ctx context.Context) {
		if cerr := file.Close(); cerr != nil {
			slog.WarnContext(ctx, "closing the uploaded company logo", "err", cerr)
		}
	}(r.Context())

	source, err := io.ReadAll(file)
	if err != nil {
		httperr.Write(w, r, err)
		return nil, "", false
	}
	img, err := imagenorm.Decode(source)
	if err != nil {
		if errors.Is(err, imagenorm.ErrUnsupported) {
			httperr.Write(w, r, &httperr.DetailedError{
				Status: http.StatusUnsupportedMediaType, Code: "unsupported_media_type",
				Detail: "the upload is not an image this server can read; PNG, JPEG, GIF, WebP, ICO and SVG work",
			})
			return nil, "", false
		}
		httperr.Write(w, r, err)
		return nil, "", false
	}
	png, err = imagenorm.SquarePNG(img, companyLogoEdge)
	if err != nil {
		httperr.Write(w, r, err)
		return nil, "", false
	}
	return png, boundedFilename(header.Filename), true
}

// boundedFilename is what the field's history will show for this change. A
// browser sends the name the file had, and a name is under the control of
// whoever made the file — so it is trimmed to something a line of history can
// hold, and a nameless part simply names nothing rather than inventing a name.
func boundedFilename(name string) string {
	// Cut by RUNE. A byte cut through a multi-byte character leaves a fragment
	// that is not text at all, and the name is stored as a string and rendered
	// back to a person.
	runes := []rune(strings.TrimSpace(name))
	if len(runes) > companyLogoNameMax {
		return string(runes[:companyLogoNameMax])
	}
	return string(runes)
}

// collectLogoObject deletes bytes nothing references any more. A failure is
// logged rather than returned: the write the caller asked for has happened, and
// an object nobody names costs storage and nothing else.
func (h companyHandlers) collectLogoObject(ctx context.Context, key *string) {
	if h.blob == nil || key == nil || *key == "" {
		return
	}
	if err := h.blob.Delete(ctx, *key); err != nil {
		slog.WarnContext(ctx, "collecting the superseded company logo failed",
			"key", *key, "err", err)
	}
}
