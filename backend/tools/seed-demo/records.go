// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// What HAPPENED on a company: the correspondence, calls, meetings and tasks,
// and who each of them was with.
//
// Where an account STANDS — its lifecycle, the rate card, the offers on the
// table — is next door in standing.go. The two split because this file grew
// past the 500-line cap, and the seam was already there: these phases record
// events, those ones record state.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// seedActivities files the correspondence, calls, meetings and tasks.
//
// Every write carries source_system + source_id, which the activity API
// treats as an idempotency key — so this phase converges without a probe of
// its own, and a re-run neither duplicates a thread nor re-opens a task.
func seedActivities(c *client, seats *sessions, cfg demoConfig, refs pipelineRefs, mode runMode) (int, error) {
	created := 0
	// One read for the phase: which source ids already exist. Counting what
	// this run genuinely created needs the before-state, and asking per
	// activity was the seeder's worst quadratic.
	// Who each entry's conversation was with, by dataset position, filled as
	// the loop below resolves it. The reconciliation reads this rather than
	// asking again.
	counterparts := map[int]string{}
	seenSourceIDs := map[string]seededActivity{}
	if mode != modeDryRun {
		loaded, err := loadActivitySourceIDs(c)
		if err != nil {
			return 0, err
		}
		seenSourceIDs = loaded
	}
	for i, act := range cfg.Activities {
		orgID, ok := refs.orgsByDom[strings.ToLower(act.Company)]
		if !ok {
			return created, fmt.Errorf("activity %d names company %q, which is not seeded", i, act.Company)
		}
		if mode == modeDryRun {
			created++
			continue
		}
		// WHO writes this is the point, not a detail. The product records the
		// author as a participant, and the network view reads participants to
		// answer "who on our team knows this contact". Posting everything as
		// one account makes that account know everybody.
		author := seats.as(handlerOf(act, cfg, refs))

		// The counterpart comes back with the body and is KEPT, rather than
		// re-derived by the reconciliation below. Who a conversation was with
		// has to be ONE answer: the repair pass decides by comparing what is on
		// file against it, and a second derivation that disagreed would relink
		// activities nothing had changed — on a surface whose retention stamp
		// the database will not let anyone lift. It is also two paginated reads
		// per activity that nobody needs twice.
		body, counterpart, err := activityBody(c, refs, act, orgID, i)
		if err != nil {
			return created, err
		}
		counterparts[i] = counterpart

		// Idempotent on source_system+source_id, so a re-run replays the same
		// row and the reply cannot tell a create from a convergence. The
		// source ids present before this phase say what was genuinely absent.
		_, before := seenSourceIDs[fmt.Sprintf("act-%d", i)]
		if err := author.post("/v1/activities", body, nil); err != nil {
			if _, ok := conflictingID(err); ok {
				continue
			}
			if isNotFound(err) && act.Assignee != "" {
				// Row scope hides an account from anyone outside the team that
				// owns it, and hiding it means 404 rather than 403 — existence
				// is not leaked. So a dataset entry naming a colleague on the
				// wrong team reads as a missing company, which sends the reader
				// looking for the wrong bug.
				return created, fmt.Errorf(
					"activity %d: %s cannot see %s — a colleague can only be named on an account their own team owns",
					i, act.Assignee, act.Company)
			}
			return created, fmt.Errorf("activity %d (%s on %s): %w", i, act.Kind, act.Company, err)
		}
		if !before {
			created++
		}
	}
	if err := relinkActivitiesToProjects(c, cfg, refs, seenSourceIDs, mode); err != nil {
		return created, err
	}
	if err := relinkActivitiesToPeople(c, cfg, refs, seenSourceIDs, counterparts, mode); err != nil {
		return created, err
	}
	return created, nil
}

// activityBody is the write for one dataset entry: which kind it survives as,
// who and what it links, and the fields that kind is allowed to carry.
func activityBody(c *client, refs pipelineRefs, act demoActivity, orgID string, i int) (jsonBody, string, error) {
	// One of the two offsets is set: DaysAgo for something that happened,
	// DaysIn for something still to come.
	occurred := -act.DaysAgo
	if act.DaysIn > 0 {
		occurred = act.DaysIn
	}
	// Who the activity was with, when it was with anybody — needed both to
	// pick the kind actually filed and to link that person below.
	personID, err := counterpartFor(c, act, orgID)
	if err != nil {
		return nil, "", fmt.Errorf("activity %d on %s: %w", i, act.Company, err)
	}
	// A meeting or a call the database can reach through nobody there is
	// filed as a note about the account instead — activity_link_no_company_meeting
	// refuses a direct company link on either kind, and without an attendee
	// that link is the only way the company timeline would ever see it.
	effectiveKind := act.Kind
	if (act.Kind == "meeting" || act.Kind == "call") && personID == "" {
		effectiveKind = "note"
	}
	// An activity links to every record it touched rather than belonging to
	// one, so a mail is one row that appears on the company, on the person it
	// was with, and on the deal it moved. Linking only the company — which is
	// what this did first — leaves every person's timeline empty, which is
	// where a rep actually looks.
	links := activityLinks(effectiveKind, personID, orgID, refs, act)
	body := jsonBody{
		"kind":          effectiveKind,
		"occurred_at":   refs.timestamp(occurred),
		"source":        seedSource,
		"source_system": seedSourceSystem,
		"source_id":     fmt.Sprintf("act-%d", i),
		"links":         links,
	}
	addIfSet(body, "subject", act.Subject)
	addIfSet(body, "body", act.Body)
	addIfSet(body, "direction", act.Direction)
	// field_not_valid_for_kind: only a kind that survived as "meeting" above
	// may carry meeting_status — a downgraded note may not.
	if effectiveKind == "meeting" {
		addIfSet(body, "meeting_status", act.MeetingStatus)
	}
	if act.DurationSeconds > 0 {
		body["duration_seconds"] = act.DurationSeconds
	}
	// assignee_id and due_at belong to a TASK and to nothing else — the
	// activity_task_fields CHECK refuses them on a mail or a meeting, because
	// those record what happened rather than what somebody owes. Who handled
	// the others is carried by the record's owner instead.
	if act.Kind == "task" {
		if assignee, ok := refs.usersByRef[act.Assignee]; ok {
			body["assignee_id"] = assignee
		}
		if act.DaysIn > 0 {
			body["due_at"] = refs.timestamp(act.DaysIn)
		}
	}
	return body, personID, nil
}

// handlerOf is the colleague who had this conversation.
//
// Derived rather than configured, so a company ingested next month is covered
// without anyone editing a list: the dataset may name an assignee, and
// otherwise it is whoever owns the account. Both fall back to the seeding
// account, which is what happens for a company nobody has been assigned yet.
func handlerOf(act demoActivity, cfg demoConfig, refs pipelineRefs) demoUser {
	wanted := act.Assignee
	if wanted == "" {
		wanted = refs.ownerRefByDomain[strings.ToLower(act.Company)]
	}
	for _, user := range cfg.Users {
		if user.Ref == wanted {
			return user
		}
	}
	return demoUser{}
}

// activityLinks is what one activity touched: its company or its counterpart
// — never both on a meeting or a call, per the database's own rule — plus any
// open deal there.
//
// The counterpart is derived rather than listed, because a dataset that named
// it per activity would have to be rewritten for every company ingested
// later; effectiveKind and personID are decided by the caller, which already
// downgraded a meeting or call reached through nobody to a note.
func activityLinks(effectiveKind, personID, orgID string, refs pipelineRefs, act demoActivity) []jsonBody {
	var links []jsonBody

	// activity_link_no_company_meeting refuses a direct company link on a
	// meeting or a call: the company is reached through the attendee instead.
	// Every other kind still links it directly — a note or a task is about the
	// account rather than with anybody, an email may legitimately have no
	// single attendee, and a meeting or call reached through nobody has
	// already become a note, above.
	if effectiveKind != "meeting" && effectiveKind != "call" {
		links = append(links, jsonBody{"entity_type": "organization", "entity_id": orgID})
	}
	if personID != "" {
		links = append(links, jsonBody{"entity_type": "person", "entity_id": personID})
	}

	for _, deal := range refs.dealsByOrg[refs.orgForDomain(act.Company)] {
		links = append(links, jsonBody{"entity_type": "deal", "entity_id": deal})
		break // one deal: an account with two is ambiguous, and guessing wrong is worse than not guessing
	}
	// And the delivery work, so a project has a timeline rather than a start
	// date and nothing else. One project per activity is not a style choice
	// here: uq_activity_link_project makes it a database constraint.
	if project := projectForActivity(refs, act); project != "" {
		links = append(links, jsonBody{"entity_type": "project", "entity_id": project})
	}
	return links
}

// counterpartFor answers WHO at the customer an activity was with.
//
// The dataset's own answer wins. `person` names somebody by their full name,
// and that is the only way the record can agree with what the mail SAYS: the
// body is signed by a named human, and until this existed the link went to
// whoever sorted most senior instead. A mail signed "Karoline Juettner" filed
// against the Geschaeftsfuehrer is not a near-miss — asked who complained, an
// assistant reads the signature and answers with a name the CRM does not hold.
//
// A name the account does not employ is an ERROR rather than a silent
// fallback. Getting the senior contact when the dataset asked for a specific
// person would restore the exact defect this closes, and quietly.
//
// Without `person` the old behaviour stands: the most senior employee, which
// is a heuristic and better than an empty timeline for the ~180 companies the
// dataset never names.
func counterpartFor(c *client, act demoActivity, orgID string) (string, error) {
	// A note or a task is INTERNAL — it is about the account, not with
	// anybody. The ruling lives here rather than at the call site because
	// three readers make it: the create, the repair pass, and the post-seed
	// check. A repair that thought a task had a counterpart would file one
	// against every note in the demo.
	if !isConversation(act.Kind) {
		return "", nil
	}
	staff, err := staffBySeniority(c, orgID)
	if err != nil {
		return "", err
	}
	if act.Person == "" {
		if len(staff) == 0 {
			return "", nil
		}
		return staff[0], nil
	}
	// Every match, not the first. staffBySeniority orders by title, so taking
	// the first would hand the activity to the more SENIOR of two colleagues
	// who share a name — silently, and that is the same wrong-person link this
	// function exists to stop. Two people called "Alex Kim" at one account is
	// a question the dataset has to answer, not one to guess at.
	var matched []string
	for _, personID := range staff {
		name, err := personName(c, personID)
		if err != nil {
			return "", err
		}
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(act.Person)) {
			matched = append(matched, personID)
		}
	}
	switch len(matched) {
	case 1:
		return matched[0], nil
	case 0:
		return "", fmt.Errorf(
			"activity on %s names person %q, who is not employed there — the body's sign-off and the dataset have to agree",
			act.Company, act.Person)
	default:
		return "", fmt.Errorf(
			"activity on %s names person %q, and %d people there answer to it — the dataset cannot say which, so it must not guess",
			act.Company, act.Person, len(matched))
	}
}

// personNames caches a person's name for the run.
//
// The lookup is per ACTIVITY and a story account carries a dozen of them
// against the same handful of colleagues, so without this the same rows are
// re-read tens of times. A person's name does not change mid-run.
var personNames = map[string]string{}

func personName(c *client, personID string) (string, error) {
	if name, ok := personNames[personID]; ok {
		return name, nil
	}
	var out struct {
		FullName string `json:"full_name"`
	}
	if err := c.get("/v1/people/"+personID, nil, &out); err != nil {
		return "", fmt.Errorf("reading person %s: %w", personID, err)
	}
	personNames[personID] = out.FullName
	return out.FullName, nil
}

// loadActivitySourceIDs reads the source_id of every activity ONCE.
//
// It replaces a per-activity search that listed activities on every call and
// stopped at 200 rows. Both halves were bugs waiting for scale: the search was
// O(activities²) over a run, and the cap meant activity 201 was never found,
// so a converging re-run filed a duplicate of it instead of recognising it.
func loadActivitySourceIDs(c *client) (map[string]seededActivity, error) {
	seen := map[string]seededActivity{}
	err := c.getAll("/v1/activities", nil, func(raw json.RawMessage) error {
		return indexSeededActivities(raw, seen)
	})
	if err != nil {
		return nil, fmt.Errorf("listing activities: %w", err)
	}
	return seen, nil
}

// indexSeededActivities adds one page of activities to the index, keeping only
// the rows this tool captured.
func indexSeededActivities(raw json.RawMessage, seen map[string]seededActivity) error {
	var rows []struct {
		ID           string `json:"id"`
		SourceSystem string `json:"source_system"`
		SourceID     string `json:"source_id"`
		OccurredAt   string `json:"occurred_at"`
		Subject      string `json:"subject"`
		Version      int    `json:"version"`
		Links        []struct {
			EntityType string `json:"entity_type"`
			EntityID   string `json:"entity_id"`
		} `json:"links"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return err
	}
	for _, row := range rows {
		// BOTH halves of the key, because source_id alone is not one.
		// The database is unique on (source_system, source_id), and the
		// seeder's own ids are "act-0", "act-1" — spellings a connector
		// is free to use too. Keying on the id alone once only miscounted
		// how many rows a run created; now that this map decides which
		// activity gets relinked, the same collision would file somebody
		// else's mail under a demo project and stamp it with six-year
		// retention that cannot be lifted.
		if row.SourceSystem != seedSourceSystem || row.SourceID == "" {
			continue
		}
		found := seededActivity{
			ID: row.ID, OccurredAt: row.OccurredAt,
			Subject: row.Subject, Version: row.Version,
		}
		for _, link := range row.Links {
			switch link.EntityType {
			case "project":
				found.ProjectID = link.EntityID
			case "organization":
				found.OrganizationID = link.EntityID
			case "person":
				// EVERY one, not the first. This tool writes exactly one
				// person link, so a second is somebody else's fact — and the
				// repair has to be able to see there is one before it
				// replaces anything.
				found.PersonIDs = append(found.PersonIDs, link.EntityID)
			}
		}
		seen[row.SourceID] = found
	}
	return nil
}

// seededActivity is one activity already on file, as the reconciliation pass
// needs to see it: its id, so it can be relinked, and the project it is filed
// under, so a pass that has nothing to do does nothing.
type seededActivity struct {
	ID        string
	ProjectID string
	// PersonIDs are the people this activity is already linked to, in the
	// order the server returned them. A LIST, because the count is the answer
	// the person repair turns on: this tool writes one, so anything else came
	// from somewhere it must not overwrite.
	PersonIDs []string
	// Subject is what the stored row says it is about, and half of this row's
	// IDENTITY. Source ids here are positional, so reordering the dataset
	// renames every row after the insertion — within one company the
	// organization check cannot see that, and the subject can.
	Subject string
	// Version is what the row carried when this snapshot was read. It goes
	// back to the server as If-Match on any repair, so a row somebody has
	// touched since is refused rather than overwritten from stale state.
	Version int
	// OccurredAt is when the activity says it happened, as the server stored
	// it. The reconciliation dates against THIS rather than against the
	// dataset's days_ago offset: the offset is relative to the day the seeder
	// runs, and occurred_at was frozen on the first run and never moves after
	// it, so on any later day the two disagree.
	OccurredAt string
	// OrganizationID is the account this activity is filed on. The
	// reconciliation checks it against the dataset entry before touching
	// anything: source ids here are positional ("act-0", "act-1"), so
	// reordering the activities array silently remaps which stored row an
	// index names, and relinking the wrong one cannot be undone.
	OrganizationID string
}
