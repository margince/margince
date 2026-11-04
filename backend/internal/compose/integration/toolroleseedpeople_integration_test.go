// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sweepSeed is what a state seed reaches: the app, the pool it notes facts in,
// and the seeders of the kinds it builds on.
type sweepSeed struct {
	t      *testing.T
	e      *apptest.AppEnv
	pool   *recordPool
	create map[string]func() string
}

// call sends one route as the signed-in admin and stops the sweep on any
// failure. A seed that fails would leave its calls reading as measured.
//
//craft:ignore naked-any a request body is whichever JSON shape the route takes
func (s sweepSeed) call(method, path string, body any, headers map[string]string, out any) {
	s.t.Helper()
	var raw json.RawMessage
	if status := s.e.Call(s.t, method, path, body, headers, &raw); status < 200 || status >= 300 {
		s.t.Fatalf("seeding %s %s → %d %s", method, path, status, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			s.t.Fatalf("seeding %s %s answered %s: %v", method, path, raw, err)
		}
	}
}

func (s sweepSeed) post(path string, body AnyMap) string { return seededID(s.t, s.e, path, body) }

// ownerExec writes what no route writes, as the database owner: a state only
// a connector, a job or another party's session reaches.
//
//craft:ignore naked-any SQL arguments take whichever Go type their column holds
func (s sweepSeed) ownerExec(sql string, args ...any) {
	s.t.Helper()
	if _, err := s.e.Owner.Exec(s.t.Context(), sql, args...); err != nil {
		s.t.Fatalf("seeding with %q: %v", sql, err)
	}
}

func uniqueEmail() string { return "sweep-" + ids.NewV7().String() + "@role-sweep.test" }

// peopleSeeders make colleagues in the states the member and access calls
// need: active, invited, deactivated, retiring, and on the admin's team.
func peopleSeeders(s sweepSeed) map[string]func() string {
	invited := func(role string, teams ...string) string {
		email := uniqueEmail()
		body := AnyMap{"email": email, "display_name": "Role Sweep", "role": role}
		if len(teams) > 0 {
			body["team_ids"] = teams
		}
		id := s.post("/v1/users", body)
		s.pool.note(id, "email", email)
		return id
	}
	// No route makes a seat active but the invitee's own password link, so
	// the seed stands in for the colleague who accepted.
	active := func(role string, teams ...string) string {
		id := invited(role, teams...)
		s.ownerExec(`UPDATE app_user SET status = 'active' WHERE id = $1`, id)
		var user struct {
			UpdatedAt string `json:"updated_at"`
		}
		s.call("GET", "/v1/users/"+id, nil, nil, &user)
		s.pool.note(id, "updated_at", user.UpdatedAt)
		return id
	}
	deactivated := func() string {
		id := invited("rep")
		s.call("POST", "/v1/users/"+id+"/deactivate", AnyMap{}, nil, nil)
		return id
	}
	preview := func(source string, query url.Values) string {
		var answer struct {
			Token string `json:"token"`
		}
		s.call("GET", "/v1/users/"+source+"/retirement-preview?"+query.Encode(), nil, nil, &answer)
		return answer.Token
	}
	return map[string]func() string{
		"user":             func() string { return active("rep") },
		"invited_user":     func() string { return invited("rep") },
		"deactivated_user": deactivated,
		"me":               func() string { return sessionUserID(s.t, s.e) },
		"retiring_user": func() string {
			id := deactivated()
			s.pool.note(id, "token", preview(id, url.Values{"mode": {"remove"}}))
			return id
		},
		"merge_source": func() string {
			id, into := invited("rep"), active("rep")
			s.pool.note(id, "to_user_id", into)
			s.pool.note(id, "token", preview(id, url.Values{"mode": {"merge"}, "to_user_id": {into}}))
			return id
		},
		"teammate": func() string { return teammate(s, active) },
		"teammate_plan": func() string {
			mate := teammate(s, active)
			s.asColleague(mate).call("POST", "/v1/weekly-plans/current", AnyMap{}, nil, nil)
			return mate
		},
		"teammate_commitment": func() string {
			colleague := s.asColleague(teammate(s, active))
			colleague.call("POST", "/v1/weekly-plans/current", AnyMap{}, nil, nil)
			return colleague.post("/v1/weekly-plans/commitments", AnyMap{"label": uniqueName()})
		},
		// A review is answered by someone other than the deal's owner, so the
		// teammate owns the deal and asks.
		"colleague_legal_review": func() string {
			mate := teammate(s, active)
			stages := apptest.DiscoverSeededPipeline(s.t, s.e)
			deal := s.post("/v1/deals", AnyMap{
				"name": "Role Sweep", "amount_minor": 100, "currency": "EUR", "pipeline_id": stages.PipelineID,
				"stage_id": stages.Open, "company_id": s.create["company"](), "source": "manual", "owner_id": mate,
			})
			return s.asColleague(mate).post("/v1/deals/"+deal+"/legal-reviews", AnyMap{})
		},
		"intro_ask": func() string { return introAsk(s, active) },
		"accepted_intro_ask": func() string {
			ask := introAsk(s, active)
			s.call("POST", "/v1/intro-requests/"+ask+"/decision", AnyMap{"decision": "accepted", "version": 1}, nil, nil)
			return ask
		},
	}
}

// teammate is an active manager on a team the admin is on.
func teammate(s sweepSeed, active func(role string, teams ...string) string) string {
	team := s.post("/v1/teams", AnyMap{"name": "Sweep " + ids.NewV7().String()})
	s.call("PUT", "/v1/teams/"+team+"/members/"+sessionUserID(s.t, s.e), AnyMap{}, nil, nil)
	id := active("manager", team)
	s.pool.note(id, "team", team)
	return id
}

// introAsk is a teammate's ask that the admin introduce them to a contact.
func introAsk(s sweepSeed, active func(role string, teams ...string) string) string {
	contact := s.create["contact"]()
	return s.asColleague(teammate(s, active)).post("/v1/contacts/"+contact+"/intro-requests", AnyMap{
		"introducer_user_id": sessionUserID(s.t, s.e), "route_type": "direct", "internal_reason": "Role sweep",
	})
}

// asColleague is the seed signed in as a colleague, for what only another
// seat files: an ask of the admin, a plan the admin leads. The colleague
// shares the admin's password, as no route sets another seat's.
func (s sweepSeed) asColleague(user string) sweepSeed {
	s.t.Helper()
	s.ownerExec(`UPDATE app_user SET password_hash = (SELECT password_hash FROM app_user WHERE email = $1) WHERE id = $2`, toolRoleUser, user)
	transport, ok := s.e.Client.Transport.(*http.Transport)
	jar, err := cookiejar.New(nil)
	if !ok || err != nil {
		s.t.Fatalf("the harness client cannot be copied for a second seat: %T, %v", s.e.Client.Transport, err)
	}
	colleague := *s.e
	colleague.Client = &http.Client{Transport: transport.Clone(), Jar: jar}
	s.e = &colleague
	s.call("POST", "/v1/auth/login", AnyMap{"email": s.pool.fact(user, "email"), "password": "correct-horse-battery"}, nil, nil)
	return s
}

// accessSeeders make the shares and ownership asks the access calls act on.
func accessSeeders(s sweepSeed) map[string]func() string {
	return map[string]func() string{
		"record_grant": func() string {
			return s.post("/v1/record-grants", AnyMap{
				"record_type": "company", "record_id": s.create["company"](), "subject_type": "user",
				"subject_id": s.create["user"](), "access": "read",
			})
		},
		"offered_company": func() string {
			company := s.create["company"]()
			s.call("POST", "/v1/records/company/"+company+"/ownership-offer", AnyMap{"to_id": s.create["user"]()}, nil, nil)
			return company
		},
		// Only a seat that may not already change the record asks for it. So
		// the ask is filed on a colleague's company while the admin is a rep.
		"requested_company": func() string {
			company := s.post("/v1/companies", AnyMap{"display_name": "Sweep " + ids.NewV7().String(), "source": "manual", "owner_id": s.create["user"]()})
			assignRoleTo(s.t, s.e, toolRoleUser, "rep")
			defer assignRoleTo(s.t, s.e, toolRoleUser, "admin")
			s.call("POST", "/v1/records/company/"+company+"/ownership-request", AnyMap{}, nil, nil)
			return company
		},
	}
}
