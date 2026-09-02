// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The surfaces a worked CRM has beyond its records: the tags people filter by,
// the lists they save, the views each seat keeps, the fields somebody added
// because the product did not ship one, and the projects delivery runs.
//
// These are the screens a demo reaches after the obvious ones. Empty, they
// teach that the product has no such feature — a Tags page with nothing on it
// looks identical to a Tags page that does not work.
//
// Everything here is workspace-wide rather than per-company, so the phase is
// small and runs once. What varies per company is which records get tagged,
// and that comes from the plan like everything else.

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// seedSurfaces creates the collection and customisation records, and returns
// how many were new.
func seedSurfaces(c *client, seats *sessions, cfg demoConfig, refs pipelineRefs, plan map[string]profile, mode runMode) (int, error) {
	created := 0
	for _, step := range []struct {
		what string
		run  func() (int, error)
	}{
		{"tags", func() (int, error) { return seedTags(c, refs, plan, mode) }},
		// Projects themselves are NOT here — they are created earlier, before
		// seedActivities, because an activity links to the project it was
		// about and cannot link to a row that does not exist yet.
		//
		// Staffing them stays here, and reads them back rather than taking
		// ids from the pass that made them: a re-run creates no project, so a
		// pass that staffed only what it had just created would leave every
		// earlier project empty forever.
		{"project stakeholders", func() (int, error) { return seedProjectStakeholders(c, mode) }},
		{"quotas", func() (int, error) { return seedQuotas(c, cfg, refs, mode) }},
	} {
		n, err := step.run()
		if err != nil {
			return created, fmt.Errorf("%s: %w", step.what, err)
		}
		created += n
	}
	return created, nil
}

// demoTags are the labels a sales team actually keeps. Each one has to be
// applied to something: a tag nobody uses is a row, not a demonstration.
var demoTags = []struct {
	Name  string
	Color string
	// Applies decides which companies carry this tag, from their profile.
	Applies func(profile) bool
}{
	{"Key Account", "#b45309", func(p profile) bool { return p.Lifecycle == "customer" }},
	{"Churn Risk", "#b91c1c", func(p profile) bool { return p.Lifecycle == "former_customer" }},
	{"Inbound", "#1d4ed8", func(p profile) bool { return p.LeadState != "" }},
	// `parked` is not a lifecycle the product carries, so the planner maps it
	// to `target` and this tag is what preserves the distinction.
	{"Parked", "#6b7280", func(p profile) bool { return p.Lifecycle == "target" && p.DealStage == "" }},
}

func seedTags(c *client, refs pipelineRefs, plan map[string]profile, mode runMode) (int, error) {
	created := 0
	existing, err := tagsByName(c, mode)
	if err != nil {
		return 0, err
	}
	for _, tag := range demoTags {
		id := existing[tag.Name]
		if id == "" {
			if mode == modeDryRun {
				created++
				continue
			}
			var out struct {
				ID string `json:"id"`
			}
			if err := c.post("/v1/tags", jsonBody{"name": tag.Name, "color": tag.Color}, &out); err != nil {
				if _, conflict := conflictingID(err); !conflict {
					return created, fmt.Errorf("tag %q: %w", tag.Name, err)
				}
			} else {
				created++
				id = out.ID
			}
		}
		if id == "" || mode == modeDryRun {
			continue
		}
		// Applying is idempotent server-side, so a re-run re-asserts the same
		// label rather than duplicating it.
		for _, domain := range sortedDomains(plan) {
			if !tag.Applies(plan[domain]) {
				continue
			}
			orgID, ok := refs.orgsByDom[domain]
			if !ok {
				continue
			}
			body := jsonBody{"entity_type": "organization", "entity_id": orgID}
			if err := c.post("/v1/tags/"+id+"/apply", body, nil); err != nil && !isConflict(err) {
				return created, fmt.Errorf("applying %q to %s: %w", tag.Name, domain, err)
			}
		}
	}
	return created, nil
}

func tagsByName(c *client, mode runMode) (map[string]string, error) {
	out := map[string]string{}
	if mode == modeDryRun {
		return out, nil
	}
	err := c.getAll("/v1/tags", nil, func(raw json.RawMessage) error {
		var rows []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return err
		}
		for _, row := range rows {
			out[row.Name] = row.ID
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	return out, nil
}

// Custom fields are NOT seeded, and cannot be.
//
// POST /v1/custom-fields answers 501:
//
//	operation custom-field schema changes is specified but not yet implemented
//
// The contract describes the add-field engine in full — picklists with
// options, currency fields, the retire path — and the handler is a stub. So
// the Settings > Custom fields screen stays empty in the demo, and that is
// the product's own state rather than a hole in the seeder. The code that
// would fill it is deleted rather than commented out; this note is the
// record, and the day the handler lands it is three POSTs to write.

// seedQuotas gives every seller a target for the current quarter, so the
// attainment the product computes from closed-won deals has something to be a
// percentage of.
func seedQuotas(c *client, cfg demoConfig, refs pipelineRefs, mode runMode) (int, error) {
	created := 0
	start, end := currentQuarter(refs)
	existing := map[string]bool{}
	if mode != modeDryRun {
		query := url.Values{"period_start": {start}}
		err := c.getAll("/v1/quotas", query, func(raw json.RawMessage) error {
			var rows []struct {
				OwnerID string `json:"owner_id"`
			}
			if err := json.Unmarshal(raw, &rows); err != nil {
				return err
			}
			for _, row := range rows {
				existing[row.OwnerID] = true
			}
			return nil
		})
		if err != nil {
			return 0, fmt.Errorf("listing quotas: %w", err)
		}
	}
	for _, ref := range sellerIDs(cfg, refs) {
		ownerID, ok := refs.usersByRef[ref]
		if !ok || existing[ownerID] {
			continue
		}
		if mode == modeDryRun {
			created++
			continue
		}
		body := jsonBody{
			"owner_id":     ownerID,
			"period_start": start,
			"period_end":   end,
			// A round quarterly target, varied per seat so the attainment bars
			// are not all the same length.
			// money-scale-exempt: this MINTS a euro figure rather than converting
			// a stored one, and the currency it is minted for is the literal on
			// the very next line. There is no amount here whose scale a table
			// could be asked about.
			"target_minor": int64(150000+hashIndex("quota:"+ref, 12)*25000) * 100, // money-scale-exempt: minted in EUR, see above
			"currency":     "EUR",
		}
		if err := c.post("/v1/quotas", body, nil); err != nil {
			if _, conflict := conflictingID(err); conflict {
				continue
			}
			return created, fmt.Errorf("quota for %s: %w", ref, err)
		}
		created++
	}
	return created, nil
}

// currentQuarter is the quarter the run happens in, so a demo seeded today
// shows a target somebody is currently working against.
func currentQuarter(refs pipelineRefs) (start, end string) {
	now := refs.now
	quarter := (int(now.Month()) - 1) / 3
	first := now.AddDate(0, -(int(now.Month()) - 1 - quarter*3), -(now.Day() - 1))
	return first.Format("2006-01-02"), first.AddDate(0, 3, -1).Format("2006-01-02")
}
