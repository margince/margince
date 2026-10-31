// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// Where each rep's funnel differs most from the team's, frozen with the week.
//
// Built only from what the stage history recorded at the time of each move:
// the owner, the pipeline and the stage's meaning then. A row missing any of
// the three is left out of both sides, never handed to today's owner.

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	// Thirteen weeks is a quarter: long enough for most stages to see five
	// entries, short enough to describe how the team sells now.
	funnelWindowWeeks = 13
	// Below five entries a stage has stories, not a rate. Held on both sides.
	funnelMinimumEntries = 5
	// funnelDeviationsPerRep is how many stages a lead is shown per rep.
	funnelDeviationsPerRep = 3
)

// FunnelDeviation is one stage where a rep converts differently from the team.
// DeviationBp is the rep's rate less the team's, in basis points.
type FunnelDeviation struct {
	PipelineID    ids.UUID
	PipelineName  string
	StageID       ids.UUID
	StageName     string
	NextStageName string
	RepEntered    int
	RepConverted  int
	TeamEntered   int
	TeamConverted int
	DeviationBp   int
}

// stageEntries is one owner's entries into one stage over the window.
type stageEntries struct {
	owner      ids.UUID
	deviation  FunnelDeviation
	entered    int
	converting int
}

// teamFunnelSQL counts, per owner and stage, the deals that entered the stage
// in the window and how many of them later moved on. "Moved on" is a later move
// to a won stage, or to a later stage of the same pipeline. Stage order is read
// as configured now: that is the weekly funnel's published meaning, which the
// Analytics stage cohort does not share. Moves are read in write order. An
// imported move shows the source system's history, not the rep's selling: it
// never converts an entry, and an entry whose next move it is is left out, as
// the stage cohort leaves it out of its rate.
var teamFunnelSQL = `
	WITH entries AS (` + StageEntriesSQL("$%[1]d", "$%[2]d", "h.owner_id_at_change = ANY($%[3]d) AND (%[4]s)") + `)
	SELECT e.owner_id, e.pipeline_id, p.name, e.stage_id, s.name,
	       coalesce((SELECT n.name FROM stage n
	                  WHERE n.pipeline_id = s.pipeline_id AND n."position" > s."position"
	                  ORDER BY n."position", n.id LIMIT 1), ''),
	       count(*),
	       count(*) FILTER (WHERE EXISTS (
	         SELECT 1 FROM deal_stage_history l
	           LEFT JOIN stage ls ON ls.id = l.to_stage_id AND ls.pipeline_id = e.pipeline_id
	          WHERE l.deal_id = e.deal_id
	            AND ` + StageMoveOrder("l") + ` > ` + StageMoveOrder("eh") + ` AND l.changed_at < $%[2]d
	            AND l.provenance IS DISTINCT FROM 'imported'
	            AND l.pipeline_id_at_change = e.pipeline_id
	            AND (l.semantic_at_change = 'won' OR ls."position" > s."position")))
	  FROM entries e
	  JOIN deal_stage_history eh ON eh.id = e.id
	  JOIN stage s ON s.id = e.stage_id AND s.pipeline_id = e.pipeline_id
	  JOIN pipeline p ON p.id = e.pipeline_id
	 WHERE NOT EXISTS (
	         SELECT 1 FROM (SELECT x.provenance FROM deal_stage_history x
	                         WHERE x.deal_id = e.deal_id
	                           AND ` + StageMoveOrder("x") + ` > ` + StageMoveOrder("eh") + ` AND x.changed_at < $%[2]d
	                         ORDER BY ` + StageMoveOrder("x") + ` LIMIT 1) nx
	          WHERE nx.provenance = 'imported')
	 GROUP BY e.owner_id, e.pipeline_id, p.name, e.stage_id, s.name, s.pipeline_id, s."position"`

// measureTeamFunnel attaches each measured rep's top deviations.
//
// The team's side counts every frozen member, measured or not: a rep on leave
// still sold the deals they sold. A caller who may not read deals gets no
// funnel rather than counts over deals they cannot open.
func measureTeamFunnel(ctx context.Context, tx pgx.Tx, review *TeamReview, end time.Time) error {
	if !auth.ReadGranted(ctx, "deal") || len(review.Reps) == 0 {
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	from, to := arg(end.AddDate(0, 0, -7*funnelWindowWeeks)), arg(end)
	who := arg(rosterIDs(*review))
	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(teamFunnelSQL, from, to, who, orUnbounded(scope)), args...)
	if err != nil {
		return fmt.Errorf("weekly: reading the team's funnel: %w", err)
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (stageEntries, error) {
		var e stageEntries
		d := &e.deviation
		err := row.Scan(&e.owner, &d.PipelineID, &d.PipelineName, &d.StageID, &d.StageName,
			&d.NextStageName, &e.entered, &e.converting)
		return e, err
	})
	if err != nil {
		return fmt.Errorf("weekly: reading the team's funnel: %w", err)
	}
	for i := range review.Reps {
		review.Reps[i].Funnel = repDeviations(review.Reps[i].UserID, entries)
	}
	return nil
}

// repDeviations ranks one rep's stages by how far they sit from the team,
// largest first. Ties break on the stage id, so a re-run picks the same three.
func repDeviations(rep ids.UUID, entries []stageEntries) []FunnelDeviation {
	type totals struct{ entered, converted int }
	team := map[ids.UUID]totals{}
	for _, e := range entries {
		t := team[e.deviation.StageID]
		team[e.deviation.StageID] = totals{t.entered + e.entered, t.converted + e.converting}
	}
	var out []FunnelDeviation
	for _, e := range entries {
		t := team[e.deviation.StageID]
		if e.owner != rep || e.entered < funnelMinimumEntries || t.entered < funnelMinimumEntries {
			continue
		}
		d := e.deviation
		d.RepEntered, d.RepConverted = e.entered, e.converting
		d.TeamEntered, d.TeamConverted = t.entered, t.converted
		d.DeviationBp = e.converting*basisPointsWhole/e.entered - t.converted*basisPointsWhole/t.entered
		// A stage where the rep converts as the team does is not a deviation.
		if d.DeviationBp != 0 {
			out = append(out, d)
		}
	}
	slices.SortFunc(out, func(a, b FunnelDeviation) int {
		if c := cmp.Compare(absInt(b.DeviationBp), absInt(a.DeviationBp)); c != 0 {
			return c
		}
		return cmp.Compare(a.StageID.String(), b.StageID.String())
	})
	return out[:min(len(out), funnelDeviationsPerRep)]
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
