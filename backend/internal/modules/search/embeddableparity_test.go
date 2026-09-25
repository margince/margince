// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// The set form and the per-row form admit the same rows.
//
// Two declarations describe what is embeddable. embedText is the per-row
// statement the live indexer runs; pendingSources is the set form the re-embed
// scan and the pending count share. Their own comment already says they must
// never diverge about what an entity's TEXT is — this holds the other half,
// which is which rows count at all.
//
// They diverged: embedText's activity statement carried `audience =
// 'workspace'` and pendingSources carried nothing. Nothing published, because
// UpsertEmbedding re-checks the audience on write — but every held message's
// subject and body was handed to the embedder on each binding change before
// that refusal, and the pending count named rows nothing would ever embed, so
// the backlog could not reach zero on an installation holding mail.
//
// It reads the DECLARATIONS rather than the behaviour because the divergence is
// a property of the pair, and a behavioural test needs a database and one
// seeded row per entity to say the same thing less directly.

import (
	"regexp"
	"testing"
)

// narrowingClause matches a predicate that restricts which rows are
// embeddable, as opposed to the liveness test both forms already carry.
//
// `archived_at` is excluded deliberately: both forms spell it, so matching it
// would report every entity as agreeing about a clause neither had to declare.
var narrowingClause = regexp.MustCompile(`(?i)\b(audience|restricted_at|visibility)\b`)

func TestTheSetFormAndThePerRowFormAgreeOnWhatIsEmbeddable(t *testing.T) {
	t.Parallel()
	if len(pendingSources) == 0 || len(embedText) == 0 {
		t.Fatal("one of the two declarations is empty, so this gate is comparing nothing")
	}
	for entityType, src := range pendingSources {
		perRow, declared := embedText[entityType]
		if !declared {
			t.Errorf("%s is in pendingSources and not in embedText: the set form would count and scan "+
				"rows the live indexer has no statement for", entityType)
			continue
		}
		wantNarrowed := narrowingClause.MatchString(perRow)
		gotNarrowed := narrowingClause.MatchString(src.embeddable)
		switch {
		case wantNarrowed && !gotNarrowed:
			t.Errorf("embedText[%s] narrows which rows are embeddable (%s) and pendingSources[%s] does not.\n"+
				"The scan then hands the embedder text from rows the per-row indexer refuses — paid for, sent "+
				"to the provider, and thrown away — and the pending count names work nothing will ever do.",
				entityType, narrowingClause.FindString(perRow), entityType)
		case !wantNarrowed && gotNarrowed:
			t.Errorf("pendingSources[%s] narrows on %s and embedText[%s] does not, so the set form is stricter "+
				"than the indexer: rows the live path embeds are never counted as pending and never re-embedded "+
				"on a binding change, which reads as a corpus that is complete when it is stale.",
				entityType, narrowingClause.FindString(src.embeddable), entityType)
		}
	}
	// The other direction: an entity the indexer knows and the set form does not
	// is one a binding change silently never rebuilds.
	for entityType := range embedText {
		if _, declared := pendingSources[entityType]; !declared {
			t.Errorf("%s is in embedText and not in pendingSources: the live indexer maintains it and no "+
				"re-embed pass will ever rebuild it after a binding change", entityType)
		}
	}
}

// And the clause itself agrees, not merely its presence: a set form narrowing
// on a DIFFERENT audience value than the indexer is the same defect wearing a
// passing gate.
func TestTheTwoFormsNarrowOnTheSameAudienceValue(t *testing.T) {
	t.Parallel()
	for entityType, src := range pendingSources {
		setForm := audienceValueIn(src.embeddable)
		perRow := audienceValueIn(embedText[entityType])
		if setForm == perRow {
			continue
		}
		t.Errorf("pendingSources[%s] admits audience %s and embedText[%s] admits %s. Both look narrowed and "+
			"they select different rows — the set form scans and counts one population while the indexer "+
			"maintains another, which is the original divergence with a gate in front of it.",
			entityType, quotedOrNone(setForm), entityType, quotedOrNone(perRow))
	}
}

// audienceValueIn answers the literal an `audience = '...'` comparison names,
// or "" when the SQL does not narrow on audience at all.
var audienceComparison = regexp.MustCompile(`(?i)\baudience\s*=\s*'([a-z_]+)'`)

func audienceValueIn(sql string) string {
	m := audienceComparison.FindStringSubmatch(sql)
	if m == nil {
		return ""
	}
	return m[1]
}

func quotedOrNone(value string) string {
	if value == "" {
		return "every row (no audience clause)"
	}
	return "'" + value + "'"
}
