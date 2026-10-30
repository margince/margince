// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// waitingBacklog is who is waiting on three seats, written through the
// activity writer and the owed-verdict writer. cards is each seat's waiting
// cards the list must reach; never is what it must never serve.
type waitingBacklog struct {
	r       reminderEnv
	contact ids.UUID
	cards   map[ids.UUID]map[string]bool
	never   map[string]bool
	// held is the request past the horizon that Rep1 took; its task stays.
	held string
}

// waitsPastTheCap outgrows activities.WaitingScanCap, so Rep1's lane is cut.
const waitsPastTheCap = activities.WaitingScanCap + 5

func seedWaitingBacklog(t *testing.T) waitingBacklog {
	t.Helper()
	r := setupReminders(t)
	w := waitingBacklog{
		r: r, contact: r.e.SeedContact(t, "Dana Buyer", nil),
		cards: map[ids.UUID]map[string]bool{}, never: map[string]bool{},
	}
	// The plain requests are the newest, so the lane's first scan page is all
	// cards and its read is cut. The other cases sit below them.
	plain := func(seat ids.UUID, count int) []ids.UUID {
		var sent []ids.UUID
		for i := range count {
			at := r.now.Add(-time.Duration(i+1) * 20 * time.Minute)
			sent = append(sent, w.mail(t, seat, "dana@buyer.example", fmt.Sprintf("Question %d", i), "", at, true))
		}
		return sent
	}
	rep1 := plain(r.e.Rep1, waitsPastTheCap)
	plain(r.e.Rep2, 10)
	plain(r.e.Rep3, 5)
	// Done on three of Rep1's: the reader's own Done leaves them off.
	var done []activities.WorklistRowRef
	for _, id := range rep1[40:43] {
		done = append(done, activities.WorklistRowRef{Source: string(crmcontracts.WorklistItemSourceCustomerWaiting), RowID: id.String()})
		w.never[id.String()] = true
		delete(w.cards[r.e.Rep1], id.String())
	}
	if err := r.e.Activities.MarkWorklistRowsDone(w.seat(r.e.Rep1), done); err != nil {
		t.Fatalf("marking waits done: %v", err)
	}
	// One conversation of three requests is one card, its newest.
	for i := range 3 {
		w.mail(t, r.e.Rep1, "dana@buyer.example", fmt.Sprintf("Follow-up %d", i), "fold-thread",
			w.older().Add(-time.Duration(i+1)*time.Hour), true)
	}
	// A sending system's mail leaves unless it asks us something.
	w.mail(t, r.e.Rep1, "bounces@sendgrid.net", "Delivery report", "", w.older().Add(-5*time.Hour), false)
	w.mail(t, r.e.Rep1, "notify@sendgrid.net", "Please confirm your account", "", w.older().Add(-6*time.Hour), true)
	// Unjudged mail with one sender and subject is one card, its newest.
	w.mail(t, r.e.Rep1, "dana@buyer.example", "Price list", "", w.older().Add(-7*time.Hour), false)
	w.mail(t, r.e.Rep1, "dana@buyer.example", "Price list", "price-list-again", w.older().Add(-8*time.Hour), false)
	// Mail captured before the stamp existed carries none; the rule still drops it.
	legacy := w.mail(t, r.e.Rep1, "alerts@sendgrid.net", "Weekly digest", "", w.older().Add(-9*time.Hour), false)
	r.e.WsExec(t, `UPDATE activity SET sender_is_machine = false WHERE id = $1`, legacy)
	// Past the horizon a request leaves, unless a human took it.
	w.mail(t, r.e.Rep1, "dana@buyer.example", "An old question", "", r.now.AddDate(0, 0, -200), true)
	taken := w.mail(t, r.e.Rep1, "dana@buyer.example", "An old request Rep1 took", "", r.now.AddDate(0, 0, -201), true)
	w.takeRequest(t, taken)
	w.expectFold(t)
	return w
}

// older is where the cases below the plain requests start.
func (w *waitingBacklog) older() time.Time { return w.r.now.AddDate(0, 0, -5) }

// mail logs one inbound mail to seat, filed under the contact. A request is
// confirmed by the owed-verdict writer; thread "" gives the mail its own.
// cards learns which mails the fold keeps, by the rule the test states.
func (w *waitingBacklog) mail(t *testing.T, seat ids.UUID, from, subject, thread string, at time.Time, asks bool) ids.UUID {
	t.Helper()
	inbound := "inbound"
	if thread == "" {
		thread = "thread-" + subject + "-" + seat.String()
	}
	to := w.r.e.WsScalar(t, `SELECT lower(email) FROM app_user WHERE id = $1`, seat)
	row, _, err := w.r.e.Activities.LogActivity(w.r.e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &inbound, OccurredAt: &at, Source: "manual",
		ThreadKey: thread, CounterpartyEmail: from, EmailFrom: from, EmailTo: []string{to},
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: w.contact}},
	})
	if err != nil {
		t.Fatalf("logging %q: %v", subject, err)
	}
	id := ids.UUID(row.Id)
	if asks {
		confirmWaitingRequest(t, w.r.e, id, at)
	}
	if w.cards[seat] == nil {
		w.cards[seat] = map[string]bool{}
	}
	w.cards[seat][id.String()] = true
	return id
}

func (w *waitingBacklog) seat(user ids.UUID) context.Context {
	return w.r.e.As(user, []ids.UUID{w.r.e.Team1}, integration.AdminPerms)
}

// takeRequest has Rep1 take an old request as their own task, due yesterday.
func (w *waitingBacklog) takeRequest(t *testing.T, request ids.UUID) {
	t.Helper()
	due := w.r.now.AddDate(0, 0, -1)
	row, _, err := w.r.e.Activities.LogActivity(w.seat(w.r.e.Rep1), activities.LogActivityInput{
		Kind: "task", Source: "manual", RequestActivityID: &request, DueAt: &due,
	})
	if err != nil {
		t.Fatalf("taking the request: %v", err)
	}
	w.held = ids.UUID(row.Id).String()
}

// expectFold names what the queue drops of Rep1's mail. That is the two earlier
// requests of a conversation, the older mail on one subject, two sending
// systems' mails and both requests past the horizon. A fixture that stops
// reaching a rule fails here rather than passing the walk below.
func (w *waitingBacklog) expectFold(t *testing.T) {
	t.Helper()
	rep1 := w.cards[w.r.e.Rep1]
	dropped := 0
	for id := range rep1 {
		var mail struct {
			Subject string    `json:"s"`
			Thread  string    `json:"t"`
			At      time.Time `json:"a"`
		}
		encoded := w.r.e.WsScalar(t, `SELECT json_build_object('s', subject, 't', thread_key, 'a', occurred_at)::text
			FROM activity WHERE id = $1`, ids.MustParse(id))
		if err := json.Unmarshal([]byte(encoded), &mail); err != nil {
			t.Fatal(err)
		}
		earlier := mail.Thread == "fold-thread" && mail.At.Before(w.older().Add(-90*time.Minute)) ||
			mail.Thread == "price-list-again"
		switch mail.Subject {
		case "Delivery report", "Weekly digest", "An old question", "An old request Rep1 took":
			earlier = true
		}
		if earlier {
			delete(rep1, id)
			w.never[id] = true
			dropped++
		}
	}
	if dropped != 7 {
		t.Fatalf("the fixture names %d mails the queue drops, want 7", dropped)
	}
}

// servedRow is one row the walk served, with its place in the tail's order.
type servedRow struct {
	id     string
	source crmcontracts.WorklistItemSource
	at     *time.Time
}

// walkWorklist follows next_cursor to the end. It answers every row served,
// the first page, and the rows the first page froze as the ranked top.
func walkWorklist(
	ctx context.Context, t *testing.T, r reminderEnv, scope string, owner ids.UUID,
) ([]servedRow, crmcontracts.Worklist, map[string]bool) {
	t.Helper()
	svc := newAttentionService(r.e.Pool, approvals.NewService(r.e.DB()), func() time.Time { return r.now })
	var served []servedRow
	var first crmcontracts.Worklist
	frozen := map[string]bool{}
	cursor := ""
	for page := 0; ; page++ {
		if page > 60 {
			t.Fatalf("%s: the walk did not end in 60 pages", scope)
		}
		day, err := svc.Worklist(ctx, scope, "", owner, 25, cursor)
		if err != nil {
			t.Fatalf("%s page %d: %v", scope, page, err)
		}
		for _, row := range day.Queue {
			at := row.DueAt
			if row.Source == crmcontracts.WorklistItemSourceCustomerWaiting {
				at = row.OccurredAt
			}
			served = append(served, servedRow{row.Id, row.Source, at})
		}
		if page == 0 {
			first = day
			if day.NextCursor != nil {
				frozen = frozenTop(t, r.e, *day.NextCursor)
			}
		}
		if day.NextCursor == nil {
			return served, first, frozen
		}
		cursor = *day.NextCursor
	}
}

// frozenTop reads the ranked rows a first page froze, from the walk its
// cursor names.
func frozenTop(t *testing.T, e *integration.Env, token string) map[string]bool {
	t.Helper()
	cursor, err := storekit.DecodeOpaque[struct {
		Snapshot ids.UUID `json:"s"`
	}](token)
	if err != nil {
		t.Fatal(err)
	}
	var rows []worklistsnap.Row
	encoded := e.WsScalar(t, `SELECT rows::text FROM worklist_snapshot WHERE id = $1`, cursor.Snapshot)
	if err := json.Unmarshal([]byte(encoded), &rows); err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		out[row.RowID] = true
	}
	return out
}

// placedBefore orders two served rows as the tail merges them.
func placedBefore(a, b servedRow) bool {
	switch {
	case a.at == nil || b.at == nil:
		return b.at == nil && a.at != nil
	case !a.at.Equal(*b.at):
		return a.at.Before(*b.at)
	}
	return a.source < b.source
}

// Each scope's customer_waiting total is the cards the list holds when walked
// to its end, and the walk reaches each of them once. Below the ranked top the
// rows of both sources arrive in the tail's shared order.
func TestTheWaitingTotalMatchesTheMergedWalk(t *testing.T) {
	w := seedWaitingBacklog(t)
	r := w.r
	for i := range plannedCapForTest + 6 {
		r.task(t, fmt.Sprintf("Rep1 due %d", i), r.now.Add(-time.Duration(i+1)*3*time.Hour), nil, &r.e.Rep1)
	}
	team := map[string]bool{}
	for _, seat := range []ids.UUID{r.e.Rep1, r.e.Rep2} {
		for id := range w.cards[seat] {
			team[id] = true
		}
	}
	checks := []struct {
		name, scope string
		ctx         context.Context
		owner       ids.UUID
		cards       map[string]bool
	}{
		{"mine", "mine", w.seat(r.e.Rep1), ids.Nil, w.cards[r.e.Rep1]},
		{"named owner", "mine", r.e.Admin(), r.e.Rep1, w.cards[r.e.Rep1]},
		{"team", "team", w.seat(r.e.Rep1), ids.Nil, team},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			served, first, frozen := walkWorklist(check.ctx, t, r, check.scope, check.owner)
			seen := map[string]bool{}
			waited := 0
			for _, row := range served {
				if seen[row.id] {
					t.Fatalf("%s %s was served twice", row.source, row.id)
				}
				seen[row.id] = true
				if w.never[row.id] {
					t.Errorf("%s was served, and the queue drops it", row.id)
				}
				if row.source == crmcontracts.WorklistItemSourceCustomerWaiting {
					waited++
				}
			}
			for id := range check.cards {
				if !seen[id] {
					t.Errorf("waiting card %s was never reached", id)
				}
			}
			// The lane's scan was cut and no count fits the first page. The
			// category is a floor, and scrolling reaches every card.
			if total, stated := totalOf(first, "customer_waiting"); stated || waited != len(check.cards) {
				t.Errorf("customer_waiting: total %d (stated %v), walked %d, want a floor and %d", total, stated, waited, len(check.cards))
			}
			if check.scope == "mine" && !seen[w.held] {
				t.Error("the task Rep1 took for an old request was never reached")
			}
			assertTailOrder(t, served, frozen)
		})
	}
}

// assertTailOrder holds that every row past the ranked top comes after all of
// it, in the tail's order, and that both sources reach the tail.
func assertTailOrder(t *testing.T, served []servedRow, frozen map[string]bool) {
	t.Helper()
	start := 0
	for i, row := range served {
		if frozen[row.id] {
			start = i + 1
		}
	}
	tail := served[start:]
	sources := map[crmcontracts.WorklistItemSource]bool{}
	for i, row := range tail {
		if frozen[row.id] {
			t.Fatalf("ranked row %s came after the tail began", row.id)
		}
		sources[row.source] = true
		if i > 0 && placedBefore(row, tail[i-1]) {
			t.Fatalf("tail row %d (%s %v) came after %s %v, out of the shared order", i, row.source, row.at, tail[i-1].source, tail[i-1].at)
		}
	}
	if !sources[crmcontracts.WorklistItemSourceTask] || !sources[crmcontracts.WorklistItemSourceCustomerWaiting] {
		t.Errorf("the tail merged %v, want tasks and waiting cards both", sources)
	}
}

// The tail's cards are the lane's own fold over every row the scan reads: one
// answer to "who is waiting", whichever side draws it.
func TestTheWaitingTailIsTheFoldOfTheScan(t *testing.T) {
	w := seedWaitingBacklog(t)
	r := w.r
	ctx := w.seat(r.e.Rep1)
	seam := attentionWaiting{store: activities.NewStore(r.e.DB()), now: func() time.Time { return r.now }}
	owner := r.e.Rep1
	narrow := attention.WaitingFor{Owner: &owner}
	var scanned []activities.WaitingReply
	var before activities.WaitingPosition
	for {
		rows, err := seam.store.WaitingRepliesNarrowedBefore(ctx, r.now, before, waitingNarrow(narrow))
		if err != nil {
			t.Fatal(err)
		}
		scanned = append(scanned, rows...)
		if len(rows) < activities.WaitingScanCap {
			break
		}
		last := rows[len(rows)-1]
		before = activities.WaitingPosition{At: last.OccurredAt, ID: last.ActivityID}
	}
	folded := map[ids.UUID]bool{}
	for _, row := range keepWaitingCustomers(scanned) {
		folded[row.ActivityID] = true
	}
	tailed := map[ids.UUID]bool{}
	for after := ""; ; {
		read, err := seam.WaitingAfter(ctx, r.now, narrow, after, 7)
		if err != nil {
			t.Fatal(err)
		}
		for _, card := range read.Cards {
			tailed[card.Row.ActivityID] = true
		}
		if !read.More {
			break
		}
		after = read.Through
	}
	if len(tailed) != len(folded) || len(tailed) != len(w.cards[r.e.Rep1]) {
		t.Fatalf("the tail draws %d cards, the lane's fold of the whole scan %d, the fixture names %d",
			len(tailed), len(folded), len(w.cards[r.e.Rep1]))
	}
	for id := range folded {
		if !tailed[id] {
			t.Errorf("the lane's card %s is not one the tail draws", id)
		}
	}
}
