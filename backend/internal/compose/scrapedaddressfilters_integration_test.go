// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestScrapedCompanyAddressesAreFoundByThePublishedLocationFilters(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	addresses := []struct{ name, address string }{
		{"Structured website", "Example Street 1, Tower B, 12345 Example City, Germany"},
		{"Different website", "Example Avenue 2, 75000 Other City, France"},
		{"Ambiguous website", "Website Road 2"},
	}
	var companies []string
	for _, fixture := range addresses {
		row, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: fixture.name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.CompanyKind](ids.UUID(row.Id))
		if err := e.Contacts.ApplyEnrichment(ctx, id, contacts.ApplyColdStartProfileInput{
			SourceURL: "https://location.example.test/imprint",
			Fields: []contacts.ColdStartFieldInput{{
				Field: "registered_address", Value: fixture.address,
				SourceURL: "https://location.example.test/imprint", EvidenceSnippet: fixture.address, Confidence: 0.9,
			}},
		}); err != nil {
			t.Fatal(err)
		}
		companies = append(companies, id.String())
	}
	handler := filterPreviewHandlers{pool: e.Pool, collections: NewCollectionsStore(e.Pool)}
	for _, filter := range []struct{ field, value, want string }{
		{"city", "Example City", companies[0]},
		{"country", "de", companies[0]},
		{"city", "Other City", companies[1]},
		{"country", "FR", companies[1]},
	} {
		body := fmt.Sprintf(`{"resource":"company","filter":{"field":%q,"op":"eq","value":%q}}`, filter.field, filter.value)
		req := httptest.NewRequest(http.MethodPost, "/v1/filters/preview", strings.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.PreviewFilter(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("%s=%s returned %d: %s", filter.field, filter.value, response.Code, response.Body.String())
		}
		var result struct {
			MatchCount int `json:"match_count"`
			Rows       []struct {
				ID string `json:"id"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.MatchCount != 1 || len(result.Rows) != 1 || result.Rows[0].ID != filter.want {
			t.Errorf("%s=%s selected %+v, want only %s", filter.field, filter.value, result, filter.want)
		}
	}
}
