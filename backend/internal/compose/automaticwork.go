// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How much work Margince hands each colleague by itself, and what became of it.
// A rule that routes work to the wrong colleague shows up as a share of items
// that colleague moved on. Counts only: never a subject or a body.
//
// A decider who rejects a proposal was the right colleague to ask, so a
// rejection is never counted as "Not mine". Archiving a task undone may mean
// the work was not needed, so it counts as dismissed.

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	automaticWorkDefaultDays = 7
	automaticWorkMaxDays     = 30
	// automaticWorkShareThreshold is a reading for an admin, never a trigger:
	// nothing reassigns or files work when a share passes it.
	automaticWorkShareThreshold = 0.1
	// automaticWorkSmallSample is where one item stops moving the share by more
	// than the threshold itself.
	automaticWorkSmallSample = 10
)

// automaticWorkKinds lists the kinds in the order the report shows them, with
// whether a colleague can say an item of it is not theirs. An approval's decider
// can only accept or reject.
var automaticWorkKinds = []crmcontracts.AutomaticWorkKind{
	{Kind: crmcontracts.AutomaticWorkKindNamePromiseTask, RecordsNotMine: true},
	{Kind: crmcontracts.AutomaticWorkKindNameReplyReminder, RecordsNotMine: true},
	{Kind: crmcontracts.AutomaticWorkKindNameApprovalRequest, RecordsNotMine: false},
}

type automaticWorkHandlers struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// GetAutomaticWorkHealth reports the automatic work per colleague for the window.
func (h automaticWorkHandlers) GetAutomaticWorkHealth(
	w http.ResponseWriter, r *http.Request, params crmcontracts.GetAutomaticWorkHealthParams,
) {
	if !admitHealthReader(w, r) {
		return
	}
	days := automaticWorkDefaultDays
	if params.Days != nil {
		// The generated parameter does not apply the contract's range.
		if *params.Days < 1 || *params.Days > automaticWorkMaxDays {
			httperr.Write(w, r, httperr.Validation("days", "out_of_range",
				fmt.Sprintf("the window must be between 1 and %d days", automaticWorkMaxDays)))
			return
		}
		days = *params.Days
	}
	now := h.now()
	serveHealthReport(w, r, h.pool, "automatic work health",
		func(ctx context.Context, tx pgx.Tx) (crmcontracts.AutomaticWorkHealth, error) {
			return readAutomaticWorkHealth(ctx, tx, now, days)
		})
}

// readAutomaticWorkHealth builds the report for the `days` calendar days that
// end today in the installation's zone.
func readAutomaticWorkHealth(ctx context.Context, tx pgx.Tx, now time.Time, days int) (crmcontracts.AutomaticWorkHealth, error) {
	zone, from, lastDay, err := automaticWorkWindow(ctx, tx, now, days)
	if err != nil {
		return crmcontracts.AutomaticWorkHealth{}, err
	}
	rows, err := readAutomaticWorkRows(ctx, tx, zone, from, now)
	if err != nil {
		return crmcontracts.AutomaticWorkHealth{}, err
	}
	marks, err := readWaitingNotMine(ctx, tx, from, now)
	if err != nil {
		return crmcontracts.AutomaticWorkHealth{}, err
	}
	seats, err := readAutomaticWorkSeats(ctx, tx, namedSeats(rows, marks))
	if err != nil {
		return crmcontracts.AutomaticWorkHealth{}, err
	}
	out := crmcontracts.AutomaticWorkHealth{
		GeneratedAt:      now,
		Days:             days,
		FirstDay:         calendarDay(from),
		LastDay:          calendarDay(lastDay),
		ShareThreshold:   automaticWorkShareThreshold,
		SmallSampleBelow: automaticWorkSmallSample,
		Kinds:            automaticWorkKinds,
		Colleagues:       []crmcontracts.AutomaticWorkColleague{},
	}
	byColleague, unattributed, err := groupAutomaticWork(rows, seats)
	if err != nil {
		return crmcontracts.AutomaticWorkHealth{}, err
	}
	out.Unattributed = unattributed
	for _, seat := range seats {
		out.Colleagues = append(out.Colleagues, crmcontracts.AutomaticWorkColleague{
			UserId:         openapi_types.UUID(seat.id),
			DisplayName:    seat.name,
			Status:         seat.status,
			Tallies:        tallies(byColleague[seat.id]),
			WaitingNotMine: marks[seat.id],
		})
	}
	return out, nil
}

func automaticWorkWindow(ctx context.Context, tx pgx.Tx, now time.Time, days int) (string, time.Time, time.Time, error) {
	zone, err := identity.TimezoneAppliedTx(ctx, tx)
	if err != nil {
		return "", time.Time{}, time.Time{}, err
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return "", time.Time{}, time.Time{}, fmt.Errorf("automatic work: loading zone: %w", err)
	}
	local := now.In(loc)
	last := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return zone, last.AddDate(0, 0, 1-days), last, nil
}

func calendarDay(t time.Time) openapi_types.Date {
	return openapi_types.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)}
}

func namedSeats(rows []automaticWorkRow, marks map[ids.UUID]int) []ids.UUID {
	named := []ids.UUID{}
	for _, row := range rows {
		if row.colleague != nil && !slices.Contains(named, *row.colleague) {
			named = append(named, *row.colleague)
		}
	}
	for user := range marks {
		if !slices.Contains(named, user) {
			named = append(named, user)
		}
	}
	return named
}

// colleagueDays is one colleague's counts per kind and per day.
type colleagueDays map[crmcontracts.AutomaticWorkKindName]map[time.Time]crmcontracts.AutomaticWorkCounts

// groupAutomaticWork files each row under its seat. A row naming no seat that
// still exists is counted as unattributed, so the total stays whole.
func groupAutomaticWork(rows []automaticWorkRow, seats []automaticWorkSeat) (map[ids.UUID]colleagueDays, int, error) {
	known := map[ids.UUID]bool{}
	for _, seat := range seats {
		known[seat.id] = true
	}
	byColleague := map[ids.UUID]colleagueDays{}
	unattributed := 0
	for _, row := range rows {
		if row.colleague == nil || !known[*row.colleague] {
			unattributed += row.count
			continue
		}
		colleague := byColleague[*row.colleague]
		if colleague == nil {
			colleague = colleagueDays{}
			byColleague[*row.colleague] = colleague
		}
		if colleague[row.kind] == nil {
			colleague[row.kind] = map[time.Time]crmcontracts.AutomaticWorkCounts{}
		}
		counts := colleague[row.kind][row.day]
		if err := addOutcome(&counts, row.outcome, row.count); err != nil {
			return nil, 0, err
		}
		colleague[row.kind][row.day] = counts
	}
	return byColleague, unattributed, nil
}

// The outcomes automaticItemsSQL assigns, spelled as the SQL spells them.
const (
	workNotMine   = "not_mine"
	workRerouted  = "rerouted"
	workDismissed = "dismissed"
	workRejected  = "rejected"
	workCompleted = "completed"
	workExpired   = "expired"
	workOpen      = "open"
)

func addOutcome(c *crmcontracts.AutomaticWorkCounts, outcome string, n int) error {
	switch outcome {
	case workNotMine:
		c.NotMine += n
	case workRerouted:
		c.Rerouted += n
	case workDismissed:
		c.Dismissed += n
	case workRejected:
		c.Rejected += n
	case workCompleted:
		c.Completed += n
	case workExpired:
		c.Expired += n
	case workOpen:
		c.Open += n
	default:
		return fmt.Errorf("automatic work: unknown outcome %q", outcome)
	}
	c.Created += n
	return nil
}

// tallies gives one tally per kind, zero when the colleague had none of it.
func tallies(colleague colleagueDays) []crmcontracts.AutomaticWorkTally {
	out := make([]crmcontracts.AutomaticWorkTally, 0, len(automaticWorkKinds))
	for _, kind := range automaticWorkKinds {
		tally := crmcontracts.AutomaticWorkTally{Kind: kind.Kind, ByDay: []crmcontracts.AutomaticWorkDay{}}
		perDay := colleague[kind.Kind]
		dates := make([]time.Time, 0, len(perDay))
		for day := range perDay {
			dates = append(dates, day)
		}
		slices.SortFunc(dates, func(a, b time.Time) int { return a.Compare(b) })
		for _, day := range dates {
			counts := perDay[day]
			tally.ByDay = append(tally.ByDay, crmcontracts.AutomaticWorkDay{Date: calendarDay(day), Counts: counts})
			sumCounts(&tally.Counts, counts)
		}
		tally.SmallSample, tally.NotMineShare, tally.AboveThreshold = automaticRouting(tally.Counts, kind.RecordsNotMine)
		out = append(out, tally)
	}
	return out
}

func sumCounts(total *crmcontracts.AutomaticWorkCounts, day crmcontracts.AutomaticWorkCounts) {
	total.Created += day.Created
	total.NotMine += day.NotMine
	total.Rerouted += day.Rerouted
	total.Dismissed += day.Dismissed
	total.Rejected += day.Rejected
	total.Completed += day.Completed
	total.Expired += day.Expired
	total.Open += day.Open
}

func automaticRouting(counts crmcontracts.AutomaticWorkCounts, recordsNotMine bool) (bool, *float32, bool) {
	small := counts.Created < automaticWorkSmallSample
	if !recordsNotMine || counts.Created == 0 {
		return small, nil, false
	}
	share := float32(counts.NotMine) / float32(counts.Created)
	return small, &share, !small && share > automaticWorkShareThreshold
}
