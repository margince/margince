// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SeatAddresses answers whether an address belongs to a live seat, and how
// that is shown. A login, a connected mailbox or a provider-attested alias
// speaks for the whole workspace. A typed or inferred alias speaks for its own
// seat alone, because any seat can type a client's address.
//
// Never return the members to a client: a seat's aliases are private to it.
type SeatAddresses struct {
	proven   map[string]struct{}
	logins   map[string]struct{}
	declared map[string]map[ids.UUID]struct{}
}

// WorkspaceSeatAddresses reads the set from the same records MailSeatIdentities
// folds. A domain-kind identity is not read, so it covers no address. A writer
// that acts on the answer reads it through SeatAddressesFor instead.
func WorkspaceSeatAddresses(ctx context.Context, tx pgx.Tx) (SeatAddresses, error) {
	records, err := readMailSeatIdentities(ctx, tx)
	if err != nil {
		return SeatAddresses{}, err
	}
	set := SeatAddresses{proven: map[string]struct{}{}, logins: map[string]struct{}{}, declared: map[string]map[ids.UUID]struct{}{}}
	for _, record := range records {
		address := mailidentity.SeatAddressKey(record.label, record.source)
		if address == "" || !record.decidesNew() {
			continue
		}
		if record.source == loginSource {
			set.logins[address] = struct{}{}
		}
		if provenSeatSource(record.source) {
			set.proven[address] = struct{}{}
			continue
		}
		seat, err := ids.Parse(record.seat)
		if err != nil {
			return SeatAddresses{}, err
		}
		if set.declared[address] == nil {
			set.declared[address] = map[ids.UUID]struct{}{}
		}
		set.declared[address][seat] = struct{}{}
	}
	return set, nil
}

func provenSeatSource(source string) bool {
	switch source {
	case loginSource, provedMailboxSource, "provider":
		return true
	}
	return false
}

// Covers reports whether address, in any case, is a seat address as seen by
// asking: proven for any seat, or declared by asking itself. ids.Nil asks for
// proof alone, which is the answer for every other seat's records.
func (s SeatAddresses) Covers(address string, asking ids.UUID) bool {
	folded := mailidentity.SeatAddressKey(address, "")
	if _, ok := s.proven[folded]; ok {
		return true
	}
	return s.declaredBy(folded, asking)
}

// IsLogin reports whether address is a live seat's login, which only the
// seat's own account changes.
func (s SeatAddresses) IsLogin(address string) bool {
	_, ok := s.logins[mailidentity.SeatAddressKey(address, "")]
	return ok
}

// For lists the addresses Covers admits for asking, sorted, for binding as
// one SQL array argument.
func (s SeatAddresses) For(asking ids.UUID) []string {
	out := make([]string, 0, len(s.proven))
	for address := range s.proven {
		out = append(out, address)
	}
	for address := range s.declared {
		if _, proven := s.proven[address]; !proven && s.declaredBy(address, asking) {
			out = append(out, address)
		}
	}
	sort.Strings(out)
	return out
}

// HoldsNoSeatAddressSQL is true when the contact holds no live address among
// seats, the addresses For listed. A stored address may keep its Unicode domain,
// so each seat is bound in every spelling.
func HoldsNoSeatAddressSQL(contactID string, seats []string, arg func(any) int) string {
	spellings := make([]string, 0, 2*len(seats))
	for _, seat := range seats {
		spellings = append(spellings, mailidentity.AddressSpellings(seat)...)
	}
	return fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM contact_email seat_address
		WHERE seat_address.contact_id = %s AND seat_address.archived_at IS NULL
		  AND lower(seat_address.email) = ANY($%d))`, contactID, arg(spellings))
}

func (s SeatAddresses) declaredBy(address string, asking ids.UUID) bool {
	_, ok := s.declared[address][asking]
	return ok && !asking.IsZero()
}

// SeatAddressesFor reads the set after taking the shared lock on each address
// the caller will ask about. A claim of one of them that commits first is in
// the answer; a later one waits until this transaction ends.
func SeatAddressesFor(ctx context.Context, tx pgx.Tx, addresses ...string) (SeatAddresses, error) {
	if err := LockSeatAddressChecks(ctx, tx, addresses...); err != nil {
		return SeatAddresses{}, err
	}
	return WorkspaceSeatAddresses(ctx, tx)
}

// LockSeatAddressChecks takes the shared lock a check of these addresses holds.
// A writer of a row that an alias claim also writes takes it before that row.
func LockSeatAddressChecks(ctx context.Context, tx pgx.Tx, addresses ...string) error {
	return lockSeatAddresses(ctx, tx, `pg_advisory_xact_lock_shared`, addresses)
}

// LockSeatAddressClaims is taken by every writer that adds, moves or withdraws
// a seat's claim, over every address the change touches: the old and the new.
// It waits for every check of them in flight. Take it before any row lock, and
// wait on no contact row under it.
func LockSeatAddressClaims(ctx context.Context, tx pgx.Tx, addresses ...string) error {
	return lockSeatAddresses(ctx, tx, `pg_advisory_xact_lock`, addresses)
}

// lockSeatAddresses keys each lock on SeatAddressKey, the fold Covers compares,
// and takes them sorted, so two holders of overlapping sets queue in one order.
func lockSeatAddresses(ctx context.Context, tx pgx.Tx, lock string, addresses []string) error {
	keys := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if folded := mailidentity.SeatAddressKey(address, ""); folded != "" {
			keys = append(keys, folded)
		}
	}
	slices.Sort(keys)
	for _, key := range slices.Compact(keys) {
		if _, err := tx.Exec(ctx, `SELECT `+lock+`(hashtext('margince:own-address:' || $1)::bigint)`, key); err != nil {
			return fmt.Errorf("locking a seat address: %w", err)
		}
	}
	return nil
}
