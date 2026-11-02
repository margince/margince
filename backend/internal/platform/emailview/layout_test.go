// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package emailview

import (
	"regexp"
	"strings"
	"testing"
)

func weekWithItems() Content {
	var body Content
	body.Item(Item{Title: "Fleet renewal", Lines: []Line{{Text: "Nordwind"}, {Text: "Due today", Tone: Info}}, Verb: "Reply"})
	body.ItemAction("Review", "https://example.test/#deal/1")
	body.Item(Item{Title: "Studio licences", Lines: []Line{{Text: "Lost", Tone: Danger}}})
	body.Action("Open the week", "https://example.test/#weekly")
	body.Footnote("You get this because the weekly mail is on.")
	return body
}

func TestAnItemTakesOnlyItsOwnLinkAndTheMessageKeepsItsButton(t *testing.T) {
	main, footer := renderBlocks(weekWithItems().Blocks, "")
	if !strings.Contains(main, `href="https://example.test/#deal/1"`) || !strings.Contains(main, "Reply →") {
		t.Fatal("the item lost its own link or verb")
	}
	button := regexp.MustCompile(`class="c-textOnAccentControl"[^>]*>Open the week<`)
	if !button.MatchString(main) {
		t.Fatal("the closing action following an item must stay the primary button")
	}
	if strings.Contains(main, "Open the week →") {
		t.Fatal("an unlinked item took the message's action as its link")
	}
	if !strings.Contains(footer, "weekly mail is on") || strings.Contains(main, "weekly mail is on") {
		t.Fatal("a footnote must render in the footer only")
	}
}

func TestNoElementCarriesTwoClassAttributes(t *testing.T) {
	var body Content
	body.Lead("Your week.")
	body.Stats(Stat{Label: "Won", Value: "€1", Tone: Success})
	body.Facts(Fact{Label: "New deals", Value: "5"})
	body.Note("Expires Friday.", Info)
	body.Append(weekWithItems())
	rendered := Render(body.Message("Week", "en", ""))
	doubled := regexp.MustCompile(`<[a-z0-9]+[^>]*\sclass="[^"]*"[^>]*\sclass="`)
	if found := doubled.FindString(rendered); found != "" {
		t.Fatalf("a second class attribute is ignored, so its dark or narrow rule never applies: %s", found)
	}
	for _, layout := range []string{"pad", "facts", "stats", "items"} {
		if !regexp.MustCompile(`class="[^"]*\b` + layout + `\b`).MatchString(rendered) {
			t.Errorf("no element carries the %q class the stylesheet targets", layout)
		}
	}
}

func TestThePlainAlternativeKeepsEveryWordOnce(t *testing.T) {
	text := weekWithItems().String()
	for _, want := range []string{"Fleet renewal\nNordwind\nDue today", "Review\nhttps://example.test/#deal/1", "Studio licences\nLost", "Open the week\nhttps://example.test/#weekly"} {
		if !strings.Contains(text, want) {
			t.Errorf("plain text lost %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Reply") {
		t.Fatal("the verb repeats what the link line already says")
	}
}
