// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A {subject, body} model reply has ONE reader.
//
// It had two. The introduction REQUEST (org360, written to a colleague) and the
// introduction NOTE (network, forwarded to a customer) each unmarshalled the
// same envelope and each refused on the same three conditions, written
// independently. The prompts differ deliberately — two registers, argued in the
// code beside each wording table — but the contract underneath never had a
// reason to be spelled twice.
//
// What the duplication cost was not drift. It was that the DEFECT was copied:
// both refused an empty subject while neither prompt asked for one, and both
// required a name in the body that neither prompt demanded. A model obeying
// either prompt was refused, the reader silently got the template, and fixing
// one copy left the other live on a customer-facing path.
//
// So this is the test that fails when a third appears. A new drafting surface
// reads its reply through compose/draftreply, or it states here why it cannot.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// A reader of this reply does three things, and the gate asks for all three:
// it declares the subject and body fields of the envelope, and it unfences the
// text — which is what a caller does to a MODEL reply and to nothing else.
// Matching the tags alone flagged an email row and a licence response that
// merely have a subject; matching Unfence alone would flag every other reply
// shape in the tree. The tag patterns admit an option list, because
// `json:"subject,omitempty"` is the same envelope field and an exact-match
// pattern would have read past it.
//
// What it cannot see: a reader that unwraps a model reply some other way, or
// one that names the fields without the tags. Both are reachable and neither
// is how this tree writes a reply reader today. The floor below is what stops
// the scan failing SHORT, which is the one way this gate could report PASS
// while reading nothing.
var replyEnvelopeMarkers = []*regexp.Regexp{
	regexp.MustCompile(`json:"subject[,"]`),
	regexp.MustCompile(`json:"body[,"]`),
	regexp.MustCompile(`Unfence\(`),
}

// draftReplyOwner is the one file allowed to define the envelope, and the floor
// this census is held to. A scan that stops finding it has stopped reading the
// tree rather than found the tree clean — the one way this gate could pass
// while seeing nothing.
const draftReplyOwner = "internal/compose/draftreply/draftreply.go"

// statedReasons are the readers that do NOT share the helper and say why,
// which is the other half of the rule: two writers of one invariant either
// share a helper or state their reason where the next reader meets it.
//
// It is not an escape hatch for a copy. A reason here has to name a difference
// in the CONTRACT, not in the caller's convenience, and adding one without a
// difference is the finding this gate exists to make.
var statedReasons = gatekit.Waive(map[string]string{
	"internal/compose/replydraftmodel.go": "the customer reply draft is a different contract over the same envelope: " +
		"it refuses a subject carrying a line break and enforces the RFC length bounds its schema declares, " +
		"leaves the subject un-normalized because the caller sends it as typed, and splits parse from validate " +
		"so the retry validator judges the answer the caller will get. draftreply.Parse would trim and " +
		"plain-text the subject and drop both refusals.",
	"internal/compose/accountdraft/write.go": "an outward customer draft carries its greeting INSIDE the body, " +
		"and the model runs it into the first line often enough that reading the reply means splitting it " +
		"(draftfloor.SplitGreetingLine over the recipient's name) before anything can ask whether the body is " +
		"empty. What counts as the body differs, so the refusal cannot be shared until the helper can express " +
		"that transform — a candidate for adoption, not a difference of taste.",
	"internal/compose/persondraft/write.go": "the same outward-draft contract as accountdraft: the greeting is " +
		"split out of the body before the empty check, so what the refusal judges is not the text the model " +
		"returned. These two are also each other's copy, which is its own finding and its own change.",
})

// contractsPrefix is generated from backend/api/crm.yaml. Its envelopes are the
// PUBLIC shape, not a reading of a model reply, and it is never hand-edited.
const contractsPrefix = "internal/contracts"

func TestASubjectBodyModelReplyHasOneReader(t *testing.T) {
	t.Parallel()
	var readers []string
	ownerSeen := false

	// "internal", not "..": the gates package runs with the module directory as
	// its cwd, and walking the repo root sweeps every sibling git worktree on
	// this machine — their copies of these files would be reported as this
	// tree's, which is a finding about somebody else's branch.
	err := filepath.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		reads := true
		for _, marker := range replyEnvelopeMarkers {
			if !marker.Match(body) {
				reads = false
				break
			}
		}
		if !reads {
			return nil
		}
		rel := filepath.ToSlash(path)
		switch {
		case rel == draftReplyOwner:
			ownerSeen = true
		case statedReasons.Waived(t, rel):
			// Says why it does not share the helper; the assertion below holds
			// the register to reasons that name a contract difference.
		case strings.HasPrefix(rel, contractsPrefix):
			// The wire contract, generated from backend/api/crm.yaml.
		case strings.Contains(rel, "/certcase_"):
			// The certification harness reads a reply to SCORE it, which is the
			// one place a second reading is the point: a case that read the
			// answer through the production parse would certify the parse
			// rather than the model.
		default:
			readers = append(readers, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}

	if !ownerSeen {
		t.Fatalf("the scan never found %s, so it is reading a tree this gate does not describe "+
			"— a census that can fail short has already failed", draftReplyOwner)
	}
	// A stated reason describing a reader that is gone is a claim about a tree
	// this one no longer is.
	statedReasons.AssertAllMatched(t)
	if len(readers) > 0 {
		t.Errorf("a {subject, body} model reply is read outside compose/draftreply:\n\t%s\n\n"+
			"Read it through draftreply.Parse. Two readers of one reply is how the same "+
			"defect reached two sites: each refused what neither prompt asked for, and "+
			"fixing one left the other live.", strings.Join(readers, "\n\t"))
	}
}
