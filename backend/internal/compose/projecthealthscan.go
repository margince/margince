// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The project health job: once a day, each in-flight project's quiet and
// overdue state becomes a reading in its health history, written as the AI and
// marked so, and its owner is told what was set and why.
//
// It judges from two rules that already exist and spells neither again: the
// quiet rule of projects/quiet.go, and an open task past its due date as the
// project page's commitments section counts it. Whether a reading may land at
// all is the projects store's call, made under the project lock.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// projectHealthActor is the principal every reading the job writes names in
// captured_by.
const projectHealthActor = "agent:project-health"

// projectHealthNameBound is how much of a project's name the notice subject
// quotes, well inside the store's 200-character subject.
const projectHealthNameBound = 120

// ProjectHealthScanArgs runs one fleet-wide project health pass.
type ProjectHealthScanArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ProjectHealthScanArgs) Kind() string { return "project_health_scan" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide).
func (ProjectHealthScanArgs) FleetWide() {}

type projectHealthWorker struct {
	pool *pgxpool.Pool
	scan *ProjectHealthScan
	log  *slog.Logger
}

func (w *projectHealthWorker) Work(ctx context.Context, _ *river.Job[ProjectHealthScanArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.scanWorkspace))
}

func (w *projectHealthWorker) scanWorkspace(ctx context.Context, workspace ids.UUID) error {
	pass, err := w.scan.RunWorkspace(ctx, workspace)
	// Logged on every pass: considered is what tells a calm day from a broken
	// read, which written alone cannot.
	w.log.InfoContext(ctx, "project health pass", "workspace", workspace,
		"considered", pass.Considered, "written", pass.Written,
		"held_by_user", pass.HeldByUser, "failed", pass.Failed)
	return err
}

// addProjectHealthJobs registers the job and hands back its schedule, whose
// cadence is api/jobs.yaml's.
func addProjectHealthJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	addDeclaredWorker[ProjectHealthScanArgs](reg, &projectHealthWorker{
		pool: pool, scan: NewProjectHealthScan(pool, time.Now), log: log,
	})
	return periodicFor(cfg, ProjectHealthScanArgs{})
}

// ProjectHealthScan judges one workspace's in-flight projects.
type ProjectHealthScan struct {
	pool       *pgxpool.Pool
	projects   *projects.Store
	activities *activities.Store
	notices    *notices.Store
	now        func() time.Time
}

// NewProjectHealthScan builds the pass over the installation's pool.
func NewProjectHealthScan(pool *pgxpool.Pool, now func() time.Time) *ProjectHealthScan {
	db := InstallationDB(pool)
	return &ProjectHealthScan{
		pool: pool, projects: projects.NewStore(db).SetClock(now),
		activities: activities.NewStore(db), notices: notices.NewStore(db), now: now,
	}
}

// ProjectHealthPass counts what one workspace's pass did.
type ProjectHealthPass struct {
	Considered int
	Written    int
	HeldByUser int
	Failed     int
}

// healthCandidate is one in-flight project and what the quiet rule says of it.
type healthCandidate struct {
	ProjectID ids.ProjectID
	Name      string
	OwnerID   *ids.UUID
	Quiet     bool
}

// RunWorkspace judges every in-flight project of one workspace, each in a
// transaction of its own, so one project's failure leaves the others written.
func (s *ProjectHealthScan) RunWorkspace(ctx context.Context, workspace ids.UUID) (ProjectHealthPass, error) {
	wsCtx := principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), projectHealthActor)
	now := s.now()
	var candidates []healthCandidate
	if err := database.WithWorkspaceTx(wsCtx, s.pool, func(tx pgx.Tx) error {
		var err error
		candidates, err = scanHealthCandidates(wsCtx, tx, now)
		return err
	}); err != nil {
		return ProjectHealthPass{}, err
	}
	pass := ProjectHealthPass{Considered: len(candidates)}
	var failures []error
	for _, candidate := range candidates {
		outcome, err := s.judge(wsCtx, candidate, now)
		if err != nil {
			pass.Failed++
			failures = append(failures, fmt.Errorf("project %s: %w", candidate.ProjectID, err))
			continue
		}
		switch outcome {
		case projects.AIHealthWritten:
			pass.Written++
		case projects.AIHealthHeldByUser:
			pass.HeldByUser++
		case projects.AIHealthUnchanged:
		}
	}
	return pass, errors.Join(failures...)
}

func scanHealthCandidates(ctx context.Context, tx pgx.Tx, now time.Time) ([]healthCandidate, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.name, p.owner_id, `+projects.ProjectQuietSQL("p", "$1", 2)+`
		  FROM project p
		 WHERE p.archived_at IS NULL
		   AND `+projects.ProjectInFlightSQL("p")+`
		 ORDER BY p.id`,
		now, projects.DefaultProjectQuietDays)
	if err != nil {
		return nil, fmt.Errorf("scan projects for a health reading: %w", err)
	}
	defer rows.Close()
	var out []healthCandidate
	for rows.Next() {
		var found healthCandidate
		var id ids.UUID
		if err := rows.Scan(&id, &found.Name, &found.OwnerID, &found.Quiet); err != nil {
			return nil, err
		}
		found.ProjectID = ids.From[ids.ProjectKind](id)
		out = append(out, found)
	}
	return out, rows.Err()
}

// judge writes one project's reading and, when it landed and the project has an
// owner, the owner's notice in the same transaction. A project with no owner
// gets the reading and no notice: nobody else is told in their place.
func (s *ProjectHealthScan) judge(ctx context.Context, candidate healthCandidate, now time.Time) (projects.AIHealthOutcome, error) {
	var outcome projects.AIHealthOutcome
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		overdue, err := s.hasOverdueTask(ctx, tx, candidate.ProjectID, now)
		if err != nil {
			return err
		}
		finding := projectHealthFinding{Quiet: candidate.Quiet, Overdue: overdue}
		words := projectHealthCopyFor(identity.BaseLanguageForRecord(ctx, tx))
		reason := finding.reason(words)
		written, result, err := s.projects.RecordAIHealthTx(ctx, tx, projects.RecordAIHealthInput{
			ProjectID: candidate.ProjectID, State: finding.state(), Note: reason,
		})
		outcome = result
		if err != nil || result != projects.AIHealthWritten || candidate.OwnerID == nil {
			return err
		}
		_, err = s.notices.CreateTx(ctx, tx, notices.NewNotice{
			Recipient: ids.From[ids.UserKind](*candidate.OwnerID),
			Kind:      notices.KindProjectHealthAI,
			// The name is cut, not the subject: the store caps the subject, and
			// a long name would push the state word off its end.
			Subject: fmt.Sprintf(words.noticeSubject,
				textcut.Runes(candidate.Name, projectHealthNameBound), words.state(finding.state())),
			Body:      reason + " " + words.noticeUndo,
			DedupeKey: fmt.Sprintf("%s:%s", notices.KindProjectHealthAI, written.Id),
			Target:    notices.Target{Type: string(datasource.RecordProject), ID: candidate.ProjectID.UUID},
		})
		return err
	})
	return outcome, err
}

// hasOverdueTask answers whether an open task filed under the project is past
// its due date. Tasks the product minted for itself are left out, because a
// reminder nobody promised is no commitment that slipped. The sweep is ordered
// soonest due first, undated last, so the first task alone answers.
func (s *ProjectHealthScan) hasOverdueTask(ctx context.Context, tx pgx.Tx, id ids.ProjectID, now time.Time) (bool, error) {
	entityType := string(datasource.RecordProject)
	tasks, _, err := s.activities.ListOpenTasksTx(ctx, tx, activities.ListOpenTasksInput{
		AsOf: now, EntityType: &entityType, EntityID: &id.UUID, Limit: 1, ExcludeSystemMinted: true,
	})
	if err != nil {
		return false, err
	}
	return len(tasks) > 0 && deadline.Passed(tasks[0].DueAt, now), nil
}

// projectHealthFinding is what the two rules say of one project.
type projectHealthFinding struct {
	Quiet   bool
	Overdue bool
}

// state maps the finding onto a health state: both rules give off_track, one
// gives at_risk, neither gives on_track. Whether on_track is written at all is
// the store's call, because only the job's own alarm is taken back by it.
func (f projectHealthFinding) state() crmcontracts.ProjectHealthState {
	switch {
	case f.Quiet && f.Overdue:
		return crmcontracts.ProjectHealthStateOffTrack
	case f.Quiet || f.Overdue:
		return crmcontracts.ProjectHealthStateAtRisk
	default:
		return crmcontracts.ProjectHealthStateOnTrack
	}
}

// reason is the one sentence the reading carries as its note.
func (f projectHealthFinding) reason(words projectHealthCopy) string {
	switch {
	case f.Quiet && f.Overdue:
		return fmt.Sprintf(words.quietAndOverdue, projects.DefaultProjectQuietDays)
	case f.Quiet:
		return fmt.Sprintf(words.quiet, projects.DefaultProjectQuietDays)
	case f.Overdue:
		return words.overdue
	default:
		return words.active
	}
}
