// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// What the upload decoder does with what a browser sends, without a database:
// every refusal here is written before a row is touched, so the transport's
// half of the contract is provable on its own.

// A multipart body with one part named as the caller says, carrying `content`.
func multipartWith(t *testing.T, part, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	file, err := form.CreateFormFile(part, filename)
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := file.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := form.Close(); err != nil {
		t.Fatalf("close form: %v", err)
	}
	return body, form.FormDataContentType()
}

// A real PNG, taller than it is wide, so the square re-encode has something
// to do: the stored mark must come back square and no larger than the edge.
func tallPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 300, 600))
	for y := range 600 {
		for x := range 300 {
			img.Set(x, y, color.RGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return out.Bytes()
}

func decodeUpload(t *testing.T, body *bytes.Buffer, contentType string) (*httptest.ResponseRecorder, []byte, string, bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/company/logo", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	out, name, ok := decodeCompanyLogoUpload(rec, req)
	return rec, out, name, ok
}

func TestAnUploadedImageIsReEncodedAsASquarePNGNoLargerThanTheEdge(t *testing.T) {
	body, contentType := multipartWith(t, "file", "  brand.png  ", tallPNG(t))
	rec, out, name, ok := decodeUpload(t, body, contentType)
	if !ok {
		t.Fatalf("a readable PNG was refused: %d %s", rec.Code, rec.Body.String())
	}
	if name != "brand.png" {
		t.Fatalf("filename = %q, want the trimmed name", name)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("the stored bytes are not a PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != bounds.Dy() {
		t.Fatalf("stored mark is %dx%d, want square", bounds.Dx(), bounds.Dy())
	}
	if bounds.Dx() > companyLogoEdge {
		t.Fatalf("stored mark edge %d exceeds %d", bounds.Dx(), companyLogoEdge)
	}
}

func TestBytesThatAreNotAnImageAreRefusedAsUnsupportedMedia(t *testing.T) {
	body, contentType := multipartWith(t, "file", "notes.txt", []byte("this is not a picture"))
	rec, _, _, ok := decodeUpload(t, body, contentType)
	if ok {
		t.Fatal("text was accepted as an image")
	}
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusUnsupportedMediaType, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported_media_type") {
		t.Fatalf("refusal does not name its code: %s", rec.Body.String())
	}
}

func TestAFormWithoutTheFilePartIsAValidationRefusal(t *testing.T) {
	body, contentType := multipartWith(t, "picture", "brand.png", tallPNG(t))
	rec, _, _, ok := decodeUpload(t, body, contentType)
	if ok {
		t.Fatal("a form naming no `file` part was accepted")
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestABodyThatIsNotMultipartIsRefusedBeforeAnythingIsDecoded(t *testing.T) {
	rec, _, _, ok := decodeUpload(t, bytes.NewBufferString(`{"file":"brand.png"}`), "application/json")
	if ok {
		t.Fatal("a JSON body was accepted as an upload")
	}
	if rec.Code < http.StatusBadRequest || rec.Code >= http.StatusInternalServerError {
		t.Fatalf("status = %d, want a client refusal", rec.Code)
	}
}

// The name is shown back to a person in the field's history and is under the
// control of whoever made the file, so it is cut — by rune, because a cut
// through a multi-byte character leaves a fragment that is not text.
func TestTheFilenameIsBoundedByRuneAndTrimmed(t *testing.T) {
	long := strings.Repeat("ä", companyLogoNameMax+10)
	got := boundedFilename("  " + long + "  ")
	if runes := []rune(got); len(runes) != companyLogoNameMax {
		t.Fatalf("kept %d runes, want %d", len(runes), companyLogoNameMax)
	}
	if !strings.HasPrefix(got, "ää") || strings.ContainsRune(got, '�') {
		t.Fatalf("the cut broke a character: %q", got[:8])
	}
	if boundedFilename("   ") != "" {
		t.Fatal("a nameless part invented a name")
	}
}
