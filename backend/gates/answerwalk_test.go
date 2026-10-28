// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Only the answer predicate walks a thread for our reply.
//
// Other outbound thread walks call the shared answer predicate.
// Exceptions below name the different question they ask.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// answerWalkOwner is where the one answer lives.
const answerWalkOwner = "internal/platform/database/storekit/answered.go:threadAnswerArm"

// answerWalksAdmitted are the thread walks that ask something other than
// "was this inbound answered", keyed by file and declaration.
var answerWalksAdmitted = gatekit.Waive(map[string]string{
	"internal/modules/activities/awaitingreply.go:awaitingTheirAnswerSQL": "the mirror question: whether a message the READER sent is still the last word on its thread, so the reader is reminded to follow up. It asks about our outbound, never whether an inbound was answered, and decides no needs-reply badge",
	answerWalkOwner: "the thread arm of the one answer: every surface that shows needs-reply, a waiting row, request review or the response time reads it through answeredSQL or firstAnswerAtSQL",
	"internal/modules/activities/awaitingreply.go:awaitingRepliesSQL":                "the waiting-replies lane query; it lists rows through awaitingTheirAnswerSQL and asks the same mirror question about our outbound",
	"internal/modules/activities/emailmove.go:EmailStatesFor":                        "the email row's page read; its move is moveSQL, and it asks no answer question of its own",
	"internal/modules/activities/emailmove.go:moveSQL":                               "the email row's waiting-for-them badge composes awaitingTheirAnswerSQL for a mail the reader sent; whether an inbound is owed is owedSQL, which reads answeredSQL",
	"internal/modules/activities/emailmove.go:MailMoveColumnSQL":                     "moveSQL as a column, for a reader that already selects the row; it asks no answer question of its own",
	"internal/modules/activities/waitingsql.go:waitingRepliesSQL":                    "the Engaged column asks whether we wrote on the thread BEFORE the message arrived, to rank the row; it is reported, never used to hide one",
	"internal/modules/activities/ourownoutbound.go:ourOutboundInThisThread":          "the settlement pass and the owed-verdict prior message find OUR attested message to one correspondent, to stamp what a request was judged through or to show the model what we last wrote. A reply settles no request by itself, so this decides no badge and no lane row; the settlement's answers off the thread come from answered.go's own arms",
	"internal/compose/requestsettle.go:conversationRows":                             "reads the request and the messages after it, both directions, bound to one attested correspondent, as the settlement model's evidence, or the one such message a verdict is recorded through; it answers nothing itself",
	"internal/modules/activities/promisefollowup.go:promiseFollowUpSQL":              "our follow-up on a promise we made: our later mail on the promise's thread to the contact it was made to. The Done? cue offers it and the company scan holds a commitment finding to it; it asks about a promise, never whether an inbound was answered",
	"internal/modules/capture/sinkreply.go:emitReply":                                "capture's reply formula: an inbound on a thread we wrote on is a reply for the engagement score, whichever came first; a question about the counterparty, not about whether this message is owed",
	"internal/modules/consent/authorizeevidence.go:copiedOnThisMatter":               "the consent gate's Cc rule asks whether one of our seats sent a captured mail on the thread a recipient was copied on, as evidence for writing to them about that thread; it never asks whether an inbound was answered and decides no badge",
	"internal/modules/capture/correspondencegate.go:wroteOnTwoThreadsTx":             "counts the separate threads we wrote to one address on, as evidence of a relationship for the first-time-sender gate; it reads no inbound and answers nothing",
	"internal/modules/capture/correspondencegate.go:wroteBackTx":                     "the entry point of wroteBack's question; it decides no badge and no lane row",
	"internal/modules/capture/correspondencegate.go:wroteBack":                       "the first-time-sender gate asks whether the workspace has a conversation with this address on this thread at all, deliberately unordered in time so a post-dated Date header cannot defeat it; not whether one message was answered",
	"internal/modules/capture/correspondencegate.go:answeredExchange":                "the exchange wroteBack and addressAnswered share: an inbound reply from the address on a thread where we sent attested mail to it, unordered in time; it decides no badge and no lane row",
	"internal/modules/activities/owedreview.go:owedReviewReadTx":                     "the owed-verdict review shows the reader our attested last message to one correspondent, composed from ourOutboundInThisThread; it decides no badge and no lane row",
	"internal/modules/activities/owedverdict.go:priorOutboundVisibleJoin":            "the owed verdict's prior message, our attested mail to one correspondent through ourOutboundInThisThread, shown to the model as what we last wrote; it decides no badge",
	"internal/modules/activities/owedverdict.go:priorOutboundFrom":                   "the FROM and WHERE of the owed verdict's prior message: our newest attested mail to one correspondent before the candidate, through ourOutboundInThisThread. It finds the message to show or test, never whether the inbound was answered (TestARequestIsNotJudgedAgainstOurLastMessageReadOnlyInPart reads through it)",
	"internal/modules/activities/owedverdict.go:priorContextComplete":                "whether that prior message, found by priorOutboundFrom, was read in full; a candidate whose prior message was not is not judged. It asks about the prior message's text, never whether the inbound was answered (TestARequestIsNotJudgedAgainstOurLastMessageReadOnlyInPart: refused when read in part, offered when read in full)",
	"internal/modules/activities/requestsettlement.go:newestAnswer":                  "the settlement pass reads its own evidence query (settlementAnswersSQL) for the newest candidate answer to stamp; whether an inbound was answered stays answered.go's",
	"internal/modules/activities/requestsettlement.go:repliedRequestsSQL":            "selects requests that have settlement evidence through settlementAnswersSQL, to stamp what a request was judged through; it decides no needs-reply badge",
	"internal/modules/activities/requestsettlement.go:settlementAnswersSQL":          "the settlement evidence: our attested reply to the request's correspondent (ourOutboundInThisThread) or an authenticated off-thread answer, to stamp what a request was judged through; a reply settles no request by itself",
	"internal/modules/activities/samecorrespondence.go:SameCorrespondenceSQL":        "decides which messages on a thread belong to one correspondence: our attested mail to its correspondent or company, or authenticated mail from them. It feeds a reply's context, a thread move, the viewer's standing and the extraction window, never whether an inbound was answered",
	"internal/modules/activities/relinkbatch.go:liveConversationMembers":             "lists the messages of one correspondence through SameCorrespondenceSQL so a thread move files exactly them; it decides no badge",
	"internal/compose/signalextractwindow.go:windowAnchor":                           "picks the message an extraction window is bound to, preferring a proven one (our attested mail or authenticated inbound); it asks which message anchors the read, never whether an inbound was answered",
	"internal/modules/activities/whichdealanswer.go:answerWhichDealTx":               "a colleague's deal answer files the rest of one conversation on the chosen deal; it reads our attested mail and authenticated inbound mail in the thread to decide which rows may follow, never whether an inbound was answered",
	"internal/modules/capture/replyevidence.go:AddressAnsweredTx":                    "the entry point of addressAnswered's reverse question: whether the address answered us on a thread we wrote on; it decides no badge and no lane row",
	"internal/modules/capture/sinkdealdecide.go:threadDeals":                         "the deal rule follows a deal our own attested mail in the thread is filed under, and only when the mail's company owns that deal; it asks which deal a message is about, never whether an inbound was answered",
	"internal/modules/capture/replyevidence.go:addressAnswered":                      "the reverse question: whether the address answered US, on a thread we wrote on, as the evidence that publishes a contact kept private while our mail went unanswered; it decides no badge and no lane row",
	"internal/modules/activities/requestsettlementevidence.go:settlementEvidenceSQL": "names the request and its settlement answers (settlementAnswersSQL) as id:version pairs, so a verdict records what it read; it decides no needs-reply badge",
	"internal/modules/activities/requestsettlementevidence.go:evidenceUnchanged":     "locks the settlement answers (settlementAnswersSQL) and compares them with what the verdict read; it decides no needs-reply badge",
	"internal/modules/activities/requestsettlementlock.go:lostSettlementBasisSQL":    "finds settled verdicts with no settlement answer left (settlementAnswersSQL), to withdraw them as unsure; it decides no needs-reply badge",
	"internal/modules/activities/requestsettlementlock.go:withdrawLostSettlement":    "rechecks one verdict through lostSettlementBasisSQL under the verdict's locks before withdrawing it; it decides no needs-reply badge",
	"internal/modules/activities/requestsettlementlock.go:holdSettlementEvidenceTx":  "locks the request's settlement answers (settlementAnswersSQL) before its conversation keys, so a verdict and a writer take locks in one order; it decides no needs-reply badge",
	"internal/modules/activities/requestsettlementrepair.go:settlementRepairItemTx":  "the reviewed repair asks whether a legacy settled request still has a settlement answer (settlementAnswersSQL) to judge; it decides no needs-reply badge",
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
	return answerWalkSitesIn(path, file, packageFragments(map[string]*ast.File{path: file}))
}

// fragment is a package-level SQL helper and the file it is declared in.
type fragment struct {
	body ast.Node
	path string
}

// packageFragments maps every receiver-less function of one package to its
// body, across all its files. A walk assembled from a helper in a sibling
// file is still one walk.
func packageFragments(files map[string]*ast.File) map[string]fragment {
	out := map[string]fragment{}
	for path, file := range files {
		for _, candidate := range file.Decls {
			if fn, ok := candidate.(*ast.FuncDecl); ok && fn.Recv == nil {
				out[fn.Name.Name] = fragment{body: fn.Body, path: path}
			}
		}
	}
	return out
}

// answerWalkSitesIn is answerWalkSites with the package's helpers.
func answerWalkSitesIn(path string, file *ast.File, fragments map[string]fragment) []string {
	var sites []string
	for _, decl := range file.Decls {
		name := declarationName(decl)
		if slices.ContainsFunc(answerWalkStatements(decl, fragments), walksForOutbound) {
			sites = append(sites, path+":"+name)
		}
	}
	return sites
}

// A shared fragment can hold the thread match while its caller adds direction.
// A helper in a sibling file, or one that is itself a walk, is expanded too.
// So a caller composing it with more SQL is seen. Calling the answer owner's
// file is the delegation the census asks for, so it is not expanded.
func answerWalkStatements(decl ast.Decl, fragments map[string]fragment) []string {
	statements := gatekit.SQLStatementsOf(decl)
	ast.Inspect(decl, func(node ast.Node) bool {
		joined, ok := node.(*ast.BinaryExpr)
		if !ok || joined.Op != token.ADD {
			return true
		}
		parts := gatekit.SQLStatementsOf(joined)
		seen := map[string]bool{}
		var include func(ast.Node, string)
		include = func(root ast.Node, pkg string) {
			ast.Inspect(root, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				key, callee := fragmentKey(call, pkg)
				if key == "" || seen[key] {
					return true
				}
				helper, known := fragments[key]
				if !known {
					return true
				}
				seen[key] = true
				body := helper.body
				bodySQL := gatekit.SQLStatementsOf(body)
				if helper.path == answerWalkOwnerFile {
					return true
				}
				parts = append(parts, bodySQL...)
				include(body, callee)
				return true
			})
		}
		include(joined, "")
		statements = append(statements, strings.Join(parts, " "))
		return true
	})
	return statements
}

// fragmentKey names the helper a call reaches: a bare name inside pkg, or a
// shared kernel package's function as "pkg.Name". It answers the package the
// helper's own bare calls resolve in.
func fragmentKey(call *ast.CallExpr, pkg string) (key, callee string) {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		if pkg != "" {
			return pkg + "." + fun.Name, pkg
		}
		return fun.Name, ""
	case *ast.SelectorExpr:
		if x, ok := fun.X.(*ast.Ident); ok {
			return x.Name + "." + fun.Sel.Name, x.Name
		}
	}
	return "", ""
}

// kernelFragments are the shared kernel's SQL helpers, keyed "pkg.Name". A
// module composes a walk from them as it does from its own helpers, so the
// census expands them too.
func kernelFragments(t *testing.T) map[string]fragment {
	t.Helper()
	out := map[string]fragment{}
	err := filepath.WalkDir(filepath.Join("internal", "shared", "kernel"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			return err
		}
		for _, candidate := range file.Decls {
			if fn, ok := candidate.(*ast.FuncDecl); ok && fn.Recv == nil {
				out[file.Name.Name+"."+fn.Name.Name] = fragment{body: fn.Body, path: filepath.ToSlash(path)}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the shared kernel's helpers: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("no shared kernel helper was read, so a walk composed from one would go unseen")
	}
	return out
}

// answerWalkOwnerFile is the file whose helpers are the one answer.
const answerWalkOwnerFile = "internal/platform/database/storekit/answered.go"

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
	kernel := kernelFragments(t)
	for _, root := range []string{"internal", "extensions", "cmd"} {
		walkRoot := root
		if root == "extensions" {
			walkRoot = filepath.Join("..", "extensions")
		}
		packages := map[string]map[string]*ast.File{}
		err := filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
				return err
			}
			file, err := gatekit.ParseFile(path, 0)
			if err != nil {
				return err
			}
			dir := filepath.Dir(path)
			if packages[dir] == nil {
				packages[dir] = map[string]*ast.File{}
			}
			packages[dir][filepath.ToSlash(path)] = file
			return nil
		})
		for _, files := range packages {
			fragments := packageFragments(files)
			maps.Copy(fragments, kernel)
			for path, file := range files {
				found = append(found, answerWalkSitesIn(path, file, fragments)...)
			}
		}
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
				"answeredSQL in platform/database/storekit/answered.go; call it, or name this walk in "+
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
		"shared fragment": "package p\nfunc thread() string { return `r.thread_key = a.thread_key` }; func f() string { return thread() + ` AND r.direction = 'outbound'` }\n",
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
		file, err := parser.ParseFile(gatekit.SourceFileSet(), name+".go", source, 0)
		if err != nil {
			t.Fatalf("parsing the planted %s walk: %v", name, err)
		}
		if len(answerWalkSites(name+".go", file)) == 0 {
			t.Errorf("the census missed a planted %s walk", name)
		}
	}
	// A walk assembled from a helper in a sibling file is one walk too. So is
	// a caller composing a helper that is itself a walk.
	for name, pair := range map[string][2]string{
		"sibling fragment": {
			"package p\nfunc thread() string { return `r.thread_key = a.thread_key` }\n",
			"package p\nfunc f() string { return thread() + ` AND r.direction = 'outbound'` }\n",
		},
		"composed": {
			"package p\nfunc ours() string { return `r.thread_key = a.thread_key AND r.direction = 'outbound'` }\n",
			"package p\nfunc f() string { return `SELECT 1 WHERE ` + ours() + ` AND r.restricted_at IS NULL` }\n",
		},
	} {
		files := map[string]*ast.File{}
		for i, source := range pair {
			path := fmt.Sprintf("%s%d.go", name, i)
			file, err := parser.ParseFile(gatekit.SourceFileSet(), path, source, 0)
			if err != nil {
				t.Fatalf("parsing the planted %s walk: %v", name, err)
			}
			files[path] = file
		}
		caller := name + "1.go"
		if len(answerWalkSitesIn(caller, files[caller], packageFragments(files))) == 0 {
			t.Errorf("the census missed a planted %s walk", name)
		}
	}
	// A walk composed from the shared kernel's conversation helpers is a walk.
	kernelComposed := "package p\nfunc f() string { return correspondence.ConversationSQL(\"r\", \"a\") + ` AND ` + correspondence.OurOutboundSQL(\"r\") }\n"
	composedFile, err := parser.ParseFile(gatekit.SourceFileSet(), "kernel.go", kernelComposed, 0)
	if err != nil {
		t.Fatalf("parsing the planted kernel walk: %v", err)
	}
	fragments := packageFragments(map[string]*ast.File{"kernel.go": composedFile})
	maps.Copy(fragments, kernelFragments(t))
	if len(answerWalkSitesIn("kernel.go", composedFile, fragments)) == 0 {
		t.Error("the census missed a walk composed from the shared kernel's helpers")
	}
	inboundOnly := "package p\nconst q = `SELECT 1 FROM activity n WHERE n.thread_key = a.thread_key " +
		"AND n.direction = 'inbound'`\n"
	file, err := parser.ParseFile(gatekit.SourceFileSet(), "inbound.go", inboundOnly, 0)
	if err != nil {
		t.Fatalf("parsing the inbound walk: %v", err)
	}
	if sites := answerWalkSites("inbound.go", file); len(sites) != 0 {
		t.Errorf("the census flagged a walk for a newer INBOUND message, which is not an answer: %v", sites)
	}
}
