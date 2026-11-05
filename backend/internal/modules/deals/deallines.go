// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A deal's own product lines, and the value they set on it.
//
// A deal in `lines` mode takes its amount, recurring value and product split
// from these rows and from nothing else: a typed amount, a hand-set product
// list and an accepted offer are each refused or ignored for it. Every write
// here advances the deal's commercial_revision under the deal's row lock, so
// two sellers editing from one revision cannot both win.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maxDealLines bounds one deal's lines, as the hand-set product list is.
const maxDealLines = 200

// AddDealLineInput is one line to add. Decimals are exact strings.
type AddDealLineInput struct {
	ExpectedRevision int64
	ReplaceEstimate  bool
	Currency         *string
	ProductID        *ids.ProductID
	Description      *string
	Unit             *string
	Quantity         *string
	UnitPriceMinor   *int64
	DiscountPct      *string
	TaxRate          *string
	TaxCategory      *string
	BillingModel     *string
	IntervalMonths   *int
	IntervalCount    *int
}

// UpdateDealLineInput is a sparse line edit. A ...Set flag says the caller
// named the field, which a nil cannot: null is how a value becomes unknown.
type UpdateDealLineInput struct {
	ExpectedRevision int64
	Description      *string
	Unit             *string
	QuantitySet      bool
	Quantity         *string
	PriceSet         bool
	UnitPriceMinor   *int64
	DiscountPct      *string
	TaxRate          *string
	TaxCategory      *string
	Classified       bool
	BillingModel     *string
	IntervalMonths   *int
	IntervalCountSet bool
	IntervalCount    *int
}

// LineOwnedValueError refuses a typed amount, recurring value, currency or
// product list on a deal whose lines set them.
type LineOwnedValueError struct{ Field string }

func (e *LineOwnedValueError) Error() string {
	return "this deal takes its " + e.Field + " from its product lines; change the lines instead"
}

// FieldFault names the field the caller tried to set.
func (e *LineOwnedValueError) FieldFault() (field, code, message string) {
	return e.Field, "deal_has_lines", e.Error()
}

// EstimatePresentError refuses a first line on a deal that holds a typed value
// the request did not say to replace.
type EstimatePresentError struct{}

func (e *EstimatePresentError) Error() string {
	return "this deal holds a typed value; send replace_estimate: true to price it from its lines instead"
}

// FieldFault points at the flag that confirms the replacement.
func (e *EstimatePresentError) FieldFault() (field, code, message string) {
	return "replace_estimate", "estimate_present", e.Error()
}

// DealLinesClosedError refuses a line edit on a deal that is no longer open:
// its lines are what it was won or lost at.
type DealLinesClosedError struct{ Status string }

func (e *DealLinesClosedError) Error() string {
	return "the deal is " + e.Status + "; reopen it to change its products"
}

// MessageFault names the state and no field.
func (e *DealLinesClosedError) MessageFault() (code, message string) {
	return "deal_not_open", e.Error()
}

// LineCurrencyError refuses a line in a currency other than the deal's lines.
type LineCurrencyError struct{ Lines, Asked string }

func (e *LineCurrencyError) Error() string {
	return "this deal's lines are priced in " + e.Lines + ", not " + e.Asked +
		"; remove every line before you price the deal in another currency"
}

// FieldFault points at the currency.
func (e *LineCurrencyError) FieldFault() (field, code, message string) {
	return currencyField, "line_currency_mismatch", e.Error()
}

func staleRevision(expected, current int64) error {
	return &apperrors.VersionSkewError{Message: fmt.Sprintf(
		"The deal's products changed since revision %d (now %d). Reload them and apply your change again.", expected, current)}
}

// AddDealLine adds a product or custom item to a deal.
func (s *Store) AddDealLine(ctx context.Context, dealID ids.DealID, in AddDealLineInput) (crmcontracts.DealCommercial, error) {
	if err := requireLineWrite(ctx, in.ProductID != nil); err != nil {
		return crmcontracts.DealCommercial{}, err
	}
	var out crmcontracts.DealCommercial
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		open, err := openDealLines(ctx, tx, dealID, in.ExpectedRevision)
		if err != nil {
			return err
		}
		if open.Head.Mode != modeLines && (open.Head.AmountMinor != nil || open.Head.ArrMinor != nil) && !in.ReplaceEstimate {
			return &EstimatePresentError{}
		}
		if len(open.Lines) >= maxDealLines {
			return &FigureError{Field: "line_items", Rule: fmt.Sprintf("holds at most %d lines on one deal", maxDealLines)}
		}
		currency, err := lineCurrency(open, in.Currency)
		if err != nil {
			return err
		}
		line, err := s.resolveNewDealLine(ctx, tx, currency, in)
		if err != nil {
			return err
		}
		line.Position = nextDealLinePosition(open.Lines)
		if err := insertDealLine(ctx, tx, dealID, line); err != nil {
			return err
		}
		if err := s.settleDealLines(ctx, tx, open, map[string]any{"line_added": line.ID}); err != nil {
			return err
		}
		out, err = s.dealCommercialForCaller(ctx, tx, dealID)
		return err
	})
	return out, err
}

// UpdateDealLine changes one line of a deal.
func (s *Store) UpdateDealLine(ctx context.Context, dealID ids.DealID, lineID ids.UUID, in UpdateDealLineInput) (crmcontracts.DealCommercial, error) {
	if err := requireLineWrite(ctx, false); err != nil {
		return crmcontracts.DealCommercial{}, err
	}
	var out crmcontracts.DealCommercial
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		open, err := openDealLines(ctx, tx, dealID, in.ExpectedRevision)
		if err != nil {
			return err
		}
		current, ok := open.line(lineID)
		if !ok {
			return apperrors.ErrNotFound
		}
		next, err := s.patchDealLine(ctx, tx, current, in)
		if err != nil {
			return err
		}
		if sameDealLine(next, current) {
			out, err = s.dealCommercialForCaller(ctx, tx, dealID)
			return err
		}
		if err := updateDealLineRow(ctx, tx, next); err != nil {
			return err
		}
		if err := s.settleDealLines(ctx, tx, open, map[string]any{"line_changed": lineID}); err != nil {
			return err
		}
		out, err = s.dealCommercialForCaller(ctx, tx, dealID)
		return err
	})
	return out, err
}

// RemoveDealLine removes one line. The last one removed leaves the deal in
// `lines` mode with no value, rather than bringing an old estimate back.
func (s *Store) RemoveDealLine(ctx context.Context, dealID ids.DealID, lineID ids.UUID, expectedRevision int64) (crmcontracts.DealCommercial, error) {
	if err := requireLineWrite(ctx, false); err != nil {
		return crmcontracts.DealCommercial{}, err
	}
	var out crmcontracts.DealCommercial
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		open, err := openDealLines(ctx, tx, dealID, expectedRevision)
		if err != nil {
			return err
		}
		if _, ok := open.line(lineID); !ok {
			return apperrors.ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM deal_line_item WHERE id = $1 AND deal_id = $2`, lineID, dealID); err != nil {
			return fmt.Errorf("delete deal line: %w", err)
		}
		if err := s.settleDealLines(ctx, tx, open, map[string]any{"line_removed": lineID}); err != nil {
			return err
		}
		out, err = s.dealCommercialForCaller(ctx, tx, dealID)
		return err
	})
	return out, err
}

// UseDealEstimate returns a deal with no line to a typed estimate. It brings
// no old value back: the deal is unpriced until somebody types one.
func (s *Store) UseDealEstimate(ctx context.Context, dealID ids.DealID, expectedRevision int64) (crmcontracts.DealCommercial, error) {
	if err := requireLineWrite(ctx, false); err != nil {
		return crmcontracts.DealCommercial{}, err
	}
	var out crmcontracts.DealCommercial
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		open, err := openDealLines(ctx, tx, dealID, expectedRevision)
		if err != nil {
			return err
		}
		if len(open.Lines) > 0 {
			return &LineOwnedValueError{Field: "line_items"}
		}
		if open.Head.Mode == modeLines {
			if err := s.leaveLinesMode(ctx, tx, open); err != nil {
				return err
			}
		}
		out, err = s.dealCommercialForCaller(ctx, tx, dealID)
		return err
	})
	return out, err
}

// requireLineWrite is the object gate every line write passes: the deal's
// update grant, and the catalogue's read grant to pick a product from it.
func requireLineWrite(ctx context.Context, picksProduct bool) error {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return err
	}
	if picksProduct {
		return auth.Require(ctx, "product", principal.ActionRead)
	}
	return nil
}

// openedDealLines is what a line write holds once it is admitted: the deal's
// row lock, its commercial state and its lines, all read under that lock.
type openedDealLines struct {
	DealID ids.DealID
	Lock   storekit.RowLock
	Head   dealCommercialHead
	Lines  []dealLineRow
}

func (o openedDealLines) line(id ids.UUID) (dealLineRow, bool) {
	for _, line := range o.Lines {
		if line.ID == id {
			return line, true
		}
	}
	return dealLineRow{}, false
}

// openDealLines locks the deal, then asks the write and mask checks, then
// compares the revision the caller read. The lock comes first so a
// reassignment or a second editor cannot slip between the check and the write.
func openDealLines(ctx context.Context, tx pgx.Tx, dealID ids.DealID, expectedRevision int64) (openedDealLines, error) {
	lock, err := storekit.LockRow(ctx, tx, dealTable, dealID.UUID, storekit.LiveOnly)
	if err != nil {
		return openedDealLines{}, err
	}
	if err := auth.EnsureWritable(ctx, tx, dealTable, dealID.UUID); err != nil {
		return openedDealLines{}, err
	}
	readable, err := dealAmountReadable(ctx, tx, dealID)
	if err != nil {
		return openedDealLines{}, err
	}
	if !readable {
		return openedDealLines{}, apperrors.ErrPermissionDenied
	}
	head, err := readDealCommercialHead(ctx, tx, dealID)
	if err != nil {
		return openedDealLines{}, err
	}
	if head.Status != string(DealOpen) {
		return openedDealLines{}, &DealLinesClosedError{Status: head.Status}
	}
	if head.Revision != expectedRevision {
		return openedDealLines{}, staleRevision(expectedRevision, head.Revision)
	}
	lines, err := readDealLineRows(ctx, tx, dealID)
	if err != nil {
		return openedDealLines{}, err
	}
	return openedDealLines{DealID: dealID, Lock: lock, Head: head, Lines: lines}, nil
}

// lineCurrency is the currency a new line is priced in: the other lines',
// else the request's, else the deal's.
func lineCurrency(open openedDealLines, asked *string) (string, error) {
	if len(open.Lines) > 0 {
		held := open.Lines[0].Currency
		if asked != nil && *asked != held {
			return "", &LineCurrencyError{Lines: held, Asked: *asked}
		}
		return held, nil
	}
	if asked != nil {
		return *asked, checkCurrency(*asked)
	}
	if open.Head.Currency != nil {
		return *open.Head.Currency, nil
	}
	return "", &RequiredFieldError{Field: currencyField}
}

func nextDealLinePosition(lines []dealLineRow) int {
	next := 1
	for _, line := range lines {
		if line.Position >= next {
			next = line.Position + 1
		}
	}
	return next
}

// lineOwnedColumns are the deal columns a deal priced from lines takes from
// its lines and from nothing else.
var lineOwnedColumns = []string{amountField, arrField, currencyField, arrSourceField}

// refuseTypedMoneyOnLines refuses a patch that moves a line-owned column on a
// deal priced from its lines. Re-sending the value the deal holds moves
// nothing and passes, so a form that echoes the record still saves.
func refuseTypedMoneyOnLines(ctx context.Context, tx pgx.Tx, dealID ids.DealID, p *storekit.Patch) error {
	moved := p.Moved()
	var field string
	for _, column := range lineOwnedColumns {
		if _, ok := moved[column]; ok {
			field = column
			break
		}
	}
	if field == "" {
		return nil
	}
	head, err := readDealCommercialHead(ctx, tx, dealID)
	if err != nil {
		return err
	}
	if head.Mode == modeLines {
		return &LineOwnedValueError{Field: field}
	}
	return nil
}
