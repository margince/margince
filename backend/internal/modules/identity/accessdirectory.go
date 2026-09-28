// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// What a "who can see this record" answer needs from identity: every live
// member, the live shares on the record, and — for a caller who administers
// members — role keys and team names. It walks the roster, team and grant reads
// under the caller's own gates rather than reading the tables again, so the
// disclosure boundaries are theirs.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AccessMember is one live member as the directory lists them. Roles is nil
// unless the directory is the management view.
type AccessMember struct {
	ID          ids.UUID
	DisplayName string
	CreatedAt   time.Time
	Roles       []string
}

// AccessShare is one live share on the record.
type AccessShare struct {
	SubjectType string
	SubjectID   ids.UUID
	Access      string
	ExpiresAt   *time.Time
}

// AccessDirectory is the whole directory for one record.
type AccessDirectory struct {
	Members []AccessMember
	Shares  []AccessShare
	// Management is ListUsers' own answer to whether this caller gets role keys
	// and team memberships; TeamNames is filled only when it is true.
	Management bool
	TeamNames  map[ids.UUID]string
	// OwnerTeams are the teams the record's owner is a member of, whatever
	// the owner's own status: the team-scope write arm reads the membership
	// rows and not whether the owner can still sign in.
	OwnerTeams []ids.UUID
}

// directoryPage is the largest page the roster reads serve, so the walk takes
// as few round trips as the contract allows.
const directoryPage = 200

// RecordAccessDirectory reads the directory for one record. The caller must be
// able to read the record for its shares to be listed; ListRecordGrants drops
// the ones it could not.
func (s *Service) RecordAccessDirectory(
	ctx context.Context, recordType string, recordID ids.UUID, owner *ids.UUID,
) (AccessDirectory, error) {
	var out AccessDirectory
	if err := s.walkRoster(ctx, &out); err != nil {
		return AccessDirectory{}, err
	}
	if out.Management {
		names, err := s.teamNames(ctx)
		if err != nil {
			return AccessDirectory{}, err
		}
		out.TeamNames = names
	}
	shares, err := s.recordShares(ctx, recordType, recordID)
	if err != nil {
		return AccessDirectory{}, err
	}
	out.Shares = shares
	if owner != nil {
		if out.OwnerTeams, err = s.membershipTeams(ctx, *owner); err != nil {
			return AccessDirectory{}, err
		}
	}
	return out, nil
}

func (s *Service) membershipTeams(ctx context.Context, user ids.UUID) ([]ids.UUID, error) {
	var out []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT team_id FROM team_membership WHERE user_id = $1`, user)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	return out, err
}

func (s *Service) walkRoster(ctx context.Context, out *AccessDirectory) error {
	limit := directoryPage
	var cursor *string
	for {
		page, err := s.ListUsers(ctx, ListUsersInput{Cursor: cursor, Limit: &limit})
		if err != nil {
			return err
		}
		out.Management = page.Management
		for _, u := range page.Users {
			out.Members = append(out.Members, AccessMember{
				ID: u.ID, DisplayName: u.DisplayName, CreatedAt: u.CreatedAt, Roles: u.Roles,
			})
		}
		if !page.Page.HasMore {
			return nil
		}
		cursor = &page.Page.NextCursor
	}
}

func (s *Service) teamNames(ctx context.Context) (map[ids.UUID]string, error) {
	names := map[ids.UUID]string{}
	limit := directoryPage
	var cursor *string
	for {
		teams, page, err := s.ListTeams(ctx, ListTeamsInput{Cursor: cursor, Limit: &limit})
		if err != nil {
			return nil, err
		}
		for _, team := range teams {
			names[team.ID] = team.Name
		}
		if !page.HasMore {
			return names, nil
		}
		cursor = &page.NextCursor
	}
}

func (s *Service) recordShares(ctx context.Context, recordType string, recordID ids.UUID) ([]AccessShare, error) {
	var out []AccessShare
	limit := directoryPage
	var cursor *string
	for {
		grants, page, err := s.ListRecordGrants(ctx, ListGrantsInput{
			RecordType: &recordType, RecordID: &recordID, Cursor: cursor, Limit: &limit,
		})
		if err != nil {
			return nil, err
		}
		for _, g := range grants {
			out = append(out, AccessShare{
				SubjectType: g.SubjectType, SubjectID: g.SubjectID, Access: g.Access, ExpiresAt: g.ExpiresAt,
			})
		}
		if !page.HasMore {
			return out, nil
		}
		cursor = &page.NextCursor
	}
}
