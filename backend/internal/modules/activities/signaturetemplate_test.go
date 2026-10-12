// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestATemplateSignsWithTheSendersOwnValuesEscaped(t *testing.T) {
	t.Parallel()
	store := (&Store{}).
		WithSignature(&stubSignature{
			body:     "my own text",
			template: `<p><b>{name}</b><br>{title}<br><span style="color:#2a7;font-size:12px;background:url(x)">{phone}</span></p><script>alert(1)</script>`,
			title:    "Head of <Sales>",
			phone:    "+49 30 1234",
		}).
		WithSenderName(&stubSenderName{name: "Anna Example"})

	sign, err := store.signOffAs(humanCtx(ids.NewV7()), "Hello", "Hi", "Anna Example")
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	if sign.Kind != SignOffTemplate {
		t.Fatalf("kind = %s, want the template over the sender's own text", sign.Kind)
	}
	for _, want := range []string{"<b>Anna Example</b>", "Head of &lt;Sales&gt;", `style="color:#2a7;font-size:12px"`} {
		if !strings.Contains(sign.HTML, want) {
			t.Errorf("markup %q lacks %q", sign.HTML, want)
		}
	}
	for _, refused := range []string{"<script", "alert", "url("} {
		if strings.Contains(sign.HTML, refused) {
			t.Errorf("markup %q kept %q", sign.HTML, refused)
		}
	}
	if sign.Text != "Anna Example\nHead of <Sales>\n+49 30 1234" {
		t.Errorf("text part = %q, want one line per line break", sign.Text)
	}
}

func TestWithoutATemplateTheSendersOwnTextSigns(t *testing.T) {
	t.Parallel()
	store := (&Store{}).WithSignature(&stubSignature{body: "Anna\nCompany A"})
	sign, err := store.signOffAs(humanCtx(ids.NewV7()), "Hello", "Hi", "Anna")
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	if sign.Kind != SignOffSignature || sign.HTML != "" || sign.Text != "Anna\nCompany A" {
		t.Fatalf("sign-off = %+v, want the sender's own plain text", sign)
	}
}

func TestATemplateSignOffLandsInTheMarkupPart(t *testing.T) {
	t.Parallel()
	got := signedHTML("<p>Hello</p>", SignOff{Text: "Anna", HTML: "<p><b>Anna</b></p>", Kind: SignOffTemplate}, sendDeliverability{})
	if !strings.Contains(got, "<p><b>Anna</b></p>") || strings.Contains(got, "<p>Anna</p>") {
		t.Fatalf("markup = %q, want the template's markup rather than the text escaped", got)
	}
}

// {logo} is the workspace logo embedded by content id, and only when there is
// one. Any other image, a remote one above all, is dropped.
func TestTheLogoPlaceholderEmbedsTheWorkspaceLogoAndNothingRemote(t *testing.T) {
	t.Parallel()
	template := `<p>{logo}<img src="https://tracker.example/pixel.png">{name}</p>`
	with, _, err := renderSignatureTemplate(template, signatureValues{Name: "Anna", HasLogo: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(with, `src="cid:signature-logo@margince"`) {
		t.Errorf("markup %q lacks the embedded logo", with)
	}
	if strings.Contains(with, "tracker.example") {
		t.Errorf("markup %q kept a remote image", with)
	}
	without, _, err := renderSignatureTemplate(template, signatureValues{Name: "Anna"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(without, "<img") {
		t.Errorf("markup %q shows an image though the workspace has no logo", without)
	}
}

// The text part keeps each list item, quote and rule-separated part on its
// own line, as the markup shows them.
func TestTheTextPartBreaksWhereTheMarkupBreaks(t *testing.T) {
	t.Parallel()
	_, text, err := renderSignatureTemplate(
		`<b>{name}</b><ul><li>{title}</li><li>{phone}</li></ul><hr>Gradion<blockquote>Quote</blockquote>`,
		signatureValues{Name: "Anna", Title: "Head of Sales", Phone: "+49 30 1234"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "Anna\nHead of Sales\n+49 30 1234\nGradion\nQuote"; text != want {
		t.Fatalf("text part = %q, want %q", text, want)
	}
}

// A preview renders the form's unsaved values over the stored ones, and the
// logo key rides only with markup that shows the logo.
func TestAPreviewRendersTheUnsavedDraft(t *testing.T) {
	t.Parallel()
	store := (&Store{}).
		WithSignature(&stubSignature{template: "<p>{name}</p>", title: "Stored title", logoKey: "logos/a.png"}).
		WithSenderName(&stubSenderName{name: "Anna"})
	ctx := humanCtx(ids.NewV7())

	stored, err := store.previewSignOff(ctx, "", "", signatureDraft{})
	if err != nil || stored.Text != "Anna" || stored.LogoKey != "" {
		t.Fatalf("stored preview = %+v, err %v; want the stored template and no logo", stored, err)
	}
	template, title := "<p>{logo}{name}<br>{title}</p>", "Draft title"
	drafted, err := store.previewSignOff(ctx, "", "", signatureDraft{Template: &template, Title: &title})
	if err != nil {
		t.Fatal(err)
	}
	if drafted.Text != "Anna\nDraft title" || drafted.LogoKey != "logos/a.png" || !strings.Contains(drafted.HTML, signatureLogoSrc) {
		t.Fatalf("drafted preview = %+v, want the draft's template and title with the logo", drafted)
	}
}
