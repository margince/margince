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
		// The TOTALS are over the window rather than over the page: a preview
		// drawing five lines must be able to say "5 of 23" without paging to
		// find out, and counting the array it was handed would report the page
		// as the total.
		receipt.Totals = crmcontracts.MagicTotals{
			Done:             len(entries),
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
	if since != nil {
		return since.UTC(), nil
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
	return cutoff.UTC(), nil
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
