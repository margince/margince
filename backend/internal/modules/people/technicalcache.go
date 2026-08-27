// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The public lookups the cache holds, one row per (query, kind).
const (
	CacheKindMX      = "mx"
	CacheKindTXT     = "txt"
	CacheKindDMARC   = "dmarc"
	CacheKindDKIM    = "dkim"
	CacheKindAddress = "address"
	CacheKindCNAME   = "cname"
	CacheKindReverse = "reverse"
	CacheKindCertLog = "certlog"
)

// CachedLookup is one remembered answer.
type CachedLookup struct {
	// Answer is what the source said, already classified. Nothing raw is kept
	// — see RememberTechnical.
	Answer []string
	// Found distinguishes "the source answered and there is nothing" from "we
	// have never asked". Both read as an empty Answer, and only one of them is
	// worth re-asking soon.
	Found       bool
	RetrievedAt time.Time
}

// LookupTTL is how long each kind of answer is trusted.
//
// They differ because the underlying facts move at different speeds. A
// certificate log is append-only and a new subdomain shows up within a day,
// while a company's MX record changes once every few years — but the whole
// point of this feature is noticing when it does, so nothing is cached longer
// than a week. THAT is the constraint: a TTL longer than the refresh cadence
// would make the scheduled pass unable to observe the move it exists to catch.
var LookupTTL = map[string]time.Duration{
	CacheKindMX:      24 * time.Hour,
	CacheKindTXT:     24 * time.Hour,
	CacheKindDMARC:   24 * time.Hour,
	CacheKindDKIM:    7 * 24 * time.Hour,
	CacheKindAddress: 12 * time.Hour,
	CacheKindCNAME:   12 * time.Hour,
	CacheKindReverse: 7 * 24 * time.Hour,
	CacheKindCertLog: 24 * time.Hour,
}

// LookupTechnical reads a remembered answer, and reports false when there is
// none or when the one on file has expired.
//
// The cache is installation-global rather than per workspace, exactly like the
// place cache and for the same reason: a domain's DNS records are the same for
// every tenant that holds that domain, and asking a shared public service once
// per tenant for the same answer is the behaviour that gets an installation
// blocked. It is MANDATORY for that reason, not an optimisation.
//
// An expired row is not deleted here: the next Remember overwrites it, and a
// read that deleted rows would need a write transaction for a cache miss.
func (s *Store) LookupTechnical(ctx context.Context, query, kind string) (CachedLookup, bool, error) {
	key := normalizeLookupQuery(query)
	if key == "" {
		return CachedLookup{}, false, nil
	}
	var (
		raw     []byte
		found   bool
		fetched time.Time
	)
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT answer, found, fetched_at
			  FROM technical_lookup_cache
			 WHERE query = $1 AND record_kind = $2 AND expires_at > now()`,
			key, kind).Scan(&raw, &found, &fetched)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return CachedLookup{}, false, nil
	}
	if err != nil {
		return CachedLookup{}, false, fmt.Errorf("reading the technical lookup cache: %w", err)
	}
	var answer []string
	if err := json.Unmarshal(raw, &answer); err != nil {
		return CachedLookup{}, false, fmt.Errorf("reading a cached technical lookup: %w", err)
	}
	return CachedLookup{Answer: answer, Found: found, RetrievedAt: fetched}, true, nil
}

// RememberTechnical records what a source answered.
//
// THE ANSWER MUST ALREADY BE CLASSIFIED. A raw certificate hostname can carry a
// person's name, so the allowlist runs before this call, never after it: a
// cache holding raw names would put personal data in a table the erasure and
// subject-access paths do not reach, which is exactly what the company-level
// guarantee promises does not happen.
//
// A repeat write REFRESHES rather than being ignored — the opposite of the
// geocode cache, which keeps its first answer because a place does not move.
// These answers do move, and a cache that could not be refreshed would freeze
// the record at whatever the first pass saw.
func (s *Store) RememberTechnical(ctx context.Context, query, kind string, answer CachedLookup) error {
	key := normalizeLookupQuery(query)
	if key == "" {
		return nil
	}
	ttl, known := LookupTTL[kind]
	if !known {
		return fmt.Errorf("technical lookup cache: %q is not a lookup kind this cache holds", kind)
	}
	values := answer.Answer
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("recording a technical lookup: %w", err)
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO technical_lookup_cache (query, record_kind, answer, found, fetched_at, expires_at)
			VALUES ($1, $2, $3, $4, now(), now() + $5::interval)
			ON CONFLICT (query, record_kind)
			DO UPDATE SET answer = EXCLUDED.answer, found = EXCLUDED.found,
			              fetched_at = EXCLUDED.fetched_at, expires_at = EXCLUDED.expires_at`,
			key, kind, encoded, answer.Found, ttl.String())
		if err != nil {
			return fmt.Errorf("recording a technical lookup: %w", err)
		}
		return nil
	})
}

// normalizeLookupQuery folds the spellings of one name onto one key, so a
// domain asked about with different casing or a trailing dot is one cache row
// rather than three.
func normalizeLookupQuery(query string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(query)), ".")
}
