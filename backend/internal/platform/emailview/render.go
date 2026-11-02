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
const PresentationVersion = 3

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
	brand := strings.TrimSpace(message.Brand)
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
	var main, footer string
	if len(message.Blocks) > 0 {
		main, footer = renderBlocks(message.Blocks, message.ActionPlaceholder)
	} else {
		main = renderText(message)
	}
	return document(language, message.Preheader, header(brand)+titleRow(message.Title, main)+footerRow(footer))
}

func renderText(message Message) string {
	var body strings.Builder
	for paragraph := range strings.SplitSeq(strings.ReplaceAll(message.Text, "\r\n", "\n"), "\n\n") {
		if strings.TrimSpace(paragraph) == "" {
			continue
		}
		if isSection(paragraph, message.Sections) {
			body.WriteString(sectionHeading(strings.TrimSpace(paragraph)))
		} else {
			body.WriteString(renderParagraph(paragraph, message.Title, message.ActionPlaceholder))
		}
	}
	return body.String()
}

// document is the page around the card. The dark sheet repaints role classes
// for clients that honour prefers-color-scheme; the rest keep the light card.
func document(language, preheader, card string) string {
	return `<!doctype html><html lang="` + html.EscapeString(language) + `"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark"><meta name="supported-color-schemes" content="light dark">` +
		`<style>@media(max-width:480px){.pad{padding-left:20px!important;padding-right:20px!important}.facts th,.facts td{display:block!important;width:auto!important}.facts th{padding:10px 0 0!important;border-top:0!important}.facts td{border-top:0!important;padding:2px 0 10px!important}.stats td{display:block!important;width:auto!important;border-left:0!important}}` +
		`@media(prefers-color-scheme:dark){` + darkSheet(":root") + `}</style></head>` +
		`<body data-email-version="` + strconv.Itoa(PresentationVersion) + `"` + look{ink: textPrimary, fill: bgPage}.attrs("margin:0;font:"+body.font()+";overflow-wrap:anywhere;word-wrap:break-word") + `>` +
		`<div style="display:none;max-height:0;overflow:hidden" aria-hidden="true">` + html.EscapeString(preheader) + `</div>` +
		`<table role="presentation" width="100%" cellspacing="0" cellpadding="0"` + look{fill: bgPage}.attrs("") + `><tr><td align="center" style="padding:32px 12px">` +
		`<!--[if mso]><table role="presentation" width="600" cellspacing="0" cellpadding="0"><tr><td><![endif]-->` +
		`<table role="presentation" width="100%" cellspacing="0" cellpadding="0"` + look{fill: pane, edge: paneEdge, shadow: shadowRest}.attrs("table-layout:fixed;max-width:600px;border:1px solid;border-radius:"+rLg.light()) + `>` +
		card + `</table><!--[if mso]></td></tr></table><![endif]--></td></tr></table></body></html>`
}

// header is the wordmark that names who the mail comes from: a brand tile
// with its initials beside the name, over a hairline.
func header(brand string) string {
	band := look{edge: borderSubtle}.attrs("padding:22px 32px;border-bottom:1px solid", "pad")
	tile := `<td width="24" height="24" align="center" valign="middle"` + look{ink: textOnAccentControl, fill: accent}.attrs("width:24px;height:24px;border-radius:6px;font:"+headingXXSmall.font()) + `>` + html.EscapeString(initials(brand)) + `</td>`
	return `<tr><td` + band + `><table role="presentation" cellspacing="0" cellpadding="0"><tr>` + tile +
		`<td valign="middle"` + look{ink: textPrimary}.attrs("padding-left:10px;font:"+wordmark.font()) + `>` + html.EscapeString(brand) + `</td></tr></table></td></tr>`
}

func titleRow(title, main string) string {
	heading := `<h1` + look{ink: textPrimary}.attrs("margin:0 0 16px;font:"+headingXLarge.font()+";overflow-wrap:anywhere") + `>` + html.EscapeString(title) + `</h1>`
	return `<tr><td class="pad" style="padding:32px 32px 16px">` + heading + main + `</td></tr>`
}

func footerRow(footer string) string {
	if footer == "" {
		return ""
	}
	rule := look{ink: textSecondary, edge: borderSubtle}.attrs("border-top:1px solid;padding-top:16px;font:" + bodySmall.font())
	return `<tr><td class="pad" style="padding:0 32px 28px"><div` + rule + `>` + footer + `</div></td></tr>`
}

// monogram is the design system's Avatar at its smallest rung.
func monogram(name string) string {
	return `<table role="presentation" cellspacing="0" cellpadding="0"><tr><td width="28" height="28" align="center" valign="middle"` +
		look{ink: textSecondary, fill: bgWell, edge: paneEdge}.attrs("width:26px;height:26px;border:1px solid;border-radius:9999px;font:"+headingXXSmall.font()) + `>` + html.EscapeString(initials(name)) + `</td></tr></table>`
}

func initials(name string) string {
	var letters []rune
	for word := range strings.FieldsSeq(name) {
		for _, r := range word {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				letters = append(letters, unicode.ToUpper(r))
				break
			}
		}
		if len(letters) == 2 {
			break
		}
	}
	return string(letters)
}

func renderParagraph(text, actionLabel, placeholder string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if facts, ok := factRows(lines); ok {
		return facts
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	if actionURL(last, placeholder) {
		label := actionLabel
		lines = lines[:len(lines)-1]
		if n := len(lines); n > 0 && !actionURL(strings.TrimSpace(lines[n-1]), placeholder) {
			label, lines = strings.TrimSpace(lines[n-1]), lines[:n-1]
		}
		prose := ""
		if len(lines) > 0 {
			prose = paragraph(strings.Join(lines, "\n"))
		}
		return prose + button(label, last, true)
	}
	return paragraph(text)
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
