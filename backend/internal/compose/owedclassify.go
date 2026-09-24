// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Judging whether an unanswered message actually asks us for anything.
//
// The waiting queue proves a customer wrote and nobody replied. It cannot tell
// an unanswered question from a report, a receipt or a monthly statement — and
// two of the three messages that opened this work were correctly unanswered and
// correctly nobody's job. This pass reads what survives the queue's own rules
// and says which of those messages is asking.
//
// It is the capture-classify shape (ADR-0063) applied to a different question,
// and it reuses that engine's mechanics deliberately: the backlog is a query
// rather than a work table, each call commits, the fence is minted per call,
// the schema and a deterministic validator both bound the answer, and a verdict
// below the confidence floor is re-asked once and then left unjudged.
//
// Uncertain mail remains reviewable. Confirmed requests can produce a personal
// reminder through the activities writer, preserving the message as evidence.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/owedverdict"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

const (
	// owedBatchSize is how many messages one call judges. Ten, like the
	// capture-label pass: the same bodies, the same truncation, the same
	// light-tier window.
	owedBatchSize = 10
	// owedBodyLimit truncates each body for the prompt.
	owedBodyLimit = 1500
	// owedPriorBodyLimit truncates OUR earlier message in the thread, which
	// rides along as context.
	//
	// Shorter than the judged body on purpose. It is there to say what the
	// customer is answering, not to be judged itself, and reusing 1500 would
	// double a full batch to some thirty thousand characters — which crowds the
	// small-local window this task's ladder starts on, for context that only has
	// to carry the gist.
	owedPriorBodyLimit = 600
	// owedRestaleCap bounds the re-judge sweep per workspace per pass,
	// separately from owedCatchUpCap. Two budgets because they are two
	// populations: new mail must never wait behind a sweep of history.
	owedRestaleCap = 200
	// owedConfidenceFloor: below it the message is re-asked SOLO, and below it
	// again it stays unjudged and reviewable, without claiming Focus priority.
	owedConfidenceFloor = 0.7
	// owedCatchUpCap bounds one pass PER WORKSPACE. Per workspace rather than
	// shared, for the reason capture_counterparty_verdict states: a shared
	// counter lets one large backlog spend the budget and starve every
	// workspace behind it.
	owedCatchUpCap = 500
)

// owedVerdicts is the closed set the validator admits, derived from the store's
// own constants rather than retyped: the column has a CHECK constraint on these
// two words, and a third spelling here would be a verdict the database refuses
// after the model call has already been paid for.
var owedVerdicts = map[string]bool{
	activities.OwedVerdictAsksUs:    true,
	activities.OwedVerdictInformsUs: true,
}

// OwedClassifier drives the verdict pass for one workspace at a time.
//
// The column belongs to activities; this engine reads and writes it only
// through that store, like the capture-label engine beside it.
type OwedClassifier struct {
	pool  *pgxpool.Pool
	store *activities.Store
	brain completer
	log   *slog.Logger
	// now bounds the backlog read at an instant, and is a field so a test can
	// pin it: the candidates are the WAITING queue's, whose own horizon and
	// staleness rules are measured from this moment.
	now func() time.Time
}

// NewOwedClassifier builds the engine over the pool and one model lane.
//
// The own-domain seam is bound here because the backlog is the WAITING QUEUE
// narrowed to unjudged rows, and that queue excludes a colleague's message.
// Unbound, the pass would spend model calls judging internal mail the queue
// would never show anybody.
func NewOwedClassifier(pool *pgxpool.Pool, brain completer, now func() time.Time, log *slog.Logger) *OwedClassifier {
	if now == nil {
		now = time.Now
	}
	db := InstallationDB(pool)
	return &OwedClassifier{
		pool: pool,
		store: activities.NewStore(db).WithOwnDomains(
			ownDomainReader{store: capture.NewOwnDomainStore(db)}),
		brain: brain,
		log:   log,
		now:   now,
	}
}

// owedCandidate is one backlog row as the prompt sees it.
type owedCandidate = activities.OwedCandidate

// The JSON keys the schema declares and the payload decodes.
//
// Named rather than typed twice, because a schema key that stops matching its
// struct tag decodes to a zero value — and a zero verdict then fails the
// validator as "not asks_us|informs_us", which is a confusing way to learn that
// a key was renamed in one place and not the other.
const (
	owedResultsKey = "results"
	owedVerdictKey = "verdict"
)

// owedResult is one model verdict.
type owedResult struct {
	ID         string            `json:"id"`
	Verdict    string            `json:"verdict"` // owedVerdictKey
	Confidence schema.Confidence `json:"confidence"`
}

type owedPayload struct {
	Results []owedResult `json:"results"` // owedResultsKey
}

// RunWorkspace judges up to cap backlog messages in the workspace bound in ctx.
//
// A budget stop, or no provider bound, ends the pass cleanly: what is judged is
// committed and the rest is simply still unjudged, which the next cycle reads
// again. Only infrastructure faults return an error.
func (c *OwedClassifier) RunWorkspace(ctx context.Context, maxVerdicts int) error {
	if err := c.store.CaptureEmailRequests(ctx, c.now()); err != nil {
		return err
	}
	if err := c.judgeWorkspace(ctx, maxVerdicts); err != nil {
		return err
	}
	return c.store.CaptureEmailRequests(ctx, c.now())
}

// judgeWorkspace judges new mail first, then re-judges what older rules decided.
//
// THE ORDER IS A RULE, not an accident of statement order. The two populations
// have separate budgets and separate reads precisely so that a sweep of history
// can never delay a customer who wrote this morning: on the deploy that moves
// the prompt, every judged row in the installation is stale at once, and a
// single shared bound would spend the whole pass on them.
func (c *OwedClassifier) judgeWorkspace(ctx context.Context, maxVerdicts int) error {
	if c.brain == nil {
		return nil
	}
	if maxVerdicts <= 0 {
		maxVerdicts = owedCatchUpCap
	}
	if err := c.drain(ctx, maxVerdicts, "backlog", func() ([]owedCandidate, time.Time, error) {
		return c.store.OwedBacklog(ctx, c.now(), owedBatchSize, owedBodyLimit, owedPriorBodyLimit)
	}); err != nil {
		return err
	}
	return c.drain(ctx, owedRestaleCap, "re-judge", func() ([]owedCandidate, time.Time, error) {
		return c.store.OwedRestale(ctx, owedverdict.Ruleset, owedBatchSize, owedBodyLimit, owedPriorBodyLimit)
	})
}

// drain judges one population until it is empty, its budget is spent, or it
// stops making progress. `what` names it in the log, so a pass that ends early
// says which half ended.
//
// Bounded by the model calls it makes as well as by verdicts written, and the
// second bound is the one that has to exist: a message the model will not
// commit to stays unjudged and is read again, so a batch mixing one confident
// row with nine abstentions makes progress by the verdict count and re-asks the
// same nine every iteration. sweepCalls counts every call, a split batch's
// included, so the bound holds whatever the batches do.
func (c *OwedClassifier) drain(ctx context.Context, maxVerdicts int, what string, next func() ([]owedCandidate, time.Time, error)) error {
	judged := 0
	calls := newSweepCalls(maxVerdicts, owedBatchSize)
	for judged < maxVerdicts && calls.remain() {
		// The instant this batch was READ, on the DATABASE's clock, which the
		// write compares against so a slow call cannot overwrite a verdict
		// reached while it was thinking.
		batch, readAt, err := next()
		if err != nil {
			return fmt.Errorf("owed classify: reading %s: %w", what, err)
		}
		if len(batch) == 0 {
			return nil
		}
		n, err := c.judgeBatch(ctx, batch, readAt, calls)
		judged += n
		if sweepPaused(err) {
			c.log.InfoContext(ctx, "owed classify: stopping the pass",
				"population", what, "judged", judged, "reason", err)
			return nil
		}
		if err != nil {
			return fmt.Errorf("owed classify: draining %s: %w", what, err)
		}
		if n == 0 {
			// Every verdict stayed below the floor, so the same rows would come
			// back forever. They wait for the next cycle.
			c.log.InfoContext(ctx, "owed classify: batch made no progress, moving on",
				"population", what)
			return nil
		}
	}
	return nil
}

// judgeBatch judges one batch with ONE model call and commits each verdict.
//
// The per-call commit IS the checkpoint. A message below the floor is re-asked
// on its own — which escalates the routing ladder by being its own structured
// call — and one still below it afterwards is left unjudged rather than guessed.
// A batch the models decline is asked message by message, so the one message
// they will not judge is recorded declined and the rest of the batch proceeds.
func (c *OwedClassifier) judgeBatch(ctx context.Context, batch []owedCandidate, readAt time.Time, calls *sweepCalls) (int, error) {
	if !calls.take() {
		return 0, nil
	}
	verdicts, err := c.ask(ctx, batch)
	if declinedByTheModels(err) {
		return c.judgeEach(ctx, batch, readAt, calls)
	}
	if err != nil {
		return 0, err
	}
	judged := 0
	var retry []owedCandidate
	byID := map[string]owedCandidate{}
	for _, m := range batch {
		byID[m.ID.String()] = m
	}
	for _, v := range verdicts {
		msg, ok := byID[v.ID]
		if !ok {
			continue // the validator guarantees this; belt and braces
		}
		if v.Confidence < owedConfidenceFloor {
			retry = append(retry, msg)
			continue
		}
		applied, err := c.store.SetOwedVerdict(ctx, msg.ID, v.Verdict, owedverdict.Ruleset, readAt)
		if err != nil {
			return judged, err
		}
		if applied {
			judged++
		}
	}
	solo, err := c.judgeEach(ctx, retry, readAt, calls)
	return judged + solo, err
}

// judgeEach asks about each message in its own call and commits a verdict
// above the floor. A message every rung declines is that message's outcome: it
// is recorded declined, which takes it out of both populations for this pass
// and every later one, and the next message is asked.
func (c *OwedClassifier) judgeEach(ctx context.Context, msgs []owedCandidate, readAt time.Time, calls *sweepCalls) (int, error) {
	judged := 0
	for _, msg := range msgs {
		if !calls.take() {
			return judged, nil
		}
		solo, err := c.ask(ctx, []owedCandidate{msg})
		if declinedByTheModels(err) {
			recorded, markErr := c.store.MarkOwedVerdictDeclined(ctx, msg.ID)
			if markErr != nil {
				return judged, markErr
			}
			c.log.WarnContext(ctx, "owed classify: the models declined one message, which stays unjudged",
				"activity_id", msg.ID, "recorded", recorded, "err", err)
			continue
		}
		if err != nil {
			return judged, err
		}
		if len(solo) != 1 || solo[0].Confidence < owedConfidenceFloor {
			continue
		}
		applied, err := c.store.SetOwedVerdict(ctx, msg.ID, solo[0].Verdict, owedverdict.Ruleset, readAt)
		if err != nil {
			return judged, err
		}
		if applied {
			judged++
		}
	}
	return judged, nil
}

// owedRequest builds the ONE model call that judges one batch.
//
// A pure function of the batch, so the certification lane issues the request
// that SHIPS rather than a re-creation of it — a copy certifies a copy.
//
// One fence for the whole call, wrapping each message in its own span. The
// prompt carries several senders at once and none of them has seen the nonce,
// so no message can close its own span and reach the text of another sender's
// mail to have it judged.
//
//promptlang:exempt the reply is a closed set of verdict enum values keyed by id, never a sentence — validateOwedPayload refuses anything outside owedSchema's vocabulary, so a language instruction could only turn an enum into a parse failure.
//promptvoice:exempt the reply is a closed set of verdict enum values keyed by id, never a sentence.
func owedRequest(batch []owedCandidate) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         owedverdict.SystemFor(fence),
		Messages:       []model.Message{{Role: chatRoleUser, Content: owedverdict.Prompt(fence, batch)}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: owedSchema(),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// ask makes one structured call for the given messages.
func (c *OwedClassifier) ask(ctx context.Context, batch []owedCandidate) ([]owedResult, error) {
	resp, err := ai.Ask(ctx, c.brain, owedRequest(batch), owedShapeValid(batch))
	if err != nil {
		return nil, err
	}
	var payload owedPayload
	if err := json.Unmarshal([]byte(ai.Unfence(resp.Text)), &payload); err != nil {
		return nil, fmt.Errorf("owed classify: unparseable model output: %w", err)
	}
	if msg := validateOwedPayload(payload, batch); msg != "" {
		return nil, fmt.Errorf("owed classify: %s", msg)
	}
	return payload.Results, nil
}

// owedShapeValid is the deterministic hard floor beneath the response schema:
// every requested id exactly once, ids verbatim, verdicts in the closed set.
func owedShapeValid(batch []owedCandidate) ai.Validator {
	return func(text string) error {
		var payload owedPayload
		if err := json.Unmarshal([]byte(ai.Unfence(text)), &payload); err != nil {
			return fmt.Errorf("output is not the required JSON shape: %w", err)
		}
		if msg := validateOwedPayload(payload, batch); msg != "" {
			return errors.New(msg)
		}
		return nil
	}
}

// validateOwedPayload names the first batch-fidelity violation, or "" when the
// payload is exact.
func validateOwedPayload(payload owedPayload, batch []owedCandidate) string {
	requested := make([]string, len(batch))
	for i, m := range batch {
		requested[i] = m.ID.String()
	}
	if msg := checkBatchFidelity(payload.Results, requested); msg != "" {
		return msg
	}
	// The vocabulary is this site's own, so it is checked here rather than in
	// the shared contract: "is this a verdict" and "is this a label" are
	// different questions, and an error naming the wrong closed set sends a
	// reader to the wrong prompt.
	for _, r := range payload.Results {
		if !owedVerdicts[r.Verdict] {
			return fmt.Sprintf("verdict %q is not asks_us|informs_us", clampToken(r.Verdict))
		}
		if r.Confidence < 0 || r.Confidence > 1 {
			return fmt.Sprintf("confidence %v is outside [0,1]", r.Confidence)
		}
	}
	return ""
}

func (r owedResult) answeredID() string { return r.ID }

// owedSchema is the generation-time shape guardrail.
func owedSchema() json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			owedResultsKey: schema.Array(schema.Object(
				map[string]schema.Node{
					"id":                    schema.String(),
					owedVerdictKey:          schema.Enum(activities.OwedVerdictAsksUs, activities.OwedVerdictInformsUs),
					extractionConfidenceKey: schema.Number(),
				},
				"id", owedVerdictKey, extractionConfidenceKey)),
		},
		owedResultsKey))
}
