// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// How a changed value is read: an amount with its currency, a stage or member
// with its name. The stored value travels unchanged beside it, so an older
// client keeps working and a newer one never prints minor units or an id.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// ReferenceNames names records of one type under the reader's own grants. A
// record the reader may not read is absent from the answer.
type ReferenceNames interface {
	Labels(ctx context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error)
}

// WithReferenceNames binds the naming of stages and members. Unbound, a change
// carries no labels, which a client reads as "not readable".
func (s *Service) WithReferenceNames(names ReferenceNames) *Service {
	s.names = names
	return s
}

// readAs states how a change's value is read. A money change takes the
// currency the write's own image carries; held currencies fill the rest.
func readAs(change *crmcontracts.MagicFieldChange, reading fieldReading, before, after map[string]any) {
	if reading.kind == "" {
		return
	}
	change.Type = new(reading.kind)
	if reading.precision != "" {
		change.Precision = new(reading.precision)
	}
	if reading.kind != crmcontracts.MagicFieldChangeTypeMoney {
		return
	}
	for _, image := range []map[string]any{after, before} {
		if code, ok := image[values.AmountCurrency].(string); ok && code != "" {
			change.Currency = &code
			return
		}
	}
}

// heldCurrency is the currency a deal holds now, and when a write last set it.
type heldCurrency struct {
	code  string
	setAt time.Time
}

// at answers the currency the deal held at an instant, which is the current one
// only when no later write set it.
func (h heldCurrency) at(instant time.Time) (string, bool) {
	if h.code == "" || h.setAt.After(instant) {
		return "", false
	}
	return h.code, true
}

// heldCurrencies reads the currency of each deal among the entries. A deal is
// the only record type with money, so the other types need no read.
func heldCurrencies(ctx context.Context, tx pgx.Tx, entries []entry) (map[ids.UUID]heldCurrency, error) {
	var deals []ids.UUID
	for _, e := range entries {
		if e.EntityType == typeDeal {
			deals = append(deals, e.EntityID)
		}
	}
	out := map[ids.UUID]heldCurrency{}
	if len(deals) == 0 {
		return out, nil
	}
	if err := auth.Require(ctx, typeDeal, principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		return nil, err
	}
	// The trail is pinned to 'deal' as a literal, so the read visibly reaches no activity image.
	rows, err := tx.Query(ctx, `
		SELECT d.id, COALESCE(d.currency::text, ''),
		       (SELECT max(a.occurred_at) FROM audit_log a
		         WHERE a.entity_type = 'deal' AND a.entity_id = d.id AND a.after ? $1)
		  FROM deal d WHERE d.id = ANY($2)`, values.AmountCurrency, deals)
	if err != nil {
		return nil, fmt.Errorf("read the currency a deal holds: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var held heldCurrency
		var setAt *time.Time
		if err := rows.Scan(&id, &held.code, &setAt); err != nil {
			return nil, fmt.Errorf("read the currency a deal holds: %w", err)
		}
		if setAt != nil {
			held.setAt = *setAt
		}
		out[id] = held
	}
	return out, rows.Err()
}

// typedChangesOf is changesOf with each money change's currency settled from
// the deal when the image did not carry one.
func typedChangesOf(e entry, held map[ids.UUID]heldCurrency) []crmcontracts.MagicFieldChange {
	changes := changesOf(e)
	for i := range changes {
		c := &changes[i]
		if c.Type == nil || *c.Type != crmcontracts.MagicFieldChangeTypeMoney || c.Currency != nil {
			continue
		}
		if code, ok := held[e.EntityID].at(e.OccurredAt); ok {
			c.Currency = &code
		}
	}
	return changes
}

// labelledTypes are the change kinds named by a record of another type.
var labelledTypes = map[crmcontracts.MagicFieldChangeType]string{
	crmcontracts.MagicFieldChangeTypeStage:  "stage",
	crmcontracts.MagicFieldChangeTypeMember: "user",
}

// labelReferences names the stages and members the changes point at, one read
// per kind. It opens its own reads, so it runs after the page's transaction.
func (s *Service) labelReferences(ctx context.Context, lists [][]crmcontracts.MagicFieldChange) error {
	if s.names == nil {
		return nil
	}
	wanted := map[string][]ids.UUID{}
	for _, list := range lists {
		for _, c := range list {
			if named, ok := labelledType(c); ok {
				for _, value := range []any{c.Before, c.After} {
					if id, isID := idOf(value); isID {
						wanted[named] = append(wanted[named], id)
					}
				}
			}
		}
	}
	names := map[string]map[ids.UUID]string{}
	for named, want := range wanted {
		labels, err := s.names.Labels(ctx, named, want)
		if err != nil {
			return fmt.Errorf("name the %ss a change points at: %w", named, err)
		}
		names[named] = labels
	}
	for _, list := range lists {
		for i := range list {
			if named, ok := labelledType(list[i]); ok {
				list[i].BeforeLabel = labelOf(names[named], list[i].Before)
				list[i].AfterLabel = labelOf(names[named], list[i].After)
			}
		}
	}
	return nil
}

func labelledType(c crmcontracts.MagicFieldChange) (string, bool) {
	if c.Type == nil {
		return "", false
	}
	named, ok := labelledTypes[*c.Type]
	return named, ok
}

func idOf(value any) (ids.UUID, bool) { //craft:ignore naked-any a value decoded from an audit image's JSON has no static type
	text, ok := value.(string)
	if !ok {
		return ids.Nil, false
	}
	id, err := ids.Parse(text)
	return id, err == nil && !id.IsZero()
}

func labelOf(labels map[ids.UUID]string, value any) *string { //craft:ignore naked-any a value decoded from an audit image's JSON has no static type
	id, ok := idOf(value)
	if !ok || labels[id] == "" {
		return nil
	}
	return new(labels[id])
}
