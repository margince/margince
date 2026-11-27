// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// A kind an extension declared is a searchable type, but the registry is filled
// at boot, so it cannot be a row of searchBranches. Each request builds one
// branch per kind from the registry.
//
// A row has no search_tsv. Who may see it is the kind's read right and its row
// scope, as for a link. A title match makes it a text-only type like a tag: no
// vector lane, no graph, no near matches.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// extensionTypes lists every declared kind's name.
func extensionTypes() []string {
	kinds := recordkinds.All()
	names := make([]string, len(kinds))
	for i, kind := range kinds {
		names[i] = kind.Name
	}
	return names
}

// requestedTypes is the types a search covers. It is the ones the search names,
// else every core type, plus the kinds the caller may read when the search asks
// for them. A kind they may not read is left out, never refused.
func requestedTypes(ctx context.Context, in Input) []string {
	if len(in.Types) > 0 {
		return in.Types
	}
	var types []string
	for _, b := range searchBranches {
		types = append(types, b.entity)
	}
	if in.WithExtensions {
		for _, kind := range extensionTypes() {
			if knownEntity(ctx, kind) {
				types = append(types, kind)
			}
		}
	}
	return types
}

// admittedExtensionSQL builds one ranked SELECT per requested kind the caller
// may read. A title matches when it holds every word of the query, in any case.
// A title that starts with the first word ranks higher. The match is a LIKE on
// lower(title), which the trigram index VerifyRecordKinds requires serves.
//
// The core branches bind the query as headPos and tailPos (0 without a
// fragment). A statement with an unreferenced parameter fails whole.
// Every core branch may be denied, so each branch names them in an always-true test.
func admittedExtensionSQL(ctx context.Context, types []string, query string, headPos, tailPos int, arg func(any) int) ([]string, error) {
	bound := fmt.Sprintf("$%d::text IS NOT NULL", headPos)
	if tailPos != 0 {
		bound += fmt.Sprintf(" AND $%d::text IS NOT NULL", tailPos)
	}
	var branches []string
	for _, kind := range recordkinds.All() {
		if !slices.Contains(types, kind.Name) || !mayRead(ctx, kind.Name) {
			continue
		}
		scope, err := auth.ExtensionScopeClause(ctx, kind.Name, "t", arg)
		if err != nil {
			return nil, err
		}
		if scope != "" {
			scope = " AND " + scope
		}
		column := "t." + pgx.Identifier{kind.TitleColumn()}.Sanitize()
		title := "lower(" + column + ")"
		words := strings.Fields(strings.ToLower(query))
		holds := make([]string, len(words))
		for i, word := range words {
			holds[i] = fmt.Sprintf(`%s LIKE '%%' || $%d::text || '%%' ESCAPE '\'`, title, arg(storekit.EscapeLike(word)))
		}
		branches = append(branches, fmt.Sprintf(
			`SELECT $%d::text AS rtype, t.id, %s AS title, NULL::text AS snippet,
			        (CASE WHEN strpos(%s, $%d::text) = 1 THEN 1.0 ELSE 0.5 END)::float8 AS score, %s,
			        NULL::text AS identifier_match
			 FROM %s t WHERE t.archived_at IS NULL AND %s AND %s%s`,
			arg(kind.Name), column,
			title, arg(words[0]), noEmployer,
			pgx.Identifier{"ext", kind.Table}.Sanitize(), strings.Join(holds, " AND "), bound, scope))
	}
	return branches, nil
}

// excludeClause renders the ids a search leaves out over the union's output id,
// as withinClause does, or "" when there are none.
func excludeClause(exclude []ids.UUID, arg func(any) int) string {
	if len(exclude) == 0 {
		return ""
	}
	return fmt.Sprintf("NOT (id = ANY($%d))", arg(exclude))
}
