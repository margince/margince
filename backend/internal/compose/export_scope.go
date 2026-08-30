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
// obligations: row scope says which records a caller may reach, and audience
// says who may read one message's content, which does not yield to
// row_scope=all (auth.ActivityContentClause). An attachment's filename and an
// audit image's before-and-after are that message's content.
//
// Today no HUMAN reaches the unbounded branch: person and organization are
// owner-private (auth.ownerPrivateTables), so UnboundedFor is false for every
// seat including an admin, and an admin export has always faced the audience
// through the bounded arms below. The branch is the system principal's, and
// composing the arm here rather than returning an empty clause is what keeps
// that true if the owner-private set ever changes — the alternative is a
// silent widening at a distance, in a file nobody would think to re-read.
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
	// Two shapes reach a message, not one. An audit row may target the ACTIVITY,
	// and it may target an ATTACHMENT of it — whose image carries the filename,
	// and a filename states what the message is about. Excluding only the first
	// is the half-fix that leaves the same content reachable by the longer
	// route, so both arms below take this one test.
	noLimitedMessage, err := carriesNoLimitedMessage(ctx, alias, arg)
	if err != nil {
		return "", err
	}
	if auth.UnboundedFor(actor, "person", "organization", "deal", "lead") {
		// The system principal's branch, and only its: person and organization
		// are owner-private (auth.ownerPrivateTables), so UnboundedFor is false
		// for every human including one with row_scope=all, and an admin export
		// takes the bounded path below.
		//
		// It carries the message test anyway, because polymorphicVisible asks
		// the audience of a row that names an activity DIRECTLY and an
		// attachment row would pass that arm on its own. Composing it here
		// rather than returning the entity clause alone is what keeps the
		// branch honest if the owner-private set ever changes.
		return fmt.Sprintf("(%s AND %s)", entity, noLimitedMessage), nil
	}
	return fmt.Sprintf("(%s OR (%s.actor_id = $%d AND %s))",
		entity, alias, arg(actor.ID), noLimitedMessage), nil
}

// carriesNoLimitedMessage renders "this audit row is not about a message whose
// content the caller may not read".
//
// Two shapes reach a message. The row may target the ACTIVITY, and it may
// target an ATTACHMENT of one — whose image carries the filename, and a
// filename states what the message is about. Both take the same audience
// clause, so an attachment of an OPEN message still travels: the limit is the
// message's audience, never the fact that a row points at an attachment.
func carriesNoLimitedMessage(ctx context.Context, alias string, arg func(any) int) (string, error) {
	readable, err := auth.ActivityContentClause(ctx, "av", arg)
	if err != nil {
		return "", err
	}
	// One arm per shape rather than a CASE that picks the id: an attachment row
	// has to reach its activity through the attachment table, and folding that
	// hop into the same EXISTS as the direct arm made a subquery whose
	// correlation was easy to get subtly wrong and impossible to read.
	return fmt.Sprintf(
		`(%[1]s.entity_type NOT IN ('activity', 'attachment')
		  OR (%[1]s.entity_type = 'activity' AND EXISTS (
		       SELECT 1 FROM activity av WHERE av.id = %[1]s.entity_id AND %[2]s))
		  OR (%[1]s.entity_type = 'attachment' AND NOT EXISTS (
		       SELECT 1 FROM attachment ata
		        JOIN activity av ON av.id = ata.entity_id
		       WHERE ata.id = %[1]s.entity_id AND ata.entity_type = 'activity'
		         AND NOT (%[2]s))))`,
		alias, readable), nil
}
