// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The cg:scheduled-send-stop consumer: a stop holds the contact's queued
// marketing at once, instead of when each message comes due.
//
// It lives here because the question crosses two modules: consent owns the
// stop and its event, activities owns the queued message, and neither imports
// the other.
//
// Idempotency is the hold's own predicate: only a row still `scheduled` is
// held, so a redelivered stop holds nothing twice. events.Dedupe sits in front
// of that as a cache, never as the guarantee. The fire path still asks the
// engine again when a message comes due, so a missed event is caught there.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// scheduledSendStopActor names this consumer in the audit rows it causes.
const scheduledSendStopActor = "system:scheduled-send-stop"

// consentWithdrawn is the consent.changed state a stop is recorded as.
const consentWithdrawn = "withdrawn"

// ScheduledSendStop holds a stopped contact's queued marketing.
type ScheduledSendStop struct {
	store    *activities.Store
	identity *identity.Service
	log      *slog.Logger
}

// NewScheduledSendStop builds the consumer over the installation's handle.
func NewScheduledSendStop(pool *pgxpool.Pool, ident *identity.Service, log *slog.Logger) *ScheduledSendStop {
	return &ScheduledSendStop{store: activities.NewStore(InstallationDB(pool)), identity: ident, log: log}
}

// HandleEvent routes one envelope. Anything that is not a stop on a contact
// answers nil, so the consumer group keeps flowing.
func (s *ScheduledSendStop) HandleEvent(ctx context.Context, env events.Envelope) error {
	purpose, stop, err := stopOf(env)
	if err != nil || !stop {
		return err
	}
	ws, err := s.identity.InstallationWorkspace(ctx)
	if err != nil {
		return err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithCorrelationID(ctx, env.EventID)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: scheduledSendStopActor,
	})
	held, err := s.store.HoldMarketingToContact(ctx, ids.From[ids.ContactKind](env.Entity.ID), purpose)
	if err != nil {
		return fmt.Errorf("scheduled send stop: %w", err)
	}
	if held > 0 {
		s.log.InfoContext(ctx, "held queued marketing to a contact who stopped it", "contact_id", env.Entity.ID, "held", held)
	}
	return nil
}

// stopOf reads whether an envelope stops marketing to a contact, and for which
// purpose. A suppression binds every marketing send, so it names none. A
// withdrawal names the purpose it withdrew. A lead's consent.changed shares the
// event type and is not this consumer's: a scheduled send addresses contacts.
func stopOf(env events.Envelope) (purpose string, stop bool, err error) {
	if env.Entity.Type != "contact" || env.Entity.ID == ids.Nil {
		return "", false, nil
	}
	switch env.Type {
	case string(crmcontracts.ConsentSuppressed):
		return "", true, nil
	case string(crmcontracts.ConsentChanged):
		var changed crmcontracts.PublicEventConsentChanged
		if err := json.Unmarshal(env.Payload, &changed); err != nil {
			return "", false, fmt.Errorf("scheduled send stop: reading consent.changed: %w", err)
		}
		return changed.Purpose, changed.NewState == consentWithdrawn, nil
	default:
		return "", false, nil
	}
}
