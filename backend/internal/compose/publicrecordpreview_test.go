// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
)

func TestPublicRecordPreviewRejectsPathsBeforeBindingAPrincipal(t *testing.T) {
	for _, path := range []string{
		"deals/not-a-uuid", "deals/01a00000-0000-7000-8000-000000000001/notes",
		"users/01a00000-0000-7000-8000-000000000001", "deals/%30%31a00000-0000-7000-8000-000000000001",
	} {
		t.Run(path, func(t *testing.T) {
			handler := publicRecordPreviews(Server{}, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("preview reached the authenticated router") }))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, publicRecordPreviewPrefix+path, nil))
			expected := http.StatusNotFound
			if path == "deals/not-a-uuid" {
				expected = http.StatusUnprocessableEntity
			}
			if response.Code != expected {
				t.Fatalf("status: %d", response.Code)
			}
		})
	}
}

func TestUnavailableRecordPreviewStillOpensProtectedApp(t *testing.T) {
	preview := recordPreview{Title: "Margince", Href: "/#/deals/01a00000-0000-7000-8000-000000000001"}
	var body strings.Builder
	if err := previewCardHTML.Execute(&body, recordPreviewCard(preview)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.String(), `src="/open-record.js"`) || !strings.Contains(body.String(), preview.Href) {
		t.Fatal(body.String())
	}
	if strings.Contains(body.String(), `property="og:title"`) {
		t.Fatal("unavailable card advertises record metadata")
	}
}

// The card prints a deal field only when it carries the installation's masks
// and none of them withholds that field.
func TestACardWithholdsWhatAnyRoleMasksAndEverythingWithoutTheMasks(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		ctx      context.Context
		withheld bool
	}{
		"masks unknown":     {ctx: context.Background(), withheld: true},
		"no role masks it":  {ctx: withCardWithheld(context.Background(), nil), withheld: false},
		"a role masks it":   {ctx: withCardWithheld(context.Background(), []string{fieldmask.DealCompanyID}), withheld: true},
		"another is masked": {ctx: withCardWithheld(context.Background(), []string{fieldmask.DealAmountMinor}), withheld: false},
	} {
		if got := cardWithholds(tc.ctx, fieldmask.DealCompanyID); got != tc.withheld {
			t.Errorf("%s: cardWithholds(company_id) = %v, want %v", name, got, tc.withheld)
		}
	}
}
