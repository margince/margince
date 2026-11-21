// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package recordexport writes a list of contacts, companies, deals or leads as a
// file the CSV import reads back. Each cell is the text the importer compares a
// row against, so an unchanged record uploads as "same".
package recordexport

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/formulacell"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The import columns that need more than the stored text of one field. The
// importer's own spellings of them are asserted equal in the compose package.
const (
	OwnerEmail = "owner_email"
	Tags       = "tags"
	Pipeline   = "pipeline"
	Stage      = "stage"
	Amount     = "amount"
	Currency   = "currency"
	CompanyRef = "company_ref"
	// EmployerName is a contact's employer; a lead's company_name is plain text.
	EmployerName = "company_name"
	LeadStatus   = "status"
	ID           = "id"
)

// ByteOrderMark is what Excel needs at the start of a file to read it as UTF-8.
var ByteOrderMark = []byte{0xEF, 0xBB, 0xBF}

// tagSeparator joins tag names in one cell, as the importer splits them.
const tagSeparator = "; "

// Spec is one export: the object, its columns in file order, and how the importer
// reads a field of a stored record.
type Spec struct {
	Object  string
	Targets []string
	// Stored is the text the importer compares a mapped cell against for one
	// field of the record's JSON.
	Stored func(record map[string]json.RawMessage, target string) string
}

// Renders reports whether the export can write a cell for the target of an
// object, so a column the importer gains cannot reach a file as an empty one.
func Renders(object, target string) bool {
	return !slices.Contains(unrenderable[object], target)
}

// unrenderable are import fields no stored record carries: the importer writes
// the author on create only, and a deal's stakeholder contact is an edge.
var unrenderable = map[string][]string{
	"contact": {"author"},
	"company": {"author"},
	"lead":    {"author"},
	"deal":    {"contact_ref"},
}

// ExportedTargets is the import fields of an object that a file carries.
func ExportedTargets(object string, importFields []string) []string {
	out := make([]string, 0, len(importFields)+1)
	for _, field := range importFields {
		if Renders(object, field) {
			out = append(out, field)
		}
	}
	return append(out, ID)
}

// Rows reads the records the caller may read, in the order given, and returns one
// row of cells per record. A record the read refuses is left out: the ids come
// from the caller's own scoped selection, so a refusal is a record that went away.
func Rows(ctx context.Context, tx pgx.Tx, spec Spec, idList []ids.UUID) ([][]string, error) {
	records := make([]map[string]json.RawMessage, 0, len(idList))
	for _, id := range idList {
		record, err := read(ctx, tx, spec.Object, id)
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			continue
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	names, err := nameLookups(ctx, tx, records)
	if err != nil {
		return nil, err
	}
	rows := make([][]string, len(records))
	for i, record := range records {
		row := make([]string, len(spec.Targets))
		for at, target := range spec.Targets {
			row[at] = formulacell.Guard(cell(spec, names, record, target))
		}
		rows[i] = row
	}
	return rows, nil
}

// CSV renders the header and rows with the byte order mark Excel needs to read
// the file as UTF-8.
func CSV(header []string, rows [][]string) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(ByteOrderMark)
	out := csv.NewWriter(&buf)
	if err := out.Write(header); err != nil {
		return nil, err
	}
	if err := out.WriteAll(rows); err != nil {
		return nil, err
	}
	return buf.Bytes(), out.Error()
}

// read is the record as the detail page reads it: the same row scope and the same
// masks, so the file holds nothing the reader could not open.
func read(ctx context.Context, tx pgx.Tx, object string, id ids.UUID) (map[string]json.RawMessage, error) {
	var record any
	var err error
	switch object {
	case "contact":
		record, err = contacts.NewStore(nil).GetContactTx(ctx, tx, ids.From[ids.ContactKind](id), storekit.LiveOnly, contacts.CustomColumns{})
	case "company":
		record, err = contacts.NewStore(nil).GetCompanyTx(ctx, tx, ids.From[ids.CompanyKind](id), storekit.LiveOnly, contacts.CustomColumns{})
	case "lead":
		record, err = contacts.NewStore(nil).GetLeadTx(ctx, tx, ids.From[ids.LeadKind](id), storekit.LiveOnly, contacts.CustomColumns{})
	case "deal":
		record, err = deals.NewStore(nil, deals.Installation{}).GetDealTx(ctx, tx, ids.From[ids.DealKind](id), storekit.LiveOnly, deals.CustomColumns{})
	default:
		return nil, fmt.Errorf("export: %q has no readable file", object)
	}
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("export: encoding a %s: %w", object, err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil, fmt.Errorf("export: reading a %s: %w", object, err)
	}
	return out, nil
}

// cell is one cell. The columns that name another record by its words are
// resolved through names; every other column is the importer's own reading.
func cell(spec Spec, names lookups, record map[string]json.RawMessage, target string) string {
	switch target {
	case OwnerEmail:
		return names.owners[uuidOf(record["owner_id"])]
	case Tags:
		return tagCell(record["tags"])
	case Pipeline:
		return names.pipelines[uuidOf(record["pipeline_id"])]
	case Stage:
		return names.stages[uuidOf(record["stage_id"])]
	case CompanyRef:
		return names.companies[uuidOf(record["company_id"])]
	case Amount:
		return amountCell(record)
	case EmployerName:
		if spec.Object == "contact" {
			return employerName(record["employer"])
		}
	case LeadStatus:
		return openLeadStatus(spec.Stored(record, target))
	case ID:
		return spec.Stored(record, ID)
	}
	return spec.Stored(record, target)
}

// openLeadStatus is the status a file may place a lead on. The importer takes
// only the open ladder, so a converted or disqualified lead leaves it empty.
func openLeadStatus(status string) string {
	if slices.Contains(contacts.OpenLeadStatuses(), status) {
		return status
	}
	return ""
}

// amountCell is the deal's amount in major units of its currency, empty when the
// reader's mask withholds the amount or the deal has none.
func amountCell(record map[string]json.RawMessage) string {
	var minor *int64
	var currency string
	if json.Unmarshal(record["amount_minor"], &minor) != nil || minor == nil {
		return ""
	}
	if json.Unmarshal(record["currency"], &currency) != nil || currency == "" {
		return ""
	}
	return values.MajorUnits(*minor, currency)
}

func tagCell(raw json.RawMessage) string {
	var tags []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &tags) != nil {
		return ""
	}
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return strings.Join(names, tagSeparator)
}

func employerName(raw json.RawMessage) string {
	var employer struct {
		CompanyName string `json:"company_name"`
	}
	if json.Unmarshal(raw, &employer) != nil {
		return ""
	}
	return employer.CompanyName
}

// uuidOf is the id a JSON field holds, the zero id for an absent or masked one.
func uuidOf(raw json.RawMessage) ids.UUID {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return ids.UUID{}
	}
	id, err := ids.Parse(text)
	if err != nil {
		return ids.UUID{}
	}
	return id
}
