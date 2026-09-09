// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The sweep that takes a stored part's octets back out of the provider
// original.
//
// PROOF BEFORE REMOVAL is the whole design. capture writes the original before
// the attachment rows exist and before the object store has been spoken to, so
// at capture time there is nothing to prove; by the time this runs there is.
// Every part this removes has an attachment row, a storage key, an object at
// that key of the length the row declares, and octets that hash to the digest
// the row recorded — and then partslim locates those octets' encoding in the
// message and removes only the bytes it found. A part failing any of those
// keeps its bytes.
//
// That is what makes the sweep safe on a deployment with no object store
// wired, and safe for a part dropped for size: both have nothing to prove, so
// nothing is removed.
//
// This is a REWRITE of append-once evidence, and the second sanctioned one
// beside retention's erase. sinkraw.go names both.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/partslim"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PartSlimBatch bounds one pass.
//
// Each row costs one object read per part plus an encode of those octets, so a
// batch is bounded outbound work and belongs on a queue of its own. Fifty
// rather than a larger number because the objects are megabytes: a batch holds
// one row's payload and one part's octets at a time, and the ceiling that
// matters is how long the pass runs, not how much it holds.
const PartSlimBatch = 50

// partSlimObjectTimeout bounds one object read. Generous against a healthy
// store carrying the tens of megabytes a message may hold, short enough that a
// sick one cannot hold the pass — the backlog is re-read next cadence anyway.
const partSlimObjectTimeout = 2 * time.Minute

// PartSlimStore slims raw_capture rows in batches.
type PartSlimStore struct {
	db   *database.DB
	blob blobstore.Store
}

// NewPartSlimStore binds the sweep to its database and object store.
//
// A nil store slims nothing and says so by stamping the rows it looked at: with
// nowhere to read the bytes from there is no proof to have, and removing a copy
// on the strength of a row that merely CLAIMS an object would destroy the only
// one there is.
func NewPartSlimStore(db *database.DB, blob blobstore.Store) *PartSlimStore {
	return &PartSlimStore{db: db, blob: blob}
}

// CandidatePart is one attachment row that might name bytes this sweep can
// remove from the original.
type CandidatePart struct {
	Ordinal    int
	StorageKey string
	ByteSize   int64
	Checksum   string
}

// UnmarshalJSON reads the shape the candidates query aggregates. Spelled out
// rather than tagged so the SQL's key names and this struct cannot drift
// without the compiler noticing the field it no longer fills.
func (c *CandidatePart) UnmarshalJSON(data []byte) error {
	var wire struct {
		Ordinal int    `json:"ordinal"`
		Key     string `json:"key"`
		Bytes   int64  `json:"bytes"`
		Sum     string `json:"sum"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("capture: reading a candidate part: %w", err)
	}
	c.Ordinal, c.StorageKey, c.ByteSize, c.Checksum = wire.Ordinal, wire.Key, wire.Bytes, wire.Sum
	return nil
}

// PartSlimResult is what one pass did, for the operator watching the backlog.
type PartSlimResult struct {
	Considered int
	Slimmed    int
	Freed      int64
	// Unproved counts parts left in place because the object store could not
	// vouch for them. It is the number that says the store is not answering, as
	// distinct from a message whose encoding simply could not be located.
	Unproved int
}

// errPartSlimRaced marks a row another writer changed under the sweep. Not a
// fault: the row is left unstamped and re-read next cadence.
var errPartSlimRaced = errors.New("capture: the original changed under the slim")

// SlimBatch reads a batch of unconsidered rows, removes what it can prove, and
// stamps every row it considered.
func (s *PartSlimStore) SlimBatch(ctx context.Context, limit int) (PartSlimResult, error) {
	if limit <= 0 {
		limit = PartSlimBatch
	}
	rows, err := s.candidates(ctx, limit)
	if err != nil {
		return PartSlimResult{}, err
	}
	var out PartSlimResult
	for _, row := range rows {
		out.Considered++
		if err := s.slimRow(ctx, row, &out); err != nil {
			return out, err
		}
	}
	return out, nil
}

// candidateRow is one raw_capture row and the attachment rows joined to it.
type candidateRow struct {
	id      ids.UUID
	payload []byte
	parts   []CandidatePart
}

// candidates reads the next unconsidered rows with their attachment rows.
//
// The join is on the pair capture writes: attachment.external_source_id is
// "<source_system>:<source_id>" and external_part_id is sinkparts.partIdentity's
// "part:<n>". Rows with no attachment rows at all are selected too — they still
// have to be stamped, or the sweep reads them again every cadence forever.
func (s *PartSlimStore) candidates(ctx context.Context, limit int) ([]candidateRow, error) {
	var out []candidateRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT rc.id, rc.payload,
			       coalesce(jsonb_agg(jsonb_build_object(
			           'ordinal', split_part(at.external_part_id, ':', 2)::int,
			           'key',     at.storage_key,
			           'bytes',   at.byte_size,
			           'sum',     at.checksum)
			         ORDER BY at.external_part_id)
			         FILTER (WHERE at.id IS NOT NULL), '[]'::jsonb)
			  FROM raw_capture rc
			  LEFT JOIN attachment at
			    ON at.external_source_id = rc.source_system || ':' || rc.source_id
			   AND at.archived_at IS NULL
			   AND at.external_part_id LIKE 'part:%'
			   AND at.storage_key <> ''
			 WHERE rc.parts_slimmed_at IS NULL
			 GROUP BY rc.id, rc.payload
			 ORDER BY rc.received_at
			 LIMIT $1`, limit)
		if err != nil {
			return fmt.Errorf("capture: selecting the originals to slim: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var row candidateRow
			if err := rows.Scan(&row.id, &row.payload, &row.parts); err != nil {
				return fmt.Errorf("capture: reading an original to slim: %w", err)
			}
			out = append(out, row)
		}
		return rows.Err()
	})
	return out, err
}

// slimRow removes one row's provable parts and stamps it.
func (s *PartSlimStore) slimRow(ctx context.Context, row candidateRow, out *PartSlimResult) error {
	proved, unproved := s.proveParts(ctx, row.parts)
	out.Unproved += unproved
	if len(proved) == 0 {
		return s.stamp(ctx, row.id)
	}
	stripped, count, err := partslim.StripStoredParts(row.payload, proved)
	if err != nil {
		// An original this cannot rewrite is stamped rather than retried: the
		// fault is in the message or the rows describing it, and neither
		// changes on its own. Left unstamped it would be re-read, re-fetched
		// and re-refused every cadence for as long as the row exists.
		return s.stamp(ctx, row.id)
	}
	if count == 0 || len(stripped) >= len(row.payload) {
		// Nothing came out, or the stanza cost more than the octets it replaced
		// — which happens on a part small enough, and is why this is measured
		// rather than assumed.
		return s.stamp(ctx, row.id)
	}
	saved := int64(len(row.payload) - len(stripped))
	err = s.write(ctx, row, stripped)
	if errors.Is(err, errPartSlimRaced) {
		return nil
	}
	if err != nil {
		return err
	}
	out.Slimmed++
	out.Freed += saved
	return nil
}

// write replaces one payload, refusing to land stale bytes.
func (s *PartSlimStore) write(ctx context.Context, row candidateRow, stripped []byte) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The payload guard is the concurrency story: InsertRawCaptureTx's
		// ON CONFLICT DO UPDATE can refresh this payload from a redelivery while
		// the sweep holds the old bytes. That redelivery leaves parts_slimmed_at
		// NULL, so the row is re-read next cadence — what must never happen is
		// this write landing the STALE stripped bytes over the fresh ones.
		tag, err := tx.Exec(ctx, `
			UPDATE raw_capture
			   SET payload = $2::jsonb, parts_slimmed_at = now()
			 WHERE id = $1 AND parts_slimmed_at IS NULL AND payload = $3::jsonb`,
			row.id, stripped, row.payload)
		if err != nil {
			return fmt.Errorf("capture: slimming an original: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return errPartSlimRaced
		}
		return nil
	})
}

// stamp records that the sweep considered a row without changing its payload.
func (s *PartSlimStore) stamp(ctx context.Context, id ids.UUID) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE raw_capture SET parts_slimmed_at = now() WHERE id = $1`, id); err != nil {
			return fmt.Errorf("capture: stamping a considered original: %w", err)
		}
		return nil
	})
}

// proveParts reads each candidate's object and keeps the ones it can vouch for,
// reporting how many it could not.
func (s *PartSlimStore) proveParts(ctx context.Context, parts []CandidatePart) ([]partslim.StoredPart, int) {
	if s.blob == nil {
		return nil, len(parts)
	}
	var proved []partslim.StoredPart
	unproved := 0
	for _, part := range parts {
		if part.StorageKey == "" || part.Checksum == "" || part.ByteSize <= 0 {
			unproved++
			continue
		}
		body, err := s.readObject(ctx, part)
		if err != nil {
			unproved++
			continue
		}
		proved = append(proved, partslim.StoredPart{
			Ordinal:    part.Ordinal,
			Sha256:     part.Checksum,
			Bytes:      part.ByteSize,
			StorageKey: part.StorageKey,
			Body:       body,
		})
	}
	return proved, unproved
}

// readObject reads one object and checks it against the row that names it.
//
// The digest is checked by partslim before it removes anything, so what this
// owes is the length: reading one byte past it turns an object that has GROWN
// into a refusal here rather than a digest mismatch two frames later, and keeps
// a store that lies about its lengths from being read unboundedly into memory.
func (s *PartSlimStore) readObject(ctx context.Context, part CandidatePart) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, partSlimObjectTimeout)
	defer cancel()
	reader, object, err := s.blob.Get(ctx, part.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the object behind part:%d: %w", part.Ordinal, err)
	}
	defer reader.Close()
	if object.Size != part.ByteSize {
		return nil, fmt.Errorf("capture: part:%d is %d bytes in the store, %d in its row",
			part.Ordinal, object.Size, part.ByteSize)
	}
	body, err := io.ReadAll(io.LimitReader(reader, part.ByteSize+1))
	if err != nil {
		return nil, fmt.Errorf("capture: reading the object behind part:%d: %w", part.Ordinal, err)
	}
	if int64(len(body)) != part.ByteSize {
		return nil, fmt.Errorf("capture: part:%d read back %d bytes, its row says %d",
			part.Ordinal, len(body), part.ByteSize)
	}
	return body, nil
}
