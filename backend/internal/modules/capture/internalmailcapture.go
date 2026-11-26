// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type internalMailAdmission struct {
	History *internalMailHistoryReceipt `json:"history,omitempty"`
	InternalMailSource
	PolicySince time.Time `json:"policy_since"`
}

type internalMailAdmissionKey struct{}

// InternalMailRecorder writes through the activity owner's transaction seam.
// It receives the mailbox connection and generation the run syncs.
type InternalMailRecorder func(ctx context.Context, tx pgx.Tx, id ids.ActivityID, authorization json.RawMessage, connection ids.UUID, generation int) error

// WithInternalMailRecorder keeps an unwired sink on its default internal-mail exclusion.
func (s *Sink) WithInternalMailRecorder(record InternalMailRecorder) *Sink {
	next := *s
	next.internalMailRecorder = record
	return &next
}

//nolint:nilnil // No admission preserves the default exclusion of internal mail.
func (s *Sink) internalMailAdmissionTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (*internalMailAdmission, error) {
	if !s.internalMailRecordEligible(ctx, rec) {
		return nil, nil
	}
	_, owner := capturePrincipal(ctx)
	if err := settings.ShareLockForDecision(ctx, tx, internalMailPolicyKey); err != nil {
		return nil, err
	}
	policy, err := settings.ApplyTx(ctx, tx, internalMailPolicy)
	if err != nil || !policy.Enabled || policy.Since == nil {
		return nil, err
	}
	provider := deliveringConnector(ctx)
	for _, source := range policy.Sources {
		if !internalMailSourceCoversRecord(ctx, source, provider, owner, rec, *policy.Since) {
			continue
		}
		current, err := internalMailSourceCurrentTx(ctx, tx, source)
		if err != nil || !current {
			return nil, err
		}
		if run := backfillRunOf(ctx); run != nil {
			return internalMailHistoryAdmissionTx(ctx, tx, *run, rec.ProviderReceivedAt, source, *policy.Since)
		}
		return &internalMailAdmission{InternalMailSource: source, PolicySince: *policy.Since}, nil
	}
	return nil, nil
}

func internalMailAdmitted(ctx context.Context) bool {
	admission, ok := ctx.Value(internalMailAdmissionKey{}).(*internalMailAdmission)
	return ok && admission != nil
}

func (s *Sink) recordInternalMailTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	admission, ok := ctx.Value(internalMailAdmissionKey{}).(*internalMailAdmission)
	if !ok || admission == nil {
		return nil
	}
	run, bound := ctx.Value(mailboxConnectionKey{}).(mailboxConnection)
	if !bound {
		return apperrors.ErrPermissionDenied
	}
	raw, err := json.Marshal(admission)
	if err != nil {
		return err
	}
	return s.internalMailRecorder(ctx, tx, id, raw, run.ID, run.Generation)
}

func (s *Sink) internalMailRecordEligible(ctx context.Context, rec connector.NormalizedRecord) bool {
	fields, ok := rec.Fields.(ActivityFields)
	actor, owner := capturePrincipal(ctx)
	_, shared := sharedMailboxOf(ctx)
	return ok && fields.Kind == kindEmail && fields.ChannelProvider == "" && isMailboxRun(ctx) &&
		actor.Type == principal.PrincipalConnector && (!owner.IsZero() || shared) && s.internalMailRecorder != nil && !rec.ProviderReceivedAt.IsZero()
}

func internalMailSourceCoversRecord(ctx context.Context, source InternalMailSource, provider string, owner ids.UUID, rec connector.NormalizedRecord, since time.Time) bool {
	if !internalMailSourceIsRun(ctx, source, owner) || source.Provider != provider {
		return false
	}
	if backfillRunOf(ctx) == nil && (rec.ProviderReceivedAt.Before(since) || rec.ProviderReceivedAt.Before(source.EnrolledAt)) {
		return false
	}
	return len(source.Containers) == 0 || slices.ContainsFunc(rec.Containers, func(container string) bool { return slices.Contains(source.Containers, container) })
}

// internalMailSourceIsRun matches a shared run by its connection and a
// personal run by its owner, so neither kind admits under the other.
func internalMailSourceIsRun(ctx context.Context, source InternalMailSource, owner ids.UUID) bool {
	if shared, ok := sharedMailboxOf(ctx); ok {
		return source.OwnerUserID == nil && source.ConnectionID == shared
	}
	return source.OwnerUserID != nil && *source.OwnerUserID == owner
}
