// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A judge of a meeting's attendees holds storekit.LockMeetingAttendeeChecks. A
// row lock does not hold off a new attendee, so every writer takes the exclusive form.
//
// The census is every function under internal/ that writes activity_participant
// from a Go string. It cannot see a write in a SQL trigger or function, a
// statement spelled at run time, or extension code outside internal/. A COPY
// into the table fails here, because it carries no statement to read.

import (
	"go/ast"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const attendeeWriteLock = "LockMeetingAttendeeWrites"

// attendeeLocks are the storekit calls that take the attendee lock, shared or
// exclusive. The page form takes the exclusive one before its row locks.
var attendeeLocks = []string{attendeeWriteLock, "LockMeetingAttendeePage", "LockMeetingAttendeeChecks"}

// attendeeSweeps are the writers that change attendee rows across whatever
// meetings a subject or a domain reaches, keyed "package-dir:FuncName". The
// hourly evidence pass corrects a judgement such a sweep makes stale.
var attendeeSweeps = gatekit.Waive(map[string]string{
	"internal/modules/privacy:scrubSubjectFromParticipants": "an erasure clears one subject from every meeting it attended, " +
		"so no one meeting keys the lock. A judge that read the old rows is corrected by the hourly pass",
	"internal/modules/privacy:scrubContactGraphTraces": "a retention pass clears an expired subject from every meeting it attended, " +
		"so no one meeting keys the lock. A judge that read the old rows is corrected by the hourly pass",
	"internal/modules/contacts:relinkParticipantRows": "a merge moves the source contact's rows on every activity it appeared in. " +
		"The row keeps naming the same party",
	"internal/modules/contacts:promoteParticipantsToContact": "binds a contact to the rows that carry an address it proves, " +
		"on every activity of a cohort. The row keeps naming the same party",
	"internal/modules/activities:storeParticipantDomains": "folds the domain of stored addresses by row id, over whatever " +
		"activities a batch spans. It restates the address the row already carries",
	"internal/modules/activities:CopyRosterSeatsTx": "copies a row's one roster seat into its seat set by row id, " +
		"over whatever meetings a batch spans. Readers take both columns as one set, so no attendee set changes",
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
	byDir := map[string][]gatekit.ParsedFile{}
	for _, parsed := range attendeeWriteScope.Files(t) {
		dir := filepath.Dir(parsed.Path)
		byDir[dir] = append(byDir[dir], parsed)
	}
	assertTheTextAgrees(t, byDir)
	var unlocked []string
	for dir, files := range byDir {
		graph := packageCallGraph(t, dir)
		// A package-qualified call is not an edge in the graph, so a function
		// naming a lock is marked as calling the exclusive one.
		for _, entry := range graph {
			if entry.reads[attendeeWriteLock] || entry.reads["LockMeetingAttendeePage"] {
				entry.calls[attendeeWriteLock] = true
			}
		}
		assertEveryWriteHasAWriter(t, dir, files, graph)
		for key, entry := range graph {
			if writesAttendees(entry) && !guardedBy(graph, key, attendeeWriteLock) {
				unlocked = append(unlocked, strings.TrimPrefix(dir, "../")+":"+key)
			}
		}
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

// assertTheTextAgrees is the second count: a plain search of the source text.
// A file only it finds means the parsed census read less of the tree. A file
// only the census finds means the two readings disagree.
func assertTheTextAgrees(t *testing.T, byDir map[string][]gatekit.ParsedFile) {
	t.Helper()
	parsed := map[string]bool{}
	for _, files := range byDir {
		for _, file := range files {
			parsed[strings.TrimPrefix(file.Path, "../")] = true
		}
	}
	searched := map[string]bool{}
	for path, src := range internalSources(t) {
		if participantWrite.Match(src) {
			searched[path] = true
		}
	}
	if len(searched) == 0 {
		t.Fatal("the text search found no write of activity_participant, which this tree is known to carry")
	}
	for path := range searched {
		if !parsed[path] {
			t.Errorf("%s writes activity_participant by its text, and the parsed census never read it", path)
		}
	}
	for path := range parsed {
		if !searched[path] {
			t.Errorf("%s writes activity_participant by its parse, and the text search never found it", path)
		}
	}
}

// assertEveryWriteHasAWriter fails a write the call graph attributes to no
// function: a function whose own text holds it, or a package value nobody names.
func assertEveryWriteHasAWriter(t *testing.T, dir string, files []gatekit.ParsedFile, graph map[string]*graphFunc) {
	t.Helper()
	for _, file := range files {
		for _, decl := range file.File.Decls {
			if !participantWrite.MatchString(declText(decl)) {
				continue
			}
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if entry := graph[scrubKey(receiverTypeName(d), d.Name.Name)]; entry == nil || !writesAttendees(entry) {
					t.Errorf("%s: %s writes activity_participant, and the call graph does not count it", file.Path, d.Name.Name)
				}
			case *ast.GenDecl:
				for _, name := range declaredValueNames(d) {
					if !namedByAWriter(graph, name) {
						t.Errorf("%s: %s holds a write of activity_participant that no counted writer names", dir, name)
					}
				}
			}
		}
	}
}

func declaredValueNames(decl *ast.GenDecl) []string {
	var names []string
	for _, spec := range decl.Specs {
		if value, ok := spec.(*ast.ValueSpec); ok {
			for _, name := range value.Names {
				names = append(names, name.Name)
			}
		}
	}
	return names
}

func namedByAWriter(graph map[string]*graphFunc, name string) bool {
	for _, entry := range graph {
		if entry.reads[name] && writesAttendees(entry) {
			return true
		}
	}
	return false
}

// activityRowLock is a row lock a statement takes, and rowLockCalls are the Go helpers
// that take one on an activity.
var (
	activityRowLock = regexp.MustCompile(`(?is)\bFOR\s+(UPDATE|SHARE|NO\s+KEY\s+UPDATE|KEY\s+SHARE)\b`)
	rowLockCalls    = []string{"LockRow", "LockActivityLive", "lockActivityForWrite"}
)

// A judge holding the shared lock waits on the meeting's rows. A writer that
// holds one of those rows and then asks for the exclusive lock is a deadlock.
// Within one function the order is visible, so it is checked here.
func TestNoFunctionTakesARowLockBeforeTheAttendeeLock(t *testing.T) {
	t.Parallel()
	checked := 0
	for path, src := range internalSources(t) {
		if !slices.ContainsFunc(attendeeLocks, func(name string) bool { return strings.Contains(string(src), name+"(") }) {
			continue
		}
		file, err := gatekit.ParseFile(filepath.Join(gatekit.ModuleRoot(t), path), 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			lockAt, firstRowLock := attendeeLockOrder(fn)
			if !lockAt.IsValid() {
				continue
			}
			checked++
			if firstRowLock.IsValid() && firstRowLock < lockAt {
				t.Errorf("%s: %s takes a row lock before the attendee lock. Take storekit's attendee lock first",
					path, fn.Name.Name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no function takes the attendee lock, so this check read nothing")
	}
}

// attendeeLockOrder answers where fn first takes an attendee lock and where it
// first takes a row lock, in source order.
func attendeeLockOrder(fn *ast.FuncDecl) (lockAt, rowLockAt token.Pos) {
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			name := calledName(n)
			if slices.Contains(attendeeLocks, name) && !lockAt.IsValid() {
				lockAt = n.Pos()
			}
			if slices.Contains(rowLockCalls, name) && !rowLockAt.IsValid() {
				rowLockAt = n.Pos()
			}
		case *ast.BasicLit:
			if text, ok := gatekit.LiteralText(n); ok && activityRowLock.MatchString(text) && !rowLockAt.IsValid() {
				rowLockAt = n.Pos()
			}
		}
		return true
	})
	return lockAt, rowLockAt
}

// attendeeCopy is a bulk COPY into the table, from SQL or from pgx's CopyFrom.
var attendeeCopy = regexp.MustCompile(`(?is)\bCOPY\s+activity_participant\b|Identifier\{\s*"activity_participant"\s*\}`)

// A COPY writes rows with no statement the census can read, so it is refused.
func TestNothingCopiesIntoTheAttendeeTable(t *testing.T) {
	t.Parallel()
	for path, src := range internalSources(t) {
		if attendeeCopy.Match(src) {
			t.Errorf("%s copies into activity_participant. Write the rows with a statement that takes the attendee lock", path)
		}
	}
}

// internalSources reads every product Go file under internal/, keyed by its
// path from the module root.
func internalSources(t *testing.T) map[string][]byte {
	t.Helper()
	root := gatekit.ModuleRoot(t)
	out := map[string][]byte{}
	err := gatekit.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return err
		}
		src, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		out[filepath.ToSlash(rel)] = src
		return err
	})
	if err != nil {
		t.Fatalf("reading internal/: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("no product source under internal/, so these checks read nothing")
	}
	return out
}

func writesAttendees(entry *graphFunc) bool {
	return slices.ContainsFunc(entry.statements, participantWrite.MatchString)
}
