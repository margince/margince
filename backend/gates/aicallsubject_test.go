// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Every model call either NAMES the record it is about, or says why it names
// none.
//
// With payload capture on, `ai_call_payload` holds the request and response of
// every call, and for a transcript reading the request IS the meeting. An
// Art. 17 erasure reaches that table two ways: by the subject's addresses in
// the text, and — since the citation columns — by the record the call said it
// was about. The first is crude and misses a transcript that names its speakers
// rather than their addresses; the second reaches it exactly, and only for calls that named something.
//
// So the population of that column is the whole of its value, and a column that
// is right for half the call sites is worse than none, because a purge would
// trust it. What this census holds is that no site is undecided: every one is
// classified, a new one fails until somebody classifies it, and a classification
// that stops matching a site fails too.
//
// WHAT IT DOES NOT HOLD is that a claim is true. "This call is about that
// record" is a reading of the code, and the note beside each entry is where the
// next reader checks it. What the gate can check it does: a site claiming to
// name a subject must point at a file that actually reaches ai.WithSubject, so
// an entry cannot be an assertion about nothing.

// askSiteRoot is the tier every model call in this product is made from. The
// router is the door; compose is where the doors are opened.
const askSiteRoot = "internal/compose"

// subjectClaim is one site that names its subject: the file carrying the
// WithSubject call, and what the citation says the call is about.
//
// The witness is a FILE rather than a function because the two are routinely
// apart — a lane names its subject once at the entry point and the model call
// sits three frames down, which is the shape that makes the retry and the
// escalation cite the same record as the first attempt.
type subjectClaim struct {
	witness string
	names   string
}

// namesItsSubject: the site cites a record, and the witness is where it is
// named.
var namesItsSubject = map[string]subjectClaim{
	"internal/compose/transcriptpropose.go:ask": {
		"internal/compose/transcriptproposerun.go",
		"the meeting whose transcript IS the request — the case this citation was built for",
	},
	"internal/compose/replydraftmodel.go:completeWith": {
		"internal/compose/replydraftvoice.go",
		"the message being answered, named once so the critic retry cites what the first draft did",
	},
	"internal/compose/captureenrich.go:enrichOne": {
		"internal/compose/captureenrich.go",
		"the contact whose signature block is being read",
	},
	"internal/compose/confidentialityverdictask.go:ask": {
		"internal/compose/confidentialityverdictask.go",
		"the message being judged, whose subject and body are the request",
	},
	"internal/compose/documentextractrun.go:ask": {
		"internal/compose/documentextractrun.go",
		"the record the document hangs on — a contract or an invoice is the largest copy of somebody's words this product sends",
	},
	"internal/compose/signalextract.go:ask": {
		"internal/compose/signalextract.go",
		"the account the conversation is with; the messages are several and the account is one",
	},
	"internal/compose/companybrief/write.go:writeWithModel":             {"internal/compose/companybrief/service.go", "the company the brief describes"},
	"internal/compose/companybrief/ask.go:answerWithModel":              {"internal/compose/companybrief/service.go", "the company the question is about"},
	"internal/compose/contactbrief/write.go:writeWithModel":             {"internal/compose/contactbrief/service.go", "the contact the brief is written for"},
	"internal/compose/companydossier/write.go:writeWithModel":           {"internal/compose/companydossier/service.go", "the company the dossier describes"},
	"internal/compose/companydossier/growthfitwrite.go:assessWithModel": {"internal/compose/companydossier/service.go", "the same company, assessed"},
	"internal/compose/meetingbrief/model.go:writeWithModel":             {"internal/compose/meetingbrief/service.go", "the meeting the brief prepares"},
	"internal/compose/meetingbrief/planwrite.go:writePlanWithModel":     {"internal/compose/meetingbrief/service.go", "the same meeting: the plan runs under the brief's own context"},
	"internal/compose/draftcore/writer.go:writeWithModel": {
		"internal/compose/leaddraft/service.go",
		"whoever the draft is for, named by the surface that asked for it — the lead lane does; the contact and account lanes are the residue below",
	},
}

// namesNoSubject: the call is about no single record, and the reason is what
// stops the next reader adding one.
var namesNoSubject = gatekit.Waive(map[string]string{
	"internal/compose/captureclassify.go:ask":                   "one call labels a BATCH of unread messages. The batch is the unit the economics are built on, and a citation naming one of its rows would be wrong about the others",
	"internal/compose/owedclassify.go:ask":                      "one call judges many messages at once, as the classifier above does; a citation naming one row would be wrong about the rest",
	"internal/compose/requestsettle.go:ask":                     "a batch of settlement candidates, one call for all of them",
	"internal/compose/captureverdictask.go:ask":                 "it judges an ADDRESS the workspace has not made a record of yet — that is the question, and there is nothing to cite until it is answered",
	"internal/compose/briefs/briefl2.go:askModel":               "the call ranks a queue against each other; it is about the ordering rather than about any one item in it",
	"internal/compose/corpusask.go:askCorpusLane":               "passages retrieved from across the workspace's documents, which is several records by construction",
	"internal/compose/fxrefresh.go:extract":                     "a published exchange-rate page. No record, and no personal data to erase",
	"internal/compose/modelraterefresh.go:extract":              "a published model-pricing page. No record, and no personal data to erase",
	"internal/compose/sitereaddebug.go:CompleteValidated":       "a recording WRAPPER rather than a site: it forwards the caller's context unchanged, so whatever subject the caller named travels through it",
	"internal/compose/sitereaddebug.go:debugTriage":             "the operator debug lane, run against a URL before any record is chosen",
	"internal/compose/voicebuilddemo.go:demonstrationDraft":     "a member's own writing, read to build their voice profile. The subject is that profile, which is not a record the citation's vocabulary names",
	"internal/compose/voicebuildeval.go:evaluateVoiceCandidate": "the same voice-profile build, one candidate draft at a time, and the profile is not a record the vocabulary names",
	"internal/compose/voicebuildeval.go:judgeVoiceDrafts":       "the same voice-profile build, judging its own candidates against each other rather than against any record",
	"internal/compose/weeklyjobs.go:narrate":                    "a member's own week across every record they touched",
	"internal/compose/weeklylearningsjob.go:learn":              "the same week as the narrative above, read for what it teaches rather than for what happened",
})

// subjectOwed: the call IS about one record and does not say so yet. Each entry
// is a thread to pull, not a decision — the id exists at the call site or one
// frame above it.
//
// They are listed rather than left silent because the citation's value is what
// it covers, and a reader deciding whether to trust it for an erasure needs to
// see the uncovered half. Every one of them is reached by the content match
// exactly as it was before the column existed.
var subjectOwed = gatekit.Waive(map[string]string{
	"internal/compose/company360/introdraftwrite.go:introFromModel":       "the contact the introduction is about; the facts carry them",
	"internal/compose/network/intronotewrite.go:noteFromModel":            "the contact the note introduces, on the network surface rather than the account page",
	"internal/compose/company360/roleproposals.go:readProposals":          "the deal whose seats are being proposed",
	"internal/compose/companyscan/write.go:Read":                          "the company being scanned",
	"internal/compose/dealstatus/service.go:ask":                          "the deal whose status is written",
	"internal/compose/offerdraft.go:draftCandidates":                      "the deal the offer is for",
	"internal/compose/stageevidenceread.go:ask":                           "the deal whose stage criteria are read; the spans quoted are from its messages",
	"internal/compose/enrichextract.go:extractFields":                     "the company the extraction is filed against",
	"internal/compose/siteprofile.go:extractProfile":                      "the company whose site was read",
	"internal/compose/sitepagefacts.go:extractPageFacts":                  "the company whose site is being read, one page at a time",
	"internal/compose/deepreadtriage.go:classifySeed":                     "the company whose site is being read, deciding which of its pages are worth a reading",
	"internal/compose/onboardingcompanymessage.go:answer":                 "the company being onboarded",
	"internal/compose/onboardingacts.go:answerAct":                        "the same onboarding conversation, one act at a time; the company exists by then and nobody has threaded it",
	"internal/compose/onboardingsitereadmessage.go:answerCompanySiteRead": "the same onboarding, reading that company's site",
})

// wantMinimumAskSites is the floor under the extractor. A census that stopped
// recognising this tree's call shape would find nothing and report PASS, which
// reads exactly like a tree where every call names its subject.
const wantMinimumAskSites = 35

func TestEveryModelCallNamesItsSubjectOrSaysWhyItCannot(t *testing.T) {
	t.Parallel()
	sites := askSites(t)
	if len(sites) < wantMinimumAskSites {
		t.Fatalf("only %d model-call sites found under %s, want at least %d — the extractor has stopped "+
			"recognising this tree's call shape, and a census that reads a smaller tree reports PASS",
			len(sites), askSiteRoot, wantMinimumAskSites)
	}
	for _, site := range sites {
		claim, named := namesItsSubject[site]
		none := namesNoSubject.Waived(t, site)
		owed := subjectOwed.Waived(t, site)
		switch {
		case named && (none || owed), none && owed:
			t.Errorf("%s is classified twice; it is one call and has one answer", site)
		case named:
			assertWitnessNamesASubject(t, site, claim)
		case none, owed:
		default:
			t.Errorf("%s makes a model call and is not classified.\n"+
				"  With payload capture on, its request and response are stored. An erasure reaches them "+
				"by the record the call cites, or by matching the subject's address in the text.\n"+
				"  Name the record with ai.WithSubject and add it to namesItsSubject, or say why the call "+
				"is about no single record in namesNoSubject — or, if it is about one and nobody has "+
				"threaded the id yet, in subjectOwed.", site)
		}
	}
	t.Logf("model calls: %d named, %d name none, %d owed", len(namesItsSubject),
		len(namesNoSubject.Subjects()), len(subjectOwed.Subjects()))
	namesNoSubject.AssertAllMatched(t)
	subjectOwed.AssertAllMatched(t)
	for site := range namesItsSubject {
		if !slices.Contains(sites, site) {
			t.Errorf("namesItsSubject names %s, which makes no model call — the entry describes code that "+
				"has moved or gone", site)
		}
	}
}

// assertWitnessNamesASubject is the half of a claim a gate can check: the file
// the entry points at must actually reach ai.WithSubject. It does not prove the
// site runs under that context — that is the reading the note is for — but it
// refuses an entry asserting nothing at all.
func assertWitnessNamesASubject(t *testing.T, site string, claim subjectClaim) {
	t.Helper()
	if claim.names == "" {
		t.Errorf("%s claims a subject and does not say what it names", site)
	}
	raw, err := os.ReadFile(claim.witness)
	if err != nil {
		t.Errorf("%s names %s as the file that cites its subject, and it cannot be read: %v", site, claim.witness, err)
		return
	}
	if !strings.Contains(string(raw), "WithSubject(") {
		t.Errorf("%s names %s as the file that cites its subject, and that file calls no WithSubject",
			site, claim.witness)
	}
}

// askSites walks the compose tier for calls to ai.Ask and reports each as
// "<path>:<enclosing function>".
//
// Derived from the tree rather than listed, for the reason every census here
// is: a list is a second copy of the subject, and the copy is the one that
// stops growing.
func askSites(t *testing.T) []string {
	t.Helper()
	var sites []string
	scope := gatekit.Scope{
		Roots: []string{askSiteRoot},
		Subject: func(_ string, file *ast.File) bool {
			for _, decl := range file.Decls {
				if fn, isFunc := decl.(*ast.FuncDecl); isFunc && callsAsk(fn) {
					return true
				}
			}
			return false
		},
		Exempt: gatekit.Waive(map[string]string{}),
	}
	for _, parsed := range scope.Files(t) {
		for _, decl := range parsed.File.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || !callsAsk(fn) {
				continue
			}
			sites = append(sites, filepath.ToSlash(parsed.Path)+":"+fn.Name.Name)
		}
	}
	sort.Strings(sites)
	return sites
}

// callsAsk reports whether this declaration makes the model call itself.
func callsAsk(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != "Ask" {
			return true
		}
		if pkg, isIdent := sel.X.(*ast.Ident); isIdent && pkg.Name == "ai" {
			found = true
		}
		return true
	})
	return found
}
