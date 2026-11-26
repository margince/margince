// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A merge archives its source as a retirement does. The merged row names the
// word that took its records, and it cannot be restored.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestAMergedTagNamesItsSurvivorAndRefusesARestore(t *testing.T) {
	e := setupMergePin(t)
	if _, err := e.store.MergeTags(e.Admin(), e.source, e.target, e.targetVersion); err != nil {
		t.Fatalf("merging: %v", err)
	}

	merged, _, err := e.store.GetTag(e.Admin(), e.source)
	if err != nil {
		t.Fatalf("reading the merged tag: %v", err)
	}
	if merged.MergedIntoID == nil || *merged.MergedIntoID != e.target {
		t.Fatalf("the merged tag names %v as its survivor, want %v", merged.MergedIntoID, e.target)
	}
	if merged.MergedIntoName == nil || *merged.MergedIntoName != "SkewTarget" {
		t.Errorf("the merged tag names the survivor %v, want SkewTarget", merged.MergedIntoName)
	}

	// The survivor's name is read where the merged row is, so a rename shows there.
	e.rename(t, "Renamed")
	listed, _, err := e.store.ListTags(e.Admin(), storekit.IncludeArchived)
	if err != nil {
		t.Fatalf("listing the vocabulary: %v", err)
	}
	var seen bool
	for _, row := range listed {
		if row.ID == e.source {
			seen = true
			if row.MergedIntoName == nil || *row.MergedIntoName != "Renamed" {
				t.Errorf("the listed merged tag names %v, want the survivor's new name", row.MergedIntoName)
			}
		}
	}
	if !seen {
		t.Fatal("the merged tag is missing from the list that includes archived words")
	}

	if _, err := e.store.RestoreTag(e.Admin(), e.source); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("restoring a merged tag answered %v, want a conflict", err)
	}
	after, _, err := e.store.GetTag(e.Admin(), e.source)
	if err != nil {
		t.Fatalf("reading the merged tag again: %v", err)
	}
	if after.ArchivedAt == nil {
		t.Error("the refused restore brought the merged tag back")
	}
}

// A word retired by hand names no survivor and still comes back.
func TestARetiredTagNamesNoSurvivorAndRestores(t *testing.T) {
	e := setupMergePin(t)
	retired, err := e.store.ArchiveTag(e.Admin(), e.source)
	if err != nil {
		t.Fatalf("retiring: %v", err)
	}
	if retired.MergedIntoID != nil {
		t.Errorf("a retired tag names %v as its survivor", *retired.MergedIntoID)
	}
	restored, err := e.store.RestoreTag(e.Admin(), e.source)
	if err != nil {
		t.Fatalf("restoring a retired tag: %v", err)
	}
	if restored.ArchivedAt != nil {
		t.Error("the restored tag is still archived")
	}
}
