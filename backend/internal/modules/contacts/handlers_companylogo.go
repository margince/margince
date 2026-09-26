// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

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

// logoCacheControl is the cache every logo response carries — 200 and 304
// alike, so a client revalidating gets the same freshness window as one that
// fetched fresh bytes.
//
// `immutable` because the URL already NAMES these bytes: LogoURL bakes a digest
// of the object key into its query token, the key is minted fresh per upload,
// and a replacement therefore takes a different URL. There is no version of
// this URL that can go stale, so a browser has nothing to revalidate — which is
// the whole cost on a list. Twenty-five rows are twenty-five image URLs, a
// browser runs about six at a time against one host, and past a five-minute
// window every one of them became a 304 round trip the reader waits through.
// Now a second visit to the list, and the return from any record on it, fetches
// none of them.
//
// Bounded at a day rather than the conventional year: `private` already keeps
// this in one reader's own browser, but a logo they were shown before their
// access was withdrawn should not be renderable from that cache indefinitely.
// A day costs nothing — every repeat within a working session is already free.
const logoCacheControl = "private, max-age=86400, immutable"

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
	// Decided BEFORE the object is opened: a legacy reader that opened the
	// padded bytes must not see the key turn tight under a concurrent
	// write-back and then stream what it holds as the trimmed revision.
	tight := storedTrimmed(key) || h.tightLogos.has(key)
	rc, object, err := h.blob.Get(r.Context(), key)
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
	// Bytes known to be tight go out as stored: no read into memory, no
	// decode, no pixel scan. That is every mark stored since PutLogo, and a
	// legacy one once this process has checked it.
	if tight {
		writeLogo(w, r, id, etag, rc, object.Size)
		return
	}
	h.streamLegacyLogo(w, r, companyID, id, slot, key, etag, rc)
}

// streamLegacyLogo serves a mark stored before PutLogo trimmed at write time:
// such an object may still carry the transparent square canvas older uploads
// were given, so it is cropped here and the crop written back.
func (h Handlers) streamLegacyLogo(w http.ResponseWriter, r *http.Request, companyID ids.CompanyID, id crmcontracts.Id, slot LogoSlot, key, etag string, rc io.ReadCloser) {
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
	writeLogo(w, r, id, etag, io.NopCloser(bytes.NewReader(logo)), int64(len(logo)))
	// A body this small can sit in net/http's own write buffer until the
	// handler returns, so without an explicit flush here the reader would
	// wait on the write-back below before receiving anything they asked for
	// — silently trading their own latency for the NEXT reader's benefit.
	if fErr := http.NewResponseController(w).Flush(); fErr != nil {
		slog.WarnContext(r.Context(), "flushing the company logo response", "err", fErr)
	}
	// TrimTransparentPNG returns src itself when the canvas was already tight:
	// nothing to write back, and nothing to decode on this key again.
	if bytes.Equal(logo, source) {
		h.tightLogos.add(key)
		return
	}
	// Backgrounded, not merely context-detached: logoWriteBackTimeout only
	// bounds a Store whose Put actually watches its context, and the shipped
	// filesystem and in-memory stores both discard theirs (blobstore.Put's own
	// doc comment makes no promise either way). A blocked write on one of
	// those must not hold this handler's goroutine, and with it the reader's
	// connection, open for as long as the disk stays stuck.
	// Coalesced by key: a burst of readers landing on the same untrimmed
	// object right after an upload or a migration would otherwise each start
	// their own write-back of the identical bytes to the identical key. Only
	// the first claims it; the rest find it already in flight and skip.
	if _, running := h.logoWritesInFlight.LoadOrStore(key, struct{}{}); !running {
		go h.writeBackTrimmedLogo(context.WithoutCancel(r.Context()), companyID, slot, key, logo)
	}
}

// writeLogo sends one mark's bytes under the headers every logo response
// carries, whichever path produced them.
func writeLogo(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, etag string, body io.ReadCloser, size int64) {
	// These bytes were normalized from a third-party website's asset, and three
	// things keep that from mattering at the response. The media type is fixed
	// rather than read back from the object's metadata — the contract declares
	// this endpoint image/png and every stored object is this server's own PNG
	// re-encode, so nothing a site influenced decides how its bytes are
	// interpreted. Then the type cannot be sniffed into something active, and
	// the document that renders can reach nothing.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", logoCacheControl)
	w.Header().Set("ETag", etag)
	httperr.StreamObject(w, r, httperr.StreamedObject{
		Download: httperr.Download{ContentType: imagenorm.ContentType, Inline: true, Size: size},
		Body:     body,
	}, "company logo "+id.String())
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
		h.tightLogos.add(key)
		return
	}
	if err := h.blob.Delete(writeCtx, key); err != nil {
		slog.WarnContext(ctx, "collecting a write-back that raced a logo replacement", "err", err)
	}
}
