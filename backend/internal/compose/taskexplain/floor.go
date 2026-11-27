// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package taskexplain

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/contactbrief"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

type phrase = langcopy.Phrase

// floorPhrases are whole sentences per language; only a name or a quote slots in.
var floorPhrases = struct {
	NamedSaid, BareSaid          phrase // name, quote / quote
	NamedSubject, BareSubject    phrase // name, subject / subject
	TheyLast, YouLast, OtherLast phrase // name, quote / quote / quote
	NothingSince                 phrase
}{
	NamedSaid:    phrase{En: "%s wrote: “%s”", De: "%s schrieb: „%s“", Vi: "%s đã viết: “%s”"},
	BareSaid:     phrase{En: "The message behind this task says: “%s”", De: "Die Nachricht hinter dieser Aufgabe sagt: „%s“", Vi: "Tin nhắn dẫn đến việc này viết: “%s”"},
	NamedSubject: phrase{En: "This task comes from %s's message “%s”.", De: "Diese Aufgabe stammt aus einer Nachricht von %s: „%s“.", Vi: "Việc này đến từ tin nhắn của %s: “%s”."},
	BareSubject:  phrase{En: "This task comes from the message “%s”.", De: "Diese Aufgabe stammt aus der Nachricht „%s“.", Vi: "Việc này đến từ tin nhắn “%s”."},
	TheyLast:     phrase{En: "%s's latest message: “%s”", De: "Die neueste Nachricht von %s: „%s“", Vi: "Tin nhắn mới nhất của %s: “%s”"},
	YouLast:      phrase{En: "Your latest message: “%s”", De: "Deine neueste Nachricht: „%s“", Vi: "Tin nhắn mới nhất của bạn: “%s”"},
	OtherLast:    phrase{En: "The latest message: “%s”", De: "Die neueste Nachricht: „%s“", Vi: "Tin nhắn mới nhất: “%s”"},
	NothingSince: phrase{En: "Nothing newer has come in since.", De: "Seitdem ist nichts Neueres eingegangen.", Vi: "Từ đó đến nay chưa có tin nhắn mới hơn."},
}

// quoteWords bounds a quoted line; a panel sentence is not the message.
const quoteWords = 24

// Floor states what happened from the records alone: the message the task
// came from, and the newest message since. It says nothing about stakes or the
// next step, because only a reading of those records could.
func Floor(in Input, lang string) []claims.Sentence {
	say := langcopy.For(lang)
	var out []claims.Sentence
	if line, ok := sourceLine(in.Source, say); ok {
		out = append(out, cite(line, in.Source.ID))
	}
	if newest, ok := newestSince(in); ok {
		out = append(out, cite(newestLine(newest, say), newest.ID))
	} else if in.Source != nil && len(out) > 0 && in.Relationship != nil {
		out = append(out, cite(say.Say(floorPhrases.NothingSince), in.Source.ID))
	}
	return out
}

func cite(text, activityID string) claims.Sentence {
	return claims.Sentence{
		Text: text, Nature: natureFact,
		Evidence: []claims.Evidence{{EntityType: citeActivity, EntityID: activityID}},
	}
}

func sourceLine(source *contactbrief.ActIn, say langcopy.Spoken) (string, bool) {
	if source == nil || source.Withheld {
		return "", false
	}
	name := speakerName(source)
	switch {
	case source.Preview != "" && name != "":
		return fmt.Sprintf(say.Say(floorPhrases.NamedSaid), name, quoted(source.Preview)), true
	case source.Preview != "":
		return fmt.Sprintf(say.Say(floorPhrases.BareSaid), quoted(source.Preview)), true
	case source.Subject != "" && name != "":
		return fmt.Sprintf(say.Say(floorPhrases.NamedSubject), name, source.Subject), true
	case source.Subject != "":
		return fmt.Sprintf(say.Say(floorPhrases.BareSubject), source.Subject), true
	}
	return "", false
}

// newestSince is the newest readable message on the relationship, when it is
// not the source itself. Recent is newest first.
func newestSince(in Input) (contactbrief.ActIn, bool) {
	if in.Relationship == nil {
		return contactbrief.ActIn{}, false
	}
	for _, act := range in.Relationship.Recent {
		if in.Source != nil && (act.ID == in.Source.ID || act.At < in.Source.At) {
			return contactbrief.ActIn{}, false
		}
		if act.Withheld || (act.Preview == "" && act.Subject == "") {
			continue
		}
		return act, true
	}
	return contactbrief.ActIn{}, false
}

func newestLine(act contactbrief.ActIn, say langcopy.Spoken) string {
	words := act.Preview
	if words == "" {
		words = act.Subject
	}
	switch {
	case act.Speaker != nil && act.Speaker.Kind == contactbrief.SpeakerYou:
		return fmt.Sprintf(say.Say(floorPhrases.YouLast), quoted(words))
	case speakerName(&act) != "":
		return fmt.Sprintf(say.Say(floorPhrases.TheyLast), speakerName(&act), quoted(words))
	}
	return fmt.Sprintf(say.Say(floorPhrases.OtherLast), quoted(words))
}

func speakerName(act *contactbrief.ActIn) string {
	if act.Speaker == nil || act.Speaker.Kind == contactbrief.SpeakerYou {
		return ""
	}
	return act.Speaker.Name
}

func quoted(text string) string {
	words := strings.Fields(text)
	if len(words) <= quoteWords {
		return strings.Join(words, " ")
	}
	return strings.Join(words[:quoteWords], " ") + "…"
}
