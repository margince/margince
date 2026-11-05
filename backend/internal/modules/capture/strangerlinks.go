// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whether a mail's links lead away from its sender.

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/owndomain"
)

// foreignLinkTx reports whether any link leads to a site unrelated to the
// sender. Related means the sender's own site, one of our own domains, or a
// domain of a company that also holds the sender's domain. A link list the
// parser cut short counts as foreign, because the unread rest could be.
func foreignLinkTx(ctx context.Context, tx pgx.Tx, sender string, envelope *MailEnvelope) (bool, error) {
	if envelope.LinksTruncated {
		return true, nil
	}
	own, err := ownDomainsTx(ctx, tx)
	if err != nil {
		return false, err
	}
	candidates := unrelatedSites(sender, envelope.LinkHosts, own)
	if len(candidates) == 0 {
		return false, nil
	}
	senderDomain := owndomain.OfAddress(sender)
	siblings, err := companySiblingSitesTx(ctx, tx, senderDomain, siteOf(senderDomain))
	if err != nil {
		return false, err
	}
	for _, site := range candidates {
		if !siblings[site] {
			return true, nil
		}
	}
	return false, nil
}

// unrelatedSites returns the sites of the link hosts outside the sender's own
// site and outside our own domains.
func unrelatedSites(sender string, hosts []string, own InternalDomains) []string {
	senderSite := siteOf(owndomain.OfAddress(sender))
	var sites []string
	for _, host := range hosts {
		site := siteOf(host)
		if site == "" || site == senderSite || own.CoversDomain(host) {
			continue
		}
		sites = append(sites, site)
	}
	return sites
}

// siteOf is what two hosts must share to be one site: the registrable domain,
// or the canonical address for an IP literal. A sender's IP literal arrives
// bracketed, as in user@[192.0.2.1], and a link's arrives bare.
func siteOf(host string) string {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	literal := strings.TrimPrefix(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"), "ipv6:")
	if ip := net.ParseIP(literal); ip != nil {
		return ip.String()
	}
	return freemail.Registrable(host)
}

// companySiblingSitesTx returns the sites of the companies that hold the
// sender's domain. A link to such a company's other site is the sender's own.
// It reads domains only, never who the company is.
func companySiblingSitesTx(ctx context.Context, tx pgx.Tx, senderDomain, senderSite string) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT sibling.domain
		  FROM company_domain held
		  JOIN company_domain sibling ON sibling.company_id = held.company_id AND sibling.archived_at IS NULL
		 WHERE held.archived_at IS NULL AND held.domain = ANY($1::text[])`,
		[]string{senderDomain, senderSite})
	if err != nil {
		return nil, fmt.Errorf("capture: reading the sender's company domains: %w", err)
	}
	defer rows.Close()
	sites := map[string]bool{}
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			return nil, err
		}
		sites[siteOf(domain)] = true
	}
	return sites, rows.Err()
}
