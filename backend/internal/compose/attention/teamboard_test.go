// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What the team board answers, who may ask it, and the two ways a count can be
// wrong in the direction that matters: a figure presented as a total when it is
// a floor, and work attributed to nobody quietly dropped.

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var boardInstant = time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)

// roster is the membership reader, answering a fixed team.
type roster []TeamMember

func (r roster) SharesLiveTeamWithCaller(context.Context, ids.UUID) (bool, error) {
	return true, nil
}

func (r roster) LiveTeammatesOfCaller(context.Context) ([]TeamMember, bool, error) {
	return []TeamMember(r), false, nil
}

// waitingSaying is the who-is-waiting lane over a fixed list.
type waitingSaying struct {
	rows []WaitingCustomer
	// cut says the SCAN behind these rows stopped at its bound. Its own field
	// rather than len(rows) >= some number, because that is exactly the
	// inference the real lane cannot make: it filters after it scans, so the
	// rows it returns are fewer than the rows it read.
	cut bool
}

func (w waitingSaying) Unanswered(context.Context, time.Time) ([]WaitingCustomer, bool, error) {
	return w.rows, w.cut, nil
}

// overdueSaying is the counting reader over a fixed tally.
type overdueSaying map[ids.UUID]int

func (o overdueSaying) OverduePerAssignee(context.Context, time.Time) (map[ids.UUID]int, error) {
	return map[ids.UUID]int(o), nil
}

func boardService(members ...TeamMember) *Service {
	return &Service{
		teammates: roster(members),
		now:       func() time.Time { return boardInstant },
	}
}

func boardReaderAt(tier principal.RowScope) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type:        principal.PrincipalHuman,
		UserID:      theReader,
		Permissions: principal.Permissions{RowScope: tier},
	})
}

// A rep who can see only their own rows has no team question to ask, and
// offering them a board of one would advertise a surface whose every count is
// theirs already.
func TestAnOwnScopedReaderIsRefusedTheBoard(t *testing.T) {
	t.Parallel()

	svc := boardService()
	if _, err := svc.TeamBoard(boardReaderAt(principal.RowScopeOwn)); err == nil {
		t.Fatal("a reader who sees only their own work was given the team's board")
	}
}

// The two tiers that reach past the reader are admitted. A refusal test that
// refuses everybody proves nothing about the rule it claims to hold.
func TestTeamAndAllScopedReadersGetTheBoard(t *testing.T) {
	t.Parallel()

	for _, tier := range []principal.RowScope{principal.RowScopeTeam, principal.RowScopeAll} {
		svc := boardService(TeamMember{UserID: theReader, DisplayName: "the reader"})
		board, err := svc.TeamBoard(boardReaderAt(tier))
		if err != nil {
			t.Fatalf("row scope %q was refused the team board: %v", tier, err)
		}
		if len(board.Members) != 1 {
			t.Fatalf("row scope %q drew %d rows, wanted the one member the roster holds",
				tier, len(board.Members))
		}
	}
}

// Every count is attributed to the person the work names, and the three sources
// land in three different columns of one row.
func TestEachTeammatesWorkLandsInTheirOwnRow(t *testing.T) {
	t.Parallel()

	svc := boardService(
		TeamMember{UserID: theReader, DisplayName: "Aa Reader"},
		TeamMember{UserID: theColleague, DisplayName: "Bb Colleague"},
	)
	svc.waiting = waitingSaying{rows: []WaitingCustomer{
		{OwnerID: theColleague}, {OwnerID: theColleague}, {OwnerID: theReader},
	}}
	svc.overdueLoad = overdueSaying{theColleague: 4}
	colleague := theColleague
	svc.atRisk = stubAtRisk{rows: []RiskyDeal{{OwnerID: &colleague}}}

	board, err := svc.TeamBoard(boardReaderAt(principal.RowScopeTeam))
	if err != nil {
		t.Fatalf("the board refused a team-scoped reader: %v", err)
	}
	if len(board.Members) != 2 {
		t.Fatalf("the board drew %d rows over a team of two", len(board.Members))
	}
	// Ordered by display name, so a manager finds a person where they left them.
	if board.Members[0].DisplayName != "Aa Reader" {
		t.Fatalf("the board led with %q, wanted the alphabetically first member",
			board.Members[0].DisplayName)
	}
	theirs := board.Members[1].Counts
	if theirs.Waiting != 2 {
		t.Errorf("the colleague's waiting count was %d, wanted the 2 filed under them", theirs.Waiting)
	}
	if theirs.Overdue != 4 {
		t.Errorf("the colleague's overdue count was %d, wanted 4", theirs.Overdue)
	}
	if theirs.AtRisk != 1 {
		t.Errorf("the colleague's at-risk count was %d, wanted 1", theirs.AtRisk)
	}
	mine := board.Members[0].Counts
	if mine.Waiting != 1 || mine.Overdue != 0 || mine.AtRisk != 0 {
		t.Errorf("the reader's own row read %+v, wanted only their single wait", mine)
	}
}

// Work that names nobody is REPORTED, not dropped.
//
// It is the wait most likely to go unseen — nobody is looking at it by
// definition — so a board that folded it away would show a clean team over
// exactly the work that goes missing.
func TestWorkNobodyOwnsIsReportedRatherThanDropped(t *testing.T) {
	t.Parallel()

	svc := boardService(TeamMember{UserID: theReader, DisplayName: "the reader"})
	svc.waiting = waitingSaying{rows: []WaitingCustomer{{}, {}}}
	svc.overdueLoad = overdueSaying{{}: 3}
	svc.atRisk = stubAtRisk{rows: []RiskyDeal{{OwnerID: nil}}}

	board, err := svc.TeamBoard(boardReaderAt(principal.RowScopeTeam))
	if err != nil {
		t.Fatalf("the board refused a team-scoped reader: %v", err)
	}
	if board.Unassigned.Waiting != 2 {
		t.Errorf("the unassigned waiting count was %d, wanted the 2 nobody owns",
			board.Unassigned.Waiting)
	}
	if board.Unassigned.Overdue != 3 {
		t.Errorf("the unassigned overdue count was %d, wanted 3", board.Unassigned.Overdue)
	}
	if board.Unassigned.AtRisk != 1 {
		t.Errorf("the unassigned at-risk count was %d, wanted the ownerless deal",
			board.Unassigned.AtRisk)
	}
	// And it must not have been folded into a member's row instead, which would
	// blame somebody for work nothing assigns to them.
	if got := board.Members[0].Counts; got.Waiting != 0 || got.Overdue != 0 || got.AtRisk != 0 {
		t.Errorf("the reader's row read %+v, wanted nothing — every row here names nobody", got)
	}
}

// A lane read to its bound reports a FLOOR, and the board says so.
//
// Under-reporting is the one direction that must not fail silently: a figure
// presented as a total when more work sits past the bound tells a lead their
// team is lighter than it is, and no assertion anywhere says otherwise.
func TestACountReadToItsBoundIsReportedAsAFloor(t *testing.T) {
	t.Parallel()

	// ONE row, and the lane saying its scan was cut. That pairing is the whole
	// case: the seam drops machine senders and folds duplicate threads after the
	// SQL cap, so a scan that read two hundred can return one — and a board that
	// inferred truncation from the row count would call this a complete answer.
	svc := boardService(TeamMember{UserID: theReader, DisplayName: "the reader"})
	svc.waiting = waitingSaying{rows: []WaitingCustomer{{OwnerID: theReader}}, cut: true}

	board, err := svc.TeamBoard(boardReaderAt(principal.RowScopeTeam))
	if err != nil {
		t.Fatalf("the board refused a team-scoped reader: %v", err)
	}
	if !board.Truncated {
		t.Fatal("the waiting scan stopped at its bound and the board called its count a total")
	}

	svc.waiting = waitingSaying{rows: []WaitingCustomer{{OwnerID: theReader}}, cut: false}
	board, err = svc.TeamBoard(boardReaderAt(principal.RowScopeTeam))
	if err != nil {
		t.Fatalf("the board refused a team-scoped reader: %v", err)
	}
	if board.Truncated {
		t.Fatal("a complete waiting scan was reported as truncated")
	}
}

// The at-risk lane's OWN cut flag decides, never its row count.
//
// This is the case a row count cannot see. That lane filters after it scans, so
// a sweep that stopped at fifty can leave three survivors — and three is exactly
// what a small, complete pipeline returns. Inferring from the count called the
// truncated read complete, which is the direction that tells a lead their team
// is lighter than it is.
func TestTheAtRiskLanesOwnCutFlagDecidesRatherThanItsRowCount(t *testing.T) {
	t.Parallel()

	owner := theColleague
	svc := boardService(TeamMember{UserID: theReader, DisplayName: "the reader"})
	svc.atRisk = stubAtRisk{rows: []RiskyDeal{{OwnerID: &owner}}, cut: true}

	board, err := svc.TeamBoard(boardReaderAt(principal.RowScopeTeam))
	if err != nil {
		t.Fatalf("the board refused a team-scoped reader: %v", err)
	}
	if !board.Truncated {
		t.Fatal("the at-risk sweep stopped at its bound and the board called its " +
			"single surviving row a complete count")
	}

	svc.atRisk = stubAtRisk{rows: []RiskyDeal{{OwnerID: &owner}}, cut: false}
	board, err = svc.TeamBoard(boardReaderAt(principal.RowScopeTeam))
	if err != nil {
		t.Fatalf("the board refused a team-scoped reader: %v", err)
	}
	if board.Truncated {
		t.Fatal("a complete at-risk sweep was reported as truncated")
	}
}

// A roster cut at its own bound is truncation too.
//
// Both ways of falling short reach the reader as one flag, because what they
// need to know is the same: the figures in front of them are floors. A board
// that reported only the COUNT bounds would present a hundred people as the
// whole of a larger team and say nothing.
func TestATeamLargerThanTheRosterCapIsReportedAsTruncated(t *testing.T) {
	t.Parallel()

	svc := &Service{
		teammates: rosterCutAt{},
		now:       func() time.Time { return boardInstant },
	}
	board, err := svc.TeamBoard(boardReaderAt(principal.RowScopeTeam))
	if err != nil {
		t.Fatalf("the board refused a team-scoped reader: %v", err)
	}
	if !board.Truncated {
		t.Fatal("the roster was cut at its bound and the board presented the " +
			"names it drew as the whole team")
	}
}

// rosterCutAt is the membership reader answering one member and reporting that
// more exist.
type rosterCutAt struct{}

func (rosterCutAt) SharesLiveTeamWithCaller(context.Context, ids.UUID) (bool, error) {
	return true, nil
}

func (rosterCutAt) LiveTeammatesOfCaller(context.Context) ([]TeamMember, bool, error) {
	return []TeamMember{{UserID: theReader, DisplayName: "the reader"}}, true, nil
}

// A source that could not answer is an ERROR, never a column of zeros.
//
// The ranked queue can name a withheld source on the wire because it returns
// rows and has somewhere to say so. A board is nothing but numbers, and a zero
// meaning "you may not see this" is indistinguishable from one meaning "they are
// clear" — which is the reading that tells a lead their team is fine.
func TestASourceThatCouldNotAnswerFailsRatherThanReadingAsZero(t *testing.T) {
	t.Parallel()

	svc := boardService(TeamMember{UserID: theReader, DisplayName: "the reader"})
	svc.overdueLoad = overdueFailing{}

	if _, err := svc.TeamBoard(boardReaderAt(principal.RowScopeTeam)); err == nil {
		t.Fatal("a source that failed to answer was drawn as a column of zeros")
	}
}

type overdueFailing struct{}

func (overdueFailing) OverduePerAssignee(context.Context, time.Time) (map[ids.UUID]int, error) {
	return nil, errors.New("counting overdue tasks")
}

// Without the membership reader the board has no roster, and answering it as an
// empty team would report that nobody works here.
func TestAnUnboundMembershipReaderRefusesRatherThanDrawingAnEmptyTeam(t *testing.T) {
	t.Parallel()

	svc := &Service{now: func() time.Time { return boardInstant }}
	if _, err := svc.TeamBoard(boardReaderAt(principal.RowScopeTeam)); err == nil {
		t.Fatal("a board with no membership reader drew an empty team instead of refusing")
	}
}

// The team scope keeps the team's rows and nobody else's.
//
// It used to narrow NOTHING, and the gap is not theoretical: the task lane's
// gate is a link walk, and auth.ActivityDiscoverClause coalesces the empty link
// set to TRUE, so a task carrying no record link is discoverable by everyone in
// the installation. A team-scoped reader asking for `team` was handed exactly
// those rows under a heading that says "my team" — while resolveOwner refuses to
// open that same person's queue by name.
func TestTheTeamScopeKeepsTheTeamsRowsAndNobodyElses(t *testing.T) {
	t.Parallel()

	outsider := ids.MustParse("01a05500-0000-7000-8000-0000000000ff")
	svc := &Service{
		teammates: roster{
			{UserID: theReader, DisplayName: "the reader"},
			{UserID: theColleague, DisplayName: "a teammate"},
		},
		now: func() time.Time { return boardInstant },
	}
	rows := []ranked{
		{item: crmcontracts.WorklistItem{Id: "mine"}, owner: theReader},
		{item: crmcontracts.WorklistItem{Id: "teammate"}, owner: theColleague},
		{item: crmcontracts.WorklistItem{Id: "outsider"}, owner: outsider},
		{item: crmcontracts.WorklistItem{Id: "nobody"}},
	}

	kept := map[string]bool{}
	for _, row := range svc.narrowToScope(boardReaderAt(principal.RowScopeTeam), rows, scopeTeam, ids.UUID{}) {
		kept[row.item.Id] = true
	}
	if !kept["mine"] || !kept["teammate"] {
		t.Errorf("the team scope dropped the reader's own or their teammate's row: %v", kept)
	}
	if kept["outsider"] {
		t.Error("a colleague on no team of the reader's arrived on the team queue — the " +
			"same person resolveOwner refuses to open by name")
	}
	if !kept["nobody"] {
		t.Error("work naming nobody was dropped from the team queue, which is the only " +
			"wider scope most readers hold")
	}
}

// Without the membership reader the team scope answers NOTHING.
//
// The same fail-closed rule resolveOwner keeps for its own nil case: a scope
// named `team` that handed back every row it had read would be widening rather
// than narrowing, which is a security hole wearing the shape of a missing lane.
func TestTheTeamScopeFailsClosedWithoutTheMembershipReader(t *testing.T) {
	t.Parallel()

	svc := &Service{now: func() time.Time { return boardInstant }}
	rows := []ranked{{item: crmcontracts.WorklistItem{Id: "somebody's"}, owner: theColleague}}
	if got := svc.narrowToScope(boardReaderAt(principal.RowScopeTeam), rows, scopeTeam, ids.UUID{}); len(got) != 0 {
		t.Fatalf("the team scope answered %d rows with no membership reader bound", len(got))
	}
}
