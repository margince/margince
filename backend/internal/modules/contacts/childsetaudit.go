// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A contact's emails and phones and a company's domains live in tables of
// their own, so Put back judges them by the audit trail. The latest later entry
// that names the key holds the current value.
//
// A writer that leaves the key out is invisible to that judgement, and
// replaying an earlier entry would drop its rows. TestEveryWriterOfAChildSetAuditsItsKey
// fails a writer that never reaches one of the three doors below.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	auditKeyEmails = "emails"
	auditKeyPhones = "phones"
)

// recordEmails puts a contact's address set into an audit image pair.
//
//gate:child-set-audit-door
func recordEmails(before, after map[string]any, was, now *[]crmcontracts.ContactEmail) {
	before[auditKeyEmails], after[auditKeyEmails] = was, now
}

// recordPhones puts a contact's number set into an audit image pair.
//
//gate:child-set-audit-door
func recordPhones(before, after map[string]any, was, now *[]crmcontracts.ContactPhone) {
	before[auditKeyPhones], after[auditKeyPhones] = was, now
}

// recordDomains puts a company's domain set into an audit image pair, in the
// rows readLiveDomains and domainSummaries produce.
//
//gate:child-set-audit-door
func recordDomains(before, after map[string]any, was, now []map[string]any) {
	before[auditKeyDomains], after[auditKeyDomains] = was, now
}

// contactSetsSeen is a contact's emails and phones as a writer found them.
type contactSetsSeen struct{ was crmcontracts.Contact }

// seeContactSets reads the sets a writer is about to change.
func seeContactSets(ctx context.Context, tx pgx.Tx, id ids.ContactID) (contactSetsSeen, error) {
	was, err := readContactRow(ctx, tx, id, storekit.IncludeArchived, nil)
	if err != nil {
		return contactSetsSeen{}, fmt.Errorf("read the contact's emails and phones: %w", err)
	}
	return contactSetsSeen{was: was}, nil
}

// recordContactSetChange adds to the images each set the writer changed, and only those.
func (s contactSetsSeen) recordContactSetChange(ctx context.Context, tx pgx.Tx, id ids.ContactID, before, after map[string]any) error {
	now, err := readContactRow(ctx, tx, id, storekit.IncludeArchived, nil)
	if err != nil {
		return fmt.Errorf("read the contact's emails and phones: %w", err)
	}
	_, differs := storekit.ChangedColumns(contactSetsImage(s.was), contactSetsImage(now))
	if _, ok := differs[auditKeyEmails]; ok {
		recordEmails(before, after, s.was.Emails, now.Emails)
	}
	if _, ok := differs[auditKeyPhones]; ok {
		recordPhones(before, after, s.was.Phones, now.Phones)
	}
	return nil
}

// companyDomainsSeen is a company's live domains as a writer found them.
type companyDomainsSeen struct{ was []map[string]any }

// seeCompanyDomains reads the domains a writer is about to change.
func seeCompanyDomains(ctx context.Context, tx pgx.Tx, id ids.CompanyID) (companyDomainsSeen, error) {
	_, rows, _, err := readLiveDomains(ctx, tx, id)
	return companyDomainsSeen{was: rows}, err
}

// recordCompanyDomainChange adds the domain set to the images when the writer changed it.
func (s companyDomainsSeen) recordCompanyDomainChange(ctx context.Context, tx pgx.Tx, id ids.CompanyID, before, after map[string]any) error {
	_, now, _, err := readLiveDomains(ctx, tx, id)
	if err != nil {
		return err
	}
	if sameDomainRows(s.was, now) {
		return nil
	}
	recordDomains(before, after, s.was, now)
	return nil
}

// sameDomainRows compares two reads of one company's domains, which come back
// in no promised order.
func sameDomainRows(a, b []map[string]any) bool {
	key := func(row map[string]any) string {
		return fmt.Sprintf("%v|%v", row["domain"], row["is_primary"])
	}
	keys := func(rows []map[string]any) []string {
		out := make([]string, len(rows))
		for i, row := range rows {
			out[i] = key(row)
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(keys(a), keys(b))
}

// mergedDomainsSeen is the domains of both companies of a merge before it
// moves them.
type mergedDomainsSeen struct{ source, target companyDomainsSeen }

func seeMergedDomains(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) (mergedDomainsSeen, error) {
	source, err := seeCompanyDomains(ctx, tx, sourceID)
	if err != nil {
		return mergedDomainsSeen{}, err
	}
	target, err := seeCompanyDomains(ctx, tx, targetID)
	return mergedDomainsSeen{source: source, target: target}, err
}

// mergedContactSetsSeen is the emails and phones of both contacts of a merge
// before it moves them.
type mergedContactSetsSeen struct{ source, target contactSetsSeen }

func seeMergedContactSets(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) (mergedContactSetsSeen, error) {
	source, err := seeContactSets(ctx, tx, sourceID)
	if err != nil {
		return mergedContactSetsSeen{}, err
	}
	target, err := seeContactSets(ctx, tx, targetID)
	return mergedContactSetsSeen{source: source, target: target}, err
}
