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
	if !strings.Contains(main, `href="https://example.test/#deal/1"`) || !strings.Contains(main, "Reply →") || !strings.Contains(main, ">Fleet renewal</a>") {
		t.Fatal("the item lost its own link or verb")
	}
	button := regexp.MustCompile(`<a href="https://example.test/#weekly" class="c-textOnAccentControl"[^>]*>Open the week<`)
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
	body.Stats(Stat{Label: "Won", Value: "€1"})
	body.Facts(Fact{Label: "New deals", Value: "5"})
	body.Note("Expires Friday.", Info)
	body.Append(weekWithItems())
	rendered := Render(body.Message("Week", "en", ""), Letterhead{})
	doubled := regexp.MustCompile(`<[a-z0-9]+[^>]*\sclass="[^"]*"[^>]*\sclass="`)
	if found := doubled.FindString(rendered); found != "" {
		t.Fatalf("a second class attribute is ignored, so its dark or narrow rule never applies: %s", found)
	}
	targets := regexp.MustCompile(`\.([a-z]+)\b`).FindAllStringSubmatch(layoutSheet(), -1)
	if len(targets) == 0 {
		t.Fatal("the layout sheet targets no class; this check can no longer see its subject")
	}
	for _, target := range targets {
		if !regexp.MustCompile(`class="[^"]*\b` + target[1] + `\b`).MatchString(rendered) {
			t.Errorf("no element carries the %q class the layout sheet targets", target[1])
		}
	}
}

// Gmail discards a whole style block over one rule it rejects, so the layout
// must not share a block with the dark sheet.
func TestThePhoneLayoutSurvivesAClientThatDropsTheDarkSheet(t *testing.T) {
	rendered := Render(weekWithItems().Message("Week", "en", ""), Letterhead{})
	blocks := regexp.MustCompile(`<style>(.*?)</style>`).FindAllStringSubmatch(rendered, -1)
	var layout, dark int
	for _, block := range blocks {
		if strings.Contains(block[1], layoutSheet()) {
			layout++
		}
		if strings.Contains(block[1], "prefers-color-scheme") {
			dark++
			if strings.Contains(block[1], "max-width") {
				t.Fatal("the phone layout shares a style block with the dark sheet")
			}
		}
	}
	if layout != 1 || dark != 1 {
		t.Fatalf("want one layout block and one dark block, got %d and %d", layout, dark)
	}
	if strings.Contains(layoutSheet(), "@media(") {
		t.Fatal("write each media query as `@media screen and (…)`, the form every mail client parses")
	}
}

func TestOnAPhoneAnItemsLinkDropsUnderItsText(t *testing.T) {
	main, _ := renderBlocks(weekWithItems().Blocks, "")
	cells := regexp.MustCompile(`<td valign="top" class="itemtext">.*?</td><td[^>]*class="itemlink"[^>]*><a href="https://example.test/#deal/1"`)
	if !cells.MatchString(main) {
		t.Fatal("the linked item lacks the text and link cells the phone layout stacks")
	}
	if strings.Count(main, `class="itemlink"`) != 1 {
		t.Fatal("an item without a link drew an empty link cell")
	}
}

func TestThePlainAlternativeKeepsEveryWord(t *testing.T) {
	want := "Fleet renewal\nNordwind\nDue today\nReply\n\n" +
		"Review\nhttps://example.test/#deal/1\n\n" +
		"Studio licences\nLost\n\n" +
		"Open the week\nhttps://example.test/#weekly\n\n" +
		"You get this because the weekly mail is on."
	if got := weekWithItems().String(); got != want {
		t.Fatalf("plain text =\n%s\nwant\n%s", got, want)
	}
}

func TestADroppedItemLeavesNoLinkForTheRecordBefore(t *testing.T) {
	var body Content
	body.Item(Item{Title: "Fleet renewal"})
	body.ItemAction("Review", "https://example.test/#deal/1")
	body.Item(Item{Title: "  "})
	body.ItemAction("Review", "https://example.test/#deal/2")
	if strings.Contains(body.String(), "#deal/2") {
		t.Fatal("the link of a dropped item was kept and would attach to another record")
	}
}

func TestATrailingActionWithoutALinkLeavesTheButtonToTheLastLinkedOne(t *testing.T) {
	var body Content
	body.Action("Open the week", "https://example.test/#weekly")
	body.Action("Confirm", "javascript:confirm()")
	main, _ := renderBlocks(body.Blocks, "")
	if !strings.Contains(main, `<a href="https://example.test/#weekly" class="c-textOnAccentControl"`) {
		t.Fatal("an action that renders as text took the primary button from a linked one")
	}
}

func TestDarkReadersGetTheDarkShadow(t *testing.T) {
	rendered := Render(weekWithItems().Message("Week", "en", ""), Letterhead{})
	if !strings.Contains(rendered, ".sh-shadow-rest{box-shadow:"+mailCSS(tokens[shadowRest].Dark)) || !strings.Contains(rendered, `class="bg-pane bd-paneEdge sh-shadow-rest"`) {
		t.Fatal("the card's shadow keeps its light value in dark mode")
	}
}
