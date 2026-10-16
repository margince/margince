// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

func TestAnSVGWithLiveTextIsRefusedAndTheCurrentMarkStays(t *testing.T) {
	wordmark := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 50">` +
		`<circle cx="25" cy="25" r="20"/><text x="50" y="30">Acme</text></svg>`)
	slots := []struct {
		name   string
		path   string
		slot   contacts.LogoSlot
		upload func(companyHandlers, http.ResponseWriter, *http.Request)
	}{
		{"wide logo", "/v1/company/logo", contacts.LogoWide, companyHandlers.UploadAnchorCompanyLogo},
		{"square icon", "/v1/company/logo/icon", contacts.LogoIcon, companyHandlers.UploadAnchorCompanyLogoIcon},
	}
	for _, c := range slots {
		t.Run(c.name, func(t *testing.T) {
			e := integration.Setup(t)
			handlers := companyHandlers{store: e.Contacts, blob: blobstore.NewMemory()}
			company := theCompanyExists(t, e)
			ctx := e.As(e.Rep1, nil, integration.AdminPerms)

			uploadMark(t, e, handlers, logoFixture(t, 400, 400), "acme-logo.png")
			uploadIcon(t, e, handlers, logoFixture(t, 256, 256), "acme-badge.png")
			before, err := e.Contacts.CompanyLogoKey(ctx, company.CompanyID, c.slot)
			if err != nil {
				t.Fatalf("reading the uploaded mark: %v", err)
			}

			body, contentType := markUpload(t, wordmark, "acme.svg")
			request := httptest.NewRequest(http.MethodPost, c.path, body).WithContext(ctx)
			request.Header.Set("Content-Type", contentType)
			recorder := httptest.NewRecorder()
			c.upload(handlers, recorder, request)

			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("upload → %d %s, want 422", recorder.Code, recorder.Body.String())
			}
			var problem struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decoding the problem: %v", err)
			}
			if problem.Code != "svg_text_not_outlined" {
				t.Fatalf("problem code = %q, want svg_text_not_outlined", problem.Code)
			}
			after, err := e.Contacts.CompanyLogoKey(ctx, company.CompanyID, c.slot)
			if err != nil || after != before {
				t.Fatalf("the mark is %q (err %v) after a refused upload, want %q kept", after, err, before)
			}
		})
	}
}
