// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The counterparty verdict engine (ADR-0072/A118 §4): the resolver for what the
// tiered creation gate deferred. Capture answers the cheap deterministic
// questions itself and defers only the ambiguous first-time sender to a ledger
// row; this engine claims those rows, asks one bounded model call per SENDER,
// and turns each answer into a disposition.
//
// Three verdicts, and the asymmetry between them is the point. `real` creates
// the records capture withheld. `noise` hides the mail and schedules its
// redaction. `unsure` — including every answer below the confidence floor —
// creates nothing and destroys nothing; it stages a proposal for a human. The
// floor therefore only ever costs an extra question, never a wrong deletion:
// a prompt-injected or simply mistaken low-confidence "noise" abstains instead
// of hiding a real prospect's mail.
//
// The backlog is the ledger's due-scan, claimed under a lease with a token, so
// several replicas may drain it and a worker that dies mid-batch strands
// nothing. Every disposition commits on its own transaction — the per-row commit
// IS the checkpoint, so a budget stop or a crash keeps whatever was decided.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/schema"
)

const (
	// verdictClaimSize is how many rows one pass LEASES at a time. It is not a
	// prompt batch — each sender is judged on its own call — so this only bounds
	// how much work a single claim takes on before committing its results.
	verdictClaimSize = 8
	// verdictConfidenceFloor is the ADR-0072 §4 pin. Below it the item is
	// re-asked SOLO once; still below, it is terminally `unsure` — never
	// guessed into `noise`, which is the only verdict that hides anything.
	verdictConfidenceFloor = 0.7
	// verdictCreateFloor is what a CREATING answer needs, and it is higher —
	// see clearsItsFloor for why the two mistakes are not the same size.
	verdictCreateFloor = 0.85
	// verdictRetryBackoff spaces a row that failed for a reason it may outlive
	// (a provider fault, a malformed reply).
	verdictRetryBackoff = 30 * time.Minute
	// verdictCatchUpCap bounds one pass so a large backlog is drained over
	// several cycles rather than in one unbounded run.
	verdictCatchUpCap = 200
)

// CounterpartyVerdictEngine drains the capture disposition ledger.
type CounterpartyVerdictEngine struct {
	pool       *pgxpool.Pool
	pending    *capture.PendingStore
	contacts   *contacts.Store
	activities *activities.Store
	approvals  *approvals.Service
	brain      completer
	// triage queues the read that decides whether a domain a verdict just
	// admitted deserves a company. A `real` answer creates the CONTACT; whether
	// they have an employer is a separate question this engine does not answer.
	triage *domainTriageTrigger
	// tagFiler files a created contact under the word its connector was set to.
	tagFiler *connectorTagFiler
	// transactional carries the operator's `transactional_never` allowlist, so
	// the address gate below gives the same answer the tier ladder gives. A
	// deployment that genuinely sells to one of the listed products declares it
	// once; a verdict lane that read no allowlist would turn that declaration
	// into a suppression at the one door that creates records.
	transactional *capture.TransactionalList
	log           *slog.Logger
}

// NewCounterpartyVerdictEngine builds the engine over the pool and the verdict
// model lane. It reaches contacts through the module's own store — the ONE dedupe
// chokepoint every other creation path uses, so a verdict-created contact is
// indistinguishable from one capture created directly.
// lists carries the deployment's capture suppression config — the same value the
// sink is composed from, so the two doors that can refuse a sender read one
// allowlist rather than two.
func NewCounterpartyVerdictEngine(
	pool *pgxpool.Pool, brain completer, lists CaptureConfig, log *slog.Logger,
) *CounterpartyVerdictEngine {
	return &CounterpartyVerdictEngine{
		pool:       pool,
		pending:    capture.NewPendingStore(InstallationDB(pool)),
		contacts:   newCounterpartyStore(pool),
		activities: activities.NewStore(InstallationDB(pool)),
		approvals:  approvals.NewService(InstallationDB(pool)),
		brain:      brain,
		triage:     newDomainTriageTrigger(pool, log),
		tagFiler:   newConnectorTagFiler(pool),
		transactional: capture.NewTransactionalList(
			lists.TransactionalExtra, lists.TransactionalNever),
		log: log,
	}
}

// CanJudge reports whether a model lane was composed for this deployment. An
// installation with no AI configured still runs every other stage — what it does
// not do is fall back to creating records on sight, so deferred senders stay
// deferred rather than becoming the junk this ADR exists to prevent.
func (e *CounterpartyVerdictEngine) CanJudge() bool { return e.brain != nil }

// verdictActor names the engine in audit and provenance. The verdict pass acts
// as a system-typed principal rather than impersonating anyone — no human asked
// for this decision, and the records it creates take their OWNER from the ledger
// row (the human who granted the connection), so ownership stays honest without
// the actor pretending to be them.
//
// The `agent:` prefix is the contract's, not a description of the process type:
// crm.yaml declares captured_by as `human:<uuid> | agent:<id> | connector:<name>`
// and that value is served to clients, so a `system:` spelling would be a
// malformed field on the wire. Every sibling background writer that creates
// records stamps `agent:` for the same reason.
const verdictActor = "agent:" + verdictReason

// workspaceCtx adds the pass's provenance — the system actor its writes are
// attributed to and a fresh correlation id — to a context whose WORKSPACE the
// caller has already bound. It does not bind the workspace itself: the job
// layer does that from the args' own role declaration, and re-binding here
// would make this a second, independent source of truth for the tenant.
func (e *CounterpartyVerdictEngine) workspaceCtx(ctx context.Context) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: verdictActor,
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// verdictResult is one model answer.
type verdictResult struct {
	ID         string            `json:"id"`
	Verdict    string            `json:"verdict"`
	Confidence schema.Confidence `json:"confidence"`
}

type verdictPayload struct {
	Results []verdictResult `json:"results"`
}

// RunWorkspace drains up to maxVerdicts deferred dispositions in the workspace
// already bound in ctx. A budget stop ends the pass cleanly: what was decided
// is committed, and the rest stays claimable for the next cycle.
//
// The cap is per workspace, not per pass: a shared counter lets one large
// backlog consume the whole budget and starve every workspace after it.
func (e *CounterpartyVerdictEngine) RunWorkspace(ctx context.Context, maxVerdicts int) error {
	if maxVerdicts <= 0 {
		maxVerdicts = verdictCatchUpCap
	}
	budget := reAskBudgetFor(maxVerdicts)
	return e.inWorkspace(ctx, func(wsCtx context.Context, _ ids.UUID) error {
		resolved := 0
		for resolved < maxVerdicts {
			batch, err := e.pending.ClaimDue(wsCtx, verdictClaimSize)
			if err != nil {
				return fmt.Errorf("verdict: claiming the disposition backlog: %w", err)
			}
			if len(batch) == 0 {
				return nil
			}
			n, err := e.judgeClaimed(wsCtx, batch, budget)
			resolved += n
			if errors.Is(err, ai.ErrBudgetDeferred) {
				// Every row this pass never reached is refunded: no model saw
				// them, and with only PendingMaxAttempts to spend, charging for
				// a budget stop would let two quiet cycles exhaust an address's
				// allowance without a verdict ever being attempted on its
				// merits — an infrastructure condition turned into a per-sender
				// terminal answer nobody asked for.
				e.releaseBatch(wsCtx, batch)
				e.log.InfoContext(wsCtx, "counterparty verdict: budget exhausted, stopping the pass", "resolved", resolved)
				return nil
			}
			if err != nil {
				return fmt.Errorf("verdict: draining the disposition backlog: %w", err)
			}
		}
		return nil
	})
}

// judgeClaimed judges each claimed row on its OWN model call, and applies each
// disposition on its own transaction.
//
// ONE SENDER PER MODEL CALL. The only text in a prompt is the text of the sender
// being judged, so a hostile message has nobody else to speak for: it cannot
// dictate another sender's verdict, and a reply its content breaks is charged to
// it alone. Putting several mutually untrusted senders in one call makes both of
// those reachable, and no validator can tell a dictated answer from a judged one
// when the victim's id was legitimately in the request. The extra calls land on
// the cheapest rung of a background task, which is the right price for a
// decision that creates or destroys records.
func (e *CounterpartyVerdictEngine) judgeClaimed(
	ctx context.Context, claimed []capture.PendingCounterparty, budget *reAskBudget,
) (int, error) {
	applied := 0
	for _, row := range claimed {
		n, err := e.judgeOne(ctx, row, budget)
		if err != nil {
			// WHY it failed decides whether the row pays for it, and the cause
			// has to be read BEFORE the deferral — a Defer clears claimed_by, so
			// a refund attempted afterwards matches nothing and is lost in
			// silence.
			//
			// A budget stop never reached a model: refunded, or two quiet cycles
			// would exhaust an address's allowance and retire a genuine sender
			// to `unsure` for no reason but the workspace running out of budget.
			// Any other fault is a property of this message, which an outsider
			// writes, so it is charged — otherwise content crafted to break the
			// answer would be re-judged forever at one paid call a time.
			outOfBudget := errors.Is(err, ai.ErrBudgetDeferred)
			if deferErr := e.pending.Defer(ctx, row, verdictRetryBackoff,
				"the verdict could not be completed", outOfBudget); deferErr != nil {
				return applied, deferErr
			}
			if outOfBudget {
				return applied, err
			}
			e.log.WarnContext(ctx, "counterparty verdict: judging a sender failed",
				"disposition", row.ID.String(), "err", err)
			continue
		}
		applied += n
	}
	return applied, nil
}

// judgeOne asks about ONE sender and applies what comes back, if it clears the
// floor. An answer still below the floor retires the row to `unsure` for a human
// rather than spending another attempt on a question this model cannot answer.
func (e *CounterpartyVerdictEngine) judgeOne(
	ctx context.Context, row capture.PendingCounterparty, budget *reAskBudget,
) (int, error) {
	// The OWNER's own decision first, and no model call at all when there is
	// one. A contact who told this product what a sender is has answered the
	// question; asking anyway would spend a call to be told something we then
	// have to discard, and a machine that could overturn them would make every
	// correction temporary.
	decided, kind, err := e.ownerDecided(ctx, row)
	if err != nil {
		return 0, err
	}
	if decided {
		return e.applyOwnerDecision(ctx, row, kind)
	}
	if addressIsARoleMailbox(row.Email) {
		return e.applyJudged(ctx, row, capture.KindRoleMailbox, capture.VerdictMeasurement{})
	}
	// The other address-only refusal, and the one this lane was missing: an
	// address nobody answers at all. See addressNamesNoContact for what the gap
	// cost — a stray `contact` answer at 0.95 for an expense tool's receipts
	// address, and the contact it minted.
	//
	// Settled as a ROLE MAILBOX rather than as transactional, and the difference
	// is what the kind authorizes rather than what it reads like. `transactional`
	// takes apply's noise arm, which suppresses the sender's whole DOMAIN for
	// company creation and hides their mail — so one `noreply@` at a real
	// customer would refuse that customer a company record for every colleague,
	// on the strength of a local part. This kind creates no contact and touches
	// nothing else, which is the whole claim being made here: nobody answers at
	// this ADDRESS. What the domain is remains the model's question, and the
	// mail stays where a human can read it.
	if addressNamesNoContact(row.Email, row.Domain, e.transactional) {
		return e.applyJudged(ctx, row, capture.KindRoleMailbox, capture.VerdictMeasurement{})
	}
	// Everything above answers from the address and the ledger alone; what
	// follows needs a model. An installation without one asks a human instead —
	// see askAHumanInstead.
	if !e.CanJudge() {
		return e.askAHumanInstead(ctx, row)
	}
	answers, servedModel, err := e.ask(ctx, row)
	if err != nil {
		return 0, err
	}
	if len(answers) == 1 && clearsItsFloor(answers[0]) {
		stray, settled, err := e.strayAgainstItsOwnHistory(ctx, row, answers[0])
		if err != nil {
			return 0, err
		}
		if stray {
			return e.askAboutAStrayAnswer(ctx, row, answers, servedModel, settled)
		}
		return e.applyJudged(ctx, row, answers[0].Verdict,
			capture.MeasuredVerdict(float64(answers[0].Confidence), servedModel))
	}
	// One re-ask below the floor, then terminal (ADR-0072 §4). The retry is not
	// a hope that the same question answers differently: an unbound structured
	// call escalates the routing ladder, so the second attempt is a stronger
	// model looking at the same message.
	//
	// A pass whose re-ask budget is spent takes the answer the floor already
	// gives it: terminally unsure, a human decides. Not deferred — the row has
	// been judged, and a deferral would spend one of PendingMaxAttempts on a
	// pass that never asked its second question. Not accepted either: the whole
	// point of the floor is that an answer this unconfident does not act.
	if !budget.spend() {
		// The same helper, asked with no retry answer to prefer: a human is
		// owed what the model DID say, which here is the one answer it gave.
		if err := e.pending.Retire(ctx, row, "below the confidence floor, and this pass had no re-ask left",
			lastMeasurement(nil, "", answers, servedModel)); err != nil {
			return 0, err
		}
		return 1, nil
	}
	retry, retryModel, err := e.ask(ctx, row)
	if err != nil {
		return 0, err
	}
	if len(retry) == 1 && clearsItsFloor(retry[0]) {
		// The history guard binds here too. A first answer below the floor
		// followed by a confident creating re-ask is the SAME contradiction the
		// first-answer branch refuses, and checking only there would leave the
		// guard reachable by being unsure once — which is the cheaper path for
		// exactly the borderline sender it exists over.
		stray, settled, err := e.strayAgainstItsOwnHistory(ctx, row, retry[0])
		if err != nil {
			return 0, err
		}
		if stray {
			return e.askAboutAStrayAnswer(ctx, row, retry, retryModel, settled)
		}
		return e.applyJudged(ctx, row, retry[0].Verdict,
			capture.MeasuredVerdict(float64(retry[0].Confidence), retryModel))
	}
	// Terminally unsure: a human decides, and the ledger says so explicitly
	// rather than by having quietly run out of attempts.
	// The LAST answer travels with the retirement. A sender lands here because
	// the model had an opinion it could not hold confidently enough — "it said
	// contact at 0.78 twice" is why a human is now being asked, and dropping it
	// would hand them the question with none of the evidence.
	if err := e.pending.Retire(ctx, row, "below the confidence floor on a re-ask",
		lastMeasurement(retry, retryModel, answers, servedModel)); err != nil {
		return 0, err
	}
	return 1, nil
}

// releaseBatch returns claimed rows to the queue when the pass stops before
// reaching them — so the attempt is always refunded here: by definition no model
// saw these. The row that CAUSED the stop was already deferred by judgeClaimed,
// and its claim is spent, so this pass over it is a deliberate no-op rather than
// a second refund. Best
// effort by nature: the lease expiry is the backstop that makes this an
// optimization rather than a correctness requirement, so a release that itself
// fails is logged and the row waits out its lease.
//
// The stored reason is fixed rather than the error's text: disposition_reason is
// read back by operators and by the review queue, and a provider's raw message
// is exactly the kind of internal detail that must not travel there. The cause
// reaches the log instead, where it belongs.
func (e *CounterpartyVerdictEngine) releaseBatch(ctx context.Context, batch []capture.PendingCounterparty) {
	for _, row := range batch {
		if err := e.pending.Defer(ctx, row, verdictRetryBackoff, "the pass stopped before reaching this sender", true); err != nil {
			e.log.WarnContext(ctx, "counterparty verdict: releasing a claimed row failed",
				"disposition", row.ID.String(), "err", err)
		}
	}
}
