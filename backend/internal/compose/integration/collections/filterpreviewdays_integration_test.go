// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

import (
	"io"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// On the wire a date column is its calendar day and a timestamp an instant.
// So the client formats each without guessing which one it holds.
func TestAFilterPreviewSendsADateAsItsDayAndATimestampAsAnInstant(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithSchemaPool(integration.SchemaPool(t)))
	e.BootstrapWorkspace(t)
	var field integration.AnyMap
	if status := e.Call(t, "POST", "/v1/custom-fields", integration.AnyMap{
		"object": "contact", "label": "Renewal Day", "type": "date", "source": "manual",
	}, nil, &field); status != http.StatusCreated {
		t.Fatalf("create custom field: status=%d body=%v", status, field)
	}
	column, named := field["column_name"].(string)
	if !named || column == "" {
		t.Fatalf("created field carries no column_name: %v", field)
	}
	if status := e.Call(t, "POST", "/v1/contacts", integration.AnyMap{
		"linkedin_url": apptest.ContactProfile(),
		"full_name":    "Day Subject", "source": "manual", column: "2025-12-01",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("create contact: status=%d", status)
	}

	var got previewBody
	if status := e.Call(t, "POST", "/v1/filters/preview", integration.AnyMap{
		"resource": "contact",
		"filter":   integration.AnyMap{"field": "name", "op": "contains", "value": "Day Subject"},
	}, nil, &got); status != http.StatusOK || len(got.Rows) != 1 {
		t.Fatalf("preview: status=%d rows=%d, want 200 and the one contact", status, len(got.Rows))
	}
	if day := got.Rows[0][column]; day != "2025-12-01" {
		t.Errorf("%s went out as %#v, want the day 2025-12-01", column, day)
	}
	created, isText := got.Rows[0]["created_at"].(string)
	if _, err := time.Parse(time.RFC3339Nano, created); !isText || err != nil {
		t.Errorf("created_at went out as %#v, want an RFC 3339 instant", got.Rows[0]["created_at"])
	}
	// The full bundle writes the same column as the same day.
	contacts := integration.BundleEntries(t, bundle(t, e))["contact.csv"]
	if days := integration.CSVColumn(t, contacts, column); !slices.Equal(days, []string{"2025-12-01"}) {
		t.Errorf("the bundle's contact.csv holds %s = %v, want the day 2025-12-01", column, days)
	}
}

func bundle(t *testing.T, e *apptest.AppEnv) []byte {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.TS.URL+"/v1/exports/bundle", nil)
	if err != nil {
		t.Fatal(err)
	}
	//nolint:bodyclose // apptest.CloseBody closes it in the deferred call below, which the checker cannot follow across the helper.
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatalf("bundle request: %v", err)
	}
	defer apptest.CloseBody(t, resp)
	raw, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("bundle → %d, %v; want 200", resp.StatusCode, err)
	}
	return raw
}
