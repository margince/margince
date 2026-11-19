// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealscout

// What an opened deal rests on, asked again under the opening's own locks. The
// cited evidence must still be readable by the whole workspace, and an amount
// must be stated in the document's own words.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealverdict"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EvidenceHolds takes FOR SHARE on every cited item and the activities
// behind it, then asks Deal Scout's visibility rule of each again. An
// explicit request holds on mail of any audience, as the scout admitted it.
func EvidenceHolds(ctx context.Context, tx pgx.Tx, evidence []deals.SuggestionEvidence) (bool, error) {
	for _, item := range evidence {
		activities, request, ok, err := lockCitedItem(ctx, tx, item)
		if err != nil || !ok {
			return false, err
		}
		for _, activity := range activities {
			held, err := citedSourceHolds(ctx, tx, activity, request)
			if err != nil || !held {
				return false, err
			}
		}
	}
	return true, nil
}

// lockCitedItem holds one cited item and answers the activities it rests on,
// and whether it is an explicit request.
func lockCitedItem(ctx context.Context, tx pgx.Tx, item deals.SuggestionEvidence) ([]ids.UUID, bool, bool, error) {
	var query string
	var ref ids.UUID
	switch {
	case item.ActivityID != nil:
		return []ids.UUID{*item.ActivityID}, false, true, nil
	case item.SignalID != nil:
		ref = *item.SignalID
		query = `SELECT array(SELECT (` + storekit.CitedActivityID("cite") + `)::uuid
		          FROM jsonb_array_elements(s.evidence) cite WHERE cite->>'source_type' = 'activity'),
		       request.asked
		  FROM signal s
		  CROSS JOIN LATERAL (SELECT s.kind = 'new_opportunity' AND ` + BuyerSignal("s", true) + ` AS asked) request
		 WHERE s.id = $1 AND s.status = 'open' AND s.archived_at IS NULL
		   AND (s.visibility = 'workspace' OR (request.asked AND s.visibility = 'owner'))
		   FOR SHARE OF s`
	case item.AttachmentID != nil:
		ref = *item.AttachmentID
		query = `SELECT ARRAY[at.activity_id], false FROM attachment at
		 WHERE at.id = $1 AND at.archived_at IS NULL AND NOT at.bytes_withheld
		   FOR SHARE OF at`
	default:
		return nil, false, false, nil
	}
	var activities []ids.UUID
	var request bool
	err := tx.QueryRow(ctx, query, ref).Scan(&activities, &request)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, false, nil
	}
	if err != nil {
		return nil, false, false, fmt.Errorf("compose: holding the evidence a deal opens on: %w", err)
	}
	return activities, request, true, nil
}

// citedSourceHolds holds one activity and its links. It asks whether the whole
// workspace may still read it, or for a request whether it is still live.
func citedSourceHolds(ctx context.Context, tx pgx.Tx, activity ids.UUID, request bool) (bool, error) {
	if _, err := tx.Exec(ctx, `SELECT 1 FROM activity_link WHERE activity_id = $1 FOR SHARE`, activity); err != nil {
		return false, fmt.Errorf("compose: holding the evidence's links: %w", err)
	}
	var readable bool
	err := tx.QueryRow(ctx, `SELECT `+WorkspaceEvidence("a")+` OR ($2 AND `+RequestSource("a")+`)
	  FROM activity a WHERE a.id = $1 FOR SHARE OF a`, activity, request).Scan(&readable)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("compose: rereading the evidence's audience: %w", err)
	}
	return readable, nil
}

// scoutStatedAmount is the amount the newest cited document's current reading
// states in its own quoted words, with its currency. A reading whose quote is
// missing or names another figure gives no amount.
func scoutStatedAmount(ctx context.Context, tx pgx.Tx, evidence []deals.SuggestionEvidence) (*int64, *string, error) {
	for _, item := range evidence {
		if item.AttachmentID == nil {
			continue
		}
		var value, quote, currency *string
		err := tx.QueryRow(ctx, `
			SELECT (SELECT f->>'Value' FROM jsonb_array_elements(x.fields) f
			         WHERE f->>'Field' = 'amount_minor' AND NOT coalesce((f->>'Omitted')::boolean, false)),
			       (SELECT f->>'SourceQuote' FROM jsonb_array_elements(x.fields) f
			         WHERE f->>'Field' = 'amount_minor' AND NOT coalesce((f->>'Omitted')::boolean, false)),
			       (SELECT f->>'Value' FROM jsonb_array_elements(x.fields) f
			         WHERE f->>'Field' = 'currency' AND NOT coalesce((f->>'Omitted')::boolean, false))
			  FROM attachment_extraction x
			 WHERE x.attachment_id = $1 AND x.status = 'done' AND x.question = 'deal_facts'
			 ORDER BY x.finished_at DESC LIMIT 1
			   FOR SHARE OF x`, *item.AttachmentID).Scan(&value, &quote, &currency)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("compose: rereading the document's amount: %w", err)
		}
		if value == nil || currency == nil {
			continue
		}
		amount, stated := quoteBackedAmount(*value, *currency, quote)
		return amount, stated, nil
	}
	return nil, nil, nil
}

// quoteBackedAmount keeps a read amount only when the quote states that very
// number in that very currency. The value is a model's reading, so one that is
// no integer is a reading that states no amount, not a failure of the read.
func quoteBackedAmount(value, currency string, quote *string) (*int64, *string) {
	if quote == nil {
		return nil, nil
	}
	minor, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, nil
	}
	stated, statedCurrency, ok := dealverdict.QuotedPrice(*quote)
	if !ok || stated != minor || statedCurrency != currency {
		return nil, nil
	}
	return &minor, &currency
}

// citedSignal is one cited signal as the opening rereads it: its summary,
// and its request facts when it is an explicit request.
type citedSignal struct {
	summary string
	request *Request
}

// readCitedSignals rereads every cited signal, newest first, and reports
// whether any of them may not reach the deal. One such signal withholds every
// title from the deal's timeline.
func readCitedSignals(ctx context.Context, tx pgx.Tx, evidence []deals.SuggestionEvidence) ([]citedSignal, bool, error) {
	share, err := activities.SharesPrivateRequestsTx(ctx, tx)
	if err != nil {
		return nil, false, err
	}
	var out []citedSignal
	withheld := false
	for _, item := range evidence {
		if item.SignalID == nil {
			continue
		}
		var cited citedSignal
		var asked bool
		var request Request
		err := tx.QueryRow(ctx, `
			SELECT s.summary, facts.cite IS NOT NULL, facts.cite->'fields'->>'`+DecisionDateKey+`',
			       facts.cite->'fields'->>'`+BudgetQuoteKey+`', `+RequestShareableSQL("s", share)+`
			  FROM signal s
			  LEFT JOIN LATERAL (SELECT request_cite AS cite FROM jsonb_array_elements(s.evidence) request_cite
			         WHERE s.kind = 'new_opportunity' AND `+ExplicitRequestCite("request_cite")+` LIMIT 1) facts ON true
			 WHERE s.id = $1`, *item.SignalID).Scan(&cited.summary, &asked, &request.DecisionDate,
			&request.BudgetQuote, &request.Shareable)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, false, fmt.Errorf("compose: rereading the signals a deal opens on: %w", err)
		}
		withheld = withheld || !request.Shareable
		if asked {
			cited.request = &request
		}
		out = append(out, cited)
	}
	return out, withheld, nil
}

// Prefill is what an opened deal carries, under the opening's locks. DealTerms
// decides its amount and close date. A shareable request gives its summary as
// the description. Each value a model read is marked as model-written.
// Withheld says some cited signal's mail may not reach the deal.
func Prefill(now time.Time, today TodayFunc) func(context.Context, pgx.Tx, []deals.SuggestionEvidence) (deals.DealPrefill, error) {
	return func(ctx context.Context, tx pgx.Tx, evidence []deals.SuggestionEvidence) (deals.DealPrefill, error) {
		documentMinor, documentCurrency, err := scoutStatedAmount(ctx, tx, evidence)
		if err != nil {
			return deals.DealPrefill{}, err
		}
		signals, withheld, err := readCitedSignals(ctx, tx, evidence)
		if err != nil {
			return deals.DealPrefill{}, err
		}
		base, err := identity.BaseCurrencyAppliedTx(ctx, tx)
		if err != nil {
			return deals.DealPrefill{}, err
		}
		day, err := today(ctx, tx, now)
		if err != nil {
			return deals.DealPrefill{}, err
		}
		requests := make([]*Request, len(signals))
		for i, signal := range signals {
			requests[i] = signal.request
		}
		request := NewestRequest(requests)
		terms := DealTerms(documentMinor, documentCurrency, request, base, day)
		fill := deals.DealPrefill{
			AmountMinor: terms.AmountMinor, Currency: terms.Currency, CloseDate: terms.CloseDate, Withheld: withheld,
		}
		for i := range signals {
			if request == nil || signals[i].request != request {
				continue
			}
			if request.Shareable {
				fill.Description = &signals[i].summary
				fill.ModelWritten = append(fill.ModelWritten, storekit.FieldStamp{Field: dealDescriptionField})
			}
			break
		}
		if terms.BudgetRead {
			fill.ModelWritten = append(fill.ModelWritten, storekit.FieldStamp{Field: dealAmountField})
		}
		if terms.CurrencyAssumed {
			fill.ModelWritten = append(fill.ModelWritten, storekit.FieldStamp{Field: dealCurrencyField})
		}
		return fill, nil
	}
}

// TodayFunc is the installation's calendar day at an instant.
type TodayFunc func(ctx context.Context, tx pgx.Tx, now time.Time) (time.Time, error)

// The deal columns an opening marks as model-written.
const (
	dealDescriptionField = "description"
	dealAmountField      = "amount_minor"
	dealCurrencyField    = "currency"
)
