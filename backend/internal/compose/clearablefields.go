// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which fields a restore can put back to NOTHING.
//
// A JSON null cannot say "clear this": every field on every update request is
// an optional pointer, so a null decodes to nil and the module reads it as "the
// caller did not supply this" — the write succeeds and the field keeps its
// value. The reversal path therefore names cleared fields separately, and this
// is the set each record type's own update path will honour.
//
// clearablefields_test.go holds this equal to the maps the stores declare. A
// field named here that the store does not clear is a restore that reports
// success and changes nothing, which is the one outcome worse than refusing.

// clearableFields are the wire fields each record type's update path can set to
// NULL. Absences are deliberate and each has a reason:
//
//   - activity has NO clearable fields. Its update statement writes every column
//     as coalesce($n, col), so the placeholder's NULL selects the current value
//     and no argument can clear one. Changing that is a change to its update
//     semantics rather than to the reversal path.
//   - a deal's amount_minor and currency are absent because money is read as one
//     field, and a half-cleared pair states an amount in no currency.
//   - a deal's partner_org_id and partner_attribution are BOTH present and mean
//     one instruction. deal_partner_attribution_pairing admits no row where one
//     half survived the other, so forgetting either forgets both, and a reversal
//     of a partner-add names both halves as null.
//   - a lead's status and score override are absent because they are lifecycle
//     positions and a sticky decision, not values.
//   - a person's full_name and an organization's display_name are absent because
//     a record with no name is not a record anybody can find again.
//
//nolint:goconst // the rows are wire FIELD names and record types read as data; the constants goconst points at are other concepts that spell the same word — a report field, a filter param — and hiding these behind them would assert a correspondence this table exists to state on its own
var clearableFields = map[string][]string{
	"person": {"first_name", "last_name", "title", "owner_id"},
	"organization": {
		"legal_name", "description", "industry", "size_band",
		"linkedin_url", "owner_id", "parent_org_id",
	},
	"lead": {"title", "company_name", "candidate_org_key", "project_id", "owner_id"},
	"deal": {
		"expected_close_date", "forecast_category", "wait_until", "owner_id",
		"organization_id", "project_id", "partner_org_id", "partner_attribution",
	},
	"project": {
		"description", "owner_id", "started_at", "target_end_date", "ended_at",
	},
	"activity": {},
}

// canClear reports whether this record type's update path can set the field to
// NULL.
func canClear(entityType, field string) bool {
	for _, clearable := range clearableFields[entityType] {
		if clearable == field {
			return true
		}
	}
	return false
}
