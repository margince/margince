// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/relationshipmark"
)

const dealLinkJoin = "JOIN deal d ON d.id = l.deal_id"

func employmentAtActivity(edge, activity string) string {
	return employment.CurrentMembershipSQL(edge) +
		" AND " + employment.InPlaceForSQL(edge, activity) +
		" AND " + relationshipmark.ContactNotMarkedSQL(edge+".contact_id")
}

var contactLinkCondition = "r.contact_id = l.contact_id AND " + employmentAtActivity("r", "la")

// CompanyFiledActivityReach walks indexed links out from the account. The
// operand is a SQL identifier or bound placeholder expression.
func CompanyFiledActivityReach(operand string) string {
	return SQLf(`
		SELECT l.activity_id, l.company_id FROM activity_link l WHERE l.company_id = %[1]s
		UNION
		SELECT l.activity_id, d.company_id FROM activity_link l
		%[2]s WHERE d.company_id = %[1]s
		UNION
		SELECT l.activity_id, r.company_id FROM activity_link l
		JOIN activity la ON la.id = l.activity_id
		JOIN relationship r ON %[3]s
		WHERE r.company_id = %[1]s`, operand, dealLinkJoin, contactLinkCondition)
}

// CompanyClockRowsSQL is the rows a company's stored activity clock is folded
// from, aliased clock_row, in the `FROM ... WHERE` shape occurredday's clock
// helpers take. Only a row the clock's writer admits counts
// (last_activity_of_company_activity), and a held row sets no precision.
func CompanyClockRowsSQL(company string) string {
	return "FROM (" + CompanyFiledActivityReach(company+".id") + ") clock_link " +
		"JOIN activity clock_row ON clock_row.id = clock_link.activity_id WHERE clock_row.restricted_at IS NULL" +
		" AND last_activity_of_company_activity(clock_row.id, " + company + ".id) IS NOT NULL"
}

// CompanyActivityReach also follows participants whose employment was in
// place when the activity occurred. Several paths can reach the same pair.
func CompanyActivityReach(operand string) string {
	return CompanyFiledActivityReach(operand) + SQLf(`
		UNION
		SELECT p.activity_id, r.company_id FROM relationship r
		JOIN activity_participant p ON p.contact_id = r.contact_id
		JOIN activity a ON a.id = p.activity_id
		WHERE r.company_id = %s AND %s`, operand, employmentAtActivity("r", "a"))
}

// CompanyActivityExists applies reach to an activity aliased as a. Callers
// still apply their own activity visibility scope.
func CompanyActivityExists(operand string) string {
	return companyActivityMemberOf(CompanyActivityReach(operand))
}

// CompanyFiledActivityExists applies the link-only walk to activity a.
func CompanyFiledActivityExists(operand string) string {
	return companyActivityMemberOf(CompanyFiledActivityReach(operand))
}

func companyActivityMemberOf(reach string) string {
	return "a.id IN (SELECT reach.activity_id FROM (" + reach + ") reach)"
}

// CompanyActivityLinkAttribution starts from activities to file signals.
// Deal links count through archived or lost deals, as they do in reach.
func CompanyActivityLinkAttribution() string {
	return `SELECT DISTINCT l.activity_id, o.company_id
		FROM activity_link l
		JOIN activity la ON la.id = l.activity_id
		LEFT ` + dealLinkJoin + `
		LEFT JOIN relationship r ON ` + contactLinkCondition + `
		CROSS JOIN LATERAL (VALUES (l.company_id), (d.company_id), (r.company_id)) AS o(company_id)
		WHERE o.company_id IS NOT NULL`
}
