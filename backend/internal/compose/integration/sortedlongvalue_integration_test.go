// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A cursor travels in a URL, so it must stay small however long the last row's
// sort value is, and the walk must still reach every row once.
func TestASortedListPagesPastARowWithAVeryLongSortValue(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithSchemaPool(SchemaPool(t)))
	apptest.BootstrapWorkspaceSession(t, e, "Long Sort", "admin@longsort.test", "Admin")
	stamp := "zzlongsort"
	shared := stamp + strings.Repeat("d", 300)
	names := []string{stamp + strings.Repeat("a", 20000), stamp + "b", shared + "1", shared + "2"}
	for _, name := range names {
		if status := e.Call(t, "POST", "/v1/companies", AnyMap{"display_name": name, "source": "manual"}, nil, nil); status != http.StatusCreated {
			t.Fatalf("create company → %d", status)
		}
	}

	var seen []string
	cursor := ""
	for page := range 8 {
		query := url.Values{"q": {stamp}, "sort": {"display_name"}, "limit": {"1"}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var body struct {
			Data []struct {
				DisplayName string `json:"display_name"`
			} `json:"data"`
			Page struct {
				HasMore    bool   `json:"has_more"`
				NextCursor string `json:"next_cursor"`
			} `json:"page"`
		}
		if status := e.Call(t, "GET", "/v1/companies?"+query.Encode(), nil, nil, &body); status != http.StatusOK {
			t.Fatalf("page %d → %d", page+1, status)
		}
		for _, row := range body.Data {
			seen = append(seen, row.DisplayName)
		}
		if len(body.Page.NextCursor) > 2048 {
			t.Fatalf("page %d minted a %d-character cursor; it must stay small enough for a URL", page+1, len(body.Page.NextCursor))
		}
		if !body.Page.HasMore {
			break
		}
		cursor = body.Page.NextCursor
	}
	if len(seen) != len(names) {
		t.Fatalf("the walk reached %d rows, want all %d", len(seen), len(names))
	}
	// Names sharing a prefix longer than the key still come once each.
	unique := map[string]bool{}
	for _, name := range seen {
		unique[name] = true
	}
	if len(unique) != len(names) {
		t.Fatalf("the walk repeated a row: %d distinct of %d", len(unique), len(seen))
	}
}
