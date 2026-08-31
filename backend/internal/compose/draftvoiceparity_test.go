// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every surface that drafts an email a person sends under their own name reads
// that person's own voice, and this is what keeps that true.
//
// Before this, only the reply drafter did. The two composers carried a comment
// saying draftrules "carries the user's own voice instead" — it does not, and
// nothing failed when it stopped being true, because nothing had ever checked.
// The result reached users as the same rep getting two different writers
// depending on which button they pressed.
//
// WHICH surfaces is derived from the tree, for the reason the shared-rules
// census next door is: a hand-written list is a thing to forget to add to, and
// forgetting is the failure. The corpus is the same one — every production file
// under compose that composes draftrules.Shared — because a file that writes
// correspondence under the shared rules is exactly a file that writes it in
// somebody's name.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/accountdraft"
	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/compose/persondraft"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// newDraftFence is the boundary a composer call declares. One per call in
// production; one per test here, for the same reason.
func newDraftFence() promptfence.Fence { return promptfence.New() }

// voicedComposerPrompts is each composer's system turn WITH a profile loaded.
// Both, always: the two were written from one template, and a rule applied to
// one of them is exactly the drift a parity gate exists to catch.
func voicedComposerPrompts(fence promptfence.Fence) map[string]string {
	return map[string]string{
		"the person composer":  persondraft.VoicedSystemPromptFor(fence),
		"the account composer": accountdraft.VoicedSystemPromptFor(fence),
	}
}

// unvoicedComposerPrompts is each composer's system turn with no profile.
func unvoicedComposerPrompts(fence promptfence.Fence) map[string]string {
	return map[string]string{
		"the person composer":  persondraft.SystemPromptFor(fence),
		"the account composer": accountdraft.SystemPromptFor(fence),
	}
}

// aiVoiceProfileWithPersonality is a fixture profile carrying the one field a
// human writes by hand.
func aiVoiceProfileWithPersonality(personality string) ai.VoiceProfile {
	return ai.VoiceProfile{PersonalityMD: personality}
}

// aiVoiceVersionWithExemplar is a fixture active version carrying the derived
// profile the builder writes AND one verbatim excerpt of the author's own mail.
//
// The exemplar is stored the way the voice store hands it back — inside
// profile_json — so the block under test is assembled by the same decoder
// production assembles it with, rather than by a shape only this test produces.
func aiVoiceVersionWithExemplar(profileMD, exemplar string) ai.VoiceProfileVersion {
	return ai.VoiceProfileVersion{
		ProfileVersion: 1,
		VoiceProfileMD: profileMD,
		ProfileJSON: map[string]any{
			"exemplars": []any{map[string]any{
				"register": "du", "kind": "email", "text": exemplar,
			}},
		},
	}
}

// Every drafting surface reaches the sender's voice seam.
//
// It asserts the IMPORT rather than a rendered prompt, and that is the honest
// bar for a census: a voiced prompt only exists when a profile is loaded, so
// there is no assembled string to compare for a surface that has none. What can
// be checked without a profile is whether the surface can reach the seam at
// all, and one that cannot is one no profile could ever reach.
//
// The prompt itself is checked where a profile exists to render:
// TestAVoicedComposerPromptCarriesTheProfile below.
//
// A SURFACE, not a file. The reply drafter composes the shared rules in
// replydraftmodel.go and loads the voice in replydraftvoice.go beside it, which
// is one surface split across two files for length rather than two surfaces. So
// the unit is the directory: a file that composes the rules is satisfied by any
// production file in its own package reaching draftvoice. Anything wider would
// let compose's several hundred files answer for each other.
func TestEveryDraftingSurfaceLoadsTheSenderVoice(t *testing.T) {
	surfaces := filesComposingTheSharedRules(t)
	if len(surfaces) == 0 {
		t.Fatal("no file under internal/compose composes draftrules.Shared, so this gate is reading an " +
			"empty tree rather than a governed one")
	}
	for _, where := range surfaces {
		if !packagePassesOnALoadedVoice(t, where) {
			t.Errorf("%s drafts correspondence a person sends under their own name, but nothing in its "+
				"package passes a draftvoice.Load result on to anything — so a rep who has built a voice "+
				"profile is written by a generic writer here and by their own voice elsewhere. Load it "+
				"with draftvoice.Load and hand the result to the call that renders the prompt", where)
		}
	}
}

// packagePassesOnALoadedVoice reports whether some production file in the
// package holding path calls draftvoice.Load AND uses the result — as an
// argument, or assigned to a name that is later used.
//
// The USE, not just the call. A bare draftvoice.Load(...) whose answer is
// discarded compiles, satisfies a call census, and leaves every draft unvoiced;
// so does a live call sitting in a function the drafting path never reaches.
// This cannot chase the second case — a static reader does not know which
// functions run — and it is not the whole guarantee on its own. The behaviour
// is pinned by TestAVoicedComposerPromptCarriesTheProfile, which drives each
// composer's real request builder; this sweep is what notices a NEW surface
// that never wired the seam at all.
func packagePassesOnALoadedVoice(t *testing.T, path string) bool {
	t.Helper()
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the package at %s: %v", dir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gen.go") {
			continue
		}
		if fileUsesALoadedVoice(t, filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

// fileUsesALoadedVoice reports whether the file calls draftvoice.Load in a
// position where its answer goes somewhere: returned to a caller, passed as an
// argument to another call, or bound to a name that is read again.
func fileUsesALoadedVoice(t *testing.T, path string) bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	used := false
	ast.Inspect(parsed, func(n ast.Node) bool {
		if used {
			return false
		}
		switch node := n.(type) {
		case *ast.ReturnStmt:
			// Handed back to a caller: return draftvoice.Load(...). The reply
			// drafter's loadVoice is this shape, and a reader that knew only
			// about arguments and assignments would report it as unwired.
			for _, result := range node.Results {
				if isVoiceLoadCall(result) {
					used = true
					return false
				}
			}
		case *ast.CallExpr:
			// Passed straight into another call: Write(ctx, lane, in, Load(...)).
			for _, arg := range node.Args {
				if isVoiceLoadCall(arg) {
					used = true
					return false
				}
			}
		case *ast.AssignStmt:
			// Bound to a name, which must then be read somewhere in the file.
			for i, rhs := range node.Rhs {
				if !isVoiceLoadCall(rhs) || i >= len(node.Lhs) {
					continue
				}
				name, ok := node.Lhs[i].(*ast.Ident)
				if ok && name.Name != "_" && identReadElsewhere(parsed, name) {
					used = true
					return false
				}
			}
		}
		return true
	})
	return used
}

// isVoiceLoadCall reports whether an expression is a draftvoice.Load call.
func isVoiceLoadCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Load" {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "draftvoice"
}

// identReadElsewhere reports whether the name is genuinely read again away from
// where it was defined — a value assigned and never read is a value discarded.
//
// A mention under `_ = name` does not count. That statement exists to silence
// the compiler about an unused variable, so counting it would let the one shape
// it appears in — a loaded voice deliberately parked — read as a voice in use.
func identReadElsewhere(file *ast.File, defined *ast.Ident) bool {
	discards := blankAssignedNames(file)
	read := false
	ast.Inspect(file, func(n ast.Node) bool {
		if read {
			return false
		}
		ident, ok := n.(*ast.Ident)
		if !ok || ident.Name != defined.Name || ident.Pos() == defined.Pos() {
			return true
		}
		if discards[ident.Pos()] {
			return true
		}
		read = true
		return false
	})
	return read
}

// blankAssignedNames collects the positions of identifiers that appear only as
// the right-hand side of a `_ = name` statement.
func blankAssignedNames(file *ast.File) map[token.Pos]bool {
	out := map[token.Pos]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			name, ok := lhs.(*ast.Ident)
			if !ok || name.Name != "_" || i >= len(assign.Rhs) {
				continue
			}
			if rhs, ok := assign.Rhs[i].(*ast.Ident); ok {
				out[rhs.Pos()] = true
			}
		}
		return true
	})
	return out
}

// A voiced composer prompt actually carries the profile.
//
// The import census above proves a surface CAN reach the seam. This proves the
// reach lands: the block the model receives holds the author's own words, and
// the system turn states what that block is allowed to govern. Both composers
// are checked, because both were written from the same template and a fix
// applied to one of them is the failure this pair exists to catch.
func TestAVoicedComposerPromptCarriesTheProfile(t *testing.T) {
	// Every field that descends from the author's corpus, each with its own
	// recognisable text: an exemplar IS a verbatim excerpt of somebody's mail,
	// so a gate that checked only the hand-written personality would leave the
	// two fields most likely to quote a third party unwatched.
	const (
		personality = "I write short. I never say circling back."
		derived     = "Sentences run under twelve words."
		exemplar    = "Servus Martin, das Angebot geht morgen raus."
	)
	corpus := []string{personality, derived, exemplar}
	voice := draftvoice.Context{
		Profile: aiVoiceProfileWithPersonality(personality),
		Version: aiVoiceVersionWithExemplar(derived, exemplar),
		OK:      true,
	}
	// Through each composer's OWN request builder, not through Block alone: a
	// block that renders correctly and is never added to the user turn is the
	// defect this test is named for, and asserting on Block would pass on it.
	for what, req := range voicedComposerRequests(t, voice) {
		sent := userTurn(t, what, req)
		missing := false
		for _, want := range append(append([]string{}, corpus...), draftvoice.VocabularyRule) {
			if !strings.Contains(sent, want) {
				t.Errorf("%s sends no %q to the model, so it is told to write in a voice it was never "+
					"shown", what, want)
				missing = true
			}
		}
		// Absent text cannot be fenced, and reporting it as unfenced would send
		// the next reader after the wrong defect.
		if missing {
			continue
		}
		// The author's own words are DATA — a profile is corpus text, and
		// corpus text routinely quotes what somebody else wrote. So the
		// personality must sit between a fence open and the next close, never
		// loose in the turn.
		//
		// Read structurally rather than against a fence built here: each
		// request mints its OWN nonce, so a marker this test constructed would
		// never match the one the request carries.
		for _, text := range corpus {
			if !fencedIn(t, sent, text) {
				t.Errorf("%s carries the author's own %q outside this call's fence, so a profile built "+
					"from a mail somebody sent them could instruct the model", what, text)
			}
		}
		// The system turn is where instructions live. Corpus text there is an
		// instruction whatever it says, and no fence in the user turn can
		// undo that.
		for _, text := range corpus {
			if strings.Contains(req.System, text) {
				t.Errorf("%s copies the author's own %q into the system turn, where it reads as an "+
					"instruction rather than as data", what, text)
			}
		}
	}
}

// voicedComposerRequests is the model request each composer builds for a
// sender who has a voice profile.
func voicedComposerRequests(t *testing.T, voice draftvoice.Context) map[string]model.Request {
	t.Helper()
	person, err := persondraft.GroundedRequest(persondraft.Input{
		Recipient: persondraft.RecipientIn{ID: "p1", Name: "Marek", FirstName: "Marek"},
	}, voice)
	if err != nil {
		t.Fatalf("the person composer builds no request: %v", err)
	}
	account, err := accountdraft.GroundedRequest(accountdraft.Input{
		Company:   "Northwind",
		Recipient: accountdraft.RecipientIn{ID: "p1", Name: "Priya"},
	}, voice)
	if err != nil {
		t.Fatalf("the account composer builds no request: %v", err)
	}
	return map[string]model.Request{
		"the person composer":  person,
		"the account composer": account,
	}
}

// fencedIn reports whether every occurrence of text in turn sits inside a
// CLOSED fenced span: after an opening marker, and with that span's close
// still to come.
//
// Both halves are required. A check that only looked backwards would accept an
// opening marker never closed, which is not a boundary — everything after it,
// including the rest of the prompt, would read as one unterminated span.
//
// The markers are read OFF THE TURN, because the nonce is minted per call and a
// marker this test constructed would never match the one the request carries.
// The turn's own opening marker is found by asking promptfence to parse it,
// rather than by spelling the prefix here: a literal copy of that prefix would
// be a second spelling of it, and would go quietly permissive — matching
// nothing, so finding no unfenced text — the day the real one changed.
func fencedIn(t *testing.T, turn, text string) bool {
	t.Helper()
	opening, closing := markersOf(t, turn)
	idx := strings.Index(turn, text)
	if idx < 0 {
		return false
	}
	for idx >= 0 {
		before := turn[:idx]
		lastOpen := strings.LastIndex(before, opening)
		lastClose := strings.LastIndex(before, closing)
		if lastOpen < 0 || lastClose > lastOpen {
			return false
		}
		// The span must also END. Without this, fence.Open()+text — an opening
		// marker with no close — passes as fenced.
		after := turn[idx+len(text):]
		nextClose := strings.Index(after, closing)
		nextOpen := strings.Index(after, opening)
		if nextClose < 0 || (nextOpen >= 0 && nextOpen < nextClose) {
			return false
		}
		next := strings.Index(turn[idx+len(text):], text)
		if next < 0 {
			return true
		}
		idx = idx + len(text) + next
	}
	return true
}

// markersOf recovers the fence this turn was built with, from the turn.
//
// It reads the first "<…>" tag and hands it to promptfence.FromMarker, which
// answers only for a marker that package could have minted — so a tag from the
// payload rather than from the boundary is refused here rather than silently
// becoming the boundary this gate measures against.
func markersOf(t *testing.T, turn string) (opening, closing string) {
	t.Helper()
	start := strings.Index(turn, "<")
	for start >= 0 {
		end := strings.Index(turn[start:], ">")
		if end < 0 {
			break
		}
		tag := turn[start : start+end+1]
		if fence, ok := promptfence.FromMarker(strings.TrimSuffix(strings.TrimPrefix(tag, "<"), ">")); ok {
			return fence.Open(), fence.Close()
		}
		next := strings.Index(turn[start+1:], "<")
		if next < 0 {
			break
		}
		start = start + 1 + next
	}
	t.Fatal("the composer's user turn carries no fence marker promptfence could have minted, so there " +
		"is no boundary to measure the profile against")
	return "", ""
}

// userTurn is the one message a composer sends, which is where the profile has
// to arrive: a block in the system turn would be an instruction, and a voice
// profile is the author's own text.
func userTurn(t *testing.T, what string, req model.Request) string {
	t.Helper()
	if len(req.Messages) != 1 {
		t.Fatalf("%s sends %d messages; this gate reads the single user turn", what, len(req.Messages))
	}
	return req.Messages[0].Content
}

// An unvoiced surface says nothing about a profile.
//
// The mirror of the test above, and the one that catches the cheap fix: a
// system turn that always names the voice rule would pass a "carries the rule"
// check while pointing at a block that never arrives.
func TestAnUnvoicedComposerPromptNamesNoProfile(t *testing.T) {
	fence := newDraftFence()
	for what, system := range unvoicedComposerPrompts(fence) {
		if strings.Contains(system, draftvoice.SystemRule) {
			t.Errorf("%s tells the model to obey a voice profile even when none is loaded, so it points "+
				"at a block that never arrives in the user turn", what)
		}
	}
	for what, system := range voicedComposerPrompts(fence) {
		if !strings.Contains(system, draftvoice.SystemRule) {
			t.Errorf("%s sends a voice profile without telling the model what it governs, so nothing "+
				"stops a fact being bent to fit the author's phrasing", what)
		}
	}
}

// Margince's own personality stays out of correspondence.
//
// A draft goes out over the USER's name, so Margince's voice inside one would
// be Margince signing somebody else's mail. This is the rule the two composers'
// promptvoice:exempt waivers claim, asserted rather than trusted — a waiver's
// stated reason can be false, and this is the one that would be expensive.
func TestNoComposerPromptCarriesMargincesOwnVoice(t *testing.T) {
	fence := newDraftFence()
	// Merged under DISTINCT keys. Both maps are keyed by composer, so writing
	// one into the other replaced every voiced prompt with its unvoiced twin
	// and left the voiced half untested — which is the half a Margince-voice
	// regression would land in.
	prompts := map[string]string{}
	for what, system := range voicedComposerPrompts(fence) {
		prompts[what+" (voiced)"] = system
	}
	for what, system := range unvoicedComposerPrompts(fence) {
		prompts[what+" (unvoiced)"] = system
	}
	for what, system := range prompts {
		if strings.Contains(system, promptvoice.Heading) {
			t.Errorf("%s carries Margince's own voice block, which would have Margince's personality "+
				"sign a mail the user sends under their name", what)
		}
	}
}
