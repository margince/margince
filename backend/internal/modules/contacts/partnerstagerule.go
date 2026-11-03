// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What a partner write may put on the row: its kinds, and the stage it moves
// to. Every writer of a partner reaches these through UpsertPartner, so the
// rule that a stage meaning active needs an activated partner is held once.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

const (
	partnerFieldKinds = "partner_kinds"
	partnerFieldStage = "relationship_stage"
)

// legacyPartnerRoles are the kinds the deprecated partner_role can name.
var legacyPartnerRoles = []string{
	string(crmcontracts.PartnerKindHosting), string(crmcontracts.PartnerKindConsulting), string(crmcontracts.PartnerKindStrategic),
}

// partnerKindsOf settles the kinds a write stores: the list when sent, else the
// deprecated role as a list of one. The result is sorted and without repeats.
func partnerKindsOf(in UpsertPartnerInput) ([]string, error) {
	var kinds []string
	switch {
	case in.PartnerKinds != nil:
		kinds = slices.Clone(*in.PartnerKinds)
	case strings.TrimSpace(in.PartnerRole) != "":
		kinds = []string{strings.TrimSpace(in.PartnerRole)}
	}
	if len(kinds) == 0 {
		return nil, httperr.Validation(partnerFieldKinds, "partner_kinds_required",
			"partner_kinds needs at least one of hosting, consulting, strategic or referral")
	}
	for _, kind := range kinds {
		if !crmcontracts.PartnerKind(kind).Valid() {
			return nil, httperr.Validation(partnerFieldKinds, codeInvalidValue,
				"partner_kinds holds only hosting, consulting, strategic or referral; implementation work is not a partner kind")
		}
	}
	slices.Sort(kinds)
	return slices.Compact(kinds), nil
}

// legacyPartnerRole is the deprecated single role a sorted list of kinds reads
// as. It is the first kind the old role could name, or nil for referral only.
func legacyPartnerRole(kinds []string) *string {
	for _, kind := range kinds {
		if slices.Contains(legacyPartnerRoles, kind) {
			return &kind
		}
	}
	return nil
}

// UnknownPartnerStageError refuses a key the stage list does not hold.
type UnknownPartnerStageError struct{ Key string }

func (e *UnknownPartnerStageError) Error() string {
	return "relationship_stage " + e.Key + " is not in the partner stage list; pick one from Settings › Partners"
}

// FieldFault names the stage as the invalid input.
func (e *UnknownPartnerStageError) FieldFault() (field, code, message string) {
	return partnerFieldStage, "unknown_partner_stage", e.Error()
}

// InactivePartnerStageError refuses a retired stage for a move. A partner
// already on it keeps it.
type InactivePartnerStageError struct{ Key string }

func (e *InactivePartnerStageError) Error() string {
	return "relationship_stage " + e.Key + " is retired; pick an active stage or reactivate it in Settings › Partners"
}

// FieldFault names the stage as the invalid input.
func (e *InactivePartnerStageError) FieldFault() (field, code, message string) {
	return partnerFieldStage, "inactive_partner_stage", e.Error()
}

// PartnerNotActivatedError refuses a stage meaning active for a partner that
// was never activated.
type PartnerNotActivatedError struct{ Key string }

func (e *PartnerNotActivatedError) Error() string {
	return "relationship_stage " + e.Key + " is an active stage, and this partner is not activated yet. " +
		"A partner is activated when someone accepts its first partner lead, or when a deal it sourced is won"
}

// FieldFault names the stage as the invalid input.
func (e *PartnerNotActivatedError) FieldFault() (field, code, message string) {
	return partnerFieldStage, "partner_not_activated", e.Error()
}

// partnerStageWrite is the stage an upsert lands. set is nil when the stage
// stays; insert is what a new row starts on, since the column has no default.
type partnerStageWrite struct {
	set    *string
	insert string
}

// resolvePartnerStage decides the stage of one upsert under the company lock.
// A new programme with no stage starts on the first active candidate stage. A
// move is refused onto an unknown or retired stage, and onto a stage meaning
// active while the partner is not activated.
func resolvePartnerStage(ctx context.Context, tx pgx.Tx, in UpsertPartnerInput) (partnerStageWrite, error) {
	var standing string
	var activated bool
	err := tx.QueryRow(ctx,
		`SELECT relationship_stage, activation_basis IS NOT NULL FROM partner WHERE company_id = $1`,
		in.CompanyID).Scan(&standing, &activated)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return partnerStageWrite{}, fmt.Errorf("read the partner's stage: %w", err)
	}
	var current *string
	if err == nil && !in.MustCreate {
		current = &standing
	}
	if in.RelationshipStage == nil {
		if current != nil {
			return partnerStageWrite{insert: *current}, nil
		}
		first, err := defaultPartnerStageTx(ctx, tx)
		return partnerStageWrite{set: &first, insert: first}, err
	}
	key := strings.TrimSpace(*in.RelationshipStage)
	if current != nil && *current == key {
		return partnerStageWrite{set: &key, insert: key}, nil
	}
	meaning, err := ensureAssignablePartnerStage(ctx, tx, key)
	if err != nil {
		return partnerStageWrite{}, err
	}
	if err := ensureStageAdmissible(key, meaning, activated); err != nil {
		return partnerStageWrite{}, err
	}
	return partnerStageWrite{set: &key, insert: key}, nil
}

// ensureAssignablePartnerStage refuses an unknown or retired key and answers
// the stage's meaning. The row is locked, so a retirement or meaning change
// committing between this check and the partner write cannot slip through.
func ensureAssignablePartnerStage(ctx context.Context, tx pgx.Tx, key string) (crmcontracts.PartnerStageMeaning, error) {
	var active bool
	var meaning string
	err := tx.QueryRow(ctx, `SELECT active, meaning FROM partner_stage WHERE key = $1 FOR SHARE`, key).Scan(&active, &meaning)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", &UnknownPartnerStageError{Key: key}
	}
	if err != nil {
		return "", fmt.Errorf("check partner stage %q: %w", key, err)
	}
	if !active {
		return "", &InactivePartnerStageError{Key: key}
	}
	return crmcontracts.PartnerStageMeaning(meaning), nil
}

// ensureStageAdmissible is the activation rule: a partner stands on a stage
// meaning active only once it was activated.
func ensureStageAdmissible(key string, meaning crmcontracts.PartnerStageMeaning, activated bool) error {
	if meaning == crmcontracts.PartnerStageMeaningActive && !activated {
		return &PartnerNotActivatedError{Key: key}
	}
	return nil
}

// defaultPartnerStageTx is the stage a new programme starts on: the first
// active candidate stage in ladder order. The catalog refuses to retire the
// last one, so a missing row is a broken list, not a caller's mistake.
func defaultPartnerStageTx(ctx context.Context, tx pgx.Tx) (string, error) {
	var key string
	err := tx.QueryRow(ctx, `
		SELECT key FROM partner_stage
		 WHERE active AND meaning = 'candidate'
		 ORDER BY sort_order, key LIMIT 1 FOR SHARE`).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("the partner stage list has no active candidate stage: %w", apperrors.ErrConflict)
	}
	if err != nil {
		return "", fmt.Errorf("read the first partner stage: %w", err)
	}
	return key, nil
}

// partnerStageFacts is what a partner read shows about its stage.
type partnerStageFacts struct {
	label    string
	labelKey *string
	meaning  crmcontracts.PartnerStageMeaning
}

// loadPartnerStageFacts reads the whole ladder once per page of partners. The
// list is a handful of rows, so one read beats a join on every partner query.
func loadPartnerStageFacts(ctx context.Context, tx pgx.Tx) (map[string]partnerStageFacts, error) {
	rows, err := tx.Query(ctx, `SELECT key, label, label_key, meaning FROM partner_stage`)
	if err != nil {
		return nil, fmt.Errorf("read partner stages: %w", err)
	}
	defer rows.Close()
	out := map[string]partnerStageFacts{}
	for rows.Next() {
		var key, meaning string
		var facts partnerStageFacts
		if err := rows.Scan(&key, &facts.label, &facts.labelKey, &meaning); err != nil {
			return nil, err
		}
		facts.meaning = crmcontracts.PartnerStageMeaning(meaning)
		out[key] = facts
	}
	return out, rows.Err()
}
