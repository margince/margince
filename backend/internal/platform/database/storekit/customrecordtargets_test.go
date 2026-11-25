// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// What the probe leaves to CustomBindValues never reaches the database, so a nil
// transaction proves the probe was not asked.
func TestRecordTargetsSkipWhatIsNotAWellFormedRecordValue(t *testing.T) {
	active := []fieldcatalog.Column{
		{Name: "cf_visit", Type: fieldcatalog.TypeRecord, Options: []string{"ext:alpha:visit"}},
		{Name: "cf_note", Type: fieldcatalog.TypeText},
		{Name: "cf_loose", Type: fieldcatalog.TypeRecord},
	}
	values := map[string]any{
		"cf_visit": "not-a-uuid",
		"cf_note":  "0194f3a0-0000-7000-8000-000000000001",
		"cf_loose": "0194f3a0-0000-7000-8000-000000000001",
	}
	if err := EnsureRecordTargets(context.Background(), nil, active, values); err != nil {
		t.Fatalf("EnsureRecordTargets = %v, want nil for values it leaves to the shape check", err)
	}
	if err := EnsureRecordTargets(context.Background(), nil, active, map[string]any{"cf_visit": nil}); err != nil {
		t.Fatalf("a cleared record field: %v", err)
	}
}
