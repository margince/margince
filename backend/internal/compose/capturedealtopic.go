// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The capture deal topic check: capture_classify/deal_topic. Some deals capture
// only suggests: the company's one open deal, a stakeholder's deal, or one of
// several. Before filing one, a model judges whether the mail is about each
// candidate. Only a confident answer files; no answer files nothing.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/dealtopicprompt"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

const (
	// dealTopicConfidenceFloor is the confidence below which no answer files
	// or rules out a deal.
	dealTopicConfidenceFloor = 0.85
	// dealTopicBodyLimit truncates the mail body the model reads.
	dealTopicBodyLimit = 1500
	// dealTopicRecentSubjects is how many subjects of mail already on a deal
	// describe it.
	dealTopicRecentSubjects = 3
)

// The verdicts the check answers per deal, under dealTopicVerdictKey.
const (
	dealTopicVerdictKey = "verdict"

	dealTopicAbout    = "about"
	dealTopicNotAbout = "not_about"
	dealTopicUnsure   = "unsure"
)

var dealTopicVerdicts = map[string]bool{dealTopicAbout: true, dealTopicNotAbout: true, dealTopicUnsure: true}

// dealTopicInput and dealTopicDeal are the prompt's input.
type (
	dealTopicInput = dealtopicprompt.Input
	dealTopicDeal  = dealtopicprompt.Deal
)

type dealTopicResult struct {
	DealID     string            `json:"deal_id"`
	Verdict    string            `json:"verdict"`
	Confidence schema.Confidence `json:"confidence"`
}

func (r dealTopicResult) answeredID() string { return r.DealID }

func (r dealTopicResult) answering(id string) dealTopicResult { r.DealID = id; return r }

type dealTopicPayload struct {
	Results []dealTopicResult `json:"results"`
}

// dealTopicChecker is capture.DealTopicCheck over the capture_classify lane.
type dealTopicChecker struct {
	db    *database.DB
	brain completer
}

// NewDealTopicCheck is the check over brain, or nil when no model is
// configured, which capture reads as an abstention.
//
//nolint:ireturn // capture.DealTopicCheck IS the seam; a nil interface is how "no model" reaches capture.
func NewDealTopicCheck(pool *pgxpool.Pool, brain completer) capture.DealTopicCheck {
	if brain == nil {
		return nil
	}
	return dealTopicChecker{db: InstallationDB(pool), brain: brain}
}

// Digest names the question as of tx: the prompt and the whole input the
// model would read. A mail or a candidate the seat cannot read answers "".
func (dealTopicChecker) Digest(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deals []ids.UUID) (string, error) {
	in, readable, err := readDealTopicInput(ctx, tx, mail, deals)
	if err != nil || !readable {
		return "", err
	}
	return dealTopicDigest(in)
}

// dealTopicDigest names the prompt and the input it renders, so a changed
// deal name, project, filed subject or mail asks again.
func dealTopicDigest(in dealTopicInput) (string, error) {
	rendered, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("deal topic: digesting the input: %w", err)
	}
	sum := sha256.Sum256(append([]byte(dealtopicprompt.Ruleset+"\x00"), rendered...))
	return hex.EncodeToString(sum[:]), nil
}

// Check asks which of deals the mail is about. It abstains with
// capture.ErrTopicAbstained when the input it reads no longer digests to
// digest. It abstains too when no model is configured, the budget is spent or
// every model declines.
func (c dealTopicChecker) Check(ctx context.Context, mail ids.ActivityID, deals []ids.UUID, digest string) (capture.DealTopic, error) {
	var in dealTopicInput
	var readable bool
	err := c.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		in, readable, err = readDealTopicInput(ctx, tx, mail, deals)
		return err
	})
	if err != nil {
		return capture.DealTopic{}, err
	}
	if current, err := dealTopicDigest(in); err != nil || !readable || current != digest {
		return capture.DealTopic{}, errors.Join(capture.ErrTopicAbstained, err)
	}
	about := ai.WithSubject(ctx, mail.Ref(), in.Subject)
	resp, err := ai.Ask(about, c.brain, dealTopicRequest(in, model.AboutActivities(mail.UUID)), dealTopicShapeValid(in))
	if sweepPaused(err) || declinedByTheModels(err) || errors.Is(err, ai.ErrSubjectRestricted) {
		return capture.DealTopic{}, fmt.Errorf("%w: %w", capture.ErrTopicAbstained, err)
	}
	if err != nil {
		return capture.DealTopic{}, err
	}
	var payload dealTopicPayload
	if err := json.Unmarshal([]byte(ai.Unfence(resp.Text)), &payload); err != nil {
		return capture.DealTopic{}, fmt.Errorf("deal topic: unparseable model output: %w", err)
	}
	return dealTopicOf(resolveBatchRefs(payload.Results, dealtopicprompt.Refs(in)))
}

// dealTopicOf decides an answer. A deal judged about at or above the floor is
// filed only when every other deal is judged not_about at or above the floor.
// Every deal judged so rules them all out. Anything else is doubt.
func dealTopicOf(results []dealTopicResult) (capture.DealTopic, error) {
	var picked []string
	unsure := 0
	for _, r := range results {
		confident := r.Confidence >= dealTopicConfidenceFloor
		switch {
		case r.Verdict == dealTopicAbout && confident:
			picked = append(picked, r.DealID)
		case r.Verdict != dealTopicNotAbout || !confident:
			unsure++
		}
	}
	switch {
	case unsure > 0 || len(results) == 0 || len(picked) > 1:
		return capture.DealTopic{}, nil
	case len(picked) == 1:
		deal, err := ids.Parse(picked[0])
		if err != nil {
			return capture.DealTopic{}, fmt.Errorf("deal topic: the answer names %q, which is no deal id: %w", clampToken(picked[0]), err)
		}
		return capture.DealTopic{Deal: deal, Confident: true}, nil
	}
	return capture.DealTopic{Confident: true}, nil
}

// dealTopicRequest builds the one model call that judges one mail. It is a
// pure function of its input, so the certification case sends the request
// production sends.
//
//promptlang:exempt the reply is a closed set of verdict enum values keyed by deal id, never a sentence — validateDealTopicPayload refuses anything outside dealTopicSchema's vocabulary.
//promptvoice:exempt the reply is a closed set of verdict enum values keyed by deal id, never a sentence.
func dealTopicRequest(in dealTopicInput, about *model.Subjects) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         dealtopicprompt.SystemFor(fence),
		Messages:       []model.Message{{Role: chatRoleUser, Content: dealtopicprompt.Prompt(fence, in)}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: dealTopicSchema(dealtopicprompt.Refs(in).All()),
		SecretStripper: ai.NewSecretStripper(),
		Subjects:       about,
	}
}

// dealTopicShapeValid is the validator ai.Ask re-asks on.
func dealTopicShapeValid(in dealTopicInput) ai.Validator {
	return func(text string) error {
		var payload dealTopicPayload
		if err := json.Unmarshal([]byte(ai.Unfence(text)), &payload); err != nil {
			return fmt.Errorf("output is not the required JSON shape: %w", err)
		}
		if msg := validateDealTopicPayload(payload, in); msg != "" {
			return errors.New(msg)
		}
		return nil
	}
}

// validateDealTopicPayload names the first violation. A deal may be
// unanswered or answered twice, or named by a ref the prompt never showed. A
// verdict may fall outside the closed set, or a confidence outside [0,1].
func validateDealTopicPayload(payload dealTopicPayload, in dealTopicInput) string {
	refs := dealtopicprompt.Refs(in)
	if msg := checkBatchFidelity(payload.Results, refs.All()); msg != "" {
		return msg
	}
	for _, r := range resolveBatchRefs(payload.Results, refs) {
		if !dealTopicVerdicts[r.Verdict] {
			return fmt.Sprintf("verdict %q is not about|not_about|unsure", clampToken(r.Verdict))
		}
		if r.Confidence < 0 || r.Confidence > 1 {
			return fmt.Sprintf("confidence %v is outside [0,1]", r.Confidence)
		}
	}
	return ""
}

// dealTopicSchema is the generation-time shape guardrail.
func dealTopicSchema(requested []string) json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			owedResultsKey: schema.Array(schema.Object(
				map[string]schema.Node{
					dealIDField:             requestedIDNode(requested),
					dealTopicVerdictKey:     schema.Enum(dealTopicAbout, dealTopicNotAbout, dealTopicUnsure),
					extractionConfidenceKey: schema.Number(),
				},
				dealIDField, dealTopicVerdictKey, extractionConfidenceKey,
			)),
		},
		owedResultsKey,
	))
}
