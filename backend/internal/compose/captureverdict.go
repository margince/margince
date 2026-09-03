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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
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
	people     *people.Store
	activities *activities.Store
	approvals  *approvals.Service
	brain      completer
	// triage queues the read that decides whether a domain a verdict just
	// admitted deserves a company. A `real` answer creates the PERSON; whether
	// they have an employer is a separate question this engine does not answer.
	triage *domainTriageTrigger
	log    *slog.Logger
}

// NewCounterpartyVerdictEngine builds the engine over the pool and the verdict
// model lane. It reaches people through the module's own store — the ONE dedupe
// chokepoint every other creation path uses, so a verdict-created person is
// indistinguishable from one capture created directly.
func NewCounterpartyVerdictEngine(pool *pgxpool.Pool, brain completer, log *slog.Logger) *CounterpartyVerdictEngine {
	return &CounterpartyVerdictEngine{
		pool:       pool,
		pending:    capture.NewPendingStore(InstallationDB(pool)),
		people:     newCounterpartyStore(pool),
		activities: activities.NewStore(InstallationDB(pool)),
		approvals:  approvals.NewService(InstallationDB(pool)),
		brain:      brain,
		triage:     newDomainTriageTrigger(pool, log),
		log:        log,
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
			n, err := e.judgeClaimed(wsCtx, batch)
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
func (e *CounterpartyVerdictEngine) judgeClaimed(ctx context.Context, claimed []capture.PendingCounterparty) (int, error) {
	applied := 0
	for _, row := range claimed {
		n, err := e.judgeOne(ctx, row)
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
func (e *CounterpartyVerdictEngine) judgeOne(ctx context.Context, row capture.PendingCounterparty) (int, error) {
	// The OWNER's own decision first, and no model call at all when there is
	// one. A person who told this product what a sender is has answered the
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
		return e.applyJudged(ctx, row, answers[0].Verdict,
			capture.MeasuredVerdict(float64(answers[0].Confidence), servedModel))
	}
	// One re-ask below the floor, then terminal (ADR-0072 §4). The retry is not
	// a hope that the same question answers differently: an unbound structured
	// call escalates the routing ladder, so the second attempt is a stronger
	// model looking at the same message.
	retry, retryModel, err := e.ask(ctx, row)
	if err != nil {
		return 0, err
	}
	if len(retry) == 1 && clearsItsFloor(retry[0]) {
		return e.applyJudged(ctx, row, retry[0].Verdict,
			capture.MeasuredVerdict(float64(retry[0].Confidence), retryModel))
	}
	// Terminally unsure: a human decides, and the ledger says so explicitly
	// rather than by having quietly run out of attempts.
	// The LAST answer travels with the retirement. A sender lands here because
	// the model had an opinion it could not hold confidently enough — "it said
	// person at 0.78 twice" is why a human is now being asked, and dropping it
	// would hand them the question with none of the evidence.
	if err := e.pending.Retire(ctx, row, "below the confidence floor on a re-ask",
		lastMeasurement(retry, retryModel, answers, servedModel)); err != nil {
		return 0, err
	}
	return 1, nil
}

// apply commits one verdict. The ledger resolution and whatever the verdict
// causes share a transaction, so a row can never read `real` without the records
// it promised, nor `noise` without the hiding it authorized.
//
// Resolve's compare-and-set decides who acts: only the caller that actually
// closed the row runs the effect, which makes a replayed job or a raced sibling
// a no-op rather than a second creation.
func (e *CounterpartyVerdictEngine) apply(
	ctx context.Context, row capture.PendingCounterparty, kind string, ownerSaidSo bool,
	measured capture.VerdictMeasurement,
) (bool, error) {
	verdict, known := statusForKind(kind)
	if !known {
		return false, fmt.Errorf("verdict: %q is not a sender kind", kind)
	}
	var acted bool
	var triageDomain string
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		won, err := e.pending.ResolveAs(ctx, tx, row, verdict, kind, verdictReason, ownerSaidSo, measured)
		if err != nil || !won {
			return err
		}
		acted = true
		// Exhaustive over verdictKinds, held by TestEveryVerdictKindHasAnEffect
		// rather than by this comment: two kinds once reached the prompt with no
		// arm here, and the claim of exhaustiveness is what stopped anybody
		// checking. A `default` that fell through to hideNoise is how a new kind
		// would silently start hiding real mail, so there is none.
		switch kind {
		case capture.KindPerson:
			triageDomain, err = e.createCounterparty(ctx, tx, row)
			return err
		case capture.KindRoleMailbox, capture.KindOrganizationSender:
			// Real correspondence with no human to name. The message stays
			// visible; no contact is invented for a mailbox nobody owns.
			return nil
		case capture.KindNewsletter, capture.KindTransactional, capture.KindSpam:
			// A seat's own `keep out` does NOT suppress the domain. The two
			// statements are different sizes: the classifier calling a sender
			// noise is a judgement about the sender, and suppressing their
			// domain workspace-wide follows from it; a person saying "keep this
			// out of my mail" is a statement about their own mailbox, and one
			// rep who once received mail from a partner could otherwise refuse
			// that company to every colleague — with a per-record person grant
			// and no capture-settings grant at all.
			//
			// The mail hide still runs: it is what "keep out" means, and the
			// noise scope already excludes anything a colleague corresponded
			// with.
			if !ownerSaidSo {
				if err := e.suppressSenderDomain(ctx, tx, row, kind); err != nil {
					return err
				}
			}
			return e.hideNoise(ctx, tx, row)
		case capture.KindAdvisor:
			// A genuine contact who is the OWNER's. The record is made — a
			// founder's lawyer is somebody they correspond with — and stays
			// owner-scoped, because publishing it to the workspace announces
			// that the founder has a lawyer and what about.
			triageDomain, err = e.createOwnerScopedCounterparty(ctx, tx, row)
			return err
		case capture.KindPersonal:
			// No record at all: a family member is not a counterparty of the
			// business. The mail itself is not destroyed here — the purge that
			// does that is its own change, with an undo window in front of it —
			// so this withholds the record and leaves the thread to the
			// mailbox owner.
			//
			// hideNoise is deliberately NOT called. Its scope excludes every
			// address the workspace has written to, which is every address this
			// kind is ever about, so it would be a no-op that read like a hide.
			return nil
		}
		return fmt.Errorf("verdict: no effect defined for sender kind %q", kind)
	})
	if err != nil {
		// The address is the only identifying detail here and it is already in
		// this workspace's own timeline; the model's answer is not, so the
		// verdict names what was being attempted without echoing content.
		return false, fmt.Errorf("verdict: applying %s to %s: %w", verdict, row.Email, err)
	}
	// Post-commit, like the capture path's own trigger and for the same reason:
	// the records are already durable, and queueing the read that decides their
	// company must not be able to roll them back. A miss is the sweep's.
	if triageDomain != "" && e.triage != nil {
		e.triage.domainPending(ctx, triageDomain)
	}
	return acted, nil
}

// verdictReason is what the ledger records as the authority for a machine
// disposition, distinguishing it from a T2 registry rule or a human decision.
const verdictReason = "capture_counterparty_verdict"

// hideNoise is the `noise` effect's first stage: the mail stops being visible
// immediately, and its content is redacted later by the sweep (ADR-0072 §4's
// hide-then-redact). The delay is the undo window — the whole reason a verdict
// is allowed to hide anything is that a wrong one can still be taken back.
func (e *CounterpartyVerdictEngine) hideNoise(ctx context.Context, tx pgx.Tx, row capture.PendingCounterparty) error {
	// The scope rule lives with the ledger (noiseMailScope): a verdict may only
	// reach inbound, unattested, unlinked mail from an address the workspace has
	// never written to. Resolved on the SAME transaction, so what is hidden is
	// what was true when the verdict committed.
	due, err := e.pending.NoiseMailForTx(ctx, tx, row.Email, noiseSweepBatch)
	if err != nil {
		return err
	}
	_, err = e.activities.HideCapturedNoiseTx(ctx, tx, due)
	return err
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

// suppressSenderDomain refuses the sender's domain a company.
//
// Hiding the mail is not enough on its own. A newsletter publisher or an
// expense tool has a real corporate website, so if a NAMED employee ever writes
// from that domain the triage reads their site, finds a genuine company, and
// creates it — the vendor arrives in the CRM by another door. The refusal has
// to be a standing decision about the domain, which is why it is recorded here
// rather than inferred from the sender each time.
//
// It never overwrites a human's admission (the store's guard), so an admin who
// let a domain in keeps it in no matter how much bulk mail follows.
//
// A free-mail domain is skipped: nobody's employer is gmail.com, so there is no
// company to refuse, and suppressing it would put a consumer mail provider in
// the admin's blocked list as though it were a decision anyone made.
// A sender the workspace CORRESPONDS with is never refused a company, whatever
// the classifier called this particular message. The two facts do not conflict:
// a supplier's marketing blast is a newsletter and the supplier is still a
// company this business works with. Hiding that one message is right; refusing
// the domain on the strength of it is not, and the refusal is the standing,
// workspace-wide half.
//
// This guard became load-bearing when the create tiers started raising verdict
// questions of their own — before that only unjudged strangers reached the
// ledger, and correspondence was already excluded by construction.
func (e *CounterpartyVerdictEngine) suppressSenderDomain(ctx context.Context, tx pgx.Tx, row capture.PendingCounterparty, kind string) error {
	if row.Domain == "" {
		return nil
	}
	corresponds, err := e.pending.CorrespondsWith(ctx, tx, row.Email)
	if err != nil {
		return err
	}
	if corresponds {
		return nil
	}
	return e.people.SuppressBulkSenderDomainTx(ctx, tx, row.Domain,
		"mail from this domain was judged "+kind+", so it is not a company this business works with")
}

// ownerDecided answers whether the mailbox owner already settled this sender,
// and which kind their decision amounts to.
//
// `business` becomes `person`: the owner is saying this is somebody the CRM
// should hold, which is the one kind that creates a record. `keep_out` becomes
// `spam`, the noise kind whose effects — hide the mail, suppress the domain —
// are what "keep this out for good" means. Neither invents a new kind: the
// ledger's vocabulary is closed: a decision spelled outside it would sit in a
// column every downstream reader parses against a fixed set, and be skipped.
func (e *CounterpartyVerdictEngine) ownerDecided(ctx context.Context, row capture.PendingCounterparty) (bool, string, error) {
	var decision string
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		decision, err = capture.OverrideForTx(ctx, tx, row.OwnerID, row.Email)
		return err
	}); err != nil {
		return false, "", err
	}
	switch decision {
	case capture.OverrideBusiness:
		return true, capture.KindPerson, nil
	case capture.OverrideKeepOut:
		return true, capture.KindSpam, nil
	}
	return false, "", nil
}
