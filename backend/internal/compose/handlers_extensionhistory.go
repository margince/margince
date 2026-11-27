// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What installed extensions add to one core record's History
// (GET /v1/extension-history). Core builds every entry from a declaration or
// from a signal row, so an extension writes no timeline entry of its own.

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

// extensionHistoryCap bounds each source, so a record with a long tail of
// extension rows cannot make the History read unbounded.
const extensionHistoryCap = 50

// ListExtensionHistory is served on the analytics receiver because that is
// where the report directory, the other read over extension tables, lives. The
// record is probed first, so a record outside the caller's row scope is a 404.
func (h analyticsQueryHandlers) ListExtensionHistory(w http.ResponseWriter, r *http.Request, params crmcontracts.ListExtensionHistoryParams) {
	// The type names an auth object and a table, so an unknown one stops here
	// instead of reaching the row probe.
	if !params.EntityType.Valid() {
		httperr.Write(w, r, httperr.Validation("entity_type", "invalid_parameter",
			"entity_type must be one of contact, company, deal"))
		return
	}
	ctx := r.Context()
	entity, id := string(params.EntityType), ids.UUID(params.EntityId)
	entries := []crmcontracts.ExtensionHistoryEntry{}
	err := database.WithWorkspaceTx(ctx, h.db.Pool(), func(tx pgx.Tx) error {
		if err := auth.Require(ctx, entity, principal.ActionRead); err != nil {
			return err
		}
		if err := auth.EnsureVisibleExisting(ctx, tx, entity, id); err != nil {
			return err
		}
		rows, err := extensionRecordEntries(ctx, tx, entity, id)
		if err != nil {
			return err
		}
		sigs, err := extensionSignalEntries(ctx, tx, entity, id)
		entries = append(entries, rows...)
		entries = append(entries, sigs...)
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	slices.SortStableFunc(entries, func(a, b crmcontracts.ExtensionHistoryEntry) int { return b.At.Compare(a.At) })
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ExtensionHistoryResponse{Data: entries})
}

// extensionRecordEntries reads, kind by kind, the rows that reference the
// record. A kind the caller may not read is left out, as in the kind directory.
// Of a kind they may read, they see the rows inside its row scope.
func extensionRecordEntries(ctx context.Context, tx pgx.Tx, entity string, id ids.UUID) ([]crmcontracts.ExtensionHistoryEntry, error) {
	var out []crmcontracts.ExtensionHistoryEntry
	for _, d := range recordkinds.All() {
		if !auth.Allows(ctx, d.Name, principal.ActionRead) {
			continue
		}
		args := []any{id}
		arg := func(v any) int { args = append(args, v); return len(args) }
		scope, err := auth.ExtensionScopeClause(ctx, d.Name, "t", arg)
		if err != nil {
			return nil, err
		}
		query, ok := extensionHistoryQuery(d, entity, scope)
		if !ok {
			continue
		}
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var (
				rowID ids.UUID
				title string
				at    time.Time
			)
			if err := rows.Scan(&rowID, &title, &at); err != nil {
				rows.Close()
				return nil, err
			}
			if strings.TrimSpace(title) == "" {
				title = d.DisplayName
			}
			out = append(out, crmcontracts.ExtensionHistoryEntry{
				Id: crmcontracts.Id(rowID), Source: crmcontracts.ExtensionHistoryEntrySourceRecord, Kind: d.Name, Title: title, At: at,
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// extensionHistoryQuery is the read of one kind's rows about one core record,
// bound as $1, narrowed by scope unless it is empty. It answers false for a
// kind with no timeline, or none for this record. The declaration validated
// every identifier. Each is quoted anyway.
func extensionHistoryQuery(d recordkinds.Declared, entity, scope string) (string, bool) {
	if d.Timeline == nil {
		return "", false
	}
	i := slices.IndexFunc(d.Timeline.Subjects, func(s extension.TimelineSubject) bool { return s.Entity == entity })
	if i < 0 {
		return "", false
	}
	q := func(name string) string { return pgx.Identifier{name}.Sanitize() }
	if scope != "" {
		scope = ` AND ` + scope
	}
	return `SELECT t.id, coalesce(t.` + q(d.Timeline.LabelField) + `::text, ''), t.` + q(d.Timeline.AtColumn) +
		` FROM ` + extSchemaPrefix + q(d.Table) + ` t WHERE t.` + q(d.Timeline.Subjects[i].Column) +
		` = $1 AND t.archived_at IS NULL AND t.` + q(d.Timeline.AtColumn) + ` IS NOT NULL` + scope +
		` ORDER BY t.` + q(d.Timeline.AtColumn) + ` DESC, t.id DESC LIMIT ` + strconv.Itoa(extensionHistoryCap), true
}

// extensionSignalEntries reads the signals of an extension kind about the
// record, through the signals module's own visible reader. A caller with no
// right on signals gets the rows without them.
func extensionSignalEntries(ctx context.Context, tx pgx.Tx, entity string, id ids.UUID) ([]crmcontracts.ExtensionHistoryEntry, error) {
	limit := extensionHistoryCap
	list, _, err := signals.ListSignalsTx(ctx, tx, signals.ListSignalsInput{
		Limit: &limit, SubjectType: &entity, SubjectID: &id, ExtensionKindsOnly: true,
	}, signalSummaryWords)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, nil
	}
	out := make([]crmcontracts.ExtensionHistoryEntry, 0, len(list))
	for _, s := range list {
		out = append(out, crmcontracts.ExtensionHistoryEntry{
			Id: s.Id, Source: crmcontracts.ExtensionHistoryEntrySourceSignal, Kind: s.Kind, Title: s.Summary, At: s.DetectedAt,
		})
	}
	return out, err
}
