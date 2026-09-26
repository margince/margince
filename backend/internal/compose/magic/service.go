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
	// troubled is OPTIONAL: unbound, the could-not-complete lane is empty rather
	// than the page refusing. An installation that runs no automations has
	// nothing to say there, and a lane that failed because a seam was not wired
	// would look exactly like one with nothing in it.
	troubled TroubledRuns
	// undo is OPTIONAL for the same reason: unbound, every done line reads
	// not-undoable with a stated reason rather than the page refusing.
	undo UndoJudge
	// sources is OPTIONAL for the same reason: unbound, the watching lane is
	// empty rather than the page refusing. An installation that captures from
	// nothing has no source health to report.
	sources SourceHealth
	// pending is OPTIONAL for the same reason: unbound, the needs-you lane is
	// empty rather than the page refusing.
	pending PendingDecisions
	now     func() time.Time
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
		lines, housekeeping := linesOf(entries, limit)
		// Asked after the lines are drawn and inside the page's own
		// transaction: the judge reads the record each line names, and a
		// second connection inside this one can deadlock against a lock it
		// holds.
		if err := s.judgeUndoOn(ctx, tx, lines); err != nil {
			return err
		}
		// A line standing for many records has no single way back: the judge
		// read one of its rows, and its answer about that row is not an answer
		// about the rest. The line says nothing rather than something untrue.
		for i := range lines {
			if lines[i].Count != nil && *lines[i].Count > 1 {
				lines[i].Undo = nil
			}
		}
		// Retention, as counts beside the records' own lines; it names no
		// record, so it has nothing for the undo judge to read.
		retention, err := retentionSince(ctx, tx, from)
		if err != nil {
			return err
		}
		receipt.Done = mergeNewestFirst(retention, lines, limit)
		if housekeeping > 0 {
			notShown[string(crmcontracts.MagicNotShownReasonMagicNotShownUnadmittedAction)] += housekeeping
		}
		receipt.NotShown = notShownOf(notShown)
		return nil
	})
	if err != nil {
		return crmcontracts.MagicReceipt{}, fmt.Errorf("read the machinery's receipt: %w", err)
	}
	// THE OTHER THREE LANES READ OUTSIDE THAT TRANSACTION, and must.
	//
	// Each reaches its module through a seam that opens its own connection. Run
	// inside the page's transaction they would hold one while acquiring a
	// second, which at concurrency equal to the pool size is every request
	// holding what the next one waits for. Only the done lane needs the page's
	// snapshot, because only its undo judge reads the records its lines name.
	if err := s.gatherSeamLanes(ctx, &receipt, from, limit); err != nil {
		return crmcontracts.MagicReceipt{}, fmt.Errorf("read the machinery's receipt: %w", err)
	}
	// THE TOTALS COUNT WHAT IS DRAWN, and say so by being derived from the
	// drawn lines rather than from the fetch behind them. The fetch is neither
	// the page nor the window: each of the six arms applies the same LIMIT
	// separately, so a bound of 100 can pull 600 rows while the page holds 100,
	// and a client drawing "5 of 23" from that reads a 23 that means nothing.
	//
	// A true window count needs its own COUNT per arm without the bound. That
	// arrives with the cursor, which is the thing that makes a window total
	// worth having; until then the honest claim is the smaller one.
	receipt.Totals = crmcontracts.MagicTotals{
		Done:             len(receipt.Done),
		NeedsYou:         len(receipt.NeedsYou),
		CouldNotComplete: len(receipt.CouldNotComplete),
		Watching:         len(receipt.Watching),
	}
	return receipt, nil
}

// gatherSeamLanes fills the three lanes that come from a module rather than
// from this page's own query, and names every source that refused.
//
// A refusal is COLLECTED, never short-circuited: one withheld lane must not
// hide another, or an administrator who lost two grants is told about one.
func (s *Service) gatherSeamLanes(
	ctx context.Context, receipt *crmcontracts.MagicReceipt, from time.Time, limit int,
) error {
	failed, refused, err := s.couldNotComplete(ctx, from, limit)
	if err != nil {
		return err
	}
	receipt.CouldNotComplete = failed
	waiting, queueRefused, err := s.needsYou(ctx, limit)
	if err != nil {
		return err
	}
	receipt.NeedsYou = waiting
	// The window does not bound this lane. A standing condition is true now or
	// it is not, and a mailbox that broke before the reader last looked is
	// exactly the one they most need told about.
	watched, sourceRefused, err := s.watching(ctx, receipt.AsOf)
	if err != nil {
		return err
	}
	receipt.Watching = watched
	for _, unavailable := range []*crmcontracts.WorklistSourceUnavailable{
		refused, queueRefused, sourceRefused,
	} {
		if unavailable != nil {
			receipt.SourcesUnavailable = append(receipt.SourcesUnavailable, *unavailable)
		}
	}
	return nil
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
