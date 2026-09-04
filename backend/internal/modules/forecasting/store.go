// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// noteBound is what a caller may put in front of a reader. A call's note says
// why the number is what it is; past this it is a document.
const noteBound = 2000

// codeInvalid is the ParseError code for a value that is the wrong shape, as
// distinct from one that is missing or out of range.
const codeInvalid = "invalid"

// The scopes a call or a snapshot can be about.
const (
	// ScopeUnset is what a caller who named no scope asked for, and it is NOT
	// the workspace. Which population an unnamed request means depends on who
	// is asking — a rep means their own — and this module cannot answer that:
	// the lens lives in the principal, and resolving it needs a read the
	// composition owns. So the omission is carried, not decided, and the seam
	// that fetches the deals resolves it.
	//
	// Spelling it as the empty string is deliberate: a zero Scope is unset, so
	// a caller who forgets to resolve gets the honest "which population?" from
	// checkScope rather than silently measuring the installation.
	ScopeUnset     = ""
	ScopeWorkspace = "workspace"
	// ScopeManagedTeams is a READ result and never a request or a write: it is
	// what a manager's omitted scope resolves to — their teams and themselves —
	// and it names no single subject a forecast could be recorded against.
	//
	// It is a kind of its own rather than being reported as the workspace,
	// because the two are different populations and a standing call is looked
	// up BY the scope. Reported as workspace, a manager's reading would fetch
	// management's own workspace call and print its amount, author and note
	// beside team-only totals — a disclosure the same manager asking for the
	// workspace explicitly would have been refused.
	ScopeManagedTeams = "managed_teams"
	ScopeTeam         = "team"
	ScopeOwner        = "owner"
)

// Store owns the forecast tables.
type Store struct {
	db *database.DB
}

// NewStore wires the store to its pool.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

// Scope names whose forecast is being read or called.
type Scope struct {
	Kind string
	// Nil exactly when Kind is ScopeWorkspace, which has no subject to name.
	ID *ids.UUID
}

// Call is one person's assertion about what will close.
type Call struct {
	ID           ids.UUID
	PeriodStart  time.Time
	PeriodEnd    time.Time
	Scope        Scope
	AmountMinor  int64
	Currency     string
	Note         string
	AuthorID     ids.UUID
	SupersedesID *ids.UUID
	CreatedAt    time.Time
}

// NewCall is what a caller supplies to record a current call.
type NewCall struct {
	Period      Period
	Scope       Scope
	AmountMinor int64
	Currency    string
	Note        string
}

// RecordCall writes a current call, superseding the scope's previous one.
//
// It writes NO deal row, and that is the point of the endpoint: calling a
// number is a statement about the pipeline, not an edit of it. A manager who
// disagrees with the derived figure records what they believe instead of
// reaching into deals to make the derivation say it.
//
// The supersession is resolved and the write happens in ONE transaction. Read
// outside it, two managers calling at the same moment both find the same
// predecessor and the chain forks — two heads, and no way to say which was
// current.
func (s *Store) RecordCall(ctx context.Context, in NewCall) (Call, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return Call{}, err
	}
	note, err := checkCall(in)
	if err != nil {
		return Call{}, err
	}
	author, err := callAuthor(ctx)
	if err != nil {
		return Call{}, err
	}

	var out Call
	err = database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		out, err = s.writeCallTx(ctx, tx, in, note, author)
		return err
	})
	if err != nil {
		return Call{}, err
	}
	return out, nil
}

// RecordCallTx is RecordCall for a caller already holding a transaction — the
// handler, which resolved the period in that same transaction so a settings
// change mid-request cannot put the read and the write in different periods.
//
// The gate and the validation run here too rather than being assumed done by
// the caller: an entry point that trusts its caller to have checked is one
// deletion away from being an ungated write.
func (s *Store) RecordCallTx(ctx context.Context, tx pgx.Tx, in NewCall) (Call, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return Call{}, err
	}
	note, err := checkCall(in)
	if err != nil {
		return Call{}, err
	}
	author, err := callAuthor(ctx)
	if err != nil {
		return Call{}, err
	}
	return s.writeCallTx(ctx, tx, in, note, author)
}

// InTx runs fn inside one workspace transaction.
//
// Gated on read even though it reads nothing itself: everything a caller can do
// through it begins with reading the forecast, and an ungated transaction
// opener on a store is a door whose lock is somebody else's responsibility to
// remember.
func (s *Store) InTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		return fn(ctx, tx)
	})
}

// IsNoStandingCall answers whether a lookup found no call, as distinct from
// having failed. "Nobody has called this period" is a real answer a reader
// renders, not an error.
func IsNoStandingCall(err error) bool { return errors.Is(err, errNoStandingCall) }

// writeCallTx is the write itself, with every input already checked.
func (s *Store) writeCallTx(
	ctx context.Context, tx pgx.Tx, in NewCall, note string, author ids.UUID,
) (Call, error) {
	var out Call
	err := func() error {
		previous, err := s.standingCallTx(ctx, tx, in.Period, in.Scope)
		switch {
		case errors.Is(err, errNoStandingCall):
			// The first call of a period supersedes nothing, which is a
			// different fact from replacing nothing and is recorded as such.
		case err != nil:
			return err
		default:
			out.SupersedesID = &previous.ID
		}
		out.PeriodStart = in.Period.StartDate
		out.PeriodEnd = in.Period.EndDate
		out.Scope = in.Scope
		out.AmountMinor = in.AmountMinor
		out.Currency = in.Currency
		out.Note = note
		out.AuthorID = author
		capturedBy, err := storekit.CapturedBy(ctx)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO forecast_call
			    (period_start, period_end, scope_kind, scope_id, amount_minor,
			     currency, note, author_id, supersedes_id, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id, created_at`,
			out.PeriodStart, out.PeriodEnd, out.Scope.Kind, out.Scope.ID,
			out.AmountMinor, out.Currency, nullIfEmpty(note), out.AuthorID,
			out.SupersedesID, capturedBy).
			Scan(&out.ID, &out.CreatedAt); err != nil {
			return fmt.Errorf("forecasting: writing the call: %w", err)
		}
		auditID, err := storekit.AuditEvent(ctx, tx, "create", "forecast_call", out.ID,
			map[string]any{
				"period_start": out.PeriodStart.Format(time.DateOnly),
				"period_end":   out.PeriodEnd.Format(time.DateOnly),
				"scope_kind":   out.Scope.Kind,
				"amount_minor": out.AmountMinor,
				"currency":     out.Currency,
			})
		if err != nil {
			return err
		}
		event := crmcontracts.PublicEventForecastCreated{
			CallId:       openapi_types.UUID(out.ID),
			AuthorUserId: openapi_types.UUID(author),
			PeriodStart:  openapi_types.Date{Time: out.PeriodStart},
			PeriodEnd:    openapi_types.Date{Time: out.PeriodEnd},
			AmountMinor:  out.AmountMinor,
			Currency:     out.Currency,
		}
		if out.SupersedesID != nil {
			superseded := openapi_types.UUID(*out.SupersedesID)
			event.SupersedesId = &superseded
		}
		return storekit.EmitEvent(ctx, tx, auditID, author, event)
	}()
	if err != nil {
		return Call{}, err
	}
	return out, nil
}

// checkCall refuses what the table's CHECKs would refuse, naming the field so
// a caller gets an answer they can act on rather than a constraint violation.
// It returns the trimmed note, which is the one input it normalizes.
func checkCall(in NewCall) (string, error) {
	// A Period carries the same window twice, and only the date half is
	// written. A caller assembling one by hand can make the two halves name
	// different quarters, and the call would then be recorded under a period
	// nobody asked for — with the instant half, which the reading used, gone.
	if !in.Period.consistent() {
		return "", &values.ParseError{
			Field: "period", Code: codeInvalid,
			Message: "the period's day bounds and instant bounds name different windows",
		}
	}
	if in.AmountMinor < 0 {
		return "", &values.ParseError{
			Field: "amount_minor", Code: "out_of_range",
			Message: "a called forecast is not a negative amount",
		}
	}
	if !values.ValidCurrency(in.Currency) {
		return "", &values.ParseError{
			Field: "currency", Code: codeInvalid,
			Message: "a called amount names the currency it is in",
		}
	}
	if err := checkScope(in.Scope); err != nil {
		return "", err
	}
	return bounded("note", in.Note, noteBound)
}

// CurrentCall answers the standing call for a period and scope, or ErrNotFound
// when nobody has made one.
func (s *Store) CurrentCall(ctx context.Context, period Period, scope Scope) (Call, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return Call{}, err
	}
	if err := checkScope(scope); err != nil {
		return Call{}, err
	}
	var out Call
	err := database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		var note *string
		err := tx.QueryRow(ctx, `
			SELECT id, period_start, period_end, scope_kind, scope_id,
			       amount_minor, currency, note, author_id, supersedes_id, created_at
			FROM forecast_call
			WHERE period_start = $1 AND period_end = $2
			  AND scope_kind = $3 AND scope_id IS NOT DISTINCT FROM $4
			ORDER BY created_at DESC, id DESC
			LIMIT 1`,
			period.StartDate, period.EndDate, scope.Kind, scope.ID).
			Scan(&out.ID, &out.PeriodStart, &out.PeriodEnd, &out.Scope.Kind,
				&out.Scope.ID, &out.AmountMinor, &out.Currency, &note,
				&out.AuthorID, &out.SupersedesID, &out.CreatedAt)
		if err != nil {
			return err
		}
		if note != nil {
			out.Note = *note
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Call{}, apperrors.ErrNotFound
		}
		return Call{}, err
	}
	return out, nil
}

// currentCallTx answers the id of the standing call, for superseding.
//
// IS NOT DISTINCT FROM rather than =, because the workspace scope's id is NULL
// and `scope_id = NULL` is never true: written with =, a workspace call would
// find no predecessor and every call would look like the first one, silently
// discarding the chain for the one scope every installation has.
func (s *Store) CurrentCallTx(ctx context.Context, tx pgx.Tx, period Period, scope Scope) (Call, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return Call{}, err
	}
	return s.standingCallTx(ctx, tx, period, scope)
}

// standingCallTx is the lookup itself. Ungated on purpose and unexported for
// exactly that reason: its two callers are CurrentCallTx, which gates read, and
// the write path, which has already required create on the same object. A
// second Require there would refuse a manager who may call but not read, which
// is not a seat this product has.
func (s *Store) standingCallTx(ctx context.Context, tx pgx.Tx, period Period, scope Scope) (Call, error) {
	var out Call
	var note *string
	// FOR UPDATE, and it is what makes the chain linear. Read without it, two
	// managers calling at the same moment both find the same predecessor, both
	// insert a call superseding it, and both commit — leaving two heads for one
	// period with nothing to say which is current. The lock serializes them, so
	// the second reads the first's call and supersedes THAT.
	err := tx.QueryRow(ctx, `
		SELECT id, period_start, period_end, scope_kind, scope_id, amount_minor,
		       currency, note, author_id, supersedes_id, created_at
		FROM forecast_call
		WHERE period_start = $1 AND period_end = $2
		  AND scope_kind = $3 AND scope_id IS NOT DISTINCT FROM $4
		ORDER BY created_at DESC, id DESC
		LIMIT 1
		FOR UPDATE`,
		period.StartDate, period.EndDate, scope.Kind, scope.ID).
		Scan(&out.ID, &out.PeriodStart, &out.PeriodEnd, &out.Scope.Kind, &out.Scope.ID,
			&out.AmountMinor, &out.Currency, &note, &out.AuthorID, &out.SupersedesID,
			&out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Call{}, errNoStandingCall
	}
	if err != nil {
		return Call{}, fmt.Errorf("forecasting: reading the standing call: %w", err)
	}
	if note != nil {
		out.Note = *note
	}
	return out, nil
}

// errNoStandingCall says this period and scope have never been called.
//
// A sentinel rather than a nil id with a nil error: the FIRST call of a period
// is a real outcome, and returning nothing twice reads exactly like a lookup
// that forgot to set its result.
var errNoStandingCall = errors.New("forecasting: no standing call for this period and scope")

// checkScope holds the rule the table's CHECK holds, so a caller gets a named
// field back rather than a constraint violation.
func checkScope(scope Scope) error {
	switch scope.Kind {
	case ScopeWorkspace:
		if scope.ID != nil {
			return &values.ParseError{
				Field: "scope_id", Code: "not_allowed",
				Message: "a workspace forecast names no subject",
			}
		}
	case ScopeTeam, ScopeOwner:
		if scope.ID == nil {
			return &values.ParseError{
				Field: "scope_id", Code: "required",
				Message: "a team or owner forecast names whose it is",
			}
		}
	default:
		return &values.ParseError{
			Field: "scope_kind", Code: codeInvalid,
			Message: "a forecast is about the workspace, a team, or one owner",
		}
	}
	return nil
}

func callAuthor(ctx context.Context) (ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		// A call is an assertion BY somebody. A system principal has nobody to
		// attribute it to, so there is no such thing as a call it could make.
		return ids.Nil, apperrors.ErrPermissionDenied
	}
	return actor.UserID, nil
}

// nullIfEmpty keeps an unwritten note out of the column as NULL rather than as
// "": an empty string claims the author wrote nothing, which is the same fact,
// but two spellings of it make every reader test for both.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// bounded trims a caller's prose and refuses what is past the limit, naming the
// field so the answer points at what to shorten.
func bounded(field, value string, limit int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > limit {
		return "", &values.ParseError{
			Field: field, Code: "too_long",
			Message: fmt.Sprintf("%s is at most %d characters", field, limit),
		}
	}
	return trimmed, nil
}
