// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type legacyAuditEntry struct {
	id, entity                       ids.UUID
	kind, action, actorType, actorID string
	at                               time.Time
	needed                           bool
}

// RedactLinkedHistoricalAuditPageTx corrects old linked copies one audit row at
// a time. A metadata proof leaves the domain record's erasure boundary alone.
func RedactLinkedHistoricalAuditPageTx(ctx context.Context, tx pgx.Tx, after, until ids.UUID, limit int, country CountryReader) (HistoricalAuditPage, error) {
	return redactLinkedHistoricalPage(ctx, tx, after, until, limit, country, false)
}

// RedactLinkedHistoricalIdentitiesPageTx runs after the linked-copy pass, while
// original request references are still available to identify older copies.
func RedactLinkedHistoricalIdentitiesPageTx(ctx context.Context, tx pgx.Tx, after, until ids.UUID, limit int, country CountryReader) (HistoricalAuditPage, error) {
	return redactLinkedHistoricalPage(ctx, tx, after, until, limit, country, true)
}

func redactLinkedHistoricalPage(ctx context.Context, tx pgx.Tx, after, until ids.UUID, limit int, country CountryReader, identitiesOnly bool) (HistoricalAuditPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return HistoricalAuditPage{}, err
	}
	floor, err := ResolveFloorPolicy(ctx, tx, country)
	if err != nil {
		return HistoricalAuditPage{}, err
	}
	entries, err := legacyAuditEntries(ctx, tx, after, until, limit, legacyAuditKinds(), identitiesOnly)
	if err != nil {
		return HistoricalAuditPage{}, err
	}
	page := HistoricalAuditPage{}
	for _, entry := range entries {
		page.Scanned = append(page.Scanned, entry.id)
		if !entry.needed {
			continue
		}
		changed, err := redactLinkedHistoricalEntry(ctx, tx, entry, floor)
		if err != nil {
			return HistoricalAuditPage{}, err
		}
		if changed {
			page.Changed++
		}
	}
	return page, nil
}

// legacyAuditEntries pages over the audit rows of the given entity types.
// identities picks the request-create rows; false takes the rest.
func legacyAuditEntries(ctx context.Context, tx pgx.Tx, after, until ids.UUID, limit int, kinds []string, identities bool) ([]legacyAuditEntry, error) {
	// Spelled as plain equalities for the identity pass, so it can read the
	// request rows through idx_audit_entity.
	split := `NOT (history.entity_type=@request_entity AND history.action=@create)`
	if identities {
		split = `history.entity_type=@request_entity AND history.action=@create`
	}
	rows, err := tx.Query(ctx, `SELECT history.id,history.entity_type,history.entity_id,history.action,history.actor_type,history.actor_id,history.occurred_at,
 NOT(history.action=ANY(@verbs))
 AND NOT EXISTS(SELECT 1 FROM audit_log own WHERE own.entity_type=history.entity_type AND own.entity_id=history.entity_id
  AND `+storekit.PhysicalErasureCertificateSQL("own", "@verbs")+`) AND (
 `+storekit.AuditRedactionNeededSQL+`
 OR EXISTS(SELECT 1 FROM event_outbox retained WHERE retained.envelope->'trace'->>'audit_log_id'=history.id::text))
 FROM audit_log history WHERE history.id>@after AND history.id<@until
 AND history.entity_type=ANY(@kinds)
 AND `+split+`
 ORDER BY history.id LIMIT @limit`,
		pgx.StrictNamedArgs{
			"verbs": storekit.AuditRedactionActions(), "after": after, "until": until, auditPageLimitParameter: limit,
			"kinds": kinds, "request_entity": rightsRequestEntity, "create": actionCreate,
		})
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (legacyAuditEntry, error) {
		var entry legacyAuditEntry
		err := row.Scan(&entry.id, &entry.kind, &entry.entity, &entry.action, &entry.actorType, &entry.actorID, &entry.at, &entry.needed)
		return entry, err
	})
}

func redactLinkedHistoricalEntry(ctx context.Context, tx pgx.Tx, entry legacyAuditEntry, floor FloorPolicy) (bool, error) {
	match, found, err := legacyAuditSubject(ctx, tx, entry)
	if err != nil || !found {
		return false, err
	}
	return correctLinkedHistoricalEntry(ctx, tx, entry, match, floor)
}

// correctLinkedHistoricalEntry redacts one entry whose erased subject is proved,
// unless that subject is held.
func correctLinkedHistoricalEntry(ctx context.Context, tx pgx.Tx, entry legacyAuditEntry, match legacyAuditMatch, floor FloorPolicy) (bool, error) {
	held, err := historicalAuditUnderHold(ctx, tx, entry.id, match.kind, match.entity, floor)
	if err != nil || held {
		return false, err
	}
	proof, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "audit_log", entry.id, nil,
		map[string]any{"payload_redaction": true, "legacy_erased_record": true, "boundary": match.certificate.String()})
	if err != nil {
		return false, err
	}
	return true, storekit.RedactHistoricalAuditEntry(ctx, tx, proof)
}

type legacyAuditMatch struct {
	kind                string
	entity, certificate ids.UUID
}

func legacyAuditSubject(ctx context.Context, tx pgx.Tx, entry legacyAuditEntry) (legacyAuditMatch, bool, error) {
	query, args := legacyAuditSubjectQuery(entry)
	if query == "" {
		return legacyAuditMatch{}, false, nil
	}
	var match legacyAuditMatch
	err := tx.QueryRow(ctx, query, args).Scan(&match.kind, &match.entity, &match.certificate)
	if errors.Is(err, pgx.ErrNoRows) {
		return legacyAuditMatch{}, false, nil
	}
	return match, err == nil, err
}

var legacyPrivacyBoundary = storekit.PhysicalErasureCertificateSQL("erasure", "@verbs") + `
 AND (@occurred,@audit) <= (erasure.occurred_at,erasure.id)`
