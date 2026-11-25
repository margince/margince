// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	sqlArgSubject = "subject"
	sqlArgEmail   = "email"
	sqlArgEmails  = "emails"
	sqlArgLead    = "lead"
	sqlArgKey     = "key"
	sqlArgContact = "contact"
	sqlArgKind    = "kind"
)

func rawEmailMatchSQL(payload, email string) string {
	return `(` + payload + `)::text ILIKE '%' || ` + storekit.EscapeLikeSQL(email) + ` || '%' ESCAPE '\'`
}

func rawChannelMatchSQL(payload, source, provider, account string) string {
	return `(` + source + ` = ` + provider + ` AND ((` + payload + `)->'message'->'from'->>'id' = ` + account + `
 OR (` + payload + `)->'my_chat_member'->'chat'->>'id' = ` + account + `))`
}

func rawSubjectMatchSQL(payload, source string) string {
	return `EXISTS (SELECT 1 FROM unnest(@emails::text[]) AS e(email)
 WHERE ` + rawEmailMatchSQL(payload, "e.email") + `)
 OR EXISTS (SELECT 1 FROM unnest(@providers::text[], @accounts::text[]) AS i(provider, account)
 WHERE ` + rawChannelMatchSQL(payload, source, "i.provider", "i.account") + `)`
}

type originalSubjectPattern struct {
	Version    int      `json:"version"`
	EmailBits  []int    `json:"email_bits,omitempty"`
	ChannelIDs []string `json:"channel_ids,omitempty"`
}

type originalHintQuery struct {
	Subjects        *originalSubjectPattern `json:"subjects,omitempty"`
	MatchedSubjects []string                `json:"matched_subjects,omitempty"`
}

func externalOriginalCandidateSQL(scope rawSubjectScope, args pgx.NamedArgs) string {
	bind := func(value originalHintQuery) string {
		name := fmt.Sprintf("original_hint_%d", len(args))
		args[name] = value
		return "@" + name + "::jsonb"
	}
	clauses := []string{
		`(payload_object->'subjects'->>'version') IS DISTINCT FROM '` + fmt.Sprint(originalstore.SubjectHintVersion) + `'`,
		`payload_object @> ` + bind(originalHintQuery{MatchedSubjects: []string{scope.contact.String()}}),
	}
	for _, channel := range scope.channels {
		name := fmt.Sprintf("original_provider_%d", len(args))
		args[name] = channel.Provider
		pattern := originalHintQuery{Subjects: &originalSubjectPattern{Version: originalstore.SubjectHintVersion, ChannelIDs: []string{channel.ChannelUserID}}}
		clauses = append(clauses, `(source_system = @`+name+` AND payload_object @> `+bind(pattern)+`)`)
	}
	if len(scope.emails) > 0 {
		clauses = append(clauses, `jsonb_typeof(payload_object->'subjects'->'email_bits') IS DISTINCT FROM 'array'`,
			`coalesce(payload_object->'subjects'->>'email_saturated','false') <> 'false'`)
		for _, query := range originalstore.EmailHintQueries(scope.emails) {
			if query == nil {
				return `payload_object IS NOT NULL`
			}
			clauses = append(clauses, `payload_object @> `+bind(originalHintQuery{Subjects: &originalSubjectPattern{Version: originalstore.SubjectHintVersion, EmailBits: query}}))
		}
	}
	return `payload_object IS NOT NULL AND (` + strings.Join(clauses, ` OR `) + `)`
}

// linkedOriginalsSQL selects the raw_capture ids of originals whose activity is
// filed on one record through activity_link. The column is contact_id or
// lead_id, and subject is the SQL for the record id, such as "$1".
//
// A member who chose the record for a message bound no identity to it. The
// original may name no address or account the hint arms could find. The link
// is then the only evidence of whom the message is about.
//
// Export and disclosure read every such original. Erasure reads the ones it may
// destroy. A legal hold on the activity keeps its original. So does another
// activity that names the original without being filed on this record alone.
// A message filed on a second live subject is one.
func linkedOriginalsSQL(column, subject string, erasing bool) string {
	query := `SELECT a.raw_capture_id FROM activity a JOIN activity_link l ON l.activity_id = a.id
	  WHERE l.` + column + ` = ` + subject + ` AND a.raw_capture_id IS NOT NULL`
	if !erasing {
		return query
	}
	return query + notTransitivelyHeld("a.id") + `
	    AND NOT EXISTS (SELECT 1 FROM activity o WHERE o.raw_capture_id = a.raw_capture_id
	      AND (NOT EXISTS (SELECT 1 FROM activity_link ol WHERE ol.activity_id = o.id AND ol.` + column + ` = ` + subject + `)
	        OR ` + otherLiveSubjectSQL("contact_id", column, subject) + `
	        OR ` + otherLiveSubjectSQL("lead_id", column, subject) + `))`
}

// otherLiveSubjectSQL is true when the activity o is also filed on a live
// record of this kind. For the kind being erased that means another record.
func otherLiveSubjectSQL(kind, column, subject string) string {
	differs := ""
	if kind == column {
		differs = ` AND xl.` + kind + ` <> ` + subject
	}
	return `EXISTS (SELECT 1 FROM activity_link xl JOIN ` + strings.TrimSuffix(kind, "_id") + ` s ON s.id = xl.` + kind + `
	          WHERE xl.activity_id = o.id AND s.archived_at IS NULL` + differs + `)`
}

// deleteLinkedOriginalsTx destroys the originals of messages filed on one record.
func deleteLinkedOriginalsTx(ctx context.Context, tx pgx.Tx, column string, record ids.UUID) (int64, error) {
	return deleteRawOriginalsTx(ctx, tx, `r.id IN (`+linkedOriginalsSQL(column, "@linked_subject::uuid", true)+`)`,
		pgx.NamedArgs{"linked_subject": record.String()})
}

// deleteSubjectOriginalsTx destroys the originals tied to a contact without an
// address: those tagged with the contact, and those of messages filed on it by
// choice.
func deleteSubjectOriginalsTx(ctx context.Context, tx pgx.Tx, contact ids.ContactID) (int64, error) {
	tagged, err := deleteRawOriginalsTx(ctx, tx,
		`r.payload_object @> jsonb_build_object('matched_subjects', jsonb_build_array(@subject::text))`,
		pgx.NamedArgs{sqlArgSubject: contact.String()})
	if err != nil {
		return 0, err
	}
	linked, err := deleteLinkedOriginalsTx(ctx, tx, "contact_id", contact.UUID)
	return tagged + linked, err
}

// wipeTwinsAndOriginals anonymizes the subject's lead twins and
// destroys the originals of messages filed on them by choice.
func wipeTwinsAndOriginals(ctx context.Context, tx pgx.Tx, contact ids.ContactID, emails []string) ([]ids.UUID, error) {
	wiped, err := anonymizeLeadTwins(ctx, tx, contact, emails)
	for _, lead := range wiped {
		if err != nil {
			break
		}
		_, err = deleteLinkedOriginalsTx(ctx, tx, "lead_id", lead)
	}
	return wiped, err
}
