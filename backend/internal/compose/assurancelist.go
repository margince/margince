// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The findings one caller may see.
//
// The list goes through the deal's own row scope rather than reading
// assurance_exception directly. On a deal that predicate renders TRUE — deals
// are an IDENTITY table, workspace-readable by every seat, and what keeps one
// the owner's is the write arm rather than the read arm (auth/tableclass.go).
// So today the scope narrows nothing here.
//
// The join is what makes that a DECISION rather than an accident. A finding
// reaches a reader only if the deal it is about does, so the day a record type
// arrives scoped — or a deal stops being workspace-readable — this list narrows
// with it instead of quietly outliving the rule. Reading the exception table
// alone would leave a second answer to "who may see this deal" that nothing
// keeps in step.
//
// It lives in compose rather than in the assurance module because the clause is
// over `deal`, which the module does not own and may not import.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AssuranceExceptions reads the open findings this caller may see.
//
// The scope is applied in SQL rather than by filtering afterwards. Filtering a
// full list in Go means the rows crossed a boundary they should not have, and
// the count of what was dropped is itself a statement about how much there is.
func AssuranceExceptions(ctx context.Context, tx pgx.Tx) ([]assurance.Exception, error) {
	return assuranceExceptionsOwnedBy(ctx, tx, nil, nil, 0)
}

// AssuranceExceptionsIn reads them for one population, narrowed by the same
// rule the forecast and the reports measure that population by. A population
// the caller may not measure is refused there.
func AssuranceExceptionsIn(
	ctx context.Context, tx pgx.Tx, population assurance.Population,
) ([]assurance.Exception, error) {
	if population.Kind == "" {
		if population.ID != nil {
			return nil, fmt.Errorf("scope_id names a team or owner and needs scope_kind: %w", apperrors.ErrInvalidArgument)
		}
		return AssuranceExceptions(ctx, tx)
	}
	requested := RequestedScope{Kind: population.Kind, ID: population.ID}
	return assuranceExceptionsOwnedBy(ctx, tx, nil, &requested, 0)
}

// TeamAssuranceExceptions reads the open findings on deals the members own
// now. Ownership is the deal's live owner, never the finding's own owner_id,
// which is set when the finding is first raised and not kept in step.
func TeamAssuranceExceptions(
	ctx context.Context, tx pgx.Tx, members []ids.UUID, limit int,
) ([]assurance.Exception, error) {
	if len(members) == 0 {
		return nil, nil
	}
	return assuranceExceptionsOwnedBy(ctx, tx, members, nil, limit)
}

// assuranceExceptionsOwnedBy is both readers' body. Nil owners reads every
// finding the caller may see; a zero limit reads them all.
func assuranceExceptionsOwnedBy(
	ctx context.Context, tx pgx.Tx, owners []ids.UUID, requested *RequestedScope, limit int,
) ([]assurance.Exception, error) {
	// Both halves, and the object half first. Every finding here is ABOUT a
	// deal and carries its name, so the row scope below answers which deals
	// and never whether this caller may read deals at all — under row_scope=all
	// it answers everything. The one caller is a model tool (list_input_checks),
	// which makes the object half the difference between a scoped answer and a
	// list of deal names a seat could not open on any screen.
	//
	// No findings rather than a refusal: "nothing to check" is a real answer
	// this tool already returns, and it is the one that leaves the caller with
	// what they may see.
	if !auth.ReadGranted(ctx, tableDeal) {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	scopeClause, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
	if err != nil {
		return nil, err
	}
	if scopeClause == "" {
		scopeClause = sqlUnnarrowed
	}
	if owners != nil {
		scopeClause = fmt.Sprintf("(%s) AND d.owner_id = ANY($%d)", scopeClause, arg(owners))
	}
	if requested != nil {
		_, population, err := AnalyticsPopulationClause(ctx, tx, *requested, "d", arg, unownedIsPartOfDefault)
		if err != nil {
			return nil, err
		}
		if population != "" {
			scopeClause = "(" + scopeClause + ") AND " + population
		}
	}
	bound := ""
	if limit > 0 {
		bound = fmt.Sprintf(" LIMIT $%d", arg(limit))
	}

	// An INNER join to deal, so a finding whose subject is not a deal — or is a
	// deal that has since been archived away — does not arrive without a
	// visibility decision having been made about it. A LEFT join would let
	// exactly the rows nobody can gate through.
	sql := fmt.Sprintf(`
		SELECT `+exceptionColumns+`, d.name, d.owner_id
		FROM assurance_exception e
		JOIN deal d ON d.id = e.subject_id
		WHERE e.status = 'open'
		  AND e.subject_kind = 'deal'
		  AND d.archived_at IS NULL
		  AND %s
		ORDER BY
		  -- Material first: the manager reading this before a call has minutes,
		  -- and a high-severity finding on a large deal is what those minutes
		  -- are for.
		  CASE e.severity WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END,
		  e.affected_minor DESC NULLS LAST,
		  e.last_seen_at DESC, e.id`+bound, scopeClause)

	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the findings: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, scanLabelledException)
}

// exceptionColumns is the select list every read of a finding shares, aliased
// `e`. Two readers exist — this one and bundleableFindings, which asks a
// different question of the same rows — and the column ORDER is what scanException
// below binds to positionally, so the two have to move together or the scan
// silently reads a currency into a status.
const exceptionColumns = `e.id, e.type, e.subject_kind, e.subject_id, e.severity,
	       e.affected_minor, e.currency, e.owner_id, e.status,
	       e.claim, e.observed, e.first_seen_at, e.last_seen_at`

// scanLabelledException reads one row of exceptionColumns plus the subject's
// name and live owner, which only this reader selects.
//
// The two are appended rather than added to exceptionColumns. That list is
// shared with bundleableFindings and bound POSITIONALLY. A column inserted there
// would be read into a neighbouring field by the other scanner, silently.
func scanLabelledException(row pgx.CollectableRow) (assurance.Exception, error) {
	return scanExceptionInto(row, true)
}

// scanException reads one row of exceptionColumns.
//
// The nullable columns are read through pointers and folded in afterwards
// because assurance.Exception carries a plain string for the currency: SQL NULL
// there is "no money was named", which is a different fact from the empty
// string only in that nothing may print it as a currency.
func scanException(row pgx.CollectableRow) (assurance.Exception, error) {
	return scanExceptionInto(row, false)
}

// scanExceptionInto is both scanners' body; labelled says whether the row
// carries the trailing subject name.
func scanExceptionInto(row pgx.CollectableRow, labelled bool) (assurance.Exception, error) {
	var out assurance.Exception
	var id, subjectID ids.UUID
	var owner *ids.UUID
	var currency *string
	var firstSeen, lastSeen time.Time
	targets := []any{
		&id, &out.Type, &out.SubjectKind, &subjectID, &out.Severity,
		&out.AffectedMinor, &currency, &owner, &out.Status,
		&out.Claim, &out.Observed, &firstSeen, &lastSeen,
	}
	var dealOwner *ids.UUID
	if labelled {
		targets = append(targets, &out.SubjectLabel, &dealOwner)
	}
	err := row.Scan(targets...)
	out.ID = id
	out.SubjectID = subjectID
	out.FirstSeenAt = firstSeen
	out.LastSeenAt = lastSeen
	if currency != nil {
		out.Currency = *currency
	}
	out.OwnerID = owner
	if labelled {
		out.OwnerID = dealOwner
	}
	return out, err
}
