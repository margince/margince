// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How a person row becomes the store's inputs, and back again.
//
// Split from csvfields.go because a person is the one import object whose
// identity lives in a CHILD TABLE. Every other target is a column on the record:
// map it, write it, compare it. An email is a row in person_email with its own
// primary flag, its own position and its own uniqueness across the estate, so
// the create path, the patch path and the diff each need a shape the flat helpers
// next door do not have. Keeping that in one file is what stops the shape leaking
// into the organization and lead paths, which do not want it.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/people"
)

// personCreateFrom builds the create input for one mapped row.
//
// FullName is composed rather than merely read, because a person row must carry
// SOME name and a spreadsheet often splits it. The ladder is full_name as given,
// else first+last joined, else the email — the same fallback promotion already
// makes when a lead has an address and no name, and for the same reason: a
// record named by its email is worth having, and one named by nothing is not.
// A row with neither is refused by the caller, which is the only shape that
// names no person at all.
func personCreateFrom(fields map[string]string, source string) people.CreatePersonInput {
	in := people.CreatePersonInput{
		// Recorded as the import it is. unknown_legacy is the honest answer
		// where a door cannot say why a contact exists; here the door knows,
		// and letting it default would make a CSV lane
		// indistinguishable from a rep typing a name in.
		Acquisition: people.Acquisition{Kind: people.AcquiredPurchasedOrImported},
		FullName:    personFullName(fields),
		Source:      source,
	}
	in.FirstName = importString(fields, "first_name")
	in.LastName = importString(fields, "last_name")
	in.Title = importString(fields, fieldTitle)
	in.Address = addressFrom(fields)
	in.Emails = personEmailsFrom(fields)
	return in
}

// personUpdateFrom builds the patch for the fields that actually differ.
// Emails stays nil unless the file carried the column, so a row that never
// mentioned an address does not read as "this person now has none".
func personUpdateFrom(changed map[string]string) people.UpdatePersonInput {
	in := people.UpdatePersonInput{
		FullName:  importString(changed, fieldFullName),
		FirstName: importString(changed, "first_name"),
		LastName:  importString(changed, "last_name"),
		Title:     importString(changed, fieldTitle),
		Address:   addressFrom(changed),
	}
	in.Emails = personEmailsFrom(changed)
	return in
}

// personFullName is the name ladder personCreateFrom documents.
func personFullName(fields map[string]string) string {
	if name := strings.TrimSpace(fields[fieldFullName]); name != "" {
		return name
	}
	joined := strings.TrimSpace(strings.TrimSpace(fields["first_name"]) + " " + strings.TrimSpace(fields["last_name"]))
	if joined != "" {
		return joined
	}
	return strings.TrimSpace(fields[fieldEmail])
}

// personEmailsFrom reads the single email column a spreadsheet carries into the
// child-row shape the store takes. A file has one Email column, so the address
// it names is the primary work address; a person's other addresses are a flip
// concern and are preserved on update rather than expressed here.
func personEmailsFrom(fields map[string]string) []people.PersonEmailInput {
	email := strings.TrimSpace(fields[fieldEmail])
	if email == "" {
		return nil
	}
	return []people.PersonEmailInput{{
		Email: email, EmailType: personEmailTypeWork, IsPrimary: true, Position: 1,
		// A spreadsheet is not correspondence: the address is asserted by
		// whoever exported the file, never vouched for by a reply.
		VouchedNotCorresponded: true,
	}}
}

// personEmailTypeWork is the kind a spreadsheet's Email column names. A file
// carries one address per person and no column saying what sort it is, and a
// work address is what a CRM import means.
const personEmailTypeWork = "work"

// emailsMergedOnto folds the file's single email column onto the addresses the
// person already holds, so an import that names one address does not archive the
// others.
//
// It is addressMergedOnto's argument applied to child rows: replacePersonEmails
// makes the live set mirror what it is given, which is right for a caller that
// sends the whole set and wrong for a spreadsheet carrying one Email column. The
// file's address becomes the primary WORK address; every stored address it did
// not name is carried through.
//
// Only same-type rows are demoted. uq_person_email_primary allows one primary
// per (person_id, email_type), not one per person — so clearing the flag on
// every retained row would strip a person's primary PERSONAL address because
// the file named a work one, which is data the import was never given and had
// no business changing.
//
// The bool reports whether the file carried an email at all. False means leave
// the person's addresses alone rather than send an empty set, which
// replacePersonEmails would read as "archive them all".
func emailsMergedOnto(current []byte, mapped []people.PersonEmailInput) ([]people.PersonEmailInput, bool, error) {
	if len(mapped) == 0 {
		return nil, false, nil
	}
	var record struct {
		Emails []struct {
			Email     string `json:"email"`
			EmailType string `json:"email_type"`
			IsPrimary bool   `json:"is_primary"`
		} `json:"emails"`
	}
	if err := json.Unmarshal(current, &record); err != nil {
		return nil, false, fmt.Errorf("import: reading the stored emails: %w", err)
	}
	incoming := strings.ToLower(strings.TrimSpace(mapped[0].Email))
	merged := append([]people.PersonEmailInput(nil), mapped...)
	merged[0].Position = 1
	merged[0].IsPrimary = true
	for _, held := range record.Emails {
		if strings.EqualFold(strings.TrimSpace(held.Email), incoming) {
			continue
		}
		kind := held.EmailType
		if kind == "" {
			kind = personEmailTypeWork
		}
		merged = append(merged, people.PersonEmailInput{
			Email: held.Email, EmailType: kind,
			// Demoted only if it competes with the incoming address for the
			// same type's primary slot. A primary personal address keeps its
			// flag when the file named a work one.
			IsPrimary:              held.IsPrimary && kind != mapped[0].EmailType,
			Position:               len(merged) + 1,
			VouchedNotCorresponded: true,
		})
	}
	return merged, true, nil
}

// storedPrimaryEmail reads the `email` target's current value for either object
// that advertises it.
//
// A lead encodes its address as a top-level `email` string. A PERSON does not:
// it holds an `emails` array of child rows, so `current["email"]` is always
// absent and the address compared as changed on every re-import — the same
// defect the nested address paths had, in a shape a dotted path cannot reach.
// The primary is the one a spreadsheet's single Email column names, so it is the
// one to compare against; a person's other addresses are not expressible in the
// file and are preserved by the writer rather than diffed here.
func storedPrimaryEmail(current map[string]json.RawMessage) json.RawMessage {
	if raw, ok := current[fieldEmail]; ok {
		return raw
	}
	raw, ok := current["emails"]
	if !ok {
		return nil
	}
	var rows []struct {
		Email     string `json:"email"`
		EmailType string `json:"email_type"`
		IsPrimary bool   `json:"is_primary"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil || len(rows) == 0 {
		return nil
	}
	// The primary WORK address, because that is what personEmailsFrom writes and
	// so the only row the file's column can be compared against. A person may
	// hold a primary personal address at the same time — the slot is per type —
	// and matching on is_primary alone would let row order decide which one the
	// comparison sees, reporting an update whenever it picked the other.
	var chosen string
	for _, row := range rows {
		if row.IsPrimary && row.EmailType == personEmailTypeWork {
			chosen = row.Email
			break
		}
	}
	if chosen == "" {
		for _, row := range rows {
			if row.EmailType == personEmailTypeWork {
				chosen = row.Email
				break
			}
		}
	}
	if chosen == "" {
		chosen = rows[0].Email
	}
	encoded, err := json.Marshal(chosen)
	if err != nil {
		return nil
	}
	return encoded
}
