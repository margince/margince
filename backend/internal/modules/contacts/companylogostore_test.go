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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Refused at the trim, which comes before the intent is recorded and before the
// bytes are put: the error is the decode's own, and the store holds nothing.
func TestPutLogoRefusesBytesThatAreNotAnImageAndStoresNothing(t *testing.T) {
	blob := blobstore.NewMemory()
	ctx := principal.SystemActing(context.Background(), "deepread")
	const base = "ws/company_logo/c/2"
	key, err := new(Store).PutLogo(ctx, blob, base, []byte("not a png"))
	if !errors.Is(err, imagenorm.ErrUnsupported) {
		t.Fatalf("PutLogo answered %v, want the bytes refused as no image", err)
	}
	if key != "" {
		t.Fatalf("a refused trim answered key %q, want none: nothing was written to collect", key)
	}
	if _, _, err := blob.Get(ctx, base+trimmedLogoSuffix); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("a refused trim left an object behind: %v", err)
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
