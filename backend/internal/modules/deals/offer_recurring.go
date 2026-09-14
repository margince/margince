// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a recurring offer line is worth per year, and what the buyer actually
// committed to.
//
// Its own file beside offer_totals.go because these are DIFFERENT figures over
// the same lines, and conflating them is the mistake this exists to prevent.
// The offer's gross is what the buyer owes across the whole commitment; the ARR
// is what one year of the recurring part is worth. A quarterly line at 3,000
// committed for four quarters is 12,000 of committed value AND 12,000 a year;
// the same line committed for eight quarters is 24,000 committed and still
// 12,000 a year. One number cannot be both.
//
// Two rules the whole file turns on:
//
// ARR is NET and annualized per period. Tax is a government's share, not
// revenue, so annualizing the gross would report a figure the business never
// receives. The annualization multiplies the per-period net by the periods in
// a year (12 / interval_months), which is exact for every cadence the schema
// admits — 1, 3, 6 and 12 all divide 12.
//
// An UNCLASSIFIED line contributes nothing to either figure. It is not treated
// as one-off and not guessed at as recurring: nobody said, and a total built on
// a guess is worse than a total that says it does not know. Every line written
// before this classification existed is unclassified, so this is the ordinary
// case for historical offers rather than an edge.

import (
	"context"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Billing classifications. The empty string is the third state — nobody has
// said — and it is spelled as an absence rather than as a member, because a
// vocabulary that admits "unknown" as a value invites code to treat it as an
// answer.
const (
	BillingOneTime   = "one_time"
	BillingRecurring = "recurring"
)

// monthsPerYear is what annualizing a per-period figure multiplies through.
const monthsPerYear = 12

// intervalCountField names the wire field every refusal about a committed term
// points at.
const intervalCountField = "interval_count"

// RecurringLineInput is one line's classification beside the money inputs the
// totals engine already consumes. The classification travels separately
// because OfferLineInput is the money vocabulary and this is not money: a line
// carries the same price whether or not anybody has said how often it repeats.
type RecurringLineInput struct {
	Line OfferLineInput
	// BillingModel is "", BillingOneTime or BillingRecurring.
	BillingModel string
	// IntervalMonths is how many months one period spans. Meaningful only for
	// a recurring line, where the schema guarantees it is 1, 3, 6 or 12.
	IntervalMonths *int
	// IntervalCount is how many periods the buyer committed to. Null on a
	// recurring line means the term is still being settled.
	IntervalCount *int
}

// RecurringFigures are the two derived offer-level figures, in minor units.
type RecurringFigures struct {
	// ArrMinor is one year of the recurring lines' net value. Zero where no
	// line is classified recurring.
	ArrMinor int64
	// NetTcvMinor is the net total the buyer commits to: one-off lines in
	// full, plus each recurring line's per-period net times its committed
	// periods. A recurring line with no settled term contributes nothing,
	// because there is no committed total until somebody says how long.
	NetTcvMinor int64
}

// OfferRecurringTotals derives both figures over an offer's accepted lines.
//
// It runs each line through the SAME LineTotals the gross uses rather than
// re-deriving the net here. Two spellings of "what this line is worth net"
// would eventually disagree about a rounding, and the offer would then report
// an ARR that its own line list does not add up to.
func OfferRecurringTotals(lines []RecurringLineInput) (RecurringFigures, error) {
	arr, tcv := new(big.Int), new(big.Int)
	for i, line := range lines {
		figures, err := LineTotals(line.Line)
		if err != nil {
			return RecurringFigures{}, fmt.Errorf("line %d: %w", i+1, err)
		}
		net := big.NewInt(figures.NetMinor)

		switch line.BillingModel {
		case BillingOneTime:
			// Owed once, in full, and worth nothing per year.
			tcv.Add(tcv, net)
		case BillingRecurring:
			if line.IntervalMonths == nil || *line.IntervalMonths <= 0 {
				// The schema's shape CHECK refuses this row, so reaching it
				// means a writer bypassed the constraint. Refusing beats
				// dividing by zero or quietly annualizing at some default.
				return RecurringFigures{}, fmt.Errorf(
					"line %d is recurring with no billing interval", i+1)
			}
			// 12 / interval_months is exact for every admitted cadence, so
			// the annual figure needs no rounding of its own — it is a whole
			// number of periods times an already-rounded per-period net.
			periodsPerYear := big.NewInt(int64(monthsPerYear / *line.IntervalMonths))
			arr.Add(arr, new(big.Int).Mul(net, periodsPerYear))
			if line.IntervalCount != nil {
				tcv.Add(tcv, new(big.Int).Mul(net, big.NewInt(int64(*line.IntervalCount))))
			}
		default:
			// Unclassified: counted toward neither. Nobody said what this
			// price does, and both figures are claims that require an answer.
		}
	}

	var out RecurringFigures
	var err error
	if out.ArrMinor, err = minorFromBig(arr, "arr_minor", "line_items"); err != nil {
		return RecurringFigures{}, err
	}
	if out.NetTcvMinor, err = minorFromBig(tcv, "net_tcv_minor", "line_items"); err != nil {
		return RecurringFigures{}, err
	}
	return out, nil
}

// validBillingShape refuses a half-stated classification before it reaches the
// constraint, so the caller gets a field to fix rather than a schema name.
//
// It states the same three shapes product_billing_shape and oli_billing_shape
// hold in SQL. Stated twice on purpose: the constraint is what holds for a
// tool, a restore or a future writer, and this is what makes the refusal
// legible to the human in front of the form.
func validBillingShape(model *string, intervalMonths *int) error {
	switch {
	case model == nil:
		// Unclassified. A cadence without a model is half an answer: it says
		// how often something repeats while refusing to say that it does.
		if intervalMonths != nil {
			return &BillingShapeError{
				Field:  "billing_model",
				Reason: "a billing interval says how often the price repeats, so it needs a billing model that says the price repeats at all",
			}
		}
	case *model == BillingOneTime:
		if intervalMonths != nil {
			return &BillingShapeError{
				Field:  "billing_interval_months",
				Reason: "a one-off price has no billing interval, because there is nothing to repeat",
			}
		}
	case *model == BillingRecurring:
		if intervalMonths == nil {
			return &BillingShapeError{
				Field:  "billing_interval_months",
				Reason: "a recurring price needs the number of months one billing period spans",
			}
		}
	default:
		return &BillingShapeError{
			Field:  "billing_model",
			Reason: "a price is one_time or recurring, or carries no classification at all",
		}
	}
	return nil
}

// BillingShapeError maps to 422: a classification that states half of itself.
type BillingShapeError struct{ Field, Reason string }

func (e *BillingShapeError) Error() string { return e.Reason }

// FieldFault names the half the caller has to supply or remove.
func (e *BillingShapeError) FieldFault() (field, code, message string) {
	return e.Field, "billing_shape", e.Reason
}

// validIntervalCount refuses a committed term on a line that does not repeat.
//
// Separate from validBillingShape because the count is an offer-line idea and
// the shape rule is shared with products, which carry no commitment: a product
// is a price, and how long somebody buys it for is settled on the paper.
func validIntervalCount(model *string, count *int) error {
	if count == nil {
		return nil
	}
	if model == nil || *model != BillingRecurring {
		return &BillingShapeError{
			Field:  intervalCountField,
			Reason: "only a recurring line commits to a number of periods, because a one-off price is paid once",
		}
	}
	if *count < 1 {
		return &BillingShapeError{
			Field:  intervalCountField,
			Reason: "a commitment covers at least one period",
		}
	}
	return nil
}

// validPatchedIntervalCount refuses a committed term the patched line cannot
// carry, judged on the line as the patch LEAVES it rather than on the request.
//
// A patch can name the count without the classification, or the classification
// without the count, so neither half on its own says what the resulting line is.
// Reading the row is what makes "you set a term on a one-off line" and "you made
// this line one-off while it still carries a term" the same refusal.
func validPatchedIntervalCount(ctx context.Context, tx pgx.Tx, lineID ids.UUID, in UpdateOfferLineInput) error {
	model := in.BillingModel
	count := in.IntervalCount
	if !in.Classified || !in.IntervalCountSet {
		var storedModel *string
		var storedCount *int
		if err := tx.QueryRow(ctx,
			`SELECT billing_model, interval_count FROM offer_line_item WHERE id = $1`,
			lineID).Scan(&storedModel, &storedCount); err != nil {
			return fmt.Errorf("read line for interval check: %w", err)
		}
		if !in.Classified {
			model = storedModel
		}
		if !in.IntervalCountSet {
			count = storedCount
		}
	}
	return validIntervalCount(model, count)
}
