// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import "github.com/margince/margince/backend/internal/shared/kernel/threadverdict"

// The judgements a deal link carries in activity_link.deal_relevance. A link
// nobody judged holds NULL, and it counts.
const (
	DealRelevant  = "relevant"
	DealUnrelated = "unrelated"
	DealUncertain = "uncertain"
)

// Who judged a deal link, in activity_link.relevance_source. The classifier
// never overwrites a human's judgement.
const (
	RelevanceByHuman      = "human"
	RelevanceByClassifier = "classifier"
)

// DealEvidenceSQL admits an activity that may speak for deal being alive. It
// refuses what is personal (NotPersonalSQL) and an activity whose link to deal
// is judged unrelated or uncertain. deal is a compile-time SQL expression. The
// deal clock's migration carries this text.
func DealEvidenceSQL(activity, deal string) string {
	return evidenceRefusingMarks(activity, deal, "'"+DealUnrelated+"', '"+DealUncertain+"'")
}

// DealKeptOpenSQL is DealEvidenceSQL that still admits a link the classifier
// is unsure of. Doubt warms no deal, and it closes none for silence either.
func DealKeptOpenSQL(activity, deal string) string {
	return evidenceRefusingMarks(activity, deal, "'"+DealUnrelated+"'")
}

func evidenceRefusingMarks(activity, deal, refused string) string {
	return "(" + NotPersonalSQL(activity) + " AND NOT EXISTS (SELECT 1 FROM activity_link judged WHERE judged.activity_id = " +
		activity + ".id AND judged.deal_id = " + deal + " AND judged.deal_relevance IN (" + refused + ")))"
}

// NotPersonalSQL refuses only what the product already decided is personal: a
// thread a seat holds as personal, or a row whose audience reason says so. A
// calendar privacy mark decides nothing, since business meetings carry one too.
func NotPersonalSQL(alias string) string {
	return "(coalesce(" + alias + ".audience_reason, '') <> '" +
		threadverdict.ReasonForKind(threadverdict.KindPersonal) + "' AND " + threadverdict.NoPersonalHoldSQL(alias) + ")"
}

// DealLinkStandsSQL holds for a deal link nobody marked unrelated. Only such a
// link lends the activity its deal's project, or files the rest of its thread
// on that deal. link is the activity_link alias.
func DealLinkStandsSQL(link string) string {
	return link + ".deal_relevance IS DISTINCT FROM '" + DealUnrelated + "'"
}
