// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The SubRip reader, which is also how any colon-labelled plain text is read:
// one line-by-line attribution both the turn parser and the quote filter walk,
// so the two cannot disagree about whose a line is.

import "strings"

// parseSRT reads SubRip blocks: index + timing lines dropped,
// `Speaker:` prefixes attribute turns across wrapped lines.
func parseSRT(content string) []speakerTurn {
	var turns []speakerTurn
	for _, line := range scanSRT(content) {
		if line.Text != "" {
			turns = append(turns, speakerTurn{Speaker: line.Speaker, Text: line.Text})
		}
	}
	return turns
}

// srtLine is one line of a SubRip-shaped source and the label it belongs to.
// Raw is kept so prose can be rebuilt line for line without the turns of a
// label it quotes; Text is empty for a line that carries no spoken words.
type srtLine struct {
	Raw     string
	Speaker string
	Text    string
}

// scanSRT attributes every line of a source the way parseSRT reads it: a label
// holds until the next blank line, so a wrapped turn stays one speaker's.
func scanSRT(content string) []srtLine {
	raws := strings.Split(content, "\n")
	lines := make([]srtLine, 0, len(raws))
	current := ""
	for _, raw := range raws {
		line := srtLine{Raw: raw}
		trimmed := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		switch {
		case trimmed == "":
			current = ""
		case strings.Contains(trimmed, "-->") || isCueIdentifier(trimmed):
		default:
			if speaker, ok := timestampSpeakerLine(trimmed); ok {
				current = speaker
				line.Speaker = speaker
				break
			}
			speaker, text := SplitSpeakerLine(trimmed)
			if speaker != "" {
				current = speaker
			}
			line.Speaker, line.Text = current, text
		}
		lines = append(lines, line)
	}
	return lines
}
