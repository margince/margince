// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// descriptionField is the free-text column an import may map onto.
const descriptionField = "description"

// fieldFullName is a lead's own name column, spelled once for the three places
// that map onto it.
const fieldFullName = "full_name"

// fieldPhone is a phone number. A lead holds one in a column; a contact's
// numbers are child rows, and the file's number becomes its primary work one.
const fieldPhone = "phone"

// fieldLinkedIn is a LinkedIn URL: a company's page, a lead's profile, or the
// LinkedIn handle in a contact's social links.
const fieldLinkedIn = "linkedin_url"

// fieldAddressCountry is the country of a contact's or company's address, which
// the stores hold as an ISO 3166-1 alpha-2 code.
const fieldAddressCountry = "address.country"

// fieldEmail is the lead's identifying field: the natural key an import
// recognizes a row by, and the one value the store canonicalizes on write.
const fieldEmail = "email"

// fieldDomain is a company's identifying field, and the one a spreadsheet
// most often carries as a Website column.
//
// A company's name describes it; its domain identifies it. DedupeCompany
// asks about the domain FIRST and treats a hit as an exact collision, where a
// name match is only ever the fuzzy tier — so a file that carries domains is
// matched on a real key, and one that does not is matched on a string two
// different companies may share.
const fieldDomain = "domain"

// fieldTitle is the job title, carried by both a lead and a contact: the same
// column of the same contact file, whichever object it is imported as.
const fieldTitle = "title"

// leadStatusNew is where an imported prospect starts: the unworked state a
// human moves it out of, which is the whole point of landing machine-sourced
// rows as leads rather than as contacts.
const leadStatusNew = "new"

// leadTargetStatus is the lead's status column. A file may place a lead only
// on the open ladder; the terminal statuses go through promote and disqualify.
const leadTargetStatus = "status"

// The fields a CSV import may target, per object. Each one round-trips: the
// writer can both create and update it. A create-only target would stop
// honouring the file on the second upload.
//
// The caller picks `lead` or `contact` per run. Neither skips the identity ladder.
var csvTargets = map[string][]string{
	migration.ObjectLead: {fieldFullName, fieldEmail, fieldPhone, fieldTitle, companyNameField, fieldLinkedIn, leadTargetStatus, csvTargetAuthor, csvTargetOwner, csvTargetTags},
	// `domain` is what actually identifies a company. Its absence is why a
	// spreadsheet's website column had nowhere to go, and why company dedupe
	// falls back to matching names — which two real companies may legitimately
	// share. It round-trips: the create input takes a domain set and the patch
	// input takes the same set as a replace-set.
	migration.ObjectCompany: append([]string{fieldDisplayName, fieldLegalName, fieldIndustry, fieldSizeBand, descriptionField, fieldDomain, fieldLinkedIn, csvTargetAuthor, csvTargetOwner, csvTargetTags}, recordAddressTargets...),
	// The owner is named by the seat's email (csvowners.go), never by its id.
	// `author` is a name (csvauthor.go).
	migration.ObjectContact: append([]string{fieldFullName, "first_name", "last_name", fieldEmail, fieldPhone, fieldLinkedIn, fieldTitle, csvTargetAuthor, csvTargetOwner, csvTargetTags}, recordAddressTargets...),
	// A stage change is an advance (dealUpdateFrom); the lost reason is written once.
	migration.ObjectDeal: {
		dealTargetName, dealTargetPipeline, dealTargetStage, dealTargetAmount, dealTargetCurrency, dealTargetCloseDate,
		csvTargetCompanyRef, dealTargetContactRef, csvTargetOwner, csvTargetTags, descriptionField, dealTargetLostReason,
	},
	// The kind, direction and links are fixed when an activity lands (isCreateOnlyTarget).
	// An activity carries no tags and no custom fields.
	migration.ObjectActivity: {
		activityTargetType, activityTargetSubject, activityTargetBody, activityTargetOccurredAt, activityTargetLinks,
		activityTargetDuration, activityTargetMeetingStatus, activityTargetDirection, csvTargetAuthor, csvTargetOwner,
	},
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

// csvTargetRef is the column holding the row's own id in the source system, the
// one other files' links name. It is the part's source key, never a stored field.
const csvTargetRef = "ref"

// csvTargetCompanyRef names a contact's employer by the company's ref, its
// domain, its one name or its id. Like the employer name it writes an edge.
const csvTargetCompanyRef = migration.AssocTargetCompanyRef

// recordAddressTargets are the address fields a CSV column may name, spelled with
// the `address.` prefix the record-fields schema already uses for the nested
// shape (`address: {city, country, …}`). The mapping itself stays flat — one
// column, one target — because a spreadsheet has no nesting to carry.
//
// They were absent until 2026-08-23, and their absence taught a model that a
// company has no address at all: asked to import a City column it reported
// that companies know only display_name, legal_name, industry, size_band
// and description, then wrote "Standort: Hamburg" into the DESCRIPTION to
// avoid losing the value. Every other door — read_record, create_record,
// update_record — has carried the address the whole time.
//
// Both halves of the round-trip rule this list is built on hold: the create
// input and the patch input each take an *Address, so a mapped column is
// written on the first import and rewritten on the second. That holds for a
// contact as well as a company, which is why one list serves both — the
// six names are the contract's, not either object's.
var recordAddressTargets = []string{
	"address.line1", "address.line2", "address.city",
	"address.region", "address.postal_code", "address.country",
}

// csvSourceKeyDefault names the column a run identifies rows by when the
// request supplies none: the field that is the object's own natural identity.
// Stated per object rather than guessed, and the report says which was used.
var csvSourceKeyDefault = map[string]string{
	migration.ObjectLead:    fieldEmail,
	migration.ObjectCompany: fieldDisplayName,
	migration.ObjectContact: fieldEmail,
	// Two deals may share a name, so a deal is known only by its ref.
	migration.ObjectDeal: csvTargetRef,
	// An activity has no natural key at all.
	migration.ObjectActivity: csvTargetRef,
}

// importTargets is the closed set a mapping may name for one object: its core
// fields, then each active custom field by key, in column order.
func importTargets(object string, cat importCatalog) ([]string, error) {
	core, ok := csvTargets[object]
	if !ok {
		return nil, fmt.Errorf("import: %q has no mappable fields", object)
	}
	targets := append([]string(nil), core...)
	if hasCustomFields(object) {
		targets = append(targets, slices.Sorted(maps.Keys(cat.byName))...)
	}
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
		targets = append(targets, csvEmployerName, csvTargetCompanyRef)
	}
	return append(targets, csvTargetRef), nil
}

// hasCustomFields reports whether an object's store takes custom-field cells.
// The activities store has no member for them.
func hasCustomFields(object string) bool { return object != migration.ObjectActivity }

// selectsByID reports whether an object's rows may name the record they are.
//
// Companies only, for now. A lead is identified by its email and the store's
// own unique key refuses a second one, so a lead row has no ambiguity for an id
// to resolve — and advertising a column that changes nothing would be worse than
// not offering it.
func selectsByID(object string) bool {
	return object == migration.ObjectCompany
}

// linksEmployer reports whether an object's rows may name a company to be
// linked to, rather than written.
//
// Contacts only: a contact file's company column is the contact's employer, which
// is a RELATIONSHIP between two records. A company row naming a company
// would be naming itself, and a lead holds its employer as free text on the lead
// itself (`company_name`, an ordinary writable field) precisely because a lead is
// not yet a record the estate links things to.
func linksEmployer(object string) bool {
	return object == migration.ObjectContact
}

// isNonFieldTarget reports whether a mapped target names something other than a
// value the record stores.
//
// `id` and `ref` name the record, the employer columns an edge, and `tags`
// tags added beside the write. None is a column, so a stored-JSON lookup would
// report every unedited row as changed on each re-import.
func isNonFieldTarget(field string) bool {
	return field == csvTargetID || field == csvEmployerName || field == csvTargetRef ||
		field == csvTargetCompanyRef || field == dealTargetContactRef || field == csvTargetTags
}

// changedFields reports which mapped values differ from what the stored record
// already holds. encoded is the record's own JSON.
//
// The comparison goes through that JSON rather than a hand-written per-field
// comparator: the wire shape and the mapping targets are the same vocabulary,
// so a field added to the contract is compared automatically, while a
// comparator would keep compiling and quietly stop noticing it.
func changedFields(encoded []byte, mapped map[string]string, typed map[string]any) (map[string]string, error) {
	var current map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &current); err != nil {
		return nil, fmt.Errorf("import: reading the stored record: %w", err)
	}

	mapped = withNamePartsAsFullName(mapped)
	changed := make(map[string]string, len(mapped))
	for field, incoming := range mapped {
		// A deal's company is a column of the deal: its row carries the id its
		// ref resolved to, and a different one is a change.
		if field == csvTargetCompanyRef && dealCompanyChanged(current, typed) {
			changed[field] = incoming
		}
		// `id`, `ref` and the employer column are not values the record holds
		// (isNonFieldTarget). The author is written on create only
		// (isCreateOnlyTarget).
		if isNonFieldTarget(field) || isCreateOnlyTarget(field) {
			continue
		}
		// An empty box changes nothing, so it is never an update. A lead's
		// status would otherwise read as moved to nothing on every re-import.
		if field == leadTargetStatus && incoming == "" {
			continue
		}
		want := canonicalFor(field, incoming)
		value, checked := typed[field]
		if field == csvTargetOwner && !checked {
			// An address no seat holds leaves the record's owner as it is.
			continue
		}
		if checked {
			var err error
			if want, err = wireText(value); err != nil {
				return nil, err
			}
		}
		if textOf(storedValue(current, field)) != want {
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
	if column, renamed := dealStoredColumns[field]; renamed {
		return current[column]
	}
	if column, renamed := activityStoredColumns[field]; renamed {
		return current[column]
	}
	if field == activityTargetOccurredAt {
		return storedInstant(current[field])
	}
	parent, child, nested := strings.Cut(field, ".")
	if !nested {
		// These targets may name a child collection, not a column. A flat
		// lookup finds nothing and reports them changed on every re-import.
		switch field {
		case fieldEmail:
			return storedPrimaryEmail(current)
		case fieldDomain:
			return storedPrimaryDomain(current)
		case fieldPhone:
			return storedPrimaryPhone(current)
		case fieldLinkedIn:
			return storedLinkedIn(current)
		case csvTargetOwner:
			return current["owner_id"]
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
	case fieldIndustry:
		return canonicalIndustry(trimmed)
	case fieldEmail:
		return correspondence.Fold(trimmed)
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
func leadCreateFrom(fields map[string]string, typed map[string]any, sourceSystem, externalID, source string) contacts.CreateLeadInput {
	in := contacts.CreateLeadInput{
		Status:       leadStatusNew,
		SourceSystem: &sourceSystem,
		SourceID:     &externalID,
		Source:       source,
		Author:       authorFrom(fields),
		OwnerID:      ownerFrom(fields, typed),
	}
	if status, ok := typed[leadTargetStatus].(string); ok {
		in.Status = status
	}
	in.FullName = importString(fields, fieldFullName)
	in.Email = importString(fields, fieldEmail)
	in.Phone = checkedString(fields, typed, fieldPhone)
	in.LinkedInURL = checkedString(fields, typed, fieldLinkedIn)
	in.Title = importString(fields, fieldTitle)
	in.CompanyName = importString(fields, "company_name")
	in.CustomFields = customFieldsFrom(fields, typed)
	return in
}

// leadUpdateFrom builds the patch for the fields that actually differ.
func leadUpdateFrom(changed map[string]string, typed map[string]any) contacts.UpdateLeadInput {
	var status *string
	if text, ok := typed[leadTargetStatus].(string); ok {
		if _, given := changed[leadTargetStatus]; given {
			status = &text
		}
	}
	return contacts.UpdateLeadInput{
		Status:       status,
		FullName:     importString(changed, fieldFullName),
		Email:        importString(changed, fieldEmail),
		Phone:        checkedString(changed, typed, fieldPhone),
		LinkedInURL:  checkedString(changed, typed, fieldLinkedIn),
		Title:        importString(changed, fieldTitle),
		CompanyName:  importString(changed, "company_name"),
		OwnerID:      ownerFrom(changed, typed),
		CustomFields: customFieldsFrom(changed, typed),
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
		Country:    importString(fields, fieldAddressCountry),
	}
	if addr.Line1 == nil && addr.Line2 == nil && addr.City == nil &&
		addr.Region == nil && addr.PostalCode == nil && addr.Country == nil {
		return nil
	}
	return &addr
}

// checkedString is a core cell in the spelling checkRow gave it, or the
// file's text where the cell was not checked.
func checkedString(fields map[string]string, typed map[string]any, name string) *string {
	if _, given := fields[name]; !given {
		return nil
	}
	if checked, ok := typed[name].(string); ok {
		return &checked
	}
	return importString(fields, name)
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
