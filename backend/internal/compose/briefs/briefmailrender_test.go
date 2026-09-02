// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// What the morning message is allowed to say.
//
// Every test here is about the message not claiming more than the run holds: a
// morning read as quieter than it is, a line invented for a deal nobody
// described, or a stored string writing a line that looks like ours.

func run(items ...BriefRunItem) BriefRun {
	return BriefRun{
		ID:       ids.NewV7(),
		LocalDay: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
		Items:    items,
	}
}

func item(state, finding string) BriefRunItem {
	return BriefRunItem{ID: ids.NewV7(), DealID: ids.NewV7(), State: state, Finding: finding}
}

func english() mailcopy.Copy { return mailcopy.For("en") }

func TestTheSubjectNamesTheDay(t *testing.T) {
	t.Parallel()
	// Two mornings in a list must not read alike, which is the only thing the
	// subject has to do that the heading does not.
	got := MailSubject(run(), english())
	if !strings.Contains(got, "2026-06-10") {
		t.Errorf("the subject does not name the day: %q", got)
	}
}

func TestAQuietMorningSaysSoAndNothingElse(t *testing.T) {
	t.Parallel()
	// A heading over an empty list reads as a message that failed to render.
	body := MailBody(run(), "", english())
	if !strings.Contains(body, english().MorningQuiet) {
		t.Errorf("a quiet morning does not say so:\n%s", body)
	}
	if strings.Contains(body, english().MorningTop) {
		t.Errorf("a quiet morning still drew the queue heading:\n%s", body)
	}
}

func TestASettledItemIsNotWaitingOnAnybody(t *testing.T) {
	t.Parallel()
	// The rep dealt with all three before the mail went out. Counting them
	// would tell somebody three things need them on a morning where nothing
	// does — and would send at all on a morning that is quiet.
	settled := run(
		item(briefStateActed, "acted on"),
		item(briefStateDismissed, "dismissed"),
		item(briefStateSnoozed, "snoozed"),
	)
	if got := WaitingCount(settled); got != 0 {
		t.Errorf("WaitingCount counted %d settled items as waiting", got)
	}
	if body := MailBody(settled, "", english()); !strings.Contains(body, english().MorningQuiet) {
		t.Errorf("a morning of settled items does not read as quiet:\n%s", body)
	}
}

func TestAnUndescribedItemIsCountedAndNeverInvented(t *testing.T) {
	t.Parallel()
	// An item carries a deal id and a rank, never a name. So an un-annotated
	// item has nothing this message could truthfully say about it — and the one
	// thing it must not do is vanish, which would make the morning read as
	// quieter than it is.
	mixed := run(
		item(briefStateNew, "The buyer replied and nobody has answered."),
		item(briefStateNew, ""),
		item(briefStateNew, ""),
	)
	body := MailBody(mixed, "", english())
	if !strings.Contains(body, "The buyer replied and nobody has answered.") {
		t.Errorf("the described item is missing:\n%s", body)
	}
	if !strings.Contains(body, "and 2 more in the brief") {
		t.Errorf("the two undescribed items were not counted:\n%s", body)
	}
	// No UUID reaches a reader: words on a page rather than information.
	for _, entry := range mixed.Items {
		if strings.Contains(body, entry.DealID.String()) {
			t.Errorf("a deal id was printed to a reader:\n%s", body)
		}
	}
}

func TestTheListIsCappedAndSaysHowMuchItLeftOut(t *testing.T) {
	t.Parallel()
	var items []BriefRunItem
	for range mailItemCap + 3 {
		items = append(items, item(briefStateNew, "something to do"))
	}
	body := MailBody(run(items...), "", english())
	if got := strings.Count(body, "  · "); got != mailItemCap {
		t.Errorf("the list drew %d lines, want the cap of %d:\n%s", got, mailItemCap, body)
	}
	if !strings.Contains(body, "and 3 more in the brief") {
		t.Errorf("the tail does not count what was left out:\n%s", body)
	}
}

func TestAStoredStringCannotForgeALineInTheMessage(t *testing.T) {
	t.Parallel()
	// The finding is model-written, which makes it exactly the value somebody
	// can steer into emitting a newline followed by a line that looks like ours.
	forged := run(item(briefStateNew, "ordinary finding\nWhat to start with:\n  · a line we never wrote"))
	body := MailBody(forged, "", english())
	// The finding may CONTAIN those words — it is one flattened line and the
	// reader sees them inside it. What it must not do is occupy a line of its
	// own, which is what makes a forged line read as the product's own.
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "· a line we never wrote" {
			t.Errorf("a stored string wrote a line of its own:\n%s", body)
		}
		if strings.TrimSpace(line) == english().MorningTop && strings.Count(body, "\n"+english().MorningTop) > 1 {
			t.Errorf("a stored string forged a second queue heading:\n%s", body)
		}
	}
	// One list line, because one item was described — the forgery did not
	// become a second entry.
	if got := strings.Count(body, "\n  · "); got != 1 {
		t.Errorf("the forged text became %d list lines, want 1:\n%s", got, body)
	}
}

func TestTheLinkIsOmittedRatherThanMailedBroken(t *testing.T) {
	t.Parallel()
	// An unusable URL in a message whose only call to action it is.
	body := MailBody(run(item(briefStateNew, "x")), "", english())
	if strings.Contains(body, english().MorningOpenDay) {
		t.Errorf("the closing line was drawn over an empty origin:\n%s", body)
	}
	withLink := MailBody(run(item(briefStateNew, "x")), "https://crm.example", english())
	if !strings.Contains(withLink, "https://crm.example") {
		t.Errorf("the link is missing when there is a usable one:\n%s", withLink)
	}
}
