// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A deal row names its pipeline and stage by name. The run reads the live
// pipelines once, and every deal row is resolved against that one read.

import (
	"context"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pipelineObject is the RBAC object of the pipeline configuration.
const pipelineObject = "pipeline"

// dealStage is one live stage, with whether a deal on it is open, won or lost.
type dealStage struct {
	id       ids.StageID
	semantic crmcontracts.StageSemantic
}

// pipelineStages is one live pipeline and its stages by folded name. names
// keeps the stages in pipeline order for a refusal to list.
type pipelineStages struct {
	id     ids.PipelineID
	name   string
	stages map[string]dealStage
	names  []string
}

// dealStages is what a run's deal rows are checked against. It holds the live
// pipelines by folded name, read only with the pipeline read grant, and today.
type dealStages struct {
	readable        bool
	byPipeline      map[string]pipelineStages
	names           []string
	defaultPipeline string
	today           time.Time
}

// stageChoice is the stage a row's cells name, and what a deal on it is.
type stageChoice struct {
	pipeline ids.PipelineID
	stage    ids.StageID
	semantic crmcontracts.StageSemantic
}

// readDealStages reads the live pipelines and today once for a run. Without
// the pipeline read grant it reads only today, and every stage is a problem.
func readDealStages(ctx context.Context, db *database.DB) (dealStages, error) {
	store := deals.NewStore(db, DealsInstallation())
	today, err := store.InstallationToday(ctx)
	if err != nil {
		return dealStages{}, err
	}
	if !auth.Allows(ctx, pipelineObject, principal.ActionRead) {
		return dealStages{today: today}, nil
	}
	pipelines, err := store.ListPipelines(ctx, storekit.LiveOnly)
	if err != nil {
		return dealStages{}, err
	}
	return dealStagesFrom(pipelines, today), nil
}

// dealStagesFrom indexes pipelines read in their order. A name two pipelines
// share, or two stages of one pipeline, resolves to the first.
func dealStagesFrom(pipelines []crmcontracts.Pipeline, today time.Time) dealStages {
	out := dealStages{readable: true, byPipeline: map[string]pipelineStages{}, today: today}
	for _, p := range pipelines {
		key := foldName(p.Name)
		if _, taken := out.byPipeline[key]; taken {
			continue
		}
		entry := pipelineStages{id: ids.From[ids.PipelineKind](ids.UUID(p.Id)), name: p.Name, stages: map[string]dealStage{}}
		for _, s := range stagesOf(p) {
			if _, taken := entry.stages[foldName(s.Name)]; taken {
				continue
			}
			entry.stages[foldName(s.Name)] = dealStage{id: ids.From[ids.StageKind](ids.UUID(s.Id)), semantic: s.Semantic}
			entry.names = append(entry.names, s.Name)
		}
		out.byPipeline[key] = entry
		out.names = append(out.names, p.Name)
		if p.IsDefault {
			out.defaultPipeline = key
		}
	}
	return out
}

func stagesOf(p crmcontracts.Pipeline) []crmcontracts.Stage {
	if p.Stages == nil {
		return nil
	}
	return *p.Stages
}

// foldName is a pipeline or stage name as a cell may spell it.
func foldName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// resolve finds the stage a row names inside the pipeline it names, or the
// default pipeline when the pipeline cell is empty.
func (d dealStages) resolve(pipeline, stage string) (stageChoice, *cellProblem) {
	if !d.readable {
		return stageChoice{}, problemOf(dealTargetStage, stage, migration.Why(migration.ReasonCannotReadPipelines))
	}
	key := foldName(pipeline)
	if key == "" {
		key = d.defaultPipeline
	}
	if key == "" {
		return stageChoice{}, problemOf(dealTargetPipeline, pipeline, migration.Why(migration.ReasonPipelineUnnamed))
	}
	p, found := d.byPipeline[key]
	if !found {
		return stageChoice{}, problemOf(dealTargetPipeline, pipeline, migration.Why(migration.ReasonPipelineUnknown,
			"value", strings.TrimSpace(pipeline), "names", strings.Join(d.names, ", ")))
	}
	s, found := p.stages[foldName(stage)]
	if !found {
		return stageChoice{}, problemOf(dealTargetStage, stage, migration.Why(migration.ReasonStageUnknown,
			"value", strings.TrimSpace(stage), "pipeline", p.name, "names", strings.Join(p.names, ", ")))
	}
	return stageChoice{pipeline: p.id, stage: s.id, semantic: s.semantic}, nil
}
