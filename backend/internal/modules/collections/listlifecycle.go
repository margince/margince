// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// UpdateListInput changes what a list is. Every change is guarded by the
// version the caller read, because a list colleagues work from must not be
// rewritten over an edit its author never saw.
type UpdateListInput struct {
	Name       *string
	Purpose    *string
	Definition map[string]any
	Sharing    *string
	TeamID     *ids.TeamID
	StewardID  *ids.UserID
	// ClearPurpose and ClearTeam distinguish "remove it" from "leave it".
	ClearPurpose bool
	ClearTeam    bool
	IfVersion    *int64
}

// ErrListArchived is a change to an archived list, which is read-only until
// it is restored.
var ErrListArchived = fmt.Errorf("an archived list is read-only: restore it first: %w", apperrors.ErrConflict)

// UpdateList changes a list's name, purpose, filter, sharing, team or steward,
// and records the new definition as a revision.
func (s *Store) UpdateList(ctx context.Context, id ids.ListID, in UpdateListInput) (listRow, error) {
	if err := s.checkUpdate(ctx, id, &in); err != nil {
		return listRow{}, err
	}
	var out listRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, _, err := editableList(ctx, tx, id)
		if err != nil {
			return err
		}
		if current.ArchivedAt != nil {
			return ErrListArchived
		}
		p := listPatch(current, in)
		if p.Empty() {
			out = current
			return nil
		}
		if err := p.ApplyGuarded(ctx, tx, listObject, id.UUID, in.IfVersion); err != nil {
			return err
		}
		if out, err = scanList(tx.QueryRow(ctx, selectList, listByID(id))); err != nil {
			return err
		}
		if err := writeRevision(ctx, tx, out); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", listObject, id.UUID, p.Before(), p.After())
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventListUpdated{
			Version: out.Version, Changed: changedKeys(p.After()),
		})
	})
	return out, err
}

// checkUpdate refuses an update before its transaction opens: no version, a
// blank name, an unknown sharing, or a filter the list cannot hold. It stores
// the trimmed name, as a new list does. The filter is judged against the
// list's own record type here because SegmentEngine reads the custom-field
// catalogue on its own connection.
func (s *Store) checkUpdate(ctx context.Context, id ids.ListID, in *UpdateListInput) error {
	if err := auth.Require(ctx, listObject, principal.ActionUpdate); err != nil {
		return err
	}
	if in.IfVersion == nil {
		return &BadInputError{Field: versionField, Reason: "name the version you read, so a change nobody saw is not overwritten"}
	}
	if in.Name != nil {
		name, err := httperr.RequireNonBlank(nameField, *in.Name)
		if err != nil {
			return err
		}
		in.Name = &name
	}
	if in.Sharing != nil {
		if err := checkSharing(*in.Sharing); err != nil {
			return err
		}
	}
	if in.Definition == nil {
		return nil
	}
	current, err := s.GetList(ctx, id)
	if err != nil {
		return err
	}
	if current.ListType != listTypeDynamic {
		return &BadInputError{Field: definitionField, Reason: "a Shortlist carries no filter definition"}
	}
	return s.validateSegmentDefinition(ctx, current.EntityType, in.Definition)
}

// listPatch is the columns an update changes, against the row it read.
func listPatch(current listRow, in UpdateListInput) *storekit.Patch {
	p := storekit.NewPatch()
	if in.Name != nil && *in.Name != current.Name {
		p.Set(nameField, current.Name, *in.Name)
	}
	switch {
	case in.ClearPurpose && current.Purpose != nil:
		p.Set(purposeField, current.Purpose, nil)
	case in.Purpose != nil:
		p.Set(purposeField, current.Purpose, *in.Purpose)
	}
	if in.Definition != nil {
		p.Set(definitionField, current.Definition, in.Definition)
	}
	if in.Sharing != nil && *in.Sharing != current.Sharing {
		p.Set(sharingField, current.Sharing, *in.Sharing)
	}
	switch {
	case in.ClearTeam && current.TeamID != nil:
		p.Set(teamIDField, current.TeamID, nil)
	case in.TeamID != nil:
		p.Set(teamIDField, current.TeamID, *in.TeamID)
	}
	if in.StewardID != nil {
		p.Set(stewardIDField, current.StewardID, *in.StewardID)
	}
	return p
}

func changedKeys(after map[string]any) []string {
	keys := make([]string, 0, len(after))
	for _, key := range []string{nameField, purposeField, definitionField, sharingField, teamIDField, stewardIDField} {
		if _, ok := after[key]; ok {
			keys = append(keys, key)
		}
	}
	return keys
}

// ArchiveList retires a list. Its members and history stay, and a restore
// brings it back as it was.
func (s *Store) ArchiveList(ctx context.Context, id ids.ListID) (listRow, error) {
	return s.setArchived(ctx, id, true)
}

// RestoreList brings an archived list back.
func (s *Store) RestoreList(ctx context.Context, id ids.ListID) (listRow, error) {
	return s.setArchived(ctx, id, false)
}

func (s *Store) setArchived(ctx context.Context, id ids.ListID, archive bool) (listRow, error) {
	if err := auth.Require(ctx, listObject, principal.ActionDelete); err != nil {
		return listRow{}, err
	}
	var out listRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, lock, err := editableList(ctx, tx, id)
		if err != nil {
			return err
		}
		if (current.ArchivedAt != nil) == archive {
			if archive {
				return apperrors.ErrNotFound
			}
			return fmt.Errorf("the list is not archived: %w", apperrors.ErrConflict)
		}
		p := storekit.NewPatch()
		if archive {
			p.Set("archived_at", nil, time.Now())
		} else {
			p.Set("archived_at", current.ArchivedAt, nil)
		}
		if err := p.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		if out, err = scanList(tx.QueryRow(ctx, selectList, listByID(id))); err != nil {
			return err
		}
		return auditArchiveChange(ctx, tx, id, archive, p)
	})
	return out, err
}

func auditArchiveChange(ctx context.Context, tx pgx.Tx, id ids.ListID, archive bool, p *storekit.Patch) error {
	if archive {
		auditID, err := storekit.Audit(ctx, tx, "archive", listObject, id.UUID, nil, nil)
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventListArchived{})
	}
	auditID, err := storekit.Audit(ctx, tx, "restore", listObject, id.UUID, p.Before(), p.After())
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventListRestored{})
}

// editableList locks a list the caller may find AND change. Finding a list is
// its sharing; changing one is list authority: its steward, or a seat that
// reads every row with the list update grant (a list admin), or, while it has
// no steward, its owner. Someone who may find a list but not change it is
// refused with a 403, because the list itself is no secret to them.
func editableList(ctx context.Context, tx pgx.Tx, id ids.ListID) (listRow, storekit.RowLock, error) {
	if _, err := readVisibleList(ctx, tx, id); err != nil {
		return listRow{}, storekit.RowLock{}, err
	}
	lock, err := storekit.LockRow(ctx, tx, listObject, id.UUID, storekit.IncludeArchived)
	if err != nil {
		return listRow{}, storekit.RowLock{}, err
	}
	// Read again under the lock, so the row judged is the row changed.
	current, err := scanList(tx.QueryRow(ctx, selectList, listByID(id)))
	if err != nil {
		return listRow{}, storekit.RowLock{}, err
	}
	if !mayEditList(ctx, current) {
		return listRow{}, storekit.RowLock{}, fmt.Errorf("only the list's steward or a list admin may change it: %w", apperrors.ErrPermissionDenied)
	}
	return current, lock, nil
}

func mayEditList(ctx context.Context, l listRow) bool {
	p, ok := principal.Actor(ctx)
	if !ok {
		return false
	}
	if auth.Unbounded(p) {
		return true
	}
	me := p.UserID
	if l.StewardID != nil {
		return l.StewardID.UUID == me
	}
	return l.OwnerID != nil && l.OwnerID.UUID == me
}

// writeRevision records what a list is from its current version on.
func writeRevision(ctx context.Context, tx pgx.Tx, l listRow) error {
	actor, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO list_revision (list_id, version, name, purpose, definition, sharing, team_id, steward_id, changed_by)
		VALUES (@list_id, @version, @name, @purpose, @definition, @sharing, @team_id, @steward_id, @changed_by)`,
		pgx.StrictNamedArgs{
			listIDField: l.ID, versionField: l.Version, nameField: l.Name, purposeField: l.Purpose, definitionField: l.Definition,
			sharingField: l.Sharing, teamIDField: l.TeamID, stewardIDField: l.StewardID, "changed_by": actor,
		})
	if err != nil {
		return fmt.Errorf("record list revision: %w", err)
	}
	return nil
}
