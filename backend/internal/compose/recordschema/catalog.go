// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package recordschema owns the record vocabulary shared by query readers.
package recordschema

import (
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Objects omits relationship edges, whose visibility follows both endpoints.
var Objects = []datasource.ObjectDef{
	{Type: datasource.EntityContact, Label: "Contact", Fields: []datasource.FieldDef{
		{Name: "full_name", Type: typeText},
		{Name: OwnerID, Type: typeUUID, Nullable: true},
		{Name: "source", Type: typeText},
		{Name: createdAt, Type: typeTimestamp},
	}},
	{Type: datasource.EntityCompany, Label: "Company", Fields: []datasource.FieldDef{
		{Name: "display_name", Type: typeText},
		{Name: "legal_name", Type: typeText, Nullable: true},
		{Name: "industry", Type: typeText, Nullable: true},
		{Name: OwnerID, Type: typeUUID, Nullable: true},
		{Name: createdAt, Type: typeTimestamp},
	}},
	{Type: datasource.EntityDeal, Label: "Deal", Fields: []datasource.FieldDef{
		{Name: "name", Type: typeText},
		{Name: "amount_minor", Type: "bigint", Nullable: true},
		{Name: "currency", Type: "char(3)", Nullable: true},
		{Name: "status", Type: typeText},
		{Name: "pipeline_id", Type: typeUUID},
		{Name: "stage_id", Type: typeUUID},
		{Name: CompanyID, Type: typeUUID, Nullable: true},
		{Name: OwnerID, Type: typeUUID, Nullable: true},
		{Name: "expected_close_date", Type: typeDate, Nullable: true},
		{Name: createdAt, Type: typeTimestamp},
	}},
	{Type: datasource.EntityLead, Label: "Lead", Fields: []datasource.FieldDef{
		{Name: "full_name", Type: typeText, Nullable: true},
		{Name: "company_name", Type: typeText, Nullable: true},
		{Name: "email", Type: typeText, Nullable: true},
		{Name: "status", Type: typeText},
		{Name: OwnerID, Type: typeUUID, Nullable: true},
		{Name: createdAt, Type: typeTimestamp},
	}},
	{Type: datasource.EntityActivity, Label: "Activity", Fields: []datasource.FieldDef{
		{Name: Kind, Type: typeText},
		{Name: "subject", Type: typeText, Nullable: true},
		{Name: "direction", Type: typeText, Nullable: true},
		{Name: "is_done", Type: "boolean"},
		{Name: "occurred_at", Type: typeTimestamp},
	}},
	{Type: datasource.EntityProject, Label: "Project", Fields: []datasource.FieldDef{
		{Name: "name", Type: typeText},
		{Name: "key", Type: typeText, Nullable: true},
		{Name: CompanyID, Type: typeUUID},
		{Name: OwnerID, Type: typeUUID, Nullable: true},
		{Name: "phase", Type: typeText},
		{Name: "started_at", Type: typeDate, Nullable: true},
		{Name: "target_end_date", Type: typeDate, Nullable: true},
		{Name: "ended_at", Type: typeDate, Nullable: true},
		{Name: createdAt, Type: typeTimestamp},
	}},
}

// The column names this package's schema shapes and audit payloads reach for
// in more than one place.
const (
	typeUUID      = "uuid"
	typeTimestamp = "timestamptz"
	typeText      = "text"
	typeDate      = "date"
	createdAt     = "created_at"
	CompanyID     = "company_id"
	OwnerID       = "owner_id"
	Kind          = "kind"
)

// Fields refuses record kinds outside the shared query vocabulary.
func Fields(entity datasource.EntityType) ([]datasource.FieldDef, bool) {
	for _, obj := range Objects {
		if obj.Type == entity {
			return obj.Fields, true
		}
	}
	return nil, false
}
