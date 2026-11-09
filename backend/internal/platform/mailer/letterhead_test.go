// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailer

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// fixedLetterhead is a source that heads every mail with one name.
type fixedLetterhead struct{}

func (fixedLetterhead) Letterhead(context.Context, string) (emailview.Letterhead, []connector.InlineImage) {
	return emailview.Letterhead{Name: "Northstar GmbH", Attribution: "Powered by Margince"}, nil
}

// logoLetterhead heads every mail with a logo, attached as an inline image.
type logoLetterhead struct{ language *string }

func (l logoLetterhead) Letterhead(_ context.Context, language string) (emailview.Letterhead, []connector.InlineImage) {
	if l.language != nil {
		*l.language = language
	}
	return emailview.Letterhead{Name: "Northstar GmbH", Attribution: "Powered by Margince", LogoContentID: connector.LetterheadLogoContentID},
		[]connector.InlineImage{{ContentID: connector.LetterheadLogoContentID, ContentType: "image/png", Body: []byte("\x89PNG logo")}}
}

func TestProductMailCarriesTheLogoAsARelatedPart(t *testing.T) {
	relay := startFakeRelay(t, false)
	host, port := relay.Address(t)
	var language string
	sender := SMTP{Host: host, Port: port, FromAddress: "product@example.test", Letterhead: logoLetterhead{language: &language}}
	message := emailview.Message{Title: "Reset your password", Text: "Choose a new password.", Language: "de"}
	if err := SendProduct(context.Background(), sender, "reader@example.test", "Reset your password", message); err != nil {
		t.Fatal(err)
	}
	if language != "de" {
		t.Errorf("the letterhead was read in %q, want the mail's language de", language)
	}
	parsed, err := mail.ReadMessage(strings.NewReader(relay.Received()))
	if err != nil {
		t.Fatal(err)
	}
	related, markup, plain := relatedPart(t, parsed)
	if want := "Northstar GmbH\nPowered by Margince\n\nChoose a new password."; strings.ReplaceAll(plain, "\r\n", "\n") != want {
		t.Fatalf("the text alternative reads %q, want the letterhead over the words: %q", plain, want)
	}
	if !strings.Contains(markup, `src="cid:`+connector.LetterheadLogoContentID+`"`) || !strings.Contains(markup, `alt="Northstar GmbH"`) {
		t.Fatalf("the markup does not show the attached logo by its content id:\n%s", markup)
	}
	if !strings.Contains(markup, "Powered by Margince") {
		t.Fatal("the markup lost the attribution line")
	}
	if related != "<"+connector.LetterheadLogoContentID+">" {
		t.Fatalf("the image part's Content-ID is %q, want the one the markup names", related)
	}
}

// relatedPart walks multipart/alternative down to its multipart/related half
// and returns the image part's Content-ID, the markup beside it, and the text
// alternative.
func relatedPart(t *testing.T, message *mail.Message) (string, string, string) {
	t.Helper()
	kind, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/alternative" {
		t.Fatalf("content type = %s, %v", kind, err)
	}
	alternatives := multipart.NewReader(message.Body, params["boundary"])
	var plain string
	for {
		part, err := alternatives.NextPart()
		if errors.Is(err, io.EOF) {
			t.Fatal("no multipart/related alternative carries the logo")
		}
		if err != nil {
			t.Fatal(err)
		}
		kind, params, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err == nil && kind == "text/plain" {
			body, err := io.ReadAll(part)
			if err != nil {
				t.Fatal(err)
			}
			plain = string(body)
			continue
		}
		if err != nil || kind != "multipart/related" {
			continue
		}
		var contentID, markup string
		inner := multipart.NewReader(part, params["boundary"])
		for {
			sub, err := inner.NextPart()
			if errors.Is(err, io.EOF) {
				return contentID, markup, plain
			}
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(sub)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(sub.Header.Get("Content-Type"), "text/html") {
				markup = string(body)
			}
			if id := sub.Header.Get("Content-ID"); id != "" {
				contentID = id
			}
		}
	}
}

// A relay built without a letterhead source refuses product mail rather than
// mailing as nobody. The refusal comes before any connection is tried.
func TestAnSMTPWithoutALetterheadSourceRefusesProductMail(t *testing.T) {
	sender := SMTP{Host: "127.0.0.1", Port: 1, FromAddress: "product@example.test"}
	ctx := context.Background()
	if err := sender.Send(ctx, "reader@example.test", "Subject", "Body"); !errors.Is(err, ErrNoLetterhead) {
		t.Errorf("Send answered %v, want ErrNoLetterhead", err)
	}
	if err := sender.SendBranded(ctx, "reader@example.test", "Subject", emailview.Message{Title: "Subject", Text: "Body"}); !errors.Is(err, ErrNoLetterhead) {
		t.Errorf("SendBranded answered %v, want ErrNoLetterhead", err)
	}
	if err := SendProduct(ctx, sender, "reader@example.test", "Subject", emailview.Message{Title: "Subject", Text: "Body"}); !errors.Is(err, ErrNoLetterhead) {
		t.Errorf("SendProduct answered %v, want ErrNoLetterhead", err)
	}
}
