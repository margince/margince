// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"

	"github.com/margince/margince/backend/internal/platform/owndomain"
)

// FoldedDomain is a column its writers derive from an address or a domain on
// the same row. Postgres cannot fold IDNA, so SQL compares the stored fold.
type FoldedDomain struct {
	Table, Source, Folded string
	Fold                  func(string) *string
}

// foldedDomains lists every such column. A Patch that writes a source stamps
// its fold beside it, so no edit path can leave the two apart.
//
// Held by: TestEveryFoldedColumnIsStampedByAPatch (backend/gates/owndomainpredicate_test.go)
// The columns the folds read and write, named once.
const (
	foldedFromEmail = "email"
	emailFolded     = "email_domain_folded"
)

var foldedDomains = []FoldedDomain{
	{Table: "activity_participant", Source: "address", Folded: "address_domain_folded", Fold: owndomain.StoredOfAddress},
	{Table: "app_user", Source: foldedFromEmail, Folded: emailFolded, Fold: owndomain.StoredOfAddress},
	{Table: "company_domain", Source: "domain", Folded: "domain_folded", Fold: owndomain.StoredOwn},
	{Table: "contact_email", Source: foldedFromEmail, Folded: emailFolded, Fold: owndomain.StoredOfAddress},
	{Table: "lead", Source: foldedFromEmail, Folded: emailFolded, Fold: owndomain.StoredOfAddress},
}

// FoldedDomains returns every folded-domain column.
func FoldedDomains() []FoldedDomain { return append([]FoldedDomain(nil), foldedDomains...) }

// FoldedDomainOf is the folded-domain column of table, and false when it has none.
func FoldedDomainOf(table string) (FoldedDomain, bool) {
	for _, f := range foldedDomains {
		if f.Table == table {
			return f, true
		}
	}
	return FoldedDomain{}, false
}

// stampFoldedDomain stages the fold of a source this patch writes on table.
// A cleared source clears its fold.
func (p *Patch) stampFoldedDomain(table string) error {
	f, ok := FoldedDomainOf(table)
	if !ok {
		return nil
	}
	written, ok := p.after[f.Source]
	if !ok {
		return nil
	}
	var source string
	switch v := written.(type) {
	case nil:
	case string:
		source = v
	case *string:
		if v != nil {
			source = *v
		}
	default:
		return fmt.Errorf("storekit: %s.%s is written as a %T, so its fold cannot be derived", table, f.Source, written)
	}
	var fold *string
	if source != "" {
		fold = f.Fold(source)
	}
	p.SetStamp(f.Folded, fold)
	return nil
}
