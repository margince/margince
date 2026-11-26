// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A constraint left NOT VALID is declared, with the reason it stays that way.
//
// NOT VALID binds every INSERT and UPDATE from the moment it is added and scans
// nothing that is already there. That is the point of it — the two-step exists
// so a large table can be constrained without an ACCESS EXCLUSIVE scan — but
// the second step is easy to forget, and forgetting it is SILENT: `\d contact`
// prints the CHECK, and only the NOT VALID suffix says the rows already in the
// table were never asked. An auditor reads a guarantee the database has not
// made, and the rows that would have failed are never named.
//
// So NOT VALID is not the error: finishing is the default, and staying is a
// decision somebody has to write down.
//
// The catalog rather than the migration sources, for the reason the trigger
// gates give: it is the schema a database really ends up with, so a constraint
// added NOT VALID in one migration and validated in another is already resolved.
//
// FOREIGN KEYS COUNT TOO — the defect is the silence rather than the constraint
// kind, and the one entry below is a foreign key.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// unvalidatedSubject is `table.constraint`, the vocabulary this gate's subjects
// are drawn from.
type unvalidatedSubject string

// permanentlyUnvalidated are the constraints this schema leaves NOT VALID on
// purpose, each with what a VALIDATE would cost. Keyed `table.constraint`.
//
// It is not a backlog. A constraint belongs here when scanning the existing
// rows cannot be made to succeed — not when nobody got round to it.
var permanentlyUnvalidated = gatekit.Waive(map[unvalidatedSubject]string{
	"analytics_share.analytics_share_scope_team_id_fkey": polymorphicBranchKey,
	"analytics_share.analytics_share_scope_user_id_fkey": polymorphicBranchKey,
	"record_grant.record_grant_company_id_fkey":          polymorphicBranchKey,
	"record_grant.record_grant_contact_id_fkey":          polymorphicBranchKey,
	"record_grant.record_grant_deal_id_fkey":             polymorphicBranchKey,
	"record_grant.record_grant_lead_id_fkey":             polymorphicBranchKey,
	"record_grant.record_grant_project_id_fkey":          polymorphicBranchKey,
	"provider_applied_field.provider_applied_field_run_subject_fkey": "" +
		"the marker-to-run key cannot be checked against history from a migration. An installation " +
		"that erased a subject before the two erasure paths ordered themselves correctly may hold a " +
		"marker whose run was scrubbed out from under it, so a VALIDATE would fail the migration on " +
		"exactly the installations that have been running longest. The ordering it depends on is now " +
		"what both purge paths do — the markers are deleted before the run is scrubbed — so what it " +
		"binds from here on is the whole guarantee asked for",
	"activity_meeting_history.activity_meeting_history_start_precision": "" +
		"rows recorded before the column hold a start whose precision nobody wrote down, and NULL is " +
		"how they say so. A VALIDATE would fail on every installation with meeting history. The " +
		"precision data task fills in only the rows the noon repair's audit proves; the rest stay " +
		"unknown on purpose, and the rule binds every row written since",
	"confirm_token.confirm_token_question_locale_fkey":            unlistedLanguage,
	"consent_text_version.consent_text_version_locale_fkey":       unlistedLanguage,
	"legal_entity.legal_entity_default_language_fkey":             unlistedLanguage,
	"offer.offer_language_fkey":                                   unlistedLanguage,
	"product_translation.product_translation_language_fkey":       unlistedLanguage,
	"team_weekly_review.team_weekly_review_summary_language_fkey": unlistedLanguage,
	"offer_template.offer_template_locale_shape":                  unlistedLocale,
	"lead_privacy_notice.lead_privacy_notice_locale_shape":        unlistedLocale,
	"finance_invoice.finance_invoice_names_its_customer_link":     unnamedCustomerLink,
	"company.company_source_check":                                unlistedRecordSource,
	"contact.contact_source_check":                                unlistedRecordSource,
	"deal.deal_source_check":                                      unlistedRecordSource,
	"offer.offer_source_check":                                    unlistedRecordSource,
	"project.project_source_check":                                unlistedRecordSource,
	"signal.signal_source_check":                                  unlistedRecordSource,
	"finance_payment.finance_payment_names_its_customer_link":     unnamedCustomerLink,
	"activity_participant.activity_participant_address_is_canonical": "" +
		"rows written before the check hold the address as typed, and the participant address data " +
		"task lowercases them after the migrations ran. A VALIDATE would fail the upgrade of an " +
		"installation whose task has not finished, and no migration may wait for a data task. The " +
		"check binds every row written since; validating it needs a minimum upgrade-from release " +
		"that guarantees the task ran",
})

// unlistedLanguage is the cost shared by the keys to the language table over
// columns that took any text before it.
const unlistedLanguage = "" +
	"rows written before the key may hold a code no unit declared, such as a typo or a language " +
	"that was never offered. The unknown_language_values data task lists them and rewrites none, " +
	"because a mapping would be a guess. A VALIDATE would fail the upgrade of an installation that " +
	"holds one. The key binds every row written since"

// unlistedLocale is the same cost for the two locale columns and their shape check.
const unlistedLocale = "" +
	"rows written before the check may hold a locale that is not a language with an optional " +
	"region. The unknown_language_values data task lists them and rewrites none, and a VALIDATE " +
	"would fail the upgrade of an installation that holds one. The check binds every row written since"

// unlistedRecordSource is the cost shared by the source checks whose older rows
// a data task corrects.
const unlistedRecordSource = "" +
	"rows written before the list hold other words, and the source data task moves them after the " +
	"migrations ran. A VALIDATE would fail the upgrade of an installation whose task has not finished, " +
	"and no migration may wait for a data task. The check binds every row written since; validating " +
	"it needs a minimum upgrade-from release that guarantees the task ran"

// unnamedCustomerLink is the cost shared by the two mirrored tables' link checks.
const unnamedCustomerLink = "" +
	"a row mirrored before rows named their customer link may name none, when its connection never " +
	"mapped its company. A VALIDATE would fail the upgrade on such an installation, and the maintainers " +
	"decided those rows stay as they are. The rule binds every row the sync writes or rewrites"

// polymorphicBranchKey is the one cost shared by every key generated from a
// polymorphic pair that had none before.
const polymorphicBranchKey = "" +
	"a key generated from a polymorphic pair that had none: rows written before it may name a " +
	"record already deleted, so a VALIDATE would fail the migration on the installations that " +
	"ran longest. It binds every new row, and a delete cascades to every row whose record exists"

// unvalidatedConstraint reads one catalog record that ends NOT VALID. The
// suffix is what the dump prints for `convalidated = false`, on a CHECK and a
// FOREIGN KEY alike.
var unvalidatedConstraint = regexp.MustCompile(
	`^(?:public|ext)\.([a-z0-9_]+)\.([a-zA-Z0-9_]+) (?:CHECK|FOREIGN KEY) .*NOT VALID$`)

func TestEveryUnvalidatedConstraintSaysWhyItStaysThatWay(t *testing.T) {
	t.Parallel()

	for _, record := range catalogRecords(t) {
		m := unvalidatedConstraint.FindStringSubmatch(strings.TrimSpace(record))
		if m == nil {
			continue
		}
		// Asked about an OFFENDER: the loop reaches here only for a constraint
		// the schema really leaves unvalidated, so an entry whose constraint was
		// validated stops being matched and AssertAllMatched reports it.
		name := unvalidatedSubject(m[1] + "." + m[2])
		if permanentlyUnvalidated.Waived(t, name) {
			continue
		}
		t.Errorf("%s is NOT VALID, so the rows already in %s were never checked against it and "+
			"nothing will ever name them. Validate it in a migration of its own, or declare it in "+
			"permanentlyUnvalidated with what a VALIDATE would cost", name, m[1])
	}
	permanentlyUnvalidated.AssertAllMatched(t)
}
