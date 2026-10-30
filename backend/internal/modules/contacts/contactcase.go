// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A contact case is every live contact reachable from one contact over open
// duplicate pairs whose both ends the reader can see. It is read off
// dedupe_candidate and never stored, so dismissing, re-opening or merging a
// pair re-shapes it at once. The lowest member id is its anchor.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// contactCaseMaxMembers is how many contacts one review may merge. A larger
// component still decides pair by pair.
const contactCaseMaxMembers = 25

type caseMember struct {
	ID        ids.UUID
	Name      string
	Email     *string
	Title     *string
	Company   *string
	OwnerID   *ids.UUID
	OwnerName *string
	// OwnerLive is a live human seat owning the member; OwnerSeat is its seat.
	OwnerLive bool
	OwnerSeat string
	Version   int64
	CreatedAt time.Time
}

type caseEdge struct {
	ID          ids.UUID
	Left, Right ids.UUID
	Confidence  float64
	Disposition string
	Evidence    json.RawMessage
}

type contactCase struct {
	// Anchor is the component's lowest member, whichever member addressed it.
	Anchor ids.UUID
	// Confidence is the strongest open pair in the component.
	Confidence float64
	Members    []caseMember
	Edges      []caseEdge
	Grants     []caseGrant
	TooLarge   bool
}

func (c contactCase) member(id ids.UUID) (caseMember, bool) {
	i := slices.IndexFunc(c.Members, func(m caseMember) bool { return m.ID == id })
	if i < 0 {
		return caseMember{}, false
	}
	return c.Members[i], true
}

// revision changes whenever an input of the merge's outcome changes. Those are
// each member, its version, owner and owner's seat, each pair, each grant on a
// member, and the policy. A decision refuses a stale one.
func (c contactCase) revision(policy bool) string {
	var b strings.Builder
	for _, m := range c.Members {
		owner := ""
		if m.OwnerID != nil {
			owner = m.OwnerID.String()
		}
		fmt.Fprintf(&b, "m:%s:%d:%s:%t:%s\n", m.ID, m.Version, owner, m.OwnerLive, m.OwnerSeat)
	}
	for _, e := range c.Edges {
		b.WriteString("p:" + e.ID.String() + ":" + e.Disposition + "\n")
	}
	for _, g := range c.Grants {
		fmt.Fprintf(&b, "g:%s:%d:%s:%t\n", g.ID, g.Version, g.Access, g.Live)
	}
	b.WriteString("policy:" + strconv.FormatBool(policy) + "\n")
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// liveOwners are the distinct live owners of the given members, in id order.
func (c contactCase) liveOwners(memberIDs []ids.UUID) []ids.UUID {
	var owners []ids.UUID
	for _, id := range memberIDs {
		if m, ok := c.member(id); ok && m.OwnerLive && !slices.Contains(owners, *m.OwnerID) {
			owners = append(owners, *m.OwnerID)
		}
	}
	slices.SortFunc(owners, compareUUID)
	return owners
}

func compareUUID(a, b ids.UUID) int { return slices.Compare(a[:], b[:]) }

func sortedUUIDs(in ...ids.UUID) []ids.UUID {
	slices.SortFunc(in, compareUUID)
	return in
}

// openContactPairs is the edge rule a case is read over. A pair counts when
// it is open and both ends pass the queue's own live visibility clause.
func openContactPairs(ctx context.Context, arg func(any) int) (string, error) {
	visible, err := dedupeVisibilityClause(ctx, arg, true)
	if err != nil {
		return "", err
	}
	return `dedupe_candidate.entity_type = 'contact' AND dedupe_candidate.disposition = 'open'
	    AND dedupe_candidate.archived_at IS NULL AND ` + visible, nil
}

// readContactCase reads the case holding start, as start's reviewer sees it.
// A contact in no open pair the reader can see answers an empty case.
func readContactCase(ctx context.Context, tx pgx.Tx, start ids.UUID) (contactCase, error) {
	components, err := readContactComponents(ctx, tx)
	if err != nil {
		return contactCase{}, err
	}
	k, ok := componentHolding(components, start)
	if !ok {
		return contactCase{}, nil
	}
	return readComponentCase(ctx, tx, k, start)
}

// readComponentCase reads the members a review of the component shows from
// the addressed member, with their pairs and the grants on them.
func readComponentCase(ctx context.Context, tx pgx.Tx, k contactComponent, address ids.UUID) (contactCase, error) {
	shown := k.shown(address)
	c := contactCase{Anchor: k.anchor, Confidence: k.confidence, TooLarge: k.tooLarge()}
	var err error
	if c.Members, err = readCaseMembers(ctx, tx, shown); err != nil {
		return contactCase{}, err
	}
	if c.Edges, err = readCaseEdges(ctx, tx, shown); err != nil {
		return contactCase{}, err
	}
	if c.Grants, err = readCaseGrants(ctx, tx, shown); err != nil {
		return contactCase{}, err
	}
	return c, nil
}

// liveCaseOwnerSQL is whether the joined owner seat u still counts as an owner
// of a case. It asks what identity asks before carrying a former owner's
// access, so the owners a case names are the ones a merge can carry.
var liveCaseOwnerSQL = "coalesce(u.removed_at IS NULL AND " + workforce.LiveHumanSQL("u") + ", false)"

// readCaseMembers reads what a reviewer compares, in id order. The employer is
// the list's own, so a company hidden from the reader stays hidden here.
func readCaseMembers(ctx context.Context, tx pgx.Tx, memberIDs []ids.UUID) ([]caseMember, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	email, err := orderByReachableEmail(ctx, arg)
	if err != nil {
		return nil, err
	}
	employer, err := orderByCurrentEmployer(ctx, arg)
	if err != nil {
		return nil, err
	}
	members := arg(memberIDs)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT contact.id, contact.full_name, %s, contact.title, %s,
		       contact.owner_id, u.display_name,
		       %s, coalesce(u.seat_type, ''),
		       contact.version, contact.created_at
		  FROM contact LEFT JOIN app_user u ON u.id = contact.owner_id
		 WHERE contact.id = ANY($%d) AND contact.archived_at IS NULL
		 ORDER BY contact.id`, email, employer, liveCaseOwnerSQL, members), args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the case members: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (caseMember, error) {
		var m caseMember
		err := row.Scan(&m.ID, &m.Name, &m.Email, &m.Title, &m.Company, &m.OwnerID, &m.OwnerName,
			&m.OwnerLive, &m.OwnerSeat, &m.Version, &m.CreatedAt)
		return m, err
	})
}

// readCaseEdges reads the pairs between members that a reviewer weighs: open
// ones, and dismissed ones so nobody merges what a colleague called different
// contacts. The pairs' own visibility clause bounds both ends again.
func readCaseEdges(ctx context.Context, tx pgx.Tx, memberIDs []ids.UUID) ([]caseEdge, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	members := arg(memberIDs)
	visible, err := dedupeVisibilityClause(ctx, arg, true)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT id, left_contact_id, right_contact_id, confidence, evidence, disposition
		  FROM dedupe_candidate
		 WHERE entity_type = 'contact' AND archived_at IS NULL
		   AND disposition IN ('open', 'not_a_duplicate')
		   AND left_contact_id = ANY($%[1]d) AND right_contact_id = ANY($%[1]d)
		   AND %[2]s
		 ORDER BY id`, members, visible), args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the case pairs: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (caseEdge, error) {
		var e caseEdge
		err := row.Scan(&e.ID, &e.Left, &e.Right, &e.Confidence, &e.Evidence, &e.Disposition)
		return e, err
	})
}

// caseGrant is one share on a member, as far as a merge's carry reads it.
type caseGrant struct {
	ID      ids.UUID
	Version int64
	Access  string
	Live    bool
}

// readCaseGrants reads the shares on the members. A merge carries them and
// folds each former owner's access into them, so a change to one changes what
// the merge would write.
func readCaseGrants(ctx context.Context, tx pgx.Tx, memberIDs []ids.UUID) ([]caseGrant, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, version, access, (expires_at IS NULL OR expires_at > now())
		  FROM record_grant
		 WHERE record_type = 'contact' AND record_id = ANY(@shared)
		 ORDER BY id`, pgx.NamedArgs{"shared": memberIDs})
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the shares on the case members: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (caseGrant, error) {
		var g caseGrant
		return g, row.Scan(&g.ID, &g.Version, &g.Access, &g.Live)
	})
}

// actorOwnsAMember says whether the caller is the live full-seat owner of one
// of the given members, the standing a delegated merge rests on.
func (c contactCase) actorOwnsAMember(actor principal.Principal, memberIDs []ids.UUID) bool {
	for _, id := range memberIDs {
		m, ok := c.member(id)
		if ok && m.OwnerLive && *m.OwnerID == actor.UserID && m.OwnerSeat == string(principal.SeatFull) {
			return true
		}
	}
	return false
}
