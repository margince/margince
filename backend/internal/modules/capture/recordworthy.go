// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whether an address could name a person at all, asked before the ladder mints
// a record for it.
//
// The tier ladder's T1 evidence is that the workspace WROTE to an address. That
// is honest evidence of intent and a poor answer to "is there a person here",
// because the mail a founder sends from their own mailbox includes expense
// reports, itineraries, invoices and password resets. Every one of those is an
// address the workspace demonstrably wrote to, and each one became a contact.
//
// So T1 asks two questions now, and this file holds the second. It is a
// property of the ADDRESS, decided without a model and without a query, which
// is what lets it run in front of the ledger rather than after a verdict.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// recordWorthy reports whether an address could name a person a CRM should
// hold, ignoring everything about the message it arrived on.
//
// It is a method on the Sink because the operator's `transactional_never`
// allowlist has to win here exactly as it wins in Suppress. A deployment that
// genuinely sells to one of these companies declares it, and a refusal that
// ignored the declaration would turn the escape hatch into a suppression.
//
// It refuses exactly two shapes, both of which say "no person is reachable
// here" rather than "this person is uninteresting":
//
//   - a machine local part, wherever it sends from. `noreply@`, `receipts@`,
//     `calendar-notification@`: a reply to one of these reaches nobody, so a
//     record naming it promises a person who does not exist.
//   - registrable mail infrastructure. Mail from a bulk-send relay names the
//     relay, never the company that hired it.
//
// A domain that merely LOOKS automated is not refused here. That is the
// registry's corroborated prefix rule, which yields to correspondence — and
// this does not, so anything it refuses has to be unambiguous on the address
// alone.
//
// This and the T2 registry overlap on the infrastructure domains, and the
// overlap is deliberate rather than a second answer to one question. T2 decides
// what the LEDGER records and what the trace says, and it yields to
// correspondence, because a company can genuinely live behind a sender lane.
// This decides whether a RECORD may be minted, and it does not yield, because
// correspondence with an address nobody answers is not correspondence. On a
// shared domain the practical difference is exactly that one bit: T2 would let
// the address through once the owner replied, and this does not.
//
// The two consult different domain lists on purpose. transactionalBaseline is
// also read by IsMachineAddress, which the attention queue uses to drop rows, so
// a product company with real salespeople must not go in it — a hidden human
// waiting on a reply is not recoverable by that queue's reader.
// personalServiceDomains is the list only this gate and T2 see.
func (s *Sink) recordWorthy(cp connector.Counterparty) bool {
	address := strings.TrimSpace(cp.Email)
	if address == "" {
		return false
	}
	// The local part is asked FIRST, and no allowlist overrules it. An operator
	// vouches for a DOMAIN — that its people are real counterparties — and says
	// nothing about `noreply@` on it, where still nobody answers.
	if machineLocalpart(address) {
		return false
	}
	if s.transactional != nil && s.transactional.Allowlisted(cp.Domain) {
		// The operator declared this domain always-legitimate, which outranks
		// both domain lists below.
		return true
	}
	base := freemail.Registrable(cp.Domain)
	if base == "" {
		// Not a hostname. Nothing that cannot be a domain names a company, and
		// a person may still be reachable at it, so this is not a refusal.
		return true
	}
	if _, infra := transactionalBaseline[base]; infra {
		return false
	}
	// A personal-service product: the owner's own traffic with a tool they use.
	// Kept separate from the infrastructure baseline because the attention queue
	// reads that one and a named human here is a real customer.
	_, service := personalServiceDomains[base]
	return !service
}

// machineLocalpart reports whether the local part of an address names a sending
// system rather than a person, by both vocabularies the registry keeps.
func machineLocalpart(address string) bool {
	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		return false
	}
	local := address[:at]
	return isMachineLocalpart(local) || hasMachineMarker(local)
}

// consumerMailSender answers T3: is this sender a personal mailbox rather than
// somebody at a company? gmail.com is not an organization whatever else is true
// of it, so its domain settles the ORGANIZATION question on its own — no
// company is named by a consumer mail domain.
//
// It settles nothing about the person. A customer writing from their private
// address and a family member writing from theirs are the same shape here, so
// the ladder suppresses the org and leaves the person to the verdict.
//
// The workspace's own additions and carve-outs are read on the CALLER's
// transaction, not cached at composition time: an admin correcting a wrong
// baseline entry means the very next message, and a cache would make them wait
// without saying so.
func consumerMailSender(ctx context.Context, tx pgx.Tx, domain string) (bool, error) {
	consumerMail, err := MatcherTx(ctx, tx)
	if err != nil {
		return false, err
	}
	return consumerMail.IsConsumer(domain), nil
}
