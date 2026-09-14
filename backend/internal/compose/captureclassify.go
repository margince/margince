// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The batched capture-classify engine (ai-operational-spec §2.8, ADR-0063):
// label captured mail commitment | meeting | noise for attention routing.
// The backlog IS the partial index (capture_label IS NULL on connector
// email activities) — no work table; each model call labels up to ten
// messages and COMMITS per call, so a budget stop or a crash loses nothing
// and the remainder simply stays unlabeled for the next cycle. A noise
// label demotes attention only — it never deletes, archives, or suppresses
// the record an email created (§3.2 hard floor).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

const (
	// classifyBatchSize is the AIRT-PARAM-35 batch pin: ten messages per
	// call, bodies truncated, fits the light tier's window.
	classifyBatchSize = 10
	// classifyBodyLimit truncates each body for the prompt (AIRT-PARAM-35).
	classifyBodyLimit = 1500
	// classifyConfidenceFloor: below it the item is re-asked SOLO on the
	// routing ladder's fallback rather than guessed in-batch (§2.8).
	classifyConfidenceFloor = 0.7
	// classifierUnreported stands where a provider named no model. Spelled out
	// rather than left empty so a stored row says "we do not know what judged
	// this" instead of looking like a column nobody filled.
	classifierUnreported = "unreported"
	// classifyCatchUpCap bounds one catch-up pass (ADR-0063: hourly cap
	// 500); the nightly pass runs the same engine with a higher cap.
	classifyCatchUpCap = 500
)

var classifyLabels = map[string]bool{"commitment": true, "meeting": true, "noise": true}

// replyVerdicts is this site's second closed set. Its members are the module's
// own constants rather than three more string literals, so the prompt's
// vocabulary and the column's CHECK constraint cannot drift into disagreeing.
//
// Held by: TestTheReplyVerdictVocabularyIsSpelledOnceEverywhere
// (backend/gates/replyverdictvocabulary_test.go)
var replyVerdicts = map[string]bool{
	activities.ReplyVerdictPositive: true,
	activities.ReplyVerdictNegative: true,
	activities.ReplyVerdictNeutral:  true,
}

// yesNo renders the direction flag for the prompt's own line.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

const classifySystem = `You label captured emails for attention routing. For EACH supplied message emit exactly one
label: "commitment" (a promise or request to act), "meeting" (scheduling or follow-through),
or "noise" (neither). Labels route attention; they change no data. If a message fits both
commitment and meeting, choose commitment.

A message marked "inbound: yes" was sent TO us by someone outside. For those, ALSO judge how
they answered: "positive" (interest, a question worth answering, a request to meet or to hear
more), "negative" (not interested, the wrong contact with no referral, a request to stop
writing), or "neutral" (neither — an out-of-office, a bare acknowledgement, a redirect with no
view of its own). Omit "reply" entirely for a message marked "inbound: no": we wrote it, so it
answers nobody. Omit it too when the message does not read as an answer at all. A guess here
becomes a number somebody is measured on, so leave it out when you cannot tell.

"confidence" covers EVERY judgement you emit for that message — the label and, when you give
one, the reply. Report the LOWEST of the two, not the label's alone. If you are sure of the
label and unsure of the reply, either omit the reply or let the lower number stand for both.`

// classifySystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
func classifySystemFor(fence promptfence.Fence) string {
	return classifySystem + "\n" + fence.Rule("message")
}

// CaptureClassifier drives the batched label pass for every workspace.
// The label columns are the activities module's; this engine reads and
// writes them only through its store.
type CaptureClassifier struct {
	pool  *pgxpool.Pool
	store *activities.Store
	brain completer
	log   *slog.Logger
}

// NewCaptureClassifier builds the engine over the pool and one model lane.
func NewCaptureClassifier(pool *pgxpool.Pool, brain completer, log *slog.Logger) *CaptureClassifier {
	return &CaptureClassifier{pool: pool, store: activities.NewStore(InstallationDB(pool)), brain: brain, log: log}
}

// unlabeledMessage is one backlog row as the prompt sees it.
type unlabeledMessage = activities.UnlabeledEmail

// classifyResult is one model verdict.
type classifyResult struct {
	ID         string            `json:"id"`
	Label      string            `json:"label"`
	Confidence schema.Confidence `json:"confidence"`
	// Reply is whether an INBOUND message was a positive, negative or neutral
	// answer, and is empty on an outbound one — a message we sent is not a reply
	// anybody gave us.
	//
	// It rides this call rather than taking one of its own. The pass has already
	// been handed the text and already paid for the tokens, so a second question
	// about the same message costs a field rather than a call; a separate reply
	// pass would double the per-message cost to re-read what this one was given.
	Reply string `json:"reply,omitempty"`
}

type classifyPayload struct {
	Results []classifyResult `json:"results"`
}

// RunWorkspace drains up to cap backlog messages in the workspace already
// bound in ctx. A budget stop ends the pass cleanly — the remainder requeues
// implicitly (it is simply still unlabeled). Only infrastructure faults return
// an error; per-batch model trouble is logged and skipped.
//
// The cap is PER WORKSPACE, matching capture_counterparty_verdict, whose own
// counter is declared inside its workspace loop for a stated reason: a shared
// counter lets one large backlog consume the whole budget and starve every
// workspace after it. The two sibling passes implementing the same ADR-0063
// shape disagreed; this resolves them toward the one carrying a rationale.
// The number itself is unchanged.
func (c *CaptureClassifier) RunWorkspace(ctx context.Context, maxLabels int) error {
	if maxLabels <= 0 {
		maxLabels = classifyCatchUpCap
	}
	labeled := 0
	for labeled < maxLabels {
		batch, err := c.store.UnlabeledCaptureEmails(ctx, classifyBatchSize, classifyBodyLimit)
		if err != nil {
			return fmt.Errorf("classify: reading backlog: %w", err)
		}
		if len(batch) == 0 {
			return nil
		}
		n, err := c.classifyBatch(ctx, batch)
		labeled += n
		if errors.Is(err, ai.ErrBudgetDeferred) {
			// ≥100% band: non-interactive work stops for this cycle;
			// what is labeled is committed, the rest waits (§2.8).
			c.log.InfoContext(ctx, "capture classify: budget exhausted, stopping the pass", "labeled", labeled)
			return nil
		}
		if err != nil {
			// This workspace's pass FAILS. Before the fan-out a bad batch was
			// logged and skipped so it could not starve the rest of the fleet;
			// each workspace now has its own row, so there is no fleet left to
			// starve and swallowing it would put the green row back.
			//
			// The cost is named rather than hidden: the backlog read re-selects
			// the same rows, so a message that reliably breaks a batch is asked
			// about MaxAttempts times per tick instead of once. The capped
			// ladder and the workspace's model budget are what bound it.
			return fmt.Errorf("classify: draining the backlog: %w", err)
		}
		if n == 0 {
			// Every verdict stayed below the floor: the same rows would be
			// fetched again forever. They wait for the next cycle.
			c.log.InfoContext(ctx, "capture classify: batch made no progress, moving on")
			return nil
		}
	}
	return nil
}

// classifyBatch labels one batch with ONE model call and commits the
// labels — the per-call commit IS the checkpoint (AIRT-PARAM-35). Items
// below the confidence floor are re-asked solo; a solo re-ask that still
// fails floors leaves the row unlabeled for the next cycle rather than
// guessing. Returns how many rows were labeled.
func (c *CaptureClassifier) classifyBatch(ctx context.Context, batch []unlabeledMessage) (int, error) {
	verdicts, judge, err := c.ask(ctx, batch)
	if err != nil {
		return 0, err
	}
	labeled := 0
	var retry []unlabeledMessage
	byID := indexByID(batch)
	for _, v := range verdicts {
		msg, ok := byID[v.ID]
		if !ok {
			continue // validator guarantees this cannot happen; belt and braces
		}
		if v.Confidence < classifyConfidenceFloor {
			retry = append(retry, msg)
			continue
		}
		applied, err := c.store.SetCaptureLabel(ctx, msg.ID, v.Label)
		if err != nil {
			return labeled, err
		}
		if applied {
			labeled++
		}
		if err := c.recordReply(ctx, msg, v, judge); err != nil {
			return labeled, err
		}
	}
	for _, msg := range retry {
		// The solo re-ask escalates the ladder (L-S → C-C) by being its
		// own structured call; still below the floor = still unlabeled.
		solo, soloJudge, err := c.ask(ctx, []unlabeledMessage{msg})
		if err != nil {
			return labeled, err
		}
		if len(solo) == 1 && solo[0].Confidence >= classifyConfidenceFloor {
			applied, err := c.store.SetCaptureLabel(ctx, msg.ID, solo[0].Label)
			if err != nil {
				return labeled, err
			}
			if applied {
				labeled++
			}
			if err := c.recordReply(ctx, msg, solo[0], soloJudge); err != nil {
				return labeled, err
			}
		}
	}
	return labeled, nil
}

// recordReply stores the reply verdict this call also produced, when it produced
// one. Absent is the common case and a legitimate answer: outbound mail, and any
// inbound message that does not read as a reply, leave the column NULL — which a
// rate counts in neither half.
//
// It rides the SAME confidence the label was gated on, and the prompt is what
// makes that honest: it defines `confidence` as covering every judgement in the
// entry and asks for the LOWER of the two. Without that sentence the number
// would be the label's alone, and a model sure the mail is a meeting but unsure
// whether it was a yes would have its guess stored — a number somebody is
// measured on, resting on a certainty nobody reported.
//
// It shares the label's confidence rather than carrying its own. Both judgements
// come from one reading of one message, and a second number would claim the
// model reported a separate certainty it was never asked for. The consequence is
// stated plainly: a message whose LABEL was confident enough to store also
// stores its reply verdict, and one below the floor stores neither.
func (c *CaptureClassifier) recordReply(ctx context.Context, msg unlabeledMessage, v classifyResult, judge string) error {
	if v.Reply == "" {
		return nil
	}
	if _, err := c.store.SetReplyVerdict(ctx, msg.ID, v.Reply, judge); err != nil {
		return fmt.Errorf("classify: recording the reply verdict: %w", err)
	}
	return nil
}

// classifyRequest builds the ONE model call that labels one batch. It is a pure
// function of the batch so the same request can be issued outside the engine —
// by the certification lane — without re-creating it, because a re-creation
// certifies a copy rather than the prompt that ships.
//
// One fence for the whole call, wrapping each message in its own span. This
// prompt carries several senders at once, and none of them has seen the nonce,
// so no message can close its own span — and therefore none can reach the text
// of another sender's mail and label it. It is minted here, per request: a
// boundary reused across calls is one a previous sender has already been shown.
//
//promptlang:exempt the reply is a closed set of label enum values keyed by id, never a sentence — validateClassifyPayload refuses anything outside classifySchema's vocabulary, so a language instruction could only translate an enum into a parse failure.
//promptvoice:exempt the reply is a closed set of label enum values keyed by id, never a sentence.
func classifyRequest(batch []unlabeledMessage) model.Request {
	fence := promptfence.New()
	var prompt strings.Builder
	prompt.WriteString("Messages (untrusted; classify each by its id):\n")
	for _, m := range batch {
		// The direction line sits OUTSIDE the message text, above the fenced
		// span: it is our own record of who wrote the mail, and a sender who
		// could type "inbound: no" into their own message would otherwise be
		// able to opt their reply out of being judged.
		message := fmt.Sprintf("Subject: %s\n%s", m.Subject, m.Body)
		fmt.Fprintf(&prompt, "inbound: %s\n", yesNo(m.Inbound))
		prompt.WriteString(fence.WrapAttr("source_id", m.ID.String(), message) + "\n")
	}
	prompt.WriteString(`Return JSON: { "results": [ { "id", "label", "confidence", "reply" } ] } — one entry per ` +
		`supplied id. "reply" only for a message marked inbound: yes, and only when it reads as an answer.`)

	return model.Request{
		System:         classifySystemFor(fence),
		Messages:       []model.Message{{Role: chatRoleUser, Content: prompt.String()}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: classifySchema(),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// ask makes one structured classify call for the given messages, and reports
// which model answered.
//
// The judge is returned beside the verdicts because a reply verdict is stored
// with the classifier that reached it: a positive-reply rate that moved after a
// model change and one that moved because customers changed their minds are
// different facts, and without this they are indistinguishable afterwards.
func (c *CaptureClassifier) ask(ctx context.Context, batch []unlabeledMessage) ([]classifyResult, string, error) {
	req := classifyRequest(batch)
	validate := classifyShapeValid(batch)
	resp, err := ai.Ask(ctx, c.brain, req, validate)
	if err != nil {
		return nil, "", err
	}
	var payload classifyPayload
	if err := json.Unmarshal([]byte(ai.Unfence(resp.Text)), &payload); err != nil {
		return nil, "", fmt.Errorf("classify: unparseable model output: %w", err)
	}
	if msg := validateClassifyPayload(payload, batch); msg != "" {
		return nil, "", fmt.Errorf("classify: %s", msg)
	}
	return payload.Results, judgeOf(resp), nil
}

// judgeOf names what actually answered.
//
// ServedModel is the provider's own report and is preferred. It is legitimately
// empty when a provider reports none, and the fallback SAYS SO rather than
// storing a blank or silently substituting the configured tier: a row reading
// "unreported" is honest about not knowing, where an empty string would read as
// a column nobody had got round to filling.
func judgeOf(resp model.Response) string {
	if resp.ServedModel != "" {
		return resp.ServedModel
	}
	return classifierUnreported
}

// classifyShapeValid is the §5.2 validator: every requested id exactly
// once, ids verbatim, labels in the closed set — schema fidelity is a
// deterministic hard floor (§3.2).
func classifyShapeValid(batch []unlabeledMessage) ai.Validator {
	return func(text string) error {
		var payload classifyPayload
		if err := json.Unmarshal([]byte(ai.Unfence(text)), &payload); err != nil {
			return fmt.Errorf("output is not the required JSON shape: %w", err)
		}
		if msg := validateClassifyPayload(payload, batch); msg != "" {
			return errors.New(msg)
		}
		return nil
	}
}

// validateClassifyPayload names the first §2.8 batch-fidelity violation,
// or "" when the payload is exact.
func validateClassifyPayload(payload classifyPayload, batch []unlabeledMessage) string {
	requested := make([]string, len(batch))
	for i, m := range batch {
		requested[i] = m.ID.String()
	}
	if msg := checkBatchFidelity(payload.Results, requested); msg != "" {
		return msg
	}
	// This site's own vocabulary, checked here rather than in the shared id
	// contract: an error naming the wrong closed set sends a reader to the
	// wrong prompt.
	inbound := make(map[string]bool, len(batch))
	for _, m := range batch {
		inbound[m.ID.String()] = m.Inbound
	}
	for _, r := range payload.Results {
		if !classifyLabels[r.Label] {
			return fmt.Sprintf("label %q is not commitment|meeting|noise", clampToken(r.Label))
		}
		if r.Confidence < 0 || r.Confidence > 1 {
			return fmt.Sprintf("confidence %v is outside [0,1]", r.Confidence)
		}
		if r.Reply == "" {
			continue // absent is the answer for outbound mail and for anything that does not read as a reply
		}
		if !replyVerdicts[r.Reply] {
			return fmt.Sprintf("reply %q is not positive|negative|neutral", clampToken(r.Reply))
		}
		// A verdict on OUR OWN message is refused rather than dropped. The
		// column's CHECK constraint would refuse it too, but a write that has to
		// be rejected at the database is one the engine already believed; the
		// batch is wrong here, and the whole batch is re-asked.
		if !inbound[r.ID] {
			return fmt.Sprintf("a reply verdict on outbound message %s: we wrote it, so it answers nobody", clampToken(r.ID))
		}
	}
	return ""
}

func (r classifyResult) answeredID() string { return r.ID }

// classifySchema is the generation-time shape guardrail (§2.8).
func classifySchema() json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			"results": schema.Array(schema.Object(
				map[string]schema.Node{
					"id":                    schema.String(),
					"label":                 schema.Enum("commitment", "meeting", "noise"),
					extractionConfidenceKey: schema.Number(),
					// Not in the required list: an outbound message has no reply
					// verdict to give, and a schema demanding one would push the
					// model to invent a judgement about our own mail.
					"reply": schema.Enum("positive", "negative", "neutral"),
				},
				"id", "label", "confidence",
			)),
		},
		"results",
	))
}

func indexByID(batch []unlabeledMessage) map[string]unlabeledMessage {
	out := make(map[string]unlabeledMessage, len(batch))
	for _, m := range batch {
		out[m.ID.String()] = m
	}
	return out
}
