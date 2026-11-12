// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"html"
	"os"
	"strings"
	"testing"
)

// pastedSignature is a mail client's table signature with hostile markup added
// below it: handlers, url() styles, non-https images, a form and a script.
func pastedSignature(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("testdata/pasted_signature.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

func TestAPastedSignatureKeepsItsLayoutColoursAndHttpsImage(t *testing.T) {
	t.Parallel()
	markup, err := sanitizeHTML(pastedSignature(t), signaturePolicy)
	if err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{
		`<table style="font-family:Arial,sans-serif;font-size:11px;color:rgb(51,51,51);width:600px;border-collapse:collapse"><tbody><tr>`,
		`<td style="padding-right:10px;vertical-align:middle;width:80px"><img src="https://example.com/logo.png" width="96" height="21" alt="Company A"></td>`,
		`<div style="width:1px;height:40px;background-color:rgb(255,107,0);margin:0px 5px"></div>`,
		`<span style="color:rgb(149,159,172)">+00 000 0000000</span>`,
		`<a href="https://www.example.com/" style="color:rgb(255,107,0)">`,
		`<font color="#959fac">Company A Ltd`,
		`<hr style="color:rgb(26,26,26);font-family:Arial,sans-serif;font-size:11px;border:none;height:1px;background-color:rgb(255,107,0);margin:16px 0px;width:600px">`,
		`<img src="https://cdn.example/badge.png" width="10" height="10">`,
	} {
		if !strings.Contains(markup, kept) {
			t.Errorf("markup lacks %s", kept)
		}
	}
	for _, removed := range []string{
		"onclick", "onerror", "steal", "srcset", "evil.example", "url(", "track.example", "expression",
		"rgba(0,0,0,0)", "plain.example", "data:", "javascript:", "user:pw", "relative.png", "cid:other",
		"<form", "<input", "<script", "<iframe", "target=", "dir=",
	} {
		if strings.Contains(markup, removed) {
			t.Errorf("markup kept %q: %s", removed, markup)
		}
	}
}

func TestTheBodyPolicyStillReducesAPastedSignatureToItsText(t *testing.T) {
	t.Parallel()
	want, err := os.ReadFile("testdata/pasted_signature.body.html")
	if err != nil {
		t.Fatal(err)
	}
	got, err := SanitizeOutboundHTML(pastedSignature(t))
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("body policy output changed:\n got %q\nwant %q", got, want)
	}
}

func TestEachStyleValueIsCheckedByItsKind(t *testing.T) {
	t.Parallel()
	for style, want := range map[string]string{
		"color:rgb(1, 2, 3)":                             "color:rgb(1, 2, 3)",
		"color:rgba(1,2,3,0.5)":                          "color:rgba(1,2,3,0.5)",
		"color:rgba(1,2,3,0)":                            "",
		"color:transparent":                              "",
		"background-color:rgb(1,2,3,url(x))":             "",
		"border:1px solid rgb(1, 2, 3)":                  "border:1px solid rgb(1, 2, 3)",
		"border:1px solid url(x)":                        "",
		"font-size:2px":                                  "",
		"font-size:10pt":                                 "font-size:10pt",
		"font-family:'Segoe UI', sans-serif":             "",
		"font-family:'Times New Roman', serif":           "font-family:'Times New Roman', serif",
		"font-family:Arial,sans-serif":                   "font-family:Arial,sans-serif",
		"color:currentColor":                             "",
		"color:inherit":                                  "",
		"color:initial":                                  "",
		"color:rgba(1,2,3,0.3)":                          "",
		"color:#0000":                                    "",
		"color:#00000000":                                "",
		"color:Navy":                                     "color:Navy",
		"color:#fff;background-color:#FFF":               "background-color:#FFF",
		"color:rgb(1,2,3);background-color:rgb(1, 2, 3)": "background-color:rgb(1, 2, 3)",
		"color:#000;background-color:#fff":               "color:#000;background-color:#fff",
		"font-family:a\\62 c":                            "",
		"width:expression(alert(1))":                     "",
		"margin:0 auto":                                  "margin:0 auto",
		"padding:1px 2px 3px 4px 5px":                    "",
		"display:none":                                   "",
		"position:absolute;top:0":                        "",
		"WIDTH:10PX":                                     "",
		"vertical-align:middle;width:10%":                "vertical-align:middle;width:10%",
		"background:url(https://t.example/x)":            "",
	} {
		markup, err := sanitizeHTML(`<div style="`+style+`">x</div>`, signaturePolicy)
		if err != nil {
			t.Fatal(err)
		}
		if want == "" && markup != "<div>x</div>" || want != "" && markup != `<div style="`+html.EscapeString(want)+`">x</div>` {
			t.Errorf("style %q gave %s, want %q", style, markup, want)
		}
	}
}

func TestCellsAndImagesKeepOnlyTheirListedAttributes(t *testing.T) {
	t.Parallel()
	markup, err := sanitizeHTML(
		`<table width="600" cellpadding="0" bgcolor="red" onclick="x()"><tr><td colspan="2" valign="top" align="evil" width="50%" class="c">A</td></tr></table>`+
			`<img src="https://cdn.example/a.png" width="96" height="auto" alt="Logo &quot;A&quot;" style="width:96px">`,
		signaturePolicy)
	if err != nil {
		t.Fatal(err)
	}
	want := `<table width="600" cellpadding="0"><tbody><tr><td colspan="2" valign="top" width="50%">A</td></tr></tbody></table>` +
		`<img src="https://cdn.example/a.png" width="96" alt="Logo &#34;A&#34;">`
	if markup != want {
		t.Fatalf("got  %s\nwant %s", markup, want)
	}
}

func TestTheTextPartSeparatesCellsAndNamesImages(t *testing.T) {
	t.Parallel()
	_, text, err := sanitizeSignature(`<table><tr><td><img src="https://cdn.example/a.png" alt="Company A"></td><td>Member A</td></tr><tr><td>Sales</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	if text != "Company A Member A\nSales" {
		t.Fatalf("text = %q", text)
	}
}
