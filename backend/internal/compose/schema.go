// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/compose/recordschema"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

var schemaObjects = recordschema.Objects

const (
	paramCompanyID = recordschema.CompanyID
	paramOwnerID   = recordschema.OwnerID
	paramKind      = recordschema.Kind
)

func schemaFields(entity datasource.EntityType) ([]datasource.FieldDef, bool) {
	return recordschema.Fields(entity)
}
