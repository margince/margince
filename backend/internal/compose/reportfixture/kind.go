// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package reportfixture

import (
	"context"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
	"testing"
)

func prepareVisitKind(t *testing.T) {
	t.Helper()
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `ALTER TABLE ext.`+visitTable+` ADD COLUMN owner_id uuid; CREATE INDEX ON ext.`+visitTable+`(owner_id)`); err != nil {
		t.Fatal(err)
	}
	kinds := []extension.RecordKindDeclaration{{Kind: "visit", DisplayName: "Visit", Table: visitTable, OwnerColumn: "owner_id", NaturalKey: []string{"code"}, Fields: []extension.RecordKindField{{Name: "code", Label: "Code"}, {Name: "site", Label: "Site", Reportable: true}}}}
	recordkinds.Register("reportdemo", kinds)

}
