// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"bytes"
	"context"
	"html/template"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// RefusalPage is what a browser tab reads in place of a 404 or 403 problem.
// Its words come already in the reader's language.
type RefusalPage struct {
	Lang     string
	Title    string
	Sentence string
}

type refusalPageKey struct{}

// WithRefusalPage makes a 404 or 403 on this request answer page to a caller
// that asks for HTML. Every other answer stays a problem.
func WithRefusalPage(ctx context.Context, page RefusalPage) context.Context {
	return context.WithValue(ctx, refusalPageKey{}, page)
}

var refusalPageHTML = template.Must(template.New("refusal-page").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>{{.Title}}</title>
<style>body{font:16px/1.5 system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;color:#1f2933;background:#fff}
@media (prefers-color-scheme:dark){body{color:#e4e7eb;background:#1f2933}}</style>
</head><body><h1>{{.Title}}</h1><p>{{.Sentence}}</p></body></html>`))

func writeRefusalPage(w http.ResponseWriter, r *http.Request, status int) bool {
	page, ok := r.Context().Value(refusalPageKey{}).(RefusalPage)
	if !ok || (status != http.StatusNotFound && status != http.StatusForbidden) || !prefersHTML(r.Header.Get("Accept")) {
		return false
	}
	var body bytes.Buffer
	if err := refusalPageHTML.Execute(&body, page); err != nil {
		slog.ErrorContext(r.Context(), "rendering a refusal page; answering the problem instead", "err", err)
		return false
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	//craft:ignore swallowed-errors the status line is already on the wire, so a failed write (the client left) has no channel back
	_, _ = w.Write(body.Bytes())
	return true
}

// prefersHTML matches a browser opening a link in a tab: text/html ranked at
// least as high as any JSON type. A script's bare */* names neither.
func prefersHTML(accept string) bool {
	html, json := 0.0, 0.0
	for _, mediaRange := range strings.Split(accept, ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(mediaRange))
		if err != nil {
			continue
		}
		quality := 1.0
		if raw, named := params["q"]; named {
			// RFC 9110 makes a malformed weight invalid, so it counts for nothing.
			if quality, err = strconv.ParseFloat(raw, 64); err != nil {
				quality = 0
			}
		}
		switch {
		case mediaType == "text/html":
			html = max(html, quality)
		case strings.HasSuffix(mediaType, "/json") || strings.HasSuffix(mediaType, "+json"):
			json = max(json, quality)
		}
	}
	return html > 0 && html >= json
}
