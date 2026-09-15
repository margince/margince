// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The two questions the authoring surface asks of the action table in
// retentionactions.go: whether one object_type/action pair has an executor, and
// which actions a given scope may be authored with. Both read the same
// retentionActions map the dispatch does, so a pair the pass cannot run is a
// pair the author cannot write.

// SupportsRetentionAction reports whether the engine can perform this action on
// this object type. The authoring surface refuses a pair it answers false for.
func SupportsRetentionAction(objectType, action string) bool {
	_, ok := retentionActions[objectType+"/"+action]
	return ok
}

// ActionsForScope is every action a given scope may be authored with, sorted, so
// a refusal can name the alternatives instead of leaving the caller to guess at
// a set the contract's two independent enums do not express.
func ActionsForScope(objectType string) []string {
	out := make([]string, 0, 3)
	for _, action := range []string{actionArchive, actionAnonymize, actionErase} {
		if SupportsRetentionAction(objectType, action) {
			out = append(out, action)
		}
	}
	return out
}
