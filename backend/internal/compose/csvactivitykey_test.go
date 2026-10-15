// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// An activity has no natural key, so a file that maps no ref column is refused
// when it is staged. The row number is never the identity.
func TestAnActivitiesFileWithoutARefColumnIsRefusedAtStaging(t *testing.T) {
	_, err := partMappingFrom(migration.ObjectActivity,
		map[string]string{"Subject": activityTargetSubject, "Type": activityTargetType}, nil, importCatalog{})
	var refused *httperr.DetailedError
	if !errors.As(err, &refused) || len(refused.Fields) != 1 ||
		refused.Fields[0].Field != "source_key" || refused.Fields[0].Code != "unmappable" {
		t.Fatalf("a file with no ref column = %v, want the unmappable source key refusal", err)
	}
	mapped, err := partMappingFrom(migration.ObjectActivity,
		map[string]string{"Row": csvTargetRef, "Subject": activityTargetSubject}, nil, importCatalog{})
	if err != nil || mapped.SourceKey != "Row" {
		t.Fatalf("a file with a ref column = %+v, %v, want it keyed on that column", mapped, err)
	}
}
