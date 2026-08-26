// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package events is the Redis Streams side of the event backbone (events.md
// §3/§4): the outbox relay that ships committed writes onto the bus, the
// consumer-group subscriber, and the event_id dedupe wrapper that makes
// at-least-once delivery safe. The write side stays in the module stores
// (domain row + audit + outbox in one transaction); this package never
// originates an event — it only moves and delivers what a transaction
// already committed.
package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// NewClient returns the Redis client the composition root hands to the
// relay and subscribers, verified reachable — a bus that silently isn't
// there would strand every committed outbox row.
//
// The address may name a logical database as `host:port/N`. One Redis serves
// several stacks on a developer's machine, and the stream names and consumer
// groups are constants (`gw:events:crm:*`, `cg:*`), so two stacks on db 0 share
// ONE consumer group: whichever worker reads an entry first consumes it,
// resolves it against its OWN database, finds nothing, and acks. The other
// stack's event is simply gone, and the symptom — a projection that never
// runs — is indistinguishable from a broken feature. `make dev` gives each
// DEV_SLUG its own index for that reason.
func NewClient(ctx context.Context, addr string) (*redis.Client, error) {
	opts, err := ClientOptions(addr)
	if err != nil {
		return nil, err
	}
	rdb := redis.NewClient(opts)
	if err := rdb.Ping(ctx).Err(); err != nil {
		//craft:ignore swallowed-errors best-effort close of a client whose ping already failed — the unreachable-bus error is the one to report
		_ = rdb.Close()
		return nil, fmt.Errorf("bus: redis at %s unreachable: %w", addr, err)
	}
	return rdb, nil
}

// ClientOptions turns a bus address into Redis options, accepting an optional
// `/N` logical-database suffix.
//
// Spelled once and exported because the api builds a client of its own beside
// the relay's: two parsers would let one of them keep landing on db 0 while
// the other honoured the suffix, which is the same event-stealing bug wearing
// a different hat.
func ClientOptions(addr string) (*redis.Options, error) {
	// A UNIX SOCKET is an address, not a suffixed host: go-redis reads a
	// leading slash as one, and every path has slashes in it. Splitting those
	// would turn /var/run/redis.sock into host "" and database
	// "var/run/redis.sock" and refuse a deployment that worked before this
	// parameter existed.
	if strings.HasPrefix(addr, "/") {
		return &redis.Options{Network: "unix", Addr: addr}, nil
	}
	host, index, found := strings.Cut(addr, "/")
	if !found {
		return &redis.Options{Addr: addr}, nil
	}
	db, err := strconv.Atoi(index)
	if err != nil || db < 0 || db > maxRedisDB {
		return nil, fmt.Errorf(
			"bus: redis address %q names logical database %q, which is not an integer in 0..%d",
			addr, index, maxRedisDB)
	}
	return &redis.Options{Addr: host, DB: db}, nil
}

// maxRedisDB bounds the index this parser will accept. Redis's own default is
// `databases 16`; the dev compose raises it to 80 so the integration lane and
// the DEV_SLUG stacks each get a block. The ceiling here is deliberately the
// higher one: an index this parser refused but the instance serves would be a
// working configuration rejected at the door, and one it accepts that the
// instance does not connects and then fails on the first command — which reads
// as an unreachable bus, the error an operator can act on.
const maxRedisDB = 79

// publishedTotal counts rows shipped since process start — process-wide,
// not per-Relay, so the metrics endpoint reads it without holding the
// relay instance (which may live in this process or in cmd/worker).
var publishedTotal atomic.Uint64

// PublishedTotal reports how many outbox rows this process has shipped.
func PublishedTotal() uint64 { return publishedTotal.Load() }

// OutboxBacklog counts committed-but-unshipped outbox rows — the one
// number that says whether the relay is keeping up. event_outbox is a
// global infra table (no RLS), so the plain pool reads it.
func OutboxBacklog(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var n int64
	err := pool.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE published_at IS NULL`).Scan(&n)
	return n, err
}

// Relay is the events.md §4.2 outbox relay: it polls unpublished
// event_outbox rows in commit order, XADDs each envelope to its stream,
// and stamps published_at. Crash anywhere between XADD and the stamp and
// the row is re-published on the next pass — at-least-once by design;
// consumers dedupe on event_id (§3).
//
// Rows are claimed FOR UPDATE SKIP LOCKED, so concurrent relays (two app
// replicas) divide the backlog instead of double-publishing it in the
// steady state; the crash window above is the only duplicate source.
type Relay struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
	log  *slog.Logger

	// interval paces the idle poll; a full batch re-polls immediately so
	// a burst drains at Redis speed, not poll speed.
	interval  time.Duration
	batchSize int
	// maxLen caps each stream (XADD MAXLEN ~, events.md §4.4): streams
	// are a transient delivery buffer, audit_log is the permanent record.
	maxLen int64
}

// NewRelay applies the operational defaults: 200ms idle
// poll — well inside the "post-commit, best-effort" freshness consumers
// expect — batches of 256, streams capped near 2^17 entries (≈ the 72h
// §4.3 retention horizon at PoC write rates).
func NewRelay(pool *pgxpool.Pool, rdb *redis.Client, log *slog.Logger) *Relay {
	return &Relay{
		pool:      pool,
		rdb:       rdb,
		log:       log,
		interval:  200 * time.Millisecond,
		batchSize: 256,
		maxLen:    1 << 17,
	}
}

// Run relays until ctx is canceled. Errors are retried with backoff, not
// returned: the relay outliving a Redis blip is the whole point of the
// outbox — rows wait in Postgres until the bus is back.
func (r *Relay) Run(ctx context.Context) {
	backoff := r.interval
	for {
		published, err := r.relayBatch(ctx)
		switch {
		case err != nil && ctx.Err() != nil:
			return
		case err != nil:
			r.log.Error("bus: relay pass failed; outbox rows are safe and will retry", "error", err)
			backoff = min(backoff*2, 10*time.Second)
		default:
			backoff = r.interval
			if published == r.batchSize {
				continue // backlog: drain at full speed
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// outboxRow is one staged envelope; the relay ships envelope verbatim —
// it never parses or rewrites what the transaction committed.
type outboxRow struct {
	id       ids.UUID
	stream   string
	envelope []byte
}

// relayBatch publishes one batch and reports how many rows it shipped.
// On a mid-batch Redis failure the transaction still COMMITS the stamps
// of the rows that did ship (the failure is carried out of the closure
// instead of aborting it) — rolling back would re-publish the shipped
// prefix on every retry for as long as the bus stays down.
func (r *Relay) relayBatch(ctx context.Context) (int, error) {
	var published int
	var xaddErr error
	err := database.WithInfraTx(ctx, r.pool, func(tx pgx.Tx) error {
		// seq, not created_at: created_at is transaction-start time, so
		// a long tx could publish "before" an earlier-committed short
		// one. seq is insert-ordered, and row locks serialize same-entity
		// transactions, so per-entity seq order IS commit order
		// (migration 0016).
		rows, err := tx.Query(ctx,
			`SELECT id, stream, envelope FROM event_outbox
			 WHERE published_at IS NULL
			 ORDER BY seq
			 LIMIT $1
			 FOR UPDATE SKIP LOCKED`, r.batchSize)
		if err != nil {
			return err
		}
		batch, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (outboxRow, error) {
			var o outboxRow
			err := row.Scan(&o.id, &o.stream, &o.envelope)
			return o, err
		})
		if err != nil {
			return err
		}

		shipped := make([]ids.UUID, 0, len(batch))
		for _, row := range batch {
			err := r.rdb.XAdd(ctx, &redis.XAddArgs{
				Stream: row.stream,
				MaxLen: r.maxLen,
				Approx: true,
				Values: map[string]any{envelopeField: row.envelope},
			}).Err()
			if err != nil {
				xaddErr = fmt.Errorf("bus: XADD %s: %w", row.stream, err)
				break
			}
			shipped = append(shipped, row.id)
			publishedTotal.Add(1)
		}
		if len(shipped) == 0 {
			return xaddErr
		}

		if _, err := tx.Exec(ctx,
			`UPDATE event_outbox SET published_at = now() WHERE id = ANY($1)`, shipped); err != nil {
			return errors.Join(err, xaddErr)
		}
		published = len(shipped)
		return nil
	})
	if err != nil {
		return published, err
	}
	return published, xaddErr
}
