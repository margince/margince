// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// Which changed fields a reader is shown, and how each value is read.
//
// An audit image holds every column a write touched, including internal state.
// A record type a reader edits shows only fields the reader can name. Those are
// its update request's fields, custom fields, declared stored columns, and
// hand-built audit keys the History tab labels. A new column stays hidden, and
// backend/gates/magicfieldcensus_test.go fails until someone classifies it.

import (
	"reflect"
	"slices"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// fieldReading says how a reader reads one field's stored value. A zero kind
// is a value with no printable form, such as an address object.
type fieldReading struct {
	kind      crmcontracts.MagicFieldChangeType
	precision crmcontracts.MagicFieldChangePrecision
}

// recordUpdateRequests are the contract bodies a reader edits each record type
// with. Their properties are the fields a reader already knows by name.
var recordUpdateRequests = map[string]reflect.Type{
	typeContact:  reflect.TypeFor[crmcontracts.UpdateContactRequest](),
	typeCompany:  reflect.TypeFor[crmcontracts.UpdateCompanyRequest](),
	typeDeal:     reflect.TypeFor[crmcontracts.UpdateDealRequest](),
	typeLead:     reflect.TypeFor[crmcontracts.UpdateLeadRequest](),
	typeProject:  reflect.TypeFor[crmcontracts.UpdateProjectRequest](),
	typeActivity: reflect.TypeFor[crmcontracts.UpdateActivityRequest](),
}

// storedColumns are columns no update request names that a reader still
// understands. A deal changes stage through its own route. A probability figure
// is stored apart from who set it. A lead's profile link is set on capture.
var storedColumns = map[string]map[string]fieldReading{
	typeDeal: {
		"stage_id":                 {kind: crmcontracts.MagicFieldChangeTypeStage},
		"win_probability_override": {kind: crmcontracts.MagicFieldChangeTypeNumber},
	},
	typeLead: {"linkedin_url": {kind: crmcontracts.MagicFieldChangeTypeText}},
}

// memberColumns name a seat. Their values are labelled with the member's name.
var memberColumns = map[string]bool{"owner_id": true, "assignee_id": true}

// presentedFields is each governed type's readable fields, derived once.
var presentedFields = derivePresentedFields()

func derivePresentedFields() map[string]map[string]fieldReading {
	out := make(map[string]map[string]fieldReading, len(recordUpdateRequests))
	for entityType, request := range recordUpdateRequests {
		fields := map[string]fieldReading{}
		for field := range request.Fields() {
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			fields[name] = readingOf(name, field.Type)
		}
		for name, reading := range storedColumns[entityType] {
			fields[name] = reading
		}
		out[entityType] = fields
	}
	return out
}

var (
	dateType        = reflect.TypeFor[openapi_types.Date]()
	instant         = reflect.TypeFor[time.Time]()
	uuidType        = reflect.TypeFor[openapi_types.UUID]()
	plainText       = reflect.TypeFor[string]()
	contractPackage = reflect.TypeFor[crmcontracts.MagicFieldChange]().PkgPath()
)

// readingOf reads a request property's Go type. An enum is a string type the
// contract package defines; any other named string, such as an email, is text.
func readingOf(name string, t reflect.Type) fieldReading {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case strings.HasSuffix(name, values.MinorAmountSuffix) && t.Kind() == reflect.Int64:
		return fieldReading{kind: crmcontracts.MagicFieldChangeTypeMoney}
	case t == dateType:
		return fieldReading{kind: crmcontracts.MagicFieldChangeTypeDate, precision: crmcontracts.MagicFieldChangePrecisionDay}
	case t == instant:
		return fieldReading{kind: crmcontracts.MagicFieldChangeTypeDate, precision: crmcontracts.MagicFieldChangePrecisionInstant}
	case t == uuidType && memberColumns[name]:
		return fieldReading{kind: crmcontracts.MagicFieldChangeTypeMember}
	case t == uuidType:
		return fieldReading{kind: crmcontracts.MagicFieldChangeTypeReference}
	case t.Kind() == reflect.Bool:
		return fieldReading{kind: crmcontracts.MagicFieldChangeTypeBoolean}
	case t.Kind() >= reflect.Int && t.Kind() <= reflect.Int64:
		return fieldReading{kind: crmcontracts.MagicFieldChangeTypeNumber}
	case t.Kind() == reflect.String && t != plainText && t.PkgPath() == contractPackage:
		return fieldReading{kind: crmcontracts.MagicFieldChangeTypeEnum}
	case t.Kind() == reflect.String:
		return fieldReading{kind: crmcontracts.MagicFieldChangeTypeText}
	}
	return fieldReading{}
}

// presentedField answers whether a reader is shown a change to key on a record
// of entityType. A type no update request governs keeps the deny-list.
func presentedField(entityType, key string) bool {
	if bookkeepingFields[key] || evidenceKeys[key] {
		return false
	}
	fields, governed := presentedFields[entityType]
	if !governed {
		return true
	}
	if _, readable := fields[key]; readable || strings.HasPrefix(key, datasource.CustomFieldPrefix) {
		return true
	}
	return syntheticAuditFields[key] && !DeclaredInternal(entityType, key)
}

// readingFor is how a presented field's value is read; zero when the server
// declares no reading, as for a custom field.
func readingFor(entityType, key string) fieldReading {
	return presentedFields[entityType][key]
}

// PresentsField is presentedField for the census gate.
func PresentsField(entityType, key string) bool { return presentedField(entityType, key) }

// ReadingOf names the kind a presented field is read as, "" for none. The
// census gate checks it against the catalog's foreign keys.
func ReadingOf(entityType, key string) string { return string(readingFor(entityType, key).kind) }

// GovernedRecordTypes are the record types the allowlist applies to, each also
// its table's name.
func GovernedRecordTypes() []string {
	out := make([]string, 0, len(recordUpdateRequests))
	for entityType := range recordUpdateRequests {
		out = append(out, entityType)
	}
	slices.Sort(out)
	return out
}

// DeclaredInternal answers whether a column is classified as internal, so it
// is never shown.
func DeclaredInternal(entityType, column string) bool {
	return internalColumns[entityType][column] || sharedInternalColumns[column] ||
		bookkeepingFields[column] || evidenceKeys[column]
}

// InternalColumnDeclarations is the per-type internal classification, for the
// census to find a declaration that names no column.
func InternalColumnDeclarations() map[string][]string {
	out := map[string][]string{}
	for entityType, columns := range internalColumns {
		for column := range columns {
			out[entityType] = append(out[entityType], column)
		}
	}
	return out
}

// SyntheticAuditFields are the hand-built audit keys the History tab labels,
// for the gate that holds them equal to the frontend's map.
func SyntheticAuditFields() []string {
	out := make([]string, 0, len(syntheticAuditFields))
	for key := range syntheticAuditFields {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}
