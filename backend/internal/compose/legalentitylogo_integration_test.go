// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A legal entity's own mark, through the real upload transport: stored as this
// server's PNG, refused on the default entity, and collected when removed.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func entityLogoRequest(t *testing.T, e *integration.Env, method string, id ids.LegalEntityID, body []byte) *http.Request {
	t.Helper()
	path := "/v1/legal-entities/" + id.String() + "/logo"
	if body == nil {
		return httptest.NewRequest(method, path, nil).WithContext(e.As(e.Rep1, nil, integration.AdminPerms))
	}
	form, contentType := markUpload(t, body, "wien.png")
	request := httptest.NewRequest(method, path, form).WithContext(e.As(e.Rep1, nil, integration.AdminPerms))
	request.Header.Set("Content-Type", contentType)
	return request
}

func TestALegalEntityWearsAMarkOfItsOwn(t *testing.T) {
	e := integration.Setup(t)
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `DELETE FROM legal_entity WHERE NOT is_default`); err != nil {
		t.Fatalf("remove earlier entities: %v", err)
	}
	blob := blobstore.NewMemory()
	handlers := companyHandlers{store: e.Contacts, blob: blob}
	admin := e.As(e.Rep1, nil, integration.AdminPerms)
	name, legal, country := "Wien", "Wien GmbH", "at"
	vienna, err := e.Contacts.CreateLegalEntity(admin, contacts.LegalEntityInput{DisplayName: &name, LegalName: &legal, Country: &country})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	recorder := httptest.NewRecorder()
	handlers.UploadLegalEntityLogo(recorder, entityLogoRequest(t, e, http.MethodPost, vienna.ID, logoFixture(t, 300, 100)),
		crmcontracts.Id(vienna.ID.UUID))
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload → %d %s, want 200", recorder.Code, recorder.Body)
	}
	var uploaded crmcontracts.LegalEntity
	if err := json.Unmarshal(recorder.Body.Bytes(), &uploaded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if uploaded.LogoUrl == nil || !strings.HasPrefix(*uploaded.LogoUrl, "/v1/legal-entities/"+vienna.ID.String()+"/logo?v=") {
		t.Fatalf("logo_url = %v, want the entity's own logo endpoint", uploaded.LogoUrl)
	}
	key, err := e.Contacts.LegalEntityLogoKey(admin, vienna.ID)
	if err != nil {
		t.Fatalf("the entity names no key after its upload: %v", err)
	}
	if _, _, err := blob.Get(context.Background(), key); err != nil {
		t.Fatalf("the key names no stored object: %v", err)
	}

	var defaultID ids.LegalEntityID
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `SELECT id FROM legal_entity WHERE is_default`).Scan(&defaultID); err != nil {
		t.Fatalf("read the default entity: %v", err)
	}
	refused := httptest.NewRecorder()
	handlers.UploadLegalEntityLogo(refused, entityLogoRequest(t, e, http.MethodPost, defaultID, logoFixture(t, 300, 100)),
		crmcontracts.Id(defaultID.UUID))
	if refused.Code != http.StatusConflict || !strings.Contains(refused.Body.String(), "legal_entity_field_on_company_form") {
		t.Errorf("an upload to the default entity → %d %s, want 409 legal_entity_field_on_company_form", refused.Code, refused.Body)
	}

	cleared := httptest.NewRecorder()
	handlers.DeleteLegalEntityLogo(cleared, entityLogoRequest(t, e, http.MethodDelete, vienna.ID, nil), crmcontracts.Id(vienna.ID.UUID))
	if cleared.Code != http.StatusOK || strings.Contains(cleared.Body.String(), `"logo_url"`) {
		t.Errorf("removal → %d %s, want 200 with no logo_url", cleared.Code, cleared.Body)
	}
	if _, _, err := blob.Get(context.Background(), key); !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("the removed mark's bytes are still stored (%v); nothing references them", err)
	}
}
