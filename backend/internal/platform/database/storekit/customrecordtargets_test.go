// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
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

// A caller with no read right on the target kind is told the record is missing.
// The answer is the same as for a record that does not exist. No transaction is used.
func TestRecordTargetsAnswerNotFoundWhenTheCallerCannotReadTheKind(t *testing.T) {
	active := []fieldcatalog.Column{{Name: "cf_contact", Type: fieldcatalog.TypeRecord, Options: []string{"contact"}}}
	values := map[string]any{"cf_contact": "0194f3a0-0000-7000-8000-000000000001"}
	err := EnsureRecordTargets(context.Background(), nil, active, values)
	var refused CustomFieldErrors
	if !errors.As(err, &refused) || len(refused) != 1 || refused[0].Code != CustomFieldTargetMissing || refused[0].Field != "cf_contact" {
		t.Fatalf("EnsureRecordTargets = %v, want one %s error on cf_contact", err, CustomFieldTargetMissing)
	}
	if _, _, _, err = InsertFragmentsIn(context.Background(), nil, active, values, nil); !errors.As(err, &refused) {
		t.Errorf("InsertFragmentsIn = %v, want the record-target refusal", err)
	}
	if err = SetCustomFieldPatchIn(context.Background(), nil, nil, active, values, nil); !errors.As(err, &refused) {
		t.Errorf("SetCustomFieldPatchIn = %v, want the record-target refusal", err)
	}
}
