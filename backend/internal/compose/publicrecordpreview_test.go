// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	if err := recordPreviewHTML.Execute(&body, preview); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.String(), `src="/open-record.js"`) || !strings.Contains(body.String(), preview.Href) {
		t.Fatal(body.String())
	}
	if strings.Contains(body.String(), `property="og:title"`) {
		t.Fatal("unavailable card advertises record metadata")
	}
}
