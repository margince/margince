// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

type Store struct {
	// db binds the installation's workspace itself (ADR-0091 §9 step 3).
	db *database.DB
	// catalog widens the filter vocabulary with the workspace's own cf_*
	// columns, and answers which of them are still offered. Optional by design:
	// nil is the port's pass-through, and a store without one filters on core
	// fields alone.
	catalog CatalogReader
	// liveSteward is "this app_user may act" over app_user aliased u. The
	// identity module owns that rule, so compose injects it. Without it no
	// steward is judged gone, only a missing one.
	liveSteward string
	// dealAmount is a deal's worth in the base currency, as SQL over alias t.
	// The rate sheet belongs to deals and the rule to compose, which injects it.
	dealAmount string
	// baseCurrencyOf reads the installation's base currency; identity owns it.
	baseCurrencyOf func(ctx context.Context, tx pgx.Tx) (string, error)
}

// WithLiveSteward injects the identity module's rule for a seat that may act,
// rendered over app_user aliased u: it decides whether a list's steward is
// still there.
func (s *Store) WithLiveSteward(clause string) *Store {
	s.liveSteward = clause
	return s
}

// NewStore binds the store to the pool every read and write runs through.
func NewStore(db *database.DB) *Store {
	return &Store{db: db}
}

// CatalogReader is what collections needs of the custom-field catalogue, which
// is BOTH readers rather than the filterable one alone — because this module asks
// the catalogue two different questions.
//
// What a filter may SAY includes a retired column, so a saved segment built on
// one keeps evaluating (FilterableColumns). What a builder may OFFER for a new
// clause does not, per CUSTOM-FIELDS-AC-13's "hidden from API + filtering"
// (ActiveColumns). One reader cannot answer both, and the difference between them
// IS the retired set — which is why the fieldcatalog seam's own note that "a
// consumer of one has no use for the other" no longer holds for this consumer.
type CatalogReader interface {
	fieldcatalog.FilterableReader
	fieldcatalog.Reader
}

// WithFieldCatalog injects the custom-field vocabulary source. Compose calls it;
// a caller that never filters (the workflow adapter's list writes) needs no
// catalog and passes none.
func (s *Store) WithFieldCatalog(r CatalogReader) *Store {
	s.catalog = r
	return s
}

// The two list kinds (LVS-DDL-1's list_type CHECK). A static list is an
// explicit membership set; a dynamic one stores a filter its members are
// derived from, so the pair decides which of the two membership paths a
// read takes and whether a definition may be present at all.
//
// dynamicAddedBy carries the same string for a different question — who
// added a computed member, not what kind a list is.
const (
	listTypeStatic  = "static"
	listTypeDynamic = "dynamic"
)

// memberEntityTables is the closed polymorphic target set — the table
// name doubles as the RBAC object and the visibility-probe table. It is
// derived from the canonical record vocabulary rather than restated, so a
// new record type reaches lists, tags and saved views by widening one set.
var memberEntityTables = func() map[string]bool {
	m := map[string]bool{}
	for _, t := range datasource.RecordTypes() {
		m[string(t)] = true
	}
	return m
}()

// entityTypeField names the input every polymorphic refusal on this surface
// points at.
const entityTypeField = "entity_type"

// definitionField names the body key a dynamic list carries its filter tree
// under — the counterpart to a saved view's viewQueryField, and the reason a
// decode refusal takes the name from its caller rather than assuming one.
const definitionField = "definition"

// entityIDField names its sibling — the polymorphic TARGET a member or a tag
// application points at. Named beside entity_type because the two travel
// together in every body on this surface, and a refusal has to name the wire
// path, never prose.
const entityIDField = "entity_id"

// A list's own columns, as its reads, writes, patches and refusals spell them.
const (
	listIDField    = "list_id"
	listTypeField  = "list_type"
	purposeField   = "purpose"
	teamIDField    = "team_id"
	stewardIDField = "steward_id"
)

// memberEntityVocabulary renders the accepted set for the refusal message.
// Derived from the same map the check uses, because a message that restates
// the vocabulary drifts from it silently — the caller is then told a record
// type is invalid while being shown a list that does not include it.
var memberEntityVocabulary = func() string {
	names := make([]string, 0, len(memberEntityTables))
	for name := range memberEntityTables {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}()

// The three sharing settings a list may carry (list_sharing_check). Team is
// the default: a list is made to be worked from together, and nobody should
// have to find a setting to make it so.
const (
	sharingPrivate   = "private"
	sharingTeam      = "team"
	sharingWorkspace = "workspace"
)

// listObject is the list's RBAC object, audit entity type and table, and
// sharingField the input and audit key of who may find it.
const (
	listObject   = "list"
	sharingField = "sharing"
)

const listColumns = `l.id, l.name, l.entity_type, l.list_type, l.definition, l.owner_id, l.team_id,
	l.purpose, l.steward_id, l.sharing, l.version, l.created_at, l.updated_at, l.archived_at,
	(SELECT u.display_name FROM app_user u WHERE u.id = l.steward_id)`

// selectList reads one list row by its id, bound through listByID.
const selectList = "SELECT " + listColumns + " FROM list l WHERE l.id = @id"

func listByID(id ids.ListID) pgx.StrictNamedArgs { return pgx.StrictNamedArgs{"id": id} }

// catalogCap bounds the un-paginated catalog reads. Lists and tags are
// workspace-curated vocabulary — tens of rows, not record data — which
// is why the contract defines no cursor for them. The cap keeps a runaway
// workspace from turning the catalog read into an export; truncation is
// reported through the page flag, never silently.
const catalogCap = 1000

type listRow struct {
	ID         ids.ListID
	Name       string
	EntityType string
	ListType   string
	Definition map[string]any
	OwnerID    *ids.UserID
	TeamID     *ids.TeamID
	Purpose    *string
	StewardID  *ids.UserID
	Sharing    string
	Version    int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ArchivedAt *time.Time
	// StewardName is the steward's display name.
	StewardName *string
}

func scanList(r pgx.Row) (listRow, error) {
	var l listRow
	err := r.Scan(&l.ID, &l.Name, &l.EntityType, &l.ListType, &l.Definition, &l.OwnerID, &l.TeamID,
		&l.Purpose, &l.StewardID, &l.Sharing, &l.Version, &l.CreatedAt, &l.UpdatedAt, &l.ArchivedAt,
		&l.StewardName)
	return l, err
}

// ListFilter narrows the list library read.
type ListFilter struct {
	EntityType *string
	ListType   *string
	// Query matches name or purpose, case-insensitively.
	Query    *string
	Archived storekit.ArchivedFilter
}

// ListLists reads the lists this caller may find, by name.
func (s *Store) ListLists(ctx context.Context, filter ListFilter) ([]listRow, bool, error) {
	if err := auth.Require(ctx, listObject, principal.ActionRead); err != nil {
		return nil, false, err
	}
	var out []listRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		where := []string{"true"}
		if filter.EntityType != nil {
			where = append(where, fmt.Sprintf("l.entity_type = $%d", arg(*filter.EntityType)))
		}
		if filter.ListType != nil {
			where = append(where, fmt.Sprintf("l.list_type = $%d", arg(*filter.ListType)))
		}
		if filter.Query != nil && *filter.Query != "" {
			pattern := arg("%" + storekit.EscapeLike(*filter.Query) + "%")
			where = append(where, fmt.Sprintf("(l.name ILIKE $%[1]d OR l.purpose ILIKE $%[1]d)", pattern))
		}
		if filter.Archived != storekit.IncludeArchived {
			where = append(where, "l.archived_at IS NULL")
		}
		scope, err := auth.ScopeClauseFor(ctx, listObject, "l", arg)
		if err != nil {
			return err
		}
		if scope != "" {
			where = append(where, scope)
		}
		rows, err := tx.Query(ctx,
			"SELECT "+listColumns+" FROM list l WHERE "+strings.Join(where, " AND ")+
				fmt.Sprintf(" ORDER BY l.name, l.id LIMIT $%d", arg(catalogCap+1)), args...)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (listRow, error) { return scanList(row) })
		return err
	})
	if err != nil {
		return nil, false, err
	}
	if len(out) > catalogCap {
		return out[:catalogCap], true, nil
	}
	return out, false, nil
}

type CreateListInput struct {
	Name       string
	EntityType string
	ListType   string
	Definition map[string]any
	OwnerID    *ids.UserID
	TeamID     *ids.TeamID
	Purpose    *string
	// Sharing defaults to team; StewardID to the creator.
	Sharing   string
	StewardID *ids.UserID
}

func (s *Store) CreateList(ctx context.Context, in CreateListInput) (listRow, error) {
	if err := auth.Require(ctx, listObject, principal.ActionCreate); err != nil {
		return listRow{}, err
	}
	if err := s.checkNewList(ctx, &in); err != nil {
		return listRow{}, err
	}
	var out listRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var id ids.ListID
		if err := tx.QueryRow(ctx, `
			INSERT INTO list (name, entity_type, list_type, definition, owner_id, team_id, purpose, sharing, steward_id)
			VALUES (@name, @entity_type, @list_type, @definition, @owner_id, @team_id, @purpose, @sharing, @steward_id)
			RETURNING id`, pgx.StrictNamedArgs{
			nameField: in.Name, entityTypeField: in.EntityType, listTypeField: in.ListType, definitionField: in.Definition,
			ownerIDField: in.OwnerID, teamIDField: in.TeamID, purposeField: in.Purpose, sharingField: in.Sharing,
			stewardIDField: in.StewardID,
		}).Scan(&id); err != nil {
			return err
		}
		var err error
		if out, err = scanList(tx.QueryRow(ctx, selectList, listByID(id))); err != nil {
			return err
		}
		if err := writeRevision(ctx, tx, out); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "create", listObject, out.ID.UUID, nil, map[string]any{
			nameField: out.Name, entityTypeField: out.EntityType, listTypeField: out.ListType, sharingField: out.Sharing,
		})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, out.ID.UUID, crmcontracts.PublicEventListCreated{
			RecordType: out.EntityType, ListType: out.ListType, Sharing: out.Sharing,
		})
	})
	return out, err
}

// checkNewList fills a new list's defaults and refuses a list that could not
// be evaluated or found: an unknown record type, a definition on a Shortlist
// or none on a Live List, a filter the engine cannot compile, an unknown
// sharing setting.
func (s *Store) checkNewList(ctx context.Context, in *CreateListInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return &BadInputError{Field: nameField, Reason: "a list needs a name"}
	}
	if !memberEntityTables[in.EntityType] {
		return &BadInputError{Field: entityTypeField, Reason: "must be " + memberEntityVocabulary}
	}
	if in.ListType == "" {
		in.ListType = listTypeStatic
	}
	if in.Sharing == "" {
		in.Sharing = sharingTeam
	}
	if err := checkSharing(in.Sharing); err != nil {
		return err
	}
	if in.StewardID == nil {
		in.StewardID = storekit.OwnerOrActor(ctx, nil)
	}
	if in.OwnerID == nil {
		in.OwnerID = storekit.OwnerOrActor(ctx, nil)
	}
	switch in.ListType {
	case listTypeDynamic:
		if len(in.Definition) == 0 {
			return &BadInputError{Field: definitionField, Reason: "a Live List needs a filter definition"}
		}
		return s.validateSegmentDefinition(ctx, in.EntityType, in.Definition)
	case listTypeStatic:
		if len(in.Definition) > 0 {
			return &BadInputError{Field: definitionField, Reason: "a Shortlist carries no filter definition"}
		}
		return nil
	default:
		return &BadInputError{Field: listTypeField, Reason: "must be static|dynamic"}
	}
}

func checkSharing(sharing string) error {
	switch sharing {
	case sharingPrivate, sharingTeam, sharingWorkspace:
		return nil
	default:
		return &BadInputError{Field: sharingField, Reason: "must be private|team|workspace"}
	}
}

// GetList reads one list the caller may find; one they may not answers
// ErrNotFound, the same as one that does not exist.
func (s *Store) GetList(ctx context.Context, id ids.ListID) (listRow, error) {
	if err := auth.Require(ctx, listObject, principal.ActionRead); err != nil {
		return listRow{}, err
	}
	var out listRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = readVisibleList(ctx, tx, id)
		return err
	})
	return out, err
}

// readVisibleList is the list read every entry point takes inside its own
// transaction: the sharing probe, then the row.
func readVisibleList(ctx context.Context, tx pgx.Tx, id ids.ListID) (listRow, error) {
	if err := ensureListVisible(ctx, tx, id); err != nil {
		return listRow{}, err
	}
	out, err := scanList(tx.QueryRow(ctx, selectList, listByID(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return listRow{}, apperrors.ErrNotFound
	}
	return out, err
}

// validateSegmentDefinition proves a dynamic list's definition is an evaluable
// filter over the entity's closed vocabulary — core fields, this workspace's
// custom columns, and tags — before it is stored: it compiles the predicate
// (discarding the SQL) so an unknown field, a mistyped value, or an
// over-deep/over-wide tree fails as a PredicateError the transport maps to 422.
func (s *Store) validateSegmentDefinition(ctx context.Context, entityType string, definition map[string]any) error {
	engine, ok, err := s.SegmentEngine(ctx, entityType)
	if err != nil {
		return err
	}
	if !ok {
		return &BadInputError{Field: entityTypeField, Reason: "no dynamic segment engine for " + entityType}
	}
	return compileForValidation(engine, definition, definitionField)
}

// compileForValidation is the proof itself, shared by the two stored filters
// this module accepts — a dynamic list's definition and a saved view's filter
// state. It decodes the tree and compiles it against the resolved vocabulary,
// throwing the SQL away: the point is the refusal, not the statement. An
// unknown field, a mistyped value, or an over-deep/over-wide tree comes back as
// a PredicateError the transport maps to 422.
//
// Two things differ between the callers and both are parameters rather than
// branches here. The ENGINE RESOLUTION, which is why the split falls at this
// seam: a list names its entity type outright and has none if the type is not
// filterable, while a view names a plural resource that may have no engine at
// all and legitimately carry no filter. And the WIRE FIELD a decode failure
// names, because the two surfaces carry the tree under different keys.
func compileForValidation(engine storekit.Query, tree map[string]any, field string) error {
	pred, err := predicateFromDefinition(tree)
	if err != nil {
		// Dressed as the caller's own field, because on THIS path the caller
		// did send the tree. Anything else passes through untouched: the
		// tree's shape is the only part of a refusal a caller can act on.
		if errors.Is(err, errNotAFilterTree) {
			return &BadInputError{Field: field, Reason: "is not a valid filter tree"}
		}
		return err
	}
	// Refused BEFORE the compile, and only here. A picklist leaf compares
	// text, so a value outside the field's set compiles fine and selects
	// nothing — an honest answer for a value no row carries, and the wrong one
	// for the contact who just typed it, whose equivalent list URL would have
	// been told. Evaluation stays permissive on purpose: a definition stored
	// last quarter must not begin failing because somebody removed a value
	// from a custom field's option set since.
	if err := storekit.RefuseUnknownPicklistValues(pred, engine.Fields); err != nil {
		return err
	}
	discard := 0
	arg := func(any) int { discard++; return discard }
	_, err = storekit.CompilePredicate(pred, engine.Fields, arg)
	return err
}

// ensureListVisible is the list's sharing probe: whether this caller may find
// the list (platform/auth's list sharing predicate).
func ensureListVisible(ctx context.Context, tx pgx.Tx, id ids.ListID) error {
	return auth.EnsureVisible(ctx, tx, listObject, id.UUID)
}

// BadInputError maps to a 422 at the transport.
type BadInputError struct {
	Field  string
	Reason string
}

func (e *BadInputError) Error() string { return "collections: " + e.Field + ": " + e.Reason }

// FieldFault names the list or tag argument that was rejected.
func (e *BadInputError) FieldFault() (field, code, message string) {
	return e.Field, "invalid", e.Reason
}
