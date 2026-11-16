// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

// ColleagueDomainsReader reads the workspace's own email domains in the
// caller's transaction. It is the same read OwnDomains.Domains answers for a
// store: compose binds capture's ColleagueDomainsTx to both.
type ColleagueDomainsReader func(ctx context.Context, tx pgx.Tx) ([]string, error)

// boundColleagueDomains is the reader compose bound at startup, nil for none.
var boundColleagueDomains atomic.Pointer[ColleagueDomainsReader]

// UseColleagueDomains binds the own-domain reader for the reads that have no
// store to ask: the History mail badge (EmailStatesFor, MailMovesFor,
// MailMoveColumnSQL) and the other free functions that judge whether a reply is
// owed. invitationNeverOwedSQL needs the list to tell a colleague's calendar
// invitation from a customer's when the seat stamp is missing.
//
// Compose calls it once; nil unbinds. Unbound, the list is empty and only the
// seat stamp marks a colleague's invitation, the same open default an unwired
// store has.
func UseColleagueDomains(read ColleagueDomainsReader) {
	if read == nil {
		boundColleagueDomains.Store(nil)
		return
	}
	boundColleagueDomains.Store(&read)
}

// colleagueDomainsTx reads the own-domain list through the bound reader, in
// the caller's transaction.
func colleagueDomainsTx(ctx context.Context, tx pgx.Tx) ([]string, error) {
	read := boundColleagueDomains.Load()
	if read == nil {
		return nil, nil
	}
	domains, err := (*read)(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("activities: reading which domains are our own: %w", err)
	}
	return domains, nil
}

// bindColleagueDomains reads the own-domain list and binds it, returning the
// placeholder invitationNeverOwedSQL and owedSQL take.
func bindColleagueDomains(ctx context.Context, tx pgx.Tx, arg func(any) int) (string, error) {
	domains, err := colleagueDomainsTx(ctx, tx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$%d", arg(domains)), nil
}
