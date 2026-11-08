// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

// DealDeclinedKey marks the `after` of the activity_relink audit row that
// records a colleague's "not about this deal". Its entity names the deal.
const DealDeclinedKey = "declined"

// HumanDealFilingSQL is the test that a human or an agent ever filed activity
// on a deal, or took it off one. Capture's repair and the activities writer
// ask it before taking an automatic deal link back. A decline speaks for its
// one deal only, so DealDeclinedSQL reads it instead. activity is SQL.
func HumanDealFilingSQL(activity string) string {
	return humanFilingSQL(activity, `'deal'`, "") + ` AND NOT ` + DeclineRowSQL("h") + `)`
}

// DealDeclinedSQL is the test that a human or an agent said activity is not
// about deal. No automatic pass files the activity on that deal again; a
// colleague's own relink still does. Both arguments are compile-time SQL
// expressions.
func DealDeclinedSQL(activity, deal string) string {
	return humanFilingSQL(activity, `'deal'`, deal) + ` AND ` + DeclineRowSQL("h") + `)`
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

// humanFilingSQL opens the EXISTS; each caller adds its own last clause and
// closes it.
func humanFilingSQL(activity, entityType, entityID string) string {
	sameRecord := ""
	if entityID != "" {
		sameRecord = ` AND coalesce(h.after->>'entity_id', h.before->>'entity_id') = (` + entityID + `)::text`
	}
	return `EXISTS (SELECT 1 FROM audit_log h
	 WHERE h.entity_type = 'activity' AND h.entity_id = ` + activity + ` AND h.action = 'activity_relink'
	   AND (h.after->>'entity_type' = ` + entityType + ` OR h.before->>'entity_type' = ` + entityType + `)` + sameRecord + `
	   AND h.actor_type IN ('human', 'agent')`
}
