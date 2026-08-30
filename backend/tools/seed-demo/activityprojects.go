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
	ambiguous := ambiguousEntries(cfg, true)
	for i, act := range cfg.Activities {
		// On file before this run, and really the row this entry is about.
		// Both halves in one place (seededMatch), shared with the person pass:
		// the question is one question, and getting it wrong costs the same
		// either way.
		existing, ok := seededMatch(refs, act, seen, ambiguous, i)
		if !ok {
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
		// Pinned to the version this snapshot read, for the reason
		// relinkPinned gives: the decision and the write are separated by the
		// rest of the pass, and this one deletes the project link it finds.
		if err := relinkPinned(c, existing, body); err != nil {
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
	// Dated against the record, not the dataset — see projectForActivityOn.
	// An activity on file whose occurred_at the server did not return cannot
	// be dated at all, and guessing is what this function exists to avoid.
	when, _, found := strings.Cut(existing.OccurredAt, "T")
	if !found || when == "" {
		return "", false
	}
	want := projectForActivityOn(refs, act.Company, when)
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
	// The offset the dataset gives, as a date. Right for an activity being
	// written now, because that is the same clock the create call uses.
	occurred := -act.DaysAgo
	if act.DaysIn > 0 {
		occurred = act.DaysIn
	}
	return projectForActivityOn(refs, act.Company, refs.date(occurred))
}

// projectForActivityOn is projectForActivity against a date the caller names.
//
// The reconciliation pass needs this because it must NOT use the dataset's
// offset. days_ago is relative to the day the seeder runs, while the
// occurred_at on file was frozen by the first run and never moves again — so
// on any later day the two disagree, and a pass that dated by the offset would
// decide an unchanged activity now belongs to a different project. It would
// then relink it, which stamps six-year retention that cannot be lifted.
func projectForActivityOn(refs pipelineRefs, company, when string) string {
	projects := refs.projectsByOrg[refs.orgForDomain(company)]
	if len(projects) == 0 {
		return ""
	}

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
