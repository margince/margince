// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package emailview renders product mail without reading records or sending it.
package emailview

import (
	"html"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// PresentationVersion pins queued markup independently of published wording.
const PresentationVersion = 2

// These mirror the light design tokens; the token gate checks their source.
const (
	Paper       = "#eff6f3"
	Surface     = "#fbfcfb"
	Accent      = "#045647"
	Ink         = "#182c28"
	DarkPaper   = "#0c1311"
	DarkSurface = "#161d1a"
	DarkAccent  = "#3aae94"
	DarkInk     = "#edf6f4"
	Border      = "#e3eae6"
	DarkBorder  = "#22352e"
)

// Message holds approved words. Text remains the readable fallback verbatim.
type Message struct {
	Blocks            []Block
	Title             string
	Text              string
	Brand             string
	Language          string
	Preheader         string
	ActionPlaceholder string
	Sections          []string
}

// Render escapes all supplied text. Only standalone web destinations become links.
func Render(message Message) string {
	brand := message.Brand
	if brand == "" {
		brand = "Margince"
	}
	language := message.Language
	if language == "" {
		language = "en"
	}
	if message.Preheader == "" {
		message.Preheader = previewText(message)
	}

	var body strings.Builder
	if len(message.Blocks) > 0 {
		for _, block := range message.Blocks {
			body.WriteString(renderBlock(block, message.ActionPlaceholder))
		}
	} else {
		for paragraph := range strings.SplitSeq(strings.ReplaceAll(message.Text, "\r\n", "\n"), "\n\n") {
			if strings.TrimSpace(paragraph) == "" {
				continue
			}
			if isSection(paragraph, message.Sections) {
				body.WriteString(`<h2 style="font-size:19px;line-height:1.4;margin:28px 0 12px">` + html.EscapeString(strings.TrimSpace(paragraph)) + `</h2>`)
			} else {
				body.WriteString(renderParagraph(paragraph, message.Title, message.ActionPlaceholder))
			}
		}
	}
	return `<!doctype html><html lang="` + html.EscapeString(language) + `"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark"><style>@media(max-width:480px){.facts th,.facts td{display:block!important;width:auto!important}.facts th{padding:10px 0 0!important;border-bottom:0!important}.facts td{padding:0 0 12px!important}}@media(prefers-color-scheme:dark){body,.ground{background:` + DarkPaper + `!important;color:` + DarkInk + `!important}.card{background:` + DarkSurface + `!important;color:` + DarkInk + `!important}a{color:` + DarkAccent + `!important}.facts th,.facts td{border-color:` + DarkBorder + `!important}}</style></head><body data-email-version="` + strconv.Itoa(PresentationVersion) + `" style="margin:0;background:` + Paper + `;color:` + Ink + `;font-family:Arial,Helvetica,sans-serif;overflow-wrap:anywhere;word-wrap:break-word"><div style="display:none;max-height:0;overflow:hidden" aria-hidden="true">` + html.EscapeString(message.Preheader) + `</div><table role="presentation" class="ground" width="100%" cellspacing="0" cellpadding="0"><tr><td align="center" style="padding:24px 12px"><!--[if mso]><table role="presentation" width="600" cellspacing="0" cellpadding="0"><tr><td><![endif]--><table role="presentation" class="card" width="100%" cellspacing="0" cellpadding="0" style="table-layout:fixed;max-width:600px;background:` + Surface + `;border-top:4px solid ` + Accent + `;border-radius:8px"><tr><td style="padding:28px 24px"><div style="font-size:14px;font-weight:bold;letter-spacing:1px;margin-bottom:24px">` + html.EscapeString(brand) + `</div><h1 style="overflow-wrap:anywhere;font-size:24px;line-height:1.3;margin:0 0 24px">` + html.EscapeString(message.Title) + `</h1><div style="font-size:16px;line-height:1.65;overflow-wrap:anywhere">` + body.String() + `</div></td></tr></table><!--[if mso]></td></tr></table><![endif]--></td></tr></table></body></html>`
}

func renderParagraph(text, actionLabel, placeholder string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if facts, ok := factRows(lines); ok {
		return facts
	}
	var out strings.Builder
	out.WriteString(`<p style="margin:0 0 20px">`)
	for i := 0; i < len(lines); i++ {
		if i > 0 {
			out.WriteString("<br>")
		}
		label := strings.TrimSpace(lines[i])
		target := label
		if i+1 == len(lines)-1 && !actionURL(label, placeholder) && actionURL(strings.TrimSpace(lines[i+1]), placeholder) {
			target = strings.TrimSpace(lines[i+1])
			i++
		} else if actionURL(label, placeholder) {
			label = actionLabel
		}
		if actionURL(target, placeholder) && i == len(lines)-1 {
			out.WriteString(`<a style="display:inline-block;color:` + Accent + `;font-weight:bold;text-decoration:underline;overflow-wrap:anywhere" href="` + html.EscapeString(target) + `">` + html.EscapeString(label) + `</a>`)
		} else {
			out.WriteString(html.EscapeString(lines[i]))
		}
	}

	out.WriteString("</p>")
	return out.String()
}

func actionURL(text, placeholder string) bool {
	if placeholder != "" && text == placeholder {
		return true
	}
	parsed, err := url.Parse(text)
	return err == nil && parsed.Host != "" && parsed.User == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && !strings.ContainsAny(text, "\r\n\t ")
}

func isSection(text string, sections []string) bool {
	for _, section := range sections {
		if strings.TrimSpace(text) == section {
			return true
		}
	}
	return false
}

func factRows(lines []string) (string, bool) {
	if len(lines) < 2 {
		return "", false
	}
	facts := make([]Fact, 0, len(lines))
	for _, line := range lines {
		label, value, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(value) == "" || !strings.ContainsFunc(label, unicode.IsLetter) {
			return "", false
		}
		facts = append(facts, Fact{Label: strings.TrimSpace(label), Value: strings.TrimSpace(value)})
	}
	return renderFacts(facts), true
}

func previewText(message Message) string {
	for paragraph := range strings.SplitSeq(strings.ReplaceAll(message.Text, "\r\n", "\n"), "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" || paragraph == message.Title || isSection(paragraph, message.Sections) || strings.ContainsAny(paragraph, "\n") || strings.Contains(paragraph, "://") || (message.ActionPlaceholder != "" && strings.Contains(paragraph, message.ActionPlaceholder)) {
			continue
		}
		return paragraph
	}
	return ""
}
