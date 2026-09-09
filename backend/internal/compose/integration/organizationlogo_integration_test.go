// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The logo byte endpoint (A55): with an object store wired, the resolved mark
// streams as the PNG this server itself encoded, under headers that keep an
// asset harvested from a third-party website from ever acting like a document.
// Without a store the endpoint declares that by omission (501), and a company
// with no logo is a 404 the client answers with its monogram.

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// logoPNG is a small square PNG standing in for a normalized mark.
func logoPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return out.Bytes()
}

// waitForPutCount polls rather than asserting immediately, for a caller
// whose write runs on its own goroutine (organization logo write-back,
// deliberately backgrounded so a slow store cannot hold a handler open).
// Bounded by the deadline rather than a fixed sleep: an in-memory store
// settles in microseconds, so this returns on its first or second check in
// the ordinary case, and only fails as slowly as a genuine regression would.
func waitForPutCount(t *testing.T, blob *countingBlobstore, key string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if got := blob.putCount(key); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Put(%s) calls = %d after 2s, want %d", key, blob.putCount(key), want)
		}
		//craft:ignore test-sleep the poll interval for a condition wait bounded by the deadline above, not the fixed-duration sleep this check exists to catch
		time.Sleep(time.Millisecond)
	}
}

// failingFlushWriter forces http.ResponseController.Flush to report an
// error: ResponseController prefers a FlushError() error method over the
// plain Flusher it falls back to, and a bare httptest.ResponseRecorder only
// implements the latter — so nothing exercises streamLogo's flush-failure
// branch without this.
type failingFlushWriter struct {
	*httptest.ResponseRecorder
}

func (w *failingFlushWriter) FlushError() error {
	return errors.New("forced flush failure")
}

// A flush failure is best-effort, logged and nothing else: the reader
// already has their bytes buffered in the ResponseWriter by the time this
// runs, and a store that then also fails the write-back (memory, seeded
// with real bytes) still leaves the response streamLogo already decided.
func TestOrganizationLogoLogsRatherThanFailsWhenTheFlushErrors(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	handlers := people.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	orgID := seedLoggedOrg(ctx, t, e, blob, logoPNG(t))

	rec := &failingFlushWriter{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/logo", nil).WithContext(ctx)
	handlers.GetOrganizationLogo(rec, req, crmcontracts.Id(orgID.UUID))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET logo = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), logoPNG(t)) {
		t.Fatal("a failed flush must not change the bytes the reader receives")
	}
}

// seedLoggedOrg creates an organization and stores a logo for it: the bytes in
// the object store, the reference on the row — the exact two steps the deep
// read's resolve performs, in the same order.
func seedLoggedOrg(ctx context.Context, t *testing.T, e *Env, blob blobstore.Store, logo []byte) ids.OrganizationID {
	t.Helper()
	org, err := e.People.CreateOrganization(ctx, people.CreateOrganizationInput{
		DisplayName: "Voltaq Systems GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))
	key := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](e.WS), "organization_logo", orgID.String()+"/"+ids.NewV7().String())
	if err := blob.Put(ctx, key, bytes.NewReader(logo), int64(len(logo)), imagenorm.ContentType); err != nil {
		t.Fatalf("store the logo bytes: %v", err)
	}
	written, _, err := e.People.SetOrganizationLogo(ctx, orgID, key, "https://voltaq.test/touch.png")
	if err != nil {
		t.Fatalf("SetOrganizationLogo: %v", err)
	}
	if !written {
		t.Fatal("the logo write reported no change on a fresh organization")
	}
	return orgID
}

func TestOrganizationLogoStreamsTheStoredMarkUnderNonExecutableHeaders(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	handlers := people.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	want := logoPNG(t)
	orgID := seedLoggedOrg(ctx, t, e, blob, want)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/logo", nil).WithContext(ctx)
	handlers.GetOrganizationLogo(rec, req, crmcontracts.Id(orgID.UUID))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET logo = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatal("the streamed bytes are not the stored ones")
	}
	if got := rec.Header().Get("Content-Type"); got != imagenorm.ContentType {
		t.Fatalf("Content-Type = %q, want %q", got, imagenorm.ContentType)
	}
	// These two are what stop a third-party asset from being sniffed into an
	// active document on this origin.
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("the logo response carries no Content-Security-Policy")
	}

	// The record exposes the endpoint, never the bucket path behind it.
	read, err := e.People.GetOrganization(ctx, orgID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("read the organization: %v", err)
	}
	key, err := e.People.OrganizationLogoKey(ctx, orgID, people.LogoWide)
	if err != nil {
		t.Fatalf("read the stored logo key: %v", err)
	}
	wantURL := *people.LogoURL(orgID.UUID, &key, people.LogoWide)
	if read.LogoUrl == nil || *read.LogoUrl != wantURL {
		t.Fatalf("logo_url = %v, want %q", read.LogoUrl, wantURL)
	}
}

// The icon endpoint is the wide one's twin: same body, same slot argument, and
// the slot is the whole difference. What that leaves worth asserting is the
// wiring — that the icon route reads the icon column — because a route that
// read the wide one would stream a real, correct-looking PNG of the wrong
// picture, which no header or status check can see.
func TestTheLogoIconEndpointStreamsTheIconSlotAndNotTheWideOne(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	handlers := people.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	wide := logoPNG(t)
	orgID := seedLoggedOrg(ctx, t, e, blob, wide)

	// An organization wearing only the wide mark answers 404 here, which is the
	// state every record but the installation's own anchor is in — and the
	// signal a client falls back to the wide mark on.
	empty := httptest.NewRecorder()
	handlers.GetOrganizationLogoIcon(empty,
		httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/logo/icon", nil).WithContext(ctx),
		crmcontracts.Id(orgID.UUID))
	if empty.Code != http.StatusNotFound {
		t.Fatalf("GET icon on a record wearing only a wide mark = %d, want 404: %s",
			empty.Code, empty.Body.String())
	}
}

func TestOrganizationLogoRemovesLegacyTransparentCanvasAtTheDisplayBoundary(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	ctx := e.Admin()
	wide := image.NewNRGBA(image.Rect(0, 0, 32, 8))
	for y := range 8 {
		for x := range 32 {
			wide.SetNRGBA(x, y, color.NRGBA{R: 255, G: 90, A: 255})
		}
	}
	legacy, err := imagenorm.SquarePNG(wide, 32)
	if err != nil {
		t.Fatalf("encoding a legacy square-canvas logo: %v", err)
	}
	orgID := seedLoggedOrg(ctx, t, e, blob, legacy)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/logo", nil).WithContext(ctx)
	people.NewHandlers(e.DB()).WithBlobstore(blob).GetOrganizationLogo(rec, req, crmcontracts.Id(orgID.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET logo = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	displayed, err := png.Decode(rec.Body)
	if err != nil {
		t.Fatalf("display response is not a PNG: %v", err)
	}
	if bounds := displayed.Bounds(); bounds.Dx() != 32 || bounds.Dy() != 8 {
		t.Fatalf("displayed logo is %v, want the original 4:1 wordmark", bounds)
	}
}

// TrimTransparentPNG's decode-and-scan is unavoidable per read, but a legacy
// letterboxed logo's crop is deterministic on bytes that never change — so
// paying the re-encode on every single request is waste. The read that first
// computes a crop writes it back over the object it read, so this proves the
// SECOND read finds already-tight bytes and needs no further write.
func TestOrganizationLogoWritesBackATrimmedLegacyLogoSoTheNextReadNeedsNoCrop(t *testing.T) {
	e := Setup(t)
	blob := newCountingBlobstore()
	handlers := people.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	wide := image.NewNRGBA(image.Rect(0, 0, 32, 8))
	for y := range 8 {
		for x := range 32 {
			wide.SetNRGBA(x, y, color.NRGBA{R: 255, G: 90, A: 255})
		}
	}
	legacy, err := imagenorm.SquarePNG(wide, 32)
	if err != nil {
		t.Fatalf("encoding a legacy square-canvas logo: %v", err)
	}
	orgID := seedLoggedOrg(ctx, t, e, blob, legacy)
	key, err := e.People.OrganizationLogoKey(ctx, orgID, people.LogoWide)
	if err != nil {
		t.Fatalf("read the stored logo key: %v", err)
	}
	url := "/v1/organizations/" + orgID.String() + "/logo"

	first := httptest.NewRecorder()
	handlers.GetOrganizationLogo(first, httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx), crmcontracts.Id(orgID.UUID))
	if first.Code != http.StatusOK {
		t.Fatalf("first GET = %d, want 200: %s", first.Code, first.Body.String())
	}
	// The write-back runs on its own goroutine (handlers_organizationlogo.go)
	// precisely so a slow store cannot hold the handler open — see there for
	// why. That makes it genuinely concurrent with this assertion, so this
	// polls rather than asserting the count immediately after the call
	// returns; an in-memory store's Put settles in microseconds, so the loop
	// exits on its first or second check in the ordinary case and only the
	// bound (not a fixed sleep) protects against a real regression hanging.
	waitForPutCount(t, blob, key, 2)

	rc, _, err := blob.Get(ctx, key)
	if err != nil {
		t.Fatalf("reading the object back: %v", err)
	}
	stored, err := io.ReadAll(rc)
	if closeErr := rc.Close(); closeErr != nil {
		t.Fatalf("closing the object reader: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("reading the object's bytes: %v", err)
	}
	decoded, err := png.Decode(bytes.NewReader(stored))
	if err != nil {
		t.Fatalf("the written-back object is not a PNG: %v", err)
	}
	if bounds := decoded.Bounds(); bounds.Dx() != 32 || bounds.Dy() != 8 {
		t.Fatalf("the object stored after write-back is %v, want the trimmed 4:1 wordmark — it should not still be the 32x32 legacy canvas", bounds)
	}

	second := httptest.NewRecorder()
	handlers.GetOrganizationLogo(second, httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx), crmcontracts.Id(orgID.UUID))
	if second.Code != http.StatusOK {
		t.Fatalf("second GET = %d, want 200: %s", second.Code, second.Body.String())
	}
	if !bytes.Equal(second.Body.Bytes(), stored) {
		t.Fatal("the second read's bytes are not the written-back object's bytes")
	}
	if got := blob.putCount(key); got != 2 {
		t.Fatalf("Put(%s) calls after the second read = %d, want still 2 — a read of already-tight bytes must not write back", key, got)
	}
}

// A client that already holds today's picture is told so — 304, no body —
// without the endpoint decoding, scanning or re-encoding the stored PNG, or
// even opening it in blob storage. countingBlobstore is shared with
// knowledgeorphan_integration_test.go.
func TestOrganizationLogoAnswers304WithoutTouchingBlobStorageWhenTheClientAlreadyHasIt(t *testing.T) {
	e := Setup(t)
	blob := newCountingBlobstore()
	handlers := people.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	orgID := seedLoggedOrg(ctx, t, e, blob, logoPNG(t))
	url := "/v1/organizations/" + orgID.String() + "/logo"

	first := httptest.NewRecorder()
	handlers.GetOrganizationLogo(first, httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx), crmcontracts.Id(orgID.UUID))
	if first.Code != http.StatusOK {
		t.Fatalf("first GET = %d, want 200: %s", first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("the 200 response carries no ETag")
	}
	if got := blob.getCount(); got != 1 {
		t.Fatalf("blob.Get calls after the first request = %d, want 1", got)
	}

	// The ETag and LogoURL's own cache-busting query token are meant to be
	// the SAME digest of the same key (logoRevisionDigest) — pin that they
	// still are, so the two spellings cannot silently drift apart.
	key, err := e.People.OrganizationLogoKey(ctx, orgID, people.LogoWide)
	if err != nil {
		t.Fatalf("read the stored logo key: %v", err)
	}
	wantURL := *people.LogoURL(orgID.UUID, &key, people.LogoWide)
	wantDigest := wantURL[strings.LastIndex(wantURL, "=")+1:]
	if gotDigest := strings.Trim(etag, `"`); gotDigest != wantDigest {
		t.Fatalf("ETag digest = %q, want %q (LogoURL's own query token)", gotDigest, wantDigest)
	}

	matching := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx)
	req.Header.Set("If-None-Match", etag)
	handlers.GetOrganizationLogo(matching, req, crmcontracts.Id(orgID.UUID))
	if matching.Code != http.StatusNotModified {
		t.Fatalf("GET with a matching If-None-Match = %d, want 304: %s", matching.Code, matching.Body.String())
	}
	if matching.Body.Len() != 0 {
		t.Fatalf("a 304 response body = %d bytes, want none", matching.Body.Len())
	}
	if got := blob.getCount(); got != 1 {
		t.Fatalf("blob.Get calls after the matching request = %d, want still 1 (the cache hit must not touch blob storage)", got)
	}

	// RFC 9110 §13.1.2: If-None-Match comparison is WEAK, so a proxy that
	// prefixes this server's own strong tag with "W/" must still count as a
	// match rather than falling through to the full decode/re-encode path.
	weak := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx)
	req.Header.Set("If-None-Match", "W/"+etag)
	handlers.GetOrganizationLogo(weak, req, crmcontracts.Id(orgID.UUID))
	if weak.Code != http.StatusNotModified {
		t.Fatalf("GET with a weak (W/-prefixed) matching If-None-Match = %d, want 304: %s", weak.Code, weak.Body.String())
	}
	if got := blob.getCount(); got != 1 {
		t.Fatalf("blob.Get calls after the weak-match request = %d, want still 1", got)
	}

	stale := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx)
	req.Header.Set("If-None-Match", `"stale-revision"`)
	handlers.GetOrganizationLogo(stale, req, crmcontracts.Id(orgID.UUID))
	if stale.Code != http.StatusOK {
		t.Fatalf("GET with a stale If-None-Match = %d, want 200: %s", stale.Code, stale.Body.String())
	}
	if got := blob.getCount(); got != 2 {
		t.Fatalf("blob.Get calls after the stale-etag request = %d, want 2", got)
	}
}

// gatedBlobstore lets a test hold one key's Put open at the exact instant a
// concurrent write would race it — the write-back's own goroutine gives no
// other hook to land a test's step in the middle of it. Unarmed by default so
// seedLoggedOrg's own write to the SAME key (the object write-back later
// overwrites) is never held.
type gatedBlobstore struct {
	*countingBlobstore
	mu         sync.Mutex
	heldKey    string
	armed      bool
	entered    chan struct{}
	release    chan struct{}
	failPutKey string
	failDelKey string
	// attempted fires once per Put/Delete call, keyed by "op key" — the
	// signal a failed call still needs, since failPut/failDelete short-circuit
	// before countingBlobstore ever records anything.
	attempted chan string
}

func newGatedBlobstore(inner *countingBlobstore) *gatedBlobstore {
	return &gatedBlobstore{countingBlobstore: inner, attempted: make(chan string, 32)}
}

// waitAttempted blocks until op ("put"/"delete") is attempted against key, or
// fails the test after 2s.
func waitAttempted(t *testing.T, g *gatedBlobstore, op, key string) {
	t.Helper()
	want := op + " " + key
	deadline := time.After(2 * time.Second)
	for {
		select {
		case got := <-g.attempted:
			if got == want {
				return
			}
		case <-deadline:
			t.Fatalf("%q was never attempted within 2s", want)
		}
	}
}

// hold arms the gate: the next Put(s) to key block after entering (signalling
// on entered) until releaseHeld is called.
func (g *gatedBlobstore) hold(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.heldKey = key
	g.armed = true
	g.entered = make(chan struct{}, 8)
	g.release = make(chan struct{})
}

func (g *gatedBlobstore) releaseHeld() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.armed = false
	close(g.release)
}

// failPut makes the next Put(s) to key answer an error instead of storing —
// the store this call's caller must log rather than fail its own request
// over, since the response it answers is already on the wire.
func (g *gatedBlobstore) failPut(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failPutKey = key
}

// failDelete is failPut's twin for the collection write-back runs when a
// replace raced its own Put.
func (g *gatedBlobstore) failDelete(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failDelKey = key
}

func (g *gatedBlobstore) Delete(ctx context.Context, key string) error {
	g.mu.Lock()
	failing := g.failDelKey != "" && key == g.failDelKey
	g.mu.Unlock()
	g.attempted <- "delete " + key
	if failing {
		return errors.New("simulated delete failure")
	}
	return g.countingBlobstore.Delete(ctx, key)
}

func (g *gatedBlobstore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	g.mu.Lock()
	gate := g.armed && key == g.heldKey
	failing := g.failPutKey != "" && key == g.failPutKey
	entered, release := g.entered, g.release
	g.mu.Unlock()
	g.attempted <- "put " + key
	if failing {
		return errors.New("simulated put failure")
	}
	if gate {
		entered <- struct{}{}
		<-release
	}
	return g.countingBlobstore.Put(ctx, key, r, size, contentType)
}

// waitForNotFound polls rather than asserting immediately, for the same
// reason waitForPutCount does: the collection this proves runs on the
// write-back's own goroutine.
func waitForNotFound(t *testing.T, blob blobstore.Store, key string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, _, err := blob.Get(context.Background(), key)
		if errors.Is(err, blobstore.ErrNotFound) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Get(%s) never answered ErrNotFound after 2s (err=%v)", key, err)
		}
		//craft:ignore test-sleep the poll interval for a condition wait bounded by the deadline above, not the fixed-duration sleep this check exists to catch
		time.Sleep(time.Millisecond)
	}
}

// A burst of readers landing on the same untrimmed object must start at most
// ONE write-back for it: streamLogo coalesces by key (handlers_organizationlogo.go)
// precisely so a request storm right after an upload or a migration cannot pile
// up one goroutine per reader, all racing to write the identical bytes to the
// identical key.
func TestOrganizationLogoWriteBackCoalescesConcurrentReadersOfTheSameUntrimmedKey(t *testing.T) {
	e := Setup(t)
	inner := newCountingBlobstore()
	blob := newGatedBlobstore(inner)
	handlers := people.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	wide := image.NewNRGBA(image.Rect(0, 0, 32, 8))
	for y := range 8 {
		for x := range 32 {
			wide.SetNRGBA(x, y, color.NRGBA{R: 255, G: 90, A: 255})
		}
	}
	legacy, err := imagenorm.SquarePNG(wide, 32)
	if err != nil {
		t.Fatalf("encoding a legacy square-canvas logo: %v", err)
	}
	orgID := seedLoggedOrg(ctx, t, e, blob, legacy)
	key, err := e.People.OrganizationLogoKey(ctx, orgID, people.LogoWide)
	if err != nil {
		t.Fatalf("read the stored logo key: %v", err)
	}
	url := "/v1/organizations/" + orgID.String() + "/logo"

	blob.hold(key)
	const readers = 5
	var wg sync.WaitGroup
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			handlers.GetOrganizationLogo(rec,
				httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx),
				crmcontracts.Id(orgID.UUID))
		}()
	}
	wg.Wait()

	select {
	case <-blob.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("no write-back reached blob.Put within 2s")
	}
	select {
	case <-blob.entered:
		t.Fatal("a second write-back reached blob.Put concurrently — the readers were not coalesced")
	case <-time.After(50 * time.Millisecond):
	}
	blob.releaseHeld()

	// One write-back, whichever of the readers claimed it: the seed Put plus
	// this one is 2, not 1+readers.
	waitForPutCount(t, inner, key, 2)
}

// writeBackTrimmedLogo re-checks the slot's current key AFTER blob.Put, not
// only before: a replace landing WHILE the write is in flight can otherwise
// let a write-back resurrect the exact orphan deleteUnreferencedLogo exists to
// prevent (handlers_organizationlogo.go). This proves the after-check: a
// replace that lands mid-Put gets its own resurrected object collected, not
// left for nothing to ever reference again.
func TestOrganizationLogoWriteBackCollectsItsOwnObjectWhenAReplaceRacesTheWrite(t *testing.T) {
	e := Setup(t)
	inner := newCountingBlobstore()
	blob := newGatedBlobstore(inner)
	handlers := people.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	wide := image.NewNRGBA(image.Rect(0, 0, 32, 8))
	for y := range 8 {
		for x := range 32 {
			wide.SetNRGBA(x, y, color.NRGBA{R: 255, G: 90, A: 255})
		}
	}
	legacy, err := imagenorm.SquarePNG(wide, 32)
	if err != nil {
		t.Fatalf("encoding a legacy square-canvas logo: %v", err)
	}
	// The anchor organization, not a fresh one: a REPLACE that can actually
	// out-rank the first mark needs the human-authored path (SetCompanyLogo),
	// and that path always targets the installation's own company rather than
	// taking a record id (companylogo.go says why).
	offer, icp := "Revenue operations software", "RevOps at SaaS scale-ups"
	company, err := e.People.SaveCompany(ctx, people.SaveCompanyInput{
		DisplayName: "Voltaq Systems GmbH",
		Fields:      map[string]*string{"offer_summary": &offer, "icp": &icp},
	})
	if err != nil {
		t.Fatalf("save the company: %v", err)
	}
	orgID := company.OrganizationID
	staleKey := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](e.WS), "organization_logo", orgID.String()+"/"+ids.NewV7().String())
	if err := blob.Put(ctx, staleKey, bytes.NewReader(legacy), int64(len(legacy)), imagenorm.ContentType); err != nil {
		t.Fatalf("store the legacy logo bytes: %v", err)
	}
	if written, _, err := e.People.SetOrganizationLogo(ctx, orgID, staleKey, "https://voltaq.test/legacy.png"); err != nil {
		t.Fatalf("SetOrganizationLogo (seed): %v", err)
	} else if !written {
		t.Fatal("the seed write reported no change on a fresh anchor organization")
	}
	url := "/v1/organizations/" + orgID.String() + "/logo"

	blob.hold(staleKey)
	rec := httptest.NewRecorder()
	handlers.GetOrganizationLogo(rec, httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx), crmcontracts.Id(orgID.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET logo = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	select {
	case <-blob.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the write-back never reached blob.Put within 2s")
	}

	// A person replaces the mark WHILE the write-back above is blocked inside
	// blob.Put(staleKey, ...) — the exact window the after-check exists for.
	// SetCompanyLogo never declines, unlike a second resolve against the
	// mark this test's own seed already set — the human write is the one
	// case production actually lets race the write-back this way.
	replacement := "organization_logo/" + orgID.String() + "/" + ids.NewV7().String()
	if err := blob.Put(ctx, replacement, bytes.NewReader(logoPNG(t)), int64(len(logoPNG(t))), imagenorm.ContentType); err != nil {
		t.Fatalf("store the replacement logo bytes: %v", err)
	}
	if _, err := e.People.SetCompanyLogo(ctx, people.LogoWide, replacement, "replacement.png"); err != nil {
		t.Fatalf("SetCompanyLogo (replacement): %v", err)
	}

	blob.releaseHeld()
	waitForPutCount(t, inner, staleKey, 2)
	waitForNotFound(t, blob, staleKey)

	current, err := e.People.OrganizationLogoKey(ctx, orgID, people.LogoWide)
	if err != nil {
		t.Fatalf("read the stored logo key: %v", err)
	}
	if current != replacement {
		t.Fatalf("current logo key = %q, want the replacement %q", current, replacement)
	}
}

// A store whose write-back Put fails costs nothing but that read's own CPU:
// the response is already on the wire, and the object at key still holds the
// correct, if untrimmed, picture for the next read to try again against.
func TestOrganizationLogoWriteBackLogsRatherThanFailsWhenThePutErrors(t *testing.T) {
	e := Setup(t)
	inner := newCountingBlobstore()
	blob := newGatedBlobstore(inner)
	handlers := people.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	wide := image.NewNRGBA(image.Rect(0, 0, 32, 8))
	for y := range 8 {
		for x := range 32 {
			wide.SetNRGBA(x, y, color.NRGBA{R: 255, G: 90, A: 255})
		}
	}
	legacy, err := imagenorm.SquarePNG(wide, 32)
	if err != nil {
		t.Fatalf("encoding a legacy square-canvas logo: %v", err)
	}
	orgID := seedLoggedOrg(ctx, t, e, blob, legacy)
	key, err := e.People.OrganizationLogoKey(ctx, orgID, people.LogoWide)
	if err != nil {
		t.Fatalf("read the stored logo key: %v", err)
	}
	url := "/v1/organizations/" + orgID.String() + "/logo"

	blob.failPut(key)
	rec := httptest.NewRecorder()
	handlers.GetOrganizationLogo(rec, httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx), crmcontracts.Id(orgID.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET logo = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	// The response is always the freshly TRIMMED bytes — the crop runs before
	// the write-back is even considered — so a failed persist changes only
	// what is STORED, never what this read answers.
	displayed, err := png.Decode(rec.Body)
	if err != nil {
		t.Fatalf("display response is not a PNG: %v", err)
	}
	if bounds := displayed.Bounds(); bounds.Dx() != 32 || bounds.Dy() != 8 {
		t.Fatalf("displayed logo is %v, want the trimmed 4:1 wordmark even though the write-back failed", bounds)
	}

	// Best-effort: the failed Put is attempted exactly once and never retried
	// inline, and the object at key must still hold the ORIGINAL untrimmed
	// bytes — only the seed's own successful Put (count 1), never a second.
	waitAttempted(t, blob, "put", key)
	if got := inner.putCount(key); got != 1 {
		t.Fatalf("Put(%s) succeeded %d times, want only the seed's own 1", key, got)
	}
	rc, _, err := blob.Get(ctx, key)
	if err != nil {
		t.Fatalf("reading the object back: %v", err)
	}
	stored, err := io.ReadAll(rc)
	if closeErr := rc.Close(); closeErr != nil {
		t.Fatalf("closing the object reader: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("reading the object's bytes: %v", err)
	}
	if !bytes.Equal(stored, legacy) {
		t.Fatal("the object at key must still hold the untrimmed bytes after a failed write-back")
	}
}

// A store whose collecting Delete fails, after a replace raced the write-back's
// own Put, costs storage and nothing else — the row already names the
// replacement, so nothing serves the resurrected object; it is logged rather
// than retried inline.
func TestOrganizationLogoWriteBackLogsRatherThanFailsWhenTheCollectingDeleteErrors(t *testing.T) {
	e := Setup(t)
	inner := newCountingBlobstore()
	blob := newGatedBlobstore(inner)
	handlers := people.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	wide := image.NewNRGBA(image.Rect(0, 0, 32, 8))
	for y := range 8 {
		for x := range 32 {
			wide.SetNRGBA(x, y, color.NRGBA{R: 255, G: 90, A: 255})
		}
	}
	legacy, err := imagenorm.SquarePNG(wide, 32)
	if err != nil {
		t.Fatalf("encoding a legacy square-canvas logo: %v", err)
	}
	offer, icp := "Revenue operations software", "RevOps at SaaS scale-ups"
	company, err := e.People.SaveCompany(ctx, people.SaveCompanyInput{
		DisplayName: "Voltaq Systems GmbH",
		Fields:      map[string]*string{"offer_summary": &offer, "icp": &icp},
	})
	if err != nil {
		t.Fatalf("save the company: %v", err)
	}
	orgID := company.OrganizationID
	staleKey := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](e.WS), "organization_logo", orgID.String()+"/"+ids.NewV7().String())
	if err := blob.Put(ctx, staleKey, bytes.NewReader(legacy), int64(len(legacy)), imagenorm.ContentType); err != nil {
		t.Fatalf("store the legacy logo bytes: %v", err)
	}
	if written, _, err := e.People.SetOrganizationLogo(ctx, orgID, staleKey, "https://voltaq.test/legacy.png"); err != nil {
		t.Fatalf("SetOrganizationLogo (seed): %v", err)
	} else if !written {
		t.Fatal("the seed write reported no change on a fresh anchor organization")
	}
	url := "/v1/organizations/" + orgID.String() + "/logo"

	blob.hold(staleKey)
	blob.failDelete(staleKey)
	rec := httptest.NewRecorder()
	handlers.GetOrganizationLogo(rec, httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx), crmcontracts.Id(orgID.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET logo = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	select {
	case <-blob.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the write-back never reached blob.Put within 2s")
	}

	replacement := "organization_logo/" + orgID.String() + "/" + ids.NewV7().String()
	if err := blob.Put(ctx, replacement, bytes.NewReader(logoPNG(t)), int64(len(logoPNG(t))), imagenorm.ContentType); err != nil {
		t.Fatalf("store the replacement logo bytes: %v", err)
	}
	if _, err := e.People.SetCompanyLogo(ctx, people.LogoWide, replacement, "replacement.png"); err != nil {
		t.Fatalf("SetCompanyLogo (replacement): %v", err)
	}

	blob.releaseHeld()
	waitForPutCount(t, inner, staleKey, 2)
	waitAttempted(t, blob, "delete", staleKey)

	// The failed collect leaves the resurrected object behind rather than
	// retrying inline — logged, not fatal, and never blocking the row from
	// already naming the replacement.
	if _, _, err := blob.Get(ctx, staleKey); err != nil {
		t.Fatalf("the object at staleKey should still exist after a failed Delete: %v", err)
	}
	current, err := e.People.OrganizationLogoKey(ctx, orgID, people.LogoWide)
	if err != nil {
		t.Fatalf("read the stored logo key: %v", err)
	}
	if current != replacement {
		t.Fatalf("current logo key = %q, want the replacement %q — a failed collect must not roll back the row", current, replacement)
	}
}

func TestOrganizationLogoIs404WithoutOneAnd501WithoutAnObjectStore(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	ctx := e.Admin()

	bare, err := e.People.CreateOrganization(ctx, people.CreateOrganizationInput{
		DisplayName: "Kein Logo GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
	bareID := ids.From[ids.OrganizationKind](ids.UUID(bare.Id))

	// No logo on file: a 404 the client renders as a monogram.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+bareID.String()+"/logo", nil).WithContext(ctx)
	people.NewHandlers(e.DB()).WithBlobstore(blob).GetOrganizationLogo(rec, req, crmcontracts.Id(bareID.UUID))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET logo of a logo-less org = %d, want 404", rec.Code)
	}

	// An id that names nothing answers the same 404, so the endpoint never
	// confirms which organizations exist.
	rec = httptest.NewRecorder()
	missing := ids.NewV7()
	req = httptest.NewRequest(http.MethodGet, "/v1/organizations/"+missing.String()+"/logo", nil).WithContext(ctx)
	people.NewHandlers(e.DB()).WithBlobstore(blob).GetOrganizationLogo(rec, req, crmcontracts.Id(missing))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET logo of an unknown org = %d, want 404", rec.Code)
	}

	// A role with no object store wired: the organization HAS a logo, and the
	// endpoint says the deployment cannot serve it rather than nil-derefing.
	orgID := seedLoggedOrg(ctx, t, e, blob, logoPNG(t))
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/logo", nil).WithContext(ctx)
	people.NewHandlers(e.DB()).GetOrganizationLogo(rec, req, crmcontracts.Id(orgID.UUID))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("GET logo with no object store = %d, want 501", rec.Code)
	}
}
