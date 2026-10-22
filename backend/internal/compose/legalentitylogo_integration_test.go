// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A legal entity's own mark, through the real upload transport. It is stored as
// this server's PNG. A write that would be refused stores no bytes. A mark whose
// bytes cannot be deleted is handed to the sweep.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// countingBlob counts what reaches the object store, and can refuse every delete.
type countingBlob struct {
	blobstore.Store
	puts        int
	failDeletes bool
}

func (b *countingBlob) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	b.puts++
	return b.Store.Put(ctx, key, r, size, contentType)
}

func (b *countingBlob) Delete(ctx context.Context, key string) error {
	if b.failDeletes {
		return errors.New("the object store refused the delete")
	}
	return b.Store.Delete(ctx, key)
}

func entityLogoRequest(ctx context.Context, t *testing.T, method string, id ids.LegalEntityID, body []byte) *http.Request {
	t.Helper()
	path := "/v1/legal-entities/" + id.String() + "/logo"
	if body == nil {
		return httptest.NewRequest(method, path, nil).WithContext(ctx)
	}
	form, contentType := markUpload(t, body, "wien.png")
	request := httptest.NewRequest(method, path, form).WithContext(ctx)
	request.Header.Set("Content-Type", contentType)
	return request
}

func describeVienna(t *testing.T, e *integration.Env) contacts.LegalEntity {
	t.Helper()
	vienna, err := e.Contacts.CreateLegalEntity(e.As(e.Rep1, nil, integration.AdminPerms),
		contacts.LegalEntityInput{DisplayName: new("Wien"), LegalName: new("Wien GmbH"), Country: new("at")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return vienna
}

func uploadEntityMark(ctx context.Context, t *testing.T, handlers companyHandlers, id ids.LegalEntityID) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handlers.UploadLegalEntityLogo(recorder, entityLogoRequest(ctx, t, http.MethodPost, id, logoFixture(t, 300, 100)),
		crmcontracts.Id(id.UUID), crmcontracts.UploadLegalEntityLogoParams{})
	return recorder
}

func intentsFor(t *testing.T, fragment string) int {
	t.Helper()
	var count int
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM stored_object_intent WHERE storage_key LIKE '%' || $1 || '%'`, fragment).Scan(&count); err != nil {
		t.Fatalf("count intents: %v", err)
	}
	return count
}

func TestALegalEntityWearsAMarkOfItsOwn(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	handlers := companyHandlers{store: e.Contacts, blob: blob}
	admin := e.As(e.Rep1, nil, integration.AdminPerms)
	vienna := describeVienna(t, e)

	recorder := uploadEntityMark(admin, t, handlers, vienna.ID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload → %d %s, want 200", recorder.Code, recorder.Body)
	}
	var uploaded crmcontracts.LegalEntity
	if err := json.Unmarshal(recorder.Body.Bytes(), &uploaded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if uploaded.LogoUrl == nil || !strings.HasPrefix(*uploaded.LogoUrl, "/v1/legal-entities/"+vienna.ID.String()+"/logo/image?v=") {
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
	refused := uploadEntityMark(admin, t, handlers, defaultID)
	if refused.Code != http.StatusConflict || !strings.Contains(refused.Body.String(), "legal_entity_field_on_company_form") {
		t.Errorf("an upload to the default entity → %d %s, want 409 legal_entity_field_on_company_form", refused.Code, refused.Body)
	}

	cleared := httptest.NewRecorder()
	handlers.DeleteLegalEntityLogo(cleared, entityLogoRequest(admin, t, http.MethodDelete, vienna.ID, nil),
		crmcontracts.Id(vienna.ID.UUID), crmcontracts.DeleteLegalEntityLogoParams{})
	if cleared.Code != http.StatusOK || strings.Contains(cleared.Body.String(), `"logo_url"`) {
		t.Errorf("removal → %d %s, want 200 with no logo_url", cleared.Code, cleared.Body)
	}
	if _, _, err := blob.Get(context.Background(), key); !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("the removed mark's bytes are still stored (%v); nothing references them", err)
	}
}

// A caller the write would refuse is refused before the upload is read, so no
// bytes are stored and no intent is recorded for them.
func TestARefusedEntityLogoUploadStoresNothing(t *testing.T) {
	e := integration.Setup(t)
	blob := &countingBlob{Store: blobstore.NewMemory()}
	handlers := companyHandlers{store: e.Contacts, blob: blob}
	vienna := describeVienna(t, e)
	reader := integration.AdminPerms
	reader.Objects = maps.Clone(integration.AdminPerms.Objects)
	reader.Objects["legal_entity"] = principal.ObjectGrant{Read: true}

	denied := uploadEntityMark(e.As(e.Rep1, nil, reader), t, handlers, vienna.ID)
	if denied.Code != http.StatusForbidden {
		t.Errorf("a reader's upload → %d %s, want 403", denied.Code, denied.Body)
	}
	stale := entityLogoRequest(e.As(e.Rep1, nil, integration.AdminPerms), t, http.MethodPost, vienna.ID, logoFixture(t, 300, 100))
	stale.Header.Set("If-Match", "999")
	skewed := httptest.NewRecorder()
	handlers.UploadLegalEntityLogo(skewed, stale, crmcontracts.Id(vienna.ID.UUID), crmcontracts.UploadLegalEntityLogoParams{})
	if skewed.Code != http.StatusConflict || !strings.Contains(skewed.Body.String(), "version_skew") {
		t.Errorf("an upload at a stale version → %d %s, want 409 version_skew", skewed.Code, skewed.Body)
	}
	if blob.puts != 0 || intentsFor(t, "legal-entity/"+vienna.ID.String()) != 0 {
		t.Errorf("refused uploads stored %d objects and %d intents, want none",
			blob.puts, intentsFor(t, "legal-entity/"+vienna.ID.String()))
	}
}

// The key a removal drops is provisional in the ledger, so a delete the object
// store refuses leaves bytes the sweep still collects.
func TestARemovedEntityLogoIsLeftForTheSweepWhenItsDeleteFails(t *testing.T) {
	e := integration.Setup(t)
	blob := &countingBlob{Store: blobstore.NewMemory()}
	handlers := companyHandlers{store: e.Contacts, blob: blob}
	admin := e.As(e.Rep1, nil, integration.AdminPerms)
	vienna := describeVienna(t, e)
	if recorder := uploadEntityMark(admin, t, handlers, vienna.ID); recorder.Code != http.StatusOK {
		t.Fatalf("upload → %d %s", recorder.Code, recorder.Body)
	}
	key, err := e.Contacts.LegalEntityLogoKey(admin, vienna.ID)
	if err != nil {
		t.Fatalf("read the key: %v", err)
	}
	blob.failDeletes = true
	cleared := httptest.NewRecorder()
	handlers.DeleteLegalEntityLogo(cleared, entityLogoRequest(admin, t, http.MethodDelete, vienna.ID, nil),
		crmcontracts.Id(vienna.ID.UUID), crmcontracts.DeleteLegalEntityLogoParams{})
	if cleared.Code != http.StatusOK {
		t.Fatalf("removal → %d %s, want 200", cleared.Code, cleared.Body)
	}
	if _, _, err := blob.Get(context.Background(), key); err != nil {
		t.Fatalf("the failed delete removed the bytes after all: %v", err)
	}
	if intentsFor(t, key) != 1 {
		t.Errorf("the dropped key has %d intents, want 1 so the sweep collects its bytes", intentsFor(t, key))
	}
}
