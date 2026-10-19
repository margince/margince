// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command emailpreview writes fictional examples using the production renderers.
package main

import (
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/mailer"
)

func main() {
	directory := flag.String("out", "", "directory for HTML and plain-text previews")
	flag.Parse()
	if *directory == "" {
		fmt.Fprintln(os.Stderr, "pass -out with a preview directory")
		os.Exit(2)
	}
	if err := writePreviews(*directory); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type preview struct {
	name string
	text string
}

func writePreviews(directory string) error {
	var gallery []preview
	save := func(name, text, markup string) error {
		if err := writePair(directory, name, text, markup); err != nil {
			return err
		}
		gallery = append(gallery, preview{name: name, text: text})
		return nil
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	expiry := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	for _, language := range mailcopy.Languages() {
		words := mailcopy.For(string(language))
		for _, key := range consent.ControllerTemplateKeys() {
			rendered, _, err := consent.RenderControllerTemplate(key, expiry, string(language))
			if err != nil {
				return err
			}
			if err := save(key+"-"+string(language), rendered.Body, consent.RenderControllerPresentation(rendered, "Example Company")); err != nil {
				return err
			}
		}
		examples := map[string]emailview.Message{
			"reset":    {Title: words.ResetSubject, Text: words.ResetIntro + "\n\n" + words.ResetAction + "\n\nhttps://example.test/#/set-password/fictional\n\n" + words.ResetIgnore},
			"invite":   {Title: words.InviteSubject, Text: words.InviteIntro + "\n\n" + words.InviteAction + "\n\nhttps://example.test/#/set-password/fictional\n\n" + words.InviteIgnore},
			"buyer":    {Title: words.RoomSubject, Text: words.RoomIntro + "\n\n" + words.RoomAction + "\n\nhttps://example.test/#/room?c=fictional\n\n" + words.RoomDisclosure},
			"morning":  {Title: words.MorningSubject + "2026-11-09", Text: words.MailPersonal + "\n\nNorthstar · " + words.MailReply + "\n" + words.MailDue + ": 2026-11-09\n\nhttps://example.test/#/home\n\n" + words.MailCoverage},
			"approval": {Title: words.NotificationSubject + "Northstar", Text: words.NotificationIntro + "\n\nNorthstar\n\nhttps://example.test/#/home?approval=fictional"},
			"digest":   {Title: words.DigestSubject, Text: words.DigestIntro + "\n\n" + words.NoticeBacklogSubject + "\n\nhttps://example.test/#/worklist"},
		}
		completed := 4
		amountEUR, amountGBP := int64(4200000), int64(1500000)
		review := weekly.Review{LocalWeekStart: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), Counts: weekly.Counts{TasksCompleted: &completed, TasksDue: 3, TasksDone: 2, TasksCarriedOver: 1, DealsWon: 2}, Deals: []weekly.DealLine{
			{Label: "Northstar International Manufacturing & Customer Services", Outcome: weekly.OutcomeWon, AmountMinor: &amountEUR, Currency: "EUR", OccurredAt: expiry.AddDate(0, 0, -7)},
			{Label: "Krämer & Partners <London>", Outcome: weekly.OutcomeWon, AmountMinor: &amountGBP, Currency: "GBP", OccurredAt: expiry.AddDate(0, 0, -8)},
		}}
		examples["weekly"] = mailer.LocalizedMessage(weekly.MailSubject(review, words), weekly.MailBody(review, "https://example.test", words), string(language))
		examples["quiet"] = emailview.Message{Title: words.MorningSubject + "2026-11-09", Text: words.MorningQuiet}
		examples["partial"] = emailview.Message{Title: words.MorningSubject + "2026-11-09", Text: words.MailCoverage}
		for name, message := range examples {
			if message.Language == "" {
				message = mailer.LocalizedMessage(message.Title, message.Text, string(language))
			}
			if err := save(name+"-"+string(language), message.Text, emailview.Render(message)); err != nil {
				return err
			}
		}
	}
	return writeGallery(directory, gallery)
}

func writePair(directory, name, text, markup string) error {
	if err := os.WriteFile(filepath.Join(directory, name+".txt"), []byte(text), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, name+".html"), []byte(markup), 0o600)
}

func writeGallery(directory string, previews []preview) error {
	slices.SortFunc(previews, func(a, b preview) int { return strings.Compare(a.name, b.name) })
	var page strings.Builder
	page.WriteString(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Product email previews</title><style>body{font:16px Arial;margin:24px}article{margin:32px 0}.pair{display:flex;gap:24px;flex-wrap:wrap}iframe{width:390px;height:900px;border:1px solid #ccc}pre{white-space:pre-wrap;overflow-wrap:anywhere;max-width:600px}</style><h1>Product email previews</h1><p>Fictional fixtures. HTML uses the production renderer; plain text appears beside it. Browser dark mode and text zoom can be applied to each preview.</p>`)
	for _, preview := range previews {
		page.WriteString(`<article><h2>` + html.EscapeString(preview.name) + `</h2><div class="pair"><iframe title="` + html.EscapeString(preview.name) + `" src="` + html.EscapeString(preview.name) + `.html"></iframe><pre>` + html.EscapeString(preview.text) + `</pre></div></article>`)
	}
	page.WriteString(`</html>`)
	return os.WriteFile(filepath.Join(directory, "index.html"), []byte(page.String()), 0o600)
}
