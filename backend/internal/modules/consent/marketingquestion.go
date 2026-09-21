// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The question a subscription consent is given TO.
//
// A consent granted through a confirm link recorded policy_text from the
// SUBMISSION BODY — whatever the subject's browser said it had shown them. So
// the proof a controller produces to show consent was freely given quoted a
// sentence the client chose, and a client that chose a different one would be
// believed. Meanwhile consent_event.consent_text_version_id has sat unwritten
// since migration 1788669014.
//
// WHY BINDING THE MAIL'S WORDING IS NOT THE FIX, which is what an earlier
// attempt did and had to be reverted. The mail says "confirming below turns
// that into a permission we will act on". The PAGE asks something else: "news
// from time to time, roughly once a month". A subject agrees to the second. A
// proof naming the first would be checkable, published, server-controlled — and
// about the wrong proposition, which is worse than an unchecked quote of the
// right one.
//
// So the page's own question is published here, in the same three languages and
// through the same text-version machinery, and that is what a grant names.
//
// THE ANSWERS TRAVEL WITH IT. What somebody agreed to is the question and the
// option they picked together: "roughly once a month" answered with "no thanks"
// is not a consent, and a version pinning only the question would let the
// labels change under it. One published body carries all three.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// TemplateMarketingQuestion is the published key for the subscription question.
//
// A TEMPLATE KEY OF ITS OWN, not a field on the confirm-mail templates, because
// the two change for different reasons and a shared version would tie them
// together: rewording the mail's invitation would invalidate every grant's
// pinned question, and rewording the question would claim the mail changed.
const TemplateMarketingQuestion = "marketing_question"

// TemplateSubscriptionQuestion is the dedicated subscription link's question,
// which names the purpose rather than describing a frequency.
//
// A SECOND KEY, because they are two different propositions and a subject
// answers exactly one. Binding either door to the other's sentence would record
// an agreement to something nobody was asked — which is the defect this whole
// change exists to end, and the one an earlier attempt shipped.
const TemplateSubscriptionQuestion = "subscription_question"

// marketingQuestionVersion is bumped when the question or either answer
// changes. A version that moved for any other reason would tell a reader the
// proposition changed when it did not.
//
// Held by: TestThePublishedQuestionIsTheOneTheScreenAsks
// (backend/gates/marketingquestion_test.go), which compares this catalog
// against the frontend's own strings.
const marketingQuestionVersion = 1

// QuestionKeyForLink names which published question a link will ask.
//
// The two confirm links show different pages. A record-confirmation link shows
// the generic marketing question with its yes/no; a consent link shows the
// named-purpose subscription question with one affirmative button.
//
// The key is decided at MINT, from the link kind, which is the same fact the
// page branches on when it chooses which body to render — so the two agree
// about WHICH question is asked. They can still disagree about its exact words
// and language; see boundquestion.go for what the pin does and does not
// settle.
func QuestionKeyForLink(kind string) string {
	if kind == LinkConsentConfirmation {
		return TemplateSubscriptionQuestion
	}
	return TemplateMarketingQuestion
}

// RenderQuestion resolves one published question into the body a proof row
// points at.
//
// THE PURPOSE IS NOT SUBSTITUTED HERE. The subscription question carries a
// {purpose} placeholder, and the published row keeps it: what is published is
// the installation's wording, and the purpose a given link named is already on
// the proof row beside it. Substituting would publish one row per purpose and
// make the version mean something different for each.
func RenderQuestion(key, language string) string {
	words := mailcopy.For(language)
	if key == TemplateSubscriptionQuestion {
		return strings.Join([]string{
			words.ConfirmSubscriptionAsk,
			words.ConfirmSubscriptionConfirm,
		}, "\n")
	}
	return RenderMarketingQuestion(language)
}

// RenderMarketingQuestion resolves the question and its two answers into the
// one body a proof row points at.
//
// ONE BODY RATHER THAN THREE COLUMNS, because what is being pinned is a whole
// proposition. A reader following consent_text_version_id wants to see what the
// subject was asked and what they could say, in the order they saw it.
func RenderMarketingQuestion(language string) string {
	words := mailcopy.For(language)
	return strings.Join([]string{
		words.ConfirmMarketingAsk,
		words.ConfirmMarketingYes,
		words.ConfirmMarketingNo,
	}, "\n")
}

// PublishMarketingQuestionTx publishes the question in every language this
// build speaks, beside the controller templates and for the same reason: the
// installation's language can change after a link was sent, so a wording
// published only for the current setting would disappear from a later boot and
// the proof would name a row nobody could read.
func PublishMarketingQuestionTx(ctx context.Context, tx pgx.Tx, now time.Time) error {
	for _, key := range []string{TemplateMarketingQuestion, TemplateSubscriptionQuestion} {
		for _, language := range mailcopy.Languages() {
			if _, err := PublishTextVersionTx(ctx, tx, TextVersion{
				Key:         key,
				Version:     fmt.Sprintf("v%d", marketingQuestionVersion),
				Locale:      string(language),
				Body:        RenderQuestion(key, string(language)),
				PublishedAt: now,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
