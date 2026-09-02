// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// What the morning message says.
//
// SHORTER THAN THE WEEKLY, and the difference is the point. The weekly arrives
// once and summarises a week nobody can re-read; this arrives every working day
// about a queue the reader is one click from opening. So it says how much is
// waiting, hands over the overnight sentence when a pass wrote one, and links.
//
// It does NOT list the deals. A brief item carries a deal id and a rank, not a
// name — the screen resolves names through the deal's own endpoint, under the
// reader's own row scope — so a list here would either print UUIDs or need this
// lane to grow a second name resolver beside the one the API already has.
// Neither is worth it for a nudge toward a page that has the names on it.

import (
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// mailDateLayout is how a day is written, in every language.
//
// ISO, and that is deliberate: `2 January 2006` puts an English month name in
// the middle of a German sentence, and a numeric order like 06/01 is read as
// 6 January by half the world. A reader needs two things from this date — to
// tell one morning's message from the next, and to know which day.
const mailDateLayout = time.DateOnly

// MailSubject names the day, so two mornings do not read alike in a list.
func MailSubject(run BriefRun, words mailcopy.Copy) string {
	return words.MorningSubject + run.LocalDay.Format(mailDateLayout)
}

// MailBody renders one run as the message a rep reads before opening the app.
//
// homeURL is the installation's own Home. Empty omits the closing line rather
// than mailing a link built on an empty origin — an unusable URL in a message
// whose only call to action it is.
func MailBody(run BriefRun, homeURL string, words mailcopy.Copy) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s\n\n", words.MorningHeading, run.LocalDay.Format(mailDateLayout))

	waiting := waitingItems(run)
	if len(waiting) == 0 {
		// The WHOLE body. A heading over an empty list reads as a message that
		// failed to render, and a quiet morning is a real answer rather than a
		// missing one.
		b.WriteString(words.MorningQuiet + "\n")
		writeLink(&b, homeURL, words)
		return b.String()
	}

	// The sentence first, when a pass wrote one. It is the only part that reads
	// as a person talking, so it goes above the count rather than under it.
	//
	// FLATTENED like every rendered string, and this is the one that most needs
	// it: a model wrote it, and a model is exactly the source somebody can steer
	// into emitting a newline followed by a line that looks like ours.
	if run.Narrative != "" {
		b.WriteString(mailcopy.OneLine(run.Narrative) + "\n\n")
	}

	// Only the items an annotation pass gave words to. The rest are real and
	// are counted in the tail — see findingLines for why a line is never
	// invented for one.
	shown := findingLines(waiting)
	if len(shown) > 0 {
		b.WriteString(words.MorningTop + "\n")
		for _, line := range shown {
			b.WriteString("  · " + line + "\n")
		}
	}
	if rest := len(waiting) - len(shown); rest > 0 {
		fmt.Fprintf(&b, "  "+words.MorningAndMore+"\n", rest)
	}
	writeLink(&b, homeURL, words)
	return b.String()
}

// findingLines is the queue's own sentences, capped, in rank order.
//
// ONLY the items an annotation pass wrote a finding for. An item carries a deal
// id and a rank, never a name — the screen resolves names through the deal's
// own endpoint under the reader's row scope — so an un-annotated item has
// nothing this message could truthfully say about it. Printing its UUID would
// be words on a page rather than information, and inventing a line from the
// score would be this lane deciding what a deal is about.
//
// What those items get instead is the count in the tail, which is true and
// which the link answers.
func findingLines(waiting []BriefRunItem) []string {
	out := make([]string, 0, mailItemCap)
	for _, item := range waiting {
		if item.Finding == "" {
			continue
		}
		if len(out) == mailItemCap {
			break
		}
		out = append(out, mailcopy.OneLine(item.Finding))
	}
	return out
}

// mailItemCap bounds the lines a message carries.
//
// A queue is capped at ranking time, but not at a length anybody chose for a
// MESSAGE — and a mail that lists twenty items is a page, which is what the
// link is for. Five is what a reader takes in before deciding to open the app.
const mailItemCap = 5

// WaitingCount is how many items still want the rep.
//
// Exported so the lane that decides whether to SEND and the body that decides
// what a quiet morning SAYS are one answer. Two spellings of "is this morning
// quiet" would let the lane mail a message whose own text disagrees with why it
// was sent.
func WaitingCount(run BriefRun) int {
	return len(waitingItems(run))
}

// waitingItems is what still wants the rep, in rank order.
//
// An item the rep already acted on, dismissed or snoozed is not waiting on
// them: a message that counted those would tell somebody five things need them
// on a morning where they had already dealt with four.
func waitingItems(run BriefRun) []BriefRunItem {
	out := make([]BriefRunItem, 0, len(run.Items))
	for _, item := range run.Items {
		if item.State == briefStateNew {
			out = append(out, item)
		}
	}
	return out
}

// writeLink closes with the way in, when there is a usable one.
func writeLink(b *strings.Builder, homeURL string, words mailcopy.Copy) {
	if homeURL == "" {
		return
	}
	b.WriteString("\n" + words.MorningOpenDay + "\n  " + homeURL + "\n")
}
