// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package emailview

import (
	"html"
	"slices"
	"strings"
)

// look paints an element from tokens: inline light values, plus role classes
// the dark sheet repaints. A border's width and style come from css; look
// only adds its colour, after css, so it wins over a shorthand there. Layout
// classes share the one class attribute, which a second one would void.
type look struct{ ink, fill, edge, shadow token }

func (l look) attrs(css string, layout ...string) string {
	classes := slices.Clone(layout)
	style := css
	add := func(t token, class, property string) {
		if t == noToken {
			return
		}
		classes = append(classes, class+tokens[t].Name)
		if style != "" && !strings.HasSuffix(style, ";") {
			style += ";"
		}
		style += property + ":" + t.light()
	}
	add(l.ink, "c-", "color")
	add(l.fill, "bg-", "background")
	add(l.edge, "bd-", "border-color")
	add(l.shadow, "sh-", "box-shadow")
	out := ""
	if len(classes) > 0 {
		out = ` class="` + strings.Join(classes, " ") + `"`
	}
	if style != "" {
		out += ` style="` + style + `"`
	}
	return out
}

func paragraph(text string) string {
	return `<p` + look{ink: textPrimary}.attrs("margin:0 0 16px;font:"+body.font()) + `>` + strings.ReplaceAll(html.EscapeString(text), "\n", "<br>") + `</p>`
}

func sectionHeading(text string) string {
	return `<h2` + look{ink: textPrimary}.attrs("margin:24px 0 8px;font:"+headingSmall.font()) + `>` + html.EscapeString(text) + `</h2>`
}

// button is the design system's Button: primary for the message's one main
// action, ghost for every other.
func button(label, target string, primary bool) string {
	paint := look{ink: textPrimary, fill: pane, edge: borderSubtle, shadow: shadowRest}
	if primary {
		paint = look{ink: textOnAccentControl, fill: accent, edge: accent, shadow: shadowRest}
	}
	return `<table role="presentation" cellspacing="0" cellpadding="0" style="margin:8px 0 16px"><tr><td` +
		paint.attrs("border:1px solid;border-radius:"+rControl.light()) +
		`><a href="` + html.EscapeString(target) + `"` + look{ink: paint.ink}.attrs("display:inline-block;padding:9px 16px;font:"+body.medium()+";text-decoration:none;overflow-wrap:anywhere") +
		`>` + html.EscapeString(label) + `</a></td></tr></table>`
}

// panel is the design system's Panel. A list of records or figures is
// layout; a fact panel stays a data table, so its header cells keep scope.
func panel(inner, class string, data bool) string {
	role := ` role="presentation"`
	if data {
		role = ""
	}
	return `<table` + role + ` width="100%" cellspacing="0" cellpadding="0"` +
		look{fill: pane, edge: paneEdge}.attrs("border:1px solid;border-radius:"+rMd.light()+";border-collapse:separate;margin:0 0 20px", class) + `>` + inner + `</table>`
}

// hairline is the divider between two rows of one panel.
func hairline(first bool) string {
	if first {
		return ""
	}
	return "border-top:1px solid;"
}

func renderFacts(facts []Fact) string {
	var rows strings.Builder
	for i, fact := range facts {
		edge := look{ink: textSecondary, edge: borderSubtle}
		rows.WriteString(`<tr><th scope="row" align="left" valign="top"` + edge.attrs(hairline(i == 0)+"padding:10px 16px;font:"+body.font()+";width:40%") + `>` + html.EscapeString(fact.Label) +
			`</th><td valign="top"` + look{ink: textPrimary, edge: borderSubtle}.attrs(hairline(i == 0)+"padding:10px 16px 10px 0;font:"+body.medium()) + `>` + html.EscapeString(fact.Value) + `</td></tr>`)
	}
	return panel(rows.String(), "facts", true)
}

// renderStats is the StatStrip at its plate density. Each cell holds its
// figure over its label, so a narrow screen that stacks the cells keeps
// every label with its figure.
func renderStats(stats []Stat) string {
	var cells strings.Builder
	for i, stat := range stats {
		side := ""
		if i > 0 {
			side = "border-left:1px solid;"
		}
		ink := textPrimary
		if stat.Tone != Neutral {
			ink = toneInk(stat.Tone)
		}
		cells.WriteString(`<td valign="top"` + look{edge: borderSubtle}.attrs(side+"padding:14px 16px") + `>` +
			`<div` + look{ink: ink}.attrs("font:"+headingXLarge.font()) + `>` + html.EscapeString(stat.Value) + `</div>` +
			`<div` + look{ink: textSecondary}.attrs("margin-top:2px;font:"+bodySmall.font()) + `>` + html.EscapeString(stat.Label) + `</div></td>`)
	}
	return panel(`<tr>`+cells.String()+`</tr>`, "stats", false)
}

func toneInk(tone Tone) token {
	switch tone {
	case Warning:
		return warningText
	case Info:
		return infoText
	case Success:
		return successText
	case Danger:
		return dangerText
	default:
		return textPrimary
	}
}

func toneLook(tone Tone) look {
	switch tone {
	case Warning:
		return look{ink: warningText, fill: warningSurface, edge: warningBorder}
	case Info:
		return look{ink: infoText, fill: infoSurface, edge: infoBorder}
	case Success:
		return look{ink: successText, fill: successSurface, edge: successBorder}
	case Danger:
		return look{ink: dangerText, fill: dangerSurface, edge: dangerBorder}
	default:
		return look{ink: textPrimary, fill: bgWell, edge: paneEdge}
	}
}

// badge is the design system's Badge in one of its tones.
func badge(line Line) string {
	return `<span` + toneLook(line.Tone).attrs("display:inline-block;margin:0 4px 4px 0;padding:1px 8px;border:1px solid;border-radius:9999px;font:"+bodySmall.font()+";white-space:nowrap") + `>` + html.EscapeString(line.Text) + `</span>`
}

func note(text string, tone Tone) string {
	return `<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="margin:0 0 16px"><tr><td` +
		toneLook(tone).attrs("padding:10px 12px;border:1px solid;border-radius:"+rControl.light()+";font:"+body.font()) + `>` +
		strings.ReplaceAll(html.EscapeString(text), "\n", "<br>") + `</td></tr></table>`
}
