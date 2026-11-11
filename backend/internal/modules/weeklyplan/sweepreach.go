// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

import "github.com/margince/margince/backend/pkg/extension/reach"

// SweepReach declares the record a commitment is linked to, so a merge keeps
// the line naming a record a reader can open.
//
// An erasure redacts, never deletes, a commitment naming the subject: the row
// is the rep's work record, and the request reaches only what it says about the
// contact. The columns go in groups their CHECK constraints bind together:
// the link pair (link_whole) and the answer, its author and its moment
// (response_whole). The label takes a tombstone because label_present refuses
// an empty one.
func SweepReach() []reach.Table {
	return []reach.Table{{
		Name:  "weekly_plan_commitment",
		Links: []reach.Link{{Column: "linked_record_id", KindColumn: "linked_record_type"}},
		Erase: []reach.Clear{
			{Column: "label", To: reach.Tombstone},
			{Column: "help_requested", To: reach.Empty},
			{Column: "manager_response", To: reach.Empty},
			{Column: "manager_user_id", To: reach.Null},
			{Column: "responded_at", To: reach.Null},
			{Column: "linked_record_type", To: reach.Null},
			{Column: "linked_record_id", To: reach.Null},
		},
	}}
}
