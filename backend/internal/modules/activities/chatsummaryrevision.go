// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// ChatSummaryFingerprint detects edits before generated recap text is replaced.
func ChatSummaryFingerprint(subject, body string) string {
	digest := sha256.Sum256([]byte(subject + "\x00" + body))
	return hex.EncodeToString(digest[:])
}

// ChatSummaryRevision carries source state needed to preserve human recap edits.
type ChatSummaryRevision struct {
	PreserveBody bool
	Subject      string
	Body         string
	Metadata     map[string]any
	Deleted      bool
}

// ReviseChatSummaryTx updates source-derived text while preserving human recap edits.
func (s *Store) ReviseChatSummaryTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, in ChatSummaryRevision) (crmcontracts.Activity, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return crmcontracts.Activity{}, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return crmcontracts.Activity{}, err
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", id.UUID, storekit.IncludeArchived); err != nil {
		return crmcontracts.Activity{}, err
	}
	if err := auth.EnsureActivityWritableIn(ctx, tx, id.UUID, false); err != nil {
		return crmcontracts.Activity{}, err
	}
	current, err := s.GetActivityTx(ctx, tx, id, storekit.IncludeArchived, CustomColumns{})
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if !generatedChatSummary(current) {
		return crmcontracts.Activity{}, apperrors.ErrConflict
	}
	if current.ArchivedAt != nil {
		return s.reviseArchivedChatSummaryTx(ctx, tx, id, current, in)
	}
	before, metadata, err := s.reviseLiveChatSummaryText(ctx, tx, id, current, in)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE activity SET raw=@raw WHERE id=@id`, pgx.StrictNamedArgs{"raw": json.RawMessage(encoded), "id": id}); err != nil {
		return crmcontracts.Activity{}, err
	}
	audit, err := storekit.Audit(ctx, tx, "update", "activity", id.UUID, chatSummaryAuditImage(nil, before), chatSummaryAuditImage(nil, metadata))
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if err := storekit.EmitEvent(ctx, tx, audit, id.UUID, crmcontracts.PublicEventActivityUpdated{}); err != nil {
		return crmcontracts.Activity{}, err
	}
	columns, err := s.activityColumnsIn(ctx, tx)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	out, err := readActivityAt(ctx, tx, id, storekit.LiveOnly, s.now())
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	return out, withCustomValuesOf(ctx, tx, columns.cols, &out)
}

func (s *Store) reviseArchivedChatSummaryTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, current crmcontracts.Activity, in ChatSummaryRevision) (crmcontracts.Activity, error) {
	if !in.Deleted {
		return current, nil
	}
	var before map[string]any
	if err := tx.QueryRow(ctx, `SELECT raw FROM activity WHERE id=@id`, pgx.StrictNamedArgs{"id": id}).Scan(&before); err != nil {
		return crmcontracts.Activity{}, err
	}
	priorText, err := readSystemTextForMutation(ctx, tx, id)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	patch := storekit.NewPatch()
	patch.Set("subject", current.Subject, in.Subject)
	patch.Set("body", current.Body, in.Body)
	patch.Set("raw", before, in.Metadata)
	patch.Set("system_text", priorText, nil)
	if patch.Empty() {
		return current, nil
	}
	if err := patch.ApplyGuardedIn(ctx, tx, "activity", id.UUID, current.Version, storekit.IncludeArchived); err != nil {
		return crmcontracts.Activity{}, err
	}
	audit, err := storekit.Audit(ctx, tx, "update", "activity", id.UUID, chatSummaryAuditImage(patch.Before(), before), chatSummaryAuditImage(patch.After(), in.Metadata))
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if err := storekit.EmitEvent(ctx, tx, audit, id.UUID, crmcontracts.PublicEventActivityUpdated{}); err != nil {
		return crmcontracts.Activity{}, err
	}
	return s.GetActivityTx(ctx, tx, id, storekit.IncludeArchived, CustomColumns{})
}

// History needs source state, while provider bindings stay in canonical metadata.
func chatSummaryAuditImage(image, metadata map[string]any) map[string]any {
	history := make(map[string]any, len(image)+2)
	for field, value := range image {
		if field != "raw" {
			history[field] = value
		}
	}
	history["source_changed"] = metadata["source_changed"]
	history["source_deleted"] = metadata["source_deleted"]
	return history
}

// Arrival-channel normalization keeps the reserved recap namespace intact.
func generatedChatSummary(current crmcontracts.Activity) bool {
	return current.Kind == crmcontracts.ActivityKindNote && current.SourceSystem != nil && *current.SourceSystem == provenance.ChatSummarySource
}

func (s *Store) reviseLiveChatSummaryText(ctx context.Context, tx pgx.Tx, id ids.ActivityID, current crmcontracts.Activity, in ChatSummaryRevision) (map[string]any, map[string]any, error) {
	var before map[string]any
	if err := tx.QueryRow(ctx, `SELECT raw FROM activity WHERE id=@id`, pgx.StrictNamedArgs{"id": id}).Scan(&before); err != nil {
		return nil, nil, err
	}
	prior, ok := before["generated_fingerprint"].(string)
	preserved := !in.Deleted && (in.PreserveBody || !ok || prior != ChatSummaryFingerprint(deref(current.Subject), deref(current.Body)))
	metadata := in.Metadata
	if preserved {
		metadata = make(map[string]any, len(before)+1)
		maps.Copy(metadata, before)
		metadata["source_changed"] = true
	} else {
		_, err := updateActivityInTx(ctx, tx, id, UpdateActivityInput{Subject: &in.Subject, Body: &in.Body})
		if err != nil {
			return nil, nil, err
		}
		metadata["generated_fingerprint"] = ChatSummaryFingerprint(in.Subject, in.Body)
	}
	return before, metadata, nil
}
