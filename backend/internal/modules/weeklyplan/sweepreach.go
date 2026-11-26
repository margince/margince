// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

import "github.com/margince/margince/backend/pkg/extension/reach"

// SweepReach declares the record a commitment is linked to, so a merge keeps
// the line naming a record a reader can open.
//
// An erasure redacts a commitment naming the subject and keeps the row. The
// row is the rep's work record, and the request reaches only what it says about
// the contact. The CHECK constraints bind the columns in two groups: the link
// pair (link_whole), and the answer with its author and moment (response_whole).
// The label takes a tombstone because label_present refuses an empty one.
func SweepReach() []reach.Table {
	return []reach.Table{{
		Name:  "weekly_plan_commitment",
		Links: []reach.Link{{Column: fieldLinkedID, KindColumn: fieldLinkedKind}},
		Erase: []reach.Clear{
			{Column: fieldLabel, To: reach.Tombstone},
			{Column: "help_requested", To: reach.Empty},
			{Column: fieldResponse, To: reach.Empty},
			{Column: "manager_user_id", To: reach.Null},
			{Column: "responded_at", To: reach.Null},
			{Column: fieldLinkedKind, To: reach.Null},
			{Column: fieldLinkedID, To: reach.Null},
		},
	}}
}
