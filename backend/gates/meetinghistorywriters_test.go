// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every statement that writes activity.meeting_status also records the
// transition.
//
// The column says what a meeting IS; activity_meeting_history says what it
// BECAME and when, and only the second can answer a question about a period. A
// writer that sets the column without recording the transition makes every such
// question answer short — and it fails silently, because the write succeeds and
// the row reads correctly. Nothing but a census sees it.
//
// This reads the tree rather than a list of writers somebody remembered, so a
// door added next year is judged the same as the two that exist today.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// meetingHistoryRecorder names the function a writer has to call. That it is
// the only one is what TestEveryMeetingStatusWriterRecordsHistory below holds:
// a file writing the column without reaching it fails.
const meetingHistoryRecorder = "recordMeetingTransition"

// writesMeetingStatusWithoutHistory ratifies the files that name
// meeting_status in SQL for a reason that is not a write.
//
// Each says which different thing it does. A file that merely READS the column
// — a filter, a projection, a report expression — changes no meeting and owes
// no transition.
var writesMeetingStatusWithoutHistory = gatekit.Waive(map[string]string{
	"internal/modules/activities/scheduling.go": "books through LogActivity, which records the " +
		"transition for it. The status is set here because booking a meeting is what `booked` " +
		"means, and the store beside it owns the history",
	"internal/modules/activities/mapping.go": "maps a request onto LogActivityInput and executes " +
		"nothing; the store it hands that input to records the transition",
})

func TestEveryMeetingStatusWriterRecordsHistory(t *testing.T) {
	t.Parallel()
	defer writesMeetingStatusWithoutHistory.AssertAllMatched(t)

	fset := token.NewFileSet()
	var offences []string
	judged := 0
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") || strings.HasPrefix(where, "internal/contracts/") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", where, err)
		}
		if !setsMeetingStatus(file) {
			continue
		}
		judged++
		if writesMeetingStatusWithoutHistory.Waived(t, where) {
			continue
		}
		if fileCallsFunction(file, meetingHistoryRecorder) {
			continue
		}
		offences = append(offences, where+
			": names meeting_status and never calls "+meetingHistoryRecorder+
			". A door that sets the column without recording the transition makes every "+
			"period question — how many did we book last week — answer short, and the write "+
			"still succeeds. Call the recorder, or ratify this file in "+
			"writesMeetingStatusWithoutHistory saying what it does instead")
	}
	// A census that can fail short has already failed: reading no files at all
	// would report a clean pass over nothing.
	if judged == 0 {
		t.Fatal("no file mentioning meeting_status was read; this gate proves nothing")
	}
	for _, o := range offences {
		t.Error(o)
	}
}

// setsMeetingStatus reports whether a file WRITES the column — not whether it
// mentions it.
//
// Mentioning it is what almost every file here does: a filter, a projection, a
// lead ladder reading `booked` as engagement. Those change no meeting. Two
// shapes actually set it:
//
//   - SQL naming the column on the written side of a statement — an INSERT
//     column list, or `meeting_status =` in an UPDATE or a SET clause;
//   - a MeetingStatus field given a value in a composite literal, which is how
//     a caller reaches the store's own writers.
//
// Matching statements rather than lines is the point. A gate keyed on the
// column name alone would flag twenty readers, and the waiver list that
// followed would be the second copy of the subject this exists to prevent.
func setsMeetingStatus(file *ast.File) bool {
	// Statements, escapes decoded and `+` chains flattened. Reading the source
	// text instead would miss a statement assembled across two lines, and see a
	// double-quoted one as its escapes rather than as the SQL Postgres receives.
	for _, statement := range gatekit.SQLStatementsOf(file) {
		if sqlWritesMeetingStatus(statement) {
			return true
		}
	}
	writes := false
	ast.Inspect(file, func(n ast.Node) bool {
		if writes {
			return false
		}
		if kv, ok := n.(*ast.KeyValueExpr); ok {
			if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == "MeetingStatus" {
				writes = true
			}
		}
		return !writes
	})
	return writes
}

// sqlWritesMeetingStatus reports whether one SQL string names the column on the
// written side of a statement.
func sqlWritesMeetingStatus(sql string) bool {
	lowered := strings.ToLower(sql)
	if !strings.Contains(lowered, "meeting_status") {
		return false
	}
	// An assignment in an UPDATE's SET list. The verb is what distinguishes it
	// from `meeting_status = \'booked\'` in a WHERE, which is a comparison and
	// reads rather than writes — most files here do exactly that.
	if strings.Contains(lowered, "update activity") &&
		(strings.Contains(lowered, "meeting_status =") || strings.Contains(lowered, "meeting_status=")) {
		return true
	}
	// An INSERT's column list. The column appearing anywhere between INSERT
	// INTO and the closing paren of that list is a write; the same name in a
	// WHERE or a SELECT further down the same string is not.
	insert := strings.Index(lowered, "insert into")
	if insert < 0 {
		return false
	}
	rest := lowered[insert:]
	open := strings.Index(rest, "(")
	closeParen := strings.Index(rest, ")")
	if open < 0 || closeParen < open {
		return false
	}
	return strings.Contains(rest[open:closeParen], "meeting_status")
}

// fileCallsFunction reports whether anywhere in the file calls the named
// function. Whole-file rather than per-declaration: a writer may record the
// transition from a helper beside it, and that still records it.
func fileCallsFunction(file *ast.File, name string) bool {
	called := false
	ast.Inspect(file, func(n ast.Node) bool {
		if called {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			if fn.Name == name {
				called = true
			}
		case *ast.SelectorExpr:
			if fn.Sel.Name == name {
				called = true
			}
		}
		return !called
	})
	return called
}
