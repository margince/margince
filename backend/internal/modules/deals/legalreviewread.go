// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The reads of a legal review. The deal page lists a deal's reviews to any
// reader of the deal, because the deal's owner acts on legal's comments. The
// queue and the pack need the legal_review grant.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// reviewSummaryColumns is the SELECT list scanReviewSummary reads, over
// legal_review aliased r.
const reviewSummaryColumns = `r.id, r.deal_id, r.status, r.note, r.comments, r.previous_review_id,
	r.requested_by, (SELECT display_name FROM app_user WHERE id = r.requested_by),
	r.decided_by, (SELECT display_name FROM app_user WHERE id = r.decided_by),
	r.decided_at, r.created_at, r.version`

func scanReviewSummary(row pgx.Row, extra ...any) (crmcontracts.LegalReviewSummary, error) {
	var out crmcontracts.LegalReviewSummary
	var id, dealID ids.UUID
	var previous, requestedBy, decidedBy *ids.UUID
	var status string
	dest := append([]any{
		&id, &dealID, &status, &out.Note, &out.Comments, &previous,
		&requestedBy, &out.RequestedByName, &decidedBy, &out.DecidedByName,
		&out.DecidedAt, &out.CreatedAt, &out.Version,
	}, extra...)
	if err := row.Scan(dest...); err != nil {
		return out, err
	}
	out.Id, out.DealId = openapiUUID(id), openapiUUID(dealID)
	out.Status = crmcontracts.LegalReviewStatus(status)
	out.PreviousReviewId, out.RequestedBy, out.DecidedBy = uuidPtr(previous), uuidPtr(requestedBy), uuidPtr(decidedBy)
	return out, nil
}

func readReviewSummary(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.LegalReviewSummary, error) {
	out, err := scanReviewSummary(tx.QueryRow(ctx,
		`SELECT `+reviewSummaryColumns+` FROM legal_review r WHERE r.id = $1`, id))
	if err != nil {
		return out, fmt.Errorf("read the legal review: %w", err)
	}
	return out, nil
}

func dealReviewSummaries(ctx context.Context, tx pgx.Tx, dealID ids.UUID) ([]crmcontracts.LegalReviewSummary, error) {
	rows, err := tx.Query(ctx, `SELECT `+reviewSummaryColumns+`
		  FROM legal_review r WHERE r.deal_id = $1 ORDER BY r.created_at DESC, r.id DESC`, dealID)
	if err != nil {
		return nil, fmt.Errorf("list the deal's legal reviews: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.LegalReviewSummary, error) {
		return scanReviewSummary(row)
	})
}

// ListDealLegalReviews answers a deal's reviews, newest first, to a reader of
// the deal.
func (s *Store) ListDealLegalReviews(ctx context.Context, dealID ids.DealID) ([]crmcontracts.LegalReviewSummary, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.LegalReviewSummary
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := ensureLegalReviewOn(ctx, tx); err != nil {
			return err
		}
		if err := auth.EnsureVisibleLive(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		var err error
		out, err = dealReviewSummaries(ctx, tx, dealID.UUID)
		return err
	})
	return out, err
}

// GetLegalReview answers one review with its pack and the deal's other
// reviews, to a holder of legal_review.read. The pack is the reviewer's
// authority to read the deal's facts, so no deal grant is asked. What keeps
// its own boundary is in legalreviewview.go.
func (s *Store) GetLegalReview(ctx context.Context, id ids.UUID) (crmcontracts.LegalReview, error) {
	var out crmcontracts.LegalReview
	if err := auth.Require(ctx, legalReviewEntity, principal.ActionRead); err != nil {
		return out, err
	}
	if s.reviewDocs == nil {
		return out, errors.New("deals: legal review needs a document reader, and none is wired")
	}
	var packet reviewPacket
	var sight reviewerSight
	var names packetNames
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := ensureLegalReviewOn(ctx, tx); err != nil {
			return err
		}
		var raw []byte
		review, err := scanReviewSummary(tx.QueryRow(ctx,
			`SELECT `+reviewSummaryColumns+`, r.packet FROM legal_review r WHERE r.id = $1`, id), &raw)
		if err != nil {
			return notFoundOr(err, "read the legal review")
		}
		out.Review = review
		if packet, err = decodePacket(raw); err != nil {
			return err
		}
		if sight, err = readReviewerSight(ctx, tx, []ids.UUID{packet.Deal.ID}, packetCompanyOf(packet)); err != nil {
			return err
		}
		if names, err = readPacketNames(ctx, tx, packet); err != nil {
			return err
		}
		all, err := dealReviewSummaries(ctx, tx, ids.UUID(review.DealId))
		if err != nil {
			return err
		}
		out.Earlier = make([]crmcontracts.LegalReviewSummary, 0, len(all))
		for _, other := range all {
			if other.Id != review.Id {
				out.Earlier = append(out.Earlier, other)
			}
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	// After the transaction: the file reader opens its own connection.
	openable, err := s.reviewDocs.OpenableDocuments(ctx, packetDocumentIDs(packet))
	if err != nil {
		return out, err
	}
	out.Packet = packetWire(packet, names, openable, sight)
	return out, nil
}

// LegalReviewQueueFilter picks the open requests or the answered ones.
type LegalReviewQueueFilter string

// The two halves of the reviewer's queue.
const (
	QueueOpen    LegalReviewQueueFilter = "open"
	QueueDecided LegalReviewQueueFilter = "decided"
)

// reviewQueueCursor names the last row of a page and nothing else. The next
// page reads that row's sort keys from the table itself. So a token carries no
// value or date, and a forged one only names a review to page after.
type reviewQueueCursor struct {
	ID ids.UUID `json:"id"`
}

// queueValueKey and queueCloseKey are the open queue's terms over alias a. A
// missing value and a missing close date sort last, standing in as the
// smallest bigint and the latest date. A reader who may not see a term sorts
// by a constant, which orders nothing.
func queueValueKey(a string, sees bool) string {
	if !sees {
		return `0::bigint`
	}
	return `COALESCE(` + a + `.value_minor, -9223372036854775808)`
}

func queueCloseKey(a string, sees bool) string {
	if !sees {
		return `DATE '9999-12-31'`
	}
	return `COALESCE(` + a + `.expected_close_date, DATE '9999-12-31')`
}

// queueSight is which terms a reader may sort the queue by.
type queueSight struct{ value, closeDate bool }

// queueSortKeys answers which terms this reader may sort by. Both need
// deal.read over every deal, since the order spans deals of every owner. The
// value also needs no mask on the amount or its currency. The read-only mask
// is asked, so one that lifts on some rows still counts.
func queueSortKeys(ctx context.Context) (queueSight, error) {
	if !auth.ReadGranted(ctx, "deal") {
		return queueSight{}, nil
	}
	p, err := storekit.Actor(ctx)
	if err != nil || !auth.UnboundedFor(p, dealTable) {
		return queueSight{}, err
	}
	masked := auth.MaskedFields(p, maskObject, false)
	value := !slices.Contains(masked, dealAmountField) && !slices.Contains(masked, dealCurrencyField)
	return queueSight{value: value, closeDate: true}, nil
}

// ListLegalReviewQueue answers one page of the reviewer's queue.
func (s *Store) ListLegalReviewQueue(
	ctx context.Context, filter LegalReviewQueueFilter, cursor *string, limit *int,
) ([]crmcontracts.LegalReviewQueueItem, storekit.Page, error) {
	if err := auth.Require(ctx, legalReviewEntity, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	if filter != QueueOpen && filter != QueueDecided {
		return nil, storekit.Page{}, &values.ParseError{
			Field: filterStatus, Code: "value_not_allowed", Message: "status is one of open, decided",
		}
	}
	lim := storekit.ClampLimit(limit)
	var after *reviewQueueCursor
	if cursor != nil && *cursor != "" {
		c, err := storekit.DecodeOpaque[reviewQueueCursor](*cursor)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		if c.ID.IsZero() {
			return nil, storekit.Page{}, &storekit.MalformedCursorError{}
		}
		after = &c
	}
	var (
		out  []crmcontracts.LegalReviewQueueItem
		keys []reviewQueueCursor
		page storekit.Page
	)
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := ensureLegalReviewOn(ctx, tx); err != nil {
			return err
		}
		var err error
		out, keys, err = readReviewQueue(ctx, tx, filter, after, lim+1)
		return err
	})
	if err != nil {
		return nil, page, err
	}
	if len(out) > lim {
		out = out[:lim]
		next, err := storekit.EncodeOpaque(keys[lim-1])
		if err != nil {
			return nil, page, err
		}
		page = storekit.Page{HasMore: true, NextCursor: next}
	}
	return out, page, nil
}

func readReviewQueue(
	ctx context.Context, tx pgx.Tx, filter LegalReviewQueueFilter, after *reviewQueueCursor, take int,
) ([]crmcontracts.LegalReviewQueueItem, []reviewQueueCursor, error) {
	sight, err := queueSortKeys(ctx)
	if err != nil {
		return nil, nil, err
	}
	value, closeOn := queueValueKey("r", sight.value), queueCloseKey("r", sight.closeDate)
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	where := []string{"r.status = 'requested'"}
	order := value + ` DESC, ` + closeOn + ` ASC, r.created_at ASC, r.id ASC`
	if filter == QueueDecided {
		where = []string{"r.decided_at IS NOT NULL"}
		order = `r.decided_at DESC, r.id DESC`
	}
	if after != nil {
		where = append(where, queueAfterClause(filter, after.ID, arg, sight))
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT `+reviewSummaryColumns+`, (SELECT d.name FROM deal d WHERE d.id = r.deal_id),
		       (SELECT c.display_name FROM company c WHERE c.id = (r.packet #>> '{deal,company_id}')::uuid),
		       (r.packet #>> '{deal,company_id}')::uuid,
		       r.packet #>> '{deal,stage_name}', r.value_minor, r.value_currency,
		       r.expected_close_date
		  FROM legal_review r
		 WHERE %s
		 ORDER BY %s
		 LIMIT %d`, strings.Join(where, " AND "), order, take), args...)
	if err != nil {
		return nil, nil, fmt.Errorf("read the legal review queue: %w", err)
	}
	out, companies, keys, err := scanReviewQueue(rows)
	if err != nil {
		return nil, nil, err
	}
	return out, keys, withholdFromReviewer(ctx, tx, out, companies)
}

func scanReviewQueue(rows pgx.Rows) ([]crmcontracts.LegalReviewQueueItem, []*ids.UUID, []reviewQueueCursor, error) {
	defer rows.Close()
	var out []crmcontracts.LegalReviewQueueItem
	var companies []*ids.UUID
	var keys []reviewQueueCursor
	for rows.Next() {
		var item crmcontracts.LegalReviewQueueItem
		var key reviewQueueCursor
		var closeDate *time.Time
		var companyID *ids.UUID
		review, err := scanReviewSummary(rows, &item.DealName, &item.CompanyName, &companyID, &item.StageName,
			&item.ValueMinor, &item.ValueCurrency, &closeDate)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("scan the legal review queue: %w", err)
		}
		item.Review = review
		if closeDate != nil {
			item.ExpectedCloseDate = &openapi_types.Date{Time: *closeDate}
		}
		key.ID = ids.UUID(review.Id)
		out = append(out, item)
		companies = append(companies, companyID)
		keys = append(keys, key)
	}
	return out, companies, keys, rows.Err()
}

// withholdFromReviewer takes off each queue row what this reader may not see
// now. That is the deal's facts without deal.read, the value their field
// masks hide, and a company they cannot see.
func withholdFromReviewer(
	ctx context.Context, tx pgx.Tx, items []crmcontracts.LegalReviewQueueItem, companies []*ids.UUID,
) error {
	dealIDs := make([]ids.UUID, 0, len(items))
	companyOf := map[ids.UUID]ids.UUID{}
	for i, item := range items {
		dealIDs = append(dealIDs, ids.UUID(item.Review.DealId))
		if companies[i] != nil {
			companyOf[ids.UUID(item.Review.DealId)] = *companies[i]
		}
	}
	sight, err := readReviewerSight(ctx, tx, dealIDs, companyOf)
	if err != nil {
		return err
	}
	for i := range items {
		seen := sight.deals[ids.UUID(items[i].Review.DealId)]
		if !seen.facts {
			items[i].StageName, items[i].ExpectedCloseDate = nil, nil
		}
		if !seen.money {
			items[i].ValueMinor, items[i].ValueCurrency = nil, nil
		}
		if !seen.company {
			items[i].CompanyName = nil
		}
	}
	return nil
}

// queueAfterClause is the keyset predicate for one half of the queue, over
// every term its ORDER BY names, each read from the anchor row a.
func queueAfterClause(
	filter LegalReviewQueueFilter, anchor ids.UUID, arg func(any) int, sight queueSight,
) string {
	at := arg(anchor)
	if filter == QueueDecided {
		return storekit.SQLf(`(r.decided_at, r.id) < (SELECT a.decided_at, a.id FROM legal_review a WHERE a.id = $%d)`, at)
	}
	return storekit.SQLf(`EXISTS (SELECT 1 FROM legal_review a WHERE a.id = $%[5]d AND
		(%[1]s < %[3]s OR (%[1]s = %[3]s AND (%[2]s > %[4]s OR (%[2]s = %[4]s
		AND (r.created_at, r.id) > (a.created_at, a.id))))))`,
		queueValueKey("r", sight.value), queueCloseKey("r", sight.closeDate),
		queueValueKey("a", sight.value), queueCloseKey("a", sight.closeDate), at)
}
