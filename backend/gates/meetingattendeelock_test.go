// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A judge of a meeting's attendees holds storekit.LockMeetingAttendeeChecks. A
// row lock does not hold off a new attendee, so every writer takes the exclusive form.
//
// The census is every function in the tree that writes activity_participant. A
// writer passes when it, or every in-package caller, takes the lock.

import (
	"go/ast"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const attendeeWriteLock = "LockMeetingAttendeeWrites"

// attendeeSweeps are the writers that change attendee rows across whatever
// meetings a subject or a domain reaches, keyed "package-dir:FuncName". The
// hourly evidence pass corrects a judgement such a sweep makes stale.
var attendeeSweeps = gatekit.Waive(map[string]string{
	"internal/modules/privacy:scrubSubjectFromParticipants": "an erasure clears one subject from every meeting it attended, " +
		"so there is no one meeting to key the lock on",
	"internal/modules/privacy:scrubContactGraphTraces": "a retention pass clears an expired subject from every meeting it attended, " +
		"so there is no one meeting to key the lock on",
	"internal/modules/contacts:relinkParticipantRows": "a merge moves the source contact's rows on every activity it appeared in. " +
		"The row keeps naming the same party",
	"internal/modules/contacts:promoteParticipantsToContact": "binds a contact to the rows that carry an address it proves, " +
		"on every activity of a cohort. The row keeps naming the same party",
	"internal/modules/activities:storeParticipantDomains": "folds the domain of stored addresses by row id, over whatever " +
		"activities a batch spans. It restates the address the row already carries",
	"internal/modules/activities:storeParticipantEvidenceRows": "clears domain evidence by row id, across any activity a " +
		"company change reaches. The attendee set stays the same",
	"internal/compose/integration:LinkActivitySender":                             "an integration fixture, written before any judge runs",
	"internal/compose/integration/apptest:AskedAboutTheDeal":                      "an integration fixture, written before any judge runs",
	"internal/compose/integration/apptest:SeedRoute":                              "an integration fixture, written before any judge runs",
	"internal/compose/integration/apptest:ChannelConversation.SeedInboundMessage": "an integration fixture, written before any judge runs",
})

// attendeeWriteScope selects the product files holding a write of the table.
var attendeeWriteScope = gatekit.Scope{
	Roots: []string{"internal"},
	Subject: func(_ string, file *ast.File) bool {
		for _, decl := range file.Decls {
			if participantWrite.MatchString(declText(decl)) {
				return true
			}
		}
		return false
	},
	Exempt: gatekit.Waive(map[string]string{}),
}

func TestEveryAttendeeWriterTakesTheMeetingLock(t *testing.T) {
	t.Parallel()
	dirs := map[string]bool{}
	for _, parsed := range attendeeWriteScope.Files(t) {
		dirs[filepath.Dir(parsed.Path)] = true
	}
	writers, unlocked := 0, []string{}
	for dir := range dirs {
		graph := packageCallGraph(t, dir)
		// A package-qualified call is not an edge in the graph, so a function
		// naming the lock is marked as calling it.
		for _, entry := range graph {
			if entry.reads[attendeeWriteLock] {
				entry.calls[attendeeWriteLock] = true
			}
		}
		for key, entry := range graph {
			if !writesAttendees(entry) {
				continue
			}
			writers++
			if guardedBy(graph, key, attendeeWriteLock) {
				continue
			}
			unlocked = append(unlocked, strings.TrimPrefix(dir, "../")+":"+key)
		}
	}
	// The tree has writers in capture, activities, privacy and contacts. A
	// census that finds few of them read a smaller tree than the one it guards.
	if writers < 30 {
		t.Fatalf("found only %d writers of activity_participant, so the census missed most of the tree", writers)
	}
	sort.Strings(unlocked)
	for _, site := range unlocked {
		if attendeeSweeps.Waived(t, site) {
			continue
		}
		t.Errorf("%s writes activity_participant without storekit.%s.\n"+
			"Take the lock on the activity before any row lock, or list the writer in attendeeSweeps "+
			"saying why it names no meeting.", site, attendeeWriteLock)
	}
	attendeeSweeps.AssertAllMatched(t)
}

func writesAttendees(entry *graphFunc) bool {
	return slices.ContainsFunc(entry.statements, participantWrite.MatchString)
}
