// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whether ONE seat has PROVEN ONE address is theirs.
//
// The question exists for cross-seat message binding: an import states the
// addresses a message was on, and the arriving mailbox may join that row only
// if it can prove one of those addresses is its own. The stated address is
// forgeable and is used only as a lookup key; this file is the unforgeable half
// of the answer, which is why it is deliberately narrower than "which addresses
// does this seat have".

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// providerAttestedLabels are the transports whose account_label the PROVIDER
// supplied, read out of an OAuth-sealed credential bundle that the connecting
// caller never authored.
//
// An allowlist rather than a denylist of imap: a transport added later is
// unproven until somebody looks at its AccountLabel and adds it here, which is
// the direction this must fail in.
const (
	providerGmail    = "gmail"
	providerGcal     = "gcal"
	providerGraph    = "graph"
	providerGraphcal = "graphcal"
)

var providerAttestedLabels = []string{providerGmail, providerGcal, providerGraph, providerGraphcal}

// SeatProvedAddressTx reports whether one seat has proven one address is a
// mailbox of theirs, on evidence a third party supplied.
//
// ONE source counts: capture_connection.account_label, and only for a transport
// whose label the PROVIDER attested rather than the caller typed.
//
// That distinction is the whole security of this function, and it is not
// uniform across connectors — it was checked implementation by implementation:
//
//   - googleconn.AccountLabel, graph and graphcal all return `st.Owner`, read
//     out of the OAuth-sealed credential bundle. The caller never supplies it.
//   - imap.AccountLabel returns `creds.Email`, which compose/connectors_imap.go
//     fills VERBATIM from the connect request body. dialLogin proves only that
//     some server at a host the caller also chose accepted that login, so a seat
//     with their own IMAP server can mint any label they like.
//
// Admitting IMAP here would let a seat type a colleague's address, bind to that
// colleague's imported message, and have the take-over rewrite its subject, body
// and captured_by. Not hypothetical: that is
// TestATypedAccountLabelCannotTakeOverAColleaguesImport, which fails against the
// version of this file that admitted every provider.
//
// Deliberately NOT evidence, each because a seat can produce it unaided:
//
//   - source='user'. A seat declaring an address about themselves. Bounded —
//     seatItself(ctx) stops them declaring for anybody else — but bounded is not
//     verified, and this answer decides whose mail joins whose.
//   - source='provider'. Nothing writes it (owneridentitystore.go says why), so
//     admitting it would be dead code pretending to be a rule.
//   - source='delivered_to'. The alias ladder reads the receiving server's own
//     Delivered-To, which is sound for a real provider and forgeable for a seat
//     serving their own bytes over IMAP — and capture_owner_identity does not
//     record WHICH transport produced the sighting, so the two cannot be told
//     apart here. Excluded until that provenance exists.
//   - Any DOMAIN claim. A seat declares a domain with no proof of control, so a
//     domain arm would let one seat attribute every message under a company
//     domain to themselves. Exact addresses only.
//
// The cost is stated plainly because it is real: a seat on IMAP never
// attributes, and neither does a forwarding alias. They keep today's two rows.
// Narrow and correct beats wide and forgeable, and widening later wants a
// label_attested_by column rather than an argument.
//
// Held by: TestATypedAccountLabelCannotTakeOverAColleaguesImport,
// TestADeclaredAddressAloneDoesNotAttribute and TestADomainClaimDoesNotAttribute
// (backend/internal/compose/importthencapture_integration_test.go)
func SeatProvedAddressTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, address string) (bool, error) {
	folded := foldAddress(bareAddress(address))
	if folded == "" || seat == ids.Nil {
		return false, nil
	}
	proved, err := provedAddressesTx(ctx, tx)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(proved, func(p ProvedAddress) bool {
		return p.Seat == seat && p.Address == folded
	}), nil
}

// SeatsProvingAddressTx counts the seats that have proven one address.
//
// It exists for the ambiguity rule: capture_owner_identity is UNIQUE per
// (user_id, kind, value), so two seats CAN hold one address — a shared or
// handed-over mailbox is a real state, not a theoretical one. An address more
// than one seat has proven attributes to NOBODY, because attributing it to
// either would be a coin toss over whose mail joins whose.
//
// Held by: TestAnAddressTwoSeatsProvedAttributesNobody
func SeatsProvingAddressTx(ctx context.Context, tx pgx.Tx, address string) (int, error) {
	folded := foldAddress(bareAddress(address))
	if folded == "" {
		return 0, nil
	}
	proved, err := provedAddressesTx(ctx, tx)
	if err != nil {
		return 0, err
	}
	return len(seatsProving(proved)[folded]), nil
}

// ProvedUnambiguouslyTx is the answer the binding rule actually asks for: this
// seat proved the address, and no other seat did.
//
// One function rather than two calls at the call site, because the two
// questions are one rule, "may this address speak for this seat", and a
// caller that asked only the first would attribute a shared mailbox to whoever
// synced first.
func ProvedUnambiguouslyTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, address string) (bool, error) {
	folded := foldAddress(bareAddress(address))
	if folded == "" || seat == ids.Nil {
		return false, nil
	}
	proved, err := provedAddressesTx(ctx, tx)
	if err != nil {
		return false, err
	}
	seats := seatsProving(proved)[folded]
	_, mine := seats[seat]
	return mine && len(seats) == 1, nil
}

// ProvedAddress is one address one seat has proven: folded, and as the
// provider's label spelled it once the display name is stripped.
type ProvedAddress struct {
	Seat    ids.UUID
	Address string
	Label   string
}

// Spellings are the forms a stored counterparty_email of this address can take:
// capture folds a header's address case-only (correspondence.Fold), and
// foldAddress also normalises the domain.
func (p ProvedAddress) Spellings() []string {
	label := correspondence.Fold(p.Label)
	if label == p.Address {
		return []string{p.Address}
	}
	return []string{p.Address, label}
}

// UnambiguouslyProvedAddressesTx is ProvedUnambiguouslyTx asked of every
// address at once: each address one seat alone has proven, with that seat.
func UnambiguouslyProvedAddressesTx(ctx context.Context, tx pgx.Tx) ([]ProvedAddress, error) {
	proved, err := provedAddressesTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	seats := seatsProving(proved)
	return slices.DeleteFunc(proved, func(p ProvedAddress) bool {
		return len(seats[p.Address]) != 1
	}), nil
}

// provedAddressesTx reads every address a seat has proven, from the connection
// labels SeatProvedAddressTx admits.
//
// The label is stored in whatever shape the provider gave it, `Rep <rep@co>`
// included, so it is folded in Go rather than compared in SQL, the way
// ownerIdentitiesTx already folds it. A display-form label compared as a whole
// address matches no header.
func provedAddressesTx(ctx context.Context, tx pgx.Tx) ([]ProvedAddress, error) {
	rows, err := tx.Query(ctx, `
		SELECT user_id, account_label FROM capture_connection
		 WHERE coalesce(account_label, '') <> '' AND archived_at IS NULL
		   AND status = 'connected' AND provider = ANY($1)`,
		providerAttestedLabels)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the addresses seats proved: %w", err)
	}
	defer rows.Close()
	var out []ProvedAddress
	for rows.Next() {
		var seat ids.UUID
		var label string
		if err := rows.Scan(&seat, &label); err != nil {
			return nil, fmt.Errorf("capture: reading the addresses seats proved: %w", err)
		}
		bare := bareAddress(label)
		if folded := foldAddress(bare); folded != "" {
			out = append(out, ProvedAddress{Seat: seat, Address: folded, Label: bare})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: reading the addresses seats proved: %w", err)
	}
	return out, nil
}

// seatsProving groups proven addresses by address, naming each seat once.
func seatsProving(proved []ProvedAddress) map[string]map[ids.UUID]struct{} {
	out := map[string]map[ids.UUID]struct{}{}
	for _, p := range proved {
		if out[p.Address] == nil {
			out[p.Address] = map[ids.UUID]struct{}{}
		}
		out[p.Address][p.Seat] = struct{}{}
	}
	return out
}
