// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"strings"
)

// Which project a piece of correspondence belongs to, and how an installation
// that got the answer wrong — or never had one — is repaired.

// relinkActivitiesToProjects files the activities that were already on file
// under the project they belong to.
//
// It exists because posting an activity again does NOT re-link it. The create
// path is idempotent on source_system+source_id, and a replay returns the
// existing row before it reaches insertActivityLinks (activities/activity.go,
// logActivityInTx) — deliberately, because that path is a read with a read's
// row-scope gate, not a write. So an installation seeded before projects
// carried activities kept every timeline empty no matter how often it was
// reseeded, and only a seed from an empty database showed the links.
//
// The fix belongs here rather than in the replay path: /v1/activities/{id}/relink
// is the endpoint the product already offers for exactly this — associate an
// activity that exists with a record it belongs to. It is idempotent on
// (activity, entity_type, entity_id) and preserves the original source and
// captured_by, so re-running changes nothing and the provenance still reads as
// a seeded capture rather than a re-capture.
func relinkActivitiesToProjects(c *client, cfg demoConfig, refs pipelineRefs, seen map[string]seededActivity, mode runMode) error {
	if mode == modeDryRun {
		return nil
	}
	for i, act := range cfg.Activities {
		existing, ok := seen[fmt.Sprintf("act-%d", i)]
		if !ok || existing.ID == "" {
			// Not on file before this run, so the create above linked it.
			continue
		}
		want, move := projectRelinkFor(refs, act, existing)
		if !move {
			continue
		}
		// replace_existing_of_type, because at most one project link is
		// allowed per activity (uq_activity_link_project) — adding a second
		// would be refused, and the activity may be filed under the project
		// an older seeder picked by list order rather than by date.
		body := jsonBody{
			"entity_type":              "project",
			"entity_id":                want,
			"replace_existing_of_type": true,
		}
		if err := c.post("/v1/activities/"+existing.ID+"/relink", body, nil); err != nil {
			return fmt.Errorf("filing activity %d (%s on %s) under its project: %w", i, act.Kind, act.Company, err)
		}
	}
	return nil
}

// projectRelinkFor decides whether an activity already on file has to be
// moved, and to which project.
//
// Separate from the loop that performs it because the decision is the part
// worth testing: every relink is a write on a surface that stamps a six-year
// retention class the database will not let anyone lift, so a pass that moved
// what was already right would write for no reason and stamp records that had
// no business being stamped.
func projectRelinkFor(refs pipelineRefs, act demoActivity, existing seededActivity) (projectID string, move bool) {
	want := projectForActivity(refs, act)
	if want == "" {
		// Older than every project on the account, so it belongs to none.
		// Inventing a link here is worse than leaving it: the stamp is
		// permanent and unfiling does not lift it.
		return "", false
	}
	if existing.ProjectID == want {
		return "", false
	}
	return want, true
}

// projectForActivity picks which of a company's projects a mail, call or
// meeting belongs to: the LATEST one that had already started when it
// happened.
//
// The company's projects arrive oldest first, so this walks forward and keeps
// the last one still in the past. Simply taking the first project a company
// had put all nine of valantic's mails about the Shopsystem migration onto
// "Zweiter Mandant im Shopsystem", which starts nine months after the last of
// them — a timeline that could not have happened.
//
// An activity older than every project links to none. That is the honest
// answer: correspondence from before any delivery work began was not about
// delivery work.
func projectForActivity(refs pipelineRefs, act demoActivity) string {
	projects := refs.projectsByCompany[strings.ToLower(act.Company)]
	if len(projects) == 0 {
		return ""
	}
	// The same offset seedActivities computes, as a date, so it compares
	// against started_at on its own terms.
	occurred := -act.DaysAgo
	if act.DaysIn > 0 {
		occurred = act.DaysIn
	}
	when := refs.date(occurred)

	chosen := ""
	for _, proj := range projects {
		// A project with no start date cannot be dated against, and sorts
		// last; it is only reached when nothing better has been found.
		if proj.StartedAt == "" || proj.StartedAt > when {
			continue
		}
		chosen = proj.ID
	}
	return chosen
}
