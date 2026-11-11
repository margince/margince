// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Which accounts a member's imported network reaches (ADR-0078 §2.1b).
//
// This is the ONE question the ghosts exist to answer, and until now nothing
// asked it. A member could see that 5,064 connections had been imported and
// that 267 of them resolved to accounts, and could not see WHICH accounts —
// so the import produced a number rather than an answer.
//
// It is also the half of the feature that works on a one-contact workspace. The
// company page's connections card asks which COLLEAGUE knows an account, which
// has no content when there is one member; this asks whether the member's own
// network reaches it, which has content immediately.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LinkedInReachAccount is one account this member's network reaches.
type LinkedInReachAccount struct {
	CompanyID   ids.UUID
	DisplayName string
	Connections int
	// ContactsOnFile counts the confirmed matches only. The gap between it and
	// Connections is connections at this account with no confirmed contact; a
	// suggested or unmatched one may still be on file.
	ContactsOnFile int
}

// LinkedInReach is the whole answer, including what it cannot show.
type LinkedInReach struct {
	Accounts []LinkedInReachAccount
	// AccountsTotal counts every account reached, not just the page returned.
	// A truncated list read as the whole network would understate reach, which
	// is the one thing this view exists to state.
	AccountsTotal int
	// UnresolvedConnections is how many connections matched no account at all.
	// Reported because it is the honest size of what this view cannot show,
	// and because it is the number that shrinks as accounts are created.
	UnresolvedConnections int
	Page                  storekit.Page
}

// reachCursor is where a page of the ranking stopped, in all three parts of
// its order. A count alone is no position: many accounts share one, and a walk
// resumed on it skips the rest of the tie.
type reachCursor struct {
	Connections int      `json:"connections"`
	Name        string   `json:"name"`
	ID          ids.UUID `json:"id"`
}

// decodeReachCursor refuses a token that names no position in this ranking.
// Every listed account has at least one connection. A cursor minted by another
// route decodes cleanly here, and its zero count is the tell. No cursor is the
// zero position, the top of the ranking.
func decodeReachCursor(cursor string) (reachCursor, error) {
	if cursor == "" {
		return reachCursor{}, nil
	}
	after, err := storekit.DecodeOpaque[reachCursor](cursor)
	if err != nil || after.Connections < 1 || after.ID.IsZero() {
		return reachCursor{}, &storekit.MalformedCursorError{}
	}
	return after, nil
}

// MyLinkedInReach reads the caller's own network, grouped by account.
//
// Company row scope applies to the LIST, and a connection parked on an
// account the caller may not read is counted as UNRESOLVED rather than dropped.
//
// That direction is load-bearing. The two numbers are differenceable: a
// connection in neither the visible accounts nor the unresolved total would
// itself say "this employer resolved to something you are not allowed to see",
// which is an account-enumeration oracle a member drives by uploading one row
// per guessed company name. Counting it as unresolved makes it indistinguishable
// from a company nobody here has on file, which is the same answer every
// row-scoped list gives.
func (s *Store) MyLinkedInReach(ctx context.Context, limit *int, cursor string) (LinkedInReach, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return LinkedInReach{}, apperrors.ErrPermissionDenied
	}
	// The payload names companies, so it takes the company read grant
	// — a member who may not read accounts must not learn which ones exist by
	// asking about their own address book.
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return LinkedInReach{}, err
	}
	capped := storekit.ClampLimit(limit)
	after, err := decodeReachCursor(cursor)
	if err != nil {
		return LinkedInReach{}, err
	}

	var out LinkedInReach
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := s.readReachAccounts(ctx, tx, actor.UserID, capped, after, &out); err != nil {
			return err
		}
		return s.countUnresolved(ctx, tx, actor.UserID, &out)
	})
	if err != nil {
		return LinkedInReach{}, err
	}
	return out, nil
}

// countUnresolved counts the caller's connections that reach no account they
// can see — whether because the matcher placed none, or because the one it
// placed is outside their row scope.
func (s *Store) countUnresolved(ctx context.Context, tx pgx.Tx, owner ids.UUID, out *LinkedInReach) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	ownerPos := arg(owner)
	scope, err := auth.ScopeClauseFor(ctx, "company", "o", arg)
	if err != nil {
		return err
	}
	visible := sqlAlwaysVisible
	if scope != "" {
		visible = scope
	}
	return tx.QueryRow(ctx, storekit.SQLf(`
		SELECT count(*)
		  FROM linkedin_connection c
		 WHERE c.owner_user_id = $%d AND c.tombstoned_at IS NULL
		   AND NOT EXISTS (
		       SELECT 1 FROM company o
		        WHERE o.id = c.matched_company_id AND o.archived_at IS NULL AND (%s))`,
		ownerPos, visible), args...).Scan(&out.UnresolvedConnections)
}

// readReachAccounts reads one page of the ranking. It fetches one row past the
// page, which tells a full page from the last one without a second query.
func (s *Store) readReachAccounts(
	ctx context.Context, tx pgx.Tx, owner ids.UUID, limit int, after reachCursor, out *LinkedInReach,
) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	ownerPos := arg(owner)
	scope, err := auth.ScopeClauseFor(ctx, "company", "o", arg)
	if err != nil {
		return err
	}
	visible := sqlAlwaysVisible
	if scope != "" {
		visible = scope
	}
	// The keyset sits outside the CTE, so the total still counts every account
	// reached. Negating the count lets one row comparison spell the mixed
	// directions of the ORDER BY.
	resume := ""
	if !after.ID.IsZero() {
		resume = storekit.SQLf("WHERE (-connections, display_name, id) > ($%d, $%d, $%d)",
			arg(-after.Connections), arg(after.Name), arg(after.ID))
	}
	// The total rides the SAME statement as the rows. Two statements would each
	// take their own snapshot, and an import between them could report a page
	// longer than its total. The total is the join's outer side, which a page
	// past the last account still carries.
	query := storekit.SQLf(`
		WITH reached AS (
		    SELECT o.id, o.display_name,
		           count(*) AS connections,
		           count(*) FILTER (WHERE c.match_status = 'confirmed') AS on_file
		      FROM linkedin_connection c
		      JOIN company o ON o.id = c.matched_company_id AND o.archived_at IS NULL
		     WHERE c.owner_user_id = $%d AND c.tombstoned_at IS NULL AND (%s)
		     GROUP BY o.id, o.display_name
		)
		SELECT p.id, p.display_name, p.connections, p.on_file, t.total
		  FROM (SELECT count(*) AS total FROM reached) t
		  LEFT JOIN LATERAL (
		       SELECT id, display_name, connections, on_file FROM reached
		        %s
		        ORDER BY connections DESC, display_name, id
		        LIMIT $%d) p ON true
		 ORDER BY p.connections DESC, p.display_name, p.id`, ownerPos, visible, resume, arg(limit+1))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("contacts: reading which accounts a network reaches: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id                  *ids.UUID
			name                *string
			connections, onFile *int
		)
		if err := rows.Scan(&id, &name, &connections, &onFile, &out.AccountsTotal); err != nil {
			return err
		}
		// The one row of an empty page carries the total and no account.
		if id == nil || name == nil || connections == nil || onFile == nil {
			continue
		}
		out.Accounts = append(out.Accounts, LinkedInReachAccount{
			CompanyID: *id, DisplayName: *name, Connections: *connections, ContactsOnFile: *onFile,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return out.cutPage(limit)
}

// cutPage drops the row read past the page and mints the cursor to resume at.
func (r *LinkedInReach) cutPage(limit int) error {
	if len(r.Accounts) <= limit {
		return nil
	}
	r.Accounts = r.Accounts[:limit]
	last := r.Accounts[limit-1]
	token, err := storekit.EncodeOpaque(reachCursor{
		Connections: last.Connections, Name: last.DisplayName, ID: last.CompanyID,
	})
	if err != nil {
		return err
	}
	r.Page = storekit.Page{HasMore: true, NextCursor: token}
	return nil
}
