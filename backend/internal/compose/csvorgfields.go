// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How an organization row becomes the store's inputs, and back again.
//
// Split from csvfields.go for the reason csvpersonfields.go is: an organization's
// identity lives in a CHILD COLLECTION. Its domains are rows with their own
// primary flag and their own estate-wide uniqueness, so the create path, the
// patch path and the diff each need a shape the flat helpers next door do not
// have.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/people"
)

// storedPrimaryDomain reads the `domain` target's current value.
//
// An organization holds its domains as a child collection, so `current["domain"]`
// is always absent — the same shape a person's emails have, and the same defect
// if left alone. The primary is the one a spreadsheet's single column names, so
// it is the one to compare against; a company's other domains are not
// expressible in the file and the writer preserves them rather than diffing here.
func storedPrimaryDomain(current map[string]json.RawMessage) json.RawMessage {
	raw, ok := current["domains"]
	if !ok {
		return nil
	}
	var rows []struct {
		Domain    string `json:"domain"`
		IsPrimary bool   `json:"is_primary"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil || len(rows) == 0 {
		return nil
	}
	chosen := rows[0].Domain
	for _, row := range rows {
		if row.IsPrimary {
			chosen = row.Domain
			break
		}
	}
	encoded, err := json.Marshal(chosen)
	if err != nil {
		return nil
	}
	return encoded
}

// textOf renders one stored JSON value as the text a file would have carried.
// Every value a delimited file can hold arrives as text, so text is the only
// comparison that can be made without inventing a type the file never declared.
// An absent field renders empty, which no non-empty import value equals.

func organizationCreateFrom(fields map[string]string, source string) people.CreateOrganizationInput {
	in := people.CreateOrganizationInput{
		DisplayName: strings.TrimSpace(fields[fieldDisplayName]),
		Source:      source,
	}
	in.LegalName = importString(fields, "legal_name")
	in.Description = importString(fields, "description")
	in.Industry = importString(fields, fieldIndustry)
	in.SizeBand = importString(fields, "size_band")
	in.Domains = orgDomainsFrom(fields)
	in.Address = addressFrom(fields)
	return in
}

// addressMergedOnto folds the file's address components onto the ones the
// record already holds, so an import that carries a City does not blank the
// street beside it.
//
// The store's address patch is all-or-nothing by design — buildOrganizationPatch
// assigns all six columns whenever an Address is present — which is right for a
// form that always submits the whole address, and wrong for a spreadsheet whose
// columns are whatever the customer happened to export. Merging here rather
// than changing the patch keeps that door's contract intact: every other caller
// still sends a complete address and still means it.
//
// The bool reports whether the file carried an address at all. A caller that
// gets false must leave the record's address alone rather than send nil, which
// the patch builder cannot distinguish from "no address given".

func organizationUpdateFrom(changed map[string]string) people.UpdateOrganizationInput {
	in := people.UpdateOrganizationInput{
		DisplayName: importString(changed, fieldDisplayName),
		LegalName:   importString(changed, "legal_name"),
		Description: importString(changed, "description"),
		Industry:    importString(changed, fieldIndustry),
		SizeBand:    importString(changed, "size_band"),
		Address:     addressFrom(changed),
	}
	// The replace-set behind a pointer: nil leaves the stored domains alone, an
	// empty slice CLEARS them. Taken only when the column actually carried a
	// value, so a file that never mapped one reaches the first case.
	//
	// This is the file's domain ALONE. The caller merges the company's other
	// domains onto it before writing (domainsMergedOnto), because only the
	// caller has the stored record to merge against — but the target has to
	// reach this input for the round-trip rule to hold, and a domain that never
	// arrives here would be accepted, reported as written, and dropped.
	if domains := orgDomainsFrom(changed); domains != nil {
		in.Domains = &domains
	}
	return in
}

// domainsMergedOnto folds the file's single domain column onto the domains the
// company already holds, so an import that names one does not archive the rest.
//
// The store's Domains is a REPLACE-SET: whatever it is given becomes the whole
// live set, and everything absent from it is archived. That is right for a form
// submitting every domain a company has, and wrong for a spreadsheet carrying
// one Website column — a company with three domains would keep the one the file
// named and silently lose the other two.
//
// The file's domain becomes the primary; every stored domain it did not name is
// carried through, demoted so exactly one row claims that slot.
//
// The bool reports whether the file carried a domain at all. False means leave
// the company's domains alone rather than send an empty set, which the store
// reads as "archive them all".
func domainsMergedOnto(current []byte, mapped []people.OrgDomainInput) ([]people.OrgDomainInput, bool, error) {
	if len(mapped) == 0 {
		return nil, false, nil
	}
	var record struct {
		Domains []struct {
			Domain string `json:"domain"`
		} `json:"domains"`
	}
	if err := json.Unmarshal(current, &record); err != nil {
		return nil, false, fmt.Errorf("import: reading the stored domains: %w", err)
	}
	incoming := strings.ToLower(strings.TrimSpace(mapped[0].Domain))
	merged := append([]people.OrgDomainInput(nil), mapped...)
	merged[0].IsPrimary = true
	for _, held := range record.Domains {
		if strings.EqualFold(strings.TrimSpace(held.Domain), incoming) {
			continue
		}
		merged = append(merged, people.OrgDomainInput{Domain: held.Domain, IsPrimary: false})
	}
	return merged, true, nil
}

// orgDomainsFrom reads the single domain column a spreadsheet carries into the
// set shape the store takes.
//
// A file has one Website or Domain column, so the value it names is the
// company's primary domain. Nil when the column is absent or blank, which is
// what keeps "the file said nothing" distinct from "the file says none" —
// the store reads an empty set as an instruction to archive every domain the
// company holds.
func orgDomainsFrom(fields map[string]string) []people.OrgDomainInput {
	domain := strings.TrimSpace(fields[fieldDomain])
	if domain == "" {
		return nil
	}
	return []people.OrgDomainInput{{Domain: domain, IsPrimary: true}}
}

// importString reads one mapped field as a pointer, absent when the file did
// not carry it. A nil is "the file said nothing", never "the file said empty":
// the source drops blank values before they reach here, so an empty column
// cannot silently erase a value somebody entered by hand.
