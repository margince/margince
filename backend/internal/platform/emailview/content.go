// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package emailview

import (
	"html"
	"strings"
)

// Fact is a named value whose meaning is independent of its position.
type Fact struct{ Label, Value string }

// Block states presentation intent separately from customer-authored words.
type Block struct {
	Heading     string
	Text        string
	ActionLabel string
	ActionURL   string
	Facts       []Fact
}

// Content builds matching text and markup from the same semantic blocks.
type Content struct{ Blocks []Block }

// Heading starts a section without treating record titles as formatting instructions.
func (c *Content) Heading(text string) { c.Blocks = append(c.Blocks, Block{Heading: text}) }

// Paragraph adds words that cannot become a heading or a link.
func (c *Content) Paragraph(text string) {
	if strings.TrimSpace(text) != "" {
		c.Blocks = append(c.Blocks, Block{Text: text})
	}
}

// Action names what a destination does in both alternatives.
func (c *Content) Action(label, target string) {
	if label != "" && target != "" {
		c.Blocks = append(c.Blocks, Block{ActionLabel: label, ActionURL: target})
	}
}

// Facts keeps metric labels next to their values in every client.
func (c *Content) Facts(rows ...Fact) {
	if len(rows) > 0 {
		c.Blocks = append(c.Blocks, Block{Facts: rows})
	}
}

// Append combines editions without reintroducing another primary title.
func (c *Content) Append(other Content) { c.Blocks = append(c.Blocks, other.Blocks...) }

// String is the plain-text alternative, derived from the presented blocks.
func (c Content) String() string {
	var paragraphs []string
	for _, block := range c.Blocks {
		switch {
		case block.Heading != "":
			paragraphs = append(paragraphs, block.Heading)
		case block.ActionURL != "":
			paragraphs = append(paragraphs, block.ActionLabel+"\n"+block.ActionURL)
		case len(block.Facts) > 0:
			var rows []string
			for _, row := range block.Facts {
				rows = append(rows, row.Label+": "+row.Value)
			}
			paragraphs = append(paragraphs, strings.Join(rows, "\n"))
		default:
			paragraphs = append(paragraphs, block.Text)
		}
	}
	return strings.Join(paragraphs, "\n\n")
}

// Message gives a document its primary title and language.
func (c Content) Message(title, language, preview string) Message {
	return Message{Title: title, Language: language, Preheader: preview, Text: title + "\n\n" + c.String(), Blocks: c.Blocks}
}

func renderBlock(block Block, placeholder string) string {
	switch {
	case block.Heading != "":
		return `<h2 style="font-size:19px;line-height:1.4;margin:28px 0 12px">` + html.EscapeString(block.Heading) + `</h2>`
	case block.ActionURL != "":
		if !actionURL(block.ActionURL, placeholder) {
			return `<p>` + html.EscapeString(block.ActionLabel+" "+block.ActionURL) + `</p>`
		}
		return renderParagraph(block.ActionLabel+"\n"+block.ActionURL, block.ActionLabel, placeholder)
	case len(block.Facts) > 0:
		return renderFacts(block.Facts)

	default:
		return `<p style="margin:0 0 20px">` + strings.ReplaceAll(html.EscapeString(block.Text), "\n", "<br>") + `</p>`
	}
}

func renderFacts(facts []Fact) string {
	var rows strings.Builder
	for _, fact := range facts {
		rows.WriteString(`<tr><th scope="row" align="left" style="padding:10px 16px 10px 0;font-weight:600;vertical-align:top;border-bottom:1px solid ` + Border + `">` + html.EscapeString(fact.Label) + `</th><td style="padding:10px 0;vertical-align:top;border-bottom:1px solid ` + Border + `">` + html.EscapeString(fact.Value) + `</td></tr>`)
	}
	return `<table class="facts" width="100%" cellspacing="0" cellpadding="0" style="border-collapse:collapse;margin:0 0 24px">` + rows.String() + `</table>`
}

// Preview chooses actual message context without putting URLs in the inbox preview.
func (c Content) Preview(fallback string) string {
	for _, block := range c.Blocks {
		line, _, _ := strings.Cut(strings.TrimSpace(block.Text), "\n")
		if line != "" && !strings.Contains(line, "://") {
			return line
		}
	}
	return fallback
}

// AppendContent extends both alternatives together while retaining the primary title.
func (m *Message) AppendContent(content Content) {
	m.Blocks = append(m.Blocks, content.Blocks...)
	m.Text = m.Title + "\n\n" + (Content{Blocks: m.Blocks}).String()
}
