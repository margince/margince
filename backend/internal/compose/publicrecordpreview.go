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

// previewCard is the escaped HTML a link-preview fetcher reads in place of the
// app shell. Record cards and booking pages share it. Each caller names its own
// site and link text, so a record card says Margince and a booking page names
// only the host's company. html/template escapes every field.
type previewCard struct {
	Title   string
	Summary string
	// SiteName fills og:site_name; empty omits it.
	SiteName string
	// Canonical is an absolute URL for rel=canonical and og:url; empty omits both.
	Canonical string
	// Image is an absolute URL for og:image and twitter:image; empty gives a
	// text-only card.
	Image    string
	ImageAlt string
	// Href is where the page's script sends a browser that reached the card.
	Href     string
	LinkText string
}

// recordPreviewCard keeps the record card's Margince branding: the card advertises
// the app the record lives in.
func recordPreviewCard(preview recordPreview) previewCard {
	return previewCard{Title: preview.Title, Summary: preview.Summary, SiteName: "Margince", Href: preview.Href, LinkText: "Open in Margince"}
}

var previewCardHTML = template.Must(template.New("preview-card").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>{{if .Canonical}}<link rel="canonical" href="{{.Canonical}}">{{end}}{{if .Summary}}<meta property="og:type" content="website">{{if .SiteName}}<meta property="og:site_name" content="{{.SiteName}}">{{end}}
<meta property="og:title" content="{{.Title}}"><meta property="og:description" content="{{.Summary}}">{{if .Canonical}}<meta property="og:url" content="{{.Canonical}}">{{end}}
{{if .Image}}<meta property="og:image" content="{{.Image}}"><meta property="og:image:alt" content="{{.ImageAlt}}"><meta name="twitter:image" content="{{.Image}}">
{{end}}<meta name="description" content="{{.Summary}}"><meta name="twitter:card" content="summary">
<meta name="twitter:title" content="{{.Title}}"><meta name="twitter:description" content="{{.Summary}}">
{{end}}<script src="/open-record.js" defer></script></head><body>
<h1>{{.Title}}</h1><p>{{.Summary}}</p><a id="record" href="{{.Href}}">{{.LinkText}}</a>
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
	s.writePreviewCard(w, r, recordPreviewCard(preview), "record", cause)
}

// writePreviewCard answers GET and HEAD with the card. A cause sets the status
// it classifies to; the card still renders, so a browser that reached it
// still gets its link onward.
func (s Server) writePreviewCard(w http.ResponseWriter, r *http.Request, card previewCard, kind string, cause error) {
	status := http.StatusOK
	if cause != nil {
		status = http.StatusInternalServerError
		if fault, ok := httperr.Classify(cause); ok {
			status = fault.Status
		}
		s.log.WarnContext(r.Context(), kind+" preview unavailable", "err", cause)
	}
	var body bytes.Buffer
	if err := previewCardHTML.Execute(&body, card); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body.Bytes()); err != nil {
		s.log.WarnContext(r.Context(), kind+" preview response failed", "err", err)
	}
}
