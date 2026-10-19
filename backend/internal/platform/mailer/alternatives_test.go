// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailer

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
)

func TestProductMailCarriesReadableTextAndEscapedMarkup(t *testing.T) {
	relay := startFakeRelay(t, false)
	host, port := relay.Address(t)
	sender := SMTP{Host: host, Port: port, FromAddress: "product@example.test"}
	text := "The buyer needs a revised quote.\n\nhttps://example.test/#/deals/one"
	if err := sender.Send(context.Background(), "reader@example.test", "Your next customer action", text); err != nil {
		t.Fatal(err)
	}
	message, err := mail.ReadMessage(strings.NewReader(relay.Received()))
	if err != nil {
		t.Fatal(err)
	}
	kind, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/alternative" {
		t.Fatalf("content type = %s, %v", kind, err)
	}
	reader := multipart.NewReader(message.Body, params["boundary"])
	var bodies []string
	var types []string
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(quotedprintable.NewReader(part))
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, string(content))
		types = append(types, part.Header.Get("Content-Type"))
	}
	if len(bodies) != 2 || !strings.HasPrefix(types[0], "text/plain") || !strings.HasPrefix(types[1], "text/html") {
		t.Fatalf("alternatives = %v", types)
	}
	if strings.ReplaceAll(bodies[0], "\r\n", "\n") != text {
		t.Fatal("plain alternative changed the words")
	}
	if !strings.Contains(bodies[1], "The buyer needs a revised quote.") || !strings.Contains(bodies[1], `href="https://example.test/#/deals/one"`) {
		t.Fatal("markup lost the fact or destination")
	}
}
