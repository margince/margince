// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

import (
	"net/http"
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
	column, _ := field["column_name"].(string)
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
}
