// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"encoding/json"
	"testing"
)

func TestUpdatesInRunCountsEveryObjectClassAndRefusesAnUnreadableReport(t *testing.T) {
	t.Parallel()
	twoClasses, err := json.Marshal(Report{Objects: []ObjectReport{
		{Object: ObjectContact, Created: 3, Updated: 2},
		{Object: ObjectCompany, Created: 1, Updated: 4},
	}})
	if err != nil {
		t.Fatalf("encoding the fixture report: %v", err)
	}
	createdOnly, err := json.Marshal(Report{Objects: []ObjectReport{
		{Object: ObjectContact, Created: 5},
	}})
	if err != nil {
		t.Fatalf("encoding the fixture report: %v", err)
	}

	for _, tc := range []struct {
		name    string
		raw     []byte
		want    int
		wantErr bool
	}{
		// A run predating the field says nothing about updates, and nothing is
		// what it counts. A number guessed from the map would be the opposite
		// of the truth.
		{name: "no stored report", raw: nil},
		{name: "an empty report", raw: []byte(`{}`)},
		{name: "a run that only created", raw: createdOnly},
		{name: "every class counts", raw: twoClasses, want: 6},
		// Unreadable is an error, not a zero. Saying "nothing to say" about a
		// report that exists and cannot be decoded would tell the caller the
		// run corrected nothing.
		{name: "an unreadable report", raw: []byte(`{"objects":`), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := updatesInRun(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("updatesInRun(%s) = %d with no error, want a refusal", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("updatesInRun(%s): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("updatesInRun(%s) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}
