// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The "is this activity part of the engagement" predicate, spelled once.
//
// Three reads in this package need it — the baseline "what changed" walks it,
// the earlier meetings walk it, and the account history walks it — and each had
// its own copy of the same SQL. Three copies of one rule is three chances for
// one of them to answer differently about the same row, and the rule itself is
// subtle enough to be worth stating in one place: an activity is in scope when
// it is filed under THIS project, or when it is filed under no project at all.
//
// The second arm is what makes it usable. A conversation nobody filed is part
// of every engagement's history rather than none, because filing is a thing
// people forget rather than a claim that a mail was off-topic.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// projectWithinPredicate renders the SQL, or scopeAll when the brief is not
// narrowed to a project. `alias` is the activity table's alias in the caller's
// query; `link` is the alias the predicate gives its own two subqueries, which
// must not collide with anything the caller already uses.
func projectWithinPredicate(alias, link string, project *ids.ProjectID, arg func(any) int) string {
	if project == nil {
		return scopeAll
	}
	return fmt.Sprintf(`(EXISTS (
			    SELECT 1 FROM activity_link %[2]si
			    WHERE %[2]si.activity_id = %[1]s.id AND %[2]si.project_id = $%[3]d)
			  OR NOT EXISTS (
			    SELECT 1 FROM activity_link %[2]sf
			    WHERE %[2]sf.activity_id = %[1]s.id AND %[2]sf.project_id IS NOT NULL))`,
		alias, link, arg(project.UUID))
}
