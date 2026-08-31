// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func oneClickRequest(body, contentType string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/public/preferences/tok/unsubscribe", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req
}

// What Gmail sends.
func TestOneClickAcceptsTheURLEncodedBody(t *testing.T) {
	if err := requireOneClickBody(
		oneClickRequest("List-Unsubscribe=One-Click", "application/x-www-form-urlencoded"),
	); err != nil {
		t.Errorf("refused the RFC 8058 form body: %v", err)
	}
}

// What RFC 8058 §3.2 actually names FIRST. A check written against Gmail
// alone passes its own tests and turns away a conforming receiver.
func TestOneClickAcceptsTheMultipartBody(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField(oneClickField, oneClickValue); err != nil {
		t.Fatalf("build the multipart body: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close the multipart body: %v", err)
	}
	if err := requireOneClickBody(oneClickRequest(buf.String(), w.FormDataContentType())); err != nil {
		t.Errorf("refused a multipart one-click body: %v", err)
	}
}

// The product's own page and any operator with curl send no body at all.
func TestOneClickAcceptsAnEmptyBody(t *testing.T) {
	if err := requireOneClickBody(oneClickRequest("", "")); err != nil {
		t.Errorf("refused an empty body: %v", err)
	}
	if err := requireOneClickBody(oneClickRequest("", "application/x-www-form-urlencoded")); err != nil {
		t.Errorf("refused an empty form body: %v", err)
	}
	// A caller that declares a content type it never fills — e.g. a client
	// that always sets Content-Type: application/json — is still an absent
	// body, not a malformed one.
	if err := requireOneClickBody(oneClickRequest("", "application/json")); err != nil {
		t.Errorf("refused an empty body carrying an unrelated content type: %v", err)
	}
}

// A charset parameter is ordinary and must not change the verdict.
func TestOneClickToleratesAContentTypeParameter(t *testing.T) {
	if err := requireOneClickBody(
		oneClickRequest("List-Unsubscribe=One-Click", "application/x-www-form-urlencoded; charset=utf-8"),
	); err != nil {
		t.Errorf("refused a parameterised content type: %v", err)
	}
}

// Present and saying something else is the one case that is refused.
func TestOneClickRefusesABodyThatIsNotOneClick(t *testing.T) {
	cases := []struct {
		name, body, contentType string
	}{
		{"a form without the pair", "Something=Else", "application/x-www-form-urlencoded"},
		{"the wrong value", "List-Unsubscribe=Two-Click", "application/x-www-form-urlencoded"},
		{"json", `{"List-Unsubscribe":"One-Click"}`, "application/json"},
		{"multipart with no boundary", "whatever", "multipart/form-data"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := requireOneClickBody(oneClickRequest(c.body, c.contentType)); err == nil {
				t.Error("admitted a body that is not the RFC 8058 one-click pair")
			}
		})
	}
}

// A single short field has a small ceiling; anything past it is not a
// mailbox provider following the RFC.
func TestOneClickRefusesAnOversizedBody(t *testing.T) {
	huge := "List-Unsubscribe=One-Click&pad=" + strings.Repeat("x", oneClickBodyLimit)
	if err := requireOneClickBody(oneClickRequest(huge, "application/x-www-form-urlencoded")); err == nil {
		t.Error("admitted an oversized one-click body")
	}
}
