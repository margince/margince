// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// fieldFullName is a lead's own name column, spelled once for the three places
// that map onto it.
const fieldFullName = "full_name"

// fieldEmail is the lead's identifying field: the natural key an import
// recognizes a row by, and the one value the store canonicalizes on write.
const fieldEmail = "email"

// fieldDomain is an organization's identifying field, and the one a spreadsheet
// most often carries as a Website column.
//
// A company's name describes it; its domain identifies it. DedupeOrganization
// asks about the domain FIRST and treats a hit as an exact collision, where a
// name match is only ever the fuzzy tier — so a file that carries domains is
// matched on a real key, and one that does not is matched on a string two
// different companies may share.
const fieldDomain = "domain"

// fieldTitle is the job title, carried by both a lead and a person: the same
// column of the same contact file, whichever object it is imported as.
const fieldTitle = "title"

// leadStatusNew is where an imported prospect starts: the unworked state a
// human moves it out of, which is the whole point of landing machine-sourced
// rows as leads rather than as people.
const leadStatusNew = "new"

// The fields a CSV import may target, per object. A closed set, because a
// mapping names a destination and every destination here has to be one this
// writer knows how to both create and update — a target it can only create
// would silently stop honouring the file on the second upload.
//
// `lead` and `person` are both offered and the caller picks per run: a
// machine-sourced list lands as leads for a human to promote, a file the
// business already knows lands as people. Neither skips the identity ladder.
//
// Every field here round-trips: the writer can both CREATE it and UPDATE it.
// `linkedin_url` is deliberately absent from both lists even though the stores
// know the field — a lead's patch input has no LinkedIn member and an
// organization's CREATE input has none, so advertising it would accept a
// column, report the row as written, and drop the value on one of the two
// paths. A target that only half works is worse than one the screen never
// offers.
var csvTargets = map[string][]string{
	migration.ObjectLead: {fieldFullName, fieldEmail, fieldTitle, "company_name"},
	// `domain` is what actually identifies a company. Its absence is why a
	// spreadsheet's website column had nowhere to go, and why company dedupe
	// falls back to matching names — which two real companies may legitimately
	// share. It round-trips: the create input takes a domain set and the patch
	// input takes the same set as a replace-set.
	migration.ObjectOrganization: append([]string{fieldDisplayName, "legal_name", fieldIndustry, "size_band", "description", fieldDomain}, recordAddressTargets...),
	// `phone`, `social` and `owner_id` are deliberately absent. A person's
	// patch input carries no Phones member and no single-column spelling of
	// Social, and an owner is a uuid a spreadsheet cannot honestly carry —
	// storekit.OwnerOrActor already defaults it to whoever ran the import.
	migration.ObjectPerson: append([]string{fieldFullName, "first_name", "last_name", fieldEmail, fieldTitle}, recordAddressTargets...),
}

// csvTargetID is the column that names the record this row IS, by the id the CRM
// gave it — the workflow behind a corrected export: read the companies out with
// their ids, edit the file, import it back.
//
// It is not a field. Nothing is written to it, and a row carrying one is an
// UPDATE of that record rather than a candidate for one. That is the whole
// reason it exists: matching a company by NAME cannot be made safe for
// overwriting, because the dedupe ladder is built to answer "should a human look
// at these two" and blurs exactly the distinctions an overwrite must keep — it
// strips legal forms, scores a trading name against a registered one, and breaks
// ties on uuid order. Every one of those is fine for proposing a review and is a
// way to destroy the wrong company's data when it decides a write.
//
// An id has none of that. It names one record or no record.
const csvTargetID = "id"

// recordAddressTargets are the address fields a CSV column may name, spelled with
// the `address.` prefix the record-fields schema already uses for the nested
// shape (`address: {city, country, …}`). The mapping itself stays flat — one
// column, one target — because a spreadsheet has no nesting to carry.
//
// They were absent until 2026-08-23, and their absence taught a model that a
// company has no address at all: asked to import a City column it reported
// that organizations know only display_name, legal_name, industry, size_band
// and description, then wrote "Standort: Hamburg" into the DESCRIPTION to
// avoid losing the value. Every other door — read_record, create_record,
// update_record — has carried the address the whole time.
//
// Both halves of the round-trip rule this list is built on hold: the create
// input and the patch input each take an *Address, so a mapped column is
// written on the first import and rewritten on the second. That holds for a
// person as well as an organization, which is why one list serves both — the
// six names are the contract's, not either object's.
var recordAddressTargets = []string{
	"address.line1", "address.line2", "address.city",
	"address.region", "address.postal_code", "address.country",
}

// csvSourceKeyDefault names the column a run identifies rows by when the
// request supplies none: the field that is the object's own natural identity.
// Stated per object rather than guessed, and the report says which was used.
var csvSourceKeyDefault = map[string]string{
	migration.ObjectLead:         fieldEmail,
	migration.ObjectOrganization: fieldDisplayName,
	migration.ObjectPerson:       fieldEmail,
}

// importTargets is the closed set a mapping may name for one object.
//
// Custom-field (cf_*) columns are NOT in it, and that is a limit rather than an
// oversight: an import lands its rows through the stores' caller-opened
// transaction seams, which refuse custom fields by design — reading the
// catalog is exactly the second connection those seams exist to avoid. So a
// cf_ target would be accepted, reported as written, and dropped. Custom
// fields arrive when the seam can carry them, not before.
func importTargets(object string) ([]string, error) {
	core, ok := csvTargets[object]
	if !ok {
		return nil, fmt.Errorf("import: %q has no mappable fields", object)
	}
	targets := append([]string(nil), core...)
	if selectsByID(object) {
		// Accepted as a column and absent from csvTargets, because those two
		// lists answer different questions. csvTargets is what a row WRITES, and
		// TestEveryImportTargetRoundTripsThroughCreateAndUpdate holds it to that:
		// a target advertised there and reaching neither input would be accepted,
		// reported as written, and dropped.
		//
		// `id` writes nothing. It names the record the row IS.
		targets = append(targets, csvTargetID)
	}
	if linksEmployer(object) {
		targets = append(targets, csvEmployerName)
	}
	return targets, nil
}

// selectsByID reports whether an object's rows may name the record they are.
//
// Organizations only, for now. A lead is identified by its email and the store's
// own unique key refuses a second one, so a lead row has no ambiguity for an id
// to resolve — and advertising a column that changes nothing would be worse than
// not offering it.
func selectsByID(object string) bool {
	return object == migration.ObjectOrganization
}

// linksEmployer reports whether an object's rows may name a company to be
// linked to, rather than written.
//
// People only: a contact file's company column is the person's employer, which
// is a RELATIONSHIP between two records. An organization row naming a company
// would be naming itself, and a lead holds its employer as free text on the lead
// itself (`company_name`, an ordinary writable field) precisely because a lead is
// not yet a record the estate links things to.
func linksEmployer(object string) bool {
	return object == migration.ObjectPerson
}

// isNonFieldTarget reports whether a mapped target names something other than a
// value the record stores.
//
// Two targets qualify and they are different from each other, which is why this
// is a set rather than a comparison. `id` names the RECORD the row is. The
// employer column names an EDGE from that record to another one. Neither is a
// column on the record, so neither may be compared against the stored JSON: the
// lookup would find nothing, report the row as changed, and hand the update path
// a field the store has no setter for — an update on every re-import of a file
// nobody edited.
//
// Excluded in one place rather than at each caller so no path can forget one.
func isNonFieldTarget(field string) bool {
	return field == csvTargetID || field == csvEmployerName
}

// changedFields reports which mapped values differ from what the stored record
// already holds. encoded is the record's own JSON.
//
// The comparison goes through that JSON rather than a hand-written per-field
// comparator: the wire shape and the mapping targets are the same vocabulary,
// so a field added to the contract is compared automatically, while a
// comparator would keep compiling and quietly stop noticing it.
func changedFields(encoded []byte, mapped map[string]string) (map[string]string, error) {
	var current map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &current); err != nil {
		return nil, fmt.Errorf("import: reading the stored record: %w", err)
	}

	changed := make(map[string]string, len(mapped))
	for field, incoming := range mapped {
		// `id` names the record and the employer column names an edge; neither is
		// a value the record holds. See isNonFieldTarget.
		if isNonFieldTarget(field) {
			continue
		}
		if textOf(storedValue(current, field)) != canonicalFor(field, incoming) {
			changed[field] = incoming
		}
	}
	return changed, nil
}

// storedValue reads one target's current value out of the record's JSON,
// following a dotted path one level down.
//
// A flat lookup was enough while every target was a top-level field. The
// address targets are not: the record encodes them under a nested `address`
// object, so `current["address.city"]` is always absent and EVERY address
// value compared as changed — which, on the update path, rewrote the whole
// address on every re-import of an unchanged file.
func storedValue(current map[string]json.RawMessage, field string) json.RawMessage {
	parent, child, nested := strings.Cut(field, ".")
	if !nested {
		// Two targets name a CHILD COLLECTION rather than a column on the
		// record, so a flat lookup finds nothing and reports them changed on
		// every re-import of a file nobody edited.
		switch field {
		case fieldEmail:
			return storedPrimaryEmail(current)
		case fieldDomain:
			return storedPrimaryDomain(current)
		}
		return current[field]
	}
	raw, ok := current[parent]
	if !ok {
		return nil
	}
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(raw, &inner); err != nil {
		// The parent is not an object, so it holds no such child. Absent
		// rather than an error: a target naming a path the record does not
		// have compares as empty, exactly as a missing top-level field does.
		return nil
	}
	return inner[child]
}

// canonicalFor renders an imported value the way the STORE will hold it, so
// the comparison is against what will actually be written rather than against
// the file's spelling.
//
// Email is the case that forces this: the store lowercases it on write, so
// `John@Example.com` compared raw differs from the stored `john@example.com`
// forever — every re-import of an unchanged file would rewrite the row, bump
// its version, and publish an update event for a change nobody made.
func canonicalFor(field, value string) string {
	trimmed := strings.TrimSpace(value)
	switch field {
	case fieldEmail:
		return strings.ToLower(trimmed)
	case fieldDomain:
		// Through the store's OWN parser, not an approximation of it. A
		// spreadsheet's website column carries "https://www.acme.com/" where the
		// store holds "acme.com", so a comparison that folded only case would
		// rewrite the row on every re-import of an unchanged file — the same
		// defect the email lowercasing fixed, with more ways to spell it. An
		// unparseable value compares as written and is refused later by name.
		if parsed, err := values.ParseDomain(trimmed); err == nil {
			return parsed.String()
		}
		return trimmed
	}
	return trimmed
}

func textOf(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	// A number, bool or object: its literal JSON is its text.
	return strings.TrimSpace(string(raw))
}

// leadCreateFrom builds the create input for one mapped row. Only mapped
// fields are set: an absent column leaves the field absent rather than
// clearing it, because the file said nothing about it.
func leadCreateFrom(fields map[string]string, sourceSystem, externalID, source string) people.CreateLeadInput {
	in := people.CreateLeadInput{
		Status:       leadStatusNew,
		SourceSystem: &sourceSystem,
		SourceID:     &externalID,
		Source:       source,
	}
	in.FullName = importString(fields, fieldFullName)
	in.Email = importString(fields, fieldEmail)
	in.Title = importString(fields, fieldTitle)
	in.CompanyName = importString(fields, "company_name")
	return in
}

// leadUpdateFrom builds the patch for the fields that actually differ.
func leadUpdateFrom(changed map[string]string) people.UpdateLeadInput {
	return people.UpdateLeadInput{
		FullName:    importString(changed, fieldFullName),
		Email:       importString(changed, fieldEmail),
		Title:       importString(changed, fieldTitle),
		CompanyName: importString(changed, "company_name"),
	}
}

func addressMergedOnto(current []byte, mapped *crmcontracts.Address) (*crmcontracts.Address, bool, error) {
	if mapped == nil {
		return nil, false, nil
	}
	var record struct {
		Address *crmcontracts.Address `json:"address"`
	}
	if err := json.Unmarshal(current, &record); err != nil {
		return nil, false, fmt.Errorf("import: reading the stored address: %w", err)
	}
	if record.Address == nil {
		return mapped, true, nil
	}
	merged := *record.Address
	if mapped.Line1 != nil {
		merged.Line1 = mapped.Line1
	}
	if mapped.Line2 != nil {
		merged.Line2 = mapped.Line2
	}
	if mapped.City != nil {
		merged.City = mapped.City
	}
	if mapped.Region != nil {
		merged.Region = mapped.Region
	}
	if mapped.PostalCode != nil {
		merged.PostalCode = mapped.PostalCode
	}
	if mapped.Country != nil {
		merged.Country = mapped.Country
	}
	return &merged, true, nil
}

// unwritableReason names why the store will refuse this row, or "" when it
// will not. It is the one place the dry run and the commit agree about what
// cannot be written, so a preview cannot promise a create the commit then
// fails on.
//
// Today it holds one check: size_band is a closed vocabulary with a database
// CHECK constraint behind it, and a headcount column mapped onto it ("240")
// reached the preview as a clean create, then could only fail at write time.
// A dry run whose whole job is to say what WILL happen may not report the
// opposite.
//
// The vocabulary is read off the generated contract, not hand-copied, for the
// same reason people.validSizeBands is: a band added to crm.yaml must not
// leave a second list behind saying otherwise. That obligation is on the set
// rather than on the spelling, so it is gated rather than stated.
// Held by: TestEveryClosedVocabularyOverAContractEnumHoldsAllOfIt (backend/contractvocabulary_test.go)
func unwritableReason(object string, fields map[string]string) string {
	if object == migration.ObjectPerson {
		// A person's addresses are parsed before the write transaction opens
		// (parsePersonContacts), so a malformed one refuses the row at commit.
		// Unchecked here, the dry run would promise a create for every bad
		// address in the file and the commit would answer differently — the
		// same shape as the size_band defect below, on the other object.
		email, given := fields[fieldEmail]
		if !given || strings.TrimSpace(email) == "" {
			return ""
		}
		if _, err := values.ParseEmail(strings.TrimSpace(email)); err != nil {
			return fmt.Sprintf("%q is not an email address that can be written", email)
		}
		return ""
	}
	if object != migration.ObjectOrganization {
		return ""
	}
	// A company's domains are parsed before the write transaction opens
	// (parseOrgDomains), so a value the parser refuses fails the row at commit.
	// The same argument as the email arm above and the size_band one below: a dry
	// run whose job is to say what WILL happen may not report the opposite.
	if domain := strings.TrimSpace(fields[fieldDomain]); domain != "" {
		if _, err := values.ParseDomain(domain); err != nil {
			return fmt.Sprintf("%q is not a domain that can be written", domain)
		}
	}
	band, given := fields["size_band"]
	if !given || strings.TrimSpace(band) == "" {
		return ""
	}
	if importableSizeBands[strings.TrimSpace(band)] {
		return ""
	}
	return fmt.Sprintf("size_band %q is not one of %s", band, strings.Join(sizeBandVocabulary(), ", "))
}

// importableSizeBands is the closed size_band vocabulary, off the contract.
var importableSizeBands = map[string]bool{
	string(crmcontracts.OrganizationSizeBandN110):      true,
	string(crmcontracts.OrganizationSizeBandN1150):     true,
	string(crmcontracts.OrganizationSizeBandN51200):    true,
	string(crmcontracts.OrganizationSizeBandN201500):   true,
	string(crmcontracts.OrganizationSizeBandN5011000):  true,
	string(crmcontracts.OrganizationSizeBandN10015000): true,
	string(crmcontracts.OrganizationSizeBandN5000):     true,
}

// sizeBandVocabulary renders the bands in ascending order for a refusal to
// carry — sorted explicitly, because a map's range order would give the user a
// different sentence on every call.
func sizeBandVocabulary() []string {
	out := make([]string, 0, len(importableSizeBands))
	for band := range importableSizeBands {
		out = append(out, band)
	}
	sort.Strings(out)
	return out
}

// addressFrom collects the mapped `address.*` columns into the one nested
// value both the create and the patch input take. Absent when the file mapped
// none, so a company with no address column is not handed an empty address —
// which the update path would read as an instruction to clear one.
func addressFrom(fields map[string]string) *crmcontracts.Address {
	addr := crmcontracts.Address{
		Line1:      importString(fields, "address.line1"),
		Line2:      importString(fields, "address.line2"),
		City:       importString(fields, "address.city"),
		Region:     importString(fields, "address.region"),
		PostalCode: importString(fields, "address.postal_code"),
		Country:    importString(fields, "address.country"),
	}
	if addr.Line1 == nil && addr.Line2 == nil && addr.City == nil &&
		addr.Region == nil && addr.PostalCode == nil && addr.Country == nil {
		return nil
	}
	return &addr
}

func importString(fields map[string]string, name string) *string {
	value, ok := fields[name]
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// textFields narrows the engine's JSON-shaped row to the text a delimited file
// actually carried. Every value in it came from a CSV cell, so the map the rest
// of this file works with says so in its type.
func textFields(fields map[string]any) map[string]string {
	out := make(map[string]string, len(fields))
	for name, value := range fields {
		out[name] = strings.TrimSpace(fmt.Sprint(value))
	}
	return out
}
