// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// typedWrite says this write was typed by hand. Only an import run or a sync,
// which code sets on the context, keeps the source's own names; no role does.
func typedWrite(ctx context.Context) bool {
	return storekit.ImportRunOf(ctx) == nil && !storekit.Synced(ctx)
}

// checkCompanyEditText refuses a typed rename past the label limit and a changed
// description past the column's. A stored value sent back unchanged is admitted.
func checkCompanyEditText(ctx context.Context, current crmcontracts.Company, in UpdateCompanyInput) error {
	if in.DisplayName != nil && *in.DisplayName != current.DisplayName && typedWrite(ctx) {
		if err := httperr.RequireWithin(companyNameColumn, *in.DisplayName, values.TextLabelCharacters); err != nil {
			return err
		}
	}
	if in.Description != nil && current.Description != nil && *in.Description == *current.Description {
		return nil
	}
	return checkCompanyDescription(in.Description)
}

// ContactNameLimit is the most characters a contact name holds: each part, and
// the full name they join to. A typed name and an imported one answer to it alike.
const ContactNameLimit = values.TextLabelCharacters

// OverlongContactName answers the first of full_name, first_name and last_name
// whose trimmed text is past ContactNameLimit, with that text and its length.
// Two parts that fit alone but join past the limit answer last_name with the
// joined text. A file's refusal then points at a cell the file has.
func OverlongContactName(names map[string]string) (column, text string, length int, over bool) {
	for _, column := range []string{contactNameColumn, columnFirstName, columnLastName} {
		text := strings.TrimSpace(names[column])
		if length := utf8.RuneCountInString(text); length > ContactNameLimit {
			return column, text, length, true
		}
	}
	joined := JoinNameParts(names[columnFirstName], names[columnLastName])
	if length := utf8.RuneCountInString(joined); length > ContactNameLimit {
		return columnLastName, joined, length, true
	}
	return "", "", 0, false
}

// ContactNameTooLongError refuses a contact name past ContactNameLimit. It
// answers 422 too_long on every surface, and an import reads its length.
type ContactNameTooLongError struct {
	Field string
	Runes int
}

func (e *ContactNameTooLongError) Error() string {
	return fmt.Sprintf("%s holds at most %d characters; this one has %d", e.Field, ContactNameLimit, e.Runes)
}

// FieldFault names the refused name column.
func (e *ContactNameTooLongError) FieldFault() (field, code, message string) {
	return e.Field, "too_long", e.Error()
}

// FieldMax is the name limit.
func (e *ContactNameTooLongError) FieldMax() int { return ContactNameLimit }

func requireNameWithin(field, name string) error {
	if runes := utf8.RuneCountInString(name); runes > ContactNameLimit {
		return &ContactNameTooLongError{Field: field, Runes: runes}
	}
	return nil
}

func checkTypedContactNames(full string, first, last *string) error {
	for _, field := range []struct {
		name  string
		value *string
	}{{contactNameColumn, &full}, {columnFirstName, first}, {columnLastName, last}} {
		if field.value != nil {
			if err := requireNameWithin(field.name, *field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkChangedContactNames(current crmcontracts.Contact, in UpdateContactInput) error {
	for _, field := range []struct {
		name          string
		before, after *string
	}{{contactNameColumn, &current.FullName, in.FullName}, {columnFirstName, current.FirstName, in.FirstName}, {columnLastName, current.LastName, in.LastName}} {
		if field.after == nil || (field.before != nil && *field.before == *field.after) {
			continue
		}
		if err := requireNameWithin(field.name, *field.after); err != nil {
			return err
		}
	}
	return nil
}
