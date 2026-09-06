// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The signature-enrich pass (ai-operational-spec §2.9, ADR-0063): a person whose
// latest inbound mail this pass has not read yet gets ONE model read of its
// signature block — evidence-or-omit (the gateEvidence discipline: a field whose
// snippet is not verbatim in the supplied lines is dropped in code, not
// trusted), confidence floor 0.6.
//
// What lands is decided by RECENCY, not by emptiness: a signature is the contact
// stating their own details on a date, so a later one replaces what the record
// holds — a colleague's typed value included, because a number stated last week
// outranks one typed in March. The replaced value is kept for one click of undo.
//
// The single exception is a field a human CORRECTED: they read the machine's
// answer and ruled on it, and correctedAt below is how this pass defers to that
// ruling instead of re-inferring over it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

const (
	// signatureLineCount is the §2.9 input pin: the trailing non-quoted
	// lines of the person's most recent inbound mail.
	signatureLineCount = 15
	// enrichConfidenceFloor is the §2.9 acceptance floor.
	enrichConfidenceFloor = 0.6
	// enrichPassLimit bounds one pass's candidate set. A pass that fills it
	// queues a continuation rather than leaving the remainder to the nightly
	// cycle; see RunWorkspace.
	enrichPassLimit = 100
)

// enrichFieldNames is the §2.9 closed vocabulary, shared with the card reader:
// a signature and a business card state the same things about a person, and two
// vocabularies would let which one arrived decide what could be recorded.
var enrichFieldNames = map[string]bool{
	"title": true, "phone": true, "role": true, "linkedin": true, "org_name": true,
	"address": true, "website": true,
}

const signatureEnrichSystem = `You extract contact fields from ONE email signature. Allowed fields ONLY: title, phone, role,
linkedin, org_name, address, website. Emit a field ONLY if the signature lines state it verbatim; the snippet
must appear character-for-character in the supplied text. Ignore quoted replies, legal
disclaimers, and marketing taglines. Phone numbers verbatim, never normalized.
Emit address as the single line the signature prints it on. Emit website only for the
organization's own site; a social profile is never a website, and linkedin carries that one.`

// signatureEnrichSystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
func signatureEnrichSystemFor(fence promptfence.Fence) string {
	return signatureEnrichSystem + "\n" + fence.Rule("signature")
}

// CaptureEnricher drives the signature pass for every workspace.
type CaptureEnricher struct {
	pool  *pgxpool.Pool
	store *people.Store
	brain completer
	log   *slog.Logger
	// limit is how many candidates one pass takes, and therefore the count at
	// which it reports a continuation is due. enrichPassLimit in production; a
	// test sets it small so it can drive that boundary with a handful of
	// people rather than a hundred.
	limit int
}

// NewCaptureEnricher builds the pass over the pool and one model lane.
func NewCaptureEnricher(pool *pgxpool.Pool, brain completer, log *slog.Logger) *CaptureEnricher {
	return &CaptureEnricher{
		pool:  pool,
		store: people.NewStore(InstallationDB(pool)),
		brain: brain,
		log:   log,
		limit: enrichPassLimit,
	}
}

// RunWorkspace enriches up to enrichPassLimit candidates in the workspace
// already bound in ctx. A budget stop ends the pass cleanly; per-person model
// trouble is logged and the person is retried next cycle (their evidence rows
// are still absent).
//
// It reports whether the pass filled its limit AND moved at least one person,
// which is the caller's signal that more people are due right now. Two things
// make that worth acting on rather than leaving to the nightly pass. A mailbox
// sync lands hundreds of messages at once, so the 101st contact would wait
// until tonight for details a rep is about to read. And the trigger's own
// uniqueness means mail arriving mid-pass dedupes against a pass that has
// already listed its candidates — so without a continuation that message waits
// for the nightly run too, whatever the queue depth.
//
// Both halves are load-bearing; see the return statement for why progress
// alone bounds the chain.
//
// A full set is not proof that anyone is left: the last pass may have finished
// exactly at the limit. The continuation costs one job that lists no candidates
// and stops, which is the cheap side of the trade.
func (e *CaptureEnricher) RunWorkspace(ctx context.Context) (filled bool, err error) {
	// ONE PASS AT A TIME, or two of them pay for the same page.
	//
	// The candidate list is a plain read, so two passes select the same people,
	// make the same model calls and each spend for them. Three doors can put a
	// pass in flight — the nightly tick, the arrival trigger and a pass's own
	// continuation — and the first two dedupe against each other on the queue's
	// uniqueness window while the third deliberately cannot: it is enqueued from
	// inside a RUNNING job with the same args, so a window that suppressed it
	// would end the chain the continuation exists to keep.
	//
	// So the mutual exclusion is here, where the spending is. Taken and released
	// rather than waited on: a pass that finds another running has nothing to
	// add — the holder is reading the same candidates now — and its successor
	// will be queued by whoever is holding it.
	held, release, err := e.holdThePass(ctx)
	if err != nil {
		return false, err
	}
	if !held {
		e.log.InfoContext(ctx, "signature enrich: another pass holds this installation, so this one stands down")
		return false, nil
	}
	defer release()
	// The store's apply writes audit + outbox rows, so the pass binds
	// the system actor and an operation scope like every worker job.
	wsCtx := principal.WithCorrelationID(principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   "agent:enrich",
	}), ids.NewV7())
	// The workspace default, for every mailbox that never made its own choice.
	// Read once per pass rather than per candidate: it is one row, and a value
	// that changed mid-pass would sort one night's candidates by two different
	// answers to the same question.
	defaultEnrich, err := settings.Get(wsCtx, NewSettingsStore(e.pool), capture.SignatureEnrich)
	if err != nil {
		return false, fmt.Errorf("reading the signature-enrichment setting: %w", err)
	}
	candidates, err := e.store.SignatureCandidates(wsCtx, e.limit, defaultEnrich)
	if err != nil {
		return false, err
	}
	// A candidate that fails is logged, not returned, and that survives the
	// fan-out deliberately: the retry is durable in the DATA rather than in
	// River. A person whose verdict could not be parsed wrote no evidence
	// rows, so SignatureCandidates re-selects them on the next pass. Failing
	// the workspace row for one unparseable reply would retry the whole
	// candidate set to re-reach the same person.
	var advanced int
	for _, cand := range candidates {
		if err := e.enrichOne(wsCtx, cand); err != nil {
			if isBudgetStop(err) {
				e.log.InfoContext(wsCtx, "signature enrich: budget exhausted, stopping the pass")
				// No continuation: the budget is what stopped this pass, and a
				// follow-on would hit the same wall immediately. The nightly
				// cycle is the right owner of work the budget deferred.
				return false, nil
			}
			e.log.WarnContext(wsCtx, "signature enrich: candidate failed",
				"person", cand.PersonID.String(), "err", err)
			continue
		}
		advanced++
	}
	// A continuation needs BOTH a full set and somebody moved off it.
	//
	// The full set alone would chain forever. A failed candidate writes no
	// watermark on purpose — the comment above says why — so it stays the
	// newest mail and fills the next pass too. Queueing on the count alone,
	// a hundred candidates whose model call keeps failing would re-select
	// themselves, enqueue another job, and do it again: an unbroken chain of
	// jobs that each succeed, spend the model budget, and starve every older
	// person behind them. River's attempt cap cannot stop it, because no job
	// in the chain ever fails.
	//
	// Requiring progress bounds the chain by the work actually done. A pass
	// that moved nobody is the end of it, and the nightly cycle owns the
	// candidates it could not read — which is where they lived before this
	// continuation existed.
	return advanced > 0 && len(candidates) == e.limit, nil
}

func isBudgetStop(err error) bool { return errors.Is(err, ai.ErrBudgetDeferred) }

// unparseableReply reports the one drop reason that means the model failed
// rather than the signature being silent — the distinction the read cursor
// turns on.
func unparseableReply(dropped []droppedFinding) bool {
	for _, d := range dropped {
		if d.Reason == dropUnparseableReply {
			return true
		}
	}
	return false
}

// enrichOne reads one candidate's signature block, gates the model's fields
// against it, and applies the survivors by recency.
func (e *CaptureEnricher) enrichOne(ctx context.Context, cand people.SignatureCandidate) error {
	lines := signatureBlock(cand.Body)
	if lines == "" {
		// Nothing to read in this mail. The read still counts: without the
		// cursor a person whose latest mail has no signature block would be
		// selected again every night for the same empty window.
		return e.store.MarkSignatureRead(ctx, cand.PersonID, cand.ActivityID)
	}
	req := signatureEnrichRequest(cand, lines)
	resp, err := ai.Ask(ctx, e.brain, req, signatureShapeValid)
	if err != nil {
		return err
	}

	// The code-side gate: verbatim-snippet-or-drop against the exact lines
	// the model was shown, then the confidence floor.
	gated, dropped := gateEvidence(resp.Text, lines, "activity:"+cand.ActivityID.String(),
		func(name string) bool { return enrichFieldNames[name] })
	if unparseableReply(dropped) {
		// A reply no reader can parse is a fault in the ANSWER, not evidence
		// that the signature states nothing — so the read goes unrecorded and
		// this person is asked again next pass.
		return fmt.Errorf("compose: unparseable signature reply for person %s", cand.PersonID)
	}
	if len(dropped) > 0 {
		e.log.DebugContext(ctx, "signature enrich: fields dropped by the evidence gate",
			"person", cand.PersonID.String(), "dropped", len(dropped))
	}
	fields := make([]people.SignatureField, 0, len(gated))
	for _, f := range gated {
		if float64(f.Confidence) < enrichConfidenceFloor {
			continue
		}
		fields = append(fields, people.SignatureField{
			Name: f.Field, Value: f.Value, Evidence: f.EvidenceSnippet, Confidence: float64(f.Confidence),
			// The claim key the correction ledger stores for this field. The
			// apply re-reads the ledger under its own lock and defers to any
			// ruling it finds; computing the key here is what lets it, because
			// the key is a hash only this module can build.
			ClaimKey: ai.ClaimKey(ai.ProfileFieldClaimPath(f.Field)),
		})
	}
	if len(fields) == 0 {
		// The model answered and nothing survived the gate — an answer, and
		// the same answer next time unless this person writes again.
		return e.store.MarkSignatureRead(ctx, cand.PersonID, cand.ActivityID)
	}
	if _, err := e.store.ApplySignatureFields(ctx, cand.PersonID, cand.ActivityID, fields); err != nil {
		return err
	}
	// A model error above returns before this point on purpose: an
	// unanswered call is a call still owed, so the person stays a candidate.
	return e.store.MarkSignatureRead(ctx, cand.PersonID, cand.ActivityID)
}

// signatureEnrichRequest builds the ONE model call that reads one candidate's
// signature. It is a pure function of the candidate and the window their mail
// yielded so the same request can be issued outside the pass — by the
// certification lane — without re-creating it, because a re-creation certifies a
// copy rather than the prompt that ships.
//
// The lines arrive already derived by signatureBlock rather than being derived
// here: the evidence gate matches every quote against the SAME window the model
// was shown, so one derivation feeds both readers and neither can drift.
//
// The fence is minted here, per request: a boundary reused across calls is one a
// previous sender has already been shown, and every field of this prompt is
// their own writing.
//
//promptlang:exempt the fields are a title and a phone number copied out of the person's own signature block, each carrying an evidence_snippet checked against those lines — a job title is written the way its holder writes it, and translating one would both change the fact and fail the snippet check.
//promptvoice:exempt the fields are a title and a phone number copied out of a signature block, each checked against those lines; there is no sentence of ours here to have a voice.
func signatureEnrichRequest(cand people.SignatureCandidate, lines string) model.Request {
	fence := promptfence.New()
	var prompt strings.Builder
	// The person's own name and address are theirs to write, so they go INSIDE
	// the boundary like the signature does. Interpolated into a header line they
	// would be reading in the prompt's own voice, which is the whole attack.
	prompt.WriteString("Person (untrusted):\n")
	prompt.WriteString(fence.Wrap(fmt.Sprintf("Name: %s\nEmail: %s", cand.FullName, cand.Email)) + "\n")
	// Everything the vocabulary admits, and no statement about what the record
	// already holds. The pass reads a signature to find out whether what it
	// holds is still true, so naming the empty fields would ask the narrower
	// question and miss the number that changed.
	prompt.WriteString("Fields to extract when stated: [\"title\",\"phone\",\"role\",\"linkedin\",\"org_name\",\"address\",\"website\"]\n")
	prompt.WriteString("Signature block (untrusted; the trailing lines of their last email):\n")
	prompt.WriteString(fence.WrapAttr("source_id", cand.ActivityID.String(), lines) + "\n")
	prompt.WriteString(`Return JSON: { "fields": [ { "field", "value", "evidence_snippet", "confidence" } ] }`)

	return model.Request{
		System:         signatureEnrichSystemFor(fence),
		Messages:       []model.Message{{Role: chatRoleUser, Content: prompt.String()}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: signatureEnrichSchema(),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// signatureBlock returns the trailing signatureLineCount non-quoted,
// non-empty-tail lines of a stored email body — the §2.9 source window.
// Quoted history (">"-prefixed) is not identity evidence and is excluded.
func signatureBlock(body string) string {
	lines := strings.Split(body, "\n")
	var kept []string
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), ">") {
			continue
		}
		kept = append(kept, l)
	}
	// Trim trailing blank lines so the window holds content, not padding.
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	if len(kept) > signatureLineCount {
		kept = kept[len(kept)-signatureLineCount:]
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// signatureShapeValid is the §5.2 retry validator: parseable fields with
// the closed vocabulary — evidence checking stays code-side in
// gateEvidence (the model must not be trusted to self-certify).
func signatureShapeValid(text string) error {
	var parsed struct {
		Fields []extractedField `json:"fields"`
	}
	if err := json.Unmarshal([]byte(ai.Unfence(text)), &parsed); err != nil {
		return fmt.Errorf("output is not the required JSON shape: %w", err)
	}
	for _, f := range parsed.Fields {
		// The field name is MODEL output, so it is bounded before it reaches an
		// error that is logged and, on a retry, fed back into the prompt.
		if !enrichFieldNames[f.Field] {
			return fmt.Errorf("field %q is not in the allowed set", clampToken(f.Field))
		}
	}
	return nil
}

// signatureEnrichSchema is the generation-time shape guardrail (§2.9).
func signatureEnrichSchema() json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			laneFields: schema.Array(schema.Object(
				map[string]schema.Node{
					extractionFieldKey: schema.Enum("title", "phone", "role", "linkedin", "org_name", "address", "website"),
					"value":            schema.String(),
					"evidence_snippet": schema.String(),
					"confidence":       schema.Number(),
				},
				"field", "value", "evidence_snippet", "confidence",
			)),
		},
		"fields",
	))
}

// enrichPassLock names the lock the pass holds, as text rather than as a
// number: an advisory key is an integer namespace shared by the whole database,
// and a bare constant is how two unrelated passes come to hold one key.
const enrichPassLock = "capture_signature_enrich_pass"

// holdThePass takes the pass's advisory lock, and answers a release for it.
//
// SESSION-scoped, on a connection held for the pass, because the pass is many
// transactions — the candidate read, one apply per person, the watermark — and
// a transaction lock would be gone before the first model call. That also makes
// the release crash-safe in the way a lease row is not: a worker that dies
// drops its connection, and Postgres drops the lock with it, where a `held_until`
// column would keep the next pass out until the clock caught up.
//
// TRY, not wait. A pass that queued behind another would wake to the candidate
// set the holder had just emptied and spend a slot proving it, and on a
// contended installation the queue itself is what would grow.
func (e *CaptureEnricher) holdThePass(ctx context.Context) (bool, func(), error) {
	conn, err := e.pool.Acquire(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("taking a connection for the enrich pass lock: %w", err)
	}
	var held bool
	if err := conn.QueryRow(ctx,
		`SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, enrichPassLock).Scan(&held); err != nil {
		conn.Release()
		return false, nil, fmt.Errorf("taking the enrich pass lock: %w", err)
	}
	if !held {
		conn.Release()
		return false, nil, nil
	}
	return true, func() {
		// Released on a context of its own: the pass's may already be cancelled
		// — a worker timeout is exactly when the lock most needs dropping — and
		// an unlock that never ran would hold this installation's passes out
		// until the connection was recycled.
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), enrichUnlockTimeout)
		defer cancel()
		if _, err := conn.Exec(unlockCtx,
			`SELECT pg_advisory_unlock(hashtextextended($1, 0))`, enrichPassLock); err != nil {
			e.log.WarnContext(ctx, "signature enrich: releasing the pass lock", "err", err)
		}
		conn.Release()
	}, nil
}

// enrichUnlockTimeout bounds the release, which runs after the pass is over and
// must not become the reason a worker hangs.
const enrichUnlockTimeout = 5 * time.Second
