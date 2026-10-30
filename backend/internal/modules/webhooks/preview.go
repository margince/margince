// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sampleOrigin stands in for the installation's address in a preview link.
const sampleOrigin = "https://margince.example"

// sampleIDs are fixed so a preview reads the same on every render.
var sampleIDs = struct{ event, entity, correlation ids.UUID }{
	event:       ids.MustParse("0190a7f2-0000-7000-8000-000000000001"),
	entity:      ids.MustParse("0190a7f2-0000-7000-8000-000000000002"),
	correlation: ids.MustParse("0190a7f2-0000-7000-8000-000000000003"),
}

// PreviewBody renders a template against a sample of eventType with the
// engine deliveries use. It sends nothing, stores nothing, and reads no
// record, so seeing the subscriptions is permission enough.
func (s *Store) PreviewBody(ctx context.Context, eventType, template string, contentType *string) ([]byte, string, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionRead); err != nil {
		return nil, "", err
	}
	tpl, err := parseBodyTemplate(template, deref(contentType))
	if err != nil {
		return nil, "", err
	}
	envelope, err := sampleEnvelope(eventType)
	if err != nil {
		return nil, "", err
	}
	body, err := tpl.render(envelope)
	if err != nil {
		return nil, "", &BadInputError{Field: fieldBodyTemplate, Reason: err.Error()}
	}
	return body, tpl.contentType, nil
}

func sampleEnvelope(eventType string) ([]byte, error) {
	data, ok := crmcontracts.PublicEventSamples[eventType]
	if !ok {
		return nil, &BadInputError{Field: "event_type", Reason: fmt.Sprintf("%q is not a published event type", eventType)}
	}
	entityType, _, _ := strings.Cut(eventType, ".")
	entity := crmcontracts.PublicEventEntityRef{Type: entityType, Id: openapi_types.UUID(sampleIDs.entity)}
	entity.Url = recordURL(sampleOrigin, entityType, sampleIDs.entity)
	if _, named := namedRecords[entityType]; named {
		name, owner := "Sample "+entityType+" name", "Sample owner name"
		entity.Name, entity.OwnerName = &name, &owner
	}
	return json.Marshal(crmcontracts.PublicEventEnvelope{
		EventId:       openapi_types.UUID(sampleIDs.event),
		Type:          eventType,
		Version:       crmcontracts.PublicEventVersions[eventType],
		OccurredAt:    time.Date(2026, 10, 14, 9, 12, 0, 0, time.UTC),
		Actor:         crmcontracts.PublicEventActor{Type: "human"},
		Entity:        entity,
		CorrelationId: openapi_types.UUID(sampleIDs.correlation),
		Data:          json.RawMessage(data),
	})
}
