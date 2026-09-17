// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// The retrieval trust ladder: how much a piece of evidence counts for, based
// on who put it there. It is one of the three terms in rankScore (graph.go),
// and the only one that is about provenance rather than about the query or
// the clock. rankScore itself lives here with it, because the three weights
// and the ladder are one formula — §10.7.2 — and reading half of it elsewhere
// is how the ladder came to be keyed off the wrong column in the first place.

import (
	"math"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The trust factor: a human statement outranks an agent write outranks
// captured external content — the T0/T1/T2 ladder.
//
// IT IS KEYED OFF WHO SAID IT, not off the channel the row arrived through.
// The ladder used to read activity.source, where `manual` scored 1.0 and `mcp`
// scored 0.7. That worked only while the two words happened to line up with
// human and agent, and they no longer do: an agent tool call and a human form
// submission both write source=manual, so a channel-keyed ladder would hand
// every agent-written note full human-statement trust — silently, and exactly
// inside the retrieval path that answers questions about a company's history.
//
// captured_by is the authenticated principal, stamped by the write path and
// never taken from a request body, and it already spells the distinction the
// ladder is about: `human:<user>` or `agent:<passport>`.
//
// EXCEPT ON AN IMPORTED ROW, which is the one case where captured_by names a
// real human and still answers the wrong question. An import runs as whoever
// ran it, so every record it wrote carries that administrator's `human:` prefix
// — and the HubSpot migration alone is 34,521 activities that would take full
// human-statement trust, ranking somebody else's CRM history as first-party
// testimony this installation's own colleagues gave. They are captured external
// content however they arrived, and `source_system` is what says so: it names
// the system a row came from, a native write never sets it, and it is true
// whether or not the attribution repair later managed to resolve an author.
const (
	trustHumanStatement   = 1.0 // T0 — a contact said this
	trustAgentWrite       = 0.7 // T1 — an agent wrote it, on a contact's authority
	trustCapturedExternal = 0.4 // T2 — a connector or importer brought it in
)

// The two prefixes a captured_by value can carry. They are built from the
// principal types rather than written out, so the ladder cannot drift from the
// vocabulary the write path stamps.
var (
	humanWriterPrefix = string(principal.PrincipalHuman) + ":"
	agentWriterPrefix = string(principal.PrincipalAgent) + ":"
)

// trustOfWriter reads the ladder off a captured_by value, and off whether the
// row was written in this installation at all.
//
// An unrecognized or empty prefix takes T2 rather than a middle value: an
// unattributed row is captured content until something says otherwise, and
// guessing upward is the direction that misleads.
//
// `imported` applies that same principle to the one case where captured_by is
// not merely unrecognized but actively misleading. It is checked FIRST and it
// wins over the human prefix, because the prefix is true and irrelevant: it
// names the administrator who ran the IMPORT, not whoever wrote the message.
// (Not the repair — the repair writes only the author columns and leaves
// captured_by alone, so if Alice imports and Bob repairs, captured_by still
// names Alice.)
//
// WHAT FEEDS IT is `source_system IS NOT NULL`, read in anchorTimeline's query,
// and the choice of column went through two wrong versions before landing
// there. `source_author_id` alone misses the 36 of the HubSpot import's 76
// authors who never held a seat here — the departed colleagues this whole
// effort is about. Widening it to either author column still misses more: the
// repair SKIPS a row whose author it cannot resolve, leaving both columns null,
// so an imported activity with an unmatched author would keep full
// human-statement trust. Both of those test whether attribution SUCCEEDED.
// Only `source_system` tests where the row came from, which is the question the
// ladder is asking.
//
// It is also the column the author columns' own CHECK constraint ties them to,
// and a native write never sets it — source_system is exclusive to the capture
// and import paths — so nothing typed here is demoted by accident.
func trustOfWriter(capturedBy string, imported bool) float64 {
	switch {
	case imported:
		return trustCapturedExternal
	case strings.HasPrefix(capturedBy, humanWriterPrefix):
		return trustHumanStatement
	case strings.HasPrefix(capturedBy, agentWriterPrefix):
		return trustAgentWrite
	default:
		return trustCapturedExternal
	}
}

// The `similarity` term is always 0 at today's only call site, and that is the
// formula being honest rather than a dead parameter: a graph walk has no query
// to be similar TO, so §10.7.2's first term is genuinely zero there. Dropping
// it would leave two thirds of a published formula in the code and the missing
// third only in the spec.
//
//nolint:unparam // see above — the zero is meaningful, not unused.
func rankScore(similarity float64, occurredAt time.Time, capturedBy string, imported bool, now time.Time) float64 {
	days := now.Sub(occurredAt).Hours() / 24
	if days < 0 {
		days = 0
	}
	recency := math.Exp2(-days / recencyHalfLifeDays)
	return wRankSim*similarity + wRankRec*recency + wRankTrust*trustOfWriter(capturedBy, imported)
}
