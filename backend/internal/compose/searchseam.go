// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// rankedSearchDB is the handle GET /v1/search and search_records both rank on.
// server.go says why it carries the caller-predicate budget.
func rankedSearchDB(pool *pgxpool.Pool) *database.DB {
	return InstallationDB(pool).Bounded(database.CallerPredicateBudget)
}

// recordSearchSeam runs search_records on the engine GET /v1/search runs, so
// one query ranks the same records in the same order on both doors. The tool
// reads each hit back itself, so no hit decoration is bound here.
func recordSearchSeam(pool *pgxpool.Pool) agents.RecordSearch {
	store := search.NewStore(rankedSearchDB(pool))
	return func(ctx context.Context, q agents.RecordSearchQuery) (agents.RecordSearchPage, error) {
		page, err := store.Search(ctx, search.Input{
			Query: q.Text, Types: q.Types, Limit: q.Limit, Cursor: q.Cursor,
			PerType: q.PerType, WithEmployees: q.WithEmployees, NearMatches: q.NearMatches, Exclude: q.Exclude,
		})
		if err != nil {
			return agents.RecordSearchPage{}, err
		}
		out := agents.RecordSearchPage{
			Hits: make([]agents.RecordSearchHit, 0, len(page.Hits)), NextCursor: page.NextCursor,
			TypesWithMore: page.TypesWithMore,
		}
		for _, hit := range page.Hits {
			found := agents.RecordSearchHit{
				Ref:   datasource.EntityRef{Type: datasource.EntityType(hit.Type), ID: hit.ID},
				Match: agents.RecordMatch{Snippet: hit.Snippet},
			}
			if hit.WorksAt != nil {
				found.Match.WorksAt = &agents.RecordEmployer{
					CompanyID: hit.WorksAt.CompanyID, CompanyName: hit.WorksAt.CompanyName,
				}
			}
			out.Hits = append(out.Hits, found)
		}
		for _, near := range page.NearMatches {
			out.NearMatches = append(out.NearMatches, agents.RecordNearMatch{
				Ref:   datasource.EntityRef{Type: datasource.EntityType(near.Type), ID: near.ID},
				Title: near.Title, Snippet: near.Snippet,
			})
		}
		return out, nil
	}
}
