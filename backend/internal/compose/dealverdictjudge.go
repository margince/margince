// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealverdict"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// droppedNoOwner is the outcome when the owner left between the read and the
// write: nobody is there to act for.
const droppedNoOwner = "dropped:no_owner"

// recordedAction is one action as the deal's receipt keeps it. The next
// verdict reads kind and outcome back as its prior.
type recordedAction struct {
	Kind       string   `json:"kind"`
	Outcome    string   `json:"outcome"`
	Confidence float64  `json:"confidence"`
	Reason     string   `json:"reason"`
	Cited      []string `json:"cited_activity_ids"`
}

// judge reads one deal, asks the model, applies what the rules let through
// and records the verdict. A deal with every switch off or no readable
// activity is recorded without a model call.
func (p dealVerdictPass) judge(ctx context.Context, c verdictCandidate, today time.Time, lang string) error {
	var read dealVerdictRead
	err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		var err error
		read, err = readDealVerdict(ctx, tx, c.deal, c.prior, today, p.now())
		return err
	})
	if err != nil {
		return err
	}
	read.state.SwitchedOn = c.switched
	if !anySwitchOn(c.switched) || len(read.in.Activities) == 0 {
		return p.record(ctx, c, read, nil)
	}
	about := ai.WithSubject(ctx, ids.From[ids.DealKind](c.deal).Ref(), "")
	resp, err := ai.Ask(about, p.brain, dealverdict.Request(read.in, lang), dealverdict.Validator(read.in))
	if err != nil {
		return err
	}
	payload, err := dealverdict.Parse(resp.Text, read.in)
	if err != nil {
		return fmt.Errorf("compose: the deal verdict answer: %w", err)
	}
	decisions, err := p.decide(ctx, read, payload, today)
	if err != nil {
		return err
	}
	if err := p.apply(ctx, c, read, decisions); err != nil {
		return err
	}
	return p.record(ctx, c, read, decisions)
}

func anySwitchOn(switched map[string]bool) bool {
	for _, on := range switched {
		if on {
			return true
		}
	}
	return false
}

// txRates reads conversion rates in one transaction for the rules.
type txRates struct {
	ctx   context.Context
	tx    pgx.Tx
	rates *deals.FXRates
}

func (r txRates) For(currency string) (deals.FXRate, bool, error) {
	return r.rates.For(r.ctx, r.tx, currency)
}

func (p dealVerdictPass) decide(
	ctx context.Context, read dealVerdictRead, payload dealverdict.Payload, today time.Time,
) ([]dealverdict.Decision, error) {
	var decisions []dealverdict.Decision
	err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		base, err := identity.BaseCurrencyOf(ctx, tx)
		if err != nil {
			return err
		}
		decisions, err = dealverdict.Decide(read.in, read.state, payload,
			txRates{ctx: ctx, tx: tx, rates: deals.NewFXRates(base, today)})
		return err
	})
	return decisions, err
}

// apply writes the changes the rules let through, as the owner's agent. The
// deals store asks again under the row lock whether each still holds.
func (p dealVerdictPass) apply(
	ctx context.Context, c verdictCandidate, read dealVerdictRead, decisions []dealverdict.Decision,
) error {
	var changes []deals.VerdictChange
	var at []int
	for i, decision := range decisions {
		if decision.Change != nil {
			changes = append(changes, *decision.Change)
			at = append(at, i)
		}
	}
	if len(changes) == 0 {
		return nil
	}
	outcomes, err := p.applyAsOwner(ctx, c, read, changes)
	if err != nil {
		return err
	}
	for i, outcome := range outcomes {
		decisions[at[i]].Outcome = outcome
	}
	return nil
}

func (p dealVerdictPass) applyAsOwner(
	ctx context.Context, c verdictCandidate, read dealVerdictRead, changes []deals.VerdictChange,
) ([]string, error) {
	ownerCtx, err := ownersAgent(ctx, p.users, c.owner, marginceActorID)
	if errors.Is(err, apperrors.ErrNotFound) {
		outcomes := make([]string, len(changes))
		for i := range outcomes {
			outcomes[i] = droppedNoOwner
		}
		return outcomes, nil
	}
	if err != nil {
		return nil, err
	}
	return p.deals.ApplyVerdict(ownerCtx, deals.VerdictInput{
		DealID: ids.From[ids.DealKind](c.deal), Owner: c.owner, Seen: read.state.Seen, Changes: changes,
		Allowed: func(ctx context.Context, tx pgx.Tx, correction string) (bool, error) {
			mode, err := approvals.AutoApplyModeTx(ctx, tx, c.owner, deals.VerdictSwitch(correction))
			return mode == approvals.ModeAuto, err
		},
		Record: func(ctx context.Context, tx pgx.Tx, applied []deals.VerdictChange) error {
			return p.recordOnTimeline(ctx, tx, c, read, applied)
		},
	})
}

// recordOnTimeline writes one note for the night's changes on the deal. Its
// origin keeps it out of every recency clock and out of the next verdict.
func (p dealVerdictPass) recordOnTimeline(
	ctx context.Context, tx pgx.Tx, c verdictCandidate, read dealVerdictRead, applied []deals.VerdictChange,
) error {
	subject := "Margince updated this deal"
	lines := make([]string, 0, len(applied))
	for _, change := range applied {
		lines = append(lines, verdictChangeLine(change, read.state.Seen)+": "+fmt.Sprint(change.Evidence["basis"]))
	}
	body := strings.Join(lines, "\n") + "\nUndo any of these from Home."
	_, _, err := p.notes.LogActivityTx(ctx, tx, activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindNote), Subject: &subject, Body: &body, Origin: activities.OriginSystemRemediation,
		Links: []activities.ActivityLinkInput{{EntityType: entityTypeDeal, EntityID: c.deal}},
	})
	if err != nil {
		return fmt.Errorf("compose: writing the verdict on the deal's timeline: %w", err)
	}
	return nil
}

// verdictChangeLine names one change in plain words.
func verdictChangeLine(change deals.VerdictChange, seen deals.VerdictValues) string {
	switch change.Correction {
	case deals.VerdictCloseDateCorrection:
		return "Close date set to " + change.CloseDate.Format(time.DateOnly)
	case deals.VerdictAmountCorrection:
		currency := change.Currency
		if seen.Currency != nil {
			currency = *seen.Currency
		}
		return "Amount set to " + values.MajorUnits(change.AmountMinor, currency) + " " + currency
	default:
		return fmt.Sprintf("Win probability set to %d%%", change.Probability)
	}
}

// record stores the verdict on the deal's receipt, with the checksum as the
// deal stands after the verdict's own writes.
func (p dealVerdictPass) record(
	ctx context.Context, c verdictCandidate, read dealVerdictRead, decisions []dealverdict.Decision,
) error {
	actions := make([]recordedAction, 0, len(decisions))
	for _, d := range decisions {
		actions = append(actions, recordedAction{
			Kind: d.Action.Kind, Outcome: d.Outcome, Confidence: float64(d.Action.Confidence),
			Reason: d.Action.Reason, Cited: d.Action.Cited,
		})
	}
	encoded, err := json.Marshal(actions)
	if err != nil {
		return fmt.Errorf("compose: encoding the verdict: %w", err)
	}
	return database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		verdict := assurance.DealVerdict{
			DealID: c.deal, BasisDigest: c.digest, PromptVersion: dealverdict.PromptVersion,
			AssessedAt: p.now(), Actions: encoded,
		}
		var closeDate *time.Time
		if err := tx.QueryRow(ctx, `SELECT `+dealVerdictChecksumSQL("d")+`, `+viabilityFingerprintSQL("d")+
			`, d.expected_close_date FROM deal d WHERE d.id = $1`, c.deal).
			Scan(&verdict.InputChecksum, &verdict.Fingerprint, &closeDate); err != nil {
			return fmt.Errorf("compose: reading the deal's checksum: %w", err)
		}
		if closeDate != nil && !closeDate.Before(read.in.Today) {
			next := closeDate.AddDate(0, 0, 1)
			verdict.NextDueAt = &next
		}
		return p.assurance.RecordVerdict(ctx, tx, verdict)
	})
}
