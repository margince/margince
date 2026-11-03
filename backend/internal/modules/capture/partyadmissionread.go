// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func (s *Sink) partyAdmissionSources(ctx context.Context, tx pgx.Tx, activity ids.ActivityID) ([]partySource, error) {
	owner, err := partyAdmissionOwner(ctx, tx, activity)
	if err != nil || owner.IsZero() {
		return nil, err
	}
	var args []any
	arg := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	activityArg, ownerArg := arg(activity), arg(owner)
	query := `SELECT ps.user_id,ps.provider,ps.containers,ps.containers_known,false,ps.admission_blocked,ps.policy_addresses,ps.admission_generation,NULL::uuid FROM capture_party_source ps
 WHERE ps.activity_id=` + activityArg + ` AND ps.user_id=` + ownerArg + `
 UNION SELECT delivery.user_id,delivery.provider,ARRAY[]::text[],true,true,false,ARRAY(SELECT jsonb_array_elements_text(coalesce(delivery.recipients,'[]'::jsonb)||coalesce(delivery.cc,'[]'::jsonb)||coalesce(delivery.bcc,'[]'::jsonb)) UNION SELECT delivery.from_address WHERE delivery.from_address IS NOT NULL),0::bigint,delivery.id FROM comms_outbound delivery
 WHERE delivery.activity_id=` + activityArg + ` AND delivery.user_id=` + ownerArg + `
 AND ` + sentUserMailSQL("delivery") + `
 AND NOT EXISTS(SELECT 1 FROM capture_party_source revoked WHERE revoked.activity_id=delivery.activity_id AND revoked.user_id=delivery.user_id AND revoked.admission_blocked)
 ORDER BY 1,2`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[partySource])
}

func partyAdmissionOwner(ctx context.Context, tx pgx.Tx, activity ids.ActivityID) (ids.UUID, error) {
	_, owner := capturePrincipal(ctx)
	if !owner.IsZero() {
		return owner, nil
	}
	args := []any{activity}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT DISTINCT user_id FROM comms_outbound WHERE activity_id=$%d AND `+sentUserMailSQL("comms_outbound")+` ORDER BY user_id LIMIT 2`, len(args)), args...)
	if err != nil {
		return ids.Nil, err
	}
	owners, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil || len(owners) != 1 {
		return ids.Nil, err
	}
	return owners[0], nil
}

func meetingPartyEligibleSQL(activity, party string) string {
	return party + `.role IN ('attendee','organizer') AND coalesce(` + party + `.response,'')<>'declined'
 AND NOT ` + party + `.past_party_cap AND ` + party + `.roster_seat IS NOT NULL
 AND EXISTS (SELECT 1 FROM activity_participant owner
 WHERE owner.activity_id=` + activity + `.id AND owner.roster_seat=` + party + `.roster_seat
 AND owner.user_id=` + party + `.roster_seat AND NOT owner.past_party_cap
 AND ` + auth.ParticipantStandsSQL("owner", activity) + `
 AND (owner.role='organizer' OR owner.response='accepted'))
 AND NOT EXISTS(SELECT 1 FROM capture_party_source private_source WHERE private_source.activity_id=` + activity + `.id AND private_source.user_id=` + party + `.roster_seat AND private_source.admission_blocked)`
}

func eligibleAdmissionPartiesTx(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, source partySource, includeResolved bool) ([]admissionParty, error) {
	var args []any
	arg := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	activityArg, userArg, providerArg := arg(activity), arg(source.User), arg(source.Provider)
	governedArg := arg(source.GovernedSend)
	unresolved := ""
	if !includeResolved {
		unresolved = " AND p.contact_id IS NULL"
	}
	query := `SELECT DISTINCT lower(p.address),coalesce(p.display_name,'')
 FROM activity a JOIN activity_participant p ON p.activity_id=a.id
 WHERE a.id=` + activityArg + ` AND p.user_id IS NULL` + unresolved + `
 AND nullif(p.address,'') IS NOT NULL AND NOT p.past_party_cap
 AND ` + auth.ParticipantStandsSQL("p", "a") + `
 AND ` + auth.ActivityAvailableClause("a") + `
 AND ((a.kind='email' AND p.role IN ('to','cc') AND (
 EXISTS (SELECT 1 FROM capture_party_source ps WHERE ps.activity_id=a.id AND ps.user_id=` + userArg + `
 AND ps.provider=` + providerArg + ` AND ps.sent_by_owner AND lower(p.address)=ANY(ps.recipients))
 OR (` + governedArg + `::boolean AND EXISTS (SELECT 1 FROM comms_outbound delivery
 WHERE delivery.activity_id=a.id AND delivery.user_id=` + userArg + ` AND ` + sentUserMailSQL("delivery") + ` AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(delivery.recipients || delivery.cc) recipient(address) WHERE lower(recipient.address)=lower(p.address))))))
 OR (a.kind='meeting' AND p.roster_seat=` + userArg + `
 AND coalesce(a.meeting_status,'') NOT IN ('no_show','canceled') AND ` + meetingPartyEligibleSQL("a", "p") + `))
 ORDER BY 1,2`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[admissionParty])
}

func readPartyAdmissionRecord(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, source partySource, rec *connector.NormalizedRecord) error {
	var fields ActivityFields
	args := []any{activity}
	query := fmt.Sprintf(`SELECT kind,coalesce(direction,''),coalesce(subject,''),coalesce(body,''),coalesce(source,''),coalesce(thread_key,''),
 ARRAY(SELECT DISTINCT address FROM activity_participant WHERE activity_id=a.id AND address IS NOT NULL)
 FROM activity a WHERE a.id=$%d`, len(args))
	if err := tx.QueryRow(ctx, query, args...).Scan(&fields.Kind, &fields.Direction, &fields.Subject, &fields.Body, &rec.Source, &rec.ThreadKey, &rec.Addresses); err != nil {
		return err
	}
	rec.Addresses = append(rec.Addresses, source.Addresses...)
	rec.Fields = fields
	rec.NaturalKey = connector.NaturalKey{SourceSystem: source.Provider, SourceID: activity.String()}
	rec.Containers = source.Containers
	return nil
}
