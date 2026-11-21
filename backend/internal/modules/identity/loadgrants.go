// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// What a seat may DO, loaded once per authentication.
//
// Split out of service.go, which held both the session lifecycle — login,
// authenticate, logout — and the grant resolution those three each end by
// calling. They are different subjects: one is about proving who is asking, the
// other about what the answer entitles them to, and the file had grown past the
// length cap holding both.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func loadGrants(ctx context.Context, tx pgx.Tx, userID ids.UserID) (roles []string, teams []ids.TeamID, perms principal.Permissions, err error) {
	all, err := loadGrantsFor(ctx, tx, []ids.UUID{userID.UUID})
	if err != nil {
		return nil, nil, principal.Permissions{}, err
	}
	g := all[userID.UUID]
	return g.roles, g.teams, g.perms, nil
}

// seatGrants is one seat's roles, live teams and merged permissions.
type seatGrants struct {
	roles []string
	teams []ids.TeamID
	perms principal.Permissions
}

// loadGrantsFor resolves several seats in one batch of set-based reads.
// loadGrants is this with one seat, so a seat resolves the same way whichever
// door asks. A seat with no role assignment gets the zero grants.
func loadGrantsFor(ctx context.Context, tx pgx.Tx, users []ids.UUID) (map[ids.UUID]seatGrants, error) {
	read, err := loadSeatReads(ctx, tx, users)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]seatGrants, len(users))
	for _, user := range users {
		g := seatGrants{roles: read.roles[user], teams: read.teams[user], perms: policy.Merge(read.docs[user])}
		g.perms.FieldMasks = read.masks[user]
		out[user] = g
	}
	return out, nil
}

// seatReads is what loadSeatReads answers, keyed by seat.
type seatReads struct {
	docs  map[ids.UUID]map[string]policy.Document
	roles map[ids.UUID][]string
	teams map[ids.UUID][]ids.TeamID
	masks map[ids.UUID][]principal.FieldMask
}

// loadSeatReads sends the role and team reads in one exchange: neither needs
// the other's answer, so a seat's grants cost one round trip, not two.
func loadSeatReads(ctx context.Context, tx pgx.Tx, users []ids.UUID) (read seatReads, err error) {
	var batch pgx.Batch
	batch.Queue(roleDocumentsSQL, users)
	batch.Queue(liveTeamsSQL, users)
	results := tx.SendBatch(ctx, &batch)
	defer func() { err = errors.Join(err, results.Close()) }()
	roleRows, err := results.Query()
	if err != nil {
		return seatReads{}, err
	}
	if read.docs, read.roles, read.masks, err = scanRoleDocuments(roleRows); err != nil {
		return seatReads{}, err
	}
	teamRows, err := results.Query()
	if err != nil {
		return seatReads{}, err
	}
	if read.teams, err = scanLiveTeams(teamRows); err != nil {
		return seatReads{}, err
	}
	return read, nil
}

// roleDocumentsSQL reads live roles only: an archived role grants nothing.
// Archiving refuses while anybody who can sign in holds the role, so what this
// drops is the role a deactivated member still carries, and reactivating them
// does not bring an archived role's grants back with them.
//
// Each row also carries its seat's field masks, the union loadFieldMasks reads
// over those live roles. One statement is one snapshot, so the masks always
// belong to the roles read beside them.
const roleDocumentsSQL = `SELECT ra.user_id, r.key, r.permissions,
    (SELECT coalesce(json_agg(json_build_array(m.object, m.field, m.condition) ORDER BY m.object, m.field, m.condition), '[]')
       FROM (SELECT DISTINCT fm.object, fm.field, fm.condition
               FROM role_assignment sa JOIN role sr ON sr.id = sa.role_id JOIN field_mask fm ON fm.role_key = sr.key
              WHERE sa.user_id = ra.user_id AND sr.archived_at IS NULL) m)
  FROM role_assignment ra JOIN role r ON r.id = ra.role_id
  WHERE ra.user_id = ANY($1) AND r.archived_at IS NULL`

func loadRoleDocuments(ctx context.Context, tx pgx.Tx, users []ids.UUID) (map[ids.UUID]map[string]policy.Document, map[ids.UUID][]string, error) {
	rows, err := tx.Query(ctx, roleDocumentsSQL, users)
	if err != nil {
		return nil, nil, err
	}
	docs, roles, _, err := scanRoleDocuments(rows)
	return docs, roles, err
}

func scanRoleDocuments(rows pgx.Rows) (map[ids.UUID]map[string]policy.Document, map[ids.UUID][]string, map[ids.UUID][]principal.FieldMask, error) {
	defer rows.Close()
	docs := map[ids.UUID]map[string]policy.Document{}
	roles := map[ids.UUID][]string{}
	masks := map[ids.UUID][]principal.FieldMask{}
	for rows.Next() {
		var user ids.UUID
		var key string
		var raw, rawMasks []byte
		if err := rows.Scan(&user, &key, &raw, &rawMasks); err != nil {
			return nil, nil, nil, err
		}
		doc, err := policy.Parse(raw)
		if err != nil {
			// A role carrying an UNREADABLE policy document is a data defect
			// the login must surface, not silently downgrade to no access.
			//
			// "Unreadable" is now a much narrower set than it was: malformed
			// JSON, or a row_scope nothing can interpret. An object this
			// installation does not know is dropped by Parse with a log line
			// instead of failing here — because failing here failed the whole
			// LOGIN, so removing a composed extension locked out every user
			// whose role still carried its object (Task 14 UAT, F4).
			return nil, nil, nil, fmt.Errorf("crmauth: role %q: %w", key, err)
		}
		if docs[user] == nil {
			docs[user] = map[string]policy.Document{}
			if masks[user], err = decodeSeatMasks(rawMasks); err != nil {
				return nil, nil, nil, err
			}
		}
		roles[user] = append(roles[user], key)
		docs[user][key] = doc
	}
	return docs, roles, masks, rows.Err()
}

// decodeSeatMasks reads the [object, field, condition] triples roleDocumentsSQL
// aggregates. An empty union stays nil, as a seat with no mask always had it.
func decodeSeatMasks(raw []byte) ([]principal.FieldMask, error) {
	var triples [][3]string
	if err := json.Unmarshal(raw, &triples); err != nil {
		return nil, fmt.Errorf("crmauth: reading field masks: %w", err)
	}
	var out []principal.FieldMask
	for _, t := range triples {
		out = append(out, principal.FieldMask{Object: t[0], Field: t[1], Condition: principal.MaskCondition(t[2])})
	}
	return out, nil
}

// liveTeamsSQL reads live teams only. An archived team keeps its membership
// rows so a restore brings them back. While archived it resolves neither
// row scope nor a team share.
const liveTeamsSQL = `SELECT tm.user_id, tm.team_id FROM team_membership tm JOIN team t ON t.id = tm.team_id AND t.archived_at IS NULL
  WHERE tm.user_id = ANY($1)`

func loadLiveTeams(ctx context.Context, tx pgx.Tx, users []ids.UUID) (map[ids.UUID][]ids.TeamID, error) {
	rows, err := tx.Query(ctx, liveTeamsSQL, users)
	if err != nil {
		return nil, err
	}
	return scanLiveTeams(rows)
}

func scanLiveTeams(rows pgx.Rows) (map[ids.UUID][]ids.TeamID, error) {
	defer rows.Close()
	out := map[ids.UUID][]ids.TeamID{}
	for rows.Next() {
		var user ids.UUID
		var team ids.TeamID
		if err := rows.Scan(&user, &team); err != nil {
			return nil, err
		}
		out[user] = append(out[user], team)
	}
	return out, rows.Err()
}

// rawTeamIDs widens typed team ids to the untyped []ids.UUID the kernel
// principal and the authz port carry — the row-scope seams stay untyped
// (they compare team membership against polymorphic scope clauses).
func rawTeamIDs(teams []ids.TeamID) []ids.UUID {
	if teams == nil {
		return nil
	}
	out := make([]ids.UUID, len(teams))
	for i, t := range teams {
		out[i] = t.UUID
	}
	return out
}

// SeatAllows answers whether one seat's effective grants admit an action on an
// object, without asking anything about the CALLER.
//
// The other readers here answer about the principal, because almost every
// question is "may I". This one is "may THEY", which the lead-escalation seat
// needs: a desk is being nominated to receive escalations that carry the record
// they are about, and a seat that cannot open a lead is not a desk to send one
// to. Nominating is an admin's act and the caller's own authority is checked by
// the surface that offers it, so this reads grants and nothing else.
//
// A seat with no role assignment allows nothing, which is the honest answer for
// a user who holds none.
func SeatAllows(ctx context.Context, tx pgx.Tx, seat ids.UserID, object string, action principal.Action) (bool, error) {
	_, _, perms, err := loadGrants(ctx, tx, seat)
	if err != nil {
		return false, err
	}
	return perms.Allows(object, action), nil
}
