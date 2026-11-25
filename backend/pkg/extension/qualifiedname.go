// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import "regexp"

// QualifiedPrefix opens every name a unit qualifies with its own unit name,
// `ext:<unit>:<short>`. Stored rows carry it, so the spelling is permanent.
const QualifiedPrefix = "ext:"

// unitNamePart is the unit-name rule without anchors, so the full patterns
// below are built from the same text nameGrammar is.
const unitNamePart = `[a-z0-9]+(-[a-z0-9]+)*`

// QualifiedNamePattern is the full form of a record kind, signal kind, link
// type or report name. The schema CHECKs, backend/api/crm.yaml and the
// frontend each restate it, and gates/extensionnamegrammar_test.go holds every
// copy to this source.
const QualifiedNamePattern = `^ext:` + unitNamePart + `:[a-z][a-z0-9_]{0,31}$`

// CompanyTypeNamePattern is the full form of a company type. Its short part
// allows a leading digit and has no length cap, and rows stored under that rule
// keep it, so it stays a second named pattern rather than the kind rule.
const CompanyTypeNamePattern = `^ext:` + unitNamePart + `:[a-z0-9]+(_[a-z0-9]+)*$`

// shortNameGrammar is the short half of a qualified name, and of the other
// snake_case words a unit declares: job kinds, event verbs, providers, fields.
// Tool names follow toolNameGrammar instead, which also refuses `__` and a
// trailing underscore because a model reads them.
var shortNameGrammar = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// maxShortNameLength caps a qualified name's short half. With the unit name
// capped at 32 and the two separators, a full name stays inside 68 characters.
const maxShortNameLength = 32

// QualifiedName is the stored and wire form of a unit's short name. Every
// builder of `ext:<unit>:<short>` calls it, so a unit can only name a value
// under its own name.
func QualifiedName(unit Name, short string) string {
	return QualifiedPrefix + string(unit) + ":" + short
}

// IsShortName reports whether s follows the short-name rule, with no length
// cap: the rule a build tool applies to a name it composes.
func IsShortName(s string) bool {
	return shortNameGrammar.MatchString(s)
}
