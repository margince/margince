// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fakeAssets serves fixed bodies by address and counts each fetch.
type fakeAssets struct {
	mu      sync.Mutex
	bodies  map[string][]byte
	fetches map[string]int
}

func (f *fakeAssets) FetchAsset(_ context.Context, rawURL string) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetches[rawURL]++
	body, ok := f.bodies[rawURL]
	if !ok {
		return nil, "", errors.New("connection refused")
	}
	return body, "image/png", nil
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	return sizedPNG(t, 4, 2)
}

func sizedPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// previewClock is a cache clock that stands still, for a test that never
// reaches the end of the cache window.
func previewClock(t *testing.T) func() time.Time {
	now := clocktest.Now(t)
	return func() time.Time { return now }
}

func newFakeAssets(t *testing.T) *fakeAssets {
	t.Helper()
	logo := testPNG(t)
	return &fakeAssets{fetches: map[string]int{}, bodies: map[string][]byte{
		"https://cdn.example/logo.png": logo,
		"https://cdn.example/1.png":    logo, "https://cdn.example/2.png": logo,
		"https://cdn.example/3.png": logo, "https://cdn.example/4.png": logo,
		"https://cdn.example/5.png": logo, "https://cdn.example/6.png": logo,
		"https://cdn.example/page.png": []byte("<!doctype html><title>Not found</title>"),
	}}
}

func TestAPreviewDrawsAnHttpsImageFromADataCopy(t *testing.T) {
	t.Parallel()
	got := newSignaturePreviewImages(newFakeAssets(t), previewClock(t)).Inline(context.Background(),
		`<p>Anna</p><img src="https://cdn.example/logo.png" width="96" height="21" alt="Company A">`)
	if !strings.Contains(got, `<img src="data:image/png;base64,`) || strings.Contains(got, "cdn.example") {
		t.Fatalf("preview = %s", got)
	}
	if !strings.Contains(got, `width="96" height="21" alt="Company A"`) || !strings.Contains(got, "<p>Anna</p>") {
		t.Fatalf("preview lost the image's size or the text: %s", got)
	}
}

func TestAnImageThatCannotBeDrawnBecomesABoxNamingIt(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"html served as an image": "https://cdn.example/page.png",
		"a fetch that fails":      "https://down.example/logo.png",
	} {
		got := newSignaturePreviewImages(newFakeAssets(t), previewClock(t)).Inline(context.Background(),
			`<img src="`+src+`" width="120" height="40" alt="Company A">`)
		if strings.Contains(got, "<img") || strings.Contains(got, "data:") {
			t.Errorf("%s: preview kept the image: %s", name, got)
		}
		if !strings.Contains(got, "width:120px;height:40px") || !strings.Contains(got, "Company A (") {
			t.Errorf("%s: placeholder lacks the size or the name: %s", name, got)
		}
	}
}

func TestARepeatPreviewOfOneImageFetchesItOnce(t *testing.T) {
	t.Parallel()
	assets := newFakeAssets(t)
	images := newSignaturePreviewImages(assets, previewClock(t))
	markup := `<img src="https://cdn.example/logo.png"><img src="https://cdn.example/logo.png">`
	first := images.Inline(context.Background(), markup)
	second := images.Inline(context.Background(), markup)
	if first != second || strings.Count(first, "data:image/png") != 2 {
		t.Fatalf("previews differ or lack the copies:\n%s\n%s", first, second)
	}
	if n := assets.fetches["https://cdn.example/logo.png"]; n != 1 {
		t.Fatalf("fetched %d times, want 1", n)
	}
}

func TestASixthImageIsNotFetched(t *testing.T) {
	t.Parallel()
	assets := newFakeAssets(t)
	var markup strings.Builder
	for _, n := range []string{"1", "2", "3", "4", "5", "6"} {
		markup.WriteString(`<img src="https://cdn.example/` + n + `.png" alt="Image ` + n + `">`)
	}
	got := newSignaturePreviewImages(assets, previewClock(t)).Inline(context.Background(), markup.String())
	if strings.Count(got, "data:image/png") != signaturePreviewMaxImages || assets.fetches["https://cdn.example/6.png"] != 0 {
		t.Fatalf("fetches = %v, preview = %s", assets.fetches, got)
	}
	if !strings.Contains(got, "Image 6 (cdn.example)") {
		t.Fatalf("the sixth image has no placeholder: %s", got)
	}
}

func TestTheEmbeddedLogoAndMarkupWithoutImagesPassUnchanged(t *testing.T) {
	t.Parallel()
	assets := newFakeAssets(t)
	for _, markup := range []string{"<p>Anna</p>", `<p>Anna</p><img src="cid:signature-logo@margince" alt="" width="150">`} {
		if got := newSignaturePreviewImages(assets, previewClock(t)).Inline(context.Background(), markup); !strings.Contains(got, "<p>Anna</p>") || strings.Count(got, "cid:") != strings.Count(markup, "cid:") || strings.Contains(got, "data:") {
			t.Errorf("markup %q became %q", markup, got)
		}
	}
	if len(assets.fetches) != 0 {
		t.Fatalf("fetched %v", assets.fetches)
	}
}

// previewReaderIn is a context for one reader of one workspace.
func previewReaderIn(workspace, reader ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), workspace)
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, UserID: reader})
}

func TestACopyServesOnlyTheReaderAndWorkspaceThatFetchedIt(t *testing.T) {
	t.Parallel()
	assets := newFakeAssets(t)
	images := newSignaturePreviewImages(assets, previewClock(t))
	markup := `<img src="https://cdn.example/logo.png">`
	workspace, other := ids.NewV7(), ids.NewV7()
	reader, colleague := ids.NewV7(), ids.NewV7()
	for _, ctx := range []context.Context{previewReaderIn(workspace, reader), previewReaderIn(workspace, colleague), previewReaderIn(other, reader), previewReaderIn(workspace, reader)} {
		images.Inline(ctx, markup)
	}
	if n := assets.fetches["https://cdn.example/logo.png"]; n != 3 {
		t.Fatalf("fetched %d times, want one per reader and workspace", n)
	}
}

func TestACopyAndAFailureExpireAfterTheCacheWindow(t *testing.T) {
	t.Parallel()
	assets := newFakeAssets(t)
	clock := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	images := newSignaturePreviewImages(assets, func() time.Time { return clock })
	markup := `<img src="https://cdn.example/logo.png"><img src="https://down.example/logo.png">`
	images.Inline(context.Background(), markup)
	clock = clock.Add(signaturePreviewCacheTTL - time.Second)
	if got := images.Inline(context.Background(), markup); !strings.Contains(got, "data:image/png") || !strings.Contains(got, "down.example") {
		t.Fatalf("cached preview = %s", got)
	}
	for _, address := range []string{"https://cdn.example/logo.png", "https://down.example/logo.png"} {
		if n := assets.fetches[address]; n != 1 {
			t.Fatalf("%s fetched %d times inside the window, want 1", address, n)
		}
	}
	clock = clock.Add(2 * time.Second)
	images.Inline(context.Background(), markup)
	for _, address := range []string{"https://cdn.example/logo.png", "https://down.example/logo.png"} {
		if n := assets.fetches[address]; n != 2 {
			t.Fatalf("%s fetched %d times after the window, want 2", address, n)
		}
	}
}

func TestAnImageOverThePixelCapIsRefusedBeforeItIsDecoded(t *testing.T) {
	t.Parallel()
	assets := newFakeAssets(t)
	assets.bodies["https://cdn.example/huge.png"] = sizedPNG(t, 2000, 1001)
	assets.bodies["https://cdn.example/edge.png"] = sizedPNG(t, 2000, 1000)
	got := newSignaturePreviewImages(assets, previewClock(t)).Inline(context.Background(),
		`<img src="https://cdn.example/huge.png" alt="Huge"><img src="https://cdn.example/edge.png">`)
	if !strings.Contains(got, "Huge (cdn.example)") || strings.Count(got, "data:image/png") != 1 {
		t.Fatalf("preview = %.300s", got)
	}
}

func TestAListOfApprovalsShowsBoxesAndFetchesNothing(t *testing.T) {
	t.Parallel()
	assets := newFakeAssets(t)
	reader := approvalPreviewReader{signatureImages: newSignaturePreviewImages(assets, previewClock(t))}
	got := reader.signatureMarkup(context.Background(), `<p>Anna</p><img src="https://cdn.example/logo.png" alt="Company A">`)
	if len(assets.fetches) != 0 || strings.Contains(got, "<img") || !strings.Contains(got, "Company A (cdn.example)") ||
		!strings.Contains(got, activities.RemoteImageBoxAttribute) {
		t.Fatalf("fetches = %v, preview = %s", assets.fetches, got)
	}
}

// stalledAssets never answers, as a host that accepts and then sends nothing.
type stalledAssets struct{ never chan struct{} }

func (s stalledAssets) FetchAsset(context.Context, string) ([]byte, string, error) {
	<-s.never
	return nil, "", errors.New("unreachable")
}

func TestAStalledImageCostsABoxOnceTheBudgetEnds(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	never := make(chan struct{})
	t.Cleanup(func() { close(never) })
	got := newSignaturePreviewImages(stalledAssets{never: never}, previewClock(t)).Inline(ctx,
		`<img src="https://slow.example/logo.png" alt="Logo">`)
	if !strings.Contains(got, "Logo (slow.example)") {
		t.Fatalf("preview = %s", got)
	}
}
