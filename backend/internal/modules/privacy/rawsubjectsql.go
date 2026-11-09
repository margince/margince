// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/originalstore"
)

const (
	sqlArgSubject = "subject"
	sqlArgEmail   = "email"
	sqlArgEmails  = "emails"
	sqlArgLead    = "lead"
	sqlArgKey     = "key"
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
