// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// domainEmploymentCandidateSQL is whether a contact is reachable at a folded
// domain: live, unmerged, with a live address there. The planter and the
// confirm count both ask it.
func domainEmploymentCandidateSQL(contact, domain string) string {
	// An address somebody no longer uses must not attach them to a new
	// employer: a former colleague would be re-hired by a domain they left.
	return contact + `.archived_at IS NULL AND ` + contact + `.merged_into_id IS NULL
 AND EXISTS (SELECT 1 FROM contact_email pe WHERE pe.contact_id = ` + contact + `.id
  AND pe.archived_at IS NULL AND ` + owndomain.CoversSQL("pe.email_domain_folded", domain) + `)`
}

// countLinkedContactsTx counts, per domain, the contacts the reader may see
// that plantDomainEmployment would employ on a confirm. It asks the planter's
// questions: the same candidates, less seats and current primary employments.
func countLinkedContactsTx(ctx context.Context, tx pgx.Tx, domains []string, out map[string]DomainQuestionContext) error {
	var args []any
	bind := func(value any) int { args = append(args, value); return len(args) }
	folded := make([]string, len(domains))
	for i, domain := range domains {
		folded[i] = owndomain.Fold(domain)
	}
	domainsArg := fmt.Sprintf("$%d::text[]", bind(domains))
	foldedArg := fmt.Sprintf("$%d::text[]", bind(folded))
	visible, err := contactVisibleSQL(ctx, bind)
	if err != nil {
		return err
	}
	query := `SELECT q.domain, c.id, lower(e.email)
   FROM unnest(` + domainsArg + `, ` + foldedArg + `) AS q(domain, folded)
   JOIN contact c ON ` + domainEmploymentCandidateSQL("c", "q.folded") + `
   JOIN contact_email e ON e.contact_id = c.id AND e.archived_at IS NULL
  WHERE ` + visible + `
    AND NOT EXISTS (SELECT 1 FROM relationship r WHERE r.contact_id = c.id AND ` + employment.CurrentPrimarySlotSQL("r") + `)`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("contacts: counting the contacts a confirm would employ: %w", err)
	}
	addresses, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (linkedAddress, error) {
		var one linkedAddress
		return one, row.Scan(&one.domain, &one.contact, &one.address)
	})
	if err != nil {
		return fmt.Errorf("contacts: counting the contacts a confirm would employ: %w", err)
	}
	seats, err := storekit.WorkspaceSeatAddresses(ctx, tx)
	if err != nil {
		return err
	}
	for domain, count := range linkedPerDomain(addresses, seats) {
		one := out[domain]
		one.LinkedContactCount = count
		out[domain] = one
	}
	return nil
}

type linkedAddress struct {
	domain, address string
	contact         ids.ContactID
}

// linkedPerDomain drops every contact holding a seat's address, as the
// planter's seat check does, and counts the rest once per domain.
func linkedPerDomain(addresses []linkedAddress, seats storekit.SeatAddresses) map[string]int {
	seat := map[ids.ContactID]bool{}
	for _, one := range addresses {
		if seats.Covers(one.address, ids.Nil) {
			seat[one.contact] = true
		}
	}
	counted := map[string]map[ids.ContactID]bool{}
	for _, one := range addresses {
		if seat[one.contact] {
			continue
		}
		if counted[one.domain] == nil {
			counted[one.domain] = map[ids.ContactID]bool{}
		}
		counted[one.domain][one.contact] = true
	}
	out := map[string]int{}
	for domain, contacts := range counted {
		out[domain] = len(contacts)
	}
	return out
}
