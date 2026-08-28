// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The running page's live tally. A run's counters advance once per COMMITTED
// page (backfillpager.go), and a page is a hundred messages of provider I/O
// and capture work — long enough that the activation view sat at zero for the
// whole first page and read as an import that never started.
//
// So the page also reports what it has walked so far into the run's inflight_*
// columns, and the status read adds the two. Those columns are advisory and
// transient by construction: every write that ends a page resets them, which
// is what keeps the committed counters the one authority and a retried page
// counted once.

package capture

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// pageTally is one page's live counts — absolute since the page began, never
// deltas, so a flush that never lands is corrected by the next one instead of
// leaving the row permanently short.
type pageTally struct {
	scanned  int
	captured int
	skipped  int
}

// advance folds a connector's report in and reports whether the tally moved. A
// report at or behind the one already held is dropped: a page walked
// concurrently can deliver two reports out of order, and the later-arriving
// lower number would make the count on screen go backwards.
func (t *pageTally) advance(scanned, captured, skipped int) bool {
	if scanned <= t.scanned {
		return false
	}
	t.scanned, t.captured, t.skipped = scanned, captured, skipped
	return true
}

// defaultProgressPacing paces the live write. A real import is thousands of
// messages, and one row update per message would be tens of thousands of
// writes to a single row so a number can move faster than anyone can read it.
// Half a second still reads as continuous motion; what the pacing drops is
// only ever an intermediate value, because the tally is absolute and the
// page's commit reconciles regardless.
const defaultProgressPacing = 500 * time.Millisecond

// pageProgress accumulates what ONE backfill page walks and creates, and
// persists it as it goes. Its two sources arrive on different seams: the
// connector reports scanned/captured/skipped through connector.BackfillProgress,
// while counterparty creations happen deep inside the Sink, which reads this
// collector straight off the context — widening connector.Sink to carry a
// count would change four connectors and every test fake for a number none of
// them can produce.
type pageProgress struct {
	// A page is a batch of independent messages and nothing promises a
	// connector walks it serially. The lock is held ACROSS the flush so the
	// row cannot take an older write after a newer one; the tally itself only
	// ever moves forward (Observed refuses a report behind the one it holds),
	// so the two together are what keep an on-screen count from going
	// backwards.
	mu        sync.Mutex
	tally     pageTally
	lastFlush time.Time

	backfillID ids.UUID
	// generation fences every write against a connection rebound under the
	// running page — the same fence the page's commit carries.
	generation int
	registry   *Registry
}

var _ connector.BackfillProgress = (*pageProgress)(nil)

// pageProgressKey is the private context key — unexported and typed, so no
// other package can install or read this.
type pageProgressKey struct{}

// withPageProgress installs a fresh collector for one page. Fresh per page,
// because the counters are folded in at page commit: a shared collector would
// double-count every page after the first.
func withPageProgress(ctx context.Context, r *Registry, backfillID ids.UUID, generation int) (context.Context, *pageProgress) {
	p := &pageProgress{backfillID: backfillID, generation: generation, registry: r}
	ctx = context.WithValue(ctx, pageProgressKey{}, p)
	return connector.WithBackfillProgress(ctx, p), p
}

// pageProgressFrom returns the collector this context carries, or nil when no
// backfill page is running — the incremental sync path, where a created
// counterparty belongs to no run. The methods reached this way (counted,
// totals) tolerate a nil receiver, so absence costs a branch and never a
// panic. Observed does not: it is only ever reached through
// connector.BackfillReporter, which holds a non-nil collector or discards the
// report itself.
func pageProgressFrom(ctx context.Context) *pageProgress {
	c, _ := ctx.Value(pageProgressKey{}).(*pageProgress)
	return c
}

// Observed takes the connector's running count for this page.
func (c *pageProgress) Observed(ctx context.Context, scanned, captured, skipped int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tally.advance(scanned, captured, skipped) {
		c.persist(ctx, paced)
	}
}

// counted records ONE counterparty creation on the run, immediately, in the
// committed columns.
//
// Per creation and not per page, because a page's total is a batch and a batch
// is something to lose. Whatever writes it can fail, and nothing can rebuild it
// afterwards: capture is idempotent, so a replayed message never reaches the
// resolver again and no retry re-offers these rows to anybody. Counting each row
// as it appears leaves no batch anywhere to be dropped, double-credited, or
// fenced off.
//
// A LEDGER ROW, not an increment, and that is what makes the reported number a
// count rather than a floor.
//
// It used to be `SET col = col + 1` in a transaction of its own, after the row
// it was counting had already committed elsewhere. That addition can fail and
// nothing replays it — capture is idempotent, so no retry ever re-offers the
// message to the resolver — so a database fault spanning a page lost one count
// for every creation inside it, permanently. The columns were a floor, and
// nothing downstream said so: they drive a progress display and divide into the
// cost estimator's ratios, where a floor understates cost.
//
// The ledger is keyed on WHAT WAS CREATED — a person's row id, or the domain a
// verdict was opened on — so writing the same creation twice writes it once.
// That is what makes the write RETRYABLE, and the retry is what shrinks the
// loss window from "any failure" to "a failure that outlives it".
//
// The columns are then a projection of the ledger, recomputed in the same
// transaction, so calling this twice for one creation reports the same number
// as calling it once.
//
// Unfenced on the run's liveness and on the connection's generation. The row
// exists; a cancelled run and a rebound connection do not un-create it, and
// refusing the count would only misreport it.
//
// Detached, because a creation that lands as the job's context expires must
// still be counted, and that context is the commonest thing to die here.
func (c *pageProgress) counted(ctx context.Context, outcome EnsureOutcome) {
	if c == nil {
		return
	}
	created := createdSubjects(outcome)
	if len(created) == 0 {
		// Resolved onto rows that already existed — on a widen re-import that is
		// nearly every message.
		return
	}
	countCtx, cancel := detachedWrite(ctx)
	defer cancel()
	var err error
	for attempt := range countAttempts {
		err = c.recordCreations(countCtx, created)
		if err == nil {
			return
		}
		// Retryable only because the write is idempotent. Backing off between
		// attempts, because the fault this is riding out is a database one and
		// an immediate retry meets the same moment.
		if attempt < countAttempts-1 {
			sleepFor(countCtx, countRetryBackoff)
		}
	}
	slog.ErrorContext(ctx, "capture: a counterparty this backfill created was not counted on the run — its reported reach is one row short",
		"backfill_id", c.backfillID, "attempts", countAttempts, "err", err)
}

// countAttempts and countRetryBackoff bound the retry. Three and a short pause,
// because this is riding out a blip rather than waiting for an outage: a run
// that stalled its page on a database that is down would trade the capture for
// the counter, which is the trade the flush path already refuses.
const (
	countAttempts     = 3
	countRetryBackoff = 200 * time.Millisecond
)

// sleepFor waits, or returns early if the context ends first.
func sleepFor(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// createdSubject is one thing a run made, as the ledger keys it.
type createdSubject struct {
	kind    string
	subject string
}

// createdSubjects names what one outcome created, or nothing.
func createdSubjects(outcome EnsureOutcome) []createdSubject {
	var out []createdSubject
	if outcome.PersonCreated && outcome.PersonID != (ids.UUID{}) {
		out = append(out, createdSubject{kind: "person", subject: outcome.PersonID.String()})
	}
	// The company kind counts domains this run QUEUED for a verdict, not
	// companies it created — capture creates none. A run that met twelve new
	// domains did that work whether or not the crawls have answered yet, and
	// reporting zero would hide it.
	if outcome.CompanyQueued && outcome.QueuedDomain != "" {
		out = append(out, createdSubject{kind: "organization_queued", subject: outcome.QueuedDomain})
	}
	return out
}

// recordCreations writes the ledger rows and refreshes the run's columns from
// it, in one transaction.
//
// Refreshed rather than incremented: the columns are then a PROJECTION of the
// ledger, so a second call for the same creation — which the retry above can
// produce — leaves them where they were, and a count read off the run row is
// the number of rows behind it.
//
// Never DOWN, though, and that is not a hedge against the ledger. A run still
// paging while this ships is walked by the old binary, which counts in the
// column and writes no ledger row; the migration seeds the ledger from a
// snapshot taken before those increments, so the first recompute after the
// rollout would otherwise discard them. `greatest` costs nothing in steady
// state — the ledger only grows, so the projection alone never lowers the
// number — and it is what makes the deploy window safe without draining every
// backfill first.
//
// The run row is LOCKED first, and the lock is what makes the projection safe
// under concurrency. A page may walk its messages in parallel — nothing in the
// connector contract forbids it — and at READ COMMITTED two recomputes running
// at once each count a snapshot without the other's uncommitted row, so the
// later committer writes a total that is missing the earlier one. An increment
// could not lose that; a projection can, and no later creation repairs it once
// the run has no creations left to make. Taken BEFORE the inserts, so the whole
// write is one queue rather than a race between the insert and the count.
func (c *pageProgress) recordCreations(ctx context.Context, created []createdSubject) error {
	return c.registry.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`SELECT 1 FROM capture_backfill WHERE id = $1 FOR UPDATE`, c.backfillID); err != nil {
			return err
		}
		for _, made := range created {
			if _, err := tx.Exec(ctx, `
				INSERT INTO capture_backfill_creation (backfill_id, kind, subject)
				VALUES ($1, $2, $3)
				ON CONFLICT (backfill_id, kind, subject) DO NOTHING`,
				c.backfillID, made.kind, made.subject); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `
			UPDATE capture_backfill b
			SET people_created = greatest(counted.people, b.people_created),
			    organizations_created = greatest(counted.organizations, b.organizations_created)
			FROM (
				SELECT count(*) FILTER (WHERE kind = 'person') AS people,
				       count(*) FILTER (WHERE kind = 'organization_queued') AS organizations
				  FROM capture_backfill_creation
				 WHERE backfill_id = $1
			) counted
			WHERE b.id = $1`, c.backfillID)
		return err
	})
}

// flushPacing says whether a write may be dropped for being too soon. Only the
// message tally is written this way, and the next report restates it.
type flushPacing bool

const (
	paced   flushPacing = true
	unpaced flushPacing = false
)

// persist writes the current tally to the run row, honouring the registry's
// progressPacing when the caller allows it. Caller holds the lock.
//
// A failure is logged and dropped rather than returned, and that is the
// deliberate call: this write exists so a screen can move, and failing a
// captured message — a real, committed CRM row — because its progress ping
// did not land would trade the product for the indicator. The next message
// restates the absolute tally, and the page's own commit reconciles
// regardless, so a lost flush costs one frame of animation and nothing else.
func (c *pageProgress) persist(ctx context.Context, pacing flushPacing) {
	now := c.registry.now()
	if pacing == paced && !c.lastFlush.IsZero() && now.Sub(c.lastFlush) < c.registry.progressPacing {
		return
	}
	c.lastFlush = now
	err := c.registry.flushBackfillProgress(ctx, c.backfillID, c.generation, c.tally)
	if err == nil {
		return
	}
	if ctx.Err() != nil {
		// The worker is shutting down or the job timed out. Every remaining
		// message in the page would fail the same way, and "the import is
		// unaffected" would be a lie — the page is ending too.
		return
	}
	slog.WarnContext(ctx, "capture: the backfill's live progress was not written — the import is unaffected and the page's commit will reconcile it",
		"backfill_id", c.backfillID, "err", err)
}

// flushBackfillProgress stores the running page's tally on the run row.
//
// It also promotes a 'queued' run to 'running', because by the time a page has
// walked a message the run demonstrably IS running — leaving it queued would
// put "Import queued" above a set of numbers that are climbing.
//
// It carries BOTH fences the commit carries, for the same reasons. The live
// states, so a run someone cancelled — or one a fault already ended — is not
// resurrected by a page that has not noticed yet. And the connection
// generation, so a page still walking the account the connection was rebound
// away from cannot report that account's mail as this run's progress: the
// commit will refuse the same page and cancel the run, and until it does the
// screen must not show work that is about to be thrown away.
func (r *Registry) flushBackfillProgress(ctx context.Context, backfillID ids.UUID, generation int, t pageTally) error {
	return r.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE capture_backfill
			SET inflight_scanned = $2, inflight_captured = $3, inflight_skipped = $4,
			    status = CASE WHEN status = 'queued' THEN 'running' ELSE status END
			WHERE id = $1 AND status IN ('queued','running')
			  AND EXISTS (SELECT 1 FROM capture_connection c
			              WHERE c.id = capture_backfill.connection_id AND c.generation = $5)`,
			backfillID, t.scanned, t.captured, t.skipped, generation)
		return err
	})
}

// resetInflightProgress is what the page COMMIT carries: the commit folds the
// page's work into the committed columns from the connector's own result and
// the collector's totals, so the transient copy has done its job and goes.
//
// It BEGINS with a comma, so it splices in after an existing SET assignment
// and never as the first one.
const resetInflightProgress = `, inflight_scanned = 0, inflight_captured = 0, inflight_skipped = 0`
