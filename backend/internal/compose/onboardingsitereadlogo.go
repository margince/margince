// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The mark a website read resolves, served while it is still the dossier's:
// where the report says to fetch it, and the route that streams it. Its own
// file beside the transport because it is the one part of the dossier that
// is bytes rather than a report, and it reads and answers like the
// organization's own logo route rather than like the rest of the read.

import (
	"errors"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// siteReadLogoURL is where a client fetches the mark a read resolved, or nil
// while it resolved none. The storage key never reaches the wire, for the
// reason people.LogoURL gives: it names a bucket path, and a client's business
// is the endpoint that streams the object.
func siteReadLogoURL(read people.SiteRead) *string {
	if read.LogoObjectKey == nil || *read.LogoObjectKey == "" {
		return nil
	}
	path := "/v1/company/site-reads/" + read.ID.String() + "/logo"
	return &path
}

// getCompanySiteReadLogo streams the mark a read parked on its dossier, so
// the review shows the company it is about before the record exists. The
// same response discipline as the organization's own logo route: the type is
// fixed to the server's own PNG re-encode rather than read back from the
// object, so nothing a site influenced decides how its bytes are interpreted.
func (e *deepReadEngine) getCompanySiteReadLogo(w http.ResponseWriter, r *http.Request, readID openapi_types.UUID) {
	key, err := e.people.SiteReadLogoKey(r.Context(), ids.UUID(readID))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if e.blob == nil {
		httperr.NotImplemented(w, r, "GetCompanySiteReadLogo")
		return
	}
	rc, obj, err := e.blob.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, blobstore.ErrNotFound) {
			// The dossier points at bytes the store does not have. To the
			// client that is a read without a mark, same as any other.
			httperr.Write(w, r, apperrors.ErrNotFound)
			return
		}
		httperr.Write(w, r, err)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	// A parked mark changes only when the read resolves another, and the
	// review polls the dossier while it draws this: a short private cache
	// spares the repeat fetches without holding a stale mark for long.
	w.Header().Set("Cache-Control", "private, max-age=300")
	httperr.StreamObject(w, r, httperr.StreamedObject{
		Download: httperr.Download{ContentType: imagenorm.ContentType, Inline: true, Size: obj.Size},
		Body:     rc,
	}, "site read logo "+readID.String())
}

func (h siteReadHandlers) GetCompanySiteReadLogo(w http.ResponseWriter, r *http.Request, readID openapi_types.UUID) {
	if !companyContextReadEnabled(h.companyContextRollout) {
		httperr.NotImplemented(w, r, "getCompanySiteReadLogo (company context read rollout is disabled)")
		return
	}
	if h.engine == nil {
		httperr.NotImplemented(w, r, "getCompanySiteReadLogo (no crawl runner configured)")
		return
	}
	h.engine.getCompanySiteReadLogo(w, r, readID)
}
