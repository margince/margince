// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package emailview

import (
	"html"
	"slices"
	"strings"
)

// renderBlocks draws the body and the footnotes. Consecutive items share one
// panel, and an ItemAction right after an item is its link. The last Action
// is the message's one primary button.
func renderBlocks(blocks []Block, placeholder string) (string, string) {
	primary := lastAction(blocks, placeholder)
	var main, footer strings.Builder
	var rows []string
	flush := func() {
		if len(rows) > 0 {
			main.WriteString(panel(strings.Join(rows, ""), "items", false))
			rows = nil
		}
	}
	for i := 0; i < len(blocks); i++ {
		block := blocks[i]
		if block.Footnote {
			footer.WriteString(footnote(block, placeholder))
			continue
		}
		if block.Item != nil {
			target, label := "", ""
			if i+1 < len(blocks) && blocks[i+1].ItemLink && actionURL(blocks[i+1].ActionURL, placeholder) {
				target, label = blocks[i+1].ActionURL, blocks[i+1].ActionLabel
				i++
			}
			rows = append(rows, itemRow(*block.Item, target, label, len(rows) == 0))
			continue
		}
		flush()
		main.WriteString(renderBlock(block, placeholder, i == primary))
	}
	flush()
	return main.String(), footer.String()
}

// lastAction is the index of the last action that stands on its own and
// renders as a link.
func lastAction(blocks []Block, placeholder string) int {
	for i, block := range slices.Backward(blocks) {
		if block.ActionURL != "" && !block.Footnote && !block.ItemLink && actionURL(block.ActionURL, placeholder) {
			return i
		}
	}
	return -1
}

func renderBlock(block Block, placeholder string, primary bool) string {
	switch {
	case block.Heading != "":
		return sectionHeading(block.Heading)
	case block.ActionURL != "":
		if !actionURL(block.ActionURL, placeholder) {
			return paragraph(block.ActionLabel + " " + block.ActionURL)
		}
		return button(block.ActionLabel, block.ActionURL, primary)
	case len(block.Facts) > 0:
		return renderFacts(block.Facts)
	case len(block.Stats) > 0:
		return renderStats(block.Stats)
	case block.Lead:
		return `<p` + look{ink: textSecondary}.attrs("margin:-8px 0 24px;font:"+bodyLarge.font()) + `>` + strings.ReplaceAll(html.EscapeString(block.Text), "\n", "<br>") + `</p>`
	case block.Note:
		return note(block.Text, block.Tone)
	default:
		return paragraph(block.Text)
	}
}

// itemRow is one record in a panel: its monogram, title, plain lines, toned
// lines as badges, and the verb as its link.
func itemRow(item Item, target, label string, first bool) string {
	var meta, badges []string
	for _, line := range item.Lines {
		switch {
		case strings.TrimSpace(line.Text) == "":
		case line.Tone == Neutral:
			meta = append(meta, html.EscapeString(line.Text))
		default:
			badges = append(badges, badge(line))
		}
	}
	verb := item.Verb
	if verb == "" {
		verb = label
	}
	link := ""
	if target != "" {
		link = `<a href="` + html.EscapeString(target) + `"` + look{ink: accentText}.attrs("font:"+body.medium()+";text-decoration:none;white-space:nowrap") + `>` + html.EscapeString(verb) + ` →</a>`
	} else if verb != "" {
		meta = append(meta, html.EscapeString(verb))
	}
	text := `<div` + look{ink: textPrimary}.attrs("font:"+body.medium()) + `>` + html.EscapeString(item.Title) + `</div>`
	if len(meta) > 0 {
		text += `<div` + look{ink: textSecondary}.attrs("margin-top:2px;font:"+bodySmall.font()) + `>` + strings.Join(meta, " · ") + `</div>`
	}
	if len(badges) > 0 {
		text += `<div style="margin-top:8px">` + strings.Join(badges, "") + `</div>`
	}
	return `<tr><td` + look{edge: borderSubtle}.attrs(hairline(first)+"padding:14px 16px") + `><table role="presentation" width="100%" cellspacing="0" cellpadding="0"><tr>` +
		`<td width="40" valign="top">` + monogram(item.Title) + `</td><td valign="top">` + text + `</td>` +
		`<td valign="top" align="right" style="padding-left:12px">` + link + `</td></tr></table></td></tr>`
}

func footnote(block Block, placeholder string) string {
	if block.ActionURL != "" {
		if !actionURL(block.ActionURL, placeholder) {
			return `<p style="margin:0 0 4px">` + html.EscapeString(block.ActionLabel+" "+block.ActionURL) + `</p>`
		}
		return `<p style="margin:0 0 4px"><a href="` + html.EscapeString(block.ActionURL) + `"` + look{ink: textSecondary}.attrs("text-decoration:underline") + `>` + html.EscapeString(block.ActionLabel) + `</a></p>`
	}
	return `<p style="margin:0 0 4px">` + strings.ReplaceAll(html.EscapeString(block.Text), "\n", "<br>") + `</p>`
}
