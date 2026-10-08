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
