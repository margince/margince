// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// GET /exports/bundle — the whole-workspace export bundle's HTTP surface.
//
// The general export-run lifecycle (IEM-WIRE-1/2 — enqueue, poll, fetch from
// the blob store) is the import-export-migration chapter's own unminted
// contract surface, and inventing it here would pre-empt that extension. This
// op streams the bundle inline rather than staging it, gated on the
// installation-wide admin grant (installation_settings UPDATE, admin/ops).

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type exportBundleHandlers struct {
	writer *ExportWriter
	log    *slog.Logger
}

func newExportBundleHandlers(pool *pgxpool.Pool, log *slog.Logger) exportBundleHandlers {
	return exportBundleHandlers{writer: NewExportWriter(pool), log: log}
}

// DownloadExportBundle streams the bundle straight to the response: the
// estate can be large (it carries the whole audit log), and buffering it in
// the API process would put the workspace's size between a caller and every
// other request this process is serving.
//
// Streaming costs the ability to answer a problem document once the
// first byte is out. That trade is deliberate and the failure is still
// honest: the gate runs BEFORE any header is written, so a refusal is a
// clean 4xx, and a mid-stream failure aborts the connection — the
// client sees a truncated, invalid zip rather than a 200 that claims a
// completeness it does not have. It is logged server-side; writing a
// problem body after the body has begun would only corrupt the archive
// further.
func (h exportBundleHandlers) DownloadExportBundle(w http.ResponseWriter, r *http.Request) {
	if h.writer == nil {
		httperr.NotImplemented(w, r, "downloadExportBundle")
		return
	}
	if err := auth.Require(r.Context(), "installation_settings", principal.ActionUpdate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// The human-only class is enforced here as well as at the transport:
	// this streams the whole estate, audit log included, in one GET, and it
	// should not rest on route-pattern resolution alone.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// Headers before the first byte: the bundle streams, so this is the last
	// moment a header is settable. Size is unknown — the archive is written as
	// it is produced — so Content-Length is deliberately omitted.
	httperr.Download{ContentType: "application/zip", Filename: "margince-export.zip"}.WriteHeaders(w)
	body := &startedWriter{ResponseWriter: w}
	_, err := h.writer.WriteBundle(r.Context(), body)
	switch {
	case err == nil:
	case !body.started:
		// Nothing is on the wire yet, so the failure can still be answered.
		httperr.DropSuccessHeaders(w.Header())
		httperr.Write(w, r, err)
	default:
		h.log.ErrorContext(r.Context(), "export bundle: the bundle failed mid-stream; the client's download is truncated", "err", err)
		// A plain return would end the chunked body as if the archive were whole.
		panic(http.ErrAbortHandler)
	}
}

// startedWriter records whether the bundle put a byte on the wire. Before that
// byte a failure can be answered; after it the response must be aborted.
type startedWriter struct {
	http.ResponseWriter
	started bool
}

func (s *startedWriter) Write(chunk []byte) (int, error) {
	s.started = s.started || len(chunk) > 0
	return s.ResponseWriter.Write(chunk)
}
