// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Filing a captured mail under the one deal it is about. Capture and the deal
// repair both run AttributeDealTx. The rule, in order, first answer wins:
//   - a mail that already has a deal link is left alone, so a human relink
//     always wins;
//   - a mail filed under a project goes on that project's one open deal. None
//     or several means no link, and the company is not asked;
//   - else a deal a sibling in the thread is filed under, when it agrees with
//     the mail's company;
//   - else the one open deal of the one company the mail's participants work
//     at. With several open deals, a participant who is a stakeholder on
//     only one of them picks it.
//
// Being at a company is not proof of which of its deals a mail is about. When
// several deals stay plausible the mail goes on none of them and the Ask seam
// may raise one "Which deal is this about?" question.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// DealCandidates reads the open deals the rule chooses among. `deal` belongs
// to the deals module, so compose injects it. Each read answers at most two
// ids, because the rule only asks "only one?". It takes the caller's
// transaction for the reason ProjectKeyMatcher does.
type DealCandidates interface {
	OpenDealsOfProject(ctx context.Context, tx pgx.Tx, projectID ids.UUID) ([]ids.UUID, error)
	OpenDealsOfCompany(ctx context.Context, tx pgx.Tx, companyID ids.UUID) ([]ids.UUID, error)
	// OpenDealsWithStakeholders is the company's open deals on which one of
	// contacts is a stakeholder.
	OpenDealsWithStakeholders(ctx context.Context, tx pgx.Tx, companyID ids.UUID, contacts []ids.UUID) ([]ids.UUID, error)
}

// WhichDeal is a mail the rule could not place on one deal of the company or
// project it belongs to.
type WhichDeal struct {
	ActivityID ids.ActivityID
	// CompanyID is the one company the mail belongs to; zero when the question
	// came from a project with several open deals.
	CompanyID ids.UUID
	ProjectID ids.UUID
}

// AskWhichDeal raises the one disambiguation for a mail, in the rule's
// transaction. It decides who may be asked and asks nobody when no one is
// evidenced.
type AskWhichDeal func(ctx context.Context, tx pgx.Tx, q WhichDeal) error

// DealAttribution is the rule's seams. A nil Candidates or Filer turns the
// rule off; a nil Ask places mail and asks nobody.
type DealAttribution struct {
	Candidates DealCandidates
	Ask        AskWhichDeal
	Filer      DealFiler
}

// WithDealAttribution returns a copy that files captured mail under its deal.
func (s *Sink) WithDealAttribution(attribution DealAttribution) *Sink {
	c := *s
	c.deals = attribution
	return &c
}

// DealOutcome names what the rule concluded for one mail.
type DealOutcome string

// The outcomes, from "nothing to decide" to "linked".
const (
	DealNotMail          DealOutcome = "not_mail"
	DealAlreadyFiled     DealOutcome = "already_filed"
	DealLinked           DealOutcome = "linked"
	DealNotWritable      DealOutcome = "not_writable"
	DealNone             DealOutcome = "none"
	DealAmbiguous        DealOutcome = "ambiguous"
	DealConflictingFirms DealOutcome = "conflicting_companies"
)

// DealVerdict is the rule's answer. Deal is set only for DealLinked, and
// Question only for DealAmbiguous.
type DealVerdict struct {
	Outcome  DealOutcome
	Deal     ids.UUID
	Question WhichDeal
}

// AttributeDealTx applies the rule to one activity and writes the link it
// concludes. With ask nil it raises no question, which is what a repair of old
// mail wants.
func AttributeDealTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, rule DealAttribution) (DealVerdict, error) {
	return fileOnDeal(dealFiling{decider: ctx, writer: ctx, filer: rule.Filer}, tx, activityID, rule.Candidates, rule.Ask)
}

// dealFiling is who decides and who writes. Live capture is both. The repair
// decides with the capturing mailbox's scope and writes as its own actor. The
// reason goes in evidence, so the audit row names who really wrote.
type dealFiling struct {
	decider, writer context.Context
	evidence        map[string]any
	filer           DealFiler
}

// fileOnDeal locks the activity row first, as the human relink locks it
// (activities.relinkActivityRow). Deal links have no one-per-activity index.
// Without the lock, a relink between the guard and the insert leaves two.
func fileOnDeal(f dealFiling, tx pgx.Tx, activityID ids.ActivityID, candidates DealCandidates, ask AskWhichDeal) (DealVerdict, error) {
	if live, err := lockForDeal(f.writer, tx, activityID); err != nil || !live {
		return DealVerdict{Outcome: DealNone}, err
	}
	facts, err := readDealFacts(f.decider, tx, activityID, false)
	if err != nil || facts.outcome != "" {
		return DealVerdict{Outcome: facts.outcome}, err
	}
	verdict, err := decideDeal(f.decider, tx, facts, candidates)
	if err != nil {
		return DealVerdict{}, err
	}
	switch verdict.Outcome {
	case DealLinked:
		written, err := linkActivityToDeal(f, tx, activityID, verdict.Deal)
		if err != nil || written != DealLinked {
			return DealVerdict{Outcome: written}, err
		}
	case DealAmbiguous:
		verdict.Question.ActivityID = activityID
		if ask != nil {
			if err := ask(f.decider, tx, verdict.Question); err != nil {
				return DealVerdict{}, err
			}
		}
	}
	return verdict, nil
}

func lockForDeal(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) (bool, error) {
	if _, err := storekit.LockRow(ctx, tx, "activity", activityID.UUID, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("capture: locking the activity to file under a deal: %w", err)
	}
	return true, nil
}

// attributeDeal runs the rule after the capture committed, on attributeProject's
// terms: its own transaction, on every capture so the next one retries a
// fault. A fault is logged and never returned.
func (s *Sink) attributeDeal(ctx context.Context, rec connector.NormalizedRecord, ref datasource.EntityRef) {
	if s.deals.Candidates == nil || s.deals.Filer == nil || ref.Type != datasource.EntityActivity {
		return
	}
	if fields, ok := rec.Fields.(ActivityFields); !ok || fields.Kind != kindEmail {
		return
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := AttributeDealTx(ctx, tx, ids.From[ids.ActivityKind](ref.ID), s.deals)
		return err
	})
	if err != nil {
		s.logDealAttributionFault(ctx, rec, err)
	}
}

// logDealAttributionFault records a failed deal step in system_log, on its own
// transaction because the step's own one rolled back.
func (s *Sink) logDealAttributionFault(ctx context.Context, rec connector.NormalizedRecord, cause error) {
	detail := map[string]any{
		fieldReason:       "deal_attribution_failed",
		fieldSourceSystem: rec.NaturalKey.SourceSystem,
		fieldSourceID:     rec.NaturalKey.SourceID,
		fieldError:        cause.Error(),
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, logErr := storekit.LogSystem(ctx, tx, "capture_deal_attribution_fault", detail)
		return logErr
	})
	if err != nil {
		slog.ErrorContext(ctx, "capture: recording the deal-attribution fault", "err", err, "cause", cause)
	}
}
