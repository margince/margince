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
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
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
	sendPermissionComponent = "../frontend/src/components/sendpermission.tsx"
	// composerSurface is the file this walk must always see. A count would follow
	// how many composers the app has. A name fails when the walk goes blind, and
	// it says which file to look for. The composer's send hook names the door,
	// and compose.tsx renders the answer.
	composerSurface = "features/mail/composesend.ts"
)

var pinnedSendDoors = []string{"/activities/{id}/send-email", "/activities/{id}/send-message", "/emails"}

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

var contractSchemaRef = regexp.MustCompile(`\$ref:\s*["']?#/components/schemas/([A-Za-z][A-Za-z0-9]*)["']?`)

// operationKeyLine matches a key at an operation's own depth; requestBody is one,
// and the next one ends it. Only a body counts: a scheduled message echoes the
// reason back to its sender, and that response is the outbox, not a send.
var operationKeyLine = regexp.MustCompile(`^ {0,6}[a-zA-Z/]`)

// requestBodyLine is a prefix, so a body written in flow style on its own line,
// or followed by a comment, still counts.
var requestBodyLine = regexp.MustCompile(`^ {6}requestBody:`)

// senderReasonProperty is the field that marks a send request body: the
// sender's own word about why they are writing, recorded and granting nothing.
// Any depth and flow style count, since a body may nest it or spell it inline.
var senderReasonProperty = regexp.MustCompile(`\boperator_reason\s*:`)

// sharedRequestBody is a body this scan cannot read, so it refuses one.
var sharedRequestBody = regexp.MustCompile(`#/components/requestBodies/`)

// sendDoors reads the contract for every path whose request body carries the
// sender's reason. Those are the doors a sender's own word reaches the engine by.
func sendDoors(t *testing.T) []string {
	t.Helper()
	contract, err := os.ReadFile(sendPermissionContract)
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	doors, err := scanSendDoors(string(contract))
	if err != nil {
		t.Fatal(err)
	}
	return doors
}

// sendDoorScan is what one pass over the contract learns.
type sendDoorScan struct {
	refsByPath          map[string][]string
	inlineDoors         map[string]bool
	carriesSenderReason map[string]bool
	refsOfSchema        map[string][]string
}

func scanSendDoors(contract string) ([]string, error) {
	scan := sendDoorScan{
		refsByPath: map[string][]string{}, inlineDoors: map[string]bool{},
		carriesSenderReason: map[string]bool{}, refsOfSchema: map[string][]string{},
	}
	var path, schema string
	inSchemas, inBody := false, false
	for line := range strings.SplitSeq(contract, "\n") {
		if operationKeyLine.MatchString(line) {
			inBody = requestBodyLine.MatchString(line)
		}
		switch {
		case line == "components:":
			inSchemas, path = true, ""
		case !inSchemas && contractPathLine.MatchString(line):
			path = contractPathLine.FindStringSubmatch(line)[1]
		case !inSchemas && path != "" && inBody:
			if sharedRequestBody.MatchString(line) {
				return nil, fmt.Errorf("%s takes a shared request body, which this scan cannot read: "+
					"inline it, or teach scanSendDoors to follow #/components/requestBodies", path)
			}
			if senderReasonProperty.MatchString(line) {
				scan.inlineDoors[path] = true
			}
			for _, m := range contractSchemaRef.FindAllStringSubmatch(line, -1) {
				scan.refsByPath[path] = append(scan.refsByPath[path], m[1])
			}
		case inSchemas && contractSchemaLine.MatchString(line):
			schema = contractSchemaLine.FindStringSubmatch(line)[1]
		case inSchemas && schema != "":
			if senderReasonProperty.MatchString(line) {
				scan.carriesSenderReason[schema] = true
			}
			for _, m := range contractSchemaRef.FindAllStringSubmatch(line, -1) {
				scan.refsOfSchema[schema] = append(scan.refsOfSchema[schema], m[1])
			}
		}
	}
	return scan.doors(), nil
}

// doors follows each body's schemas one reference down, which is how an allOf
// composes a send body from a shared part that carries the reason.
func (s sendDoorScan) doors() []string {
	carries := func(schema string) bool {
		return s.carriesSenderReason[schema] ||
			slices.ContainsFunc(s.refsOfSchema[schema], func(part string) bool { return s.carriesSenderReason[part] })
	}
	doors := slices.Collect(maps.Keys(s.inlineDoors))
	for p, refs := range s.refsByPath {
		if !s.inlineDoors[p] && slices.ContainsFunc(refs, carries) {
			doors = append(doors, p)
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

// importStatement matches one import statement, across lines, with whether it
// imports only types and the module it names.
var importStatement = regexp.MustCompile(`(?s)\bimport\s+(type\s+)?[^;]*?\bfrom\s+"([^"]+)"`)

// importsValueFrom reports whether source imports a value from the module
// named base. A type-only import carries nothing that can send.
func importsValueFrom(source, base string) bool {
	for _, m := range importStatement.FindAllStringSubmatch(source, -1) {
		if m[1] == "" && path.Base(m[2]) == base {
			return true
		}
	}
	return false
}

// hookAsksTheEngine reports whether a module that renders nothing is used only
// by surfaces that ask the engine. A hook names the door, and the component
// that calls it is what a rep sees, so that component must render the answer.
func hookAsksTheEngine(rel string, sources map[string]string) bool {
	if !strings.HasSuffix(rel, ".ts") {
		return false
	}
	base := strings.TrimSuffix(path.Base(rel), ".ts")
	importers := 0
	for other, source := range sources {
		if other == rel || !importsValueFrom(source, base) {
			continue
		}
		if !asksTheEngine(source) {
			return false
		}
		importers++
	}
	return importers > 0
}

// isSendSurfaceSource is the frontend production source the census sweeps:
// TypeScript that renders, rather than a file that describes what renders.
func isSendSurfaceSource(rel string) bool {
	if !strings.HasSuffix(rel, ".ts") && !strings.HasSuffix(rel, ".tsx") {
		return false
	}
	return !describesRatherThanRenders(rel)
}

// sendSurfaceSources reads every frontend production source the census sweeps,
// keyed by its path under src.
func sendSurfaceSources(t *testing.T) map[string]string {
	t.Helper()
	sources := map[string]string{}
	walkErr := filepath.WalkDir(sendPermissionFrontend, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(sendPermissionFrontend, file)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if !isSendSurfaceSource(rel) {
			return nil
		}
		raw, readErr := os.ReadFile(file)
		if readErr != nil {
			return readErr
		}
		sources[rel] = string(raw)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking the frontend: %v", walkErr)
	}
	return sources
}

func TestEverySurfaceThatSendsAsksTheEngineFirst(t *testing.T) {
	t.Parallel()

	doors := sendDoors(t)
	// Under-recognition is the failure that reports PASS. The mail reply, the
	// channel reply and the account-started mail all carry the field, and each
	// is named so a lost door fails saying which.
	for _, door := range pinnedSendDoors {
		if !slices.Contains(doors, door) {
			t.Fatalf("the contract scan no longer derives the send door %s: the census has stopped "+
				"seeing its subject (derived %v)", door, doors)
		}
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

	sources := sendSurfaceSources(t)
	var seen, posting []string
	for _, rel := range slices.Sorted(maps.Keys(sources)) {
		source := sources[rel]
		if postsThroughADoor(source, doors) {
			posting = append(posting, rel)
		}
		if !namesADoor(source, doors) {
			continue
		}
		seen = append(seen, rel)
		if asksTheEngine(source) || hookAsksTheEngine(rel, sources) || silentSendSurfaces.Waived(t, rel) {
			continue
		}
		t.Errorf("%s posts to a send door and never asks the engine: a rep learns a refusal by "+
			"pressing Send. Render SendPermission fed by useSendPermission, or ratify the surface in "+
			"silentSendSurfaces with what its silence costs", rel)
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

// A hook passes only when every component that calls it renders the answer.
func TestASendHookIsJudgedByTheComponentsThatCallIt(t *testing.T) {
	t.Parallel()
	asks := `import { SendPermission } from "./sendpermission";
import { useSend } from "./composesend";
export const A = () => <SendPermission />;`
	silent := `import { useSend } from "./composesend";
export const B = () => null;`
	typeOnly := `import type { Refusal } from "./composesend";
export const C = () => null;`
	cases := []struct {
		name    string
		rel     string
		sources map[string]string
		want    bool
	}{
		{"every caller asks", "screens/composesend.ts", map[string]string{"screens/a.tsx": asks}, true},
		{
			"one caller is silent", "screens/composesend.ts",
			map[string]string{"screens/a.tsx": asks, "screens/b.tsx": silent},
			false,
		},
		{
			"a type import uses no door", "screens/composesend.ts",
			map[string]string{"screens/a.tsx": asks, "screens/c.tsx": typeOnly},
			true,
		},
		{"no caller proves nothing", "screens/composesend.ts", map[string]string{"screens/c.tsx": typeOnly}, false},
		{"a component is judged on its own", "screens/composesend.tsx", map[string]string{"screens/a.tsx": asks}, false},
	}
	for _, c := range cases {
		if got := hookAsksTheEngine(c.rel, c.sources); got != c.want {
			t.Errorf("%s: hookAsksTheEngine = %v, want %v", c.name, got, c.want)
		}
	}
}

// Each shape a send body can take is found as a door or refused by name. None
// may pass unseen.
func TestTheSendDoorScanFindsEveryBodyShapeOrRefusesIt(t *testing.T) {
	t.Parallel()
	const probeBody = "paths:\n  /probe:\n    post:\n      requestBody:\n        content:\n          application/json:\n"
	const answered = "      responses:\n        '200': { description: ok }\ncomponents:\n  schemas:\n"
	cases := []struct {
		name, contract string
		doors          []string
		refusal        string
	}{
		{
			name: "an inline body carrying the reason is a door",
			contract: probeBody + "            schema:\n              type: object\n              properties:\n" +
				"                operator_reason: { type: string }\n" + answered,
			doors: []string{"/probe"},
		},
		{
			name: "a shared request body is refused by path",
			contract: "paths:\n  /probe:\n    post:\n      requestBody: { $ref: '#/components/requestBodies/Probe' }\n" +
				"components:\n  schemas:\n",
			refusal: "/probe takes a shared request body",
		},
		{
			name: "a body composed from a part carrying the reason is a door",
			contract: probeBody + "            schema: { $ref: '#/components/schemas/ProbeBody' }\n" + answered +
				"    ProbeBody:\n      allOf:\n        - $ref: '#/components/schemas/SenderPart'\n" +
				"    SenderPart:\n      type: object\n      properties:\n        operator_reason: { type: string }\n",
			doors: []string{"/probe"},
		},
		{
			name: "a double-quoted body reference is followed",
			contract: probeBody + "            schema: { $ref: \"#/components/schemas/SendBody\" }\n" + answered +
				"    SendBody:\n      type: object\n      properties:\n        operator_reason: { type: string }\n",
			doors: []string{"/probe"},
		},
		{
			name: "a double-quoted part of a composed body is followed",
			contract: probeBody + "            schema: { $ref: '#/components/schemas/ProbeBody' }\n" + answered +
				"    ProbeBody:\n      allOf:\n        - $ref: \"#/components/schemas/SenderPart\"\n" +
				"    SenderPart:\n      type: object\n      properties:\n        operator_reason: { type: string }\n",
			doors: []string{"/probe"},
		},
		{
			name: "a response echoing the reason is not a door",
			contract: "paths:\n  /probe:\n    get:\n      responses:\n        '200':\n          content:\n" +
				"            application/json:\n              schema: { $ref: '#/components/schemas/Echo' }\n" +
				"components:\n  schemas:\n    Echo:\n      properties:\n        operator_reason:\n          type: string\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doors, err := scanSendDoors(tc.contract)
			if tc.refusal != "" {
				if err == nil || !strings.Contains(err.Error(), tc.refusal) {
					t.Fatalf("scan answered %v, %v; want a refusal containing %q", doors, err, tc.refusal)
				}
				return
			}
			if err != nil {
				t.Fatalf("scan refused: %v", err)
			}
			if !slices.Equal(doors, tc.doors) {
				t.Errorf("doors = %v, want %v", doors, tc.doors)
			}
		})
	}
}
