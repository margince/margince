// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/testsmtp"
)

// Mail the system relay sends is headed by the installation's own company:
// its stored logo as an inline image, else its name. The read runs with no
// signed-in member, as a password reset does.
func TestTheSystemMailLetterheadIsTheInstallationsOwnCompany(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	logoKey := "logos/" + ids.NewV7().String() + ".png"
	putPNG(t, blob, logoKey)
	anchor := ids.NewV7()
	e.WsExec(t, `INSERT INTO company (id, display_name, is_anchor, logo_object_key, logo_origin, source, captured_by)
		VALUES ($1, 'Northstar GmbH', true, $2, 'upload', 'manual', 'human:x')`, anchor, logoKey)
	source := newMailLetterhead(e.Pool, blob)

	head, inline := source.Letterhead(context.Background(), "de")
	if head.Name != "Northstar GmbH" || head.Attribution != mailcopy.For("de").PoweredBy {
		t.Fatalf("letterhead = %+v, want the company's name over the German attribution", head)
	}
	if head.LogoContentID != connector.LetterheadLogoContentID || len(inline) != 1 ||
		inline[0].ContentID != connector.LetterheadLogoContentID || inline[0].ContentType != "image/png" || len(inline[0].Body) == 0 {
		t.Fatalf("logo = %q with %d inline parts, want the company logo attached under the letterhead id", head.LogoContentID, len(inline))
	}

	// The logo removed from the company: the name heads the mail alone.
	e.WsExec(t, `UPDATE company SET logo_object_key = NULL, logo_origin = NULL WHERE id = $1`, anchor)
	head, inline = source.Letterhead(context.Background(), "en")
	if head.Name != "Northstar GmbH" || head.LogoContentID != "" || len(inline) != 0 {
		t.Fatalf("without a logo the letterhead is %+v with %d parts, want the name alone", head, len(inline))
	}

	// A key whose object is gone from the store reads as no logo, not as a
	// refused send.
	e.WsExec(t, `UPDATE company SET logo_object_key = $2, logo_origin = 'upload' WHERE id = $1`, anchor, "logos/missing.png")
	head, inline = source.Letterhead(context.Background(), "en")
	if head.Name != "Northstar GmbH" || head.LogoContentID != "" || len(inline) != 0 {
		t.Fatalf("with its object gone the letterhead is %+v with %d parts, want the name alone", head, len(inline))
	}
}

func putPNG(t *testing.T, blob blobstore.Store, key string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for x := range 40 {
		for y := range 20 {
			img.Set(x, y, color.RGBA{R: 4, G: 86, B: 71, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := blob.Put(context.Background(), key, &buf, int64(buf.Len()), "image/png"); err != nil {
		t.Fatal(err)
	}
}

// failingBlob is an object store that cannot be read: every Get fails.
type failingBlob struct{ blobstore.Store }

func (failingBlob) Get(context.Context, string) (io.ReadCloser, blobstore.Object, error) {
	return nil, blobstore.Object{}, errors.New("the object store is unreachable")
}

// A password reset still goes out when the logo cannot be read: the mail is
// headed by the company name, and the reset link is in it. This is the send
// the reset handler makes (identity/reset.go): the account message through
// mailer.SendProduct, with no signed-in member.
func TestAResetMailGoesOutWhenTheLogoStoreFails(t *testing.T) {
	e := integration.Setup(t)
	logoKey := "logos/" + ids.NewV7().String() + ".png"
	e.WsExec(t, `INSERT INTO company (id, display_name, is_anchor, logo_object_key, logo_origin, source, captured_by)
		VALUES ($1, 'Northstar GmbH', true, $2, 'upload', 'manual', 'human:x')`, ids.NewV7(), logoKey)
	relay := testsmtp.Start(t, false)
	host, port := relay.Address(t)
	smtp := mailer.SMTP{Host: host, Port: port, FromAddress: "crm@example.test",
		Letterhead: newMailLetterhead(e.Pool, failingBlob{blobstore.NewMemory()})}
	words := mailcopy.For("en")
	link := "https://crm.example.test/#/reset-password?token=abc"
	if err := mailer.SendProduct(context.Background(), smtp, "member@example.test", words.ResetSubject,
		words.AccountMessage(false, "", "", link)); err != nil {
		t.Fatalf("the reset mail was not sent: %v", err)
	}
	received := relay.Received()
	if !strings.Contains(received, "Northstar GmbH") || !strings.Contains(received, "reset-password?token=3Dabc") && !strings.Contains(received, "reset-password?token=abc") {
		t.Fatalf("the reset mail lacks the company name or the link:\n%s", received)
	}
	if strings.Contains(received, "cid:"+connector.LetterheadLogoContentID) {
		t.Fatal("the mail refers to a logo it could not attach")
	}
}

// An installation that works under two companies heads its mail with its own
// company's logo, never another's.
func TestOnlyTheAnchorCompanysLogoHeadsTheMail(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	anchorKey, otherKey := "logos/anchor.png", "logos/other.png"
	putPNG(t, blob, anchorKey)
	putWidePNG(t, blob, otherKey)
	e.WsExec(t, `INSERT INTO company (id, display_name, is_anchor, logo_object_key, logo_origin, source, captured_by)
		VALUES ($1, 'Northstar GmbH', true, $2, 'upload', 'manual', 'human:x'),
		       ($3, 'Southwind Ltd', false, $4, 'upload', 'manual', 'human:x')`, ids.NewV7(), anchorKey, ids.NewV7(), otherKey)
	want, ok, err := newSignatureLogo(blob).SignatureLogo(context.Background(), anchorKey)
	if err != nil || !ok {
		t.Fatalf("sizing the anchor logo: ok=%v err=%v", ok, err)
	}
	head, inline := newMailLetterhead(e.Pool, blob).Letterhead(context.Background(), "en")
	if head.Name != "Northstar GmbH" || len(inline) != 1 || !bytes.Equal(inline[0].Body, want.Body) {
		t.Fatalf("letterhead %q with %d parts; want the anchor company and its logo alone", head.Name, len(inline))
	}
}

// Product mail and privacy notices speak as the same company: the
// installation's own company, even where the installation-name setting still
// holds an older name.
func TestPrivacyNoticesAndProductMailNameTheSameCompany(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO company (id, display_name, is_anchor, source, captured_by)
		VALUES ($1, 'Northstar GmbH', true, 'manual', 'human:x')`, ids.NewV7())
	head, _ := newMailLetterhead(e.Pool, nil).Letterhead(context.Background(), "en")
	var body string
	if err := InstallationDB(e.Pool).Tx(e.Admin(), func(tx pgx.Tx) error {
		var err error
		_, body, err = presentControllerMailTx(e.Admin(), tx, consent.Rendered{Body: "We recorded your request.", Language: "en", Subject: "Your request"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if head.Name != "Northstar GmbH" || !strings.HasPrefix(body, head.Name+"\n\n") {
		t.Fatalf("product mail is headed %q and the privacy notice opens %q; want one name", head.Name, body)
	}
}

func putWidePNG(t *testing.T, blob blobstore.Store, key string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 80, 10))
	for x := range 80 {
		img.Set(x, 5, color.RGBA{R: 200, G: 20, B: 20, A: 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := blob.Put(context.Background(), key, &buf, int64(buf.Len()), "image/png"); err != nil {
		t.Fatal(err)
	}
}
