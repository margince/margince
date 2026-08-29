// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// What makes a passport still a credential, and the two moments it is asked.
//
// Its own file because passport.go had outgrown the size cap, and because this
// IS one concept: every condition here is a way a credential stops being one,
// and the two statements below are the same rule asked at the two moments it
// has to bind — when a call arrives carrying a token, and at every tool
// ADMISSION inside a run that authenticated once and then executes for its
// whole wall clock.
//
// Held by passport_auth_test.go, which reads this file's own source: a
// statement selecting from the aliased passport relation without building on
// agentLivenessWhere is a second rule, and a passport killed by one of two
// rules is admitted by the other.

// The LEFT JOINs and the predicate below are the liveness rule for an
// OAuth-issued passport: a revoked grant, or a disabled or soft-deleted
// client, must stop the credential on the very next call. They are the
// answer for anything the revocation cascade never saw — a row written
// before the cascade existed, or a grant an operator killed in the store —
// so authentication fails closed instead of trusting that every kill path
// remembered to walk down here.
//
// c.client_id IS NOT NULL is load-bearing: a LEFT JOIN that found no client
// row means the grant points at a client that no longer exists, which must
// fail closed rather than pass for want of a disabled_at to read.
const agentLivenessJoins = `
	LEFT JOIN oauth_grant  g ON g.id        = p.oauth_grant_id
	LEFT JOIN oauth_client c ON c.client_id = g.client_id`

// A human who owes a credential rotation is held to nothing over REST, and
// "agent ≤ human" is a runtime property: their passports must therefore resolve
// to nothing either, or the cap on the human is exactly one mint away from
// being no cap at all. This binds mid-session like every other rule here, which
// matters most on the path that raises the flag on an ESTABLISHED account —
// an operator reset (§9.1) — where live passports already exist.
//
// A locally minted passport (oauth_grant_id IS NULL) answers to no grant and
// is unaffected — the A1 path must keep working exactly as it did. The client
// half is liveClientPredicate (oauth.go), the same string the issuance path
// carries, so "still a client" cannot mean one thing at consent and another at
// authentication.
const agentLivenessPredicate = `
	AND (p.oauth_grant_id IS NULL
	     OR (g.revoked_at IS NULL AND c.client_id IS NOT NULL
	         AND ` + liveClientPredicate + `))`

// The two entry points differ in NOTHING but which column identifies the
// passport — the presented token's hash on the wire path, the row id on the
// trusted-process path — so that difference is all each one supplies.
const (
	agentByHashPredicate = `p.token_hash = $1`
	agentByIDPredicate   = `p.id = $1`
)

// agentAuthQuery assembles the agent-authentication statement around one
// caller's predicate. Both entry points build theirs here, which is what
// makes the liveness rule above impossible to have on one path and miss on
// the other.
func agentAuthQuery(predicate string) string {
	return `SELECT p.id, p.on_behalf_of, p.scopes, u.seat_type
		FROM passport p
		JOIN app_user u ON u.id = p.on_behalf_of` + agentLivenessJoins + `
		WHERE ` + predicate + agentLivenessWhere
}

// agentLivenessWhere is the liveness rule ITSELF, apart from whichever column
// identifies the passport. Every condition here is a way a credential stops
// being one: revoked, expired, granted by somebody who is gone or suspended or
// owes a password rotation, or issued against an OAuth grant or client that has
// since been killed.
//
// One string, because the rule is asked in two moments and a rule spelled twice
// is two rules. Authentication asks it when a call arrives; PassportStillLive
// asks it again at every tool ADMISSION, because a run authenticates once and
// then executes for its whole wall clock — so without the second asking, a
// revoked passport keeps working until the run ends on its own. Held by
// TestTheLivenessRuleIsAskedTheSameWayTwice.
var agentLivenessWhere = `
		  AND p.revoked_at IS NULL
		  AND now() < p.expires_at
		  AND ` + LiveMemberSQL("u") + `
		  AND u.must_change_password = false` + agentLivenessPredicate

// passportStillLiveQuery re-asks the liveness rule about a passport already
// authenticated, and about the human it is still supposed to answer to.
//
// on_behalf_of is compared rather than trusted: the principal carries a
// granting human stamped when the run started, and "agent ≤ human" is a runtime
// property. A passport re-granted to somebody else in between must not keep
// admitting calls against the authority of the human it left.
var passportStillLiveQuery = `SELECT 1
		FROM passport p
		JOIN app_user u ON u.id = p.on_behalf_of` + agentLivenessJoins + `
		WHERE p.id = $1 AND p.on_behalf_of = $2` + agentLivenessWhere
