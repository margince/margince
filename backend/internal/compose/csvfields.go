// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/migration"
	"github.com/gradionhq/margince/backend/internal/modules/people"
)

// fieldFullName is a lead's own name column, spelled once for the three places
// that map onto it.
const fieldFullName = "full_name"

// fieldEmail is the lead's identifying field: the natural key an import
// recognizes a row by, and the one value the store canonicalizes on write.
const fieldEmail = "email"

// leadStatusNew is where an imported prospect starts: the unworked state a
// human moves it out of, which is the whole point of landing machine-sourced
// rows as leads rather than as people.
const leadStatusNew = "new"

// The fields a CSV import may target, per object. A closed set, because a
// mapping names a destination and every destination here has to be one this
// writer knows how to both create and update — a target it can only create
// would silently stop honouring the file on the second upload.
//
// `lead` and not `person` is ADR-0008: machine-sourced rows land as leads and
// a human promotes them.
// Every field here round-trips: the writer can both CREATE it and UPDATE it.
// `linkedin_url` is deliberately absent from both lists even though the stores
// know the field — a lead's patch input has no LinkedIn member and an
// organization's CREATE input has none, so advertising it would accept a
// column, report the row as written, and drop the value on one of the two
// paths. A target that only half works is worse than one the screen never
// offers.
var csvTargets = map[string][]string{
	migration.ObjectLead:         {fieldFullName, fieldEmail, "title", "company_name"},
	migration.ObjectOrganization: append([]string{fieldDisplayName, "legal_name", fieldIndustry, "size_band", "description"}, orgAddressTargets...),
}

// orgAddressTargets are the address fields a CSV column may name, spelled with
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
// written on the first import and rewritten on the second.
var orgAddressTargets = []string{
	"address.line1", "address.line2", "address.city",
	"address.region", "address.postal_code", "address.country",
}

// csvSourceKeyDefault names the column a run identifies rows by when the
// request supplies none: the field that is the object's own natural identity.
// Stated per object rather than guessed, and the report says which was used.
var csvSourceKeyDefault = map[string]string{
	migration.ObjectLead:         fieldEmail,
	migration.ObjectOrganization: fieldDisplayName,
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
	return append([]string(nil), core...), nil
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
		if textOf(current[field]) != canonicalFor(field, incoming) {
			changed[field] = incoming
		}
	}
	return changed, nil
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
	if field == fieldEmail {
		return strings.ToLower(trimmed)
	}
	return trimmed
}

// textOf renders one stored JSON value as the text a file would have carried.
// Every value a delimited file can hold arrives as text, so text is the only
// comparison that can be made without inventing a type the file never declared.
// An absent field renders empty, which no non-empty import value equals.
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
	in.Title = importString(fields, "title")
	in.CompanyName = importString(fields, "company_name")
	return in
}

// leadUpdateFrom builds the patch for the fields that actually differ.
func leadUpdateFrom(changed map[string]string) people.UpdateLeadInput {
	return people.UpdateLeadInput{
		FullName:    importString(changed, fieldFullName),
		Email:       importString(changed, fieldEmail),
		Title:       importString(changed, "title"),
		CompanyName: importString(changed, "company_name"),
	}
}

func organizationCreateFrom(fields map[string]string, source string) people.CreateOrganizationInput {
	in := people.CreateOrganizationInput{
		DisplayName: strings.TrimSpace(fields[fieldDisplayName]),
		Source:      source,
	}
	in.LegalName = importString(fields, "legal_name")
	in.Description = importString(fields, "description")
	in.Industry = importString(fields, fieldIndustry)
	in.SizeBand = importString(fields, "size_band")
	in.Address = addressFrom(fields)
	return in
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
// leave a second list behind saying otherwise.
func unwritableReason(object string, fields map[string]string) string {
	if object != migration.ObjectOrganization {
		return ""
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

func organizationUpdateFrom(changed map[string]string) people.UpdateOrganizationInput {
	return people.UpdateOrganizationInput{
		DisplayName: importString(changed, fieldDisplayName),
		LegalName:   importString(changed, "legal_name"),
		Description: importString(changed, "description"),
		Industry:    importString(changed, fieldIndustry),
		SizeBand:    importString(changed, "size_band"),
		Address:     addressFrom(changed),
	}
}

// importString reads one mapped field as a pointer, absent when the file did
// not carry it. A nil is "the file said nothing", never "the file said empty":
// the source drops blank values before they reach here, so an empty column
// cannot silently erase a value somebody entered by hand.
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
