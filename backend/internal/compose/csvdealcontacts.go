// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The two links a deal row can carry: the company the deal is with, and the
// contact who is its stakeholder. Both are resolved before the row lands, by
// the run's one resolver, so the dry run and the commit find the same record.

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// relationshipObject is the RBAC object of the edges between records.
const relationshipObject = "relationship"

// resolveContact answers which contact a link cell names: by its ref, then by
// its email address, then by its id.
func (r *linkResolver) resolveContact(ctx context.Context, raw string) (linkAnswer, error) {
	return r.resolveLink(ctx, migration.ObjectContact, raw)
}

// visibleContactByEmail finds the live contact holding an address inside the
// caller's row scope. An address is unique among live contacts, so one row is
// the whole answer.
func (r *linkResolver) visibleContactByEmail(ctx context.Context, key string) (linkAnswer, error) {
	if !strings.Contains(key, "@") {
		return linkAnswer{}, nil
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return linkAnswer{}, err
	}
	var answer linkAnswer
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		emailPos := arg(canonicalFor(fieldEmail, key))
		scope, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
		if err != nil {
			return err
		}
		if scope == "" {
			scope = sqlUnnarrowed
		}
		err = tx.QueryRow(ctx, storekit.SQLf(`
			SELECT c.id FROM contact_email e JOIN contact c ON c.id = e.contact_id
			 WHERE e.email = $%d AND e.archived_at IS NULL
			   AND c.archived_at IS NULL AND c.merged_into_id IS NULL AND %s
			 LIMIT 1`, emailPos, scope), args...).Scan(&answer.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		answer.Found = err == nil
		return err
	})
	if err != nil {
		return linkAnswer{}, fmt.Errorf("import: finding the linked contact: %w", err)
	}
	return answer, nil
}

// dealEdges are the link targets of a deal row, each with the resolver that
// finds the record it names.
func (w *csvWriters) dealEdges() []struct {
	target  string
	resolve func(context.Context, string) (linkAnswer, error)
} {
	return []struct {
		target  string
		resolve func(context.Context, string) (linkAnswer, error)
	}{
		{csvTargetCompanyRef, w.links.resolveCompany},
		{dealTargetContactRef, w.links.resolveContact},
	}
}

// dealLinkProblemOf checks a deal row's company and contact before the row
// lands, under the run's broken-link policy. A link that resolves is carried
// into the row's typed cells as the id of the record it names.
func (w *csvWriters) dealLinkProblemOf(ctx context.Context, row migration.Row) (rowLink, error) {
	fields := textFields(row.Fields)
	out := rowLink{links: map[string]any{}}
	for _, edge := range w.dealEdges() {
		ref := strings.TrimSpace(fields[edge.target])
		column := columnFor(w.columns, edge.target)
		if column == "" || ref == "" {
			continue
		}
		if edge.target == dealTargetContactRef && !canSeatStakeholders(ctx) {
			broken := w.brokenLink(row, column, ref, migration.Why(migration.ReasonCannotLinkStakeholders))
			if broken.skip {
				return broken, nil
			}
			out.problem = cmp.Or(out.problem, broken.problem)
			continue
		}
		answer, err := edge.resolve(ctx, ref)
		if err != nil {
			return rowLink{}, err
		}
		switch {
		case answer.Found:
			out.links[edge.target] = answer.ID.String()
		case answer.Pending:
		default:
			broken := w.brokenLink(row, column, ref, answer.Why)
			if broken.skip {
				return broken, nil
			}
			out.problem = cmp.Or(out.problem, broken.problem)
		}
	}
	return out, nil
}

// into lays the row's resolved links over its typed cells.
func (l rowLink) into(typed map[string]any) map[string]any {
	if len(l.links) == 0 {
		return typed
	}
	if typed == nil {
		typed = map[string]any{}
	}
	maps.Copy(typed, l.links)
	return typed
}

// linkedRecord is the id a row's link cell resolved to, from its typed cells.
func linkedRecord(typed map[string]any, target string) (ids.UUID, bool) {
	text, ok := typed[target].(string)
	if !ok {
		return ids.UUID{}, false
	}
	id, err := ids.Parse(text)
	return id, err == nil
}

// canSeatStakeholders is whether the importer may add a stakeholder to a deal,
// and read the deal's edges to find out whether one is there already.
func canSeatStakeholders(ctx context.Context) bool {
	return auth.Allows(ctx, relationshipObject, principal.ActionCreate) &&
		auth.Allows(ctx, relationshipObject, principal.ActionRead)
}
