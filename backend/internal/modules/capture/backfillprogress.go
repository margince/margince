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
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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

// backfillRunOf names the run whose page this context carries, or nil on
// live sync. A sender queued here is credited to it when its verdict lands.
func backfillRunOf(ctx context.Context) *ids.UUID {
	if page := pageProgressFrom(ctx); page != nil {
		return &page.backfillID
	}
	return nil
}

// Observed takes the connector's running count for this page.
func (c *pageProgress) Observed(ctx context.Context, scanned, captured, skipped int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tally.advance(scanned, captured, skipped) {
		c.persist(ctx, paced)
	}
}

// Credit evidence and counters commit together. A creation survives cancellation
// and rebind, so its retry is not fenced by the page's current ownership.
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
	countCtx, cancel := detachedWrite(database.WithoutTransactionGuard(ctx))
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
	if outcome.ContactCreated && outcome.ContactID != (ids.UUID{}) {
		out = append(out, createdSubject{kind: "contact", subject: outcome.ContactID.String()})
	}
	// The company kind counts domains this run QUEUED for a verdict, not
	// companies it created — capture creates none. A run that met twelve new
	// domains did that work whether or not the crawls have answered yet, and
	// reporting zero would hide it.
	if outcome.CompanyQueued && outcome.QueuedDomain != "" {
		out = append(out, createdSubject{kind: "company_queued", subject: outcome.QueuedDomain})
	}
	return out
}

// Ledger insertion and its counter delta commit together; retries credit only new rows.
func (c *pageProgress) recordCreations(ctx context.Context, created []createdSubject) error {
	return c.registry.db.Tx(ctx, func(tx pgx.Tx) error {
		return recordCreationsTx(ctx, tx, c.backfillID, created)
	})
}

func recordCreationsTx(ctx context.Context, tx pgx.Tx, backfillID ids.UUID, created []createdSubject) error {
	if _, err := tx.Exec(ctx,
		`SELECT 1 FROM capture_backfill WHERE id = $1 FOR UPDATE`, backfillID); err != nil {
		return err
	}
	kinds := make([]string, 0, len(created))
	subjects := make([]string, 0, len(created))
	for _, made := range created {
		kinds = append(kinds, made.kind)
		subjects = append(subjects, made.subject)
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	run := arg(backfillID)
	query := fmt.Sprintf(`
  WITH credited AS (
   INSERT INTO capture_backfill_creation (backfill_id, kind, subject)
   SELECT $%d, kind, subject FROM unnest($%d::text[], $%d::text[]) AS c(kind, subject)
   ON CONFLICT (backfill_id, kind, subject) DO NOTHING
   RETURNING kind
  )
  UPDATE capture_backfill b
  SET contacts_created = b.contacts_created + counted.contacts,
      companies_created = b.companies_created + counted.companies
  FROM (
   SELECT count(*) FILTER (WHERE kind = 'contact') AS contacts,
          count(*) FILTER (WHERE kind = 'company_queued') AS companies
   FROM credited
  ) counted
  WHERE b.id = $%d`, run, arg(kinds), arg(subjects), run)
	_, err := tx.Exec(ctx, query, args...)
	return err
}

// Completion repairs drift once without lowering credits absent from the ledger.
func reconcileBackfillCreationsTx(ctx context.Context, tx pgx.Tx, backfillID ids.UUID) error {
	args := []any{backfillID}
	run := len(args)
	lock := fmt.Sprintf(`SELECT 1 FROM capture_backfill WHERE id = $%d FOR UPDATE`, run)
	if _, err := tx.Exec(ctx, lock, args...); err != nil {
		return err
	}
	query := fmt.Sprintf(`
  UPDATE capture_backfill b
  SET contacts_created = greatest(counted.contacts, b.contacts_created),
      companies_created = greatest(counted.companies, b.companies_created)
  FROM (
   SELECT count(*) FILTER (WHERE kind = 'contact') AS contacts,
          count(*) FILTER (WHERE kind = 'company_queued') AS companies
   FROM capture_backfill_creation WHERE backfill_id = $%[1]d
  ) counted
  WHERE b.id = $%[1]d`, run)
	_, err := tx.Exec(ctx, query, args...)
	return err
}

// CreditBackfillRunTx credits the run that queued a sender with what the
// verdict on that sender made. A sender live sync queued credits no run.
func CreditBackfillRunTx(ctx context.Context, tx pgx.Tx, dispositionID ids.UUID, outcome EnsureOutcome) error {
	created := createdSubjects(outcome)
	if len(created) == 0 {
		return nil
	}
	run, err := backfillRunForDispositionTx(ctx, tx, dispositionID)
	if err == nil && run == nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("capture: reading the run that queued a sender: %w", err)
	}
	return recordCreationsTx(ctx, tx, *run, created)
}

// BackfillContextForDispositionTx preserves import provenance when a deferred sender becomes a contact.
func BackfillContextForDispositionTx(ctx context.Context, tx pgx.Tx, dispositionID ids.UUID) (context.Context, error) {
	run, err := backfillRunForDispositionTx(ctx, tx, dispositionID)
	if err != nil || run == nil {
		return ctx, err
	}
	return storekit.WithBatch(ctx, *run), nil
}

func backfillRunForDispositionTx(ctx context.Context, tx pgx.Tx, dispositionID ids.UUID) (*ids.UUID, error) {
	var run *ids.UUID
	args := []any{dispositionID}
	err := tx.QueryRow(ctx, `SELECT backfill_id FROM capture_pending_counterparty WHERE id = `+storekit.Placeholders(args), args...).Scan(&run)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return run, err
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

// flushBackfillProgress reports only the admitted run and its unchanged connection generation.
func (r *Registry) flushBackfillProgress(ctx context.Context, backfillID ids.UUID, generation int, t pageTally) error {
	return r.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE capture_backfill
			SET inflight_scanned = $2, inflight_captured = $3, inflight_skipped = $4
			WHERE id = $1 AND status = 'running'
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
