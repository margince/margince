// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// The dead-address read: which of a handful of addresses last refused a
// delivery. DERIVED, never stored — an address is dead exactly while its
// latest hard bounce is newer than the latest clean delivery to it, so a
// later send that arrives clears the mark with no writer, no cascade, and
// nothing for erasure to reach. The founder's rule for model-derived writes
// does not even come into play: nothing is written.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// deadAddressesSQL folds every send that touched one of the asked addresses
// into two instants per address: the newest hard bounce ATTRIBUTED to it, and
// the newest clean delivery. Attribution prefers the recorded
// bounce_recipient; a row stamped before that column existed falls back to
// blaming its recipients only when it has exactly one distinct address — a
// multi-recipient row without the record must not mark the bystanders.
const deadAddressesSQL = `
SELECT went.addr,
       max(o.bounced_at) FILTER (
         WHERE o.bounce_kind = 'hard' AND (
           o.bounce_recipient = went.addr
           OR (o.bounce_recipient IS NULL AND o.bounced_at IS NOT NULL AND (
             SELECT count(DISTINCT lower(one.addr)) FROM jsonb_array_elements_text(
               o.recipients || coalesce(o.cc, '[]'::jsonb) || coalesce(o.bcc, '[]'::jsonb)
             ) AS one(addr)) = 1)
         )
       ) AS last_hard,
       max(o.sent_at) FILTER (
         WHERE o.status = 'sent' AND o.bounced_at IS NULL
       ) AS last_clean
  FROM comms_outbound o,
       LATERAL (
         SELECT DISTINCT lower(each.addr) AS addr FROM jsonb_array_elements_text(
           o.recipients || coalesce(o.cc, '[]'::jsonb) || coalesce(o.bcc, '[]'::jsonb)
         ) AS each(addr)
       ) went
 WHERE went.addr = ANY($1)
 GROUP BY went.addr`

// DeadAddressesTx answers, for each of the given addresses, when it last
// refused a delivery — present only while no clean delivery has landed since.
// Addresses are matched lowercased, as the rows store them. Gated by the
// activity read grant: a delivery outcome is timeline content, and the person
// section this feeds is withheld on the same grant. It borrows the caller's
// transaction because its one caller (the person page) reads every section
// under a single snapshot.
func (s *Store) DeadAddressesTx(ctx context.Context, tx pgx.Tx, addresses []string) (map[string]time.Time, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return map[string]time.Time{}, nil
	}
	asked := make([]string, 0, len(addresses))
	for _, address := range addresses {
		asked = append(asked, strings.ToLower(address))
	}
	dead := map[string]time.Time{}
	rows, err := tx.Query(ctx, deadAddressesSQL, asked)
	if err != nil {
		return nil, fmt.Errorf("comms: deriving dead addresses: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var address string
		var lastHard, lastClean *time.Time
		if scanErr := rows.Scan(&address, &lastHard, &lastClean); scanErr != nil {
			return nil, fmt.Errorf("comms: scanning a dead-address row: %w", scanErr)
		}
		if lastHard == nil {
			continue
		}
		if lastClean != nil && lastClean.After(*lastHard) {
			continue
		}
		dead[address] = *lastHard
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("comms: deriving dead addresses: %w", err)
	}
	return dead, nil
}
