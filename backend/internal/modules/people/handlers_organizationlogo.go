// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Streaming a company's mark. Two endpoints, one for each slot the record
// carries (orglogowrite.go says why there are two), and ONE body: the slot
// decides which key is read and nothing else about the response differs, so a
// second copy of the stream would be a second set of security headers to keep
// in step with the first.

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetOrganizationLogo streams the organization's wide mark — the lockup a
// record page and an expanded sidebar draw.
func (h Handlers) GetOrganizationLogo(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.streamLogo(w, r, id, LogoWide, "GetOrganizationLogo")
}

// GetOrganizationLogoIcon streams the square badge a collapsed sidebar draws.
// Only the installation's own company wears one today; every other record
// answers the same 404 it answers for a mark it does not have.
func (h Handlers) GetOrganizationLogoIcon(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.streamLogo(w, r, id, LogoIcon, "GetOrganizationLogoIcon")
}

// streamLogo serves one slot's stored bytes. A record with no mark in that
// slot, one this caller cannot see, and one that does not exist all answer 404:
// the client's response to all three is the same monogram, and telling them
// apart would leak which organizations exist.
func (h Handlers) streamLogo(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, slot LogoSlot, operation string) {
	key, err := h.store.OrganizationLogoKey(r.Context(), pathID[ids.OrganizationKind](id), slot)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if h.blob == nil {
		httperr.NotImplemented(w, r, operation)
		return
	}
	rc, _, err := h.blob.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, blobstore.ErrNotFound) {
			// The row points at bytes the store does not have. To the client
			// that is a company without a logo, same as any other.
			writeStoreErr(w, r, apperrors.ErrNotFound)
			return
		}
		httperr.Write(w, r, err)
		return
	}
	source, readErr := io.ReadAll(rc)
	closeErr := rc.Close()
	if closeErr != nil {
		slog.WarnContext(r.Context(), "closing organization logo reader", "err", closeErr)
	}
	if readErr != nil {
		httperr.Write(w, r, readErr)
		return
	}
	logo, err := imagenorm.TrimTransparentPNG(source)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// These bytes were normalized from a third-party website's asset, and three
	// things keep that from mattering at the response. The media type is fixed
	// rather than read back from the object's metadata — the contract declares
	// this endpoint image/png and every stored object is this server's own PNG
	// re-encode, so nothing a site influenced decides how its bytes are
	// interpreted. Then the type cannot be sniffed into something active, and
	// the document that renders can reach nothing.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	// The URL carries a revision token derived from the stored object key, so a
	// replacement takes a fresh cache entry. A company list asks for one image
	// per row, and this short private cache saves the repeated reads of each.
	w.Header().Set("Cache-Control", "private, max-age=300")
	httperr.StreamObject(w, r, httperr.StreamedObject{
		Download: httperr.Download{ContentType: imagenorm.ContentType, Inline: true, Size: int64(len(logo))},
		Body:     io.NopCloser(bytes.NewReader(logo)),
	}, "organization logo "+id.String())
}
