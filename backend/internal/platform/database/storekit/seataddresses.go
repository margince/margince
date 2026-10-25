// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
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
	declared map[string]map[ids.UUID]struct{}
}

// WorkspaceSeatAddresses reads the set from the same records MailSeatIdentities
// folds. A domain-kind identity is not read, so it covers no address.
func WorkspaceSeatAddresses(ctx context.Context, tx pgx.Tx) (SeatAddresses, error) {
	records, err := readMailSeatIdentities(ctx, tx)
	if err != nil {
		return SeatAddresses{}, err
	}
	set := SeatAddresses{proven: map[string]struct{}{}, declared: map[string]map[ids.UUID]struct{}{}}
	for _, record := range records {
		address := mailidentity.ReaderAddress(record.label, record.source)
		if address == "" {
			continue
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
	case "login", provedMailboxSource, "provider":
		return true
	}
	return false
}

// Covers reports whether address, in any case, is a seat address as seen by
// asking: proven for any seat, or declared by asking itself. ids.Nil asks for
// proof alone, which is the answer for every other seat's records.
func (s SeatAddresses) Covers(address string, asking ids.UUID) bool {
	folded := mailidentity.ReaderAddress(address, "")
	if _, ok := s.proven[folded]; ok {
		return true
	}
	return s.declaredBy(folded, asking)
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

func (s SeatAddresses) declaredBy(address string, asking ids.UUID) bool {
	_, ok := s.declared[address][asking]
	return ok && !asking.IsZero()
}
