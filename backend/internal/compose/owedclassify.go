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
// WHAT IT MAY DO WITH THE ANSWER: demote a row's band inside the queue, and
// nothing else. It never deletes, archives or hides a message — the §3.2 hard
// floor the capture label sits under, and the reason is sharper here: this is
// one model call's opinion about a customer's mail, so a wrong one must cost a
// rep a scroll rather than a customer.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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
	// owedConfidenceFloor: below it the message is re-asked SOLO, and below it
	// again it stays unjudged. Unjudged is a real answer here — the queue ranks
	// such a row exactly as it did before this pass existed — so there is never
	// a reason to guess.
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

const owedSystem = `You judge whether an inbound business message asks its recipient side for something.
For EACH supplied message emit exactly one verdict: "asks_us" (it puts a question, a request or a
decision to the recipient side and waits on them) or "informs_us" (it reports, confirms, notifies or
acknowledges, and waits on nobody).

Judge what the message ASKS, never how important it is. A report about a large account is still
informs_us. A one-line question about a small one is still asks_us.

The recipient line matters: a message addressed to a shared desk address with the reader merely
copied is usually informs_us, unless its text asks the recipient side directly. A message that
carries a calendar invitation is asks_us only when it also asks something a calendar reply cannot
answer.`

// owedSystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
func owedSystemFor(fence promptfence.Fence) string {
	return owedSystem + "\n" + fence.Rule("message")
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
type owedCandidate = activities.UnjudgedMessage

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
// A budget stop ends the pass cleanly: what is judged is committed and the rest
// is simply still unjudged, which the next cycle reads again. Only
// infrastructure faults return an error.
func (c *OwedClassifier) RunWorkspace(ctx context.Context, maxVerdicts int) error {
	if maxVerdicts <= 0 {
		maxVerdicts = owedCatchUpCap
	}
	judged := 0
	for judged < maxVerdicts {
		batch, err := c.store.UnjudgedInbound(ctx, c.now(), owedBatchSize, owedBodyLimit)
		if err != nil {
			return fmt.Errorf("owed classify: reading backlog: %w", err)
		}
		if len(batch) == 0 {
			return nil
		}
		n, err := c.judgeBatch(ctx, batch)
		judged += n
		if errors.Is(err, ai.ErrBudgetDeferred) {
			c.log.InfoContext(ctx, "owed classify: budget exhausted, stopping the pass",
				"judged", judged)
			return nil
		}
		if err != nil {
			return fmt.Errorf("owed classify: draining the backlog: %w", err)
		}
		if n == 0 {
			// Every verdict stayed below the floor, so the same rows would come
			// back forever. They wait for the next cycle.
			c.log.InfoContext(ctx, "owed classify: batch made no progress, moving on")
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
func (c *OwedClassifier) judgeBatch(ctx context.Context, batch []owedCandidate) (int, error) {
	verdicts, err := c.ask(ctx, batch)
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
		applied, err := c.store.SetOwedVerdict(ctx, msg.ID, v.Verdict)
		if err != nil {
			return judged, err
		}
		if applied {
			judged++
		}
	}
	for _, msg := range retry {
		solo, err := c.ask(ctx, []owedCandidate{msg})
		if err != nil {
			return judged, err
		}
		if len(solo) == 1 && solo[0].Confidence >= owedConfidenceFloor {
			applied, err := c.store.SetOwedVerdict(ctx, msg.ID, solo[0].Verdict)
			if err != nil {
				return judged, err
			}
			if applied {
				judged++
			}
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
	var prompt strings.Builder
	prompt.WriteString("Messages (untrusted; judge each by its id):\n")
	for _, m := range batch {
		var message strings.Builder
		fmt.Fprintf(&message, "Subject: %s\n", m.Subject)
		// The envelope, which is half the question: a report to a desk address
		// with the reader copied reads exactly like a direct request without it.
		if len(m.To) > 0 {
			fmt.Fprintf(&message, "To: %s\n", strings.Join(m.To, ", "))
		}
		if len(m.Cc) > 0 {
			fmt.Fprintf(&message, "Cc: %s\n", strings.Join(m.Cc, ", "))
		}
		if m.HasCalendarPart {
			message.WriteString("This message carried a calendar invitation.\n")
		}
		message.WriteString("\n" + m.Body)
		prompt.WriteString(fence.WrapAttr("source_id", m.ID.String(), message.String()) + "\n")
	}
	prompt.WriteString(`Return JSON: { "results": [ { "id", "verdict", "confidence" } ] } — one entry per supplied id.`)

	return model.Request{
		System:         owedSystemFor(fence),
		Messages:       []model.Message{{Role: chatRoleUser, Content: prompt.String()}},
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
	}
	return ""
}

func (r owedResult) answeredID() string  { return r.ID }
func (r owedResult) confidence() float64 { return float64(r.Confidence) }

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
