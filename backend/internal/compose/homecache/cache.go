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

	"github.com/margince/margince/backend/internal/compose/briefs"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Life is how long an answer is served as fresh: how far behind Home may be
// for what colleagues and background work changed.
const Life = 5 * time.Minute

// StaleGrace is how long past Life a day answer is still served, flagged stale,
// while it is read again behind it. Home's key names the day, which ends it
// sooner. Grant expiry, a snooze's end, the fence and a forget end it at once.
const StaleGrace = 24 * time.Hour

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
// generation, when the read began and the last instant it may be served. It
// also names the reader's reply and meeting snoozes that had lifted.
type Stamp struct {
	fence      int64
	at         time.Time
	validUntil time.Time
	lifted     []byte
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

// Kept says how a kept answer stands: its age, whether its reader has written
// since, and whether the walk its first page started has ended.
type Kept struct {
	Age       time.Duration
	Written   bool
	WalkEnded bool
}

// Walk is how an answer's first page relates to the walk it started. A walk
// lives four hours and a day answer up to a day, so an answer outlives its walk.
type Walk[A Answer] struct {
	// Started names the walk the answer's first page started, or nil.
	Started func(A) (*ids.UUID, error)
	// Ended is the answer once that walk has ended: it pages without the walk.
	Ended func(A) (A, error)
}

// ErrReadFresh is Walk.Ended's expected refusal: the kept answer cannot page on
// without its walk, so Serve reads fresh and logs nothing.
var ErrReadFresh = errors.New("homecache: the kept answer is read fresh")

// NoWalk is the Walk of an answer that never starts one.
func NoWalk[A Answer]() Walk[A] {
	return Walk[A]{
		Started: func(A) (*ids.UUID, error) {
			return nil, nil
		},
		Ended: func(answer A) (A, error) { return answer, nil },
	}
}

// Lookup answers the reader's kept answer to read and key, how it stands, and
// whether one was still valid. An answer behind the reader's write stays valid
// and says so in Kept.Written, and one whose walk has ended says so in
// Kept.WalkEnded. The access fence, expiry and a lifted snooze end it.
func Lookup[A Answer](ctx context.Context, c *Cache, reader ids.UUID, read Read, key string) (A, Kept, bool, error) {
	out, standing, found, err := lookup[A](ctx, c, reader, read, key)
	return out, standing, found == foundValid, err
}

// found is what Lookup found under a question: nothing ever kept, an answer
// that has ended, or one still valid.
type found int

const (
	foundNone found = iota
	foundEnded
	foundValid
)

func lookup[A Answer](ctx context.Context, c *Cache, reader ids.UUID, read Read, key string) (A, Kept, found, error) {
	var out A
	var payload []byte
	var ageSeconds float64
	var standing Kept
	var valid bool
	err := database.WithWorkspaceTx(ctx, c.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT c.payload, extract(epoch FROM now() - c.computed_at)::float8,
			       EXISTS (SELECT 1 FROM home_read_forgotten fg
			                WHERE fg.reader_id = c.reader_id AND fg.at >= c.computed_at),
			       c.walk_id IS NOT NULL AND NOT EXISTS (
			         SELECT 1 FROM worklist_snapshot w
			          WHERE w.id = c.walk_id AND w.reader_id = c.reader_id AND w.expires_at > now()),
			       c.valid_until > now()
			         AND EXISTS (SELECT 1 FROM home_read_fence f WHERE f.generation = c.fence)
			         AND c.lifted_snooze_digest IS NOT DISTINCT FROM `+briefs.LiftedSnoozesDigestSQL("c.reader_id", "now()")+`
			  FROM home_read_cache c
			 WHERE c.reader_id = $1 AND c.read = $2 AND c.request_key = $3`,
			reader, string(read), key).Scan(&payload, &ageSeconds, &standing.Written, &standing.WalkEnded, &valid)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, Kept{}, foundNone, nil
	}
	if err != nil {
		return out, Kept{}, foundNone, fmt.Errorf("homecache: reading a cached %s: %w", read, err)
	}
	if !valid {
		return out, Kept{}, foundEnded, nil
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return out, Kept{}, foundEnded, fmt.Errorf("homecache: decoding a cached %s: %w", read, err)
	}
	standing.Age = time.Duration(ageSeconds * float64(time.Second))
	return out, standing, foundValid, nil
}

// Serve answers read from the reader's kept answer while it holds, and from
// fresh otherwise, keeping what fresh answered. An answer near or past the end
// of its Life, behind a write, or past its walk is read again in the background.
// Stale says it is past its Life or behind a write; only the Worklist is served
// stale. A cache fault is logged and the fresh read answers.
func Serve[A Answer](
	ctx context.Context, c *Cache, reader ids.UUID, read Read, key string,
	fresh func(context.Context) (A, error), walk Walk[A],
) (answer A, stale bool, err error) {
	kept, standing, found, err := lookup[A](ctx, c, reader, read, key)
	hit := found == foundValid
	if err != nil {
		logFault(ctx, "a kept Home answer could not be read; reading it fresh", read, err)
	}
	if hit && standing.WalkEnded {
		if kept, err = walk.Ended(kept); err != nil {
			if !errors.Is(err, ErrReadFresh) {
				logFault(ctx, "a kept Home answer could not drop its ended walk; reading it fresh", read, err)
			}
			hit = false
		}
	}
	stale = hit && (standing.Age > Life || standing.Written)
	served := hit && (!stale || read == Worklist)
	switch {
	case !served && found == foundNone:
		countServed(read, outcomeFirst)
	case !served:
		countServed(read, outcomeMiss)
	case stale:
		countServed(read, outcomeStale)
	default:
		countServed(read, outcomeHit)
	}
	if !served {
		answer, err = keepFresh(ctx, c, reader, read, key, fresh, walk.Started)
		return answer, false, err
	}
	if stale || standing.WalkEnded || standing.Age > Life-refreshAhead {
		c.refresh(ctx, string(read)+"\x00"+reader.String()+"\x00"+key, func(ctx context.Context) error {
			_, err := keepFresh(ctx, c, reader, read, key, fresh, walk.Started)
			return err
		})
	}
	return kept, stale, nil
}

// logFault logs a cache fault at ERROR, or at DEBUG when the caller has left.
// A departed client is normal navigation, and an alert on errors must not fire
// on it.
func logFault(ctx context.Context, msg string, read Read, err error) {
	level := slog.LevelError
	if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		level = slog.LevelDebug
	}
	slog.Log(ctx, level, msg, slog.String("read", string(read)), slog.Any("error", err))
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
		logFault(ctx, "the Home fence could not be read; the answer is not kept", read, stampErr)
		return out, nil
	}
	walk, err := walkOf(out)
	if err == nil {
		err = Store(ctx, c, reader, read, key, stamp, walk, out)
	}
	if err != nil {
		logFault(ctx, "a Home answer could not be kept", read, err)
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
		// A refresh that outlasts the answer's remaining life is no longer ahead of it.
		detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), refreshAhead)
		defer cancel()
		if err := read(detached); err != nil {
			slog.ErrorContext(detached, "a Home answer could not be read again ahead of its expiry", "error", err)
			// A stale answer is asked for again within seconds; a failing read
			// is not repeated on each ask.
			time.AfterFunc(refreshAhead, func() { c.refreshing.Delete(question) })
			return
		}
		c.refreshing.Delete(question)
	})
}

// Settle waits for the background refreshes under way.
func (c *Cache) Settle() { c.background.Wait() }

// Stamp reads the fence before the answer is assembled. A revocation that
// commits after this read moves the fence past the stamp, so the answer is
// never served beside it. Expiry writes nothing, so the next of the reader's
// own grants to expire, and the next of their brief snoozes to end, bound the
// life. A reply or meeting snooze lifts without a write too. The stamp names
// the lifted ones, and Lookup ends the answer once that set changes.
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
			                      'infinity'::timestamptz),
			             coalesce(`+briefs.NextSnoozeEndSQL("$2::uuid", "now()")+`, 'infinity'::timestamptz)),
			       `+briefs.LiftedSnoozesDigestSQL("$2::uuid", "now()")+`
			  FROM home_read_fence f`,
			(Life+StaleGrace).Seconds(), actor.UserID, actor.TeamIDs).Scan(&s.fence, &s.at, &s.validUntil, &s.lifted)
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
		// Nothing else deletes an answer that outlived its day, and it holds display text.
		if _, err := tx.Exec(ctx, `DELETE FROM home_read_cache WHERE reader_id = $1 AND valid_until <= now()`, reader); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO home_read_cache
			       (reader_id, read, request_key, fence, computed_at, valid_until, walk_id, payload, lifted_snooze_digest)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (reader_id, read, request_key) DO UPDATE
			   SET fence = EXCLUDED.fence, computed_at = EXCLUDED.computed_at,
			       valid_until = EXCLUDED.valid_until, walk_id = EXCLUDED.walk_id,
			       payload = EXCLUDED.payload, lifted_snooze_digest = EXCLUDED.lifted_snooze_digest
			 WHERE home_read_cache.computed_at <= EXCLUDED.computed_at`,
			reader, string(read), key, stamp.fence, stamp.at, stamp.validUntil, walk, payload, stamp.lifted)
		return err
	})
	if err != nil {
		return fmt.Errorf("homecache: keeping a %s answer: %w", read, err)
	}
	return nil
}

// forgetTimeout bounds a forget, which runs before a write's status is sent.
// It is one upsert, so a writer never waits longer than this for their answer.
const forgetTimeout = 5 * time.Second

// Forget marks every answer these readers were computed before now as behind a
// write, including one a read already in flight stores afterwards. The next
// worklist read serves it flagged stale and reads it again behind; a receipt is
// read fresh. It follows a committed write, so a caller who has left must not
// cancel it and leave their Home unmarked.
func (c *Cache) Forget(ctx context.Context, readers []ids.UUID) error {
	return c.end(ctx, readers, false)
}

// end marks the readers' answers behind a write and, when drop is set, deletes
// them too, so the next read assembles a fresh day.
func (c *Cache) end(ctx context.Context, readers []ids.UUID, drop bool) error {
	if len(readers) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forgetTimeout)
	defer cancel()
	err := database.WithWorkspaceTx(ctx, c.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO home_read_forgotten (reader_id, at)
			SELECT r, clock_timestamp() FROM unnest($1::uuid[]) r
			ON CONFLICT (reader_id) DO UPDATE SET at = greatest(home_read_forgotten.at, EXCLUDED.at)`, readers); err != nil {
			return err
		}
		if !drop {
			return nil
		}
		_, err := tx.Exec(ctx, `DELETE FROM home_read_cache WHERE reader_id = ANY($1::uuid[])`, readers)
		return err
	})
	if err != nil {
		return fmt.Errorf("homecache: ending a reader's answers: %w", err)
	}
	return nil
}
