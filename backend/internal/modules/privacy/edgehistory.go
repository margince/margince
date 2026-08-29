// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The links a record's history has to include, and the two gates naming the
// other end drags in.
//
// A relationship is seven kinds in ONE table with FIVE endpoint columns, and a
// record's history is the history of the edges it currently occupies an end of.
// Which column holds the anchor follows from the anchor's KIND — a person id can
// only be person_id, an organization id is organization_id or
// counterparty_org_id — so the lookup is a set of sargable equality branches,
// never a disjunction: the read this widens rides an ordered index walk to LIMIT
// on idx_audit_entity, and an OR makes the planner materialise both sides and
// sort instead.
//
// Naming the OTHER end is what makes an edge line readable ("linked Acme as
// employer") and is also the whole hazard: the row discloses a second record. So
// both of the other end's gates are rendered HERE, inside the branch that finds
// the edge, and never applied to the window afterwards — a page filtered after
// the keyset comes back short, spends a query per row, and tells the caller by
// its own row count roughly how many events it withheld.

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EdgeEntityType is the audit spine's entity_type for an edge row. The wording
// keys on the (action, entity_type) PAIR, so it is a value here and not an
// implicit assumption in a switch — and the reversal seam dispatches on the same
// value, which is why it is exported rather than typed a second time there.
const EdgeEntityType = "relationship"

// edgeEndpoint is one of relationship's endpoint columns, the record kind it
// points at, and the column that NAMES that record on screen.
//
// This slice is the read's whole knowledge of the table's shape, which is why
// gates/edgeendpointcensus_test.go derives relationship's endpoint columns from
// the shape CHECK constraints and fails when one is missing from here: a
// forgotten column is a silently one-sided edge, and a co-sell that appears on
// only one of the two companies looks exactly like a co-sell nobody made.
//
// Two columns point at `organization`, which is what makes this a slice and not
// a map — and is also the case that a single-anchor design drops.
// entityTypeOrganization is named because TWO endpoint columns point at it, and
// a slice of literals would spell the same table twice with nothing holding the
// two spellings together.
const entityTypeOrganization = "organization"

var edgeEndpoints = []edgeEndpoint{
	{column: "person_id", entityType: "person", labelColumn: "full_name"},
	{column: "counterparty_person_id", entityType: "person", labelColumn: "full_name"},
	{column: "organization_id", entityType: entityTypeOrganization, labelColumn: "display_name"},
	{column: "counterparty_org_id", entityType: entityTypeOrganization, labelColumn: "display_name"},
	{column: "deal_id", entityType: "deal", labelColumn: "name"},
	{column: "project_id", entityType: "project", labelColumn: "name"},
}

type edgeEndpoint struct {
	column      string
	entityType  string
	labelColumn string
}

// edgeSubject is the edge an audit row was about, as one history line needs it:
// the kind, and the OTHER end named and identified. Nil on every row that
// changed a field of the record itself.
type edgeSubject struct {
	kind       string
	otherType  string
	otherID    ids.UUID
	otherLabel *string
}

// edgeAnchorsFor names the columns a record of this kind can occupy. Derived
// from edgeEndpoints rather than listed a second time: a column added there is
// searched from both ends at once, where a second list would give the census a
// green tree and the reader a one-sided edge.
func edgeAnchorsFor(entityType string) []edgeEndpoint {
	var out []edgeEndpoint
	for _, endpoint := range edgeEndpoints {
		if endpoint.entityType == entityType {
			out = append(out, endpoint)
		}
	}
	return out
}

// edgeSubjectCTE renders the `edge` CTE: every relationship the anchor record is
// an end of, paired with the other end named and labelled. The empty string
// means this read has no edge branch at all — a kind that occupies no endpoint
// column (lead, activity), which is absence and not an error.
//
// ONE branch per column the anchor can occupy, and the only predicate on
// `relationship` in it is that column's equality. That is what keeps the lookup
// on the endpoint index: a branch that also required the OTHER columns to be
// null would hand the planner a second indexable access path, and on a table
// where most rows have organization_id null it takes it — measured, and the
// reason the other end is resolved by expression here rather than by a branch
// per (anchor, other) pair.
//
// TENANT ISOLATION. Neither audit_log nor relationship carries a workspace
// column and core has no row-level security, so nothing in this statement binds
// the tenant and nothing here needs to: the caller reached it past
// EnsureVisible on the anchor, every branch is keyed by that anchor's id, and an
// edge reaches its other end by foreign key — which the composite FKs on
// relationship make same-tenant by construction. What bounds the ROWS is the
// anchor's gate, exactly as it is for the actor-name joins next door.
//
// Both of the other end's gates are conjuncts of the branch:
//
//   - VISIBILITY, through auth.EdgeReadScope — the edge's own object grant plus
//     platform/auth's endpoint conjunction, composed rather than restated here.
//     A caller who may not see the other end does not receive the row, and
//     receives no refusal either: a refusal is proof the record exists.
//   - ERASURE. The scrub tombstone is computed per (entity_type, entity_id), so
//     the anchor's own boundary says nothing about the other end — and an
//     employment image holds the role and the dates of BOTH records. Left
//     unfiltered, erasing a person would leave their employment readable on the
//     company forever, after the erasure was certified. The whole edge goes, not
//     the rows before the tombstone: every row of that edge is as much about the
//     erased end as about the anchor, so there is no position in it that stops
//     being their data.
func edgeSubjectCTE(ctx context.Context, entityType string, anchorPos int, arg func(any) int) (string, error) {
	anchors := edgeAnchorsFor(entityType)
	if len(anchors) == 0 {
		return "", nil
	}
	scope, err := auth.EdgeReadScope(ctx, "r", arg)
	if err != nil {
		return "", err
	}
	if scope == "" {
		scope = "TRUE"
	}
	scrubPos := arg(fieldHistoryScrubActions)

	branches := make([]string, 0, len(anchors))
	for i, anchor := range anchors {
		// The statement is assembled here, next to the gate it rides, because
		// gates/edgereaders_test.go asks which FUNCTION reads the edge table: a
		// read whose SQL sits one call away from its admission is
		// indistinguishable from one nobody gated.
		other, joins, where := edgeBranchParts(anchor, anchors[:i], anchorPos, scrubPos, scope)
		branches = append(branches, fmt.Sprintf(`
			SELECT r.id AS rel_id, r.kind AS edge_kind, %s
			  FROM relationship r
			  %s
			 WHERE %s`, other, joins, where))
	}
	return "edge AS (\n" + strings.Join(branches, "\n\t\t\tUNION ALL\n") + "\n\t\t)", nil
}

// edgeBranchParts renders one branch's three moving pieces: the OTHER end as
// three projected columns, the joins that name it, and the conjunction that
// bounds the rows.
//
// The type arm, the id and the label arm are emitted by ONE loop, so the three
// agree on which column they are describing by construction. Written as three
// hand-kept lists in the same order they would agree only until somebody
// inserted a column into two of them.
//
// claimed are the anchor columns EARLIER branches already answered for. A record
// that somehow sat at two ends of one edge would otherwise have that edge's rows
// served twice, once per end; excluding the earlier column makes the branches
// disjoint here rather than relying on the table's CHECK to keep them so. The
// exclusion is deliberately NOT indexable — it must not become an access path.
//
// Every identifier interpolated here is a compile-time literal from
// edgeEndpoints; the anchor id and the scrub vocabulary are bound.
func edgeBranchParts(anchor edgeEndpoint, claimed []edgeEndpoint, anchorPos, scrubPos int, scope string,
) (other, joins, where string) {
	var typeArms, labelArms, otherIDs, joined []string
	conds := []string{fmt.Sprintf("r.%s = $%d", anchor.column, anchorPos)}
	for _, earlier := range claimed {
		conds = append(conds, fmt.Sprintf("r.%s IS DISTINCT FROM $%d", earlier.column, anchorPos))
	}
	for _, endpoint := range edgeEndpoints {
		if endpoint.column == anchor.column {
			continue
		}
		alias := "other_" + endpoint.column
		typeArms = append(typeArms, fmt.Sprintf("WHEN r.%s IS NOT NULL THEN '%s'", endpoint.column, endpoint.entityType))
		labelArms = append(labelArms, fmt.Sprintf("WHEN r.%s IS NOT NULL THEN %s.%s", endpoint.column, alias, endpoint.labelColumn))
		otherIDs = append(otherIDs, "r."+endpoint.column)
		joined = append(joined, fmt.Sprintf("LEFT JOIN %s %s ON %s.id = r.%s",
			endpoint.entityType, alias, alias, endpoint.column))
		conds = append(conds, fmt.Sprintf(
			`NOT EXISTS (SELECT 1 FROM audit_log scrub
			     WHERE scrub.entity_type = '%s' AND scrub.entity_id = r.%s
			       AND scrub.action = ANY($%d))`, endpoint.entityType, endpoint.column, scrubPos,
		))
	}
	idList := strings.Join(otherIDs, ", ")
	// An edge with no end but the anchor is a row no kind's shape admits, and a
	// history line about a link to nothing would be worse than its absence.
	conds = append(conds, scope, fmt.Sprintf("COALESCE(%s) IS NOT NULL", idList))

	// ONE other end: the CASE arms and this COALESCE take the FIRST non-null
	// column in edgeEndpoints order, so a row occupying two other ends is served
	// as a link to whichever of them comes first in that slice.
	//
	// The shapes that make one other end true today are the per-kind CHECKs on
	// `relationship`: rel_partner_shape, rel_project_stakeholder_shape and
	// rel_project_company_shape each name their two columns and NULL every
	// other. rel_employment_shape and rel_stakeholder_shape do NOT — neither
	// nulls counterparty_org_id — and the create path takes the endpoints it is
	// given, so a three-endpoint employment is admissible rather than impossible.
	// Closing that is the write path's or the constraint's to do; what it costs
	// here is a line that names one of the two other ends and not the other.
	// It costs no disclosure: every endpoint column carries this branch's
	// visibility and erasure conjunctions above whether or not the label arms
	// reach it, which gates/edgeendpointcensus_test.go and
	// TestEveryEdgeBranchGatesTheOtherEndsVisibilityAndErasure hold.

	other = fmt.Sprintf(`CASE %s END AS other_type,
			       COALESCE(%s) AS other_id,
			       CASE %s END AS other_label`,
		strings.Join(typeArms, " "), idList, strings.Join(labelArms, " "))
	return other, strings.Join(joined, "\n			  "), strings.Join(conds, "\n			   AND ")
}

// edgeSubjectColumns are the CTE's own four columns as the window projects them,
// after the shared audit projection. The record's own branch selects nulls in
// the same positions, because a union has one column list and a record row was
// not about a link.
const (
	edgeSubjectColumns   = `e.edge_kind, e.other_type, e.other_id, e.other_label`
	noEdgeSubjectColumns = `NULL::text AS edge_kind, NULL::text AS other_type,
			NULL::uuid AS other_id, NULL::text AS other_label`
)

// scanEdgeAuditRow decodes one window row: the shared audit projection, then the
// edge subject when the row came from the edge branch.
//
// A row is an edge row when it carries an other end, and that is the only
// marker there is — the audit row's own entity_type is not projected, because
// the branch it arrived on already answers what it was about.
func scanEdgeAuditRow(src auditRowScanner, r *recordAuditRow) error {
	var kind, otherType, otherLabel *string
	var otherID *ids.UUID
	if err := scanRecordAuditRowWith(src, r, &kind, &otherType, &otherID, &otherLabel); err != nil {
		return err
	}
	if otherType == nil || otherID == nil {
		return nil
	}
	subject := edgeSubject{otherType: *otherType, otherID: *otherID, otherLabel: otherLabel}
	if kind != nil {
		subject.kind = *kind
	}
	r.edge = &subject
	return nil
}

// recordHistoryWindowSQL renders one keyset window over the record's own audit
// rows and the rows of its links, as a UNION ALL of two sargable branches — not
// a disjunction. The record's branch walks idx_audit_entity
// (entity_type, entity_id, occurred_at DESC) straight to LIMIT and the edge
// branch walks the same index once per link; an OR over the two would make the
// planner materialise both and sort.
//
// Each branch carries the LIMIT itself, because Postgres cannot push the outer
// one into a union arm. Taking `fetch` from each is enough and not a
// compromise: the newest `fetch` rows of the union are a subset of the newest
// `fetch` rows of each branch.
//
// conds are the boundary and cursor predicates, aliased on `a` so both branches
// take them verbatim — a keyset applied to one arm and not the other would page
// the two timelines at different speeds.
func recordHistoryWindowSQL(typePos, idPos, fetchPos int, conds []string, edgeCTE string) string {
	keyset, edgeWhere := "", ""
	if len(conds) > 0 {
		keyset = " AND " + strings.Join(conds, " AND ")
		edgeWhere = "\n\t\t\tWHERE " + strings.Join(conds, " AND ")
	}
	own := fmt.Sprintf(`
			SELECT`+recordAuditColumns+`,
				`+noEdgeSubjectColumns+`
			FROM audit_log a`+auditActorNameJoins+agentClientNameJoin+`
			WHERE a.entity_type = $%d AND a.entity_id = $%d%s
			ORDER BY a.occurred_at DESC, a.id DESC
			LIMIT $%d`, typePos, idPos, keyset, fetchPos)
	if edgeCTE == "" {
		return own
	}
	edges := fmt.Sprintf(`
			SELECT`+recordAuditColumns+`,
				`+edgeSubjectColumns+`
			FROM edge e
			JOIN audit_log a ON a.entity_type = '`+EdgeEntityType+`' AND a.entity_id = e.rel_id`+
		auditActorNameJoins+agentClientNameJoin+`%s
			ORDER BY a.occurred_at DESC, a.id DESC
			LIMIT $%d`, edgeWhere, fetchPos)
	return fmt.Sprintf(`
		WITH %s
		SELECT * FROM ((%s)
			UNION ALL
			(%s)) AS spine
		ORDER BY occurred_at DESC, id DESC
		LIMIT $%d`, edgeCTE, own, edges, fetchPos)
}

// HistoryEdge is the edge a history line was about, as the wire declares it: the
// kind, and the OTHER end named and identified so a reader can go and look at
// it. Nil on every row that changed a field of the record itself.
//
// It carries no field diff and never will. role, the dates and the
// primary-employer flag belong to the LINK rather than to either record it
// joins, so projecting them as the record's own fields would invent fields the
// record does not have — which is the same reason edge rows never enter field
// history.
type HistoryEdge struct {
	Kind            string
	OtherEntityType string
	OtherEntityID   ids.UUID
	OtherLabel      *string
}

// historyEdgeOf renders the scanned subject for a caller outside this package.
func historyEdgeOf(edge edgeSubject) HistoryEdge {
	return HistoryEdge{
		Kind:            edge.kind,
		OtherEntityType: edge.otherType,
		OtherEntityID:   edge.otherID,
		OtherLabel:      edge.otherLabel,
	}
}
