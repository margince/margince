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
// caller's transaction. Compose binds capture's ColleagueDomainsTx here and
// behind OwnDomains.Domains. A compose integration test holds the two to one list:
// TestTheBadgeAndTheWorklistReadOneOwnDomainListThroughTheProductionWiring.
type ColleagueDomainsReader func(ctx context.Context, tx pgx.Tx) ([]string, error)

// boundColleagueDomains is the reader compose bound at startup, nil for none.
var boundColleagueDomains atomic.Pointer[ColleagueDomainsReader]

// UseColleagueDomains binds the own-domain reader for the owed reads that have
// no store, such as the History mail badge. Compose calls it once; nil unbinds.
// Unbound, the list is empty, the open default an unwired store has.
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
