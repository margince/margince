// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A filter by company discloses project-company pairs, so the list refuses it,
// with one permission error, to a role without the company or relationship
// grant. With both grants it narrows the page.
func TestTheProjectListRefusesACompanyFilterToARoleWithoutTheCompanyGrant(t *testing.T) {
	t.Parallel()
	company := ids.New[ids.CompanyKind]()
	in := ListProjectsInput{CompanyID: &company, IncludeArchived: true}

	clauses, err := filterProjects(map[string]principal.ObjectGrant{
		"project": {Read: true}, "company": {Read: true}, "relationship": {Read: true},
	}, in)
	if err != nil {
		t.Fatalf("narrowing by company_id with the company and relationship grants: %v", err)
	}
	if !slices.ContainsFunc(clauses, func(c string) bool { return strings.Contains(c, "project_edge.company_id = $") }) {
		t.Errorf("clauses = %q, want the company narrowing", clauses)
	}

	// A match discloses the project-company pair, which relationship.read
	// governs.
	_, err = filterProjects(map[string]principal.ObjectGrant{"project": {Read: true}, "company": {Read: true}}, in)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("narrowing by company_id without the relationship grant → %v, want permission denied", err)
	}

	_, err = filterProjects(map[string]principal.ObjectGrant{"project": {Read: true}, "relationship": {Read: true}}, in)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("narrowing by company_id without the company grant → %v, want permission denied", err)
	}
}

// filterProjects renders the list's filters for a seat holding these grants.
func filterProjects(grants map[string]principal.ObjectGrant, in ListProjectsInput) ([]string, error) {
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test", UserID: ids.NewV7(),
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeOwn},
	})
	var args []any
	return appendProjectFilters(ctx, nil, in, func(v any) int { args = append(args, v); return len(args) })
}
