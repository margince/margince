// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailwire

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// An image the markup shows by content id rides beside the markup in a
// multipart/related part, so the recipient's client loads nothing remote.
func TestAnInlineImageRidesBesideTheMarkup(t *testing.T) {
	t.Parallel()
	logo := []byte("\x89PNG not really")
	raw := Build("rep@example.com", connector.EmailMessage{
		To: []string{"anna@example.com"}, Subject: "Hi", Body: "Hello",
		HTMLBody: `<p>Hello</p><img src="cid:` + connector.SignatureLogoContentID + `">`,
		Inline:   []connector.InlineImage{{ContentID: connector.SignatureLogoContentID, ContentType: "image/png", Body: logo}},
	})
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	alternative := parts(t, msg.Header.Get("Content-Type"), msg.Body)
	if len(alternative) != 2 || !strings.HasPrefix(alternative[1].contentType, "multipart/related") {
		t.Fatalf("alternative parts = %+v, want plain then related", alternative)
	}
	related := parts(t, alternative[1].contentType, bytes.NewReader(alternative[1].body))
	if len(related) != 2 || !strings.HasPrefix(related[0].contentType, "text/html") {
		t.Fatalf("related parts = %+v, want the markup then the image", related)
	}
	image := related[1]
	if image.contentID != "<"+connector.SignatureLogoContentID+">" || image.contentType != "image/png" {
		t.Fatalf("image part = %+v", image)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(image.body), "\r\n", ""))
	if err != nil || !bytes.Equal(decoded, logo) {
		t.Fatalf("image bytes = %q (%v), want the logo", decoded, err)
	}
}

type part struct {
	contentType, contentID string
	body                   []byte
}

func parts(t *testing.T, contentType string, body io.Reader) []part {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("content type %q: %v", contentType, err)
	}
	reader := multipart.NewReader(body, params["boundary"])
	var out []part
	for {
		p, err := reader.NextRawPart()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		data, err := io.ReadAll(p)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		out = append(out, part{contentType: p.Header.Get("Content-Type"), contentID: p.Header.Get("Content-ID"), body: data})
	}
}
