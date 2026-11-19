// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a web page calls itself, read before a user adds it to a record.
// The title they would otherwise type is then already there.

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/webread"
	"github.com/margince/margince/backend/internal/shared/kernel/documenthost"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// LinkPageReader is the slice of *webread.Fetcher a link preview needs.
type LinkPageReader interface {
	FetchPage(ctx context.Context, rawURL string) (webread.Page, error)
}

var publicWebPages = sync.OnceValue(func() LinkPageReader { return webread.New() })

// WithLinkPages returns handlers whose link preview reads pages through pages.
func (h Handlers) WithLinkPages(pages LinkPageReader) Handlers {
	h.linkPages = pages
	return h
}

// PreviewAttachmentLink answers the title and site name of the page an address
// names. It writes nothing.
func (h Handlers) PreviewAttachmentLink(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.PreviewAttachmentLinkJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	pages := h.linkPages
	if pages == nil {
		pages = publicWebPages()
	}
	preview, err := previewLink(r.Context(), pages, req.Url)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, preview)
}

// previewLink reads the page once. A page that cannot be read is an answer,
// not a failure: the user can still add the link and type its title.
func previewLink(ctx context.Context, pages LinkPageReader, raw string) (crmcontracts.AttachmentLinkPreview, error) {
	address, err := LinkAddress(raw)
	if err != nil {
		return crmcontracts.AttachmentLinkPreview{}, err
	}
	out := crmcontracts.AttachmentLinkPreview{Url: address, Host: linkHostOf(address)}
	if documenthost.Is(out.Host) {
		return out, nil
	}
	page, err := pages.FetchPage(ctx, address)
	if err != nil {
		slog.DebugContext(ctx, "link preview: page not read", "host", out.Host, "err", err)
		return out, nil
	}
	out.Read = true
	if title := textcut.Runes(page.Title, maxLinkTitleRunes); title != "" {
		out.Title = &title
	}
	if page.SiteName != "" {
		out.SiteName = &page.SiteName
	}
	return out, nil
}
