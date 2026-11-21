// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"slices"

	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Batch is the records a batch will create or change, in the state the ladder
// meets them. Each is filed under the keys a pair needs one of, so a row is
// scored against its neighbours only. A company needs a squashed name, word or
// domain in common. A contact needs an address or a registrable domain: the
// fuzzy tier reaches its bar only with an employer agreement, which is a domain.
type Batch struct {
	// NameKey folds a company name the way the caller matches an employer by
	// name. Batch.CompanyNamed reads it.
	NameKey   func(string) string
	companies []heldCompany
	contacts  []heldContact
	// gone marks a record a later row of the batch wrote again.
	goneCompany, goneContact map[int]bool
	byID                     map[ids.UUID]bool
	byKey                    map[string]int
	bySquashed, byToken      map[string][]int
	byDomain                 map[string][]int
	tokensOfLength           map[int][]string
	byEmail, byRegistrable   map[string][]int
}

func (b *Batch) init() {
	if b.byID != nil {
		return
	}
	b.goneCompany, b.goneContact, b.byID, b.byKey = map[int]bool{}, map[int]bool{}, map[ids.UUID]bool{}, map[string]int{}
	b.bySquashed, b.byToken, b.byDomain = map[string][]int{}, map[string][]int{}, map[string][]int{}
	b.tokensOfLength, b.byEmail, b.byRegistrable = map[int][]string{}, map[string][]int{}, map[string][]int{}
}

// AddCompany files a company a row leaves behind. A stored company written
// twice keeps the state its last row leaves.
func (b *Batch) AddCompany(c NewCompany) {
	b.init()
	if c.ID != (ids.UUID{}) {
		for at, earlier := range b.companies {
			b.goneCompany[at] = b.goneCompany[at] || earlier.record.ID == c.ID
		}
		b.byID[c.ID] = true
	}
	b.fileCompany(len(b.companies), heldCompanyOf(c))
}

// fileCompany files a held company under every key a pair needs one of.
func (b *Batch) fileCompany(at int, held heldCompany) {
	c := held.record
	b.companies = append(b.companies, held)
	keys := []string{"ref:" + c.Ref}
	for _, domain := range held.domains {
		keys = append(keys, "ref:"+domain)
	}
	if b.NameKey != nil {
		keys = append(keys, "name:"+b.NameKey(c.DisplayName), "name:"+b.NameKey(c.LegalName))
	}
	for _, key := range keys {
		if earlier, filed := b.byKey[key]; key != "ref:" && key != "name:" && (!filed || b.goneCompany[earlier]) {
			b.byKey[key] = at
		}
	}
	for _, form := range []companyNameForms{held.names.displayForm, held.names.legalForm} {
		if form.squashed != "" {
			b.bySquashed[form.squashed] = append(b.bySquashed[form.squashed], at)
		}
		for _, token := range form.tokens {
			if _, known := b.byToken[token]; !known {
				b.tokensOfLength[len([]rune(token))] = append(b.tokensOfLength[len([]rune(token))], token)
			}
			b.byToken[token] = append(b.byToken[token], at)
		}
	}
	for _, domain := range held.domains {
		b.byDomain[domain] = append(b.byDomain[domain], at)
	}
}

// AddContact files a contact as AddCompany files a company.
func (b *Batch) AddContact(c NewContact) {
	b.init()
	if c.ID != (ids.UUID{}) {
		for at, earlier := range b.contacts {
			b.goneContact[at] = b.goneContact[at] || earlier.record.ID == c.ID
		}
		b.byID[c.ID] = true
	}
	at, held := len(b.contacts), heldContactOf(c)
	b.contacts = append(b.contacts, held)
	for _, stored := range held.stored {
		b.byEmail[stored] = append(b.byEmail[stored], at)
	}
	for _, domain := range append(slices.Clone(held.row.mailDomains), held.employerDomains...) {
		if registrable := freemail.Registrable(normalizeDomain(domain)); registrable != "" {
			b.byRegistrable[registrable] = append(b.byRegistrable[registrable], at)
		}
	}
}

// Changes reports whether a row of the batch changes the stored record id.
func (b *Batch) Changes(id ids.UUID) bool { return b.byID[id] }

// CompanyByRef answers the company a link to ref reaches: a row of the batch
// by its own key, or by its domain.
func (b *Batch) CompanyByRef(ref string) (NewCompany, bool) {
	if at, filed := b.byKey["ref:"+ref]; filed && ref != "" {
		return b.liveCompany(at)
	}
	return b.liveCompanyAt("ref:" + normalizeDomain(ref))
}

// CompanyNamed answers the company of the batch an employer name reaches.
func (b *Batch) CompanyNamed(name string) (NewCompany, bool) {
	if b.NameKey == nil || b.NameKey(name) == "" {
		return NewCompany{}, false
	}
	return b.liveCompanyAt("name:" + b.NameKey(name))
}

func (b *Batch) liveCompanyAt(key string) (NewCompany, bool) {
	at, filed := b.byKey[key]
	if !filed {
		return NewCompany{}, false
	}
	return b.liveCompany(at)
}

func (b *Batch) liveCompany(at int) (NewCompany, bool) {
	if b.goneCompany[at] {
		return NewCompany{}, false
	}
	return b.companies[at].record, true
}

// CompanyLookAlike answers whether the ladder pairs c with a company of the
// batch. inTx runs the candidate query's admission test, and only for a pair
// that scores.
func (b *Batch) CompanyLookAlike(ctx context.Context, inTx InTx, c NewCompany) (bool, error) {
	asked := heldCompanyOf(c)
	var scored []heldCompany
	for _, at := range b.companyNeighbours(asked) {
		if b.goneCompany[at] {
			continue
		}
		spelled, scores := asked.verdict(b.companies[at])
		if spelled {
			return true, nil
		}
		if scores {
			scored = append(scored, b.companies[at])
		}
	}
	return companyAdmitted(ctx, inTx, c, scored)
}

// companyNeighbours are the companies the asked one could pair with: one whose
// domain its name spells, or one passing the name gate's first test. That test
// is a shared squashed name or a word sameCompanyToken matches.
func (b *Batch) companyNeighbours(asked heldCompany) []int {
	var near []int
	if asked.spelled != "" {
		near = append(near, b.byDomain[asked.spelled]...)
	}
	for _, form := range []companyNameForms{asked.names.displayForm, asked.names.legalForm} {
		if form.squashed != "" {
			near = append(near, b.bySquashed[form.squashed]...)
		}
		for _, token := range form.tokens {
			length := len([]rune(token))
			for width := length - companyFuzzyTokenLengthSlack; width <= length+companyFuzzyTokenLengthSlack; width++ {
				for _, filed := range b.tokensOfLength[width] {
					if sameCompanyToken(token, filed) {
						near = append(near, b.byToken[filed]...)
					}
				}
			}
		}
	}
	slices.Sort(near)
	return slices.Compact(near)
}

// ContactLookAlike answers whether the ladder pairs c with a contact of the
// batch, as Batch.CompanyLookAlike does for a company.
func (b *Batch) ContactLookAlike(ctx context.Context, inTx InTx, c NewContact, consumerMail *freemail.Matcher) (bool, error) {
	asked := askedContactOf(c, consumerMail)
	var near []int
	for _, lookup := range asked.lookups {
		near = append(near, b.byEmail[lookup]...)
	}
	for _, domain := range mailDomainsOf(c.Emails) {
		near = append(near, b.byRegistrable[freemail.Registrable(normalizeDomain(domain))]...)
	}
	var scored []string
	agreements := map[string]float64{}
	for _, at := range near {
		if b.goneContact[at] {
			continue
		}
		exact, scores := asked.verdict(b.contacts[at], agreements)
		if exact {
			return true, nil
		}
		if scores {
			scored = append(scored, b.contacts[at].record.FullName)
		}
	}
	return contactAdmitted(ctx, inTx, c.FullName, scored)
}
