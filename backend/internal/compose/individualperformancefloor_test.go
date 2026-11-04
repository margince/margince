// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// floorCase is one figure's seats against the teams its reader may read.
type floorCase struct {
	name   string
	reader string
	group  []string
	teams  map[string][]string
	left   []string
	thin   bool
}

func TestBelowFloorWithholdsATotalAnotherTeamWouldLeaveToOneColleague(t *testing.T) {
	t.Parallel()
	sales := []string{"rita", "anna", "ben", "chris"}
	north := []string{"rita", "anna", "ben"}
	cases := []floorCase{
		{name: "a nested team's parent leaves one member", reader: "rita", group: sales,
			teams: map[string][]string{"sales": sales, "north": north}, thin: true},
		{name: "the nested team itself is shown", reader: "rita", group: north,
			teams: map[string][]string{"sales": sales, "north": north}},
		{name: "the one member left over is the reader", reader: "chris", group: sales,
			teams: map[string][]string{"sales": sales, "north": north}},
		{name: "a reader outside both teams", reader: "zoe", group: sales,
			teams: map[string][]string{"sales": sales, "north": north}, thin: true},
		{name: "a partial overlap that leaves two", reader: "zoe", group: []string{"anna", "ben", "chris", "dora"},
			teams: map[string][]string{"mixed": {"anna", "ben", "xena"}}},
		{name: "a partial overlap that leaves one", reader: "zoe", group: []string{"anna", "ben", "chris", "dora"},
			teams: map[string][]string{"mixed": {"anna", "ben", "chris", "xena"}}, thin: true},
		{name: "three teams that share members pairwise", reader: "rita", group: []string{"rita", "anna", "ben"},
			teams: map[string][]string{"one": {"rita", "anna", "ben"}, "two": {"rita", "anna", "chris"}, "three": {"rita", "ben", "chris"}}, thin: true},
		{name: "a colleague who left is taken out but not counted", reader: "zoe", group: []string{"anna", "ben", "carl"},
			teams: map[string][]string{"anna's": {"anna"}}, left: []string{"carl"}, thin: true},
		{name: "a team sharing no seat", reader: "zoe", group: []string{"anna", "ben", "chris"},
			teams: map[string][]string{"elsewhere": {"xena", "yuri"}}},
		{name: "a difference that leaves nobody", reader: "zoe", group: []string{"anna", "ben"},
			teams: map[string][]string{"wider": {"anna", "ben", "chris"}}},
		{name: "a lone colleague with no team", reader: "zoe", group: []string{"anna"}, thin: true},
		{name: "a seat named twice counts once", reader: "zoe", group: []string{"anna", "anna"}, thin: true},
		{name: "the reader's own figure", reader: "rita", group: []string{"rita", "rita"},
			teams: map[string][]string{"sales": sales}},
		{name: "no seat at all", reader: "rita"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			seats := floorSeats{}
			live := map[ids.UUID]bool{}
			for _, team := range c.teams {
				for _, member := range team {
					live[seats.of(member)] = true
				}
			}
			for _, member := range c.group {
				live[seats.of(member)] = true
			}
			for _, member := range c.left {
				live[seats.of(member)] = false
			}
			teamsOf := map[ids.UUID][]floorTeam{}
			for name, members := range c.teams {
				team := floorTeam{team: seats.of("team:" + name)}
				for _, member := range members {
					teamsOf[seats.of(member)] = append(teamsOf[seats.of(member)], team)
				}
			}
			group := make([]ids.UUID, 0, len(c.group))
			for _, member := range c.group {
				group = append(group, seats.of(member))
			}
			if got := belowFloor(seats.of(c.reader), [][]ids.UUID{group}, live, teamsOf); got[0] != c.thin {
				t.Errorf("withheld = %v, want %v", got[0], c.thin)
			}
		})
	}
}

// "My teams" is a total of its own: two teams each leave two colleagues, and
// their union leaves one.
func TestBelowFloorTakesALeadsMyTeamsAway(t *testing.T) {
	t.Parallel()
	seats := floorSeats{}
	company := []ids.UUID{seats.of("anna"), seats.of("ben"), seats.of("chris"), seats.of("dora")}
	live := map[ids.UUID]bool{}
	for _, seat := range company {
		live[seat] = true
	}
	first, second, mine := floorTeam{team: seats.of("first")}, floorTeam{team: seats.of("second")}, floorTeam{managed: true}
	teamsOf := map[ids.UUID][]floorTeam{
		seats.of("anna"): {first}, seats.of("ben"): {first, second}, seats.of("chris"): {second},
	}
	if belowFloor(seats.of("lead"), [][]ids.UUID{company}, live, teamsOf)[0] {
		t.Fatal("the company is withheld by two teams that each leave two colleagues")
	}
	for _, seat := range []ids.UUID{seats.of("anna"), seats.of("ben"), seats.of("chris")} {
		teamsOf[seat] = append(teamsOf[seat], mine)
	}
	if !belowFloor(seats.of("lead"), [][]ids.UUID{company}, live, teamsOf)[0] {
		t.Error("the company is shown although taking the lead's teams away leaves one colleague")
	}
}

// floorSeats names test seats so a table reads as named colleagues.
type floorSeats map[string]ids.UUID

func (s floorSeats) of(name string) ids.UUID {
	if seat, ok := s[name]; ok {
		return seat
	}
	s[name] = ids.NewV7()
	return s[name]
}

// BenchmarkBelowFloor is the size of a large installation: 200 teams over
// 2,000 seats, each seat on two teams, and 5,000 figures of 40 seats each.
func BenchmarkBelowFloor(b *testing.B) {
	const teamCount, seatCount, groupCount, groupSize = 200, 2000, 5000, 40
	seats := make([]ids.UUID, seatCount)
	teams := make([]floorTeam, teamCount)
	for index := range teams {
		teams[index] = floorTeam{team: ids.NewV7()}
	}
	live := make(map[ids.UUID]bool, seatCount)
	teamsOf := make(map[ids.UUID][]floorTeam, seatCount)
	for index := range seats {
		seats[index] = ids.NewV7()
		live[seats[index]] = index%10 != 0
		teamsOf[seats[index]] = []floorTeam{teams[index%teamCount], teams[(index*7+3)%teamCount]}
	}
	groups := make([][]ids.UUID, groupCount)
	for index := range groups {
		groups[index] = make([]ids.UUID, groupSize)
		for slot := range groups[index] {
			groups[index][slot] = seats[(index*31+slot*13)%seatCount]
		}
	}
	b.ResetTimer()
	for b.Loop() {
		belowFloor(seats[0], groups, live, teamsOf)
	}
}
