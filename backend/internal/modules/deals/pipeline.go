// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type StageInput struct {
	Name           string
	Position       int
	Semantic       string
	WinProbability int
}

type CreatePipelineInput struct {
	Name      string
	IsDefault bool
	Position  int
	Stages    []StageInput
}

// CreatePipeline creates the pipeline with its initial stages. The
// stage_terminal_prob CHECK and uq_pipeline_default index enforce the
// won=100/lost=0 and one-default rules at the database.
func (s *Store) CreatePipeline(ctx context.Context, in CreatePipelineInput) (crmcontracts.Pipeline, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionCreate); err != nil {
		return crmcontracts.Pipeline{}, err
	}
	var out crmcontracts.Pipeline
	err := s.Tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = createPipelineTx(ctx, tx, in)
		return err
	})
	return out, err
}

// createPipelineTx is the pipeline+stages write, factored out so it can run
// either in its own transaction (CreatePipeline) or inside a caller's — the
// atomic bootstrap seeds defaults in the same transaction that mints the
// workspace (C5), so a seed failure rolls the whole tenant back.
func createPipelineTx(ctx context.Context, tx pgx.Tx, in CreatePipelineInput) (crmcontracts.Pipeline, error) {
	if err := checkNewLadder(in.Stages); err != nil {
		return crmcontracts.Pipeline{}, err
	}
	id := ids.New[ids.PipelineKind]()
	_, err := tx.Exec(ctx,
		`INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, $2, $3, $4)`,
		id, in.Name, in.IsDefault, in.Position)
	if err != nil {
		if storekit.IsUniqueViolation(err) {
			return crmcontracts.Pipeline{}, apperrors.ErrConflict
		}
		return crmcontracts.Pipeline{}, fmt.Errorf("insert pipeline: %w", err)
	}

	for _, st := range in.Stages {
		if _, err := tx.Exec(ctx,
			`INSERT INTO stage (pipeline_id, name, position, semantic, win_probability)
			 VALUES ($1, $2, $3, $4, $5)`,
			id, st.Name, st.Position, st.Semantic, st.WinProbability); err != nil {
			return crmcontracts.Pipeline{}, fmt.Errorf("insert stage: %w", err)
		}
	}

	auditID, err := storekit.Audit(ctx, tx, "create", "pipeline", id.UUID, nil, map[string]any{"name": in.Name})
	if err != nil {
		return crmcontracts.Pipeline{}, fmt.Errorf("audit pipeline create: %w", err)
	}
	// events.md §5.3b: config changes are first-class facts — one
	// pipeline.created carries the whole stage set.
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, pipelineCreatedPayload(in.Name, in.IsDefault, in.Stages)); err != nil {
		return crmcontracts.Pipeline{}, fmt.Errorf("emit pipeline.created: %w", err)
	}
	out, err := readPipeline(ctx, tx, id)
	if err != nil {
		return crmcontracts.Pipeline{}, fmt.Errorf("read created pipeline: %w", err)
	}
	return out, nil
}

// pipelineCreatedPayload builds the pipeline.created wire payload from
// createPipelineTx's inputs — the ONE place that maps CreatePipeline's
// local values onto the published schema, so a future field rename shows
// up here rather than at an independently-drifting map literal.
func pipelineCreatedPayload(name string, isDefault bool, stages []StageInput) crmcontracts.PublicEventPipelineCreated {
	out := make([]crmcontracts.PublicEventPipelineCreatedStage, 0, len(stages))
	for _, st := range stages {
		out = append(out, crmcontracts.PublicEventPipelineCreatedStage{Name: st.Name, Position: st.Position, Semantic: st.Semantic})
	}
	return crmcontracts.PublicEventPipelineCreated{Name: name, IsDefault: isDefault, Stages: out}
}

func (s *Store) GetPipeline(ctx context.Context, id ids.PipelineID) (crmcontracts.Pipeline, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return crmcontracts.Pipeline{}, err
	}
	var out crmcontracts.Pipeline
	err := s.Tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = ReadPipelineTx(ctx, tx, id)
		return err
	})
	return out, err
}

// ListPipelines answers the pipeline catalog, live rows only unless the
// caller asks for the archived ones too (the contract's include_archived).
//
// No operation on this contract archives a pipeline, so the dial changes
// nothing a deployment's own endpoints can produce. It is bound rather than
// dropped because the column and this read's filter on it are what decide the
// answer, and a row in that state — from a fork's migration, from an ops
// repair — is then reachable by the parameter that names it instead of being
// invisible to every caller.
//
// Its STAGES stay live-only either way: include_archived names the rows this
// list answers about, and a stage's own archival is what listStages' copy of
// the parameter answers.
func (s *Store) ListPipelines(ctx context.Context, archived storekit.ArchivedFilter) ([]crmcontracts.Pipeline, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.Pipeline
	err := s.Tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = listPipelinesTx(ctx, tx, archived)
		return err
	})
	return out, err
}

// listPipelinesTx is the catalog read inside a caller's transaction, so a write
// that reorders the catalog answers with what it committed.
func listPipelinesTx(ctx context.Context, tx pgx.Tx, archived storekit.ArchivedFilter) ([]crmcontracts.Pipeline, error) {
	archivedFilter := ""
	if archived == storekit.LiveOnly {
		archivedFilter = liveRowsClause
	}
	return readPipelines(ctx, tx, whereSeed+archivedFilter, nil)
}

// DefaultPipeline returns the workspace's seeded default.
func (s *Store) DefaultPipeline(ctx context.Context) (crmcontracts.Pipeline, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return crmcontracts.Pipeline{}, err
	}
	var out crmcontracts.Pipeline
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		var id ids.PipelineID
		err := tx.QueryRow(ctx,
			`SELECT id FROM pipeline WHERE is_default AND archived_at IS NULL`).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		out, err = readPipeline(ctx, tx, id)
		return err
	})
	return out, err
}

// ReadPipelineTx shares the authorized live catalog read with transactional reporting.
func ReadPipelineTx(ctx context.Context, tx pgx.Tx, id ids.PipelineID) (crmcontracts.Pipeline, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return crmcontracts.Pipeline{}, err
	}
	return readPipeline(ctx, tx, id)
}

// readPipeline is the single-row read every live-only caller uses: a
// pipeline that has been archived reads as missing.
func readPipeline(ctx context.Context, tx pgx.Tx, id ids.PipelineID) (crmcontracts.Pipeline, error) {
	pipelines, err := readPipelines(ctx, tx, `id = $1`+liveRowsClause, []any{id})
	if err != nil {
		return crmcontracts.Pipeline{}, err
	}
	if len(pipelines) == 0 {
		return crmcontracts.Pipeline{}, apperrors.ErrNotFound
	}
	return pipelines[0], nil
}

// readPipelines is the one assembly of a Pipeline: the rows where admits, in
// catalog order, with their live stages read in one statement.
func readPipelines(ctx context.Context, tx pgx.Tx, where string, args []any) ([]crmcontracts.Pipeline, error) {
	// The version travels because archivePipeline takes an If-Match. A guard
	// whose value no response carries can only be satisfied by omitting it.
	rows, err := tx.Query(ctx,
		`SELECT id, name, is_default, position, created_at, updated_at, archived_at, version
		 FROM pipeline WHERE `+where+` ORDER BY position, created_at`, args...)
	if err != nil {
		return nil, fmt.Errorf("read pipelines: %w", err)
	}
	out := []crmcontracts.Pipeline{}
	var pipelineIDs []ids.UUID
	for rows.Next() {
		var p crmcontracts.Pipeline
		var pid ids.UUID
		var version crmcontracts.RowVersion
		if err := rows.Scan(&pid, &p.Name, &p.IsDefault, &p.Position, &p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt, &version); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan pipeline: %w", err)
		}
		p.Id = openapi_types.UUID(pid)
		p.Version = &version
		out = append(out, p)
		pipelineIDs = append(pipelineIDs, pid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read pipelines: %w", err)
	}
	stages, err := readStages(ctx, tx, `s.pipeline_id = ANY($1) AND s.archived_at IS NULL`, []any{pipelineIDs})
	if err != nil {
		return nil, err
	}
	byPipeline := map[openapi_types.UUID][]crmcontracts.Stage{}
	for _, stage := range stages {
		byPipeline[stage.PipelineId] = append(byPipeline[stage.PipelineId], stage)
	}
	for i := range out {
		ladder := byPipeline[out[i].Id]
		if ladder == nil {
			ladder = []crmcontracts.Stage{}
		}
		out[i].Stages = &ladder
	}
	return out, nil
}

// defaultStages is the seeded pipeline shape a fresh workspace gets.
var defaultStages = []StageInput{
	{Name: "Qualified", Position: 1, Semantic: string(DealOpen), WinProbability: 10},
	{Name: "Discovery", Position: 2, Semantic: string(DealOpen), WinProbability: 25},
	{Name: "Proposal", Position: 3, Semantic: string(DealOpen), WinProbability: 50},
	{Name: "Negotiation", Position: 4, Semantic: string(DealOpen), WinProbability: 75},
	{Name: "Won", Position: 5, Semantic: "won", WinProbability: 100},
	{Name: "Lost", Position: 6, Semantic: "lost", WinProbability: 0},
}

// SeedDefaults provisions a fresh workspace's default pipeline. Called by
// the bootstrap composition at the edge — deals owns pipeline data,
// identity owns users and sessions; neither reaches into the other.
func (s *Store) SeedDefaults(ctx context.Context) error {
	_, err := s.CreatePipeline(ctx, CreatePipelineInput{
		Name: "Sales", IsDefault: true, Position: 0, Stages: defaultStages,
	})
	return err
}

// SeedDefaultsTx provisions the default pipeline inside a transaction the
// caller owns — the bootstrap path (C5) runs it in the same transaction as
// the workspace/admin/role inserts, so the tenant and its defaults are all
// committed or all rolled back. The caller must have bound the workspace
// GUC and a system actor on ctx.
func (s *Store) SeedDefaultsTx(ctx context.Context, tx pgx.Tx) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	_, err := createPipelineTx(ctx, tx, CreatePipelineInput{
		Name: "Sales", IsDefault: true, Position: 0, Stages: defaultStages,
	})
	return err
}

// StageSeed is one configured open stage of the bootstrap pipeline
// (A107/ADR-0061: the deployment file may shape the default pipeline).
type StageSeed struct {
	Name           string
	WinProbability int
}

// SeedPipelineTx provisions a configured default pipeline in the caller's
// bootstrap transaction. The configuration names only the OPEN stages —
// the Won/Lost terminal pair is appended here because stage semantics are
// a module invariant (the won/lost tier resolution and FX freeze hang off
// them), never an operator choice. Validated with the same rules as an
// authenticated pipeline change: createPipelineTx rejects what a runtime
// create would reject.
func (s *Store) SeedPipelineTx(ctx context.Context, tx pgx.Tx, name string, open []StageSeed) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	stages := make([]StageInput, 0, len(open)+2)
	for i, st := range open {
		stages = append(stages, StageInput{
			Name: st.Name, Position: i + 1, Semantic: string(DealOpen), WinProbability: st.WinProbability,
		})
	}
	stages = append(
		stages,
		StageInput{Name: "Won", Position: len(open) + 1, Semantic: "won", WinProbability: 100},
		StageInput{Name: "Lost", Position: len(open) + 2, Semantic: "lost", WinProbability: 0},
	)
	_, err := createPipelineTx(ctx, tx, CreatePipelineInput{
		Name: name, IsDefault: true, Position: 0, Stages: stages,
	})
	return err
}

// StageSemantic resolves one live stage's semantic + pipeline — the
// admission gate's input for the advance_deal tier resolver: won/lost is
// a property of the TARGET STAGE's configuration, never of the request
// arguments (a renamed "Won" column still resolves 🟡).
func (s *Store) StageSemantic(ctx context.Context, stageID ids.UUID) (semantic string, pipelineID ids.UUID, err error) {
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT semantic, pipeline_id FROM stage WHERE id = $1 AND archived_at IS NULL`,
			stageID).Scan(&semantic, &pipelineID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return err
	})
	return semantic, pipelineID, err
}

// BirthStageTx resolves where a deal born inside the caller's transaction
// goes: the named pipeline and stage; a named stage's own pipeline; a named
// pipeline's first open stage; or the default pipeline's first open stage.
// Runs on the caller's transaction so a seam that already holds one — the
// lead qualify path — takes no second pooled connection while it waits.
// An archived or terminal target reads as missing, and a stage that is not
// in the pipeline it was named with is a missing stage too.
func BirthStageTx(ctx context.Context, tx pgx.Tx, pipelineID *ids.UUID, stageID *ids.UUID) (ids.PipelineID, ids.StageID, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return ids.PipelineID{}, ids.StageID{}, err
	}
	var pipeline, stage ids.UUID
	var err error
	switch {
	case stageID != nil:
		query := `SELECT pipeline_id, id FROM stage WHERE id = $1 AND semantic = 'open' AND archived_at IS NULL`
		args := []any{*stageID}
		if pipelineID != nil {
			query += ` AND pipeline_id = $2`
			args = append(args, *pipelineID)
		}
		err = tx.QueryRow(ctx, query, args...).Scan(&pipeline, &stage)
	case pipelineID != nil:
		err = tx.QueryRow(ctx,
			`SELECT pipeline_id, id FROM stage WHERE pipeline_id = $1 AND semantic = 'open' AND archived_at IS NULL
			 ORDER BY position, created_at LIMIT 1`, *pipelineID).Scan(&pipeline, &stage)
	default:
		err = tx.QueryRow(ctx,
			`SELECT s.pipeline_id, s.id FROM stage s JOIN pipeline p ON p.id = s.pipeline_id
			  WHERE p.is_default AND p.archived_at IS NULL AND s.semantic = 'open' AND s.archived_at IS NULL
			  ORDER BY s.position, s.created_at LIMIT 1`).Scan(&pipeline, &stage)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.PipelineID{}, ids.StageID{}, fmt.Errorf("no open stage to open the deal in: %w", apperrors.ErrNotFound)
	}
	if err != nil {
		return ids.PipelineID{}, ids.StageID{}, fmt.Errorf("resolve the deal's birth stage: %w", err)
	}
	return ids.From[ids.PipelineKind](pipeline), ids.From[ids.StageKind](stage), nil
}
