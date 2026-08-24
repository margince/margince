// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// seedProjects files the delivery work a customer's won deal turned into.
//
// A project is born in its first phase and ADVANCED, like a deal: the phase a
// project is in is the record of what happened to it, not a column you can be
// created in.
//
// Two passes, in this order, because they answer different questions. The
// DATASET pass writes the projects demo.json names: a real project name, a
// description, and the phase the story reaches. The PLAN pass then fills the
// long tail from the profile weights, with the derived "<Company> —
// Einführung" name, for the ~180 crawled companies the dataset says nothing
// about. Dataset first so that when both name the same company the told story
// wins and the generated one is skipped as already-existing.
func seedProjects(c *client, cfg demoConfig, refs pipelineRefs, plan map[string]profile, mode runMode) (int, error) {
	created := 0
	existing, err := projectIndex(c, mode)
	if err != nil {
		return 0, err
	}
	n, err := seedDatasetProjects(c, cfg, refs, existing, mode)
	if err != nil {
		return created, err
	}
	created += n

	for _, domain := range sortedDomains(plan) {
		p := plan[domain]
		if p.Pinned || p.Project == "" {
			continue
		}
		orgID, ok := refs.orgsByDom[domain]
		if !ok {
			continue
		}
		name := projectNameFor(localeFor(domain), refs.orgNameByID[orgID])
		index := projectIndexKey(orgID, name)
		if existing[index] {
			continue
		}
		// Claimed in the SAME map the snapshot filled, because the index is no
		// longer unique per plan entry: two domains that resolve to one
		// organization derive one name, and a snapshot taken before the loop
		// cannot see the project the loop itself just created. The old key was
		// per-domain and could not collide, so this is the cost of indexing by
		// what the server leaves us.
		existing[index] = true
		if mode == modeDryRun {
			created++
			continue
		}
		body := projectCreateBody(orgID, name, refs.date(-(30 + hashIndex("projstart:"+domain, 180))))
		if owner, ok := refs.usersByRef[refs.ownerRefByDomain[domain]]; ok {
			body["owner_id"] = owner
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := c.post("/v1/projects", body, &out); err != nil {
			if _, conflict := conflictingID(err); conflict {
				continue
			}
			return created, fmt.Errorf("project for %s: %w", domain, err)
		}
		created++
		if err := advanceProject(c, out.ID, p.Project, domain); err != nil {
			return created, err
		}
	}
	return created, nil
}

// seedDatasetProjects writes the projects demo.json names.
//
// It does NOT consult the profile plan, which is the point: the plan only
// gives a project to a company its weights made a `customer`, and it skips
// every pinned company outright (surfaces.go, the Pinned guard below) — which
// is exactly the golden accounts the dataset is written about. A project named
// here is created because the dataset says this account has one.
func seedDatasetProjects(c *client, cfg demoConfig, refs pipelineRefs, existing map[string]bool, mode runMode) (int, error) {
	created := 0
	for i, proj := range cfg.Projects {
		domain := strings.ToLower(strings.TrimSpace(proj.Company))
		orgID, ok := refs.orgsByDom[domain]
		if !ok {
			return created, fmt.Errorf("project %d (%s) names company %q, which is not seeded", i, proj.Ref, proj.Company)
		}
		// Trimmed, because the server trims a project's name before storing
		// it (projects/handlers.go). Converging on the untrimmed spelling
		// would index " Portal " against a row stored as "Portal", miss it on
		// the next run, and file a second project.
		name := strings.TrimSpace(proj.Name)
		if name == "" {
			return created, fmt.Errorf("project %d (%s) has no name", i, proj.Ref)
		}
		index := projectIndexKey(orgID, name)
		if existing[index] {
			continue
		}
		existing[index] = true
		if mode == modeDryRun {
			created++
			continue
		}
		body := projectCreateBody(orgID, name, refs.date(proj.StartedInDays))
		addIfSet(body, "description", proj.Description)
		// The dataset may name the owner; otherwise the account's owner keeps
		// their own delivery work, which is what resolveOwners already decided.
		ownerRef := proj.Owner
		if ownerRef == "" {
			ownerRef = refs.ownerRefByDomain[domain]
		}
		if owner, ok := refs.usersByRef[ownerRef]; ok {
			body["owner_id"] = owner
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := c.post("/v1/projects", body, &out); err != nil {
			if _, conflict := conflictingID(err); conflict {
				continue
			}
			return created, fmt.Errorf("project %s: %w", proj.Ref, err)
		}
		created++
		if err := advanceProject(c, out.ID, proj.Phase, domain); err != nil {
			return created, err
		}
	}
	return created, nil
}

// projectPhaseOrder is the sequence a project walks. A phase is reached by
// stepping through the ones before it, so the history reads as work rather
// than as an assertion.
var projectPhaseOrder = []string{"initiative", "pursuing", "delivering", "closed"}

func advanceProject(c *client, projectID, want, domain string) error {
	// A phase this ladder does not contain would otherwise walk the whole way
	// and stop at `closed`, so a dataset typo — "deliverng", or an empty
	// string — would silently close a project that is still running. Say so
	// instead.
	if !isProjectPhase(want) {
		return fmt.Errorf("project for %s wants phase %q, which is not one of %v", domain, want, projectPhaseOrder)
	}
	for _, phase := range projectPhaseOrder {
		body := jsonBody{"to_phase": phase}
		if phase == "closed" {
			// Closing needs a reason, the same way losing a deal does — and
			// it is written in the account's language, not always in German.
			body["reason"] = projectCloseReason(localeFor(domain))
		}
		if err := c.post("/v1/projects/"+projectID+"/advance", body, nil); err != nil && !isConflict(err) {
			return fmt.Errorf("advancing %s to %s: %w", domain, phase, err)
		}
		if phase == want {
			return nil
		}
	}
	return nil
}

// isProjectPhase reports whether a phase is one advanceProject can arrive at.
func isProjectPhase(phase string) bool {
	for _, rung := range projectPhaseOrder {
		if rung == phase {
			return true
		}
	}
	return false
}

func projectNameFor(locale docLocale, company string) string {
	switch locale {
	case localeVI:
		return company + " — Trien khai"
	case localeEN:
		return company + " — Rollout"
	default:
		return company + " — Einführung"
	}
}

// projectCreateBody is the create payload, in one place so what it does NOT
// carry is visible: no "key". The server mints a project's key from its name and
// refuses a caller who sends one (422 read_only, projects/keymint.go) — the key
// is the token a person types in a subject line to file mail under a project, so
// it is not a demo's to choose.
//
// Nothing else may be added loosely either: this contract takes extra body
// properties as CUSTOM FIELD values, so a stray key is not ignored here, it is
// data. Every field below is one the contract declares.
func projectCreateBody(orgID, name, startedAt string) jsonBody {
	return jsonBody{
		"name":            name,
		"organization_id": orgID,
		"source":          seedSource,
		"started_at":      startedAt,
	}
}

// projectIndexKey identifies a seeded project by what the seeder still controls.
//
// Convergence used to ask "does a project with MY key exist", which is a
// question a caller who no longer mints the key cannot ask. Organization plus
// name is the pair the seeder derives deterministically (projectNameFor over the
// company name), so a second run recognises its own work and creates nothing —
// and two companies with the same project name stay two projects.
func projectIndexKey(orgID, name string) string {
	return orgID + "\x00" + name
}

func projectIndex(c *client, mode runMode) (map[string]bool, error) {
	out := map[string]bool{}
	if mode == modeDryRun {
		return out, nil
	}
	err := c.getAll("/v1/projects", nil, func(raw json.RawMessage) error {
		var rows []struct {
			Name           string `json:"name"`
			OrganizationID string `json:"organization_id"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return err
		}
		for _, row := range rows {
			out[projectIndexKey(row.OrganizationID, row.Name)] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	return out, nil
}

// loadProjects records the id of every seeded project, keyed by its company,
// so an activity can link to the delivery work it describes.
//
// Like loadDeals it reads the installation's OWN projects rather than walking
// demo.json, because the generated projects are projects too: a company the
// dataset does not name still has delivery work, and its correspondence
// should reach it.
func (r *pipelineRefs) loadProjects(c *client) error {
	domainByOrg := map[string]string{}
	for domain, orgID := range r.orgsByDom {
		domainByOrg[orgID] = domain
	}
	r.projectsByCompany = map[string][]seededProject{}
	byDomain := map[string][]seededProject{}
	if err := c.getAll("/v1/projects", nil, func(raw json.RawMessage) error {
		var rows []struct {
			ID             string `json:"id"`
			OrganizationID string `json:"organization_id"`
			StartedAt      string `json:"started_at"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return err
		}
		for _, row := range rows {
			domain, ok := domainByOrg[row.OrganizationID]
			if !ok {
				continue
			}
			byDomain[domain] = append(byDomain[domain], seededProject{ID: row.ID, StartedAt: row.StartedAt})
		}
		return nil
	}); err != nil {
		return err
	}
	// Oldest first, and that order is load-bearing rather than tidiness.
	//
	// A company with two projects gets its correspondence filed against the
	// FIRST one an activity could plausibly be about, and activityLinks takes
	// the head of this slice. Taking whatever order the API happened to
	// return put every one of valantic's nine mails about the Shopsystem
	// migration onto "Zweiter Mandant im Shopsystem" — a project that starts
	// nine months AFTER the last of them, so the timeline it built was one
	// that could not have happened.
	//
	// A project with no start date sorts last: it is the one an activity has
	// the weakest claim to belong to.
	for domain, projects := range byDomain {
		sort.Slice(projects, func(i, j int) bool {
			a, b := projects[i].StartedAt, projects[j].StartedAt
			if (a == "") != (b == "") {
				return b == ""
			}
			if a != b {
				return a < b
			}
			return projects[i].ID < projects[j].ID
		})
		r.projectsByCompany[domain] = projects
	}
	return nil
}

// seededProject is one project as loadProjects read it back, kept only long
// enough to order a company's projects by when they started.
type seededProject struct {
	ID        string
	StartedAt string
}

// seedDeliveryAndItsRecord writes the projects and then the activities, in
// that order.
//
// The order is the whole point. An activity links to the project it was about
// — the kickoff meeting, the acceptance mail, the escalation call — and cannot
// link to a row that does not exist yet. Projects used to be created inside
// seedSurfaces, several phases after the activities were written, which is why
// not one project in the demo carried a single activity.
func seedDeliveryAndItsRecord(c *client, seats *sessions, cfg demoConfig, refs *pipelineRefs, plan map[string]profile, mode runMode) (projects, activities int, err error) {
	projects, err = seedProjectsAndIndexThem(c, cfg, refs, plan, mode)
	if err != nil {
		return projects, 0, err
	}
	activities, err = seedActivities(c, seats, cfg, *refs, mode)
	if err != nil {
		return projects, activities, err
	}
	return projects, activities, nil
}

// seedProjectsAndIndexThem writes the projects and then reads them back, so
// the activities that follow can link to the delivery work they describe.
//
// The read-back is separate from the write on purpose: a converging re-run
// creates no project at all, and a pass that only knew what it had just
// created would leave every project from an earlier run unreachable to the
// activities forever.
func seedProjectsAndIndexThem(c *client, cfg demoConfig, refs *pipelineRefs, plan map[string]profile, mode runMode) (int, error) {
	projects, err := seedProjects(c, cfg, *refs, plan, mode)
	if err != nil {
		return projects, err
	}
	if mode == modeDryRun {
		return projects, nil
	}
	if err := refs.loadProjects(c); err != nil {
		return projects, err
	}
	return projects, nil
}
