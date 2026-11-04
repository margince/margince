// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// An admin's declaration that one group of customers was told at collection
// that they may object to advertising: the evidence the existing-customer
// route reads for that condition (existingcustomerroute.go).
//
// A declaration is never edited. It states what happened in a past period, so
// a correction is a withdrawal with a reason and a new declaration, and the
// audit log keeps both.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const customerNoticeEntity = "existing_customer_notice"

// CustomerNotice is one declaration as stored.
type CustomerNotice struct {
	ID                ids.UUID
	LegalEntityID     ids.UUID
	CustomerGroup     string
	CollectionChannel string
	NoticeText        string
	EffectiveFrom     time.Time
	EffectiveUntil    *time.Time
	PurposeIDs        []ids.UUID
	CapturedBy        string
	CreatedAt         time.Time
	WithdrawnAt       *time.Time
	WithdrawnReason   *string
}

// DeclareNoticeInput is what an admin declares. Now is the caller's clock, so
// a period that starts in the future is refused against the same moment the
// caller sees.
type DeclareNoticeInput struct {
	LegalEntityID     ids.UUID
	CustomerGroup     string
	CollectionChannel string
	NoticeText        string
	EffectiveFrom     time.Time
	EffectiveUntil    *time.Time
	PurposeIDs        []ids.UUID
	Now               time.Time
}

func (in DeclareNoticeInput) validate() error {
	for _, f := range []struct {
		field, value, want string
		limit              int
	}{
		{"customer_group", in.CustomerGroup, "say which customers this covers", 500},
		{"collection_channel", in.CollectionChannel, "say how their addresses were collected", 200},
		{"notice_text", in.NoticeText, "give the notice the customers saw", 4000},
	} {
		if n := utf8.RuneCountInString(strings.TrimSpace(f.value)); n == 0 || n > f.limit {
			return &ValidationError{Field: f.field, Reason: fmt.Sprintf("%s, in 1 to %d characters", f.want, f.limit)}
		}
	}
	switch {
	case in.LegalEntityID.IsZero():
		return &ValidationError{Field: "legal_entity_id", Reason: "name the legal entity that collected the addresses"}
	case in.EffectiveFrom.IsZero() || in.EffectiveFrom.After(in.Now):
		return &ValidationError{Field: "effective_from", Reason: "give the day the notice was first given, no later than today"}
	case in.EffectiveUntil != nil && !in.EffectiveUntil.After(in.EffectiveFrom):
		return &ValidationError{Field: "effective_until", Reason: "the end of the period must come after its start"}
	case len(in.PurposeIDs) == 0:
		return &ValidationError{Field: "purpose_ids", Reason: "name at least one marketing purpose that advertises goods like the ones bought"}
	}
	return nil
}

// requireNoticeAdmin admits a signed-in user who may configure consent. An
// agent cannot declare evidence on its user's behalf.
func requireNoticeAdmin(ctx context.Context, action principal.Action) error {
	if err := auth.Require(ctx, "consent_config", action); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return fmt.Errorf("a collection notice is declared by a signed-in user: %w", apperrors.ErrPermissionDenied)
	}
	return nil
}

// DeclareCustomerNotice records one declaration with its audit row and event.
func (s *Store) DeclareCustomerNotice(ctx context.Context, in DeclareNoticeInput) (CustomerNotice, error) {
	if err := requireNoticeAdmin(ctx, principal.ActionCreate); err != nil {
		return CustomerNotice{}, err
	}
	if err := in.validate(); err != nil {
		return CustomerNotice{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return CustomerNotice{}, err
	}
	var out CustomerNotice
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := checkNoticeReferences(ctx, tx, in); err != nil {
			return err
		}
		var id ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO existing_customer_notice
			    (legal_entity_id, customer_group, collection_channel, notice_text,
			     effective_from, effective_until, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id`,
			in.LegalEntityID, strings.TrimSpace(in.CustomerGroup), strings.TrimSpace(in.CollectionChannel),
			strings.TrimSpace(in.NoticeText), in.EffectiveFrom, in.EffectiveUntil, by).Scan(&id); err != nil {
			return fmt.Errorf("consent: recording the collection notice: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO existing_customer_notice_purpose (notice_id, purpose_id)
			SELECT $1, unnest($2::uuid[]) ON CONFLICT DO NOTHING`, id, in.PurposeIDs); err != nil {
			return fmt.Errorf("consent: recording the purposes the notice covers: %w", err)
		}
		notice, err := customerNoticeTx(ctx, tx, id)
		if err != nil {
			return err
		}
		auditID, err := storekit.AuditEvent(ctx, tx, "create", customerNoticeEntity, id, map[string]any{
			"legal_entity_id": in.LegalEntityID.String(), "effective_from": in.EffectiveFrom,
			"effective_until": in.EffectiveUntil, "purposes": len(notice.PurposeIDs),
		})
		if err != nil {
			return err
		}
		out = notice
		return storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.InternalEventExistingCustomerNoticeCreated{
			LegalEntityId: openapi_types.UUID(in.LegalEntityID),
		})
	})
	return out, err
}

// checkNoticeReferences refuses an archived entity, and a purpose that is not
// marketing: only marketing needs this route.
func checkNoticeReferences(ctx context.Context, tx pgx.Tx, in DeclareNoticeInput) error {
	var live bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM legal_entity WHERE id = $1 AND archived_at IS NULL)`,
		in.LegalEntityID).Scan(&live); err != nil {
		return fmt.Errorf("consent: reading the legal entity: %w", err)
	}
	if !live {
		return &ValidationError{Field: "legal_entity_id", Reason: "name a legal entity that is not archived"}
	}
	var marketing int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM consent_purpose
		 WHERE id = ANY($1::uuid[]) AND class = $2`, in.PurposeIDs, string(ClassMarketing)).Scan(&marketing); err != nil {
		return fmt.Errorf("consent: reading the purposes: %w", err)
	}
	if marketing != len(uniqueIDs(in.PurposeIDs)) {
		return &ValidationError{Field: "purpose_ids", Reason: "name marketing purposes only"}
	}
	return nil
}

func uniqueIDs(in []ids.UUID) map[ids.UUID]struct{} {
	out := make(map[ids.UUID]struct{}, len(in))
	for _, id := range in {
		out[id] = struct{}{}
	}
	return out
}

// CustomerNotices lists every declaration, live ones first, newest first.
func (s *Store) CustomerNotices(ctx context.Context) ([]CustomerNotice, error) {
	if err := requireNoticeAdmin(ctx, principal.ActionRead); err != nil {
		return nil, err
	}
	var out []CustomerNotice
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = customerNoticesTx(ctx, tx, nil)
		return err
	})
	return out, err
}

// WithdrawCustomerNotice stops a declaration counting from now on, with the
// reason on the row and in the audit log. A send already made on it stays
// explained by its own decision row.
func (s *Store) WithdrawCustomerNotice(ctx context.Context, id ids.UUID, reason string) (CustomerNotice, error) {
	if err := requireNoticeAdmin(ctx, principal.ActionUpdate); err != nil {
		return CustomerNotice{}, err
	}
	if err := requireReason(reason, "withdrawing a collection notice"); err != nil {
		return CustomerNotice{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return CustomerNotice{}, err
	}
	var out CustomerNotice
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var withdrawnAt time.Time
		err := tx.QueryRow(ctx, `
			UPDATE existing_customer_notice
			   SET withdrawn_at = now(), withdrawn_by = $2, withdrawn_reason = $3
			 WHERE id = $1 AND withdrawn_at IS NULL
			RETURNING withdrawn_at`, id, by, strings.TrimSpace(reason)).Scan(&withdrawnAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("consent: withdrawing the collection notice: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "update", customerNoticeEntity, id,
			map[string]any{"withdrawn_at": nil, fieldReason: nil},
			map[string]any{"withdrawn_at": withdrawnAt, fieldReason: strings.TrimSpace(reason)})
		if err != nil {
			return err
		}
		if out, err = customerNoticeTx(ctx, tx, id); err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.InternalEventExistingCustomerNoticeWithdrawn{})
	})
	return out, err
}

func customerNoticeTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (CustomerNotice, error) {
	notices, err := customerNoticesTx(ctx, tx, &id)
	if err != nil {
		return CustomerNotice{}, err
	}
	if len(notices) == 0 {
		return CustomerNotice{}, apperrors.ErrNotFound
	}
	return notices[0], nil
}

// customerNoticesTx reads one declaration, or every one when id is nil.
func customerNoticesTx(ctx context.Context, tx pgx.Tx, id *ids.UUID) ([]CustomerNotice, error) {
	rows, err := tx.Query(ctx, `
		SELECT n.id, n.legal_entity_id, n.customer_group, n.collection_channel, n.notice_text,
		       n.effective_from, n.effective_until, n.captured_by, n.created_at,
		       n.withdrawn_at, n.withdrawn_reason,
		       coalesce(array_agg(np.purpose_id ORDER BY np.purpose_id)
		                FILTER (WHERE np.purpose_id IS NOT NULL), '{}')
		  FROM existing_customer_notice n
		  LEFT JOIN existing_customer_notice_purpose np ON np.notice_id = n.id
		 WHERE $1::uuid IS NULL OR n.id = $1
		 GROUP BY n.id
		 ORDER BY n.withdrawn_at IS NOT NULL, n.created_at DESC, n.id`, id)
	if err != nil {
		return nil, fmt.Errorf("consent: reading the collection notices: %w", err)
	}
	notices, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CustomerNotice, error) {
		var n CustomerNotice
		err := row.Scan(&n.ID, &n.LegalEntityID, &n.CustomerGroup, &n.CollectionChannel, &n.NoticeText,
			&n.EffectiveFrom, &n.EffectiveUntil, &n.CapturedBy, &n.CreatedAt,
			&n.WithdrawnAt, &n.WithdrawnReason, &n.PurposeIDs)
		return n, err
	})
	if err != nil {
		return nil, fmt.Errorf("consent: reading the collection notices: %w", err)
	}
	return notices, nil
}
