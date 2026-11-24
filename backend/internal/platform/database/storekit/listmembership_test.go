// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func renderedMembership(t *testing.T, filter ListMemberFilter) string {
	t.Helper()
	if filter == nil {
		t.Fatal("no narrowing, want one that matches no row")
	}
	clause, err := filter("c.id", func(any) int { return 1 })
	if err != nil {
		t.Fatalf("rendering the narrowing: %v", err)
	}
	return clause
}

func TestAListTheCallerCannotFindNarrowsToNoRows(t *testing.T) {
	listID := ids.NewV7()
	hidden := func(context.Context, ids.UUID, string) (ListMemberFilter, error) {
		return nil, apperrors.ErrNotFound
	}
	for name, resolve := range map[string]ListMemberFilterResolver{"lists off": nil, "unknown or hidden list": hidden} {
		filter, err := ResolveListMembership(context.Background(), &listID, resolve, "contact")
		if err != nil {
			t.Fatalf("%s: %v, want a narrowing to no rows", name, err)
		}
		if clause := renderedMembership(t, filter); clause != "FALSE" {
			t.Errorf("%s: narrowed by %q, want FALSE", name, clause)
		}
	}
}

func TestAListRefusalOtherThanNotFoundStillRefuses(t *testing.T) {
	listID := ids.NewV7()
	for _, refusal := range []error{apperrors.ErrPermissionDenied, errors.New("names a list of company, not of contact")} {
		_, err := ResolveListMembership(context.Background(), &listID,
			func(context.Context, ids.UUID, string) (ListMemberFilter, error) { return nil, refusal }, "contact")
		if !errors.Is(err, refusal) {
			t.Errorf("answered %v, want the resolver's %v", err, refusal)
		}
	}
}

func TestNoListNamedNarrowsNothing(t *testing.T) {
	filter, err := ResolveListMembership(context.Background(), nil, nil, "contact")
	if err != nil || filter != nil {
		t.Fatalf("answered %v, %v, want no narrowing", filter != nil, err)
	}
}
