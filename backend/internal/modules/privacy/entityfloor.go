// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The correspondence floor of each further legal entity. A message is kept
// under the floor of the company its users work for. Those are the colleague
// who sent it, the owners of the mailboxes it arrived in and the seat that
// logged it. A message that names none of them takes the owners of the
// contacts and deals it is filed under. A message that names nobody keeps the
// default entity's floor, which is the installation's country.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// entityFloorsAt reads the floor of every live further entity whose floor is
// stricter than the default one. A weaker one is left out, because a message
// never keeps less than the default entity's floor.
func entityFloorsAt(ctx context.Context, tx pgx.Tx, current jurisdiction.RetentionClass, ref time.Time) (map[ids.UUID]jurisdiction.RetentionClass, error) {
	rows, err := tx.Query(ctx, `SELECT id, country FROM legal_entity
		 WHERE NOT is_default AND archived_at IS NULL AND country IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("privacy: reading the legal entities' countries for the retention floor: %w", err)
	}
	type entityCountry struct {
		id      ids.UUID
		country jurisdiction.Code
	}
	entities, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (entityCountry, error) {
		var e entityCountry
		return e, row.Scan(&e.id, &e.country)
	})
	if err != nil {
		return nil, fmt.Errorf("privacy: reading the legal entities' countries for the retention floor: %w", err)
	}
	var out map[ids.UUID]jurisdiction.RetentionClass
	for _, e := range entities {
		floor := countryFloor(e.country, ref)
		if !stricterThan(floor, current, ref) {
			continue
		}
		if out == nil {
			out = map[ids.UUID]jurisdiction.RetentionClass{}
		}
		out[e.id] = floor
	}
	return out, nil
}

// countryFloor is a further entity's own floor: its country's pack, or the
// strictest compiled-in floor when no pack is compiled in for it.
func countryFloor(country jurisdiction.Code, ref time.Time) jurisdiction.RetentionClass {
	if pack, ok := jurisdiction.For(country); ok {
		return correspondenceClassOf(pack)
	}
	return statutoryCorrespondenceFloor(ref)
}

func stricterThan(class, than jurisdiction.RetentionClass, ref time.Time) bool {
	return class.ProtectedSince(ref).Before(than.ProtectedSince(ref))
}

// EntityJurisdiction is what decides a legal entity's floor: whether it is
// the default entity, which follows the installation, and its own country.
type EntityJurisdiction struct {
	Default bool
	Country jurisdiction.Code
}

// CarryOnEntityChangeTx keeps the floor on mail already held when a user
// moves from one entity to another, or an entity changes its country. It
// runs the carry a jurisdiction change runs. Its two floors are what the
// members' mail had before and has after. Each is the stricter of the
// default floor and the entity's own.
func CarryOnEntityChangeTx(ctx context.Context, tx pgx.Tx, store *settings.Store, country CountryReader, from, to EntityJurisdiction, now time.Time) error {
	policy, err := resolveFloorPolicyAt(ctx, tx, country, now)
	if err != nil {
		return err
	}
	memberFloor := func(e EntityJurisdiction) jurisdiction.RetentionClass {
		if own := countryFloor(e.Country, now); !e.Default && stricterThan(own, policy.Current, now) {
			return own
		}
		return policy.Current
	}
	return carryWeakeningTx(ctx, tx, store, memberFloor(from), memberFloor(to), now)
}

// floorJSON is one floor in the shape windowEndOf reads.
func floorJSON(class jurisdiction.RetentionClass) string {
	return fmt.Sprintf(`"keep":%q,"year_end":%t`, class.Keep.String(), class.Anchor == jurisdiction.AnchorCalendarYearEnd)
}

// entitiesJSON renders the further entities' floors as the "entities" member
// of the current floor's argument, or nothing when every entity keeps the
// default floor. Sorted, so one policy always renders one text.
func entitiesJSON(byEntity map[ids.UUID]jurisdiction.RetentionClass) string {
	if len(byEntity) == 0 {
		return ""
	}
	keys := make([]string, 0, len(byEntity))
	floors := make(map[string]jurisdiction.RetentionClass, len(byEntity))
	for id, class := range byEntity {
		keys = append(keys, id.String())
		floors[id.String()] = class
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(`%q:{%s}`, key, floorJSON(floors[key])))
	}
	return `,"entities":{` + strings.Join(parts, ",") + `}`
}

// activityUsersSQL lists the users the activity aliased `a` belongs to: its
// sender, the owners of the mailboxes it arrived in and the seat that logged
// it. Only ids that name a user count, so an agent or connector stamp adds no
// entity.
const activityUsersSQL = `SELECT u.id FROM app_user u
		 WHERE u.id = a.sent_by_colleague
		    OR u.id IN (SELECT ci.user_id FROM capture_import ci WHERE ci.activity_id = a.id)
		    OR u.id::text = substring(a.captured_by from '([0-9a-f-]{36})$')`

// activityRecordOwnersSQL lists the owners of the contacts and deals the
// activity aliased `a` is filed under.
const activityRecordOwnersSQL = `SELECT c.owner_id FROM activity_link l JOIN contact c ON c.id = l.contact_id
		 WHERE l.activity_id = a.id AND c.owner_id IS NOT NULL
		 UNION SELECT d.owner_id FROM activity_link l JOIN deal d ON d.id = l.deal_id
		 WHERE l.activity_id = a.id AND d.owner_id IS NOT NULL`

// floorOwnersSQL lists the users whose entity decides the floor of the
// activity aliased `a`, as floor_owner(user_id). The record owners count only
// when the message names no users of its own.
var floorOwnersSQL = `((` + activityUsersSQL + `) UNION ALL (SELECT owners.owner_id FROM (` +
	activityRecordOwnersSQL + `) owners WHERE NOT EXISTS (` + activityUsersSQL + `))) floor_owner(user_id)`

// ownerWindowEnd is the current floor's window end over the activity aliased
// `a`. It is the latest end among the default floor and the stricter floors of
// the entities its floor owners work for.
func ownerWindowEnd(arg int) string {
	floors := fmt.Sprintf("$%d::jsonb", arg)
	return fmt.Sprintf(`CASE WHEN %[1]s->'entities' IS NULL THEN %[2]s ELSE GREATEST(%[2]s, (
		  SELECT max(%[3]s)
		    FROM %[4]s
		    CROSS JOIN LATERAL (SELECT coalesce(%[1]s->'entities'->((%[5]s)::text), %[1]s) AS floor) owner_floor
		   WHERE floor_owner.user_id IS NOT NULL)) END`,
		floors, windowEndOf(floors), windowEndOf("owner_floor.floor"),
		floorOwnersSQL, storekit.LegalEntityOfSQL("floor_owner.user_id"))
}

// ForMessagesTx narrows the policy to the entities the given messages belong
// to. A report about them then names the floor that kept them, and no other
// entity's.
func (p FloorPolicy) ForMessagesTx(ctx context.Context, tx pgx.Tx, activities []ids.UUID) (FloorPolicy, error) {
	if len(p.ByEntity) == 0 || len(activities) == 0 {
		out := p
		out.ByEntity = nil
		return out, nil
	}
	args := []any{activities}
	rows, err := tx.Query(ctx, `SELECT DISTINCT `+storekit.LegalEntityOfSQL("floor_owner.user_id")+`
		  FROM activity a CROSS JOIN LATERAL `+floorOwnersSQL+`
		 WHERE a.id = ANY(`+storekit.Placeholders(args)+`) AND floor_owner.user_id IS NOT NULL`, args...)
	if err != nil {
		return FloorPolicy{}, fmt.Errorf("privacy: reading the legal entities of the messages a floor kept: %w", err)
	}
	entities, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return FloorPolicy{}, fmt.Errorf("privacy: reading the legal entities of the messages a floor kept: %w", err)
	}
	out := p
	out.ByEntity = nil
	for _, entity := range entities {
		if class, ok := p.ByEntity[entity]; ok {
			if out.ByEntity == nil {
				out.ByEntity = map[ids.UUID]jurisdiction.RetentionClass{}
			}
			out.ByEntity[entity] = class
		}
	}
	return out, nil
}
