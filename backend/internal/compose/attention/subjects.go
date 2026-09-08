// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The subject vocabulary: what kind of record a row on the worklist is ABOUT.
//
// One file, because a suppressor matches on these and a misspelt literal
// matches nothing, drops nothing, and reads green — so the set a reader has to
// check is worth being able to see at once. They were spelled out one at a
// time, in the file that first needed each.
//
// subjectLead carries the contract's type and the rest do not, which is the
// shape their readers ask for: subjectKinds keys on the string and answers with
// the enum, and primaryLink compares against the enum directly.

import "github.com/margince/margince/backend/internal/contracts"

const (
	subjectCompany  = "company"
	subjectPerson   = "person"
	subjectDeal     = "deal"
	subjectProject  = "project"
	subjectActivity = "activity"
)

// subjectLead is the subject kind a row filed under a lead carries. Tasks reach
// it through primaryLink, which ranks a lead first: a task raised for a lead is
// ABOUT that lead even when the row also names the company it came from.
const subjectLead = crmcontracts.AttentionSubjectType("lead")
