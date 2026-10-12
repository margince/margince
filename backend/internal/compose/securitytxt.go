// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The RFC 9116 contact file. The api serves it because the api holds the
// deployment file it is rendered from; the web tier answers the same path with
// a plain 404, so an ingress that leaves it on the web service publishes no
// file rather than the app shell.

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// securityTxtPath is where RFC 9116 §3 places the file.
const securityTxtPath = "/.well-known/security.txt"

// WithSecurityTxt hands the server the rendered web.security_txt. An empty
// body leaves the route unmounted, so the mux's own 404 answers it.
func WithSecurityTxt(body string) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		s.securityTxt = body
	}
}

// SecurityTxtOption is the api's whole security.txt wiring: it reports a
// configuration problem (an Expires already past, say) at boot and hands the
// rendered document to the server, empty when nothing is configured.
func SecurityTxtOption(w deployconfig.Web, now time.Time, log *slog.Logger) Option {
	WarnSecurityTxt(w, now, log)
	return WithSecurityTxt(w.SecurityTxtBody())
}

// WarnSecurityTxt logs, once at boot, a web.security_txt whose expiry has
// passed or sits further out than RFC 9116 recommends.
func WarnSecurityTxt(w deployconfig.Web, now time.Time, log *slog.Logger) {
	for _, warning := range w.Warnings(now) {
		log.Warn("web configuration: " + warning)
	}
}

// mountSecurityTxt registers the file when the deployment configured one.
func mountSecurityTxt(mux *http.ServeMux, body string) {
	if body == "" {
		return
	}
	mux.HandleFunc("GET "+securityTxtPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, body)
	})
}
