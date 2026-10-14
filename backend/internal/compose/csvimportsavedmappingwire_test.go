// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
)

func TestSavedMappingOfOmitsAPartThatHoldsNone(t *testing.T) {
	if got := savedMappingOf(migration.PartMapping{}); got != nil {
		t.Fatalf("savedMappingOf(empty) = %v, want nil so the wire omits it", *got)
	}
	got := savedMappingOf(migration.PartMapping{Fields: map[string]string{"Email": "email"}})
	if got == nil || (*got)["Email"] != "email" {
		t.Fatalf("savedMappingOf = %v, want the part's fields", got)
	}
}
