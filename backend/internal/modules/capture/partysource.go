// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type partySourceEvidence struct {
	Activity        ids.ActivityID
	User            ids.UUID
	Provider        string
	Sent            bool
	Meeting         bool
	Containers      []string
	Recipients      []string
	Addresses       []string
	ContainersKnown bool
	Blocked         bool
	Original        *ids.UUID
}

func recordPartySourceTx(ctx context.Context, tx pgx.Tx, in partySourceEvidence) error {
	if in.User.IsZero() || in.Provider == "" {
		return nil
	}
	var imported bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capture_import WHERE activity_id=@activity AND user_id=@user)`, pgx.NamedArgs{partyActivityArg: in.Activity, partyUserArg: in.User}).Scan(&imported); err != nil {
		return err
	}
	if !imported {
		return nil
	}
	erased, err := storekit.SuppressedEmails(ctx, tx, append(append([]string{}, in.Recipients...), in.Addresses...))
	if err != nil {
		return err
	}
	in.Recipients = retainedPartyAddresses(in.Recipients, erased)
	in.Addresses = retainedPartyAddresses(in.Addresses, erased)
	args := pgx.NamedArgs{partyActivityArg: in.Activity, partyUserArg: in.User, partyProviderArg: in.Provider, "sent": in.Sent, "containers": append([]string{}, in.Containers...), "recipients": append([]string{}, in.Recipients...), "addresses": append([]string{}, in.Addresses...), "known": in.ContainersKnown, "blocked": in.Blocked, "original": in.Original, "pending": in.Sent || in.Meeting}
	query := `INSERT INTO capture_party_source(activity_id,user_id,provider,sent_by_owner,containers,recipients,policy_addresses,containers_known,admission_blocked,raw_capture_id,admission_pending)
 SELECT @activity,@user,@provider,@sent,@containers,@recipients,@addresses,@known,@blocked,coalesce(a.raw_capture_id,@original),@pending
 FROM activity a WHERE a.id=@activity
 ON CONFLICT(activity_id,user_id,provider) DO UPDATE SET
 sent_by_owner=capture_party_source.sent_by_owner OR EXCLUDED.sent_by_owner,
 admission_pending=capture_party_source.sent_by_owner OR @pending, admission_generation=capture_party_source.admission_generation+1,
 raw_capture_id=coalesce(capture_party_source.raw_capture_id,EXCLUDED.raw_capture_id),
 containers_known=capture_party_source.containers_known OR EXCLUDED.containers_known,
 containers=CASE WHEN EXCLUDED.containers_known AND (EXCLUDED.sent_by_owner OR NOT capture_party_source.sent_by_owner) THEN EXCLUDED.containers ELSE capture_party_source.containers END,
 admission_blocked=CASE WHEN EXCLUDED.containers_known AND (EXCLUDED.sent_by_owner OR NOT capture_party_source.sent_by_owner) THEN EXCLUDED.admission_blocked ELSE capture_party_source.admission_blocked OR EXCLUDED.admission_blocked END,
 recipients=CASE WHEN EXCLUDED.sent_by_owner OR NOT capture_party_source.sent_by_owner THEN EXCLUDED.recipients ELSE capture_party_source.recipients END,
 policy_addresses=CASE WHEN EXCLUDED.sent_by_owner OR NOT capture_party_source.sent_by_owner THEN EXCLUDED.policy_addresses ELSE capture_party_source.policy_addresses END
 WHERE (@pending AND NOT capture_party_source.admission_pending)
 OR (coalesce(capture_party_source.raw_capture_id,EXCLUDED.raw_capture_id) IS DISTINCT FROM capture_party_source.raw_capture_id)
 OR (EXCLUDED.sent_by_owner AND NOT capture_party_source.sent_by_owner)
 OR (EXCLUDED.containers_known AND NOT capture_party_source.containers_known)
 OR (EXCLUDED.containers_known AND (EXCLUDED.sent_by_owner OR NOT capture_party_source.sent_by_owner) AND capture_party_source.containers IS DISTINCT FROM EXCLUDED.containers)
 OR (EXCLUDED.admission_blocked AND NOT capture_party_source.admission_blocked)
 OR (EXCLUDED.containers_known AND (EXCLUDED.sent_by_owner OR NOT capture_party_source.sent_by_owner) AND capture_party_source.admission_blocked IS DISTINCT FROM EXCLUDED.admission_blocked)
 OR ((EXCLUDED.sent_by_owner OR NOT capture_party_source.sent_by_owner) AND (capture_party_source.recipients IS DISTINCT FROM EXCLUDED.recipients OR capture_party_source.policy_addresses IS DISTINCT FROM EXCLUDED.policy_addresses))`
	if _, err := tx.Exec(ctx, query, args); err != nil {
		return err
	}
	outcome := "live"
	if !in.ContainersKnown {
		outcome = "restored"
	}
	return markPartySourceKnownTx(ctx, tx, in.Activity, outcome)
}

func partyEvidenceFor(activity ids.ActivityID, user ids.UUID, provider string, rec connector.NormalizedRecord, fields ActivityFields) partySourceEvidence {
	in := partySourceEvidence{Activity: activity, User: user, Provider: provider, Sent: rec.Counterparty.SentByOwner(), Containers: rec.Containers, ContainersKnown: true, Blocked: fields.ProviderPrivate, Meeting: fields.Kind == meetingKind}
	if fields.Envelope != nil {
		in.Recipients = append(in.Recipients, fields.Envelope.OpenRecipients...)
	} else {
		for _, party := range rec.Participants {
			if party.Role == "to" || party.Role == "cc" {
				in.Recipients = append(in.Recipients, party.Email)
			}
		}
	}
	for i, address := range in.Recipients {
		in.Recipients[i] = normalizeEmail(address)
	}
	for _, address := range rec.Addresses {
		in.Addresses = append(in.Addresses, normalizeEmail(address))
	}
	return in
}

func recordPartyEvidenceTx(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, owner ids.UUID, provider string, rec connector.NormalizedRecord, fields ActivityFields) error {
	return recordPartySourceTx(ctx, tx, partyEvidenceFor(activity, owner, provider, rec, fields))
}

func (s *Sink) revokePartySourceTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) error {
	keys := []string{rec.NaturalKey.SourceID}
	if scoped, ok := scopedToSeat(ctx, rec); ok {
		keys = append(keys, scoped.NaturalKey.SourceID)
	}
	var resolved ids.ActivityID
	if kind, key := s.identityOfRecord(rec); kind != "" && s.resolveIdentity != nil {
		id, found, err := s.resolveIdentity(ctx, tx, kind, key)
		if err != nil {
			return err
		}
		if found {
			resolved = id
		}
	}
	_, err := tx.Exec(ctx, `UPDATE capture_party_source ps SET admission_blocked=true
 FROM activity a WHERE a.id=ps.activity_id AND ps.user_id=@user AND ps.provider=@provider
 AND ((a.source_system=@system AND a.source_id=ANY(@keys::text[])) OR a.id=@resolved)
 AND NOT ps.admission_blocked`, pgx.NamedArgs{partyUserArg: actorUserID(ctx), partyProviderArg: deliveringConnector(ctx), "system": rec.NaturalKey.SourceSystem, "keys": keys, "resolved": resolved})
	return err
}

func retainedPartyAddresses(addresses []string, erased map[string]bool) []string {
	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if !erased[normalizeEmail(address)] {
			out = append(out, address)
		}
	}
	return out
}

const (
	partyActivityArg = "activity"
	partyUserArg     = "user"
	partyProviderArg = "provider"
)
