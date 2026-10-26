// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// How the installation handles leads (ADR-0090/A135 settings mechanism):
// the first-response target is opt-in. Off, no lead carries an SLA field,
// the sla_state filter matches nothing, the queue orders by score alone and
// the breach scan records nothing. On, the target is the installation's
// own number rather than a compile-time constant.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
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
var FirstResponseEnabled = config.DefineStored(config.Spec[bool]{
	Key:     "contacts.first_response_enabled",
	Env:     "MARGINCE_CONTACTS_FIRST_RESPONSE_ENABLED",
	Type:    config.Bool,
	Default: config.Some(false),
	Doc:     "Track a first-response target on every lead.",
}, config.Store{
	RBACObject: leadVocabularyObject, AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // every SLA computation applies it to a lead read already gated by lead:read
})

// FirstResponseTargetMinutes is how long a lead may wait for its first
// genuine response once its clock starts.
var FirstResponseTargetMinutes = config.DefineStored(config.Spec[int]{
	Key:     "contacts.first_response_target_minutes",
	Env:     "MARGINCE_CONTACTS_FIRST_RESPONSE_TARGET_MINUTES",
	Type:    config.Int,
	Default: config.Some(int(DefaultFirstResponseTarget / time.Minute)),
	Min:     config.Some(firstResponseMinMinutes),
	Max:     config.Some(firstResponseMaxMinutes),
	Doc:     "Minutes a lead may wait for its first real response once its clock starts.",
}, config.Store{
	RBACObject: leadVocabularyObject, AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the band it produces rides lead rows the caller is separately gated for
})

// UnassignedEscalationUserID is who answers for a lead nobody owns, or whose
// owner has left.
//
// A breach goes to the lead's owner while that owner can be handed work. Else
// it goes to this seat when one is set and eligible, and to nobody when not.
//
// A SETTING, because "who runs the intake queue" is an operator's decision and
// no column holds it. Empty leaves the breach unaddressed; the queue view shows it.
var UnassignedEscalationUserID = config.DefineStored(config.Spec[string]{
	Key:  "contacts.unassigned_escalation_user_id",
	Env:  "MARGINCE_CONTACTS_UNASSIGNED_ESCALATION_USER_ID",
	Type: config.String,
	Validate: func(raw string) error {
		if raw == "" {
			return nil
		}
		if _, err := ids.Parse(raw); err != nil {
			return errors.New("the escalation seat is a user id, or empty for nobody")
		}
		return nil
	},
	Doc: "The user id of the colleague who answers for a lead nobody owns, or whose owner has left, once it misses its target; empty means nobody, " +
		"and a boot value that names no active colleague reaches nobody and logs a warning.",
}, config.Store{
	RBACObject: leadVocabularyObject, AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the SLA sweep applies it to decide who a breach is addressed to
})

// settingsAuditVerb is the audit action every contacts settings change records.
const settingsAuditVerb = "update"

// ConfigSettings are this module's settings in the configuration registry.
func ConfigSettings() []config.Any { return append(LeadSettingsSurface(), SignatureTemplate) }

// LeadSettingsSurface are the settings GET /leads/settings carries.
func LeadSettingsSurface() []config.Any {
	return []config.Any{FirstResponseEnabled, FirstResponseTargetMinutes, UnassignedEscalationUserID}
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

// loadLeadSLAPolicy reads the two settings ungated: the policy feeds a lead read
// already gated by lead:read. One paired read is one snapshot under
// `READ COMMITTED`, so a write of both cannot land between two reads.
func loadLeadSLAPolicy(ctx context.Context, tx pgx.Tx, cfg *config.Config) (leadSLAPolicy, error) {
	enabled, minutes, err := config.StoredApplyPairTx(ctx, tx, cfg, FirstResponseEnabled, FirstResponseTargetMinutes)
	if err != nil {
		return leadSLAPolicy{target: DefaultFirstResponseTarget}, fmt.Errorf("load lead sla policy: %w", err)
	}
	return leadSLAPolicy{enabled: enabled, target: time.Duration(minutes) * time.Minute}, nil
}

// slaPolicy resolves the policy for a store operation that has not opened
// its transaction yet.
func (s *Store) slaPolicy(ctx context.Context) (leadSLAPolicy, error) {
	var policy leadSLAPolicy
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		policy, err = loadLeadSLAPolicy(ctx, tx, s.bootConfig())
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

// bootConfig is the configuration this store reads boot values from: the
// injected one, which the lead-settings writes lock against, else the program's.
func (s *Store) bootConfig() *config.Config {
	if s.settings != nil {
		return s.settings
	}
	return config.Bound()
}

// WithSettings wires the configuration the lead-settings endpoints read and
// save through; compose injects it.
func (s *Store) WithSettings(cfg *config.Config) *Store {
	s.settings = cfg
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
		target, configured, err := unassignedEscalationTarget(ctx, tx, s.bootConfig())
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
// boot lock, validator and audit verb apply through the configuration.
func (s *Store) UpdateLeadSettings(ctx context.Context, in UpdateLeadSettingsInput) (crmcontracts.LeadSettings, error) {
	if err := auth.Require(ctx, leadVocabularyObject, principal.ActionUpdate); err != nil {
		return crmcontracts.LeadSettings{}, err
	}
	if s.settings == nil {
		return crmcontracts.LeadSettings{}, errors.New("contacts: lead settings are not wired; the installation cannot change them")
	}
	// Both land in ONE transaction: a target without its switch, or the
	// reverse, is not a state the list should ever render.
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if in.FirstResponseEnabled != nil {
			if err := config.SaveTx(ctx, s.settings, tx, FirstResponseEnabled, *in.FirstResponseEnabled); err != nil {
				return err
			}
		}
		if in.FirstResponseTargetMinutes != nil {
			if err := config.SaveTx(ctx, s.settings, tx, FirstResponseTargetMinutes, *in.FirstResponseTargetMinutes); err != nil {
				return err
			}
		}
		if in.ClearUnassignedEscalation {
			return config.SaveTx(ctx, s.settings, tx, UnassignedEscalationUserID, "")
		}
		if in.UnassignedEscalationUserID != nil {
			return s.saveEscalationSeatTx(ctx, tx, in.UnassignedEscalationUserID)
		}
		return nil
	})
	if err != nil {
		return crmcontracts.LeadSettings{}, err
	}
	return s.GetLeadSettings(ctx)
}

// saveEscalationSeatTx names the seat that answers for the queue. A boot-set
// seat refuses another before that seat is judged, and its own id is a no-op.
// Whether a field is locked does not depend on the value sent.
func (s *Store) saveEscalationSeatTx(ctx context.Context, tx pgx.Tx, seat *ids.UserID) error {
	named := seat.String()
	locked, err := config.LockedChange(s.settings, UnassignedEscalationUserID, func(boot string) bool { return boot == named })
	if locked || err != nil {
		return err
	}
	if err := s.ensureEscalationSeatReadsLeads(ctx, tx, seat.UUID); err != nil {
		return err
	}
	return config.SaveTx(ctx, s.settings, tx, UnassignedEscalationUserID, named)
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
