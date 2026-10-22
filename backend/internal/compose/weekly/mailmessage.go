// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"fmt"
	"strconv"
	"time"

	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// MailMessage presents frozen facts without interpreting record labels as layout.
func MailMessage(review Review, homeURL string, words mailcopy.Copy) emailview.Message {
	var body emailview.Content
	body.Paragraph(mailHighlights(review, words))
	body.Heading(words.WeeklyResults)
	figures := figuresOf(review, words)
	if figures.beforeHistory() {
		body.Paragraph(words.WeeklyBeforeHistory)
	}
	body.Facts(mailFacts(review, words)...)
	if len(review.Deals) > 0 {
		body.Heading(words.WeeklyWhatMoved)
		for _, deal := range review.Deals[:min(len(review.Deals), mailDealCap)] {
			body.Paragraph(mailDealLine(deal, words))
		}
		if rest := len(review.Deals) - mailDealCap; rest > 0 {
			body.Paragraph(fmt.Sprintf(words.WeeklyAndMore, rest))
		}
	}
	body.Action(words.WeeklyFullWeek, mailcopy.Destination(homeURL, mailcopy.WeeklyFragment(review.LocalWeekStart.Format(time.DateOnly))))
	title := words.WeeklyHeading + words.DateRange(review.LocalWeekStart, review.LocalWeekStart.AddDate(0, 0, 6))
	return body.Message(title, string(words.Language), body.Preview(words.WeeklyPreview))
}

func mailFacts(review Review, words mailcopy.Copy) []emailview.Fact {
	figures, c := figuresOf(review, words), review.Counts
	var rows []emailview.Fact
	taskWork := c.TasksDue+c.TasksDone+c.TasksCarriedOver > 0 || (c.TasksCompleted != nil && *c.TasksCompleted > 0)
	if taskWork {
		if c.TasksCompleted != nil {
			rows = append(rows, emailview.Fact{Label: words.WeeklyCompleted, Value: figures.value(figures.set.Tasks, strconv.Itoa(*c.TasksCompleted))})
		}
		rows = append(rows, emailview.Fact{Label: words.WeeklyTasksDelivered, Value: figures.value(figures.set.Tasks, fmt.Sprintf(words.WeeklyOfDue, c.TasksDone, c.TasksDue))}, emailview.Fact{Label: words.WeeklyCarried, Value: figures.value(figures.set.Tasks, strconv.Itoa(c.TasksCarriedOver))})
	}
	if c.DealsWon+c.DealsLost+c.DealsMoved > 0 || len(review.Deals) > 0 {
		if c.DealsWon > 0 {
			rows = append(rows, emailview.Fact{Label: words.WeeklyDealsWon, Value: figures.value(figures.set.Deals, figures.won(strconv.Itoa(c.DealsWon)))})
		}
		if c.DealsLost > 0 {
			rows = append(rows, emailview.Fact{Label: words.WeeklyDealsLost, Value: figures.value(figures.set.Deals, strconv.Itoa(c.DealsLost))})
		}
		if c.DealsMoved > 0 {
			rows = append(rows, emailview.Fact{Label: words.WeeklyMoved, Value: figures.value(figures.set.Deals, strconv.Itoa(c.DealsMoved))})
		}
		if review.Money.Known && review.Money.WonMinor != 0 {
			rows = append(rows, emailview.Fact{Label: words.WeeklyWonRevenue, Value: figures.value(figures.set.Deals, figures.won(words.Money(review.Money.WonMinor, review.Money.Currency)))})
		}
	}
	if c.LeadsRouted+c.LeadsAnsweredInTarget+c.LeadsBreached > 0 {
		rows = append(rows, emailview.Fact{Label: words.WeeklyLeads, Value: figures.value(figures.set.Leads, strconv.Itoa(c.LeadsRouted))}, emailview.Fact{Label: words.WeeklyAnswered, Value: figures.value(figures.set.Leads, strconv.Itoa(c.LeadsAnsweredInTarget))}, emailview.Fact{Label: words.WeeklyBreached, Value: figures.value(figures.set.Leads, strconv.Itoa(c.LeadsBreached))})
	}
	if c.MeetingsHeld+c.MeetingsWithNextStep > 0 {
		rows = append(rows, emailview.Fact{Label: words.WeeklyMeetings, Value: figures.value(figures.set.Meetings, strconv.Itoa(c.MeetingsHeld))}, emailview.Fact{Label: words.WeeklyNextSteps, Value: figures.value(figures.set.Meetings, strconv.Itoa(c.MeetingsWithNextStep))})
	}
	if c.CommitmentsDue+c.CommitmentsKept > 0 {
		rows = append(rows, emailview.Fact{Label: words.WeeklyPromises, Value: figures.value(figures.set.Commitments, fmt.Sprintf(words.WeeklyOfDue, c.CommitmentsKept, c.CommitmentsDue))})
	}

	return rows
}
