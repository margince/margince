// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import "fmt"

// looksKeptLines say once, plainly, that a promise looks kept and is no open
// driver of this meeting.
func looksKeptLines(in Input) []Sentence {
	said := in.words()
	out := make([]Sentence, 0, len(in.LooksKept))
	for _, kept := range in.LooksKept {
		text := fmt.Sprintf(said.Say(touchWords.LooksKept), kept.Body, kept.Guest, said.occurredDay(kept.MeetingAt))
		switch {
		case kept.Auto && kept.Guest == "":
			text = fmt.Sprintf(said.Say(touchWords.AutoKeptPlain), kept.Body)
		case kept.Auto:
			text = fmt.Sprintf(said.Say(touchWords.AutoKept), kept.Body, kept.Guest, said.occurredDay(kept.MeetingAt))
		}
		out = append(out, Sentence{
			Text:     text,
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: kept.SourceID}},
		})
	}
	return out
}
