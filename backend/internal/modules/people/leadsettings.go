// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// How the installation handles leads (ADR-0090/A135 settings mechanism):
// the first-response target is opt-in. Off, no lead carries an SLA field,
// the sla_state filter matches nothing, the queue orders by score alone and
// the breach scan records nothing. On, the target is the installation's
// own number rather than a compile-time constant.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DefaultFirstResponseTarget is the target an installation that turns the
// SLA on without choosing a number gets (formulas §18, LEADSLA default).
const DefaultFirstResponseTarget = 240 * time.Minute

const (
	firstResponseMinMinutes = 15
	firstResponseMaxMinutes = 7 * 24 * 60
)

// FirstResponseEnabled is whether the first-response target is tracked at
// all. Off by default: a fresh installation should not open on a list where
// every lead reads "overdue".
var FirstResponseEnabled = settings.Define[bool](
	"people.first_response_enabled", leadVocabularyObject, "update", false, nil,
).MachineryApplied() // every SLA computation applies it to a lead read already gated by lead:read

// FirstResponseTargetMinutes is how long a lead may wait for its first
// genuine response once its clock starts.
var FirstResponseTargetMinutes = settings.Define[int](
	"people.first_response_target_minutes", leadVocabularyObject, "update",
	int(DefaultFirstResponseTarget/time.Minute),
	func(minutes int) error {
		if minutes < firstResponseMinMinutes || minutes > firstResponseMaxMinutes {
			return fmt.Errorf("the target is %d..%d minutes", firstResponseMinMinutes, firstResponseMaxMinutes)
		}
		return nil
	},
).MachineryApplied() // the band it produces rides lead rows the caller is separately gated for

// UnassignedEscalationUserID is who answers for a lead nobody owns.
//
// A breach on an OWNED lead escalates to its owner, which is the desk that owes
// the answer. A breach on an unowned one had nobody: the task was written
// assigned to no one and no notice went out at all, so the queue's worst case —
// a lead nobody picked up, now past its response target — was the one case that
// reached no human.
//
// A SETTING rather than a derived answer, because "who runs the intake queue"
// is an operator's decision and no column in this installation holds it. The
// manager role says who may work a team's records; it does not say which of
// them answers for the leads nobody has taken. Empty is the honest default: an
// installation that has not said leaves the breach where it was, and the queue
// view is what surfaces it.
var UnassignedEscalationUserID = settings.Define[string](
	"people.unassigned_escalation_user_id", leadVocabularyObject, "update", "",
	func(raw string) error {
		if raw == "" {
			return nil
		}
		if _, err := ids.Parse(raw); err != nil {
			return fmt.Errorf("the escalation seat is a user id, or empty for nobody")
		}
		return nil
	},
).MachineryApplied() // the SLA sweep applies it to decide who a breach is addressed to

// Definitions is people's contribution to the settings registry; compose
// concatenates each module's list.
func Definitions() []settings.Definition {
	return []settings.Definition{
		FirstResponseEnabled, FirstResponseTargetMinutes, UnassignedEscalationUserID,
	}
}

// leadSLAPolicy is the resolved setting pair every SLA computation reads.
type leadSLAPolicy struct {
	enabled bool
	target  time.Duration
}

// atRisk is the tail of the target inside which an unanswered lead reads as
// at_risk rather than within_target: the last quarter.
func (p leadSLAPolicy) atRisk() time.Duration { return p.target / 4 }

func (p leadSLAPolicy) targetMinutes() int { return int(p.target / time.Minute) }

// loadLeadSLAPolicy reads the two settings inside the caller's transaction,
// through settings.ApplyTx and so without the settings object gate: the policy
// is an input to a lead read already gated by lead:read, and a connector or
// agent principal reading leads must not need a settings grant to see them.
//
// ApplyTx and not a raw statement, which is what this was. Both are ungated;
// only one is CHECKED. ApplyTx refuses any entry not declared MachineryApplied
// at Define time, so what used to be a sentence a reviewer agreed with is now a
// refusal the store makes — and the next ungated read of some other setting
// fails at the first test that exercises it rather than needing to be noticed.
//
// Two calls where there was one statement, which is two round trips instead of
// one inside a transaction the caller already holds. That is the price, and it
// is small against the alternative: a batched read cannot ask the registry
// whether either key was admitted to it.
func loadLeadSLAPolicy(ctx context.Context, tx pgx.Tx) (leadSLAPolicy, error) {
	policy := leadSLAPolicy{target: DefaultFirstResponseTarget}
	enabled, err := settings.ApplyTx(ctx, tx, FirstResponseEnabled)
	if err != nil {
		return policy, fmt.Errorf("load lead sla policy: %w", err)
	}
	minutes, err := settings.ApplyTx(ctx, tx, FirstResponseTargetMinutes)
	if err != nil {
		return policy, fmt.Errorf("load lead sla policy: %w", err)
	}
	policy.enabled = enabled
	policy.target = time.Duration(minutes) * time.Minute
	return policy, nil
}

// slaPolicy resolves the policy for a store operation that has not opened
// its transaction yet.
func (s *Store) slaPolicy(ctx context.Context) (leadSLAPolicy, error) {
	var policy leadSLAPolicy
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		policy, err = loadLeadSLAPolicy(ctx, tx)
		return err
	})
	return policy, err
}

// FirstResponseTracked reports whether this installation measures a
// first-response target at all.
//
// The Worklist's lead lane asks before it claims anything: with the target
// switched off no lead is LATE, and a queue that reported none overdue would be
// stating a fact nothing measures. Exported for that one caller, which cannot
// reach the unexported policy read beside it.
func (s *Store) FirstResponseTracked(ctx context.Context) (bool, error) {
	if err := auth.Require(ctx, leadEntity, principal.ActionRead); err != nil {
		return false, err
	}
	policy, err := s.slaPolicy(ctx)
	if err != nil {
		return false, err
	}
	return policy.enabled, nil
}

// WithSettings wires the installation settings store the lead-settings
// endpoints write through; compose injects it.
func (s *Store) WithSettings(store *settings.Store) *Store {
	s.settings = store
	return s
}

// GetLeadSettings answers the installation's lead handling.
func (s *Store) GetLeadSettings(ctx context.Context) (crmcontracts.LeadSettings, error) {
	if err := auth.Require(ctx, leadVocabularyObject, principal.ActionRead); err != nil {
		return crmcontracts.LeadSettings{}, err
	}
	policy, err := s.slaPolicy(ctx)
	if err != nil {
		return crmcontracts.LeadSettings{}, err
	}
	out := crmcontracts.LeadSettings{
		FirstResponseEnabled: policy.enabled, FirstResponseTargetMinutes: policy.targetMinutes(),
	}
	// Read through the same resolver the breach emitter uses, so the screen
	// cannot show a seat the escalation would refuse to address: a setting
	// naming somebody who has since been suspended reads null here for the
	// same reason it escalates to nobody there.
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		target, configured, err := unassignedEscalationTarget(ctx, tx)
		if err != nil {
			return err
		}
		if configured {
			named := openapi_types.UUID(target)
			out.UnassignedEscalationUserId = &named
		}
		return nil
	}); err != nil {
		return crmcontracts.LeadSettings{}, err
	}
	return out, nil
}

// UpdateLeadSettingsInput is a sparse patch; nil leaves the setting alone.
type UpdateLeadSettingsInput struct {
	FirstResponseEnabled       *bool
	FirstResponseTargetMinutes *int
	// UnassignedEscalationUserID names the seat that answers for the queue.
	// ClearUnassignedEscalation is the explicit "nobody": a JSON null decodes
	// to a nil pointer and reads as "not supplied", so the two are carried
	// apart the way the lead patch carries its own clears.
	UnassignedEscalationUserID *ids.UserID
	ClearUnassignedEscalation  bool
}

// UpdateLeadSettings changes the lead handling (admin/ops). Each setting's
// own validator and audit verb apply through the settings store.
func (s *Store) UpdateLeadSettings(ctx context.Context, in UpdateLeadSettingsInput) (crmcontracts.LeadSettings, error) {
	if err := auth.Require(ctx, leadVocabularyObject, principal.ActionUpdate); err != nil {
		return crmcontracts.LeadSettings{}, err
	}
	if s.settings == nil {
		return crmcontracts.LeadSettings{}, fmt.Errorf("people: lead settings are not wired; the installation cannot change them")
	}
	// Both land in ONE transaction: a target without its switch, or the
	// reverse, is not a state the list should ever render.
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if in.FirstResponseEnabled != nil {
			if err := setLeadSetting(ctx, tx, s.settings, FirstResponseEnabled.Key(), *in.FirstResponseEnabled); err != nil {
				return err
			}
		}
		if in.FirstResponseTargetMinutes != nil {
			if err := setLeadSetting(ctx, tx, s.settings, FirstResponseTargetMinutes.Key(),
				*in.FirstResponseTargetMinutes); err != nil {
				return err
			}
		}
		if in.ClearUnassignedEscalation {
			return setLeadSetting(ctx, tx, s.settings, UnassignedEscalationUserID.Key(), "")
		}
		if in.UnassignedEscalationUserID != nil {
			if err := s.ensureEscalationSeatReadsLeads(ctx, tx, in.UnassignedEscalationUserID.UUID); err != nil {
				return err
			}
			return setLeadSetting(ctx, tx, s.settings, UnassignedEscalationUserID.Key(),
				in.UnassignedEscalationUserID.String())
		}
		return nil
	})
	if err != nil {
		return crmcontracts.LeadSettings{}, err
	}
	return s.GetLeadSettings(ctx)
}

// setLeadSetting encodes one value and writes it through the settings store's
// transactional seam, which validates, audits and refuses a frozen setting.
func setLeadSetting[T bool | int | string](ctx context.Context, tx pgx.Tx, store *settings.Store, key string, value T) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("people: encoding %s: %w", key, err)
	}
	return store.SetRawTx(ctx, tx, key, raw)
}

// EscalationSeatError refuses a seat that cannot answer for the queue: 422.
type EscalationSeatError struct{}

func (e *EscalationSeatError) Error() string {
	return "the escalation seat must be an active colleague who can read leads"
}

// FieldFault names the field the caller sent.
func (e *EscalationSeatError) FieldFault() (field, code, message string) {
	return "unassigned_escalation_user_id", "seat_cannot_answer", e.Error()
}

// ensureEscalationSeatReadsLeads refuses a seat that could not act on the
// escalation it would receive.
//
// TWO questions, and the second is the one an assignment check does not ask.
// auth.EnsureAssignee answers whether a seat can be handed work at all —
// active, not an agent, not a read seat. It says nothing about LEADS, and an
// escalation carries the lead it is about: a colleague holding activity access
// and no lead grant would receive a task naming a record they cannot open,
// which is both useless to them and a disclosure nobody authorised.
func (s *Store) ensureEscalationSeatReadsLeads(ctx context.Context, tx pgx.Tx, seat ids.UUID) error {
	if err := auth.EnsureAssignee(ctx, tx, seat); err != nil {
		if errors.As(err, new(*auth.AssigneeNotAllowedError)) {
			return &EscalationSeatError{}
		}
		return err
	}
	if s.seatReadsLeads == nil {
		// Unwired: refuse rather than admit. A composition that cannot answer
		// "may they read leads" has not established the seat is safe to
		// nominate, and admitting on a missing answer is how a check becomes
		// decoration.
		return &EscalationSeatError{}
	}
	reads, err := s.seatReadsLeads(ctx, tx, seat)
	if err != nil {
		return err
	}
	if !reads {
		return &EscalationSeatError{}
	}
	return nil
}
