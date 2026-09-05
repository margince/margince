// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// The read: one window, four lanes, one page.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// LastBrief answers when the acting rep's overnight run last read the records.
//
// A seam because the brief lives in a sibling compose package, and because an
// installation without one is a real state rather than a failure: the window
// then falls back to a day, which is the honest default for "since you last
// looked" when nothing knows when that was.
type LastBrief interface {
	CutoffFor(ctx context.Context) (time.Time, bool, error)
}

// Service answers the machinery's receipt.
type Service struct {
	pool  *pgxpool.Pool
	brief LastBrief
	// troubled is OPTIONAL: unbound, the could-not-complete lane reports the
	// approvals half alone rather than refusing the page. An installation that
	// runs no automations has nothing to say there, and a lane that failed
	// because a seam was not wired would look exactly like one with nothing in
	// it.
	troubled TroubledRuns
	now      func() time.Time
}

// NewService binds the read.
func NewService(pool *pgxpool.Pool, brief LastBrief, now func() time.Time) *Service {
	return &Service{pool: pool, brief: brief, now: now}
}

// WithTroubledRuns binds the automation-health half of the could-not-complete
// lane. An option for the reason the attention feed's lanes are options.
func (s *Service) WithTroubledRuns(t TroubledRuns) *Service {
	s.troubled = t
	return s
}

// defaultWindow is how far back a reader with no brief is shown.
//
// A DAY, not a week: this surface answers "what happened since you last looked",
// and a reader opening it in the morning last looked yesterday. A longer window
// would report work they have already seen and buried the overnight run in it.
const defaultWindow = 24 * time.Hour

// maxLimit bounds a page however large a caller asks for. The contract states
// it too; this is the half that holds when a caller ignores the contract.
const maxLimit = 100

// maxLookback bounds the WINDOW the same way, and for a sharper reason.
//
// This surface answers "what happened since you last looked". Without a floor,
// any authenticated seat could pass since=1970 and page the whole ledger back to
// installation — turning a morning receipt into an arbitrary historical
// audit-log read for a seat holding one object grant, and making every scan
// unbounded besides.
//
// A month, because that is the outer edge of "recently" for a receipt somebody
// reads daily. A reader who wants more history has the record's own page, which
// is where history is a feature rather than a side effect.
const maxLookback = 30 * 24 * time.Hour

// Read answers the receipt for one window.
func (s *Service) Read(
	ctx context.Context, since *time.Time, limit int,
) (crmcontracts.MagicReceipt, error) {
	asOf := s.now().UTC()
	from, err := s.windowStart(ctx, since, asOf)
	if err != nil {
		return crmcontracts.MagicReceipt{}, err
	}
	if limit <= 0 || limit > maxLimit {
		limit = maxLimit
	}
	receipt := crmcontracts.MagicReceipt{
		AsOf:  asOf,
		Since: from,
		// Every lane starts as an empty slice, never nil: the contract declares
		// them as arrays, and a nil serialises as `null` and breaks a generated
		// client that iterates what the schema promised was a list.
		Done:               []crmcontracts.MagicLine{},
		NeedsYou:           []crmcontracts.MagicLine{},
		CouldNotComplete:   []crmcontracts.MagicLine{},
		Watching:           []crmcontracts.MagicLine{},
		NotShown:           []crmcontracts.MagicNotShown{},
		SourcesUnavailable: []crmcontracts.WorklistSourceUnavailable{},
	}
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		entries, notShown, err := doneSince(ctx, tx, from, limit)
		if err != nil {
			return err
		}
		receipt.Done = linesOf(entries, limit)
		receipt.NotShown = notShownOf(notShown)
		failed, refused, err := s.couldNotComplete(ctx, from, limit)
		if err != nil {
			return err
		}
		receipt.CouldNotComplete = failed
		if refused != nil {
			receipt.SourcesUnavailable = append(receipt.SourcesUnavailable, *refused)
		}
		// THE TOTALS COUNT WHAT IS DRAWN, and say so by being derived from the
		// drawn lines rather than from the fetch behind them.
		//
		// An earlier version used len(entries), which is neither the page nor
		// the window: each of the six arms applies the same LIMIT separately, so
		// a bound of 100 could fetch 600 rows and report that as the total while
		// the page held 100. A figure wrong in both directions is worse than no
		// figure, because a client draws "5 of 23" from it and the 23 means
		// nothing.
		//
		// A true window count needs its own COUNT per arm without the bound.
		// That arrives with the cursor, which is the thing that makes a window
		// total worth having; until then the honest claim is the smaller one.
		receipt.Totals = crmcontracts.MagicTotals{
			Done:             len(receipt.Done),
			CouldNotComplete: len(failed),
		}
		return nil
	})
	if err != nil {
		return crmcontracts.MagicReceipt{}, fmt.Errorf("read the machinery's receipt: %w", err)
	}
	return receipt, nil
}

// windowStart resolves what "since" means for this reader.
//
// A caller's own instant wins. Absent, it is the acting rep's last brief cutoff
// — the moment the night READ the records, which is what they have already seen
// — and a day where no brief exists. The fallback is stated rather than silent:
// a reader with no brief still gets a window they can name.
func (s *Service) windowStart(
	ctx context.Context, since *time.Time, asOf time.Time,
) (time.Time, error) {
	// The floor holds whatever the caller asked for. maxLookback states why.
	floor := asOf.Add(-maxLookback)
	if since != nil {
		asked := since.UTC()
		if asked.Before(floor) {
			return floor, nil
		}
		return asked, nil
	}
	if s.brief == nil {
		return asOf.Add(-defaultWindow), nil
	}
	cutoff, ran, err := s.brief.CutoffFor(ctx)
	if err != nil {
		// A brief this reader may not see, or none at all, is not a failure of
		// this read: it falls back to the day. Any OTHER error is real and
		// reaches the caller rather than being swallowed into a default window
		// that would report an honest-looking receipt over a broken read.
		if !errors.Is(err, apperrors.ErrNotFound) &&
			!errors.Is(err, apperrors.ErrPermissionDenied) {
			return time.Time{}, err
		}
		return asOf.Add(-defaultWindow), nil
	}
	if !ran {
		return asOf.Add(-defaultWindow), nil
	}
	// The floor holds here too. A rep who has not opened the product in three
	// months has a brief cutoff three months old, and the receipt is not the
	// place to hand them the quarter's ledger.
	if resolved := cutoff.UTC(); resolved.After(floor) {
		return resolved, nil
	}
	return floor, nil
}

// notShownOf turns the counted omissions into the wire's shape.
func notShownOf(counts map[string]int) []crmcontracts.MagicNotShown {
	out := make([]crmcontracts.MagicNotShown, 0, len(counts))
	for reason, count := range counts {
		if count == 0 {
			continue
		}
		out = append(out, crmcontracts.MagicNotShown{
			Reason: crmcontracts.MagicNotShownReason(reason),
			Count:  count,
		})
	}
	return out
}
