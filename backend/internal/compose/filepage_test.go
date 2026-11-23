// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

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
	for route := range served {
		if !fileRoutes[route] {
			t.Errorf("GET %s answers a file but is not in fileRoutes, so its link opened in a tab reads JSON", route)
		}
	}
	for route := range fileRoutes {
		if !served[route] {
			t.Errorf("fileRoutes names %s, which the contract does not serve as a file", route)
		}
	}
}

type goneAttachment struct{ crmcontracts.Unimplemented }

func (goneAttachment) DownloadAttachment(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	httperr.Write(w, r, apperrors.ErrNotFound)
}

func openGoneAttachment(accept, language string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/attachments/00000000-0000-0000-0000-000000000001", nil)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", language)
	rec := httptest.NewRecorder()
	contractRouter(goneAttachment{}).ServeHTTP(rec, req)
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
	if rec.Code != http.StatusNotFound || problemCode(t, rec) != "not_found" {
		t.Fatalf("answered %d %s, want the 404 problem", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.Join(rec.Header().Values("Vary"), ","), "Accept") {
		t.Errorf("Vary = %v, want Accept named, since the answer depends on it", rec.Header().Values("Vary"))
	}
}
