// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func emitSourceChange(ctx context.Context, tx pgx.Tx, auditID ids.UUID, change sourceChange) error {
	fields := map[string]any{"source": change.After}
	if change.SystemChanged {
		fields["source_system"] = change.AfterSystem
	}
	var payload events.Payload
	switch change.EntityType {
	case "contact":
		payload = crmcontracts.PublicEventContactUpdated{ChangedFields: fields}
	case "company":
		payload = crmcontracts.PublicEventCompanyUpdated{ChangedFields: fields}
	case "lead":
		payload = crmcontracts.PublicEventLeadUpdated{ChangedFields: fields}
	case "deal":
		payload = crmcontracts.PublicEventDealUpdated{ChangedFields: fields}
	case "project":
		payload = crmcontracts.PublicEventProjectUpdated{ChangedFields: fields}
	case "offer":
		if change.DealID == nil {
			return errors.New("a source update for an offer needs its deal")
		}
		payload = crmcontracts.PublicEventOfferUpdated{ChangedFields: []string{"source"}, Source: &change.After, OfferId: openapi_types.UUID(change.ID), DealId: openapi_types.UUID(*change.DealID)}
	default:
		return fmt.Errorf("source change has unknown entity %q", change.EntityType)
	}
	if err := EmitEvent(ctx, tx, auditID, change.ID, payload); err != nil {
		return err
	}
	database.Mark(database.CommitOwner(tx), sourceEventMark+auditID.String())
	return nil
}

func stageSourceChange(tx pgx.Tx, staged *stagedEvent) error {
	auditID := staged.env.Trace.AuditLogID
	if !database.Marked(database.CommitOwner(tx), sourceAuditMemo+auditID.String()) || staged.env.Type != staged.env.Entity.Type+".updated" {
		return nil
	}
	change, err := database.Remember(database.CommitOwner(tx), sourceAuditMemo+auditID.String(), func() (sourceChange, error) { return sourceChange{}, errors.New("source audit metadata is missing") })
	if err != nil {
		return err
	}
	if staged.env.Entity.ID != change.ID || staged.env.Entity.Type != change.EntityType {
		return errors.New("source event does not match its audit record")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(staged.env.Payload, &payload); err != nil {
		return err
	}
	if change.EntityType == "offer" {
		var fields []string
		if err := json.Unmarshal(payload["changed_fields"], &fields); err != nil {
			return err
		}
		if !slices.Contains(fields, "source") {
			fields = append(fields, "source")
		}
		payload["changed_fields"], err = json.Marshal(fields)
		if err == nil {
			payload["source"], err = json.Marshal(change.After)
		}
	} else {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(payload["changed_fields"], &fields); err != nil {
			return err
		}
		if fields == nil {
			fields = map[string]json.RawMessage{}
		}
		if value, exists := fields["source"]; exists {
			var existing string
			if json.Unmarshal(value, &existing) == nil && existing != change.After {
				return nil
			}
		}
		fields["source"], err = json.Marshal(change.After)
		if err == nil && change.SystemChanged {
			fields["source_system"], err = json.Marshal(change.AfterSystem)
		}
		if err == nil {
			payload["changed_fields"], err = json.Marshal(fields)
		}
	}
	if err != nil {
		return err
	}
	if staged.env.Payload, err = json.Marshal(payload); err != nil {
		return err
	}
	if staged.body, err = json.Marshal(staged.env); err != nil {
		return err
	}
	database.Mark(database.CommitOwner(tx), sourceEventMark+auditID.String())
	return nil
}
