// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The export bundle's row-visibility clauses (B-E11.10a): each member
// applies the very same predicate its list endpoint uses, so the export
// can never hand a caller a row their lists would hide. Every clause here
// composes the platform auth predicates (auth.ScopeClauseFor /
// ActivityContentClause / VisiblePredicate) — scope policy has exactly one
// spelling (ADR-0054 §8); the writer only routes each member to the
// matching one.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// memberScope renders the row-visibility predicate for one member,
// dispatching to the same clause its list endpoint uses. An empty clause
// means unbounded (row_scope=all, or the system actor) or workspace-wide
// reference data.
func memberScope(ctx context.Context, m exportMember, alias string, arg func(any) int) (string, error) {
	switch m.scope {
	case scopeShareable:
		return auth.ScopeClauseFor(ctx, m.table, alias, arg)
	case scopeActivity:
		return auth.ActivityContentClause(ctx, alias, arg)
	case scopeRelationship:
		return relationshipExportScope(ctx, alias, arg)
	case scopeAttachment:
		return polymorphicVisible(ctx, alias+".entity_type", alias+".entity_id", arg)
	case scopeAudit:
		return auditExportScope(ctx, alias, arg)
	case scopeWorkspace:
		return "", nil
	case scopePersonChild:
		return personChildExportScope(ctx, alias, arg)
	case scopeMirror:
		return mirrorExportScope(ctx, alias+".object_class", alias+".external_id", arg)
	case scopeMirrorAssoc:
		from, err := mirrorExportScope(ctx, alias+".from_type", alias+".from_id", arg)
		if err != nil {
			return "", err
		}
		to, err := mirrorExportScope(ctx, alias+".to_type", alias+".to_id", arg)
		if err != nil {
			return "", err
		}
		if from == "" || to == "" {
			// An unbounded actor scopes neither endpoint; there is no
			// predicate to conjoin (and " AND " alone is a syntax error).
			return "", nil
		}
		return from + " AND " + to, nil
	default:
		return "", fmt.Errorf("export: unknown scope mode %d for %q", m.scope, m.table)
	}
}

// mirrorExportScope is the mirror_visibility deny-join as an export
// predicate (ADR-0044: can_see=false or no entry hides the row —
// fail-closed, so an unmapped row-scoped exporter gets zero mirror
// rows, the same answer their overlay lists give).
//
// An unbounded actor (admin/ops, row_scope=all) reads the WHOLE estate,
// exactly as every other scope in this file treats them. That is
// load-bearing for the pre-flip export: the flip migrates the whole
// estate, so a bundle scoped to one operator's owner-projection would
// reconstruct only their slice — the OVA-AC-6(d) reversibility promise
// would be nominal.
func mirrorExportScope(ctx context.Context, classCol, idCol string, arg func(any) int) (string, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return "", errors.New("compose: no actor bound to export context")
	}
	if auth.Unbounded(actor) {
		return "", nil
	}
	return fmt.Sprintf(
		`EXISTS (SELECT 1 FROM mirror_visibility mv
		 WHERE mv.object_class = %s AND mv.external_id = %s
		   AND mv.mirror_user_id = $%d AND mv.can_see)`,
		classCol, idCol, arg(actor.UserID),
	), nil
}

// personChildExportScope scopes a person child row by its parent
// person's visibility — the child discloses nothing its parent read
// would not.
func personChildExportScope(ctx context.Context, alias string, arg func(any) int) (string, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return "", errors.New("compose: no actor bound to export context")
	}
	if auth.UnboundedFor(actor, "person") {
		return "", nil
	}
	predicate := auth.VisiblePredicate(actor, "person", arg)
	return fmt.Sprintf(
		`EXISTS (SELECT 1 FROM person pp WHERE pp.id = %s.person_id AND pp.archived_at IS NULL AND %s)`,
		alias, predicate("pp"),
	), nil
}

// relationshipExportScope mirrors the relationship list's
// endpoint-visibility rule: every non-null endpoint must be visible, so
// an edge never discloses a record on the far side the caller cannot see.
func relationshipExportScope(ctx context.Context, alias string, arg func(any) int) (string, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return "", errors.New("compose: no actor bound to export context")
	}
	if auth.UnboundedFor(actor, "person", "organization", "deal", "project") {
		return "", nil
	}
	// Every endpoint column the table HAS, not the ones an author remembered.
	// A relationship kind anchored on a column missing from this list exports
	// without its far side being tested, which is the disclosure the comment
	// above claims cannot happen. gates/edgeendpointcensus_test.go holds this
	// list against the table's own shape constraints.
	var clauses []string
	//nolint:goconst // these are TABLE names reaching a SQL predicate; the constant goconst
	// names is agentRecordType, a different vocabulary that spells the same word, and binding
	// this list to it would assert a correspondence that does not hold
	for _, endpoint := range []struct{ column, table string }{
		{"person_id", "person"},
		{"counterparty_person_id", "person"},
		{"organization_id", "organization"},
		{"counterparty_org_id", "organization"},
		{"deal_id", "deal"},
		{"project_id", "project"},
	} {
		predicate := auth.VisiblePredicate(actor, endpoint.table, arg)
		clauses = append(clauses, fmt.Sprintf(
			`(%[1]s.%[2]s IS NULL OR EXISTS (
			   SELECT 1 FROM %[3]s ep WHERE ep.id = %[1]s.%[2]s AND ep.archived_at IS NULL AND %[4]s))`,
			alias, endpoint.column, endpoint.table, predicate("ep"),
		))
	}
	return "(" + strings.Join(clauses, " AND ") + ")", nil
}

// polymorphicVisible renders "the referenced record is visible" for a
// polymorphic (entity_type, entity_id) pair — the attachment manifest and
// the audit-log member both ride it so neither leaks a row pointing at a
// record outside the caller's scope.
//
// An actor unbounded over every RECORD arm is spared the record walk and is
// NOT spared the activity arm. Row scope and audience are different
// obligations: row scope says which records an admin may reach, and an
// admin reaches all of them; audience says who may read one message's
// content, and it does not yield to row_scope=all (auth.ActivityContentClause
// — an admin reading a colleague's limited mail is the disclosure the limit
// exists to prevent). An attachment's filename and an audit image's before
// and after are that message's content, so an unbounded export that skipped
// this arm handed over the attachment names of every held conversation in
// the workspace.
func polymorphicVisible(ctx context.Context, typeCol, idCol string, arg func(any) int) (string, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return "", errors.New("compose: no actor bound to export context")
	}
	activityArm, err := activityMemberClause(ctx, typeCol, idCol, arg)
	if err != nil {
		return "", err
	}
	if auth.UnboundedFor(actor, "person", "organization", "deal", "lead") {
		// Every non-activity row passes; an activity row faces the audience.
		return fmt.Sprintf("(%s <> 'activity' OR %s)", typeCol, activityArm), nil
	}
	var parts []string
	for _, e := range []struct{ kind, table string }{
		{"person", "person"},
		{"organization", "organization"},
		{"deal", "deal"},
		{"lead", "lead"},
	} {
		predicate := auth.VisiblePredicate(actor, e.table, arg)
		parts = append(parts, fmt.Sprintf(
			`(%s = '%s' AND EXISTS (SELECT 1 FROM %s ep WHERE ep.id = %s AND %s))`,
			typeCol, e.kind, e.table, idCol, predicate("ep"),
		))
	}
	parts = append(parts, activityArm)
	return "(" + strings.Join(parts, " OR ") + ")", nil
}

// activityMemberClause renders "this row points at an activity whose content
// the caller may read". Split out of polymorphicVisible because both of its
// arms need it — the bounded actor as one disjunct beside the record arms, the
// unbounded actor as the only test it still owes.
func activityMemberClause(ctx context.Context, typeCol, idCol string, arg func(any) int) (string, error) {
	// Activities have no owner; they inherit visibility from their links, and
	// their content additionally from their audience.
	activityClause, err := auth.ActivityContentClause(ctx, "av", arg)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		`(%s = 'activity' AND EXISTS (SELECT 1 FROM activity av WHERE av.id = %s AND %s))`,
		typeCol, idCol, activityClause,
	), nil
}

// auditExportScope scopes the audit_log to the caller's view: a row is
// visible when it targets a record the caller can see, or when the caller
// is themselves the actor (their own trail is always theirs). Rows about
// object types outside the row-scoped core (login, config) stay with the
// unbounded admin. Full row-count reconciliation with the actor's RBAC
// view is B-E11.9; this keeps the writer from leaking before that lands.
func auditExportScope(ctx context.Context, alias string, arg func(any) int) (string, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return "", errors.New("compose: no actor bound to export context")
	}
	entity, err := polymorphicVisible(ctx, alias+".entity_type", alias+".entity_id", arg)
	if err != nil {
		return "", err
	}
	if auth.UnboundedFor(actor, "person", "organization", "deal", "lead") {
		// The unbounded clause is the activity audience arm alone (see
		// polymorphicVisible), and the actor's own trail does NOT widen it:
		// "I performed the change" is not permission to read the content of a
		// message whose audience excludes me. It never did for a bounded
		// actor either, because an admin's own audit rows about a limited
		// activity are the same disclosure by the same route.
		return entity, nil
	}
	// The own-trail arm is bounded the same way: it may admit a row about any
	// object type, and it may not admit one about an activity whose audience
	// excludes the caller. Without the second test, a colleague who touched a
	// message before it was limited exports its before-and-after image forever.
	return fmt.Sprintf("(%s OR (%s.actor_id = $%d AND %s.entity_type <> 'activity'))",
		entity, alias, arg(actor.ID), alias), nil
}
