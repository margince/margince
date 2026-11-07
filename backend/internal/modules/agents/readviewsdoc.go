// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// `margince://schema/views`: every view read_view reads and the arguments each
// takes, published as a document rather than recited into the tool's schema.
// Every client holds that listing for a whole session, so it stays small. A
// name outside the catalog is refused with every name that exists, so a caller
// who skipped the document is still told.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// ReadViewsURI is the document's stable identity.
const ReadViewsURI = "margince://schema/views"

// readViewsVersion identifies the document's shape. The views move with the
// contract and are re-read either way.
const readViewsVersion = "1"

// ReadViewsResource publishes the views read_view reads. Its views are derived
// from the table the tool is registered over, so the document and the tool's
// refusal name one catalog.
type ReadViewsResource struct{ views []ReadView }

// NewReadViewsResource binds the document to the views it describes.
func NewReadViewsResource(views []ReadView) ReadViewsResource {
	return ReadViewsResource{views: views}
}

// Resources advertises the one document this provider publishes.
func (ReadViewsResource) Resources(context.Context) []mcp.Resource {
	return []mcp.Resource{{
		URI:   ReadViewsURI,
		Name:  "read_view_views",
		Title: "Views read_view reads",
		// ScopeRead, matching read_view: a caller admitted to the tool is
		// admitted to the catalog it refuses against.
		RequiredScope: principal.ScopeRead,
		MIMEType:      mimeApplicationJSON,
		Description: "Every view read_view reads, with the path arguments it needs and the " +
			"query arguments it takes: each one's JSON type, and whether it is required, with " +
			"its allowed words or format. A view that pages also takes cursor and limit.",
	}}
}

// ReadResource composes the document. An unknown URI answers ErrNotFound,
// matching every other read on this surface.
func (r ReadViewsResource) ReadResource(_ context.Context, uri string) (mcp.ResourceContents, error) {
	if uri != ReadViewsURI {
		return mcp.ResourceContents{}, fmt.Errorf("agents: resource %q: %w", uri, apperrors.ErrNotFound)
	}
	body, err := json.Marshal(readViewsDocument(r.views))
	if err != nil {
		return mcp.ResourceContents{}, fmt.Errorf("agents: rendering the read_view catalog: %w", err)
	}
	return mcp.ResourceContents{URI: uri, MIMEType: mimeApplicationJSON, Text: string(body)}, nil
}

var _ mcp.ResourceProvider = ReadViewsResource{}

type readViewsDoc struct {
	Version string          `json:"version"`
	Views   []readViewEntry `json:"views"`
}

// readViewEntry is one view as a caller names it. Path arguments are required,
// a query argument says whether it is, and a view that pages also takes cursor
// and limit.
type readViewEntry struct {
	View  string      `json:"view"`
	Path  []string    `json:"path"`
	Query []ViewQuery `json:"query"`
}

// readViewsDocument lists the views in name order, so a client that caches the
// document does not read a reshuffle as a change.
func readViewsDocument(views []ReadView) readViewsDoc {
	entries := make([]readViewEntry, 0, len(views))
	for _, v := range views {
		entries = append(entries, readViewEntry{View: v.Name, Path: closedList(v.PathParams), Query: closedList(v.Query)})
	}
	slices.SortFunc(entries, func(a, b readViewEntry) int { return strings.Compare(a.View, b.View) })
	return readViewsDoc{Version: readViewsVersion, Views: entries}
}

// viewNames lists the views' names in order, for a refusal that names them.
func viewNames(views []ReadView) []string {
	names := make([]string, 0, len(views))
	for _, v := range views {
		names = append(names, v.Name)
	}
	slices.Sort(names)
	return names
}
