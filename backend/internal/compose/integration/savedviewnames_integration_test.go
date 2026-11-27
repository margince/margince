// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSavedViewNamesAreUniqueAcrossConcurrentSavesAndRenames(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, collectionsPerms())
	in := collections.CreateSavedViewInput{Resource: "contacts", Name: "Quarter focus", Query: map[string]any{}}
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() { <-start; _, err := store.CreateSavedView(rep, in); results <- err }()
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		fault, ok := errors.AsType[apperrors.MessageFault](err)
		if !ok || !errors.Is(err, apperrors.ErrConflict) {
			t.Fatalf("save: %v", err)
		}
		if code, _ := fault.MessageFault(); code != "view_name_taken" {
			t.Fatalf("code = %s", code)
		}
		conflicts++
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	in.Name = "Other"
	other, err := store.CreateSavedView(rep, in)
	if err != nil {
		t.Fatal(err)
	}
	name := " quarter FOCUS "
	if _, err = store.UpdateSavedView(rep, other.ID, collections.UpdateSavedViewInput{Name: &name}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("rename = %v", err)
	}
	fault, ok := errors.AsType[apperrors.MessageFault](err)
	if !ok {
		t.Fatalf("rename has no code: %v", err)
	}
	if code, _ := fault.MessageFault(); code != "view_name_taken" {
		t.Fatalf("rename code = %s", code)
	}
	in.Name = "Quarter focus"
	in.Resource = "companies"
	if _, err = store.CreateSavedView(rep, in); err != nil {
		t.Fatalf("different resource: %v", err)
	}
	in.Resource = "contacts"
	otherOwner := e.As(e.Rep3, []ids.UUID{e.Team2}, collectionsPerms())
	if _, err = store.CreateSavedView(otherOwner, in); err != nil {
		t.Fatalf("different owner: %v", err)
	}
}
