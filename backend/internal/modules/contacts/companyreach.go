// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Who on the team can get us in at one account.
//
// CompanyLinkedInReach counts it per colleague and takes the gates. This turns
// those counts into the answer a reader sees: the colleague to ask, named, with
// how many of their connections work there.
//
// The names are read here rather than in the engine because the engine answers
// a graph question and a name is a presentation of it. A count with no name is
// not actionable, and a name with no count is not evidence.

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CompanyReachColleague is one colleague's reach into one account.
type CompanyReachColleague struct {
	UserID      ids.UUID
	DisplayName string
	Connections int
	Functions   []crmcontracts.LinkedInReachFunctionCount
}

// CompanyReach answers the colleagues with reach into one account, highest
// first and then by name, so two reads of an unchanged network agree.
//
// A colleague who switched their reach off is absent, which the engine applies.
// Absent and not zero: zero would say they have no reach, which is a different
// statement and not theirs to have made.
func (s *Store) CompanyReach(ctx context.Context, companyID ids.CompanyID) ([]CompanyReachColleague, error) {
	var out []CompanyReachColleague
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.CompanyReachTx(ctx, tx, companyID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CompanyReachTx is CompanyReach inside a transaction the caller holds, for a
// read that needs the reach and other rows to agree.
func (s *Store) CompanyReachTx(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) ([]CompanyReachColleague, error) {
	// The gates live in here: company read, then the row probe that answers
	// 404 for an account the caller cannot open.
	counts, err := CompanyLinkedInReach(ctx, tx, companyID)
	if err != nil {
		return nil, err
	}
	if len(counts) == 0 {
		return []CompanyReachColleague{}, nil
	}
	return reachColleagueNames(ctx, tx, counts)
}

// reachColleagueNames puts a name to each counted colleague.
//
// A colleague the name read does not return is dropped rather than listed
// nameless. The answer exists to say whom to ask, and "somebody here has
// three connections" is not that.
func reachColleagueNames(
	ctx context.Context, tx pgx.Tx, counts map[ids.UUID]ColleagueReach,
) ([]CompanyReachColleague, error) {
	users := make([]ids.UUID, 0, len(counts))
	for user := range counts {
		users = append(users, user)
	}
	rows, err := tx.Query(ctx,
		`SELECT id, display_name FROM app_user WHERE id = ANY($1)`, users)
	if err != nil {
		return nil, fmt.Errorf("contacts: naming the colleagues with reach: %w", err)
	}
	defer rows.Close()
	out := make([]CompanyReachColleague, 0, len(counts))
	for rows.Next() {
		var user ids.UUID
		var name string
		if err := rows.Scan(&user, &name); err != nil {
			return nil, fmt.Errorf("contacts: scanning a colleague with reach: %w", err)
		}
		out = append(out, CompanyReachColleague{
			UserID: user, DisplayName: name,
			Connections: counts[user].Connections, Functions: counts[user].Functions,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: reading the colleagues with reach: %w", err)
	}
	sortByReachThenName(out)
	return out, nil
}

// sortByReachThenName puts the colleague to ask first.
//
// Highest count first, then by name. The tiebreak is what makes two reads of
// an unchanged network return the same order. A reader compares one glance
// with the next.
func sortByReachThenName(colleagues []CompanyReachColleague) {
	sort.Slice(colleagues, func(i, j int) bool {
		if colleagues[i].Connections != colleagues[j].Connections {
			return colleagues[i].Connections > colleagues[j].Connections
		}
		return colleagues[i].DisplayName < colleagues[j].DisplayName
	})
}
