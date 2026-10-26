// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H2

package gates

// A participant row that names only a seat records an inferred owner, not a
// seat that was on the message. auth.ParticipantStandsSQL tells the two apart.
//
// Five readers were routed through it once. This census answers the other
// question: that a new presence read cannot skip it.
//
// Without the census the rule holds only where someone remembered it, and a
// later reader again treats a guessed mailbox owner as a participant.
//
// The subject is a presence read rather than every read. An id-resolving join,
// a write path and a repair all name user_id without asking whether the row
// stands.
//
// The corpus is read per declaration. A reader whose table name and whose
// user_id predicate sit in different constants is still selected, because this
// tree assembles queries out of several literals.

import (
	"go/ast"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// participantTable and participantSeatColumn are read together: a declaration
// naming the table without the seat column is not asking about a seat.
var (
	participantTable      = regexp.MustCompile(`\bactivity_participant\b`)
	participantSeatColumn = regexp.MustCompile(`\buser_id\b`)
)

// participantStandsRule is the symbol a presence read composes. The census
// looks for it among the names a function reaches.
const participantStandsRule = "ParticipantStandsSQL"

// presenceReadsAdmitted ratifies a declaration that reads activity_participant
// with user_id and never asks whether the row stands, keyed
// "package-dir:FuncName". Every entry says why a guessed owner cannot matter
// there; an entry whose declaration stops reading the table fails as stale.
var presenceReadsAdmitted = gatekit.Waive(map[string]string{
	"internal/modules/activities:RetireGuessedMailboxSeatsTx": "the repair that takes guessed mailbox owners off an " +
		"activity. It is the one reader whose subject is the rows the rule distrusts, so composing the rule would make " +
		"it skip every row it exists to remove and quietly repair nothing.",
	"internal/modules/activities:TurnBackColleagueCopyTx": "undoes a stamp this code made, keyed on sent_by_colleague " +
		"and the stamp's own address rather than on a participant row standing. Only the stamp writes that state, so a " +
		"guessed owner cannot put a row into it.",
	"internal/modules/activities:readGuestRows": "selects rows with `ap.user_id IS NULL`, which name no seat at all. A " +
		"guessed mailbox owner names only a seat, so it cannot match this predicate and the question the rule answers " +
		"does not arise.",
	"internal/modules/capture:ReleaseImportTx": "releases one seat's import of one activity, by (activity, seat). It " +
		"removes rows rather than concluding from them, and a guessed owner row is among the rows a release clears.",
	"internal/modules/capture:stampSeatsPastCap": "renders the statement that stamps calendar attendance, where a seat " +
		"row means the event is in that seat's own calendar. The comment above it sets out why the seat arm is the only " +
		"one that can serve: folding the address list in would report a relationship with everyone who got the same mail.",
	"internal/modules/consent:copiedOnThisMatter": "the seat row is one of several conditions, and the deciding ones " +
		"cannot come from a guess: the copying mail must come from a sender the receiving server authenticated, and one " +
		"of our own seats must have written to that sender on the same thread.",
})

// readsParticipantSeat selects a file holding a declaration that reads the
// participant table and names the seat column.
func readsParticipantSeat(_ string, file *ast.File) bool {
	for _, decl := range file.Decls {
		if presenceRead(declText(decl)) {
			return true
		}
	}
	return false
}

// participantWrite names a statement that creates or removes participant rows
// rather than concluding anything from them.
var participantWrite = regexp.MustCompile(`(?is)\b(INSERT\s+INTO|UPDATE|DELETE\s+FROM)\s+activity_participant\b`)

// participantSelect names the table being read rather than written: a `FROM`
// or `JOIN` naming it, which an `INSERT ... SELECT` or a guarded `DELETE` also
// carries.
var participantSelect = regexp.MustCompile(`(?is)\b(FROM|JOIN)\s+activity_participant\b`)

// identifyingColumn names a column a non-standing row cannot have. The rule
// itself says which: a row that does not stand has address, contact_id and
// channel_user_id all NULL. So a read that requires one of them to be present
// cannot match a guessed owner, whatever it asks about user_id.
//
// Derived from ParticipantStandsSQL rather than listed here a second time. The
// exclusion is sound only while it names the same columns the rule does.
//
// TestTheIdentifyingColumnsAreTheOnesTheRuleNames holds the two together.
var identifyingColumn = regexp.MustCompile(
	`(?is)\b[a-z_]*\.?(address|contact_id|channel_user_id)\b\s*(=|IS\s+NOT\s+NULL|IN\b|=\s*ANY)`)

// presenceRead answers whether one declaration's whole text reads the
// participant table against a seat.
//
// A write is not a presence read. It makes or unmakes the rows, and cannot
// mistake a guessed owner for a seat that was on the message.
//
// The repairs that retire guessed rows are writes by this test, so they need
// no entry below: they act on the rows the rule distrusts.
func presenceRead(whole string) bool {
	if !participantTable.MatchString(whole) || !participantSeatColumn.MatchString(whole) {
		return false
	}
	// A read keyed on something a guessed row cannot have is not asking the
	// question the rule answers.
	if identifyingColumn.MatchString(whole) {
		return false
	}
	// Excluded only when the write is all it does with the table.
	// A function that writes and also reads the rows owes the rule for the
	// read, so matching the write alone would let that one through.
	return !participantWrite.MatchString(whole) || participantSelect.MatchString(whole)
}

// participantPresenceScope is the subtree this census claims, and the files
// outside it that hold a presence read anyway.
var participantPresenceScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: readsParticipantSeat,
	Exempt:  gatekit.Waive(map[string]string{}),
}

// composesTheRule answers whether one function reaches the standing rule,
// by calling it or by naming it for someone else to call.
func composesTheRule(entry *graphFunc) bool {
	if entry == nil {
		return false
	}
	for _, names := range []map[string]bool{entry.calls, entry.reads} {
		if names[participantStandsRule] {
			return true
		}
	}
	return false
}

func TestEveryParticipantPresenceReadAsksWhetherTheRowStands(t *testing.T) {
	t.Parallel()
	byDir := map[string][]gatekit.ParsedFile{}
	for _, parsed := range participantPresenceScope.Files(t) {
		dir := filepath.Dir(parsed.Path)
		byDir[dir] = append(byDir[dir], parsed)
	}
	if len(byDir) == 0 {
		t.Fatal("no package holds a participant presence read, which this tree is known to carry")
	}
	var unguarded []string
	for dir, files := range byDir {
		// The graph answers the caller hop, within the package only. A reader
		// guarded by a caller elsewhere is not credited here, and needs an
		// entry below rather than silence.
		graph := packageCallGraph(t, dir)
		for _, parsed := range files {
			for _, decl := range parsed.File.Decls {
				fn, isFunc := decl.(*ast.FuncDecl)
				if !isFunc {
					continue
				}
				// The declaration's whole text. The rule is composed as a call
				// and the table name may sit in another constant, so the
				// statements alone carry neither.
				whole := declText(decl)
				if !presenceRead(whole) {
					continue
				}
				// The receiver type, which is how packageCallGraph keys a
				// method. The receiver variable looks right and resolves to
				// nothing, so every method would read as composing no rule.
				key := scrubKey(receiverTypeName(fn), fn.Name.Name)
				// The rule is a Go call, so it is looked for among the names
				// the function reaches rather than in its text. declText
				// collects literals, and a call contributes its arguments.
				//
				// `reads` as well as `calls`: this tree passes a clause
				// builder by name as often as it calls it.
				if composesTheRule(graph[key]) || guardedBy(graph, key, participantStandsRule) {
					continue
				}
				unguarded = append(unguarded, strings.TrimPrefix(dir, "../")+":"+key)
			}
		}
	}
	sort.Strings(unguarded)
	for _, site := range unguarded {
		if presenceReadsAdmitted.Waived(t, site) {
			continue
		}
		t.Errorf("%s reads activity_participant against a seat and never asks whether the row stands.\n"+
			"Compose auth.%s, or admit it in presenceReadsAdmitted saying why a guessed owner cannot matter there.",
			site, participantStandsRule)
	}
	presenceReadsAdmitted.AssertAllMatched(t)
}

// The exclusion above is sound only while it names the columns the rule names.
// ParticipantStandsSQL says a row does not stand when address, contact_id and
// channel_user_id are all NULL. A read requiring one of them cannot match such
// a row.
//
// Add a fourth column to that NULL test and the exclusion starts admitting
// reads it should select.
func TestTheIdentifyingColumnsAreTheOnesTheRuleNames(t *testing.T) {
	t.Parallel()
	rule := auth.ParticipantStandsSQL("p", "a")
	nulls := regexp.MustCompile(`"p"\.([a-z_]+) IS NULL`).FindAllStringSubmatch(rule, -1)
	named := map[string]bool{}
	for _, hit := range nulls {
		named[hit[1]] = true
	}
	if len(named) == 0 {
		t.Fatalf("no NULL test found in the rule, so this test reads nothing:\n%s", rule)
	}
	for column := range named {
		if !identifyingColumn.MatchString("p." + column + " = $1") {
			t.Errorf("the rule treats %q as identifying and the exclusion does not", column)
		}
	}
	for _, column := range []string{"address", "contact_id", "channel_user_id"} {
		if !named[column] {
			t.Errorf("the exclusion names %q and the rule no longer does", column)
		}
	}
}
