// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// InternalMailPolicy records explicit source authorization and its future-only cutover.
type InternalMailPolicy struct {
	Enabled bool                 `json:"enabled"`
	Since   *time.Time           `json:"since"`
	Sources []InternalMailSource `json:"sources"`
}

// InternalMailSource binds an enrollment to the provider account's current generation.
// OwnerUserID is null for a shared mailbox: its connection and generation name it.
type InternalMailSource struct {
	HistorySince *time.Time `json:"history_since,omitempty"`
	ConnectionID ids.UUID   `json:"connection_id"`
	OwnerUserID  *ids.UUID  `json:"owner_user_id"`
	Provider     string     `json:"provider"`
	Generation   int        `json:"generation"`
	Containers   []string   `json:"containers"`
	EnrolledAt   time.Time  `json:"enrolled_at"`
	EnrolledBy   string     `json:"enrolled_by"`
}

const (
	internalMailPolicyKey = "capture.internal_mail_policy"
	providerIMAP          = "imap"
)

var internalMailPolicy = settings.Define(internalMailPolicyKey, captureSettingsObject,
	captureAuditVerb, InternalMailPolicy{Sources: []InternalMailSource{}}, validateInternalMailPolicy).
	MachineryApplied().WithAuthorization(authorizeInternalMailPolicy).
	WithChangeCheck(checkInternalMailPolicyChange).WithChanged(emitInternalMailPolicyChanged)

type internalMailPolicyDefinition struct {
	*settings.Entry[InternalMailPolicy]
}

// AuditImage keeps private folder names out of the installation-wide audit reader.
func (internalMailPolicyDefinition) AuditImage(raw json.RawMessage) json.RawMessage {
	digest := sha256.Sum256(raw)
	return json.RawMessage(`"` + hex.EncodeToString(digest[:]) + `"`)
}

func authorizeInternalMailPolicy(ctx context.Context, action principal.Action) error {
	actor, ok := principal.Actor(ctx)
	if !ok || (actor.Type != principal.PrincipalHuman && actor.Type != principal.PrincipalAgent) ||
		(action == principal.ActionUpdate && actor.Type != principal.PrincipalHuman) {
		return apperrors.ErrPermissionDenied
	}
	return auth.RequireAdmin(ctx)
}

func validateInternalMailPolicy(policy InternalMailPolicy) error {
	if policy.Enabled && (policy.Since == nil || policy.Since.IsZero()) {
		return fmt.Errorf("enable internal mail through its source controls: %w", apperrors.ErrInvalidArgument)
	}
	if policy.Sources == nil || len(policy.Sources) > 200 {
		return fmt.Errorf("select at most 200 sources: %w", apperrors.ErrInvalidArgument)
	}
	seen := make(map[ids.UUID]bool, len(policy.Sources))
	for _, source := range policy.Sources {
		if !validInternalMailSourceMetadata(source) || seen[source.ConnectionID] {
			return fmt.Errorf("select each authorized connection once: %w", apperrors.ErrInvalidArgument)
		}
		seen[source.ConnectionID] = true
		if !slices.Contains([]string{providerGmail, providerGraph, providerIMAP}, source.Provider) {
			return fmt.Errorf("select a mail connection: %w", apperrors.ErrInvalidArgument)
		}
		if err := validateInternalMailContainers(source.Provider, source.Containers); err != nil {
			return err
		}
		if source.OwnerUserID == nil && source.HistorySince != nil {
			return fmt.Errorf("a shared mailbox has no owner to start a history import, so enrol it for future mail only: %w", apperrors.ErrInvalidArgument)
		}
	}
	return nil
}

func validateInternalMailContainers(provider string, containers []string) error {
	if len(containers) > 200 {
		return fmt.Errorf("select at most 200 folders: %w", apperrors.ErrInvalidArgument)
	}
	seen := make(map[string]bool, len(containers))
	for _, container := range containers {
		if !strings.HasPrefix(container, provider+":") || len(container) <= len(provider)+1 ||
			len(container) > 1024 || strings.ContainsAny(container, "\r\n\x00") || seen[container] {
			return fmt.Errorf("select distinct folders from this provider: %w", apperrors.ErrInvalidArgument)
		}
		seen[container] = true
	}
	return nil
}

func checkInternalMailPolicyChange(ctx context.Context, tx pgx.Tx, before, next InternalMailPolicy) error {
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if next.Enabled && !before.Enabled {
		if next.Since == nil || !next.Since.Equal(now) {
			return apperrors.ErrInvalidArgument
		}
	} else if !sameOptionalInstant(before.Since, next.Since) && (next.Enabled || next.Since != nil || len(next.Sources) != 0) {
		return apperrors.ErrInvalidArgument
	}
	prior := make(map[ids.UUID]InternalMailSource, len(before.Sources))
	for _, source := range before.Sources {
		prior[source.ConnectionID] = source
	}
	ordered := slices.Clone(next.Sources)
	slices.SortFunc(ordered, func(a, b InternalMailSource) int {
		return strings.Compare(a.ConnectionID.String(), b.ConnectionID.String())
	})
	for _, source := range ordered {
		if err := checkInternalMailSource(ctx, tx, source, prior[source.ConnectionID], now); err != nil {
			return err
		}
	}
	return nil
}

func sameOptionalInstant(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func checkInternalMailSource(ctx context.Context, tx pgx.Tx, next, before InternalMailSource, now time.Time) error {
	if sameInternalMailScope(next, before) {
		if !next.EnrolledAt.Equal(before.EnrolledAt) || next.EnrolledBy != before.EnrolledBy {
			return apperrors.ErrInvalidArgument
		}
		return nil
	}
	actual, err := readInternalMailSource(ctx, tx, next.ConnectionID)
	if err != nil {
		return err
	}
	if !sameInternalMailOwner(actual.OwnerUserID, next.OwnerUserID) || actual.Provider != next.Provider || actual.Generation != next.Generation {
		return apperrors.ErrConflict
	}
	stamp, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	if next.HistorySince != nil {
		windows := BackfillWindowMonths()
		if next.HistorySince.After(now) || next.HistorySince.Before(now.AddDate(0, -windows[len(windows)-1], 0)) || !next.HistorySince.Equal(storekit.AsDate(*next.HistorySince)) {
			return apperrors.ErrInvalidArgument
		}
	}
	if !next.EnrolledAt.Equal(now) || next.EnrolledBy != stamp {
		return apperrors.ErrInvalidArgument
	}

	return nil
}

// readInternalMailSource reads a live mail connection: a live human's own, or
// a shared mailbox whose reader team is live.
func readInternalMailSource(ctx context.Context, tx pgx.Tx, id ids.UUID) (InternalMailSource, error) {
	source := InternalMailSource{ConnectionID: id}
	err := tx.QueryRow(ctx, `SELECT c.user_id, c.provider, c.generation
 FROM capture_connection c
 WHERE c.id = $1 AND c.provider IN ('gmail','graph','imap')
 AND c.status IN ('connected','error') AND c.archived_at IS NULL
 AND (c.user_id IS NOT NULL OR `+sharedMailboxLiveSQL+`) FOR SHARE OF c`, id).
		Scan(&source.OwnerUserID, &source.Provider, &source.Generation)
	if err == pgx.ErrNoRows {
		return InternalMailSource{}, apperrors.ErrNotFound
	}
	if err != nil || source.OwnerUserID == nil {
		return source, err
	}
	var live bool
	err = tx.QueryRow(ctx, `SELECT true FROM app_user u WHERE u.id = $1 AND `+workforce.LiveHumanSQL("u")+` FOR SHARE`, *source.OwnerUserID).Scan(&live)
	if err == pgx.ErrNoRows {
		return InternalMailSource{}, apperrors.ErrNotFound
	}
	return source, err
}

func emitInternalMailPolicyChanged(ctx context.Context, tx pgx.Tx, auditID ids.UUID) error {
	return storekit.EmitEvent(ctx, tx, auditID, storekit.MustWorkspace(ctx), crmcontracts.InternalEventCaptureSettingsUpdated{ChangedKeys: []string{internalMailPolicyKey}})
}

func validInternalMailSourceMetadata(source InternalMailSource) bool {
	return source.ConnectionID != ids.Nil && (source.OwnerUserID == nil || *source.OwnerUserID != ids.Nil) &&
		!source.EnrolledAt.IsZero() && source.EnrolledBy != "" && source.Generation >= 0 && source.Containers != nil
}

func sameInternalMailScope(a, b InternalMailSource) bool {
	return a.ConnectionID == b.ConnectionID && sameInternalMailOwner(a.OwnerUserID, b.OwnerUserID) && a.Provider == b.Provider &&
		a.Generation == b.Generation && slices.Equal(a.Containers, b.Containers) && sameOptionalInstant(a.HistorySince, b.HistorySince)
}

func internalMailSourceCurrentTx(ctx context.Context, tx pgx.Tx, enrolled InternalMailSource) (bool, error) {
	actual, err := readInternalMailSource(ctx, tx, enrolled.ConnectionID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return sameInternalMailOwner(actual.OwnerUserID, enrolled.OwnerUserID) && actual.Provider == enrolled.Provider && actual.Generation == enrolled.Generation, nil
}

// sameInternalMailOwner compares owners by value; two shared sources have none.
func sameInternalMailOwner(a, b *ids.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
