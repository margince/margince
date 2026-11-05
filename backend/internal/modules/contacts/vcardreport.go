// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What one imported card did, field by field. It reads the writers' own
// outcomes, so the report cannot claim a write the record does not show.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// VCardFieldAction is what the import did with one field of a card.
type VCardFieldAction string

const (
	// VCardFieldImported means the record now holds the card's value.
	VCardFieldImported VCardFieldAction = "imported"
	// VCardFieldReplaced means the card's value took the place of another,
	// which the undo buffer keeps.
	VCardFieldReplaced VCardFieldAction = "replaced"
	// VCardFieldKept means the card stated a value and the record kept its own.
	VCardFieldKept VCardFieldAction = "kept"
	// VCardFieldNotInCard means the card states nothing for the field.
	VCardFieldNotInCard VCardFieldAction = "not_in_card"
)

// VCardField is one field of a card and what the import did with it.
type VCardField struct {
	Field  string
	Value  string
	Action VCardFieldAction
}

// The report's names for what has no profile field of its own. A mobile number
// is told apart so a reader can see a card that carried none.
const (
	reportFieldName        = "name"
	reportFieldEmail       = "email"
	reportFieldMobilePhone = "mobile_phone"
)

// vcardReportFields lists the fields the import reads off a card, in report
// order. A field with no row is one the card does not state.
var vcardReportFields = []string{
	reportFieldName, reportFieldEmail, fieldPhone, reportFieldMobilePhone,
	fieldTitle, fieldCompanyName, fieldAddress, fieldLinkedin, fieldWebsite,
}

// cardReport collects one card's field rows as the writers decide them.
type cardReport []VCardField

func (r *cardReport) add(field, value string, action VCardFieldAction) {
	*r = append(*r, VCardField{Field: field, Value: value, Action: action})
}

// complete orders the rows and names every field no writer reported.
func (r cardReport) complete() []VCardField {
	out := slices.Clone(r)
	for _, field := range vcardReportFields {
		if !slices.ContainsFunc(out, func(f VCardField) bool { return f.Field == field }) {
			out = append(out, VCardField{Field: field, Action: VCardFieldNotInCard})
		}
	}
	slices.SortStableFunc(out, func(a, b VCardField) int {
		return slices.Index(vcardReportFields, a.Field) - slices.Index(vcardReportFields, b.Field)
	})
	return out
}

// phoneReportField names a number by its kind.
func phoneReportField(kind string) string {
	if kind == vcardTypeMobile {
		return reportFieldMobilePhone
	}
	return fieldPhone
}

// actionFor reads a statement's outcome as a report action. A field a human
// corrected is kept whatever the outcome says, since it was never written.
func actionFor(outcome observedOutcome, corrected bool) VCardFieldAction {
	switch {
	case corrected:
		return VCardFieldKept
	case outcome == observedReplaced:
		return VCardFieldReplaced
	case outcome.landed() || outcome == observedRestated:
		return VCardFieldImported
	default:
		return VCardFieldKept
	}
}

// createdReport reads a created card off the contact the create returned.
// url is the outcome of filing a URL the social slot did not take.
func createdReport(contact crmcontracts.Contact, entry VCardEntry, url *statedField) []VCardField {
	var r cardReport
	r.add(reportFieldName, contact.FullName, VCardFieldImported)
	if contact.Emails != nil {
		for _, e := range *contact.Emails {
			r.add(reportFieldEmail, string(e.Email), VCardFieldImported)
		}
	}
	if contact.Phones != nil {
		for _, p := range *contact.Phones {
			r.add(phoneReportField(string(p.PhoneType)), p.Phone, VCardFieldImported)
		}
	}
	if contact.Title != nil {
		r.add(fieldTitle, *contact.Title, VCardFieldImported)
	}
	if company := strings.TrimSpace(entry.Company); company != "" {
		r.add(fieldCompanyName, company, VCardFieldImported)
	}
	if contact.Address != nil {
		r.add(fieldAddress, strings.TrimSpace(entry.Address), VCardFieldImported)
	}
	if contact.Social != nil {
		if profile, ok := (*contact.Social)[profileURLField].(string); ok && profile != "" {
			r.add(fieldLinkedin, profile, VCardFieldImported)
		}
	}
	if url != nil {
		r.add(url.name, url.value, actionFor(url.outcome, false))
	}
	return r.complete()
}

// updatedReport reads a matched card off the statement the update applied.
// The address comes from the column fill, which is what the record shows.
func updatedReport(entry VCardEntry, st cardStatement, heldEmails map[string]bool, address VCardFieldAction) []VCardField {
	var r cardReport
	// A matched contact's name is never written by a card.
	r.add(reportFieldName, strings.TrimSpace(entry.FullName), VCardFieldKept)
	for _, email := range entry.Emails {
		v := strings.ToLower(strings.TrimSpace(email.Value))
		if v == "" {
			continue
		}
		action := VCardFieldKept
		if heldEmails[v] {
			action = VCardFieldImported
		}
		r.add(reportFieldEmail, v, action)
	}
	for _, f := range st.fields {
		if f.name != fieldAddress {
			r.add(f.name, f.value, actionFor(f.outcome, st.corrected[f.name]))
		}
	}
	if address != "" {
		r.add(fieldAddress, strings.TrimSpace(entry.Address), address)
	}
	reportNumbers(&r, entry, st)
	return r.complete()
}

// reportNumbers adds one row per number the card lists. A number the reader
// could not parse is kept, and a number listed twice is reported once.
func reportNumbers(r *cardReport, entry VCardEntry, st cardStatement) {
	outcomes := make(map[string]observedOutcome, len(st.numbers))
	for _, n := range st.numbers {
		outcomes[n.phone.String()] = n.outcome
	}
	reported := map[string]bool{}
	for _, phone := range entry.Phones {
		field := phoneReportField(phone.Kind)
		parsed, err := values.ParsePhone(phone.Value)
		if err != nil {
			r.add(field, strings.TrimSpace(phone.Value), VCardFieldKept)
			continue
		}
		number := parsed.String()
		if reported[number] {
			continue
		}
		reported[number] = true
		// A number the writer never reached reads as observedSkipped, the zero outcome.
		r.add(field, number, actionFor(outcomes[number], st.corrected[fieldPhone]))
	}
}

// liveEmails is the set of addresses a contact holds, lowercased.
func liveEmails(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (map[string]bool, error) {
	rows, err := tx.Query(ctx,
		`SELECT lower(email) FROM contact_email WHERE contact_id = $1 AND archived_at IS NULL`, contactID)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the matched contact's addresses: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("contacts: reading an address: %w", err)
		}
		out[email] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: reading the matched contact's addresses: %w", err)
	}
	return out, nil
}
