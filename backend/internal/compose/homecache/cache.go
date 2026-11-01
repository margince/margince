// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package homecache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Life is how long an answer is served at most: how far behind Home may be
// for what other people and background work changed.
const Life = 5 * time.Minute

// Read names one of Home's cached reads.
type Read string

const (
	// Worklist is the day's first page, as Home opens it.
	Worklist Read = "worklist"
	// Magic is the receipt of what the machinery did.
	Magic Read = "magic"
)

// Cache holds the answers.
type Cache struct {
	pool *pgxpool.Pool
	// machineActors are the audit actor types whose rows end a receipt's answer.
	machineActors []string
}

// New binds the cache. machineActors names the audit actor types a receipt
// reports, so a row any of them writes ends the receipt's cached answer.
func New(pool *pgxpool.Pool, machineActors []string) *Cache {
	return &Cache{pool: pool, machineActors: machineActors}
}

// Stamp is what a read records before it is assembled. It holds the fence
// generation, when the read began, and the last instant it may be served.
type Stamp struct {
	fence      int64
	at         time.Time
	validUntil time.Time
}

// ReaderOf answers the human whose answer this request may read or keep. Any
// other principal is never cached: an agent's reach is its passport's, and a
// system or connector principal has no reader to key on.
func ReaderOf(ctx context.Context) (ids.UUID, bool) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return ids.UUID{}, false
	}
	return actor.UserID, true
}

// Answer is a Home read this cache keeps.
type Answer interface {
	crmcontracts.Worklist | crmcontracts.MagicReceipt
}

// Lookup answers the reader's kept answer to read and key, and whether one was
// still valid.
func Lookup[A Answer](ctx context.Context, c *Cache, reader ids.UUID, read Read, key string) (A, bool, error) {
	var out A
	actors := []string{}
	if read == Magic {
		actors = c.machineActors
	}
	var payload []byte
	err := database.WithWorkspaceTx(ctx, c.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT c.payload
			  FROM home_read_cache c
			  JOIN home_read_fence f ON f.generation = c.fence
			 WHERE c.reader_id = $1 AND c.read = $2 AND c.request_key = $3
			   AND c.valid_until > now()
			   AND NOT EXISTS (
			         SELECT 1 FROM home_read_forgotten fg
			          WHERE fg.reader_id = c.reader_id AND fg.at >= c.computed_at)
			   AND (c.walk_id IS NULL OR EXISTS (
			         SELECT 1 FROM worklist_snapshot w
			          WHERE w.id = c.walk_id AND w.reader_id = c.reader_id AND w.expires_at > now()))
			   AND NOT EXISTS (
			         SELECT 1 FROM audit_log a
			          WHERE a.occurred_at >= c.computed_at AND a.actor_type = ANY($4))`,
			reader, string(read), key, actors).Scan(&payload)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, fmt.Errorf("homecache: reading a cached %s: %w", read, err)
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return out, false, fmt.Errorf("homecache: decoding a cached %s: %w", read, err)
	}
	return out, true, nil
}

// Stamp reads the fence before the answer is assembled. A revocation that
// commits after this read moves the fence past the stamp, so the answer is
// never served beside it. Grant expiry writes nothing, so it bounds the life.
func (c *Cache) Stamp(ctx context.Context) (Stamp, error) {
	var s Stamp
	err := database.WithWorkspaceTx(ctx, c.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT f.generation, now(),
			       least(now() + make_interval(secs => $1),
			             coalesce((SELECT min(g.expires_at) FROM record_grant g WHERE g.expires_at > now()),
			                      'infinity'::timestamptz))
			  FROM home_read_fence f`, Life.Seconds()).Scan(&s.fence, &s.at, &s.validUntil)
	})
	if err != nil {
		return Stamp{}, fmt.Errorf("homecache: reading the access fence: %w", err)
	}
	return s, nil
}

// Store keeps value as the reader's answer to read and key. walk is the walk
// its first page started, when it started one.
func Store[A Answer](
	ctx context.Context, c *Cache, reader ids.UUID, read Read, key string, stamp Stamp, walk *ids.UUID, value A,
) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("homecache: encoding a %s answer: %w", read, err)
	}
	err = database.WithWorkspaceTx(ctx, c.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO home_read_cache
			       (reader_id, read, request_key, fence, computed_at, valid_until, walk_id, payload)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (reader_id, read, request_key) DO UPDATE
			   SET fence = EXCLUDED.fence, computed_at = EXCLUDED.computed_at,
			       valid_until = EXCLUDED.valid_until, walk_id = EXCLUDED.walk_id,
			       payload = EXCLUDED.payload`,
			reader, string(read), key, stamp.fence, stamp.at, stamp.validUntil, walk, payload)
		return err
	})
	if err != nil {
		return fmt.Errorf("homecache: keeping a %s answer: %w", read, err)
	}
	return nil
}

// Forget ends every answer these readers were computed before now, including
// one a read already in flight stores afterwards.
func (c *Cache) Forget(ctx context.Context, readers []ids.UUID) error {
	if len(readers) == 0 {
		return nil
	}
	err := database.WithWorkspaceTx(ctx, c.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO home_read_forgotten (reader_id, at)
			SELECT r, clock_timestamp() FROM unnest($1::uuid[]) r
			ON CONFLICT (reader_id) DO UPDATE SET at = EXCLUDED.at`, readers)
		return err
	})
	if err != nil {
		return fmt.Errorf("homecache: ending a reader's answers: %w", err)
	}
	return nil
}
