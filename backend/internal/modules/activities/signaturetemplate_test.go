// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
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

// {imprint} is the sender's entity imprint: escaped like every other value,
// with its lines kept as lines in both parts.
func TestTheImprintPlaceholderKeepsItsLinesAndIsEscaped(t *testing.T) {
	t.Parallel()
	store := (&Store{}).WithSignature(&stubSignature{
		template: "<p>{name}</p><p>{imprint}</p>",
		imprint:  "Acme Wien GmbH\nFN 123456 a <HG Wien>",
	})
	sign, err := store.signOffAs(humanCtx(ids.NewV7()), "Hello", "Hi", "Anna")
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	if !strings.Contains(sign.HTML, "Acme Wien GmbH<br>FN 123456 a &lt;HG Wien&gt;") {
		t.Errorf("markup %q lacks the escaped imprint on two lines", sign.HTML)
	}
	if sign.Text != "Anna\nAcme Wien GmbH\nFN 123456 a <HG Wien>" {
		t.Errorf("text part = %q, want the imprint on its own lines", sign.Text)
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

func TestPersonalMarkupUsesTheSignaturePolicyWithoutExpandingPlaceholders(t *testing.T) {
	t.Parallel()
	store := (&Store{}).WithSignature(&stubSignature{
		html:     `<p onclick="bad()"><b>Personal</b><br><i>{name}</i> <u>{title}</u> {phone} {logo}<span style="color:#2a7;font-size:12px;background:url(x)">Team</span><a href="javascript:bad()">Link</a><img src="https://tracker.test/pixel"><script>bad()</script></p>`,
		template: "<p>Company</p>", body: "Saved plain text",
	})
	sign, err := store.signOffAs(humanCtx(ids.NewV7()), "Hello", "Hi", "Anna")
	if err != nil {
		t.Fatal(err)
	}
	if sign.Kind != SignOffSignature || sign.Text != "Personal\n{name} {title} {phone} {logo}TeamLink" {
		t.Fatalf("sign = %+v", sign)
	}
	for _, forbidden := range []string{"Company", "Saved plain text", "onclick", "javascript:", "tracker.test", "<script", "bad()", "background"} {
		if strings.Contains(sign.HTML, forbidden) {
			t.Errorf("markup kept %q: %s", forbidden, sign.HTML)
		}
	}
	for _, kept := range []string{"<b>Personal</b>", "<i>{name}</i>", "<u>{title}</u>", `style="color:#2a7;font-size:12px"`} {
		if !strings.Contains(sign.HTML, kept) {
			t.Errorf("markup lacks %q: %s", kept, sign.HTML)
		}
	}
}

func TestPersonalMarkupFallsThroughOnlyWhenItHasNoVisibleText(t *testing.T) {
	t.Parallel()
	for _, own := range []string{"", "   ", "<script>bad()</script>", `<img src="https://tracker.test/pixel">`, "<p> </p>"} {
		for _, fallback := range []struct {
			template, body string
			kind           SignOffKind
			text           string
		}{
			{"<p>Company</p>", "Saved", SignOffTemplate, "Company"},
			{"", "Saved", SignOffSignature, "Saved"},
			{"", "", SignOffClosing, "Best regards,\nAnna"},
		} {
			store := (&Store{}).WithSignature(&stubSignature{html: own, template: fallback.template, body: fallback.body})
			sign, err := store.signOffAs(humanCtx(ids.NewV7()), "Hello, I am sending the requested documents for your review.", "Documents", "Anna")
			if err != nil {
				t.Fatal(err)
			}
			if sign.Kind != fallback.kind || sign.Text != fallback.text {
				t.Errorf("own %q: sign %+v, want %s %q", own, sign, fallback.kind, fallback.text)
			}
		}
	}
}

func TestPersonalMarkupPreviewOverridesAndClearingShareTheSendDecision(t *testing.T) {
	t.Parallel()
	store := (&Store{}).WithSignature(&stubSignature{html: "<p>Saved personal</p>", template: "<p>Company</p>", logoKey: "logos/company.png"})
	for _, test := range []struct {
		markup     string
		kind       SignOffKind
		text, logo string
	}{
		{"<p>Draft personal</p>", SignOffSignature, "Draft personal", ""},
		{"", SignOffTemplate, "Company", ""},
		{"<p>Personal</p>" + signatureLogoTag, SignOffSignature, "Personal", "logos/company.png"},
	} {
		sign, err := store.signOffDrafted(humanCtx(ids.NewV7()), "", "", "Anna", signatureDraft{HTML: &test.markup})
		if err != nil {
			t.Fatal(err)
		}
		if sign.Kind != test.kind || sign.Text != test.text || sign.LogoKey != test.logo {
			t.Errorf("sign = %+v, want %+v", sign, test)
		}
	}
}

func TestMarkupWithoutAnAvailableLogoDoesNotSendABrokenEmbeddedImage(t *testing.T) {
	t.Parallel()
	for _, signature := range []*stubSignature{
		{html: "<p>Personal</p>" + signatureLogoTag},
		{template: "<p>Company</p>" + signatureLogoTag},
	} {
		sign, err := (&Store{}).WithSignature(signature).signOffAs(humanCtx(ids.NewV7()), "Hello", "Hello", "Anna")
		if err != nil {
			t.Fatal(err)
		}
		if sign.LogoKey != "" || strings.Contains(sign.HTML, "<img") {
			t.Fatalf("sign = %+v", sign)
		}
	}
}

func TestPersonalMarkupSignsOnceAboveDisclosuresAndTheUnsubscribeFooter(t *testing.T) {
	t.Parallel()
	sign, err := (&Store{}).WithSignature(&stubSignature{html: "<p><b>Personal</b></p>"}).signOffAs(humanCtx(ids.NewV7()), "Hello", "Hello", "Anna")
	if err != nil {
		t.Fatal(err)
	}
	derived := sendDeliverability{
		links:       unsubscribeLinks{unsubscribe: "https://app.test/unsubscribe/token", manage: "https://app.test/preferences/token"},
		disclosures: []DisclosureLine{{Kind: "identity", Text: "Controller identity"}},
		words:       mailcopy.For("en"),
	}
	markup := signedHTML("<p>Body</p>", sign, derived)
	if strings.Count(markup, sign.HTML) != 1 || strings.Count(markup, "Controller identity") != 1 {
		t.Fatalf("markup = %s", markup)
	}
	if strings.Index(markup, sign.HTML) > strings.Index(markup, "Controller identity") || strings.Index(markup, "Controller identity") > strings.Index(markup, derived.links.unsubscribe) {
		t.Fatalf("sign-off, disclosure and footer out of order: %s", markup)
	}
}

func TestPastedSignatureBlocksKeepTheirReadableBoundaries(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"<div>Mira Voss</div><div>Head of Sales</div><table><tr><td>+49 711</td><td>mira@example.com</td></tr></table>",
		"<h2>Mira Voss</h2><table><tr><td><p>Head of Sales</p></td></tr><tr><td>+49 711</td><td>mira@example.com</td></tr></table>",
	} {
		markup, text, err := signatureMarkup(source)
		if err != nil {
			t.Fatal(err)
		}
		want := "Mira Voss\nHead of Sales\n+49 711 mira@example.com"
		if text != want || !strings.Contains(markup, "<br>") {
			t.Fatalf("markup=%q text=%q, want %q", markup, text, want)
		}
	}
}

func TestSignatureTextKeepsLinkDestinationsAndMarkupKeepsTextVisible(t *testing.T) {
	t.Parallel()
	markup, text, err := signatureMarkup(`<span style="color:transparent">Mira</span><br><a href="https://calendar.example/mira">Book a call</a><br><a href="mailto:mira@example.com">mira@example.com</a><img src="data:image/png;base64,AAAA">`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(markup, "transparent") || strings.Contains(markup, "data:") {
		t.Fatalf("hidden text or unsupported image survived: %s", markup)
	}
	if text != "Mira\nBook a call <https://calendar.example/mira>\nmira@example.com" {
		t.Fatalf("text alternatives lost readable content: %q", text)
	}
}
