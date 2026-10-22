// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The core's half of the published system reads. They run as the invocation's
// system actor, so no grant narrows them. systemRead is what makes that safe:
// they answer only on a job tick or a bus delivery, where no caller receives the
// answer. CanRead then answers under one colleague's own grants and masks.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/companyscan"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/pkg/extension"
)

//nolint:ireturn // returning the published port IS the seam: a unit holds extension.Deals, never a core type.
func (r *callRuntime) Deals() extension.Deals { return systemDeals{rt: r} }

//nolint:ireturn // the published port, as Deals.
func (r *callRuntime) Authority() extension.Authority { return systemAuthority{rt: r} }

//nolint:ireturn // the published port, as Deals.
func (r *callRuntime) Directory() extension.Directory { return systemDirectory{rt: r} }

// systemRead is the gate every system read passes. It checks the Runtime's
// lifetime and scope, refuses an invocation whose outcome a user receives, and
// refuses a read the unit's declaration does not list.
func (r *callRuntime) systemRead(ctx context.Context, read extension.SystemRead) (context.Context, error) {
	ctx, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	if !r.systemReads {
		return nil, extension.ErrAttendedRead
	}
	if !slices.Contains(declaredSystemReads(r.unit), read) {
		return nil, fmt.Errorf("%w: add extension.SystemRead %q to it", extension.ErrUndeclaredRead, read)
	}
	return ctx, nil
}

// declaredSystemReads are the reads the named unit declares in this boot's
// composition, the set its manifest publishes.
func declaredSystemReads(unit string) []extension.SystemRead {
	for _, ext := range ComposedExtensions() {
		if string(ext.Name) == unit {
			return ext.SystemReads
		}
	}
	return nil
}

// errSystemReadFailed is what a unit sees of a failed system read. The core's
// own error text would make a table or constraint name a surface it parses.
var errSystemReadFailed = errors.New("extension: the core could not answer this read")

func (r *callRuntime) systemReadFault(ctx context.Context, err error) error {
	slog.ErrorContext(ctx, "compose: an extension system read failed", "err", err, "unit", r.unit)
	return errSystemReadFailed
}

// unitID reads an id a unit passed, refusing one the driver would otherwise
// report as a database fault.
func unitID(raw, what string) (ids.UUID, error) {
	id, err := ids.Parse(raw)
	if err != nil {
		return ids.Nil, fmt.Errorf("%w: the %s id is not a canonical UUID", extension.ErrInvalid, what)
	}
	return id, nil
}

type systemDeals struct{ rt *callRuntime }

func (d systemDeals) Get(ctx context.Context, dealID string) (extension.DealSummary, error) {
	ctx, err := d.rt.systemRead(ctx, extension.SystemReadDeals)
	if err != nil {
		return extension.DealSummary{}, err
	}
	id, err := unitID(dealID, "deal")
	if err != nil {
		return extension.DealSummary{}, err
	}
	deal, err := readDealSummary(ctx, d.rt.deps.pool, id)
	if err != nil && !errors.Is(err, extension.ErrNotFound) {
		return extension.DealSummary{}, d.rt.systemReadFault(ctx, err)
	}
	return deal, err
}

func (d systemDeals) Responsible(ctx context.Context, dealID string) ([]extension.Responsible, error) {
	ctx, err := d.rt.systemRead(ctx, extension.SystemReadDeals)
	if err != nil {
		return nil, err
	}
	id, err := unitID(dealID, "deal")
	if err != nil {
		return nil, err
	}
	responsible, err := readDealResponsible(ctx, d.rt.deps.pool, id)
	if err != nil && !errors.Is(err, extension.ErrNotFound) {
		return nil, d.rt.systemReadFault(ctx, err)
	}
	return responsible, err
}

// readDealSummary reads the deal, archived or not.
func readDealSummary(ctx context.Context, pool *pgxpool.Pool, id ids.UUID) (extension.DealSummary, error) {
	out := extension.DealSummary{ID: id.String()}
	var (
		owner      *ids.UUID
		archivedAt *time.Time
	)
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT d.name, s.name, d.status, d.amount_minor, d.currency, d.expected_close_date,
			       d.owner_id, d.archived_at
			  FROM deal d
			  JOIN stage s ON s.id = d.stage_id
			 WHERE d.id = $1`, id).
			Scan(&out.Name, &out.StageName, &out.Status, &out.AmountMinor, &out.Currency, &out.ExpectedClose,
				&owner, &archivedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return extension.DealSummary{}, extension.ErrNotFound
	}
	if err != nil {
		return extension.DealSummary{}, fmt.Errorf("compose: reading deal %s for an extension: %w", id, err)
	}
	if owner != nil {
		out.OwnerID = extension.UserID(owner.String())
	}
	if out.AmountMinor != nil && out.Currency != nil {
		out.AmountDisplay = mailcopy.For(string(mailcopy.English)).Money(*out.AmountMinor, *out.Currency)
	}
	out.Archived = archivedAt != nil
	return out, nil
}

// dealResponsibleQuery names the deal's owner, its assigned users and the
// members of its assigned teams that are not archived, as live human seats.
// The owner sorts first so the bound never cuts them off.
const dealResponsibleQuery = `
	SELECT u.id, u.id IS NOT DISTINCT FROM d.owner_id
	  FROM deal d
	  JOIN app_user u ON NOT u.is_agent AND %s AND (
	       u.id = d.owner_id
	    OR u.id IN (SELECT a.user_id FROM record_assignment a
	                 WHERE a.deal_id = d.id AND a.archived_at IS NULL AND a.user_id IS NOT NULL)
	    OR u.id IN (SELECT m.user_id FROM record_assignment a
	                  JOIN team t ON t.id = a.team_id AND t.archived_at IS NULL
	                  JOIN team_membership m ON m.team_id = t.id
	                 WHERE a.deal_id = d.id AND a.archived_at IS NULL))
	 WHERE d.id = $1
	 ORDER BY 2 DESC, u.email
	 LIMIT $2`

func readDealResponsible(ctx context.Context, pool *pgxpool.Pool, id ids.UUID) ([]extension.Responsible, error) {
	var out []extension.Responsible
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM deal WHERE id = $1)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return extension.ErrNotFound
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(dealResponsibleQuery, workforce.LiveMemberSQL("u")), id, extension.MaxResponsible)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (extension.Responsible, error) {
			var (
				user  ids.UUID
				owner bool
			)
			err := row.Scan(&user, &owner)
			return extension.Responsible{UserID: extension.UserID(user.String()), Owner: owner}, err
		})
		return err
	})
	if err != nil && !errors.Is(err, extension.ErrNotFound) {
		return nil, fmt.Errorf("compose: reading who deal %s names: %w", id, err)
	}
	return out, err
}

type systemAuthority struct{ rt *callRuntime }

func (a systemAuthority) CanRead(ctx context.Context, user extension.UserID, record extension.EntityRef, fields []string) (bool, error) {
	ctx, err := a.rt.systemRead(ctx, extension.SystemReadAuthority)
	if err != nil {
		return false, err
	}
	if record.Type != fieldmask.Deal {
		return false, fmt.Errorf("%w: CanRead answers for a deal only, not a %q", extension.ErrInvalid, record.Type)
	}
	known := dealFieldNames()
	for _, field := range fields {
		if !slices.Contains(known, field) {
			return false, fmt.Errorf("%w: a deal has no field %q", extension.ErrInvalid, field)
		}
	}
	userID, err := unitID(string(user), "user")
	if err != nil {
		return false, err
	}
	dealID, err := unitID(record.ID, "deal")
	if err != nil {
		return false, err
	}
	shown, err := dealShownTo(ctx, a.rt.deps.pool, userID, dealID, fields)
	if err != nil {
		return false, a.rt.systemReadFault(ctx, err)
	}
	return shown, nil
}

// dealFieldNames are the contract's names for a deal's fields, which are also
// the names a field mask withholds.
var dealFieldNames = sync.OnceValue(func() []string {
	contract := reflect.TypeFor[crmcontracts.Deal]()
	names := make([]string, 0, contract.NumField())
	for field := range contract.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			names = append(names, name)
		}
	}
	return names
})

// dealShownTo answers whether colleague, under their own grants, may open deal
// and read every one of fields. A mask that lifts only on a row they could
// write counts as withholding, so the answer holds on any row.
func dealShownTo(ctx context.Context, pool *pgxpool.Pool, colleague, deal ids.UUID, fields []string) (bool, error) {
	// An agent seat holds grants too, and is no colleague a unit may show a deal to.
	human, err := readColleagueEmails(ctx, pool, []ids.UUID{colleague})
	if err != nil || len(human) == 0 {
		return false, err
	}
	correlation, bound := principal.CorrelationID(ctx)
	if !bound {
		correlation = ids.NewV7()
	}
	ownCtx, err := companyscan.WorkerContext(ctx, identity.NewService(pool), ids.From[ids.UserKind](colleague), correlation)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	own, bound := principal.Actor(ownCtx)
	if !bound {
		return false, errors.New("compose: the colleague's own principal was not bound")
	}
	masked := auth.MaskedFields(own, fieldmask.Deal, false)
	if slices.ContainsFunc(fields, func(f string) bool { return slices.Contains(masked, f) }) {
		return false, nil
	}
	var visible bool
	err = database.WithWorkspaceTx(ownCtx, pool, func(tx pgx.Tx) error {
		visible, err = auth.VisibleTo(ownCtx, tx, fieldmask.Deal, deal)
		return err
	})
	return visible, err
}

type systemDirectory struct{ rt *callRuntime }

func (d systemDirectory) Emails(ctx context.Context, users []extension.UserID) (map[extension.UserID]string, error) {
	ctx, err := d.rt.systemRead(ctx, extension.SystemReadDirectory)
	if err != nil {
		return nil, err
	}
	if len(users) > extension.MaxResponsible {
		return nil, fmt.Errorf("%w: %d users in one call, and the most is %d",
			extension.ErrInvalid, len(users), extension.MaxResponsible)
	}
	wanted := make([]ids.UUID, 0, len(users))
	for _, user := range users {
		id, err := unitID(string(user), "user")
		if err != nil {
			return nil, err
		}
		wanted = append(wanted, id)
	}
	emails, err := readColleagueEmails(ctx, d.rt.deps.pool, wanted)
	if err != nil {
		return nil, d.rt.systemReadFault(ctx, err)
	}
	return emails, nil
}

// readColleagueEmails answers the address of each live human seat among users.
func readColleagueEmails(ctx context.Context, pool *pgxpool.Pool, users []ids.UUID) (map[extension.UserID]string, error) {
	out := make(map[extension.UserID]string, len(users))
	if len(users) == 0 {
		return out, nil
	}
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, email FROM app_user WHERE id = ANY($1) AND NOT is_agent AND `+workforce.LiveMemberSQL(""), users)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				id    ids.UUID
				email string
			)
			if err := rows.Scan(&id, &email); err != nil {
				return err
			}
			out[extension.UserID(id.String())] = email
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("compose: reading colleagues' addresses for an extension: %w", err)
	}
	return out, nil
}

// preflightSystemReads refuses a declared read that names no group, and one
// declared twice, which the manifest would publish twice.
func preflightSystemReads(e extension.Extension) error {
	for i, read := range e.SystemReads {
		if err := read.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if slices.Contains(e.SystemReads[:i], read) {
			return fmt.Errorf("compose: extension %q declares the system read %q twice", e.Name, read)
		}
	}
	return nil
}
