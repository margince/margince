// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importrecords

// A project row as the projects store's inputs. A project belongs to one company
// for life, and is born in a phase that is not closed. Both are written on create
// only. A row that asks to change the company is refused with its reason, not
// accepted and dropped.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/recordexport"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// The columns of projects.csv beside the owner, tags, company, description and
// the project's custom fields.
const (
	ProjectName          = "name"
	ProjectPhase         = "phase"
	ProjectDealRef       = recordexport.DealRef
	ProjectStartedAt     = "started_at"
	ProjectTargetEndDate = "target_end_date"
)

// ProjectTargets are the core fields a project file may map.
var ProjectTargets = []string{
	ProjectName, ProjectPhase, recordexport.CompanyRef, ProjectDealRef, recordexport.OwnerEmail,
	targetDescribed, ProjectStartedAt, ProjectTargetEndDate, recordexport.Tags,
}

type projectRecord struct {
	stores Stores
	cols   projects.CustomColumns
}

func (r projectRecord) Read(ctx context.Context, id ids.UUID) ([]byte, error) {
	project, err := r.stores.Projects.GetProject(ctx, ids.From[ids.ProjectKind](id), storekit.LiveOnly)
	if err != nil {
		return nil, err
	}
	return encode(project)
}

func (r projectRecord) Archived(ctx context.Context, id ids.UUID) (bool, error) {
	project, err := r.stores.Projects.GetProject(ctx, ids.From[ids.ProjectKind](id), storekit.IncludeArchived)
	return err == nil && project.ArchivedAt != nil, err
}

// CheckCells reads a project row's phase and its two dates. A phase is one a
// project may be born in, and the target end is not before the start.
func (r projectRecord) CheckCells(fields map[string]string, typed map[string]any) (map[string]any, []*CellProblem) {
	if typed == nil {
		typed = map[string]any{}
	}
	var problems []*CellProblem
	if phase := strings.ToLower(strings.TrimSpace(fields[ProjectPhase])); phase != "" {
		switch {
		case phase == projects.PhaseClosed:
			problems = append(problems, ProblemOf(ProjectPhase, fields[ProjectPhase], migration.Why(migration.ReasonProjectPhaseClosed)))
		case !slices.Contains(projects.StartPhases(), phase):
			problems = append(problems, ProblemOf(ProjectPhase, fields[ProjectPhase],
				migration.Why(migration.ReasonProjectPhaseUnknown, "value", strings.TrimSpace(fields[ProjectPhase]), "names", strings.Join(projects.StartPhases(), ", "))))
		default:
			typed[ProjectPhase] = phase
		}
	}
	for _, target := range []string{ProjectStartedAt, ProjectTargetEndDate} {
		text := strings.TrimSpace(fields[target])
		if text == "" {
			continue
		}
		date, ok := day(text)
		if !ok {
			problems = append(problems, ProblemOf(target, fields[target], migration.Why(migration.ReasonNotADate, "value", text)))
			continue
		}
		typed[target] = openapi_types.Date{Time: date}
	}
	if started, ok := typed[ProjectStartedAt].(openapi_types.Date); ok {
		if end, ok := typed[ProjectTargetEndDate].(openapi_types.Date); ok && end.Format(time.DateOnly) < started.Format(time.DateOnly) {
			problems = append(problems, ProblemOf(ProjectTargetEndDate, fields[ProjectTargetEndDate], migration.Why(migration.ReasonProjectDatesInverted)))
		}
	}
	if len(problems) > 0 {
		return nil, problems
	}
	return typed, nil
}

// RefuseNew refuses a row with no name, and a row naming no company the caller
// may link: every project belongs to one.
func (r projectRecord) RefuseNew(fields map[string]string, typed map[string]any) migration.Reason {
	if Text(fields, ProjectName) == nil {
		return migration.Why(migration.ReasonProjectUnnamed)
	}
	if _, linked := LinkedRecord(typed, targetCompanyRef); !linked && !pending(typed, targetCompanyRef) {
		return migration.Why(migration.ReasonProjectCompanyMissing)
	}
	return migration.Reason{}
}

// birthOf is what a row asks the projects store to birth. The test run and the
// commit both build it here.
func (r projectRecord) birthOf(fields map[string]string, typed map[string]any, sourceSystem string) projects.CreateProjectInput {
	in := projects.CreateProjectInput{
		Source:        string(provenance.RecordSourceImport),
		SourceSystem:  &sourceSystem,
		OwnerID:       OwnerFrom(fields, typed),
		Description:   Text(fields, targetDescribed),
		StartedAt:     dayFrom(fields, typed, ProjectStartedAt),
		TargetEndDate: dayFrom(fields, typed, ProjectTargetEndDate),
		CustomFields:  CustomFieldsFrom(fields, typed),
	}
	if name := Text(fields, ProjectName); name != nil {
		in.Name = *name
	}
	if phase, ok := typed[ProjectPhase].(string); ok {
		in.Phase = phase
	}
	if company, ok := LinkedRecord(typed, targetCompanyRef); ok {
		in.CompanyID = ids.From[ids.CompanyKind](company)
	}
	if deal, ok := LinkedRecord(typed, ProjectDealRef); ok {
		in.DealID = new(ids.From[ids.DealKind](deal))
	}
	return in
}

// PredictCreate asks the projects store what it would refuse. A company or deal
// that comes from an earlier file of the upload has no id yet. The store skips
// the checks that need it, and a company still to be created matches no deal that
// already names one.
func (r projectRecord) PredictCreate(ctx context.Context, fields map[string]string, typed map[string]any) (migration.Reason, error) {
	in := r.birthOf(fields, typed, "")
	err := database.WithWorkspaceTx(ctx, r.stores.Pool, func(tx pgx.Tx) error {
		return r.stores.Projects.RefuseImportCreate(ctx, tx, in, r.cols)
	})
	if why, refused := r.Refusal(err, fields); refused {
		return why, nil
	}
	return migration.Reason{}, err
}

func (r projectRecord) Create(ctx context.Context, land Landing, row migration.Row, typed map[string]any, sourceSystem string) (migration.EnsureResult, error) {
	fields := TextFields(row.Fields)
	in := r.birthOf(fields, typed, sourceSystem)
	err := land(ctx, row, func(tx pgx.Tx) (ids.UUID, error) {
		project, err := r.stores.Projects.CreateProjectTx(ctx, tx, in, r.cols)
		if err != nil {
			return ids.UUID{}, fmt.Errorf("import: creating project %s: %w", row.ExternalID, err)
		}
		return ids.UUID(project.Id), nil
	})
	if why, refused := r.Refusal(err, fields); refused {
		return SkippedFor(why), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}

// updateFrom builds the update for the cells that differ from the stored
// project.
func (r projectRecord) updateFrom(changed map[string]string, typed map[string]any) projects.UpdateProjectInput {
	return projects.UpdateProjectInput{
		Name:          Text(changed, ProjectName),
		OwnerID:       OwnerFrom(changed, typed),
		Description:   Text(changed, targetDescribed),
		StartedAt:     dayFrom(changed, typed, ProjectStartedAt),
		TargetEndDate: dayFrom(changed, typed, ProjectTargetEndDate),
		CustomFields:  CustomFieldsFrom(changed, typed),
	}
}

// RefuseChange refuses a row that names another company than the one the
// project holds, and one whose dates leave the target end before the start.
func (r projectRecord) RefuseChange(current []byte, changed map[string]string, typed map[string]any) migration.Reason {
	var stored crmcontracts.Project
	if err := json.Unmarshal(current, &stored); err != nil {
		return migration.Why(migration.ReasonValueUnreadable, "value", "project")
	}
	if company, named := LinkedRecord(typed, targetCompanyRef); named && stored.CompanyId != nil && ids.UUID(*stored.CompanyId) != company {
		return migration.Why(migration.ReasonProjectCompanyFixed)
	}
	why, _ := r.Refusal(projects.RefuseUpdate(stored, r.updateFrom(changed, typed)), nil)
	return why
}

func (r projectRecord) Update(ctx context.Context, id ids.UUID, changed map[string]string, typed map[string]any, _ []byte, ifVersion *int64) error {
	in := r.updateFrom(changed, typed)
	in.IfVersion = ifVersion
	_, err := r.stores.Projects.UpdateProject(ctx, ids.From[ids.ProjectKind](id), in)
	return err
}

func (r projectRecord) Undo(ctx context.Context, id ids.UUID, importedAt time.Time) error {
	_, err := r.stores.Projects.ArchiveProject(ctx, ids.From[ids.ProjectKind](id), nil, storekit.NotTouchedByHumanSince(importedAt))
	return err
}

// Refusal answers the projects store's refusals of one row as the reason the
// report shows.
func (r projectRecord) Refusal(err error, fields map[string]string) (migration.Reason, bool) {
	if err == nil {
		return migration.Reason{}, false
	}
	if _, dates := errors.AsType[*projects.ProjectDateRangeError](err); dates {
		return migration.Why(migration.ReasonProjectDatesInverted), true
	}
	if _, phase := errors.AsType[*projects.ProjectStartPhaseError](err); phase {
		return migration.Why(migration.ReasonProjectPhaseClosed), true
	}
	if linkable, refused := errors.AsType[*projects.DealNotLinkableError](err); refused {
		return migration.Why(migration.ReasonProjectDealRefused, "detail", linkable.Message), true
	}
	if mismatch, refused := errors.AsType[*projects.DealCompanyMismatchError](err); refused {
		return migration.Why(migration.ReasonProjectDealRefused, "detail", mismatch.Error()), true
	}
	return storeFieldRefusal(err, fields, nil)
}

// dayFrom is the checked day of a date cell, nil when the row maps none.
func dayFrom(fields map[string]string, typed map[string]any, target string) *time.Time {
	if _, given := fields[target]; !given {
		return nil
	}
	if date, ok := typed[target].(openapi_types.Date); ok {
		return &date.Time
	}
	return nil
}

func (r projectRecord) Lock(ctx context.Context, tx pgx.Tx, id ids.UUID) (string, error) {
	return migration.ObjectProject, r.stores.Projects.LockProjectLive(ctx, tx, ids.From[ids.ProjectKind](id))
}

// ProjectPreview is the project a row would create, as the store admits it for
// the user who approves the run. An agent's import waits on that review.
func ProjectPreview(ctx context.Context, store *projects.Store, cols Columns, fields map[string]string, typed map[string]any, sourceSystem string, creator principal.Principal) (projects.CreateProjectInput, error) {
	return store.PreviewProjectCreationForActor(ctx, projectRecord{cols: cols.projects}.birthOf(fields, typed, sourceSystem), creator)
}

// ProjectDealKey is what the run's claim on a deal is keyed by. It is the deal
// the row's link resolved to, so a ref and an id naming one deal are one claim.
// A deal the upload creates is keyed by the cell that names it.
func ProjectDealKey(fields map[string]string, typed map[string]any) string {
	if deal, linked := LinkedRecord(typed, ProjectDealRef); linked {
		return deal.String()
	}
	if pending(typed, ProjectDealRef) {
		return "pending:" + strings.TrimSpace(fields[ProjectDealRef])
	}
	return ""
}
