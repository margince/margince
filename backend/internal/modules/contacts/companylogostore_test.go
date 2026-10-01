// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A store with no database proves the refusal comes before the intent is
// recorded as well as before the bytes are put: reaching either would panic.
func TestPutLogoRefusesBytesThatAreNotAnImageAndStoresNothing(t *testing.T) {
	blob := blobstore.NewMemory()
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "agent:deepread"})
	key, err := new(Store).PutLogo(ctx, blob, "ws/company_logo/c/2", []byte("not a png"))
	if err == nil {
		t.Fatalf("PutLogo stored undecodable bytes at %q", key)
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("PutLogo refused the caller, not the bytes: %v", err)
	}
	if key != "" {
		t.Fatalf("a refused trim answered key %q, want none: nothing was written to collect", key)
	}
}

func TestALegacyMarkLargerThanAnyLogoWriterStoresIsRefusedNotBuffered(t *testing.T) {
	blob := blobstore.NewMemory()
	const key = "ws/company_logo/c/legacy.png"
	oversized := bytes.Repeat([]byte{0}, MaxLogoBytes+1)
	if err := blob.Put(context.Background(), key, bytes.NewReader(oversized), int64(len(oversized)), imagenorm.ContentType); err != nil {
		t.Fatalf("seeding the oversized object: %v", err)
	}
	handlers := NewHandlers(nil).WithBlobstore(blob)
	id := crmcontracts.Id(ids.NewV7())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/companies/"+ids.UUID(id).String()+"/logo", nil)
	handlers.streamLogoKey(rec, req, id, LogoWide, key, "GetCompanyLogo", logoCacheControl, false)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("GET an oversized legacy logo = %d, want 500", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got == imagenorm.ContentType {
		t.Fatal("an oversized object was answered as a logo")
	}
}

func TestTightLogoKeysEmptiesRatherThanGrowPastItsCap(t *testing.T) {
	tight := newTightLogoKeys()
	for i := range tightLogoKeysCap {
		tight.add("legacy/" + strconv.Itoa(i))
	}
	if !tight.has("legacy/0") {
		t.Fatal("a key added below the cap was forgotten")
	}
	tight.add("legacy/one-more")
	if tight.has("legacy/0") {
		t.Fatal("the set kept growing past its cap")
	}
	if !tight.has("legacy/one-more") {
		t.Fatal("the key that hit the cap was not kept after the reset")
	}
}
