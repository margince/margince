// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestActionKeepsAHundredRecordsAndCountsThemAll(t *testing.T) {
	batch := &ActionNeeded{Reason: "privacy_reviewer_missing"}
	for range actionRecordLimit + 5 {
		batch.Records = append(batch.Records, RecordRef{Type: "approval_history", ID: ids.NewV7()})
	}
	merged, err := mergeAction(&ActionNeeded{Reason: "privacy_reviewer_missing", Count: 7}, batch)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Count != 7+actionRecordLimit+5 || len(merged.Records) != actionRecordLimit {
		t.Fatalf("count=%d records=%d", merged.Count, len(merged.Records))
	}
}

func TestABatchWithoutRecordsLeavesTheStoredActionAlone(t *testing.T) {
	stored := &ActionNeeded{Reason: "privacy_reviewer_missing", Count: 2}
	for _, batch := range []*ActionNeeded{nil, {Reason: "privacy_reviewer_missing"}} {
		if got, err := mergeAction(stored, batch); err != nil || got != stored {
			t.Fatalf("batch %+v changed the action to %+v: %v", batch, got, err)
		}
	}
}

func TestActionRefusesMalformedRecordsAndASecondReason(t *testing.T) {
	record := RecordRef{Type: "approval_history", ID: ids.NewV7()}
	cases := map[string]struct {
		stored, batch *ActionNeeded
	}{
		"no reason":     {nil, &ActionNeeded{Records: []RecordRef{record}}},
		"free text":     {nil, &ActionNeeded{Reason: "Nobody could review", Records: []RecordRef{record}}},
		"no record ID":  {nil, &ActionNeeded{Reason: "privacy_reviewer_missing", Records: []RecordRef{{Type: "approval_history"}}}},
		"no type":       {nil, &ActionNeeded{Reason: "privacy_reviewer_missing", Records: []RecordRef{{ID: record.ID}}}},
		"second reason": {&ActionNeeded{Reason: "privacy_reviewer_missing", Count: 1}, &ActionNeeded{Reason: "other", Records: []RecordRef{record}}},
	}
	for name, c := range cases {
		if _, err := mergeAction(c.stored, c.batch); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestABatchCannotLeaveMoreRecordsThanItScanned(t *testing.T) {
	record := RecordRef{Type: "approval_history", ID: ids.NewV7()}
	out := BatchOut{Scanned: 1, Done: true, Action: &ActionNeeded{Reason: "privacy_reviewer_missing", Records: []RecordRef{record, record}}}
	if err := validateBatch(Progress{BatchLimit: 4}, out); err == nil {
		t.Fatal("a batch left two records after scanning one")
	}
}

func TestARecordMetTwiceIsCountedOnce(t *testing.T) {
	record := RecordRef{Type: "approval_history", ID: ids.NewV7()}
	first, err := mergeAction(nil, &ActionNeeded{Reason: "privacy_reviewer_missing", Records: []RecordRef{record, record}})
	if err != nil {
		t.Fatal(err)
	}
	again, err := mergeAction(first, &ActionNeeded{Reason: "privacy_reviewer_missing", Records: []RecordRef{record}})
	if err != nil || again.Count != 1 || len(again.Records) != 1 {
		t.Fatalf("one record counted %+v: %v", again, err)
	}
}
