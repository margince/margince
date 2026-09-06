// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every surface that posts to a send door asks the engine first, through the
// one component that says what it answered.
//
// The engine decides whether a message may go, and the preview doors have
// answered that since it landed. A surface that sends without asking teaches a
// rep the refusal by pressing Send and reading an error — and a surface that
// asks through its OWN reading of the preview is a second spelling of a consent
// surface, which is the worst place for one: two spellings that drift mean one
// screen tells a rep they may send and another that they may not, about the
// same message.
//
// So every frontend file that names a send door either renders SendPermission
// or is ratified here with what its silence costs.
//
// The doors are DERIVED from the contract rather than listed: a send door is an
// operation whose request body carries `operator_reason`, the field through
// which a sender's own word about a send reaches the engine — recorded beside
// the decision, and granting nothing. A list here would be a second copy of the
// contract, and the copy that stopped matching would let a fourth door ship
// with no question asked.
//
// NOT asserted: a surface that stages a send without posting to a door — the
// scheduled-send queue lists messages already staged, and the approval card
// releases them through a generic decide route. Both adopt the component today,
// and nothing derivable marks their routes as sends, so the census cannot see a
// third such surface arriving without one.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	sendPermissionContract  = "api/crm.yaml"
	sendPermissionFrontend  = "../frontend/src"
	sendPermissionComponent = "../frontend/src/screens/sendpermission.tsx"
	// composerSurface is the file this walk must always see. It stands where a
	// COUNT floor stood, deliberately: a number tracks how many composers the
	// app happens to have and gets edited down each time they consolidate, while
	// a name fails when the walk goes blind — the one way a census breaks
	// silently — and says which file to go looking for.
	composerSurface = "screens/compose.tsx"
)

// silentSendSurfaces ratifies each surface that posts to a send door without
// asking the engine first, with what the omission costs.
//
// Empty, and that is the answer rather than an absence: every surface that posts
// to a door renders the component, so there is nothing left to ratify.
// AssertAllMatched below is what keeps it that way — a key whose subject is gone
// fails here rather than standing as a permission for a file nobody can find.
var silentSendSurfaces = gatekit.Waive(map[string]string{})

// contractPathLine matches a path entry under `paths:`.
var contractPathLine = regexp.MustCompile(`^  (/[^\s:]+):\s*$`)

// contractSchemaLine matches a schema entry under `components: schemas:`.
var contractSchemaLine = regexp.MustCompile(`^    ([A-Za-z][A-Za-z0-9]*):\s*$`)

// contractSchemaRef matches a reference to a named schema anywhere under a
// path: request body, response or parameter alike. Over-recognition is the safe
// direction for a census — a response that carried the sender's reason would
// surface as a door and be looked at, where a narrower match that missed a
// request body would report PASS over an unasked send.
var contractSchemaRef = regexp.MustCompile(`\$ref: '#/components/schemas/([A-Za-z][A-Za-z0-9]*)'`)

// senderReasonProperty is the field that marks a schema as a send request body:
// the sender's own word about why they are writing, recorded and granting
// nothing.
var senderReasonProperty = regexp.MustCompile(`^        operator_reason:\s*$`)

// sendDoors reads the contract for every path whose schemas carry the sender's
// reason: the doors through which a sender's own word about a send reaches the
// engine.
func sendDoors(t *testing.T) []string {
	t.Helper()
	contract, err := os.ReadFile(sendPermissionContract)
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}

	refsByPath := map[string][]string{}
	carriesSenderReason := map[string]bool{}
	var path, schema string
	inSchemas := false
	for _, line := range strings.Split(string(contract), "\n") {
		switch {
		case line == "components:":
			inSchemas = true
			path = ""
		case !inSchemas && contractPathLine.MatchString(line):
			path = contractPathLine.FindStringSubmatch(line)[1]
		case !inSchemas && path != "":
			if m := contractSchemaRef.FindStringSubmatch(line); m != nil {
				refsByPath[path] = append(refsByPath[path], m[1])
			}
		case inSchemas && contractSchemaLine.MatchString(line):
			schema = contractSchemaLine.FindStringSubmatch(line)[1]
		case inSchemas && schema != "" && senderReasonProperty.MatchString(line):
			carriesSenderReason[schema] = true
		}
	}

	var doors []string
	for p, refs := range refsByPath {
		for _, ref := range refs {
			if carriesSenderReason[ref] {
				doors = append(doors, p)
				break
			}
		}
	}
	slices.Sort(doors)
	return doors
}

// namesADoor reports whether source names any of the doors as a whole string
// literal, whichever quote the file uses.
//
// The closing delimiter is what keeps `/emails` from matching the preview door
// `/emails:preview` that the hook itself names. The opening one is any of the
// three JavaScript quotes rather than the double quote the formatter enforces,
// because under-recognition is the one way a census must not fail: a surface
// written with backticks would otherwise be invisible and report PASS.
func namesADoor(source string, doors []string) bool {
	for _, door := range doors {
		if regexp.MustCompile("[\"'`]" + regexp.QuoteMeta(door) + "[\"'`]").MatchString(source) {
			return true
		}
	}
	return false
}

// clientPostPath captures the path a typed-client POST names. The generated
// client takes its path as a LITERAL — an interpolated one does not typecheck —
// so a surface cannot reach a door without spelling it here.
//
// This is the census's SECOND opinion, computed from the call rather than from a
// bare mention anywhere in the file, and the two are compared below. namesADoor
// asks "does this file contain the door as a whole quoted string"; a file that
// posts to `/emails/draft` names no door by that test while posting through one,
// and a door list that went stale against the contract makes every surface
// invisible at once. Either failure is silent in a single matcher and loud when
// two disagree.
var clientPostPath = regexp.MustCompile("api\\.POST\\(\\s*[\"'`]([^\"'`]+)")

// postsThroughADoor reports whether source POSTs to a path that IS a door or
// sits under one. The prefix half is what catches the surface namesADoor cannot
// see: a longer path through the same door.
func postsThroughADoor(source string, doors []string) bool {
	for _, m := range clientPostPath.FindAllStringSubmatch(source, -1) {
		for _, door := range doors {
			if m[1] == door || strings.HasPrefix(m[1], door+"/") {
				return true
			}
		}
	}
	return false
}

// rendersSendPermission matches the component's JSX tag and nothing that merely
// starts with its name: a `<SendPermissionBanner` of a surface's own would be
// the second spelling this gate exists to refuse.
var rendersSendPermission = regexp.MustCompile(`<SendPermission[\s/>]`)

// importsSendPermission matches the component's module wherever the surface
// sits: a screen beside it imports `./sendpermission`, a surface elsewhere
// reaches it through a longer path, and both are the one component.
var importsSendPermission = regexp.MustCompile(`from "[^"]*/sendpermission"`)

// asksTheEngine reports whether source renders the shared component.
func asksTheEngine(source string) bool {
	return importsSendPermission.MatchString(source) &&
		rendersSendPermission.MatchString(source)
}

// isSendSurfaceSource is the frontend production source the census sweeps:
// TypeScript that renders, rather than a file that describes what renders.
func isSendSurfaceSource(rel string) bool {
	if !strings.HasSuffix(rel, ".ts") && !strings.HasSuffix(rel, ".tsx") {
		return false
	}
	return !describesRatherThanRenders(rel)
}

func TestEverySurfaceThatSendsAsksTheEngineFirst(t *testing.T) {
	t.Parallel()

	doors := sendDoors(t)
	// Under-recognition is the failure that reports PASS. The mail reply, the
	// channel reply and the account-started mail all carry the field; fewer
	// means the scanner has stopped seeing its subject.
	if len(doors) < 3 {
		t.Fatalf("derived %d send door(s) from the contract, want at least the three that carry "+
			"operator_reason: the census has stopped seeing its subject (%v)", len(doors), doors)
	}

	// Deferred here, once the subject is fixed, so a fatal on the walk cannot
	// skip the stale-waiver report.
	defer silentSendSurfaces.AssertAllMatched(t)

	component, err := os.ReadFile(sendPermissionComponent)
	if err != nil {
		t.Fatalf("reading the shared component: %v", err)
	}
	if !strings.Contains(string(component), "export function SendPermission(") {
		t.Fatal("sendpermission.tsx no longer exports SendPermission: every surface that satisfies " +
			"this gate by naming it now renders something else, or nothing")
	}

	var seen, posting []string
	walkErr := filepath.WalkDir(sendPermissionFrontend, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(sendPermissionFrontend, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if !isSendSurfaceSource(rel) {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		source := string(raw)
		if postsThroughADoor(source, doors) {
			posting = append(posting, rel)
		}
		if !namesADoor(source, doors) {
			return nil
		}
		seen = append(seen, rel)
		if asksTheEngine(source) || silentSendSurfaces.Waived(t, rel) {
			return nil
		}
		t.Errorf("%s posts to a send door and never asks the engine: a rep learns a refusal by "+
			"pressing Send. Render SendPermission fed by useSendPermission, or ratify the surface in "+
			"silentSendSurfaces with what its silence costs", rel)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking the frontend: %v", walkErr)
	}
	// A surface the CALL sees and the mention does not is one this gate stopped
	// asking about, which is the failure that reports PASS. The reverse is fine
	// and expected: sendpermission.tsx names the preview door to ask about it and
	// posts to nothing.
	for _, rel := range posting {
		if !slices.Contains(seen, rel) {
			t.Errorf("%s posts through a send door that namesADoor cannot see, so the gate never "+
				"asked whether it renders SendPermission: widen the match or name the door as a "+
				"whole string literal", rel)
		}
	}
	// Both matchers going blind at once — a door list that stopped resolving
	// against the contract does exactly that — leaves the two agreeing on
	// nothing at all, which the comparison above reads as success.
	if !slices.Contains(seen, composerSurface) {
		t.Fatalf("the walk saw %v and not %s: the census has stopped seeing its subject",
			seen, composerSurface)
	}
}
