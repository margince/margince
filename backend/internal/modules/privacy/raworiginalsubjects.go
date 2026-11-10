// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type rawSubjectScope struct {
	contact  ids.ContactID
	emails   []string
	channels []channelIdentity
}

type externalOriginal struct {
	id     ids.UUID
	source string
	ref    originalstore.Reference
}

type originalMatches struct {
	refs map[ids.UUID]originalstore.Reference
	ids  []ids.UUID
}

var errOriginalReferenceChanged = fmt.Errorf("stored original changed during erasure; retry: %w", apperrors.ErrConflict)

func externalOriginalsTx(ctx context.Context, tx pgx.Tx, scope rawSubjectScope, lock bool) ([]externalOriginal, error) {
	providers, accounts := channelIdentityPairs(scope.channels)
	args := pgx.NamedArgs{sqlArgEmails: scope.emails, "providers": providers, "accounts": accounts, sqlArgSubject: scope.contact.String()}
	inline := ""
	if lock {
		inline = ` OR (payload_object IS NULL AND (` + rawSubjectMatchSQL("payload", "source_system") + `))`
	}
	rows, err := tx.Query(ctx, `SELECT id,source_system,payload_object FROM raw_capture WHERE `+
		externalOriginalCandidateSQL(scope, args)+inline+` ORDER BY id`, args)
	if err != nil {
		return nil, err
	}
	out, err := readOriginalReferences(rows)
	if err != nil || !lock {
		return out, err
	}
	var selected []ids.UUID
	for _, original := range out {
		if original.ref.Key == "" || originalCouldMatch(original, scope) {
			selected = append(selected, original.id)
		}
	}
	rows, err = tx.Query(ctx, `SELECT id,source_system,payload_object FROM raw_capture WHERE id=ANY(@ids) ORDER BY id FOR UPDATE`, pgx.NamedArgs{"ids": selected})
	if err != nil {
		return nil, err
	}
	return readOriginalReferences(rows)
}

func readOriginalReferences(rows pgx.Rows) ([]externalOriginal, error) {
	defer rows.Close()
	var out []externalOriginal
	for rows.Next() {
		var row externalOriginal
		var metadata []byte
		if err := rows.Scan(&row.id, &row.source, &metadata); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &row.ref); err != nil {
				return nil, fmt.Errorf("read stored original reference: %w", err)
			}
		}
		out = append(out, row)
	}

	return out, rows.Err()
}

func matchingExternalOriginalsTx(ctx context.Context, tx pgx.Tx, scope rawSubjectScope) (originalMatches, error) {
	originals, err := externalOriginalsTx(ctx, tx, scope, false)
	if err != nil {
		return originalMatches{}, err
	}
	out := originalMatches{refs: make(map[ids.UUID]originalstore.Reference, len(originals))}
	var providers, accounts []string
	for _, channel := range scope.channels {
		providers = append(providers, channel.Provider)
		accounts = append(accounts, channel.ChannelUserID)
	}
	args := pgx.NamedArgs{sqlArgEmails: scope.emails, "providers": providers, "accounts": accounts}
	for _, original := range originals {
		out.refs[original.id] = original.ref
		if !originalCouldMatch(original, scope) {
			continue
		}
		matched := slices.Contains(original.ref.MatchedSubjects, scope.contact.String()) || originalChannelMatch(original, scope)
		if !matched {
			payload, err := originalstore.Read(ctx, original.ref)
			if errors.Is(err, blobstore.ErrNotFound) && len(original.ref.MatchedSubjects) > 0 {
				continue
			}
			if err != nil {
				return originalMatches{}, err
			}
			args["payload"], args["source"] = payload, original.source
			if err := tx.QueryRow(ctx, "SELECT "+rawSubjectMatchSQL("@payload::jsonb", "@source::text"), args).Scan(&matched); err != nil {
				return originalMatches{}, err
			}
		}
		if matched {
			out.ids = append(out.ids, original.id)
		}
	}
	return out, nil
}

// Matches commit before destruction so a retry can identify a subject's original
// even when its object was deleted by a transaction that later rolled back.
func (e *Eraser) prepareOriginalErasure(ctx context.Context, subject ids.ContactID) (originalMatches, error) {
	var prepared originalMatches
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := e.erasableUnderFloor(ctx, tx, subject); err != nil {
			return err
		}
		keys, err := subjectIdentifiers(ctx, tx, subject)
		if err != nil {
			return err
		}
		prepared, err = matchingExternalOriginalsTx(ctx, tx, rawSubjectScope{subject, keys.emails, keys.identities})
		if err != nil {
			return err
		}
		for _, id := range prepared.ids {
			ref := prepared.refs[id]
			if slices.Contains(ref.MatchedSubjects, subject.String()) {
				continue
			}
			tag, err := tx.Exec(ctx, `UPDATE raw_capture r SET payload_object = jsonb_set(payload_object,
 '{matched_subjects}', coalesce(payload_object->'matched_subjects', '[]'::jsonb) || jsonb_build_array(@subject::text))
 WHERE id = @id AND payload_object->>'key' = @key AND payload_object->>'sha256' = @sha
   AND (payload_object->>'bytes')::bigint = @bytes
   AND NOT coalesce((payload_object->'matched_subjects') @> jsonb_build_array(@subject::text), false)`,
				pgx.NamedArgs{"id": id, sqlArgKey: ref.Key, "sha": ref.SHA256, "bytes": ref.Bytes, sqlArgSubject: subject.String()})
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return errOriginalReferenceChanged
			}
			ref.MatchedSubjects = append(ref.MatchedSubjects, subject.String())
			prepared.refs[id] = ref
		}
		return nil
	})
	return prepared, err
}

// Matching inline rows stay locked too, so a sweep cannot move their evidence
// out of the inline match after the external-reference manifest is checked.
func freezeOriginalErasureTx(ctx context.Context, tx pgx.Tx, scope rawSubjectScope, prepared originalMatches) error {
	current, err := externalOriginalsTx(ctx, tx, scope, true)
	if err != nil {
		return err
	}
	for _, original := range current {
		if original.ref.Key == "" {
			continue
		}
		known, found := prepared.refs[original.id]
		if !found || known.Key != original.ref.Key || known.SHA256 != original.ref.SHA256 || known.Bytes != original.ref.Bytes {
			return errOriginalReferenceChanged
		}
		if slices.Contains(known.MatchedSubjects, scope.contact.String()) && !slices.Contains(original.ref.MatchedSubjects, scope.contact.String()) {
			return errOriginalReferenceChanged
		}
	}
	return nil
}

func (e *Eraser) withPreparedOriginalErasure(ctx context.Context, subject ids.ContactID, erase func(pgx.Tx, originalMatches) error) error {
	for range 3 {
		prepared, err := e.prepareOriginalErasure(ctx, subject)
		if err == nil {
			err = e.db.Tx(ctx, func(tx pgx.Tx) error { return erase(tx, prepared) })
		}
		if !errors.Is(err, errOriginalReferenceChanged) {
			return err
		}
	}
	return errOriginalReferenceChanged
}

func freezeSubjectOriginalsTx(ctx context.Context, tx pgx.Tx, subject ids.ContactID, keys subjectKeys, prepared originalMatches) error {
	if err := refuseRivalIdentifierHolders(ctx, tx, subject, keys.emails, keys.identities); err != nil {
		return err
	}
	return freezeOriginalErasureTx(ctx, tx, rawSubjectScope{subject, keys.emails, keys.identities}, prepared)
}

func originalChannelMatch(original externalOriginal, scope rawSubjectScope) bool {
	if original.ref.Subjects == nil || original.ref.Subjects.Version != originalstore.SubjectHintVersion {
		return false
	}
	for _, channel := range scope.channels {
		if channel.Provider == original.source && slices.Contains(original.ref.Subjects.ChannelIDs, channel.ChannelUserID) {
			return true
		}
	}
	return false
}

func originalCouldMatch(original externalOriginal, scope rawSubjectScope) bool {
	if slices.Contains(original.ref.MatchedSubjects, scope.contact.String()) || original.ref.Subjects == nil || original.ref.Subjects.Version != originalstore.SubjectHintVersion || originalChannelMatch(original, scope) {
		return true
	}
	return slices.ContainsFunc(scope.emails, original.ref.Subjects.MayMentionEmail)
}
