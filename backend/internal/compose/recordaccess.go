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
		dir, err := r.directory.RecordAccessDirectory(ctx, table, id, subject.ownerID)
		if err != nil {
			return err
		}
		facts := accessFacts{
			subject: subject, shares: dir.Shares, ownerTeams: dir.OwnerTeams,
			detail: dir.Management, teamNames: dir.TeamNames,
		}
		judged, teamAccess, err := r.judgeMembers(ctx, subject, dir, caller.UserID)
		if err != nil {
			return err
		}
		you, err := r.judge(ctx, subject, []principal.Principal{caller})
		if err != nil {
			return err
		}
		out, err = assembleRecordAccess(facts, judged, verdictWire(facts, caller, you[0]), cursor, limit)
		out.TeamAccessCount = teamAccess
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

// judgeMembers judges every live member and keeps the ones who can open the
// record, in roster order.
//
// A caller outside member administration is not told who belongs to which
// team, and a verdict that only a team explains would tell them: a colleague
// who can edit through a team share is, by that fact, in the team. So for that
// caller every member is judged a second time with their teams taken away, and
// the listed verdict and reasons are that one. The members whose verdict the
// teams change are counted, not named. The caller's own row is theirs to know.
func (r RecordAccessReads) judgeMembers(
	ctx context.Context, subject accessSubject, dir identity.AccessDirectory, caller ids.UUID,
) ([]judgedMember, int, error) {
	memberIDs := make([]ids.UUID, len(dir.Members))
	for i, m := range dir.Members {
		memberIDs[i] = m.ID
	}
	authorities, err := r.directory.LiveMemberAuthorities(ctx, memberIDs)
	if err != nil {
		return nil, 0, err
	}
	var (
		members    []identity.AccessMember
		principals []principal.Principal
	)
	for _, m := range dir.Members {
		if a, live := authorities[m.ID]; live {
			members = append(members, m)
			principals = append(principals, authz.HumanPrincipal(m.ID, a.RBAC, a.Seat))
		}
	}
	verdicts, err := r.judge(ctx, subject, principals)
	if err != nil {
		return nil, 0, err
	}
	shown, shownVerdicts := principals, verdicts
	if !dir.Management {
		shown = withoutTeams(principals, caller)
		if shownVerdicts, err = r.judge(ctx, subject, shown); err != nil {
			return nil, 0, err
		}
	}
	var out []judgedMember
	teamAccess := 0
	for i, m := range members {
		if shownVerdicts[i] != verdicts[i] {
			teamAccess++
		}
		if shownVerdicts[i].canRead {
			out = append(out, judgedMember{member: m, principal: shown[i], verdict: shownVerdicts[i]})
		}
	}
	return out, teamAccess, nil
}

// withoutTeams is each principal with its teams taken away, except the
// caller's own.
func withoutTeams(ps []principal.Principal, caller ids.UUID) []principal.Principal {
	out := make([]principal.Principal, len(ps))
	for i, p := range ps {
		if p.UserID != caller {
			p.TeamIDs = nil
		}
		out[i] = p
	}
	return out
}

// judge asks the two admissions for each principal, in one transaction on the
// snapshot. Only a principal who can open the record is asked about changing
// it.
func (r RecordAccessReads) judge(ctx context.Context, subject accessSubject, ps []principal.Principal) ([]accessVerdict, error) {
	out := make([]accessVerdict, len(ps))
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		reads, err := auth.AdmitReaders(ctx, tx, subject.table, subject.id, ps)
		if err != nil {
			return err
		}
		var readers []principal.Principal
		var at []int
		for i, refusal := range reads {
			if out[i].canRead, err = admissionAnswer(refusal); err != nil {
				return err
			}
			if out[i].canRead {
				readers, at = append(readers, ps[i]), append(at, i)
			}
		}
		changes, err := auth.AdmitChangers(ctx, tx, subject.table, subject.id, readers)
		if err != nil {
			return err
		}
		for k, refusal := range changes {
			if out[at[k]].canChange, err = admissionAnswer(refusal); err != nil {
				return err
			}
		}
		return nil
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

func optionalUUID(v *crmcontracts.Id) *ids.UUID {
	if v == nil {
		return nil
	}
	id := ids.UUID(*v)
	return &id
}
