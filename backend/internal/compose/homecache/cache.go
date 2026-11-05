// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package homecache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Life is how long an answer is served at most: how far behind Home may be
// for what colleagues and background work changed.
const Life = 5 * time.Minute

// Read names one of Home's cached reads.
type Read string

const (
	// Worklist is the day's first page, as Home opens it.
	Worklist Read = "worklist"
	// Magic is the receipt of what the machinery did.
	Magic Read = "magic"
)

// refreshAhead is how near the end of Life a served answer is read again in the
// background. A reader who keeps Home open then never waits on its expiry.
const refreshAhead = time.Minute

// Cache holds the answers.
type Cache struct {
	pool *pgxpool.Pool
	// refreshing names the answers being read again in the background, so one
	// question is never assembled twice at once.
	refreshing sync.Map
	background sync.WaitGroup
}

// New binds the cache.
func New(pool *pgxpool.Pool) *Cache {
	return &Cache{pool: pool}
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

// Lookup answers the reader's kept answer to read and key, how old it is, and
// whether one was still valid.
func Lookup[A Answer](ctx context.Context, c *Cache, reader ids.UUID, read Read, key string) (A, time.Duration, bool, error) {
	var out A
	var payload []byte
	var ageSeconds float64
	err := database.WithWorkspaceTx(ctx, c.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT c.payload, extract(epoch FROM now() - c.computed_at)::float8
			  FROM home_read_cache c
			  JOIN home_read_fence f ON f.generation = c.fence
			 WHERE c.reader_id = $1 AND c.read = $2 AND c.request_key = $3
			   AND c.valid_until > now()
			   AND NOT EXISTS (
			         SELECT 1 FROM home_read_forgotten fg
			          WHERE fg.reader_id = c.reader_id AND fg.at >= c.computed_at)
			   AND (c.walk_id IS NULL OR EXISTS (
			         SELECT 1 FROM worklist_snapshot w
			          WHERE w.id = c.walk_id AND w.reader_id = c.reader_id AND w.expires_at > now()))`,
			reader, string(read), key).Scan(&payload, &ageSeconds)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, 0, false, nil
	}
	if err != nil {
		return out, 0, false, fmt.Errorf("homecache: reading a cached %s: %w", read, err)
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return out, 0, false, fmt.Errorf("homecache: decoding a cached %s: %w", read, err)
	}
	return out, time.Duration(ageSeconds * float64(time.Second)), true, nil
}

// Serve answers read from the reader's kept answer while it holds, and from
// fresh otherwise, keeping what fresh answered. An answer served near the end
// of its life is read again in the background. The cache only saves work: one
// that cannot be read or written is logged and passed over, and the answer is
// the fresh read's. walkOf names the walk an answer's first page started.
func Serve[A Answer](
	ctx context.Context, c *Cache, reader ids.UUID, read Read, key string,
	fresh func(context.Context) (A, error), walkOf func(A) (*ids.UUID, error),
) (A, error) {
	kept, age, hit, err := Lookup[A](ctx, c, reader, read, key)
	if err != nil {
		slog.ErrorContext(ctx, "a kept Home answer could not be read; reading it fresh", "read", read, "error", err)
	}
	if !hit {
		return keepFresh(ctx, c, reader, read, key, fresh, walkOf)
	}
	if age > Life-refreshAhead {
		c.refresh(ctx, string(read)+"\x00"+reader.String()+"\x00"+key, func(ctx context.Context) error {
			_, err := keepFresh(ctx, c, reader, read, key, fresh, walkOf)
			return err
		})
	}
	return kept, nil
}

// keepFresh assembles the answer and keeps it, stamped before it was read.
func keepFresh[A Answer](
	ctx context.Context, c *Cache, reader ids.UUID, read Read, key string,
	fresh func(context.Context) (A, error), walkOf func(A) (*ids.UUID, error),
) (A, error) {
	stamp, stampErr := c.Stamp(ctx)
	out, err := fresh(ctx)
	if err != nil {
		return out, err
	}
	if stampErr != nil {
		slog.ErrorContext(ctx, "the Home fence could not be read; the answer is not kept", "read", read, "error", stampErr)
		return out, nil
	}
	walk, err := walkOf(out)
	if err == nil {
		err = Store(ctx, c, reader, read, key, stamp, walk, out)
	}
	if err != nil {
		slog.ErrorContext(ctx, "a Home answer could not be kept", "read", read, "error", err)
	}
	return out, nil
}

// refresh runs read once in the background, under the request's identity but
// not its deadline. The reader has their answer; this keeps the next one.
func (c *Cache) refresh(ctx context.Context, question string, read func(context.Context) error) {
	if _, running := c.refreshing.LoadOrStore(question, true); running {
		return
	}
	c.background.Go(func() {
		defer c.refreshing.Delete(question)
		// A refresh that outlasts the answer's remaining life is no longer ahead of it.
		detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), refreshAhead)
		defer cancel()
		if err := read(detached); err != nil {
			slog.ErrorContext(detached, "a Home answer could not be read again ahead of its expiry", "error", err)
		}
	})
}

// Settle waits for the background refreshes under way.
func (c *Cache) Settle() { c.background.Wait() }

// Stamp reads the fence before the answer is assembled. A revocation that
// commits after this read moves the fence past the stamp, so the answer is
// never served beside it. Expiry writes nothing, so the next of the reader's
// own grants to expire bounds the life.
func (c *Cache) Stamp(ctx context.Context) (Stamp, error) {
	actor, _ := principal.Actor(ctx)
	var s Stamp
	err := database.WithWorkspaceTx(ctx, c.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT f.generation, now(),
			       least(now() + make_interval(secs => $1),
			             coalesce((SELECT min(g.expires_at) FROM record_grant g
			                        WHERE g.expires_at > now()
			                          AND ((g.subject_type = 'user'
			                                AND g.subject_id IN (SELECT unnest(member_identity_family($2::uuid))))
			                            OR (g.subject_type = 'team' AND g.subject_id = ANY($3)))),
			                      'infinity'::timestamptz))
			  FROM home_read_fence f`,
			Life.Seconds(), actor.UserID, actor.TeamIDs).Scan(&s.fence, &s.at, &s.validUntil)
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
			       payload = EXCLUDED.payload
			 WHERE home_read_cache.computed_at <= EXCLUDED.computed_at`,
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
			ON CONFLICT (reader_id) DO UPDATE SET at = greatest(home_read_forgotten.at, EXCLUDED.at)`, readers)
		return err
	})
	if err != nil {
		return fmt.Errorf("homecache: ending a reader's answers: %w", err)
	}
	return nil
}
