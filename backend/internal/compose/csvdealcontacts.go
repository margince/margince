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

	"github.com/margince/margince/backend/internal/compose/importrecords"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailidentity"
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
	// A technical address names no contact, even one stored before the rule.
	if !strings.Contains(key, "@") || mailidentity.HasMachineLocalpart(key) {
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

// linkEdge is one link target of a row, with the resolver that finds the record
// it names.
type linkEdge struct {
	target, object string
	resolve        func(context.Context, string) (linkAnswer, error)
}

// dealEdges are the company a deal is with and the contact who is its stakeholder.
func (w *csvWriters) dealEdges() []linkEdge {
	return []linkEdge{{csvTargetCompanyRef, migration.ObjectCompany, w.links.resolveCompany}, {dealTargetContactRef, migration.ObjectContact, w.links.resolveContact}}
}

// projectEdges are the company a project belongs to and the deal it comes from.
func (w *csvWriters) projectEdges() []linkEdge {
	return []linkEdge{{csvTargetCompanyRef, migration.ObjectCompany, w.links.resolveCompany}, {importrecords.ProjectDealRef, migration.ObjectDeal, func(ctx context.Context, ref string) (linkAnswer, error) {
		return w.links.resolveLink(ctx, migration.ObjectDeal, ref)
	}}}
}

// edgeLinkProblemOf checks a deal's or project's links before the row lands,
// under the run's broken-link policy. A link that resolves is carried into the
// row's typed cells as the id of the record it names.
func (w *csvWriters) edgeLinkProblemOf(ctx context.Context, row migration.Row, edges []linkEdge) (rowLink, error) {
	fields := textFields(row.Fields)
	out := rowLink{links: map[string]any{}}
	for _, edge := range edges {
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
			// A record this upload creates first has no id in the test run. A
			// project's own checks are told it is coming, unless an earlier run
			// already bound that ref to a record the upload will update.
			if importrecords.Is(w.object) {
				out.links[edge.target] = importrecords.PendingLink
				bound, err := w.links.bindingOf(ctx, edge.object, ref)
				if err != nil {
					return rowLink{}, err
				}
				if bound.Found {
					out.links[edge.target] = bound.ID.String()
				}
			}
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
var linkedRecord = importrecords.LinkedRecord

// canSeatStakeholders is whether the importer may add a stakeholder to a deal,
// and read the deal's edges to find out whether one is there already.
func canSeatStakeholders(ctx context.Context) bool {
	return auth.Allows(ctx, relationshipObject, principal.ActionCreate) &&
		auth.Allows(ctx, relationshipObject, principal.ActionRead)
}
