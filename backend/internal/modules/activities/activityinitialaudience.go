// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Initial audience and members commit before the creation audit and event.
// Selected membership is explicit; capture's audience_reason vocabulary does
// not describe this new task's audience. Its source link records the derivation.
func recordInitialActivity(ctx context.Context, tx pgx.Tx, id ids.ActivityID, in LogActivityInput) error {
	after := map[string]any{fieldKind: in.Kind, fieldSubject: in.Subject}
	if in.SystemText != nil {
		metadata := systemtext.ForWrite(ctx, in.SystemText, deref(in.Subject), deref(in.Body))
		after["system_text"] = metadata
	}
	maps.Copy(after, in.CustomFields)
	if len(in.audienceMembers) > 0 {
		args := []any{id}
		if result, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE activity SET audience = 'selected' WHERE id = $%d AND archived_at IS NULL AND restricted_at IS NULL`, len(args)), args...); err != nil {
			return err
		} else if result.RowsAffected() != 1 {
			return errors.New("activities: initial audience row unavailable")
		}
		if err := replaceAudienceMembers(ctx, tx, id, in.audienceMembers); err != nil {
			return err
		}
		audience, err := readAudienceImage(ctx, tx, id)
		if err != nil {
			return err
		}
		maps.Copy(after, audience)
		after["source_system"], after["source_activity_id"], after["assignee_id"] = in.SourceSystem, in.sourceActivity(), in.AssigneeID
	}
	if in.audienceOf != nil {
		if err := inheritAudienceTx(ctx, tx, id, *in.audienceOf); err != nil {
			return err
		}
		audience, err := readAudienceImage(ctx, tx, id)
		if err != nil {
			return err
		}
		maps.Copy(after, audience)
	}
	evidence := map[string]any{}
	if in.Kind == string(crmcontracts.ActivityKindEmail) && in.RFCMessageID != "" {
		evidence[fieldRFCMessageID] = in.RFCMessageID
	}
	if in.Kind == KindMeeting && in.HostUserID != nil {
		evidence[storekit.ExplicitMeetingHostEvidence] = in.HostUserID.UUID
	}
	if stated := statedCountriesEvidence(in.statedCountries); stated != nil {
		evidence[recipientCountriesField] = stated
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "create", "activity", id.UUID, nil, after, evidence)
	if err != nil {
		return err
	}
	payload, err := activityCapturedPayload(ctx, in)
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, payload)
}

// statedCountriesEvidence names each recipient whose country the sender
// stated, with the code or "cleared". An auditor reads from it who moved a
// recipient's jurisdiction on which message.
func statedCountriesEvidence(stated []commsauthz.CountryOverride) map[string]string {
	if len(stated) == 0 {
		return nil
	}
	out := make(map[string]string, len(stated))
	for _, o := range stated {
		out[o.Address] = o.Country
		if o.Cleared() {
			out[o.Address] = "cleared"
		}
	}
	return out
}

// inheritAudienceTx gives a new transcript the readers of the meeting it
// records. A transcript that already exists follows later changes through
// followRecordedMeetingTx.
func inheritAudienceTx(ctx context.Context, tx pgx.Tx, id, from ids.ActivityID) error {
	return copyMeetingReadersTx(ctx, tx, id, from)
}
