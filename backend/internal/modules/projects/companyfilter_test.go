// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// A project row masks company_id for a role without the company grant, so the
// list refuses that role a filter by company. The same filter with the grant
// narrows the page.
func TestTheProjectListRefusesACompanyFilterToARoleWithoutTheCompanyGrant(t *testing.T) {
	t.Parallel()
	company := ids.New[ids.CompanyKind]()
	in := ListProjectsInput{CompanyID: &company, IncludeArchived: true}

	clauses, err := filterProjects(map[string]principal.ObjectGrant{"project": {Read: true}, "company": {Read: true}}, in)
	if err != nil {
		t.Fatalf("narrowing by company_id with the company grant: %v", err)
	}
	if !slices.ContainsFunc(clauses, func(c string) bool { return strings.Contains(c, "c.company_id = $1") }) {
		t.Errorf("clauses = %q, want the company narrowing", clauses)
	}

	_, err = filterProjects(map[string]principal.ObjectGrant{"project": {Read: true}}, in)
	var refusal *values.ParseError
	if !errors.As(err, &refusal) || refusal.Code != auth.CodeFieldMasked || refusal.Field != filterCompanyID {
		t.Errorf("narrowing by company_id without the company grant → %v, want the masked-field refusal", err)
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
