// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package filepage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func loadContract(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatalf("loading the contract: %v", err)
	}
	return doc
}

// servesAFile reads a 200 that answers neither JSON nor a page of its own.
func servesAFile(contentTypes map[string]bool) bool {
	if len(contentTypes) == 0 {
		return false
	}
	for contentType := range contentTypes {
		if strings.Contains(contentType, "json") || contentType == "text/html" {
			return false
		}
	}
	return true
}

// notOpenedInATab are the GET routes that answer neither JSON nor a page. Only
// a machine calls them, so a refusal page would serve no reader.
var notOpenedInATab = gatekit.Waive(map[string]string{
	httpserver.BaseURL + "/channel-events/whatsapp": "Meta's subscription check, answered with the challenge as plain text; a refusal page there would reach no reader",
})

func TestEveryFileRouteIsOneTheContractServesAsAFile(t *testing.T) {
	doc := loadContract(t)
	served := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		if item.Get == nil {
			continue
		}
		ok := item.Get.Responses.Status(http.StatusOK)
		if ok == nil || ok.Value == nil {
			continue
		}
		contentTypes := map[string]bool{}
		for contentType := range ok.Value.Content {
			contentTypes[contentType] = true
		}
		if servesAFile(contentTypes) {
			served[httpserver.BaseURL+path] = true
		}
	}
	if len(served) == 0 {
		t.Fatal("the contract declares no GET that answers a file, so this reads nothing")
	}
	defer notOpenedInATab.AssertAllMatched(t)
	for route := range served {
		if !fileRoutes[route] && !notOpenedInATab.Waived(t, route) {
			t.Errorf("GET %s answers a file but is not in fileRoutes, so its link opened in a tab reads JSON", route)
		}
	}
	for route := range fileRoutes {
		if !served[route] {
			t.Errorf("fileRoutes names %s, which the contract does not serve as a file", route)
		}
	}
}

func TestEveryFileRouteDeclaresThePageItsRefusalsCanBe(t *testing.T) {
	doc := loadContract(t)
	declared := 0
	for route := range fileRoutes {
		item := doc.Paths.Find(strings.TrimPrefix(route, httpserver.BaseURL))
		if item == nil || item.Get == nil {
			t.Errorf("the contract has no GET %s", route)
			continue
		}
		for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
			refusal := item.Get.Responses.Status(status)
			if refusal == nil || refusal.Value == nil {
				continue
			}
			declared++
			if refusal.Value.Content.Get("text/html") == nil {
				t.Errorf("GET %s declares a %d without text/html, but a browser tab is answered a page", route, status)
			}
		}
	}
	if declared == 0 {
		t.Fatal("no file route declares a 403 or 404, so this read nothing")
	}
}

// goneFileRouter routes the attachment download as the contract router does,
// to a handler that answers the file is gone.
func goneFileRouter() http.Handler {
	router := chi.NewRouter()
	pattern := func(r *http.Request) string { return chi.RouteContext(r.Context()).RoutePattern() }
	router.With(Refusals(pattern)).Get(httpserver.BaseURL+"/attachments/{id}", func(w http.ResponseWriter, r *http.Request) {
		httperr.Write(w, r, apperrors.ErrNotFound)
	})
	return router
}

func openGoneAttachment(accept, language string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/attachments/00000000-0000-0000-0000-000000000001", nil)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", language)
	rec := httptest.NewRecorder()
	goneFileRouter().ServeHTTP(rec, req)
	return rec
}

func TestAGoneFileOpenedInATabReadsASentenceInTheReadersLanguage(t *testing.T) {
	for language, sentence := range map[string]string{
		"en": "This file was deleted or you no longer have access.",
		"de": "Diese Datei wurde gelöscht oder du hast keinen Zugriff mehr darauf.",
		"vi": "Tệp này đã bị xóa hoặc bạn không còn quyền truy cập.",
	} {
		rec := openGoneAttachment("text/html,application/xhtml+xml,*/*;q=0.8", language)
		if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("%s: %d %q, want a 404 page", language, rec.Code, rec.Header().Get("Content-Type"))
		}
		if !strings.Contains(rec.Body.String(), sentence) || !strings.Contains(rec.Body.String(), `lang="`+language+`"`) {
			t.Errorf("%s: the page does not say %q in its language:\n%s", language, sentence, rec.Body.String())
		}
	}
}

func TestAGoneFileAskedForByAClientStaysAProblem(t *testing.T) {
	rec := openGoneAttachment("application/json", "de")
	if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("answered %d %q, want the 404 problem", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(strings.Join(rec.Header().Values("Vary"), ","), "Accept") {
		t.Errorf("Vary = %v, want Accept named, since the answer depends on it", rec.Header().Values("Vary"))
	}
}
