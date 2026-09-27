// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Only the answer predicate walks a thread for our reply.
//
// Every other SQL statement that joins an outbound row to a thread either calls
// activities/answered.go's answeredSQL or is named below with the different
// question it asks.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// answerWalkOwner is where the one answer lives.
const answerWalkOwner = "internal/modules/activities/answered.go:threadAnswerArm"

// answerWalksAdmitted are the thread walks that ask something other than
// "was this inbound answered", keyed by file and declaration.
var answerWalksAdmitted = gatekit.Waive(map[string]string{
	answerWalkOwner: "the thread arm of the one answer: every surface that shows needs-reply, a waiting row, request review or the response time reads it through answeredSQL or firstAnswerAtSQL",
	"internal/modules/activities/waitingsql.go:waitingRepliesSQL":           "the Engaged column asks whether we wrote on the thread BEFORE the message arrived, to rank the row; it is reported, never used to hide one",
	"internal/modules/activities/ourownoutbound.go:ourOutboundInThisThread": "the settlement pass and the owed-verdict prior message find OUR attested message to one correspondent, to stamp what a request was judged through or to show the model what we last wrote. A reply settles no request by itself, so this decides no badge and no lane row; widening the settlement evidence to the answer arms changes that AI task's inputs and ships with its re-certification",
	"internal/compose/requestsettle.go:requestConversation":                 "reads the request and the messages after it, both directions, bound to one attested correspondent, as the settlement model's evidence; it answers nothing itself",
	"internal/modules/capture/sinkreply.go:emitReply":                       "capture's reply formula: an inbound on a thread we wrote on is a reply for the engagement score, whichever came first; a question about the counterparty, not about whether this message is owed",
	"internal/modules/capture/correspondencegate.go:wroteOnTwoThreadsTx":    "counts the separate threads we wrote to one address on, as evidence of a relationship for the first-time-sender gate; it reads no inbound and answers nothing",
	"internal/modules/capture/correspondencegate.go:wroteBackTx":            "the first-time-sender gate asks whether the workspace has a conversation with this address on this thread at all, deliberately unordered in time so a post-dated Date header cannot defeat it; not whether one message was answered",
})

// Both patterns read a statement lower-cased with its whitespace folded, so
// neither casing nor line breaks nor spacing around an operator hides a walk.
//
// threadMatch is a statement pinning rows to a thread: the key compared, on
// either side, with anything — another row's key, a bound value, a subselect,
// an IN list. An alias may be a format verb or empty, because a fragment
// renders its aliases in and a statement assembled with + leaves a hole.
var threadMatch = regexp.MustCompile(
	`thread_key ?(=|<>|!=|\bin\b|\bis\b)|(=|\bin ?\(|\bis not distinct from) ?[\w%\[\].]*thread_key\b`)

// ourOutbound marks a statement asking for our own sent message: the word
// outbound anywhere (an equality either way round, an IN list), a direction
// that is not inbound, the provider's attestation that we sent it, or a
// direction compared with a value this census cannot read, which it treats as
// outbound: missing a walk is the failure it must not have.
var ourOutbound = regexp.MustCompile(strings.Join([]string{
	`'outbound'`,
	`counterparty_outbound_attested`,
	`direction ?(=|<>|!=|\bin ?\(|\bis (not )?distinct from) ?(\$\d+|%|\(|any\b)`,
	`(\$\d+|%\S*) ?(=|<>|!=) ?[\w%\[\].]*direction\b`,
	`direction ?(<>|!=|\bis distinct from|\bnot in ?\() ?'inbound'`,
	`'inbound' ?(<>|!=) ?[\w%\[\].]*direction\b`,
}, "|"))

var foldSpace = regexp.MustCompile(`\s+`)

// answerWalkSites names every declaration in the source whose SQL joins an
// outbound row to a thread, as "path:declaration".
func answerWalkSites(path string, file *ast.File) []string {
	var sites []string
	for _, decl := range file.Decls {
		name := declarationName(decl)
		for _, sql := range gatekit.SQLStatementsOf(decl) {
			if walksForOutbound(sql) {
				sites = append(sites, path+":"+name)
				break
			}
		}
	}
	return sites
}

// walksForOutbound reports a statement that pins rows to a thread and asks
// for our own sent message. It does not require the two to name the same
// alias: a statement that does both about different rows is rare, and
// reading it costs a waiver line, where missing one costs the census.
func walksForOutbound(sql string) bool {
	folded := foldSpace.ReplaceAllString(strings.ToLower(sql), " ")
	return threadMatch.MatchString(folded) && ourOutbound.MatchString(folded)
}

func TestOnlyTheAnswerPredicateWalksAThreadForOurReply(t *testing.T) {
	t.Parallel()
	defer answerWalksAdmitted.AssertAllMatched(t)
	var found []string
	for _, root := range []string{"internal", "extensions", "cmd"} {
		walkRoot := root
		if root == "extensions" {
			walkRoot = filepath.Join("..", "extensions")
		}
		err := filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
				return err
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			found = append(found, answerWalkSites(filepath.ToSlash(path), file)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", walkRoot, err)
		}
	}
	sort.Strings(found)
	sawOwner := false
	for _, site := range found {
		if site == answerWalkOwner {
			sawOwner = true
		}
		if !answerWalksAdmitted.Waived(t, site) {
			t.Errorf("%s walks a thread for an outbound row. Whether an inbound was answered is "+
				"answeredSQL in activities/answered.go; call it, or name this walk in "+
				"answerWalksAdmitted with the question it asks instead", site)
		}
	}
	if !sawOwner {
		t.Fatalf("the census no longer recognises answerArms itself, so it would pass over a second copy")
	}
}

// A planted bypass is found, in each shape a walk is written in: a plain
// literal, a statement assembled with +, and a fragment whose aliases are
// format verbs.
func TestTheAnswerWalkCensusFindsAPlantedBypass(t *testing.T) {
	t.Parallel()
	planted := map[string]string{
		"literal": "package p\nconst q = `SELECT 1 FROM activity a WHERE NOT EXISTS (SELECT 1 FROM activity r " +
			"WHERE r.thread_key = a.thread_key AND r.direction = 'outbound')`\n",
		"concatenated": "package p\nvar q = `SELECT 1 FROM activity r WHERE r.thread_key = a.thread_key` + x + " +
			"` AND r.direction = 'outbound'`\n",
		"fragment": "package p\nfunc f() string { return fmt.Sprintf(`%[1]s.thread_key = %[2]s.thread_key " +
			"AND %[1]s.direction = 'outbound'`, a, b) }\n",
		"bound direction": "package p\nconst q = `SELECT 1 FROM activity r WHERE r.thread_key = a.thread_key " +
			"AND r.direction = $2`\n",
		"bound thread": "package p\nconst q = `SELECT 1 FROM activity WHERE thread_key = $1 AND direction = 'outbound'`\n",
		"attested": "package p\nconst q = `SELECT 1 FROM activity ours WHERE ours.thread_key = inbound.thread_key " +
			"AND ours.counterparty_outbound_attested`\n",
		"in list": "package p\nconst q = `SELECT 1 FROM activity r WHERE r.thread_key = a.thread_key " +
			"AND r.direction in ('outbound')`\n",
		"reversed equality": "package p\nconst q = `SELECT 1 FROM activity r WHERE a.thread_key = r.thread_key " +
			"AND 'outbound' = r.direction`\n",
		"casing and spacing": "package p\nconst q = `SELECT 1 FROM activity r WHERE R.THREAD_KEY=A.THREAD_KEY\n" +
			"   AND R.DIRECTION\n  =\n 'OUTBOUND'`\n",
		"not inbound": "package p\nconst q = `SELECT 1 FROM activity r WHERE r.thread_key = a.thread_key " +
			"AND r.direction <> 'inbound'`\n",
		"reversed bound": "package p\nconst q = `SELECT 1 FROM activity r WHERE $1 = r.thread_key " +
			"AND $2 = r.direction`\n",
		"distinct from": "package p\nconst q = `SELECT 1 FROM activity r WHERE r.thread_key IS NOT DISTINCT FROM a.thread_key " +
			"AND r.direction IS DISTINCT FROM 'inbound'`\n",
	}
	for name, source := range planted {
		file, err := parser.ParseFile(token.NewFileSet(), name+".go", source, 0)
		if err != nil {
			t.Fatalf("parsing the planted %s walk: %v", name, err)
		}
		if len(answerWalkSites(name+".go", file)) == 0 {
			t.Errorf("the census missed a planted %s walk", name)
		}
	}
	inboundOnly := "package p\nconst q = `SELECT 1 FROM activity n WHERE n.thread_key = a.thread_key " +
		"AND n.direction = 'inbound'`\n"
	file, err := parser.ParseFile(token.NewFileSet(), "inbound.go", inboundOnly, 0)
	if err != nil {
		t.Fatalf("parsing the inbound walk: %v", err)
	}
	if sites := answerWalkSites("inbound.go", file); len(sites) != 0 {
		t.Errorf("the census flagged a walk for a newer INBOUND message, which is not an answer: %v", sites)
	}
}
