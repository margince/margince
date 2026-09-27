// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// "Who can see this record" for a contact or a company. Each live member is
// judged by the record's own read and edit admissions (auth.EnsureReadable,
// auth.EnsureChangeable) under that member's live authority, so the answer and
// the product cannot disagree about who gets in. It lives here because it joins
// identity's members and shares to the contacts store's record.
//
// Everything is read in one snapshot, so every member is judged against the
// same shares, roles and record. Role, team and seat changes show on the next
// read; share expiry is announced through refresh_at so a client can refetch.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

const (
	accessTableContact = "contact"
	accessTableCompany = "company"
)

// RecordAccessReads answers GET /contacts/{id}/access and
// GET /companies/{id}/access.
type RecordAccessReads struct {
	pool      *pgxpool.Pool
	directory *identity.Service
	records   *contacts.Store
}

// NewRecordAccessReads wires the read over the installation's pool.
func NewRecordAccessReads(pool *pgxpool.Pool) RecordAccessReads {
	return RecordAccessReads{
		pool:      pool,
		directory: identity.NewService(pool),
		records:   contacts.NewStore(InstallationDB(pool)),
	}
}

// accessSubject is the record as the answer needs it: who owns it and whether
// it is open to the workspace.
type accessSubject struct {
	table      string
	id         ids.UUID
	ownerID    *ids.UUID
	visibility crmcontracts.RecordAccessVisibility
	archived   bool
}

// accessVerdict is one principal's result from the two admissions.
type accessVerdict struct {
	canRead, canChange bool
}

// judgedMember is a member who can open the record, with the principal the
// reasons are read from.
type judgedMember struct {
	member    identity.AccessMember
	principal principal.Principal
	verdict   accessVerdict
}

// Read answers one page of the members who can open the record. A caller who
// cannot open it gets the read path's own 404.
func (r RecordAccessReads) Read(
	ctx context.Context, table string, id ids.UUID, cursor *string, limit *int,
) (crmcontracts.RecordAccess, error) {
	caller, ok := principal.Actor(ctx)
	if !ok {
		return crmcontracts.RecordAccess{}, errors.New("compose: a record access read with no caller bound")
	}
	var out crmcontracts.RecordAccess
	err := database.WithWorkspaceSnapshot(ctx, r.pool, func(ctx context.Context) error {
		subject, err := r.subject(ctx, table, id)
		if err != nil {
			return err
		}
		dir, err := r.directory.RecordAccessDirectory(ctx, table, id)
		if err != nil {
			return err
		}
		facts, err := r.facts(ctx, subject, dir)
		if err != nil {
			return err
		}
		judged, err := r.judgeMembers(ctx, subject, dir.Members)
		if err != nil {
			return err
		}
		you, err := r.judge(ctx, subject)
		if err != nil {
			return err
		}
		out, err = assembleRecordAccess(facts, judged, verdictWire(facts, caller, you), cursor, limit)
		return err
	})
	return out, err
}

// subject reads the record through its own read path, which is also what
// refuses a caller who cannot open it.
func (r RecordAccessReads) subject(ctx context.Context, table string, id ids.UUID) (accessSubject, error) {
	out := accessSubject{table: table, id: id}
	var (
		owner      *ids.UUID
		visibility string
		archivedAt *time.Time
	)
	switch table {
	case accessTableContact:
		c, err := r.records.GetContact(ctx, ids.From[ids.ContactKind](id), storekit.IncludeArchived)
		if err != nil {
			return accessSubject{}, err
		}
		owner, archivedAt = optionalUUID(c.OwnerId), c.ArchivedAt
		if c.Visibility != nil {
			visibility = string(*c.Visibility)
		}
	case accessTableCompany:
		c, err := r.records.GetCompany(ctx, ids.From[ids.CompanyKind](id), storekit.IncludeArchived)
		if err != nil {
			return accessSubject{}, err
		}
		owner, archivedAt = optionalUUID(c.OwnerId), c.ArchivedAt
		if c.Visibility != nil {
			visibility = string(*c.Visibility)
		}
	default:
		return accessSubject{}, apperrors.ErrNotFound
	}
	out.ownerID, out.archived = owner, archivedAt != nil
	// The column's default is workspace; only an explicit owner narrows it.
	out.visibility = crmcontracts.RecordAccessVisibilityWorkspace
	if visibility == string(crmcontracts.RecordAccessVisibilityOwner) {
		out.visibility = crmcontracts.RecordAccessVisibilityOwner
	}
	return out, nil
}

// judgeMembers judges every member and keeps the ones who can open the record.
// A member who stops resolving between the roster read and their own is
// skipped: they hold no authority.
func (r RecordAccessReads) judgeMembers(
	ctx context.Context, subject accessSubject, members []identity.AccessMember,
) ([]judgedMember, error) {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, database.ErrNoWorkspace
	}
	var out []judgedMember
	for _, m := range members {
		p, err := authz.MemberPrincipal(ctx, r.directory, ws, m.ID)
		if errors.Is(err, apperrors.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		verdict, err := r.judge(principal.WithActor(ctx, p), subject)
		if err != nil {
			return nil, err
		}
		if verdict.canRead {
			out = append(out, judgedMember{member: m, principal: p, verdict: verdict})
		}
	}
	return out, nil
}

// judge asks the two admissions for the principal on ctx.
func (r RecordAccessReads) judge(ctx context.Context, subject accessSubject) (accessVerdict, error) {
	var out accessVerdict
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		var err error
		if out.canRead, err = admissionAnswer(auth.EnsureReadable(ctx, tx, subject.table, subject.id)); err != nil || !out.canRead {
			return err
		}
		out.canChange, err = admissionAnswer(auth.EnsureChangeable(ctx, tx, subject.table, subject.id))
		return err
	})
	return out, err
}

// admissionAnswer reads an admission's refusal as "no" and anything else as a fault.
func admissionAnswer(err error) (bool, error) {
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, apperrors.ErrNotFound),
		errors.Is(err, apperrors.ErrPermissionDenied),
		errors.Is(err, apperrors.ErrSeatTierInsufficient):
		return false, nil
	default:
		return false, err
	}
}

// facts gathers what the reasons are read from. The owner's teams come from
// the owner's own authority; an owner who no longer resolves has none.
func (r RecordAccessReads) facts(
	ctx context.Context, subject accessSubject, dir identity.AccessDirectory,
) (accessFacts, error) {
	out := accessFacts{subject: subject, shares: dir.Shares, detail: dir.Management, teamNames: dir.TeamNames}
	if subject.ownerID == nil {
		return out, nil
	}
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return accessFacts{}, database.ErrNoWorkspace
	}
	owner, err := authz.MemberPrincipal(ctx, r.directory, ws, *subject.ownerID)
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		return out, nil
	case err != nil:
		return accessFacts{}, err
	}
	out.ownerTeams = owner.TeamIDs
	return out, nil
}

func optionalUUID(v *crmcontracts.Id) *ids.UUID {
	if v == nil {
		return nil
	}
	id := ids.UUID(*v)
	return &id
}
