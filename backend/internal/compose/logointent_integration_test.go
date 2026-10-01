// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A company mark's bytes are provisional from before they are stored until the
// row that names them commits. Every case drives a real writer and reads the
// ledger the reap reads, because a mark whose intent outlives its row is
// deleted from under the record wearing it, and one whose row never arrives
// without an intent is bytes nothing can find again.

import (
	"context"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// provisional reports whether the ledger still holds key.
func provisional(t *testing.T, e *integration.Env, key string) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM stored_object_intent WHERE storage_key = $1`, key) > 0
}

// intentWitnessBlobstore notes, at each Put, whether the key was already
// provisional: the order is the invariant, and the end state cannot show it.
type intentWitnessBlobstore struct {
	blobstore.Store
	t             *testing.T
	e             *integration.Env
	recordedAtPut map[string]bool
	// afterPut runs once the bytes are stored, for a case that fails what follows.
	afterPut func()
}

func newIntentWitnessBlobstore(t *testing.T, e *integration.Env) *intentWitnessBlobstore {
	return &intentWitnessBlobstore{Store: blobstore.NewMemory(), t: t, e: e, recordedAtPut: map[string]bool{}}
}

func (b *intentWitnessBlobstore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	b.recordedAtPut[key] = provisional(b.t, b.e, key)
	if err := b.Store.Put(ctx, key, r, size, contentType); err != nil {
		return err
	}
	if b.afterPut != nil {
		b.afterPut()
	}
	return nil
}

func (b *intentWitnessBlobstore) requireRecordedBeforePut(t *testing.T, key string) {
	t.Helper()
	recorded, put := b.recordedAtPut[key]
	if !put {
		t.Fatalf("nothing was stored at %q", key)
	}
	if !recorded {
		t.Fatalf("the bytes at %q were stored before their key was recorded provisional", key)
	}
}

func TestAnUploadedMarkIsProvisionalUntilTheCompanyWearsIt(t *testing.T) {
	e := integration.Setup(t)
	blob := newIntentWitnessBlobstore(t, e)
	company := theCompanyExists(t, e)

	uploadMark(t, e, companyHandlers{store: e.Contacts, blob: blob}, logoFixture(t, 64, 64), "acme.png")

	key, err := e.Contacts.CompanyLogoKey(e.As(e.Rep1, nil, integration.AdminPerms), company.CompanyID, contacts.LogoWide)
	if err != nil {
		t.Fatalf("the company wears no mark after its own upload: %v", err)
	}
	blob.requireRecordedBeforePut(t, key)
	if provisional(t, e, key) {
		t.Fatalf("the mark the company wears at %q is still provisional; the reap would delete it", key)
	}
}

func TestAnUploadWhoseLogoWriteFailsLeavesItsBytesForTheReap(t *testing.T) {
	e := integration.Setup(t)
	blob := newIntentWitnessBlobstore(t, e)
	theCompanyExists(t, e)
	// The request is abandoned between the put and the write that would name
	// the bytes: the write fails, and the handler keeps what it stored.
	ctx, abandon := context.WithCancel(e.As(e.Rep1, nil, integration.AdminPerms))
	defer abandon()
	blob.afterPut = abandon

	body, contentType := markUpload(t, logoFixture(t, 64, 64), "acme.png")
	request := httptest.NewRequest(http.MethodPost, "/v1/company/logo", body).WithContext(ctx)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	companyHandlers{store: e.Contacts, blob: blob}.UploadAnchorCompanyLogo(recorder, request)
	if recorder.Code == http.StatusOK {
		t.Fatal("an upload whose logo write was abandoned answered 200")
	}

	if len(blob.recordedAtPut) != 1 {
		t.Fatalf("%d objects stored, want the one upload", len(blob.recordedAtPut))
	}
	for key := range blob.recordedAtPut {
		blob.requireRecordedBeforePut(t, key)
		if !provisional(t, e, key) {
			t.Fatalf("the bytes at %q no row names are not provisional; nothing could ever collect them", key)
		}
	}
}

func TestAnUploadTheCallerMayNotWriteStoresNoBytes(t *testing.T) {
	e := integration.Setup(t)
	blob := newIntentWitnessBlobstore(t, e)
	theCompanyExists(t, e)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["company"] = principal.ObjectGrant{Read: true}

	body, contentType := markUpload(t, logoFixture(t, 64, 64), "acme.png")
	request := httptest.NewRequest(http.MethodPost, "/v1/company/logo", body).WithContext(e.As(e.Rep1, nil, perms))
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	companyHandlers{store: e.Contacts, blob: blob}.UploadAnchorCompanyLogo(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("upload without update rights → %d %s, want 403", recorder.Code, recorder.Body.String())
	}
	if len(blob.recordedAtPut) != 0 {
		t.Fatalf("a refused upload stored %d object(s), want none", len(blob.recordedAtPut))
	}
}

func TestAResolvedMarkIsProvisionalUntilTheCompanyWearsIt(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	company := theCompanyExists(t, e)
	blob := newIntentWitnessBlobstore(t, e)
	base := companyLogoKey(ids.From[ids.WorkspaceKind](e.WS), company.CompanyID)

	key, err := e.Contacts.PutLogo(ctx, blob, base, logoFixture(t, 64, 64))
	if err != nil {
		t.Fatalf("store the resolved mark: %v", err)
	}
	blob.requireRecordedBeforePut(t, key)
	if written, _, err := e.Contacts.SetCompanyLogo(ctx, company.CompanyID, key, seedURL+"/touch.png"); err != nil || !written {
		t.Fatalf("SetCompanyLogo = %v, %v; want the mark written", written, err)
	}
	if provisional(t, e, key) {
		t.Fatalf("the mark the company wears at %q is still provisional; the reap would delete it", key)
	}
}

func TestAMarkTheDossierParksIsNoLongerProvisional(t *testing.T) {
	e := integration.Setup(t)
	site := &assetSite{assets: map[string][]byte{touchIconURL: logoFixture(t, 512, 512)}}
	blob := newIntentWitnessBlobstore(t, e)
	args := readTheOnboardingSite(t, e, onboardingLogoWorker(e, site, blob))

	parked, _ := parkedLogo(t, e, args.SiteReadID)
	if parked == nil || *parked == "" {
		t.Fatal("the dossier parked no mark")
	}
	blob.requireRecordedBeforePut(t, *parked)
	if provisional(t, e, *parked) {
		t.Fatalf("the parked mark at %q is still provisional; the reap would delete it", *parked)
	}

	// The confirmation moves the reference onto the company, so the key is
	// named throughout and nothing re-records it.
	company := confirmTheAnchor(t, e, args)
	worn, err := e.Contacts.CompanyLogoKey(e.As(e.Rep1, nil, integration.AdminPerms), company.CompanyID, contacts.LogoWide)
	if err != nil || worn != *parked {
		t.Fatalf("the anchor wears %q (%v), want the adopted mark at %q", worn, err, *parked)
	}
	if provisional(t, e, worn) {
		t.Fatalf("the adopted mark at %q became provisional again", worn)
	}
}
