// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What an inbound mail's envelope says about a stranger's mail, judged once
// at capture and stored on the activity. The waiting queue reads the three
// facts to keep a stranger's Bcc or foreign-link mail out of its top band.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// envelopeFacts are the activity's bcc_only, foreign_link and
// sender_was_stranger columns. Nil is NULL: not judged, which the queue reads
// as no evidence.
type envelopeFacts struct {
	bccOnly, foreignLink, senderWasStranger *bool
}

// judgeEnvelopeTx judges one inbound mail. Anything else, and list mail,
// gets no facts at all.
func judgeEnvelopeTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields,
) (envelopeFacts, error) {
	envelope, sender := fields.Envelope, rec.Counterparty.Email
	if envelope == nil || fields.Kind != kindEmail || fields.Direction != connector.DirectionInbound || sender == "" {
		return envelopeFacts{}, nil
	}
	own, err := ownDomainsTx(ctx, tx)
	if err != nil {
		return envelopeFacts{}, err
	}
	corresponded, err := correspondencePositiveTx(ctx, tx, sender)
	if err != nil {
		return envelopeFacts{}, err
	}
	foreign, err := foreignLinkTx(ctx, tx, sender, envelope.LinkHosts, own)
	if err != nil {
		return envelopeFacts{}, err
	}
	stranger := !corresponded
	facts := envelopeFacts{foreignLink: &foreign, senderWasStranger: &stranger}
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return envelopeFacts{}, err
	}
	// With no seat behind the capture, no address of ours is known. The Bcc
	// question then stays unanswered rather than answered yes.
	if !self.Empty() {
		bcc := !addressedOpenly(envelope.OpenRecipients, rec.DeliveredTo, self, own)
		facts.bccOnly = &bcc
	}
	return facts, nil
}

// addressedOpenly reports whether To or Cc names the mailbox. That is a seat
// address, the address the mail was delivered to, or any address on our own
// domains. The last covers a team address such as a sales inbox.
func addressedOpenly(recipients []string, deliveredTo string, self SelfSet, own InternalDomains) bool {
	delivered := foldAddress(deliveredTo)
	for _, recipient := range recipients {
		if self.Covers(recipient) || own.Covers(recipient) || (delivered != "" && foldAddress(recipient) == delivered) {
			return true
		}
	}
	return false
}

// foreignLinkTx reports whether any link leads to a site unrelated to the
// sender. Related means the sender's own registrable domain, one of our own
// domains, or a domain of a company that also holds the sender's domain.
func foreignLinkTx(
	ctx context.Context, tx pgx.Tx, sender string, hosts []string, own InternalDomains,
) (bool, error) {
	candidates := unrelatedSites(sender, hosts, own)
	if len(candidates) == 0 {
		return false, nil
	}
	senderDomain := domainOfAddress(sender)
	siblings, err := companySiblingSitesTx(ctx, tx, senderDomain, freemail.Registrable(senderDomain))
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

// unrelatedSites returns the registrable domains of the link hosts outside
// the sender's own site and outside our own domains.
func unrelatedSites(sender string, hosts []string, own InternalDomains) []string {
	senderSite := freemail.Registrable(domainOfAddress(sender))
	var sites []string
	for _, host := range hosts {
		site := freemail.Registrable(host)
		if site == "" || site == senderSite || own.CoversDomain(host) {
			continue
		}
		sites = append(sites, site)
	}
	return sites
}

// companySiblingSitesTx returns the registrable domains of the companies that
// hold the sender's domain. A link to such a company's other site is the
// sender's own. It reads domains only, never who the company is.
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
		sites[freemail.Registrable(strings.TrimPrefix(domain, "www."))] = true
	}
	return sites, rows.Err()
}
