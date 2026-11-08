// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

// ProjectRungKey names, in the evidence of capture's project-link audit row,
// the ladder rung that chose the project. Its values are the ProjectRung*
// constants.
const ProjectRungKey = "project_rung"

// The rungs of capture's project ladder.
const (
	ProjectRungThread  = "thread"
	ProjectRungDeal    = "deal"
	ProjectRungSubject = "subject"
)

// DealDeclinedKey marks the `after` of the activity_relink audit row that
// records a colleague's "not about this deal". Its entity names the deal.
const DealDeclinedKey = "declined"

// HumanDealFilingSQL is the test that a human or an agent filed activity on
// some deal, or took it off one. A later decline of that deal undoes it.
// Capture's repair and the activities writer ask it before taking an
// automatic deal link back. activity is a compile-time SQL expression.
func HumanDealFilingSQL(activity string) string {
	return humanFilingSQL(activity, `'deal'`, "") + ` AND NOT ` + DeclineRowSQL("h") + latestDealDecisionSQL(activity) + `)`
}

// DealDeclinedSQL is the test that the latest human or agent decision about
// activity and deal is "not about this deal". No automatic pass files the
// activity on that deal while it holds; a colleague's relink lifts it. Both
// arguments are compile-time SQL expressions.
func DealDeclinedSQL(activity, deal string) string {
	return humanFilingSQL(activity, `'deal'`, deal) + ` AND ` + DeclineRowSQL("h") + latestDealDecisionSQL(activity) + `)`
}

// latestDealDecisionSQL keeps row h only when no later human or agent row
// names the same deal, so a decision supersedes every older one about it.
func latestDealDecisionSQL(activity string) string {
	return ` AND NOT EXISTS (SELECT 1 FROM audit_log n WHERE ` + humanRowSQL("n", activity, `'deal'`) + `
	   AND ` + relinkRecordSQL("n") + ` = ` + relinkRecordSQL("h") + ` AND n.id > h.id)`
}

// HumanLeadFilingSQL is HumanDealFilingSQL for one lead: whether a human or an
// agent ever filed the activity under it or took it off. lead is a
// compile-time SQL expression.
func HumanLeadFilingSQL(activity, lead string) string {
	return humanFilingSQL(activity, `'lead'`, lead) + `)`
}

// HumanProjectFilingSQL is HumanLeadFilingSQL for one project. project is a
// compile-time SQL expression.
func HumanProjectFilingSQL(activity, project string) string {
	return humanFilingSQL(activity, `'project'`, project) + `)`
}

// DeclineRowSQL is true for the audit row alias of a decline, so a reader of
// relink rows can tell a decline from a filing.
func DeclineRowSQL(alias string) string {
	return `coalesce((` + alias + `.after->>'` + DealDeclinedKey + `')::boolean, false)`
}

// humanFilingSQL opens the EXISTS over row h; each caller adds its own last
// clause and closes it.
func humanFilingSQL(activity, entityType, entityID string) string {
	sameRecord := ""
	if entityID != "" {
		sameRecord = ` AND ` + relinkRecordSQL("h") + ` = (` + entityID + `)::text`
	}
	return `EXISTS (SELECT 1 FROM audit_log h WHERE ` + humanRowSQL("h", activity, entityType) + sameRecord
}

// humanRowSQL is a human or agent relink row of activity that names a record
// of entityType.
func humanRowSQL(alias, activity, entityType string) string {
	return alias + `.entity_type = 'activity' AND ` + alias + `.entity_id = ` + activity + ` AND ` + alias + `.action = 'activity_relink'
	   AND (` + alias + `.after->>'entity_type' = ` + entityType + ` OR ` + alias + `.before->>'entity_type' = ` + entityType + `)
	   AND ` + alias + `.actor_type IN ('human', 'agent')`
}

// relinkRecordSQL is the record a relink row is about.
func relinkRecordSQL(alias string) string {
	return `coalesce(` + alias + `.after->>'entity_id', ` + alias + `.before->>'entity_id')`
}
