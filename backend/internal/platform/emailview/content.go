// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package emailview

import (
	"strings"
)

// Fact is a named value whose meaning is independent of its position.
type Fact struct{ Label, Value string }

// Tone is a status colour from the design system's Badge.
type Tone string

// The tones a Badge or a note may take.
const (
	Neutral Tone = ""
	Warning Tone = "warning"
	Info    Tone = "info"
	Success Tone = "success"
	Danger  Tone = "danger"
)

// Line is one fact about an item. A toned line is drawn as a badge.
type Line struct {
	Text string
	Tone Tone
}

// Item is one record a message asks the reader to look at. An ItemAction
// that directly follows it becomes its link.
type Item struct {
	Title string
	Lines []Line
	// Verb is what the reader does with it, as the link's words. Its plain
	// text is the following ItemAction's own line.
	Verb string
	// OneLine keeps the plain alternative to one dash-joined line, for lists
	// a reader scans line by line.
	OneLine bool
}

// Stat is one headline figure. Its plain text is the same "Label: Value" a
// Fact reads as.
type Stat struct {
	Label, Value string
	Tone         Tone
}

// Block states presentation intent separately from customer-authored words.
type Block struct {
	Heading     string
	Text        string
	ActionLabel string
	ActionURL   string
	Facts       []Fact
	Stats       []Stat
	Item        *Item
	// Lead opens the message under its title.
	Lead bool
	// Footnote moves the block below the message, where why-you-got-this lives.
	Footnote bool
	// Note sets the text apart in its Tone; Neutral draws a quiet well.
	Note bool
	// ItemLink makes the action the link of the item before it.
	ItemLink bool
	Tone     Tone
}

// Content builds matching text and markup from the same semantic blocks.
type Content struct{ Blocks []Block }

// Heading starts a section without treating record titles as formatting instructions.
func (c *Content) Heading(text string) { c.Blocks = append(c.Blocks, Block{Heading: text}) }

// Paragraph adds words that cannot become a heading or a link.
func (c *Content) Paragraph(text string) { c.add(Block{Text: text}) }

// Lead adds the sentence that opens the message.
func (c *Content) Lead(text string) { c.add(Block{Text: text, Lead: true}) }

// Note adds words set apart in a tone, such as a security warning.
func (c *Content) Note(text string, tone Tone) { c.add(Block{Text: text, Note: true, Tone: tone}) }

// Footnote adds words below the message.
func (c *Content) Footnote(text string) { c.add(Block{Text: text, Footnote: true}) }

// FootnoteAction adds a quiet link below the message.
func (c *Content) FootnoteAction(label, target string) {
	if label != "" && target != "" {
		c.Blocks = append(c.Blocks, Block{ActionLabel: label, ActionURL: target, Footnote: true})
	}
}

func (c *Content) add(block Block) {
	if strings.TrimSpace(block.Text) != "" {
		c.Blocks = append(c.Blocks, block)
	}
}

// Action names what a destination does in both alternatives.
func (c *Content) Action(label, target string) {
	if label != "" && target != "" {
		c.Blocks = append(c.Blocks, Block{ActionLabel: label, ActionURL: target})
	}
}

// ItemAction links the item just added. It never becomes the message's button.
func (c *Content) ItemAction(label, target string) {
	if label != "" && target != "" {
		c.Blocks = append(c.Blocks, Block{ActionLabel: label, ActionURL: target, ItemLink: true})
	}
}

// Facts adds named values as a FactList.
func (c *Content) Facts(rows ...Fact) {
	if len(rows) > 0 {
		c.Blocks = append(c.Blocks, Block{Facts: rows})
	}
}

// Stats adds headline figures as a StatStrip.
func (c *Content) Stats(figures ...Stat) {
	if len(figures) > 0 {
		c.Blocks = append(c.Blocks, Block{Stats: figures})
	}
}

// Item adds one record row. An item without a title is no row at all.
func (c *Content) Item(item Item) {
	if strings.TrimSpace(item.Title) != "" {
		c.Blocks = append(c.Blocks, Block{Item: &item})
	}
}

// Append adds blocks prepared by another view.
func (c *Content) Append(other Content) { c.Blocks = append(c.Blocks, other.Blocks...) }

func (c Content) String() string {
	paragraphs := make([]string, 0, len(c.Blocks))
	for _, block := range c.Blocks {
		paragraphs = append(paragraphs, block.text())
	}
	return strings.Join(paragraphs, "\n\n")
}

// text is the block in the plain alternative; every kind keeps its words.
func (b Block) text() string {
	switch {
	case b.Heading != "":
		return b.Heading
	case b.ActionURL != "":
		return b.ActionLabel + "\n" + b.ActionURL
	case len(b.Facts) > 0:
		rows := make([]string, 0, len(b.Facts))
		for _, row := range b.Facts {
			rows = append(rows, row.Label+": "+row.Value)
		}
		return strings.Join(rows, "\n")
	case len(b.Stats) > 0:
		rows := make([]string, 0, len(b.Stats))
		for _, figure := range b.Stats {
			rows = append(rows, figure.Label+": "+figure.Value)
		}
		return strings.Join(rows, "\n")
	case b.Item != nil:
		return b.Item.text()
	default:
		return b.Text
	}
}

func (i Item) text() string {
	lines := []string{i.Title}
	for _, line := range i.Lines {
		if strings.TrimSpace(line.Text) != "" {
			lines = append(lines, line.Text)
		}
	}
	if i.OneLine {
		return strings.Join(lines, " — ")
	}
	return strings.Join(lines, "\n")
}

// Message names the content and its title; Text keeps the title first.
func (c Content) Message(title, language, preview string) Message {
	return Message{Title: title, Language: language, Preheader: preview, Text: title + "\n\n" + c.String(), Blocks: c.Blocks}
}

// Preview is the first line a reader could act on, or the fallback.
func (c Content) Preview(fallback string) string {
	for _, block := range c.Blocks {
		source := block.Text
		if block.Item != nil {
			source = block.Item.Title
		}
		if block.Footnote {
			continue
		}
		line, _, _ := strings.Cut(strings.TrimSpace(source), "\n")
		if line != "" && !strings.Contains(line, "://") {
			return line
		}
	}
	return fallback
}

// AppendContent adds blocks and keeps the plain alternative in step.
func (m *Message) AppendContent(content Content) {
	m.Blocks = append(m.Blocks, content.Blocks...)
	m.Text = m.Title + "\n\n" + (Content{Blocks: m.Blocks}).String()
}
