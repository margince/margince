// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Streaming a company's mark. Two endpoints, one for each slot the record
// carries (companylogowrite.go says why there are two), and ONE body: the slot
// decides which key is read and nothing else about the response differs, so a
// second copy of the stream would be a second set of security headers to keep
// in step with the first.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// logoCacheControl is the private, short-lived cache every logo response
// carries — 200 and 304 alike, so a client revalidating gets the same
// freshness window as one that fetched fresh bytes.
const logoCacheControl = "private, max-age=300"

// logoWriteBackTimeout bounds the write-back below, the same shape
// ai.flushDetached gives its own post-response write: generous for one small
// PUT, and short enough that a dead blob store cannot pin a handler goroutine.
const logoWriteBackTimeout = 5 * time.Second

// GetCompanyLogo streams the company's wide mark — the lockup a
// record page and an expanded sidebar draw.
func (h Handlers) GetCompanyLogo(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.streamLogo(w, r, id, LogoWide, "GetCompanyLogo")
}

// GetCompanyLogoIcon streams the square badge a collapsed sidebar draws.
// Only the installation's own company wears one today; every other record
// answers the same 404 it answers for a mark it does not have.
func (h Handlers) GetCompanyLogoIcon(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.streamLogo(w, r, id, LogoIcon, "GetCompanyLogoIcon")
}

// streamLogo serves one slot's stored bytes. A record with no mark in that
// slot, one this caller cannot see, and one that does not exist all answer 404:
// the client's response to all three is the same monogram, and telling them
// apart would leak which companies exist.
func (h Handlers) streamLogo(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, slot LogoSlot, operation string) {
	companyID := pathID[ids.CompanyKind](id)
	key, err := h.store.CompanyLogoKey(r.Context(), companyID, slot)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if h.blob == nil {
		httperr.NotImplemented(w, r, operation)
		return
	}
	// The object key is minted fresh per upload (companyLogoKey /
	// siteReadLogoKey, compose/sitelogo.go), so it already names these exact
	// bytes — the same digest LogoURL bakes into its cache-busting query
	// token doubles as the ETag. A match means the client already holds
	// today's picture: answer before touching blob storage or spending a
	// decode and a per-pixel scan on bytes it will throw away.
	etag := `"` + logoRevisionDigest(key) + `"`
	if httperr.IfNoneMatchHit(r, etag) {
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", logoCacheControl)
		w.WriteHeader(http.StatusNotModified)
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
		slog.WarnContext(r.Context(), "closing company logo reader", "err", closeErr)
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
	// per row, and this short private cache saves the repeated reads of each —
	// the ETag above extends that saving past the cache's own expiry too.
	w.Header().Set("Cache-Control", logoCacheControl)
	w.Header().Set("ETag", etag)
	httperr.StreamObject(w, r, httperr.StreamedObject{
		Download: httperr.Download{ContentType: imagenorm.ContentType, Inline: true, Size: int64(len(logo))},
		Body:     io.NopCloser(bytes.NewReader(logo)),
	}, "company logo "+id.String())
	// A body this small can sit in net/http's own write buffer until the
	// handler returns, so without an explicit flush here the reader would
	// wait on the write-back below before receiving anything they asked for
	// — silently trading their own latency for the NEXT reader's benefit.
	if fErr := http.NewResponseController(w).Flush(); fErr != nil {
		slog.WarnContext(r.Context(), "flushing the company logo response", "err", fErr)
	}
	// Only when a crop actually happened: TrimTransparentPNG returns src
	// itself, unchanged, when the canvas was already tight, and writing
	// identical bytes back would cost a PUT for nothing.
	//
	// Backgrounded, not merely context-detached: logoWriteBackTimeout only
	// bounds a Store whose Put actually watches its context, and the shipped
	// filesystem and in-memory stores both discard theirs (blobstore.Put's own
	// doc comment makes no promise either way). A blocked write on one of
	// those must not hold this handler's goroutine, and with it the reader's
	// connection, open for as long as the disk stays stuck.
	// Coalesced by key: a burst of readers landing on the same untrimmed
	// object right after an upload or a migration would otherwise each start
	// their own write-back of the identical bytes to the identical key. Only
	// the first claims it; the rest find it already in flight and skip —
	// once that one write-back finishes, every later read finds the object
	// already trimmed and TrimTransparentPNG returns src unchanged, so the
	// map never needs more than one entry per key at a time.
	if !bytes.Equal(logo, source) {
		if _, running := h.logoWritesInFlight.LoadOrStore(key, struct{}{}); !running {
			go h.writeBackTrimmedLogo(context.WithoutCancel(r.Context()), companyID, slot, key, logo)
		}
	}
}

// writeBackTrimmedLogo persists a freshly trimmed image over the object a
// caller just read, so every read after this one finds bytes that need no
// further crop.
//
// Runs on its own goroutine (see the call site) with its own timeout-bounded
// context: the response is already on the wire by the time this starts, so an
// unmount or a client that walked away must not cancel a write purely for the
// NEXT reader's benefit. Best-effort — a failure here costs nothing but the
// CPU this read already spent; the object at key still holds the correct, if
// untrimmed, picture, and the next read tries again.
//
// Re-reads the slot's CURRENT key both before and after the write: this read
// may race a replace or an archive whose own cleanup (deleteUnreferencedLogo,
// sitelogoreclaim.go) deletes the object at key, and a write-back landing on
// either side of that delete would resurrect bytes nothing references — the
// exact orphan knowledgeorphan_integration_test.go exists to catch, one
// module over. The before check skips the write outright; the after check
// covers the narrower race where the replace lands WHILE blob.Put is in
// flight, by deleting straight back out what this call just wrote rather
// than leaving it for nothing to ever reference again.
func (h Handlers) writeBackTrimmedLogo(ctx context.Context, companyID ids.CompanyID, slot LogoSlot, key string, logo []byte) {
	defer h.logoWritesInFlight.Delete(key)
	writeCtx, cancel := context.WithTimeout(ctx, logoWriteBackTimeout)
	defer cancel()
	current, err := h.store.CompanyLogoKey(writeCtx, companyID, slot)
	if err != nil {
		slog.WarnContext(ctx, "re-reading the current logo key before write-back", "err", err)
		return
	}
	if current != key {
		return
	}
	if err := h.blob.Put(writeCtx, key, bytes.NewReader(logo), int64(len(logo)), imagenorm.ContentType); err != nil {
		slog.WarnContext(ctx, "writing back a trimmed company logo", "err", err)
		return
	}
	after, err := h.store.CompanyLogoKey(writeCtx, companyID, slot)
	if err != nil {
		slog.WarnContext(ctx, "re-reading the current logo key after write-back", "err", err)
		return
	}
	if after == key {
		return
	}
	if err := h.blob.Delete(writeCtx, key); err != nil {
		slog.WarnContext(ctx, "collecting a write-back that raced a logo replacement", "err", err)
	}
}
