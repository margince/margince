// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The deal topic check. Some deals the rule only suggests: one open deal, a
// stakeholder's, or one of several. Such a deal is filed only when a model is
// highly confident the mail is about it. Explicit evidence (the mail's project, a
// proven thread) files without one. The model is never called inside a
// transaction: a filing decides, asks, and decides again under the row lock.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DealBasis is the rung of the deal rule that reached a verdict.
type DealBasis string

// The rungs, explicit evidence first.
const (
	DealBasisProject      DealBasis = "project"
	DealBasisThread       DealBasis = "thread"
	DealBasisSoleOpenDeal DealBasis = "sole_open_deal"
	DealBasisStakeholder  DealBasis = "stakeholder"
	DealBasisAmbiguous    DealBasis = "ambiguous"
)

// NeedsTopicCheck says which verdicts are suggestions
// the topic check must confirm.
func (b DealBasis) NeedsTopicCheck() bool {
	return b == DealBasisSoleOpenDeal || b == DealBasisStakeholder || b == DealBasisAmbiguous
}

// DealTopicCandidates bounds the deals one check weighs for an ambiguous mail.
const DealTopicCandidates = 5

// DealTopic is a topic check's answer. A confident zero Deal says the mail is
// about none of the deals asked about.
type DealTopic struct {
	Deal      ids.UUID
	Confident bool
}

// ErrTopicAbstained is a check that gave no answer: no model is configured,
// the budget is spent, or every model declined. The mail is then filed on no
// suggested deal, and a later pass asks again.
var ErrTopicAbstained = errors.New("capture: the deal topic check gave no answer")

// DealTopicCheck asks a model which of deals the mail is about. Compose
// injects it; a nil check abstains.
type DealTopicCheck interface {
	Check(ctx context.Context, mail ids.ActivityID, deals []ids.UUID) (DealTopic, error)
	// Ruleset names the prompt, so a changed prompt asks again.
	Ruleset() string
}

// TxRunner opens one transaction for the filing's caller.
type TxRunner interface {
	Tx(ctx context.Context, fn func(pgx.Tx) error) error
}

// dealTopicState carries the check through a filing, and its answer once
// asked. A nil state abstains.
type dealTopicState struct {
	check  DealTopicCheck
	answer *topicAnswer
}

// topicAnswer is one check's answer for the candidate set its digest names.
// answered is false for an abstention.
type topicAnswer struct {
	digest   string
	topic    DealTopic
	answered bool
}

// topicQuestion is what a filing could not settle without the model.
type topicQuestion struct {
	decider context.Context
	mail    ids.ActivityID
	deals   []ids.UUID
	digest  string
}

// dealTopicPending is a filing that stopped to ask the model. It never leaves
// this package: the second transaction settles it.
const dealTopicPending DealOutcome = "topic_pending"

// topicDigest names one question: the prompt and the candidate deals, in any
// order.
func topicDigest(ruleset string, deals []ids.UUID) string {
	keys := make([]string, len(deals))
	for i, deal := range deals {
		keys[i] = deal.String()
	}
	slices.Sort(keys)
	sum := sha256.Sum256([]byte(ruleset + "\x00" + strings.Join(keys, ",")))
	return hex.EncodeToString(sum[:])
}

// checkedFiling runs file in one transaction. When it leaves a question, it
// asks the check outside any transaction and runs file again with the answer.
// It reports whether the model was asked.
func checkedFiling(
	ctx context.Context, db TxRunner, check DealTopicCheck, file func(tx pgx.Tx, topic *dealTopicState) (*topicQuestion, error),
) (bool, error) {
	var topic *dealTopicState
	if check != nil {
		topic = &dealTopicState{check: check}
	}
	var question *topicQuestion
	run := func() error {
		return db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			question, err = file(tx, topic)
			return err
		})
	}
	if err := run(); err != nil || question == nil {
		return false, err
	}
	answer, err := askTopic(check, *question)
	if err != nil {
		return true, err
	}
	topic.answer = &answer
	if err := run(); err != nil {
		return true, err
	}
	return true, nil
}

// askTopic asks the check one question. An abstention is an answer of its
// own; any other fault is the caller's.
func askTopic(check DealTopicCheck, q topicQuestion) (topicAnswer, error) {
	topic, err := check.Check(q.decider, q.mail, q.deals)
	if errors.Is(err, ErrTopicAbstained) {
		return topicAnswer{digest: q.digest}, nil
	}
	if err != nil {
		return topicAnswer{}, err
	}
	return topicAnswer{digest: q.digest, topic: topic, answered: true}, nil
}

// settleSuggestion decides a suggested verdict by the check's answer. A
// confident pick among the candidates files that deal. A confident "none"
// files nothing and asks nobody. Doubt or no answer files nothing, and an
// ambiguous mail still asks its question.
func settleSuggestion(verdict DealVerdict, answer topicAnswer) DealVerdict {
	none := DealVerdict{Outcome: DealNone, Basis: verdict.Basis}
	switch {
	case !answer.answered || !answer.topic.Confident:
		if verdict.Outcome == DealAmbiguous {
			return verdict
		}
		return none
	case answer.topic.Deal.IsZero() || !slices.Contains(verdict.Candidates, answer.topic.Deal):
		return none
	}
	return DealVerdict{Outcome: DealLinked, Deal: answer.topic.Deal, Basis: verdict.Basis, Candidates: verdict.Candidates}
}

// decideFiling is the deal rule with the topic check folded in. A suggestion
// whose question was already answered for this mail files nothing more. One
// still to ask comes back as a question, with no verdict to write.
func decideFiling(f dealFiling, tx pgx.Tx, facts dealFacts, candidates DealCandidates) (DealVerdict, *topicQuestion, error) {
	verdict, err := decideDeal(f.decider, tx, facts, candidates)
	if err != nil || !verdict.Basis.NeedsTopicCheck() || len(verdict.Candidates) == 0 {
		return verdict, nil, err
	}
	if f.topic == nil {
		return settleSuggestion(verdict, topicAnswer{}), nil, nil
	}
	digest := topicDigest(f.topic.check.Ruleset(), verdict.Candidates)
	answer := f.topic.answer
	switch {
	case facts.topicMemo == digest:
		return DealVerdict{Outcome: DealNone, Basis: verdict.Basis}, nil, nil
	case answer == nil:
		question := &topicQuestion{decider: f.decider, mail: facts.activity, deals: verdict.Candidates, digest: digest}
		return DealVerdict{Outcome: dealTopicPending, Basis: verdict.Basis}, question, nil
	case answer.digest != digest:
		return settleSuggestion(verdict, topicAnswer{}), nil, nil
	}
	return settleSuggestion(verdict, *answer), nil, nil
}

// FileOnDealChecked files one captured activity under the deal the rule finds,
// with a suggested deal confirmed by rule.Check between two transactions. It
// is how live capture files; the refile pass and the recheck share its steps.
func FileOnDealChecked(ctx context.Context, db TxRunner, mail ids.ActivityID, rule DealAttribution) (DealVerdict, error) {
	var verdict DealVerdict
	_, err := checkedFiling(ctx, db, rule.Check, func(tx pgx.Tx, topic *dealTopicState) (*topicQuestion, error) {
		var question *topicQuestion
		var err error
		f := dealFiling{decider: ctx, writer: ctx, filer: rule.Filer, topic: topic}
		verdict, question, err = fileOnDeal(f, tx, mail, rule.Candidates, rule.Ask)
		return question, err
	})
	return verdict, err
}
