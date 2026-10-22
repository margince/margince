// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func teamCurrentMail(team weekly.TeamReview, base string, words mailcopy.Copy) emailview.Content {
	var body emailview.Content
	shown := 0
	attention := weekly.TeamMailAttention(team)
	for _, finding := range attention.Exceptions {
		if finding.Subject == nil || finding.Subject.Label == nil {
			continue
		}
		if shown == 0 {
			body.Heading(words.MailCurrent)
		}
		body.Paragraph(mailcopy.OneLine(*finding.Subject.Label))
		body.Action(words.MailReview, mailcopy.Destination(base, mailcopy.RecordFragment(string(finding.Subject.Type), finding.Subject.Id.String())))
		shown++
		if shown == 3 {
			break
		}
	}
	for _, deal := range attention.StaleDeals {
		if shown == 3 {
			break
		}
		if shown == 0 {
			body.Heading(words.MailCurrent)
		}
		body.Paragraph(mailcopy.OneLine(deal.DealName) + "\n" + fmt.Sprintf(words.MailStale, deal.DaysQuiet))
		body.Action(words.MailReview, mailcopy.Destination(base, mailcopy.RecordFragment("deal", deal.DealID.String())))
		shown++
	}
	if remaining := len(attention.Exceptions) + len(attention.StaleDeals) - shown; remaining > 0 {
		body.Paragraph(fmt.Sprintf(words.DigestAndMore, remaining))
	}
	return body
}
