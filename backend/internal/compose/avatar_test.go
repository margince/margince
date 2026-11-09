// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/webread"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// halvesImage is a landscape picture whose left half is red and right half
// blue, so a centred crop shows where it cut.
func halvesImage(width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			fill := color.NRGBA{R: 220, A: 255}
			if x >= width/2 {
				fill = color.NRGBA{B: 220, A: 255}
			}
			img.SetNRGBA(x, y, fill)
		}
	}
	return img
}

func encodedPicture(t *testing.T, encode func(*bytes.Buffer, image.Image) error, img image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := encode(&out, img); err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return out.Bytes()
}

func pngOf(out *bytes.Buffer, img image.Image) error  { return png.Encode(out, img) }
func jpegOf(out *bytes.Buffer, img image.Image) error { return jpeg.Encode(out, img, nil) }
func gifOf(out *bytes.Buffer, img image.Image) error  { return gif.Encode(out, img, nil) }

func TestAPictureIsCroppedToItsCentredSquareAndStoredAsPNG(t *testing.T) {
	for name, source := range map[string][]byte{
		"png":  encodedPicture(t, pngOf, halvesImage(80, 40)),
		"jpeg": encodedPicture(t, jpegOf, halvesImage(80, 40)),
	} {
		stored, err := normalizeAvatar(source)
		if err != nil {
			t.Fatalf("%s: a readable picture was refused: %v", name, err)
		}
		img, err := png.Decode(bytes.NewReader(stored))
		if err != nil {
			t.Fatalf("%s: the stored bytes are not a PNG: %v", name, err)
		}
		if b := img.Bounds(); b.Dx() != 40 || b.Dy() != 40 {
			t.Errorf("%s: stored %dx%d, want the 40x40 centre with nothing upscaled", name, b.Dx(), b.Dy())
		}
		if _, _, _, alpha := img.At(0, 20).RGBA(); alpha == 0 {
			t.Errorf("%s: the stored square has a transparent edge, so it was letterboxed rather than cropped", name)
		}
		left, _, _, _ := img.At(2, 20).RGBA()
		_, _, right, _ := img.At(37, 20).RGBA()
		if left == 0 || right == 0 {
			t.Errorf("%s: the crop is off centre: it should hold both halves", name)
		}
	}
}

func TestAPictureNeverExceedsTheStoredEdge(t *testing.T) {
	stored, err := normalizeAvatar(encodedPicture(t, pngOf, halvesImage(900, 700)))
	if err != nil {
		t.Fatalf("a large picture was refused: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(stored))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != avatarEdge || b.Dy() != avatarEdge {
		t.Errorf("stored %dx%d, want %dpx square", b.Dx(), b.Dy(), avatarEdge)
	}
}

func TestAGIFAnSVGAndANonImageAreRefusedAsAPicture(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>`)
	for name, source := range map[string][]byte{
		"gif":  encodedPicture(t, gifOf, halvesImage(20, 20)),
		"svg":  svg,
		"text": []byte("not a picture at all"),
	} {
		if _, err := normalizeAvatar(source); !errors.Is(err, errAvatarFormat) {
			t.Errorf("%s: %v, want the unsupported-format refusal", name, err)
		}
	}
}

func TestAProviderPictureIsFetchedOnlyFromAnHTTPSGoogleHost(t *testing.T) {
	if err := googlePictureURL("https://lh3.googleusercontent.com/a/ACg8oc=s96-c"); err != nil {
		t.Errorf("Google's own picture URL was refused: %v", err)
	}
	for _, refused := range []string{
		"http://lh3.googleusercontent.com/a/pic",
		"https://evilgoogleusercontent.com/a/pic",
		"https://googleusercontent.com.attacker.test/a/pic",
		"https://user@lh3.googleusercontent.com/a/pic",
		"https://lh3.googleusercontent.com:8443/a/pic",
		"https://169.254.169.254/latest/meta-data",
		"file:///etc/passwd",
	} {
		if err := googlePictureURL(refused); err == nil {
			t.Errorf("%s was accepted as a Google picture URL", refused)
		}
	}
}

func TestARefusedPictureURLIsNeverFetched(t *testing.T) {
	site := &assetSite{assets: map[string][]byte{}}
	pictures := &avatarPictures{fetch: site, blob: blobstore.NewMemory()}
	if err := pictures.FillFromPictureURL(context.Background(), ids.New[ids.UserKind](), "https://attacker.test/pic"); err == nil {
		t.Error("a picture URL off Google's hosts was filled")
	}
	if len(site.asked) != 0 {
		t.Errorf("the fetcher was asked for %v", site.asked)
	}
}

func TestEveryPictureRouteAnswers501WithNoObjectStore(t *testing.T) {
	h := avatarHandlers{pictures: &avatarPictures{}}
	routes := map[string]func(*httptest.ResponseRecorder, *http.Request){
		"upload": func(w *httptest.ResponseRecorder, r *http.Request) { h.UploadMyAvatar(w, r) },
		"stream": func(w *httptest.ResponseRecorder, r *http.Request) {
			h.GetUserAvatar(w, r, crmcontracts.Id(ids.NewV7()), crmcontracts.GetUserAvatarParams{})
		},
	}
	for name, serve := range routes {
		rec := httptest.NewRecorder()
		serve(rec, httptest.NewRequest(http.MethodGet, "/v1/me/avatar", nil))
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s with no object store = %d, want 501", name, rec.Code)
		}
	}
}

// failingPictureHost answers every fetch with one error, after asking ctx's redirect guard
// about a hop off Google's hosts the way webread's redirect hook does.
type failingPictureHost struct {
	err        error
	guardSaid  error
	guardAsked bool
}

func (h *failingPictureHost) FetchAsset(ctx context.Context, _ string) ([]byte, string, error) {
	h.guardAsked = true
	h.guardSaid = webread.RedirectAllowed(ctx, &url.URL{Scheme: "http", Host: "attacker.test", Path: "/pic"})
	return nil, "", h.err
}

const privatePicture = "https://lh3.googleusercontent.com/a/ACg8oc-carol-private=s96-c"

// The picture URL is personal data, and the fill's error is what a failed sign-in fill logs.
func TestAFailedPictureFetchNamesNoPartOfItsURL(t *testing.T) {
	t.Parallel()
	for name, cause := range map[string]error{
		"a status":   &webread.StatusError{Status: http.StatusForbidden, URL: privatePicture},
		"robots":     errors.Join(webread.ErrRobotsDisallowed, errors.New("/a/ACg8oc-carol-private=s96-c")),
		"transport":  &url.Error{Op: "Get", URL: privatePicture, Err: errors.New("connection reset")},
		"a redirect": &url.Error{Op: "Get", URL: privatePicture, Err: errPictureRedirect},
	} {
		host := &failingPictureHost{err: cause}
		_, err := (&avatarPictures{fetch: host}).fetchPicture(context.Background(), privatePicture)
		if err == nil {
			t.Fatalf("%s: a failed fetch reported success", name)
		}
		if strings.Contains(err.Error(), "ACg8oc") || strings.Contains(err.Error(), "googleusercontent") {
			t.Errorf("%s: the logged error %q carries the picture URL", name, err)
		}
	}
}

func TestThePictureFetchRefusesARedirectOffGooglesHosts(t *testing.T) {
	t.Parallel()
	host := &failingPictureHost{err: errors.New("unused")}
	if _, err := (&avatarPictures{fetch: host}).fetchPicture(context.Background(), privatePicture); err == nil {
		t.Fatal("the fetch reported success")
	}
	if !host.guardAsked || !errors.Is(host.guardSaid, errPictureRedirect) {
		t.Errorf("a hop to an http host off Google's was allowed: %v", host.guardSaid)
	}
}
