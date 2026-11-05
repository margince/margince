// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The test run checks every grant a commit needs. A missing one is a problem
// the reader sees before approving, never a refusal halfway through the commit.
// The commit asks the same questions again and skips what the
// answer refuses.

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// tagObject is the RBAC object of the tag vocabulary.
const tagObject = "tag"

// grantNeed is one grant the commit will exercise, and what the report says
// when the importer lacks it.
type grantNeed struct {
	Object string
	Action principal.Action
	Why    migration.Reason
}

// tagGrants says which tag grants a run's commit takes.
type tagGrants struct {
	applies    bool // tag read, to apply existing tags
	createsNew bool // tag create, for names new to the CRM
}

// grantsFor lists the object grants a run's commit takes: each part's create
// and update, plus the tag grants. Applying a tag also takes the record's
// update, which each part already needs.
func grantsFor(parts []migration.PartMapping, tags tagGrants) []grantNeed {
	var needs []grantNeed
	for _, part := range parts {
		edit := cannotEditWhy(part.Object)
		if columnFor(part.Fields, csvTargetTags) != "" {
			edit = migration.Why(migration.ReasonCannotEditAndTag, "plural", pluralOf(part.Object))
		}
		needs = append(needs,
			grantNeed{Object: part.Object, Action: principal.ActionCreate, Why: cannotCreateWhy(part.Object)},
			grantNeed{Object: part.Object, Action: principal.ActionUpdate, Why: edit})
		if part.Object == migration.ObjectContact && (columnFor(part.Fields, csvEmployerName) != "" || columnFor(part.Fields, csvTargetCompanyRef) != "" || columnFor(part.Fields, fieldEmail) != "") {
			for _, need := range []grantNeed{{Object: string(recordTypeCompany), Action: principal.ActionRead}, {Object: string(recordTypeCompany), Action: principal.ActionCreate}, {Object: string(recordTypeRelationship), Action: principal.ActionCreate}} {
				need.Why = migration.Why(migration.ReasonEmployerRefused)
				needs = append(needs, need)
			}
		}
		if part.Object == migration.ObjectDeal {
			needs = append(needs, grantNeed{
				Object: pipelineObject, Action: principal.ActionRead,
				Why: migration.Why(migration.ReasonCannotReadPipelines),
			})
		}
	}
	if tags.applies {
		needs = append(needs, grantNeed{
			Object: tagObject, Action: principal.ActionRead,
			Why: migration.Why(migration.ReasonCannotReadTags),
		})
	}
	if tags.createsNew {
		needs = append(needs, grantNeed{
			Object: tagObject, Action: principal.ActionCreate,
			Why: migration.Why(migration.ReasonCannotCreateTags),
		})
	}
	return needs
}

// cannotCreateWhy is what a part's every row says when the importer may not
// create its type: the test run's warning and the commit's skip alike.
func cannotCreateWhy(object string) migration.Reason {
	return migration.Why(migration.ReasonCannotCreate, "plural", pluralOf(object))
}

// cannotEditWhy is what a row says that would change a record the importer
// may not edit.
func cannotEditWhy(object string) migration.Reason {
	return migration.Why(migration.ReasonCannotEdit, "plural", pluralOf(object))
}

// pluralOf is the type's name as its template file spells it.
func pluralOf(object string) string {
	return strings.TrimSuffix(migration.FileNameFor(object), ".csv")
}

// grantValue names a grant the way a role spells it, for the report's value.
func grantValue(object string, action principal.Action) string {
	return object + ":" + string(action)
}

// missingGrants is one permission warning per need the importer lacks.
func missingGrants(ctx context.Context, needs []grantNeed) []migration.SkippedRow {
	var problems []migration.SkippedRow
	for _, need := range needs {
		if !auth.Allows(ctx, need.Object, need.Action) {
			problems = append(problems, migration.SkippedRow{
				Kind: migration.ProblemPermission, Value: grantValue(need.Object, need.Action),
			}.Because(need.Why))
		}
	}
	return problems
}

// checkGrants is missingGrants plus one warning per owner seat the importer may
// not name. A refused seat is dropped from owners, so the commit leaves the
// importer as that row's owner instead of being refused halfway.
func checkGrants(
	ctx context.Context, pool *pgxpool.Pool, needs []grantNeed, owners *ownerResolver,
) ([]migration.SkippedRow, error) {
	problems := missingGrants(ctx, needs)
	if len(owners.seats) == 0 {
		return problems, nil
	}
	refused, err := refusedOwners(ctx, pool, owners)
	if err != nil {
		return nil, err
	}
	for _, email := range refused {
		owners.refuse(email)
		problems = append(problems, migration.SkippedRow{Kind: migration.ProblemPermission, Value: email}.Because(ownerRefusedWhy))
	}
	return problems, nil
}

// refusedOwners answers the resolved addresses whose seat the importer may not name
// as a new record's owner, asked in one read transaction.
func refusedOwners(ctx context.Context, pool *pgxpool.Pool, owners *ownerResolver) ([]string, error) {
	var refused []string
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		for _, email := range slices.Sorted(maps.Keys(owners.seats)) {
			err := auth.EnsureNewRecordOwner(ctx, tx, owners.seats[email])
			if _, notAllowed := errors.AsType[*auth.AssigneeNotAllowedError](err); notAllowed || errors.Is(err, apperrors.ErrPermissionDenied) {
				refused = append(refused, email)
				continue
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	return refused, err
}

// checkTagAndOwnerGrants is the second grant phase, once the run's new tag
// names and owner seats are known. A refused tag create leaves the new names
// unknown, so the commit neither creates nor applies them.
func (b *bundleWriters) checkTagAndOwnerGrants(ctx context.Context, pool *pgxpool.Pool) error {
	problems, err := checkGrants(ctx, pool, grantsFor(nil, tagGrants{createsNew: len(b.tags.missing) > 0}), b.owners)
	if err != nil {
		return err
	}
	tagCreate := grantValue(tagObject, principal.ActionCreate)
	if slices.ContainsFunc(problems, func(p migration.SkippedRow) bool { return p.Value == tagCreate }) {
		b.tags.missing = nil
	}
	b.grantProblems = problems
	return nil
}

// withGrantProblems puts each part's missing object grants on that part, and
// the run's tag and owner problems on its first part in import order.
func withGrantProblems(
	ctx context.Context, report migration.Report, parts []migration.PartMapping, late []migration.SkippedRow,
) migration.Report {
	first := ""
	for _, part := range parts {
		report = withWarnings(report, part.Object, missingGrants(ctx, grantsFor([]migration.PartMapping{part}, tagGrants{})))
		if first == "" || slices.Index(migration.ImportOrder, part.Object) < slices.Index(migration.ImportOrder, first) {
			first = part.Object
		}
	}
	tags := slices.ContainsFunc(parts, func(p migration.PartMapping) bool { return columnFor(p.Fields, csvTargetTags) != "" })
	return withWarnings(report, first, append(missingGrants(ctx, grantsFor(nil, tagGrants{applies: tags})), late...))
}

// createRefusal is the skip of a row whose type the importer may not create.
// Ensure and predictRow ask it first, so the test run and the commit skip the
// same rows.
func (w *csvWriters) createRefusal(ctx context.Context, row migration.Row) *migration.SkippedRow {
	if auth.Allows(ctx, w.object, principal.ActionCreate) {
		return nil
	}
	refusal := migration.SkippedRow{
		ExternalID: row.ExternalID, Line: row.Line, Kind: migration.ProblemPermission,
		Value: grantValue(w.object, principal.ActionCreate),
	}.Because(cannotCreateWhy(w.object))
	return &refusal
}
