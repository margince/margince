// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// What a reader learns about one list beyond its row: how many of its members
// they may see, whether it is healthy, what uses it, why a record is on it,
// and what changed on it. Every answer here is row-scoped to the reader.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The health a list reports (List.health).
const (
	healthOK        = "ok"
	healthOwnerless = "ownerless"
	healthInvalid   = "invalid"
	// healthRetiredField is a Live List still evaluating on a retired custom
	// field's kept values, whose clause its steward should replace.
	healthRetiredField = "retired_field"
	// healthRetiredTag is a Live List whose filter names an archived tag, which
	// no longer matches any record, so its steward should name the live one.
	healthRetiredTag = "retired_tag"
	// healthUnavailable is a list over an extension kind this reader cannot
	// read: its unit was removed, or the reader lacks the kind's read right.
	healthUnavailable = "unavailable"
)

// ExportDetailListID is the system_log detail key a filtered export of a list
// records the list under, and what the list's dependencies read back.
const ExportDetailListID = "list_id"

// dependencyCap bounds the usage a list reports: the newest uses, not a log.
const dependencyCap = 20

// listSummary is a list as a reader sees it: the row, the members they may
// see, its health and whether they may change it.
type listSummary struct {
	listRow
	VisibleCount *int
	Health       string
	CanEdit      bool
	Dependencies []listDependency
	// RetiredFields is the retired custom fields a Live List's filter names.
	RetiredFields []string
	// RetiredTags is the archived tags a Live List's filter names.
	RetiredTags []ids.UUID
	LastCheck   *listCheck
	Pulse       *listPulse
	Joined      []ids.UUID
	Changes     *changeSummary
}

type listDependency struct {
	Kind       string
	OccurredAt time.Time
	Actor      *string
	// Rule is set for an automation rule that depends on the list.
	Rule *RuleUse
}

// RuleUse is an active automation rule that watches a Live List or adds to a
// Shortlist. ID and Name are empty for a reader who may not read automations.
type RuleUse struct {
	ID        ids.UUID
	Name      string
	Role      string
	CreatedAt time.Time
}

// WithRuleUses injects the read of the automation rules that depend on a list.
func (s *Store) WithRuleUses(read func(ctx context.Context, id ids.ListID) ([]RuleUse, error)) *Store {
	s.ruleUses = read
	return s
}

// Dependencies reads what uses a list: the active automation rules that watch
// or write it, then its filtered exports, newest first. None blocks a change:
// an export is a finished extraction, and a rule pauses itself when its list
// is archived.
func (s *Store) Dependencies(ctx context.Context, id ids.ListID) ([]listDependency, error) {
	exports, err := s.exportUses(ctx, id)
	if err != nil || s.ruleUses == nil {
		return exports, err
	}
	rules, err := s.ruleUses(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]listDependency, 0, len(rules)+len(exports))
	for i := range rules {
		out = append(out, listDependency{Kind: "automation", OccurredAt: rules[i].CreatedAt, Rule: &rules[i]})
	}
	return append(out, exports...), nil
}

// exportUses reads the newest filtered exports of a list.
func (s *Store) exportUses(ctx context.Context, id ids.ListID) ([]listDependency, error) {
	var out []listDependency
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := readVisibleList(ctx, tx, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT occurred_at, actor_id FROM system_log
			WHERE action = 'export' AND detail ->> @detail_key = @list_id::text
			ORDER BY occurred_at DESC LIMIT @cap`,
			pgx.StrictNamedArgs{"detail_key": ExportDetailListID, listIDField: id.String(), "cap": dependencyCap})
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (listDependency, error) {
			d := listDependency{Kind: "export"}
			err := row.Scan(&d.OccurredAt, &d.Actor)
			return d, err
		})
		return err
	})
	return out, err
}

// MemberExplanation says why a record is or is not on a list, for a reader
// who can see the record.
type MemberExplanation struct {
	ListType string
	Member   bool
	Eligible bool
	Clauses  *storekit.ExplainNode
	AddedBy  *string
	AddedAt  *time.Time
	Note     *string
}

// ExplainMember answers for a Live List with every clause judged by the SQL
// that decides membership, and for a Shortlist with who added the record,
// when and why. A record outside the reader's scope answers ErrNotFound.
func (s *Store) ExplainMember(ctx context.Context, listID ids.ListID, entityID ids.UUID) (MemberExplanation, error) {
	list, err := s.GetList(ctx, listID)
	if err != nil {
		return MemberExplanation{}, err
	}
	// The reader cannot see a record of a kind they cannot read.
	if kindUnavailable(ctx, list.EntityType) {
		return MemberExplanation{}, apperrors.ErrNotFound
	}
	if list.ListType == listTypeDynamic {
		engine, pred, err := s.liveFilter(ctx, list)
		if err != nil {
			return MemberExplanation{}, err
		}
		var why storekit.Explanation
		err = s.db.Tx(ctx, func(tx pgx.Tx) error {
			why, err = engine.Explain(ctx, tx, pred, entityID)
			return err
		})
		if err != nil {
			return MemberExplanation{}, err
		}
		return MemberExplanation{
			ListType: list.ListType, Member: why.Selected, Eligible: why.PassesBase, Clauses: &why.Root,
		}, nil
	}
	return s.explainChosen(ctx, list, entityID)
}

func (s *Store) explainChosen(ctx context.Context, list listRow, entityID ids.UUID) (MemberExplanation, error) {
	if err := auth.Require(ctx, list.EntityType, principal.ActionRead); err != nil {
		return MemberExplanation{}, err
	}
	out := MemberExplanation{ListType: list.ListType}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, list.EntityType, entityID); err != nil {
			return err
		}
		var addedBy string
		var addedAt time.Time
		err := tx.QueryRow(ctx, `
			SELECT added_by, created_at, note FROM list_member
			WHERE list_id = @list_id AND entity_type = @entity_type AND entity_id = @entity_id`,
			pgx.StrictNamedArgs{listIDField: list.ID, entityTypeField: list.EntityType, entityIDField: entityID}).Scan(&addedBy, &addedAt, &out.Note)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		out.Member, out.AddedBy, out.AddedAt = true, &addedBy, &addedAt
		return nil
	})
	return out, err
}

// HistoryEntry is one change on a list: a Shortlist membership change, a
// Live List member seen entering or leaving, or a revision of its definition.
type HistoryEntry struct {
	ID                ids.UUID
	Kind              string
	OccurredAt        time.Time
	Actor             string
	EntityType        *string
	EntityID          *ids.UUID
	EntityName        *string
	Reason            *string
	Note              *string
	DefinitionVersion *int64
	Version           *int64
	Name              *string
	Definition        map[string]any
	Sharing           *string
	// Changes names what a revision changed against the version before it, in
	// revisionChangesSQL's words; nil when no earlier version is kept.
	Changes []string
	// Previous holds the version before a revision, for what Changes names.
	PreviousName       *string
	PreviousDefinition map[string]any
}

// History pages through a list's changes, newest first. A membership change
// about a record the reader cannot see now is left out, whatever they could
// see when it happened.
func (s *Store) History(ctx context.Context, listID ids.ListID, limit int, cursor string) ([]HistoryEntry, storekit.Page, error) {
	limit = pageSize(limit)
	list, err := s.GetList(ctx, listID)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	var after *storekit.Cursor
	if cursor != "" {
		decoded, err := storekit.DecodeCursor("list_history", cursor)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		after = &decoded
	}
	var out []HistoryEntry
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		sql, err := historySQL(ctx, list, after, limit, arg)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanHistory)
		return err
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	if len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		next, err := storekit.EncodeCursor("list_history", last.OccurredAt, last.ID)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		return out, storekit.Page{HasMore: true, NextCursor: next}, nil
	}
	return out, storekit.Page{}, nil
}

// historySQL is the union of the list's revisions and the membership changes
// of records this reader can see, keyset-paged newest first.
func historySQL(ctx context.Context, list listRow, after *storekit.Cursor, limit int, arg func(any) int) (string, error) {
	listPos := arg(list.ID)
	visible, err := observedVisibleClause(ctx, list.EntityType, arg)
	if err != nil {
		return "", err
	}
	page := ""
	if after != nil {
		page = fmt.Sprintf("WHERE (h.occurred_at, h.id) < ($%d, $%d)", arg(after.CreatedAt), arg(after.ID))
	}
	return fmt.Sprintf(`SELECT h.* FROM (
		SELECT ev.id, 'member_' || ev.action AS kind, ev.occurred_at, ev.actor,
		       ev.entity_type, ev.entity_id, %[5]s AS entity_name, ev.reason, ev.note, ev.definition_version,
		       NULL::bigint AS version, NULL::text AS name, NULL::jsonb AS definition, NULL::text AS sharing,
		       NULL::text[] AS changes, NULL::text AS previous_name, NULL::jsonb AS previous_definition
		FROM list_member_event ev %[6]s WHERE ev.list_id = $%[1]d AND %[2]s
		UNION ALL
		SELECT r.id, 'revised', r.changed_at, r.changed_by,
		       NULL, NULL, NULL, NULL, NULL, NULL, r.version, r.name, r.definition, r.sharing,
		       %[7]s, p.name, p.definition
		FROM list_revision r
		LEFT JOIN list_revision p ON p.list_id = r.list_id AND p.version = r.version - 1
		WHERE r.list_id = $%[1]d
	) h %[3]s ORDER BY h.occurred_at DESC, h.id DESC LIMIT $%[4]d`,
		listPos, visible, page, arg(limit+1),
		recordNameExpr(list.EntityType), recordNameJoin(list.EntityType, "ev.entity_id"), revisionChangesSQL), nil
}

// filterChangedSQL is whether revision r changed the filter of the version p
// before it. The history and the visit summary both ask it, so they agree.
const filterChangedSQL = `r.definition IS DISTINCT FROM p.definition`

// revisionChangesSQL is what revision r changed against the version p before
// it, as the contract's ListHistoryEntry.changes spells it; NULL with no p.
const revisionChangesSQL = `CASE WHEN p.id IS NULL THEN NULL ELSE array_remove(ARRAY[
		CASE WHEN r.name IS DISTINCT FROM p.name THEN 'name' END,
		CASE WHEN r.purpose IS DISTINCT FROM p.purpose THEN 'purpose' END,
		CASE WHEN ` + filterChangedSQL + ` THEN 'filter' END,
		CASE WHEN (r.sharing, r.team_id) IS DISTINCT FROM (p.sharing, p.team_id) THEN 'sharing' END,
		CASE WHEN r.steward_id IS DISTINCT FROM p.steward_id THEN 'steward' END
	], NULL) END`

func scanHistory(row pgx.CollectableRow) (HistoryEntry, error) {
	var h HistoryEntry
	err := row.Scan(&h.ID, &h.Kind, &h.OccurredAt, &h.Actor, &h.EntityType, &h.EntityID, &h.EntityName,
		&h.Reason, &h.Note, &h.DefinitionVersion, &h.Version, &h.Name, &h.Definition, &h.Sharing,
		&h.Changes, &h.PreviousName, &h.PreviousDefinition)
	return h, err
}

// actorNames resolves the display names of the human principals among
// actors; other principals have none.
func (s *Store) actorNames(ctx context.Context, actors []string) (map[string]string, error) {
	byUser := map[ids.UUID]string{}
	userIDs := make([]ids.UUID, 0, len(actors))
	for _, actor := range actors {
		if id, ok := principal.HumanUserID(actor); ok {
			byUser[id] = actor
			userIDs = append(userIDs, id)
		}
	}
	names := map[string]string{}
	if len(userIDs) == 0 {
		return names, nil
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, display_name FROM app_user WHERE id = ANY(@ids)`,
			pgx.StrictNamedArgs{"ids": userIDs})
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				return err
			}
			names[byUser[id]] = name
		}
		return rows.Err()
	})
	return names, err
}
