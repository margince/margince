// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package retirementpreview holds the cases of a member's retirement preview
// that read the records behind its blockers.
package retirementpreview

import (
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type anyMap = map[string]any

type blockingRecord struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	WebURL string `json:"web_url"`
}

type blockingRecords struct {
	Blocker string           `json:"blocker"`
	Total   int              `json:"total"`
	Visible int              `json:"visible"`
	Records []blockingRecord `json:"records"`
}

type retirementPreview struct {
	Blockers        []string          `json:"blockers"`
	BlockingRecords []blockingRecords `json:"blocking_records"`
}

func (p retirementPreview) of(blocker string) blockingRecords {
	for _, entry := range p.BlockingRecords {
		if entry.Blocker == blocker {
			return entry
		}
	}
	return blockingRecords{Blocker: blocker}
}

func (p retirementPreview) names(blocker string) []string {
	names := []string{}
	for _, record := range p.of(blocker).Records {
		names = append(names, record.Name)
	}
	return names
}

type leaverScene struct {
	admin    *apptest.AppEnv
	agent    *apptest.MCPClient
	leaverID string
	// dealOfRoom maps each room the leaver stewards to the deal it opens on.
	dealOfRoom map[string]string
}

func createdID(t *testing.T, seat *apptest.AppEnv, path string, body anyMap) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	if status := seat.Call(t, "POST", path, body, nil, &created); status != http.StatusCreated || created.ID == "" {
		t.Fatalf("POST %s → %d with id %q, want 201", path, status, created.ID)
	}
	return created.ID
}

// setupLeaver gives a member of management three live Deal Rooms to steward and three saved
// reports, two shared with the workspace and one kept private.
func setupLeaver(t *testing.T) *leaverScene {
	t.Helper()
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp")}
	})
	apptest.BootstrapWorkspaceSession(t, e, "Leaving", "admin@leaving.test", "Ada Admin")
	e.DescribeCompany(t)
	const email, password = "leaver@leaving.test", "leaving-rep-password"
	s := &leaverScene{admin: e, dealOfRoom: map[string]string{}}
	s.leaverID = createdID(t, e, "/v1/users", anyMap{"email": email, "display_name": "Lea Leaver", "role": "management"})
	apptest.SetPasswordByLink(t, e, s.leaverID, password)
	leaver := e.AsSecondSeat(t, email, password)

	stages := apptest.DiscoverSeededPipeline(t, e)
	for _, title := range []string{"Room Alpha", "Room Beta", "Room Gamma"} {
		deal := createdID(t, e, "/v1/deals", anyMap{"name": title + " deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual"})
		room := createdID(t, e, "/v1/deal-rooms", anyMap{"deal_id": deal, "title": title, "source": "manual", "steward_user_id": s.leaverID})
		s.dealOfRoom[room] = deal
	}
	selection := anyMap{"period": "this_month", "target_basis": "month", "close_window": "all_open", "metrics": []string{firstMetric(t, leaver)}}
	for name, audience := range map[string]string{"Board pipeline": "workspace", "Forecast": "workspace", "Private notes": "private"} {
		createdID(t, leaver, "/v1/analytics/reports", anyMap{"name": name, "audience": audience, "selection": selection})
	}
	s.agent = apptest.NewMCPClient(e, apptest.MCPBearerToken(t, e, "admin agent", "read"))
	return s
}

func firstMetric(t *testing.T, seat *apptest.AppEnv) string {
	t.Helper()
	var catalog struct {
		Metrics []struct {
			ID string `json:"id"`
		} `json:"metrics"`
	}
	if status := seat.Call(t, "GET", "/v1/analytics/metrics", nil, nil, &catalog); status != http.StatusOK || len(catalog.Metrics) == 0 {
		t.Fatalf("the metric catalog → %d with %d metrics", status, len(catalog.Metrics))
	}
	return catalog.Metrics[0].ID
}

// preview reads the preview over HTTP and over the agent's read_workspace topic,
// and fails unless the two name the same records.
func (s *leaverScene) preview(t *testing.T) retirementPreview {
	t.Helper()
	var web retirementPreview
	if status := s.admin.Call(t, "GET", "/v1/users/"+s.leaverID+"/retirement-preview?mode=remove", nil, nil, &web); status != http.StatusOK {
		t.Fatalf("the retirement preview → %d, want 200", status)
	}
	var tool retirementPreview
	s.agent.CallOK(t, "read_workspace", map[string]any{"topic": "member_retirement", "source_id": s.leaverID}).JSON(t, &tool)
	if !reflect.DeepEqual(web, tool) {
		t.Fatalf("the two doors disagree:\nHTTP %+v\nMCP  %+v", web, tool)
	}
	return web
}

type role struct {
	Key     string `json:"key"`
	Version int64  `json:"version"`
}

// narrowAdmin changes the admin role, the only one that may retire a member.
func (s *leaverScene) narrowAdmin(t *testing.T, path string, body anyMap) {
	t.Helper()
	var directory struct {
		Roles []role `json:"roles"`
	}
	if status := s.admin.Call(t, "GET", "/v1/roles", nil, nil, &directory); status != http.StatusOK {
		t.Fatalf("listing the roles → %d, want 200", status)
	}
	at := slices.IndexFunc(directory.Roles, func(r role) bool { return r.Key == "admin" })
	if at < 0 {
		t.Fatalf("the roles %+v hold no admin role", directory.Roles)
	}
	ifMatch := map[string]string{"If-Match": strconv.FormatInt(directory.Roles[at].Version, 10)}
	if status := s.admin.Call(t, "PATCH", "/v1/roles/admin"+path, body, ifMatch, nil); status != http.StatusOK {
		t.Fatalf("PATCH /v1/roles/admin%s → %d, want 200", path, status)
	}
}

// Every blocking room is named, with the deal page it opens on.
func TestTheRetirementPreviewNamesTheRoomsThatBlockIt(t *testing.T) {
	s := setupLeaver(t)
	got := s.preview(t)
	if !slices.Contains(got.Blockers, "deal_rooms") || !slices.Contains(got.Blockers, "reports") {
		t.Fatalf("blockers = %v, want deal_rooms and reports", got.Blockers)
	}
	rooms := got.of("deal_rooms")
	if rooms.Total != 3 || rooms.Visible != 3 || !slices.Equal(got.names("deal_rooms"), []string{"Room Alpha", "Room Beta", "Room Gamma"}) {
		t.Fatalf("deal_rooms = %+v, want total 3, visible 3 and the three rooms by name", rooms)
	}
	for _, room := range rooms.Records {
		if want := s.admin.TS.URL + "/#/deals/" + s.dealOfRoom[room.ID]; room.WebURL != want {
			t.Errorf("room %s links to %q, want its deal %q", room.Name, room.WebURL, want)
		}
	}
	if reports := got.of("reports"); reports.Total != 3 || reports.Visible != 3 {
		t.Errorf("reports = %+v, want total 3 and visible 3 to an admin who reads every report", reports)
	}
	for _, entry := range got.BlockingRecords {
		if entry.Blocker == "deactivate_first" {
			t.Errorf("the state blocker deactivate_first has records: %+v", entry)
		}
	}
}

// A record the reader may not open counts in total and is never named.
func TestTheRetirementPreviewNamesOnlyWhatTheReaderMayOpen(t *testing.T) {
	s := setupLeaver(t)
	s.narrowAdmin(t, "", anyMap{"row_scope": "own"})
	got := s.preview(t)
	reports := got.of("reports")
	if reports.Total != 3 || reports.Visible != 2 || !slices.Equal(got.names("reports"), []string{"Board pipeline", "Forecast"}) {
		t.Fatalf("reports = %+v, want total 3, visible 2 and only the two shared reports", reports)
	}

	s.narrowAdmin(t, "/objects/deal_room", anyMap{"create": false, "read": false, "update": false, "delete": false})
	got = s.preview(t)
	if rooms := got.of("deal_rooms"); rooms.Total != 3 || rooms.Visible != 0 || len(rooms.Records) != 0 {
		t.Fatalf("deal_rooms = %+v, want total 3 and none named to a reader without Deal Room read", rooms)
	}
	if !slices.Contains(got.Blockers, "deal_rooms") {
		t.Errorf("blockers = %v, still want deal_rooms", got.Blockers)
	}
}
