// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// passportStamp is how a passport agent's actor id starts, agent:<uuid>. A
// product pass stamps its kind instead, as in agent:enrich.
//
//nolint:gosec // G101 false positive: a pattern for an actor id, no credential.
const passportStamp = `^agent:[0-9a-f]{8}-[0-9a-f]{4}-`

var passportStampRE = regexp.MustCompile(passportStamp)

// AuditForAUser is the audit_log form of principal.HumanOrItsAgent: a user's
// own session, or an agent on their passport. The passport decides, because
// automatic work applying a user's decision carries on_behalf_of without one.
// alias is a compile-time literal.
func AuditForAUser(alias string) string {
	return `(` + alias + `.actor_type = 'human' OR (` + alias + `.actor_type = 'agent' AND ` + alias + `.passport_id IS NOT NULL))`
}

// StampForAUserSQL is the same question asked of a captured_by style column.
// column is a compile-time literal.
func StampForAUserSQL(column string) string {
	return `(starts_with(` + column + `, '` + principal.HumanIDPrefix + `') OR ` + column + ` ~ '` + passportStamp + `')`
}

// StampIsForAUser is StampForAUserSQL over a stamp already read.
func StampIsForAUser(stamp string) bool {
	return strings.HasPrefix(stamp, principal.HumanIDPrefix) || passportStampRE.MatchString(stamp)
}
