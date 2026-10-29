// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"html/template"
	"net/http"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const publicRecordPreviewPrefix = "/v1/public/record-previews/"

// The public principal reaches only the card handler, never the authenticated router.
func publicRecordPreviews(s Server, svc *identity.Service) func(http.Handler) http.Handler {
	limit := ratelimit.New("record-previews/per-ip", ratelimit.FailClosed, 60, time.Minute)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, publicRecordPreviewPrefix) {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				httperr.Write(w, r, apperrors.ErrNotFound)
				return
			}
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, publicRecordPreviewPrefix), "/")
			if r.URL.RawPath != "" || len(parts) != 2 || (parts[0] != previewDeals && parts[0] != previewCompanies && parts[0] != previewContacts) {
				httperr.Write(w, r, apperrors.ErrNotFound)
				return
			}
			id, err := ids.Parse(parts[1])
			if err != nil {
				httperr.Write(w, r, httperr.Validation("id", "invalid_id", "Use a valid record identifier."))
				return
			}
			fallback := recordPreview{Title: "Margince", Href: "/#/" + parts[0] + "/" + id.String()}

			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
			if !limit.Allow(httpserver.ClientIP(r)) {
				s.writeRecordPreview(w, r, fallback, apperrors.ErrBudgetExceeded)
				return
			}
			workspace, err := svc.InstallationWorkspace(r.Context())
			if err != nil {
				s.writeRecordPreview(w, r, fallback, err)
				return
			}
			ctx := principal.WithWorkspaceID(r.Context(), workspace.UUID)
			ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:public_record_preview"})
			s.GetPublicRecordPreview(w, r.WithContext(ctx), parts[0], crmcontracts.Id(id))
		})
	}
}

type recordPreview struct {
	Title   string
	Summary string
	Href    string
}

var recordPreviewHTML = template.Must(template.New("record-preview").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>{{if .Summary}}<meta property="og:type" content="website"><meta property="og:site_name" content="Margince">
<meta property="og:title" content="{{.Title}}"><meta property="og:description" content="{{.Summary}}">
<meta name="description" content="{{.Summary}}"><meta name="twitter:card" content="summary">
<meta name="twitter:title" content="{{.Title}}"><meta name="twitter:description" content="{{.Summary}}">
{{end}}<script src="/open-record.js" defer></script></head><body>
<h1>{{.Title}}</h1><p>{{.Summary}}</p><a id="record" href="{{.Href}}">Open in Margince</a>
</body></html>`))

// GetPublicRecordPreview exposes only the card; its destination uses normal session and record gates.
func (s Server) GetPublicRecordPreview(w http.ResponseWriter, r *http.Request, kind string, id crmcontracts.Id) {
	preview, err := s.recordPreview(r.Context(), kind, ids.UUID(id))
	if err != nil {
		preview = recordPreview{Title: "Margince", Href: "/#/" + kind + "/" + id.String()}
	}
	s.writeRecordPreview(w, r, preview, err)
}

func (s Server) writeRecordPreview(w http.ResponseWriter, r *http.Request, preview recordPreview, cause error) {
	status := http.StatusOK
	if cause != nil {
		status = http.StatusInternalServerError
		if fault, ok := httperr.Classify(cause); ok {
			status = fault.Status
		}
		s.log.WarnContext(r.Context(), "record preview unavailable", "err", cause)
	}
	var body bytes.Buffer
	if err := recordPreviewHTML.Execute(&body, preview); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body.Bytes()); err != nil {
		s.log.WarnContext(r.Context(), "record preview response failed", "err", err)
	}
}
